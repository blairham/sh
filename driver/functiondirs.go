// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/blairham/sh/interp"
)

// Where a shell looks for function definition files when nothing has told it.
//
// One dialect has such a search — see Semantics.FunctionSearchVariable — and
// it arrives with a value rather than with nothing. Measured 2026-09-07
// against zsh 5.9.2 under `env -i` with a scratch HOME and `-f`, so no startup
// file is speaking:
//
//	FPATH=/usr/local/share/zsh/site-functions
//	     :/opt/homebrew/share/zsh/site-functions
//	     :/opt/homebrew/Cellar/zsh/5.9.2/share/zsh/functions
//
// Three facts in that, and each one decided something here.
//
// **It is that installation's own directories, not the machine's.** The same
// machine's other zsh — Apple's 5.9 at /bin/zsh — answers
// `/usr/local/share/zsh/site-functions`, `/usr/share/zsh/site-functions` and
// `/usr/share/zsh/5.9/functions` instead. Two builds of one shell, one
// machine, and they disagree about the layout as well as the prefix: the
// Homebrew build has no version segment under `share/zsh` and Apple's does. So
// there is no expression that derives one installation's directories from
// another's, and a table of paths — in a dialect or anywhere else — would be
// the recording machine's rather than any machine's. What a *portable binary*
// can honestly know about an installation is where it was itself installed,
// which is what this reads.
//
// **A missing directory is not an error.** `/usr/local/share/zsh/site-
// functions` does not exist on the machine that measurement was made on, and
// the shell carries it anyway: the entry is a convention for whoever installs
// there next, and `autoload` already walks past an entry it cannot read. So
// nothing here stats anything, and an installation with no functions in it
// yet is a search that finds nothing rather than a startup that complains.
//
// **The environment replaces it rather than adding to it.** `FPATH=/x/y` in
// the environment gives `$fpath` exactly one element, and `FPATH=` — present
// and empty — gives it one empty element rather than the three. So the test
// below is whether the environment *mentions* the name, not whether what it
// says is worth anything.
//
// What this does **not** do is point the search at another shell's function
// library. It is reachable — a person can put one on `FPATH` — and it is the
// only thing that makes a stock `add-zsh-hook` load today, since this
// installation ships no functions of its own yet. It is not the default,
// because it cannot be found portably (the two builds above) and because
// which library a shell reads is a decision about what this shell *is*.
// A name that cannot be found still refuses by name, which says more than a
// silent empty does.

// functionSearchDirs are the directories an installation of this shell keeps
// function definition files in, most preferred first.
//
// `site-functions` before `functions` is zsh's own order and the useful one:
// what a package installed for this shell wins over what the shell shipped,
// so a fix does not need the shipped file replaced.
func functionSearchDirs(prefix string) []string {
	if prefix == "" {
		return nil
	}
	share := filepath.Join(prefix, "share", "sh")
	return []string{
		filepath.Join(share, "site-functions"),
		filepath.Join(share, "functions"),
	}
}

// installPrefix is the root this shell was installed under, worked out from
// where the running binary is.
//
// `make install` puts every binary in `$(PREFIX)/libexec/sh` and the Homebrew
// formula puts them in the keg's `libexec` — one directory named `sh` inside
// one named `libexec` — so that shape is recognized and the two directories
// above it come off. Anything else falls back to the parent of the directory
// the binary sits in, which is what an uninstalled build is: for
// `<checkout>/build/sh-zsh` the installation *is* the checkout, and the
// directories it names are simply not there yet.
//
// Symlinks are resolved first, because a binary reached through a link on
// PATH has to find the tree it was installed into rather than the one the
// link sits in.
//
// A string parameter rather than a read of the process, so the derivation can
// be exercised over the layouts it has to recognize without a binary in each
// of them. The one read is in seedFunctionSearch below.
func installPrefix(exe string) string {
	if exe == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	if filepath.Base(dir) == "sh" && filepath.Base(filepath.Dir(dir)) == "libexec" {
		return filepath.Dir(filepath.Dir(dir))
	}
	return filepath.Dir(dir)
}

// seedFunctionSearch gives the function search path its default, for a
// dialect that has one and an environment that said nothing about it.
//
// Called after the dialect's own Register, because the parameter is *tied* —
// `FPATH` and `fpath` are one value in two shapes — and the tie is one of the
// things Register installs. Setting the scalar before it exists would fill a
// name nothing reads.
//
// The whole of the front end's part in this: which parameter is the dialect's
// answer, and what goes in it is a fact about this process that no library may
// reach for. The same split `$PATH` has.
func (sh Shell) seedFunctionSearch(r *interp.Runner) {
	name := sh.Semantics.FunctionSearchVariable
	if name == "" {
		return
	}
	if environmentNames(r.Env, name) {
		// Present in the environment wins, whatever it holds — measured, an
		// empty `FPATH` suppresses the default as completely as a full one
		// replaces it.
		return
	}
	exe, err := os.Executable()
	if err != nil {
		// Nothing to derive from. The default is the empty one this shell had
		// before there was a default at all, which still refuses by name.
		return
	}
	dirs := functionSearchDirs(installPrefix(exe))
	if len(dirs) == 0 {
		return
	}
	r.SetVar(name, strings.Join(dirs, ":"))
}

// environmentNames reports whether an environment list mentions a name at all,
// which is a different question from whether the name has a value.
func environmentNames(env []string, name string) bool {
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && k == name {
			return true
		}
	}
	return false
}
