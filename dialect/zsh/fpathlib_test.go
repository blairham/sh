// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// Each layout is a scratch tree this test builds rather than the machine's
// own, which is the point of the roots being the caller's: the answer is
// about directories, so a test hands it some and never reads the runner's.
//
// The trees are the shapes real zsh's own default `$fpath` was measured to
// have — see InstalledFunctionLibrary — with nothing in them: the discovery
// reads directory names and never a file.

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func roots(managers []string, system, local string) zsh.LibraryRoots {
	return zsh.LibraryRoots{Managers: managers, System: system, LocalSite: local}
}

func TestAnInstalledLibraryIsFoundUnderAPackageManagersRoot(t *testing.T) {
	t.Parallel()
	brew := t.TempDir()
	mkdirs(t, brew, "share/zsh/functions", "share/zsh/site-functions")
	got := zsh.InstalledFunctionLibrary("", roots([]string{brew}, "", ""))
	want := []string{
		filepath.Join(brew, "share/zsh/site-functions"),
		filepath.Join(brew, "share/zsh/functions"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("library = %q, want %q", got, want)
	}
}

// The site directories go in front of the library, the conventional local one
// first, because that is the order the installation's own `$fpath` has them in
// and it is what lets git's `_git` win over the library's.
func TestTheSiteDirectoriesComeBeforeTheLibraryAndOnlyWhereTheyExist(t *testing.T) {
	t.Parallel()
	prefix, local := t.TempDir(), t.TempDir()
	mkdirs(t, prefix, "share/zsh/functions", "share/zsh/vendor-completions")
	got := zsh.InstalledFunctionLibrary("", roots([]string{prefix}, "", local))
	want := []string{
		local,
		filepath.Join(prefix, "share/zsh/vendor-completions"),
		filepath.Join(prefix, "share/zsh/functions"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("library = %q, want %q", got, want)
	}
}

// Apple's build keeps its library under a version and a machine can hold two,
// so the newest is taken — numerically, or 5.9 would beat 5.10.
func TestAVersionedLibraryIsTheNewest(t *testing.T) {
	t.Parallel()
	usr := t.TempDir()
	mkdirs(t, usr, "share/zsh/5.9/functions", "share/zsh/5.10/functions", "share/zsh/notaversion/functions", "share/zsh/5.11")
	got := zsh.InstalledFunctionLibrary("", roots(nil, usr, ""))
	want := []string{filepath.Join(usr, "share/zsh/5.10/functions")}
	if !slices.Equal(got, want) {
		t.Errorf("library = %q, want %q", got, want)
	}
}

// Debian installs the unflattened tree, and its zsh lists every directory
// under the top one — parents before children, siblings by name — and not the
// top one itself.
func TestAnUnflattenedLibraryIsEveryDirectoryUnderIt(t *testing.T) {
	t.Parallel()
	usr := t.TempDir()
	mkdirs(t, usr, "share/zsh/functions/Completion/Base", "share/zsh/functions/Completion/Unix",
		"share/zsh/functions/Calendar", "share/zsh/functions/Zle")
	lib := filepath.Join(usr, "share/zsh/functions")
	got := zsh.InstalledFunctionLibrary("", roots(nil, usr, ""))
	want := []string{
		filepath.Join(lib, "Calendar"),
		filepath.Join(lib, "Completion"),
		filepath.Join(lib, "Completion/Base"),
		filepath.Join(lib, "Completion/Unix"),
		filepath.Join(lib, "Zle"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("library = %q, want %q", got, want)
	}
}

// A zsh no root covers is found through the search path, by where its binary
// is installed — read through the link a package manager puts on PATH.
func TestTheZshOnThePathNamesItsInstallation(t *testing.T) {
	t.Parallel()
	tree := t.TempDir()
	prefix := filepath.Join(tree, "store", "zsh-5.9")
	mkdirs(t, prefix, "bin", "share/zsh/5.9/functions")
	if err := os.WriteFile(filepath.Join(prefix, "bin", "zsh"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(tree, "profile", "bin")
	mkdirs(t, tree, "profile/bin")
	if err := os.Symlink(filepath.Join(prefix, "bin", "zsh"), filepath.Join(profile, "zsh")); err != nil {
		t.Fatal(err)
	}
	// A relative entry and one without a zsh come first and are passed over.
	path := "relative/bin" + string(filepath.ListSeparator) + filepath.Join(tree, "empty") +
		string(filepath.ListSeparator) + profile
	got := zsh.InstalledFunctionLibrary(path, roots([]string{filepath.Join(tree, "nobrew")}, "", ""))
	resolved, err := filepath.EvalSymlinks(prefix)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(resolved, "share/zsh/5.9/functions")}
	if !slices.Equal(got, want) {
		t.Errorf("library = %q, want %q", got, want)
	}
}

// A package manager's zsh is preferred over the one the search path names,
// because the search path a login shell starts with is the system's.
func TestAPackageManagersLibraryWinsOverThePath(t *testing.T) {
	t.Parallel()
	tree := t.TempDir()
	brew := filepath.Join(tree, "brew")
	mkdirs(t, brew, "share/zsh/functions")
	system := filepath.Join(tree, "system")
	mkdirs(t, system, "bin", "share/zsh/5.9/functions")
	if err := os.WriteFile(filepath.Join(system, "bin", "zsh"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	got := zsh.InstalledFunctionLibrary(filepath.Join(system, "bin"), roots([]string{brew}, "", ""))
	want := []string{filepath.Join(brew, "share/zsh/functions")}
	if !slices.Equal(got, want) {
		t.Errorf("library = %q, want %q", got, want)
	}
}

// Nothing installed is nothing appended, and a prefix with only site
// directories is not an installation of zsh — it is where some other program
// put a completion.
func TestNoLibraryIsNothing(t *testing.T) {
	t.Parallel()
	tree := t.TempDir()
	mkdirs(t, tree, "share/zsh/site-functions")
	if got := zsh.InstalledFunctionLibrary("", roots([]string{tree}, tree, "")); got != nil {
		t.Errorf("library = %q, want none", got)
	}
	if got := zsh.InstalledFunctionLibrary("", roots(nil, "", "")); got != nil {
		t.Errorf("library with no roots = %q, want none", got)
	}
}
