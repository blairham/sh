// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// InstalledFunctionLibrary is the function library of a zsh installed on this
// machine — the directories its own default `$fpath` ends in — or nil when
// there is none to find.
//
// The maintainer's decision on #6128 (2026-10-05): when an installed zsh's
// function directory exists, it goes on this shell's default `$fpath` after
// this installation's own two directories. That is what makes zsh's real
// completion system reachable — `_main_complete` and the completers a real
// `~/.zshrc` configures with its zstyles — and what is found is **run** as a
// user-installed script, the way any file on `$fpath` is. Nothing here opens
// a file to read it: the layout is learned from directory names alone.
//
// # Where it looks, without starting zsh
//
// The honest answer to "where is zsh's library" is `zsh -fc 'print $fpath'`,
// and that is a fork and an exec on every start of this shell. So the answer
// is reconstructed from the layouts zsh is installed in, which are few and
// were each measured as real zsh's own default `$fpath` (`env -u FPATH zsh -f`):
//
//	Homebrew, macOS     /opt/homebrew/Cellar/zsh/5.9.2/share/zsh/functions
//	                    (also linked as /opt/homebrew/share/zsh/functions)
//	Apple, /bin/zsh     /usr/share/zsh/5.9/functions
//	Debian sid          /usr/share/zsh/functions/Calendar, …/Completion,
//	                    …/Completion/Base, … — 29 directories, the source
//	                    tree's own shape, and not the top directory itself
//
// Three questions in order, and the first answer wins:
//
//  1. **The package managers' roots** the binary names — `/opt/homebrew`,
//     `/usr/local` and so on, the way [SystemStartupDirectory] is handed
//     `/etc`. A zsh somebody installed on purpose is the one they mean when
//     the operating system's is also there, and on macOS it always is.
//  2. **The zsh on `$PATH`.** The first executable called `zsh` on the
//     search path the shell started with, resolved through its links, names
//     an installation no root covers — Nix, a private prefix — by its
//     conventional `bin`/`share` split: two directories up from the binary
//     is its prefix. It is second and not first because the `$PATH` a login
//     shell starts with is the system's, before any startup file has put a
//     package manager on it, so on a Mac it names Apple's `/bin/zsh` for
//     everybody.
//  3. **The system's own root**, last, for a machine whose zsh came with the
//     operating system and is not on the search path the shell was given.
//
// The roots are absolute paths into a real machine, so they are the
// caller's: a test hands a scratch tree, and the dialect binaries name the
// real one through [SystemFunctionDirectories].
//
// Under a prefix the library is `share/zsh/functions` if that is a
// directory, and otherwise the highest-versioned `share/zsh/<version>/functions`
// — Apple's build has the version segment and Homebrew's does not, and a
// machine holding two versions side by side wants the newer one.
//
// **And the installation's own site directories come in front of its
// library**, the way they do in the installation's own `$fpath`:
// the conventional `/usr/local/share/zsh/site-functions`, the prefix's
// `share/zsh/site-functions`, and Debian's `vendor-functions` and
// `vendor-completions`, each where it exists. Those hold the completions
// *other* programs install for zsh — git's, brew's, docker's — which are not
// zsh's code at all, and real zsh lets them win over its own library's of
// the same name. Without them the library's `_git` answered `git <Tab>`
// where real zsh's answer is git's own, measured 2026-10-05 against this
// machine's startup file: zsh's grouped listing of 158 subcommands here,
// git's `-- common commands --` there.
//
// **One installation, never a mixture.** The completion system is hundreds of
// functions that call one another, and half of one version's with half of
// another's is a library nobody tested. So the search stops at the first
// installation it finds.
//
// The `zsh` on `$PATH` may be this shell itself, installed under that name.
// That needs no special case: its prefix has no `share/zsh`, so the question
// falls through to the system's root, which is the right answer — the
// library a person would get from *another* installed zsh.
//
// # What it costs
//
// Two `stat`s per root until one answers — its `share/zsh/functions`, then
// an attempt to list `share/zsh` for a versioned one, which fails at once
// where there is no `share/zsh` — and the `$PATH` walk only when no root
// did. No library directory is listed on the common layouts, because a flat
// library is recognized by its top directory alone: only the unflattened
// Debian tree is walked, and it is recognized first by one more `stat`, of
// its `Completion` subdirectory, so the 1235-entry Homebrew directory is
// never read. Measured in docs/spec/functions.md.
func InstalledFunctionLibrary(path string, roots LibraryRoots) []string {
	for _, root := range roots.Managers {
		if dirs := installationUnder(root, roots.LocalSite); dirs != nil {
			return dirs
		}
	}
	if dirs := installationOnPath(path, roots.LocalSite); dirs != nil {
		return dirs
	}
	return installationUnder(roots.System, roots.LocalSite)
}

