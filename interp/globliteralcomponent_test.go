// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A component of a pathname expansion that spells a name out is resolved by
// asking whether the path is there, and never by listing the directory above
// it. The two are different questions: a listing needs read permission on
// that directory, resolving needs only the right to pass through it.
//
// Measured 2026-09-16 in bash 5.3.20, zsh 5.9.2, ksh93u+ and dash 0.5.12,
// which answer these rows identically — so it is the core's answer and no
// dialect's, and the tests below run under the permissive base rather than
// under a preset.

// modesDir builds the fixture both directions of #3387 need.
//
//	search/   --x   may be entered, may not be listed; holds f and deep/g
//	none/     ---   neither
//	open/     rwx   holds f and a dangling link
func modesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"search", "search/deep", "none", "open"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"search/f", "search/deep/g", "open/f"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("nowhere", filepath.Join(dir, "open", "gone")); err != nil {
		t.Fatal(err)
	}
	// Restored before the temp directory is removed, or the cleanup cannot
	// read what it is deleting.
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Join(dir, "search"), 0o755)
		_ = os.Chmod(filepath.Join(dir, "none"), 0o755)
	})
	if err := os.Chmod(filepath.Join(dir, "search"), 0o100); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "none"), 0o000); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The root of a test process can read and enter anything, so the permission
// rows measure nothing there and would pass whatever the walk did.
func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: a mode that denies nothing cannot separate a listing from a resolution")
	}
}

// The name that is missed: a listing of a `--x` directory is refused, and the
// name inside it is still reachable by being named.
func TestASpelledComponentIsFoundUnderADirectoryThatCannotBeListed(t *testing.T) {
	skipAsRoot(t)
	dir := modesDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"one level", `printf "[%s]" */f`, `[open/f][search/f]`},
		{"two spelled components", `printf "[%s]" */deep/g`, `[search/deep/g]`},
		{"a spelled directory", `printf "[%s]" */deep`, `[search/deep]`},
		// The control, and it is the half that must not move: a component
		// that *describes* a name genuinely needs the listing, and every
		// column gives it up when the listing is refused.
		{"a pattern component still needs the listing", `printf "[%s]" search/*`, `[search/*]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir, tc.src); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// The name that is invented: no listing anywhere reports `.` or `..`, so a
// walk that joins them without asking offers one under a directory it cannot
// enter.
func TestADotComponentIsNotOfferedUnderADirectoryThatCannotBeEntered(t *testing.T) {
	skipAsRoot(t)
	dir := modesDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"dot", `printf "[%s]" */.`, `[open/.][search/.]`},
		{"dot dot", `printf "[%s]" */..`, `[open/..][search/..]`},
		// The trailing separator is a question about the directory the walk
		// already matched rather than a component of its own, so it lists
		// the unenterable one — in every column, and here.
		{"the empty component a trailing slash leaves", `printf "[%s]" */`, `[none/][open/][search/]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir, tc.src); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// Which question is asked, rather than whether one is: a dangling link has no
// target to stat and is still a name. A resolution that followed the link
// would drop it, and no column does.
func TestASpelledComponentNamingADanglingLinkStillMatches(t *testing.T) {
	dir := modesDir(t)
	if got, want := runIn(t, dir, `printf "[%s]" */gone`), `[open/gone]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	// And the trailing separator does ask about the target, so the same name
	// with one behind it matches nothing.
	if got, want := runIn(t, dir, `printf "[%s]" */gone/`), `[*/gone/]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
