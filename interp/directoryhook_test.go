// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Semantics.DirectoryChangeHook: the function `cd` calls once it has moved.
//
// The axis is named here and no shell is. What a shell answers, and the
// measurements behind the answer, live in the dialect — see dialect/zsh's
// chpwd_test.go, which is the one column of the panel that has one.
//
// Every hook here prints a marker nothing else in the snippet can print. That
// is the point of the marker rather than an ornament: a hook that printed
// `$PWD` after a `cd` into a directory whose name the test also prints could
// not tell "the hook ran" from "something else said where we are", and the
// cases below that assert the hook did *not* run would pass with the hook
// wired to fire always.

// hookRunner is a runner whose dialect has a directory-change hook and a hook
// list, in a directory tree with somewhere to go.
//
// The suffix is `_list` and the hook is `moved`, which are nobody's spelling.
// A test in this package names the axis; the spelling is the dialect's.
func hookRunner(t *testing.T, out *strings.Builder) (*Runner, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.DirectoryChangeHook = "moved"
	sem.HookListSuffix = "_list"
	return newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
		Stdout: out, Stderr: out,
	}), dir
}

// parseCore reads a snippet with the core grammar, for the one case that needs
// the *status* of the run — runCd runs and discards it, which is right for
// every case where something after the `cd` can echo `$?` and no use at all
// where the point is that nothing after it runs.
func parseCore(t *testing.T, src string) *syntax.File {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// The named function, then the list, in the order the list holds it.
func TestTheDirectoryChangeHookFiresOnACdThatMoved(t *testing.T) {
	var out strings.Builder
	r, _ := hookRunner(t, &out)
	runCd(t, r, `moved() { echo HOOKMARK-NAMED; }
a() { echo HOOKMARK-A; }
b() { echo HOOKMARK-B; }
moved_list=(a b)
cd sub
echo "st=$?"
`)
	const want = "HOOKMARK-NAMED\nHOOKMARK-A\nHOOKMARK-B\nst=0\n"
	if got := out.String(); got != want {
		t.Errorf("the chain printed %q, want %q", got, want)
	}
}

// A shell that names no hook runs none, which is three of the four and is
// also every Semantics nobody has filled in.
//
// The mutant this excludes is a `cd` that calls a function named by something
// other than the axis — the empty name has to mean "no hook" and not "a hook
// called nothing".
func TestACdFiresNothingWhereNoHookIsNamed(t *testing.T) {
	var out strings.Builder
	r, _ := hookRunner(t, &out)
	r.Semantics.DirectoryChangeHook = ""
	runCd(t, r, `moved() { echo HOOKMARK-NAMED; }
a() { echo HOOKMARK-A; }
moved_list=(a)
cd sub
echo "st=$?"
`)
	if got := out.String(); got != "st=0\n" {
		t.Errorf("a shell with no such hook printed %q, want st=0 alone", got)
	}
}

// A `cd` that did not move fires nothing.
//
// The load-bearing half is the marker's *absence*: this is the case a hook
// fired unconditionally would still pass if the assertion were only about
// where the shell ended up.
func TestACdThatFailedFiresNothing(t *testing.T) {
	var out strings.Builder
	r, _ := hookRunner(t, &out)
	runCd(t, r, `moved() { echo HOOKMARK-NAMED; }
cd nosuchdir_zz
echo "st=$?"
`)
	if got := out.String(); strings.Contains(got, "HOOKMARK") {
		t.Errorf("a failed cd printed %q, want no hook in it", got)
	}
	if got := out.String(); !strings.Contains(got, "st=1") {
		t.Errorf("a failed cd printed %q, want st=1", got)
	}
}

// The quiet letter suppresses it, which is the whole of what that letter
// means. See Semantics.CdHasQuietOption.
//
// Both halves in one case, because a `-q` that suppressed by *not moving*
// would pass the silence check on its own — which is the shape #1558 was.
func TestTheQuietLetterSuppressesTheDirectoryChangeHook(t *testing.T) {
	var out strings.Builder
	r, _ := hookRunner(t, &out)
	r.Semantics.CdHasQuietOption = Yes
	r.Semantics.CdRefusesUnknownOption = No
	runCd(t, r, `moved() { echo HOOKMARK-NAMED; }
a() { echo HOOKMARK-A; }
moved_list=(a)
cd -q sub
echo "st=$?"
pwd
`)
	got := out.String()
	if strings.Contains(got, "HOOKMARK") {
		t.Errorf("a quiet cd printed %q, want no hook in it", got)
	}
	if !strings.Contains(got, "st=0") || !strings.HasSuffix(strings.TrimSpace(got), "sub") {
		t.Errorf("a quiet cd printed %q, want it to have moved into sub", got)
	}
}

// Without the letter, the same line fires it — the pairing that keeps the
// case above from passing because the hook never fires at all.
func TestTheSameCdWithoutTheQuietLetterFiresIt(t *testing.T) {
	var out strings.Builder
	r, _ := hookRunner(t, &out)
	r.Semantics.CdHasQuietOption = Yes
	r.Semantics.CdRefusesUnknownOption = No
	runCd(t, r, `moved() { echo HOOKMARK-NAMED; }
cd sub
`)
	if got := out.String(); !strings.Contains(got, "HOOKMARK-NAMED") {
		t.Errorf("a plain cd printed %q, want the hook to have run", got)
	}
}

// The hook is told where the shell now is and where it was, and is told no
// arguments at all.
func TestTheDirectoryChangeHookIsToldWhereAndNothingElse(t *testing.T) {
	var out strings.Builder
	r, dir := hookRunner(t, &out)
	runCd(t, r, `moved() { echo "HOOKMARK n=$# pwd=$PWD old=$OLDPWD"; }
cd sub
`)
	want := "HOOKMARK n=0 pwd=" + filepath.Join(dir, "sub") + " old=" + dir + "\n"
	if got := out.String(); got != want {
		t.Errorf("the hook was told %q, want %q", got, want)
	}
}

// A scalar by the list's name is not a list.
//
// The reading a hook list wants is not the one `${x[0]}` wants, and the two
// are one call apart — so the case is worth holding rather than inferring.
func TestAScalarByTheListsNameIsNotAHookList(t *testing.T) {
	var out strings.Builder
	r, _ := hookRunner(t, &out)
	runCd(t, r, `a() { echo HOOKMARK-A; }
moved_list=a
cd sub
echo "st=$?"
`)
	if got := out.String(); got != "st=0\n" {
		t.Errorf("a scalar list printed %q, want st=0 alone", got)
	}
}

// A shell with no list suffix is the named function and nothing else.
func TestWithoutAListSuffixTheHookIsTheNamedFunctionAlone(t *testing.T) {
	var out strings.Builder
	r, _ := hookRunner(t, &out)
	r.Semantics.HookListSuffix = ""
	runCd(t, r, `moved() { echo HOOKMARK-NAMED; }
a() { echo HOOKMARK-A; }
moved_list=(a)
cd sub
`)
	if got := out.String(); got != "HOOKMARK-NAMED\n" {
		t.Errorf("the chain printed %q, want the named function alone", got)
	}
}

// The chain is told the status of the command before the `cd`, cannot change
// what the next command reads, and a member that fails stops nothing.
func TestTheDirectoryChangeChainIsToldTheStatusAndCannotChangeIt(t *testing.T) {
	var out strings.Builder
	r, _ := hookRunner(t, &out)
	runCd(t, r, `moved() { echo "HOOKMARK-NAMED=$?"; return 3; }
a() { echo "HOOKMARK-A=$?"; return 9; }
b() { echo "HOOKMARK-B=$?"; }
moved_list=(a b)
(exit 5)
cd sub
echo "after=$?"
`)
	const want = "HOOKMARK-NAMED=5\nHOOKMARK-A=5\nHOOKMARK-B=5\nafter=0\n"
	if got := out.String(); got != want {
		t.Errorf("the chain printed %q, want %q", got, want)
	}
}

// A member that calls `exit` ends the chain and the script with it.
func TestAMemberThatExitsEndsTheDirectoryChangeChain(t *testing.T) {
	var out strings.Builder
	r, _ := hookRunner(t, &out)
	f := parseCore(t, `moved() { echo HOOKMARK-NAMED; }
a() { echo HOOKMARK-A; exit 4; }
b() { echo HOOKMARK-B; }
moved_list=(a b)
cd sub
echo NOTREACHED
`)
	st, err := r.Run(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "HOOKMARK-NAMED\nHOOKMARK-A\n" {
		t.Errorf("the chain printed %q, want it to stop at the exit", got)
	}
	if st != 4 {
		t.Errorf("the script ended with %d, want 4", st)
	}
}

// A hook that itself moves fires the hook again, and nothing here guards
// against that beyond the limit an ordinary recursive call meets.
func TestAHookThatMovesFiresItAgain(t *testing.T) {
	var out strings.Builder
	r, _ := hookRunner(t, &out)
	if err := os.MkdirAll(filepath.Join(r.Dir, "sub", "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	runCd(t, r, `moved() { echo "HOOKMARK ${PWD##*/}"; if [ "${PWD##*/}" = sub ]; then cd deeper; fi; }
cd sub
echo "end=${PWD##*/}"
`)
	const want = "HOOKMARK sub\nHOOKMARK deeper\nend=deeper\n"
	if got := out.String(); got != want {
		t.Errorf("the chain printed %q, want %q", got, want)
	}
}