// LibraryRoots is where [InstalledFunctionLibrary] looks: absolute paths into
// a real machine, which is why they are the caller's to name.
type LibraryRoots struct {
	// Managers are the prefixes a package manager installs zsh under, most
	// preferred first.
	Managers []string
	// System is the operating system's own prefix, asked last.
	System string
	// LocalSite is the site directory every build of zsh carries whatever
	// its prefix — `/usr/local/share/zsh/site-functions` — put in front of
	// the installation's own where it exists.
	LocalSite string
}

// installationOnPath is the installation of the first `zsh` on a search path,
// read off where that binary is installed.
func installationOnPath(path, localSite string) []string {
	for _, dir := range filepath.SplitList(path) {
		if dir == "" || !filepath.IsAbs(dir) {
			// A relative entry names a different file from every working
			// directory, and the empty one is the working directory: neither
			// is an installation.
			continue
		}
		candidate := filepath.Join(dir, "zsh")
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return nil
		}
		// The first one is the one that runs, so it is the only one asked:
		// a later `zsh` on the path is a shell the person does not get by
		// typing the name.
		prefix := filepath.Dir(filepath.Dir(resolved))
		if prefix == string(filepath.Separator) {
			// `/bin/zsh` is macOS's own, and its library is under `/usr`:
			// there is no `/share`. (Linux's `/bin` is a link to `/usr/bin`
			// and has already resolved there.)
			prefix = "/usr"
		}
		return installationUnder(prefix, localSite)
	}
	return nil
}

// installationUnder is what an installation under prefix puts on its
// `$fpath` — its site directories, then its library — or nil when there is
// no library there. A prefix with site directories and no library is not an
// installation of zsh: it is where some other program put a completion.
func installationUnder(prefix, localSite string) []string {
	lib := libraryUnder(prefix)
	if lib == nil {
		return nil
	}
	var dirs []string
	for _, site := range []string{
		localSite,
		filepath.Join(prefix, "share", "zsh", "site-functions"),
		filepath.Join(prefix, "share", "zsh", "vendor-functions"),
		filepath.Join(prefix, "share", "zsh", "vendor-completions"),
	} {
		if site != "" && !slices.Contains(dirs, site) && isDir(site) {
			dirs = append(dirs, site)
		}
	}
	return append(dirs, lib...)
}

// libraryUnder is the function library an installation under prefix has, or
// nil.
func libraryUnder(prefix string) []string {
	if prefix == "" {
		return nil
	}
	share := filepath.Join(prefix, "share", "zsh")
	if lib := filepath.Join(share, "functions"); isDir(lib) {
		return libraryDirs(lib)
	}
	entries, err := os.ReadDir(share)
	if err != nil {
		return nil
	}
	best, bestVersion := "", []int(nil)
	for _, e := range entries {
		version, ok := parseVersion(e.Name())
		if !ok {
			continue
		}
		lib := filepath.Join(share, e.Name(), "functions")
		if !isDir(lib) {
			continue
		}
		if best == "" || versionLess(bestVersion, version) {
			best, bestVersion = lib, version
		}
	}
	if best == "" {
		return nil
	}
	return libraryDirs(best)
}

// libraryDirs is the directories of one library, in the order zsh's own
// `$fpath` lists them.
//
// A flattened library is its own directory. Debian installs the unflattened
// source tree instead, and its zsh lists every directory under the top one
// and not the top one itself — each parent before its children, siblings in
// name order — which is a walk.
func libraryDirs(lib string) []string {
	if !isDir(filepath.Join(lib, "Completion")) {
		return []string{lib}
	}
	var dirs []string
	_ = filepath.WalkDir(lib, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subdirectory is left out, the way `autoload`
			// walks past an entry it cannot read.
			if d != nil && d.IsDir() && p != lib {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() && p != lib {
			dirs = append(dirs, p)
		}
		return nil
	})
	return dirs
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// parseVersion reads a directory name like `5.9` or `5.9.2` as its numbers.
func parseVersion(name string) ([]int, bool) {
	parts := strings.Split(name, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		nums[i] = n
	}
	return nums, true
}

// versionLess orders versions numerically, so `5.10` is newer than `5.9`.
func versionLess(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// SystemFunctionDirectories is [InstalledFunctionLibrary] over the roots a
// package manager installs zsh under — Homebrew on Apple silicon and on
// Intel, MacPorts, Linuxbrew — with the system's own `/usr` last.
//
// It is the one place the roots are named, so the `zsh` binary and
// `sh -dialect zsh` cannot come to look in different places; see
// driver.Shell.SystemFunctionDirectories.
func SystemFunctionDirectories(path string) []string {
	return InstalledFunctionLibrary(path, LibraryRoots{
		Managers:  []string{"/opt/homebrew", "/usr/local", "/opt/local", "/home/linuxbrew/.linuxbrew"},
		System:    "/usr",
		LocalSite: "/usr/local/share/zsh/site-functions",
	})
}
