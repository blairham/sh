// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"strings"
	"testing"
)

// A `return` raised by glob qualifier code inside a command of assignments
// alone leaves the function at the list's own status — 0, or the last
// substitution's — and not the operand's; and a file whose code does not run
// because an earlier file's code asked to return is kept only where the
// status in force before the code began was 0 (#6752). Measured 2026-10-27
// on zsh 5.9.2 under -fc in the qualifier fixture (`[ab]` matches `a` and
// `b`); every `out` and `errs` is that shell's.
func TestAReturnFromQualifierCodeInABareAssignmentLeavesTheListsStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, out, errs string }{
		// The second assignment's substitution never runs, and the list is 0.
		{`f() { x=( [ab](e:"return 3":) ) y=$(print side >&2); print inf; }; f; print after $? $#x`, "after 0 1\n", ""},
		// It ran first: the list's status is still 0, since the return
		// came after it.
		{`f() { y=$(print side >&2) x=( [ab](e:"return 3":) ); print inf; }; f; print after $? $#x`, "after 0 1\n", "side\n"},
		// The last substitution's status is the list's, and the file after
		// the return is dropped because 7 stood when the code began.
		{`f() { y=$(exit 7) x=( [ab](e:"return 3":N) ); print inf; }; f; print "after $? $#x"`, "after 7 0\n", ""},
		{`f() { x=( [ab](e:"return 3":N) ) y=$(exit 7); print inf; }; f; print "after $? $#x"`, "after 0 1\n", ""},
		// No substitution ran, so 0 — and no file is kept under the 5.
		{`f() { (exit 5); x=( [ab](e:"return 3":N) ); print inf; }; f; print "after $? $#x"`, "after 0 0\n", ""},
		// A substitution that left 0 in front of the code puts the later
		// file back.
		{`f() { (exit 5); y=$(true) x=( [ab](e:"return 3":N) ); print inf; }; f; print "after $? $#x"`, "after 0 1\n", ""},
		// What the code ran before its return does not decide it.
		{`f() { x=( [ab](e:"false; return 3":) ) y=1; print inf; }; f; print after $? $#x`, "after 0 1\n", ""},
		// `return 0` keeps its own file and the 5 drops the next.
		{`f() { (exit 5); x=( [ab](e:"return 0":) ); print inf; }; f; print after $? $#x`, "after 0 1\n", ""},
		// The same rule in a builtin's words. The file kept is the one the
		// directory lists second, since the code ran on the first: see
		// laterOf.
		{`f() { (exit 5); print X [ab](e:"return 3":N); print inf; }; f; print "after $?"`, "X\nafter 0\n", ""},
		{`f() { print X [ab](e:"return 3":N); print inf; }; f; print "after $?"`, "X {later}\nafter 0\n", ""},
	} {
		dir := globCodeFixture(t)
		want := strings.ReplaceAll(tc.out, "{later}", laterOf(t, dir, "a", "b"))
		out, _, errs := runZshSplit(t, dir, tc.src)
		if out != want || errs != tc.errs {
			t.Errorf("%s\n out %q errs %q\nwant %q errs %q", tc.src, out, errs, want, tc.errs)
		}
	}
}

// With nothing kept, the pattern misses and the function's refusal ends the
// shell.
func TestAReturnFromQualifierCodeUnderAFailingStatusKeepsNoFile(t *testing.T) {
	t.Parallel()
	src := `f() { (exit 5); x=( [ab](e:"return 3":) ); print inf; }; f; print after $?`
	out, st, errs := runZshSplit(t, globCodeFixture(t), src)
	if out != "" || st != 1 || errs != "f: no matches found: [ab](e:return 3:)\n" {
		t.Errorf("out %q st %d errs %q", out, st, errs)
	}
}

// An assignment's trace line is written as far as it has got ahead of a trace
// line a nested command writes in this shell — code in a glob qualifier — and
// the rest of the line follows once the value is known. A command run in a
// child writes the line so far ahead of its own and leaves it pending, as a
// substitution does. Once part of a prefix's line has gone out, the trace
// prompt is written again before the command's words whatever the command
// is (#6752). Measured 2026-10-27 on zsh 5.9.2 under -fc in the qualifier
// fixture, both streams on one pipe.
func TestATraceLineIsWrittenAheadOfQualifierCodesOwnTrace(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a bare assignment",
			`set -x; x=( [ab](e:"print -u2 code":) )`,
			"+zsh:1> x=+zsh:1> print -u2 code\ncode\n+zsh:1> print -u2 code\ncode\n( a b ) \n",
		},
		{
			"a prefix to a builtin",
			`set -x; x=( [ab](e:"print -u2 code":) ) print in`,
			"+zsh:1> x=+zsh:1> print -u2 code\ncode\n+zsh:1> print -u2 code\ncode\n( a b ) +zsh:1> print in\nin\n",
		},
		{
			"the issue's row: a return in the code",
			`set -x; f() { x=( [ab](e:"return 3":) ) print in; }; f`,
			"+zsh:1> f\n+f:0> x=+f:0> return 3\n( {later} ) +f:0> print in\nin\n",
		},
		{
			"a prefix to a program",
			`set -x; x=( [ab](e:"print -u2 code":) ) /usr/bin/true`,
			"+zsh:1> x=+zsh:1> print -u2 code\ncode\n+zsh:1> print -u2 code\ncode\n( a b ) +zsh:1> /usr/bin/true\n",
		},
		{
			"a plain entry ahead of it",
			`set -x; y=1 x=( [ab](e:"print -u2 code":) ) print in`,
			"+zsh:1> y=1 x=+zsh:1> print -u2 code\ncode\n+zsh:1> print -u2 code\ncode\n( a b ) +zsh:1> print in\nin\n",
		},
		{
			"a plain entry behind it",
			`set -x; x=( [ab](e:"print -u2 code":) ) y=1 print in`,
			"+zsh:1> x=+zsh:1> print -u2 code\ncode\n+zsh:1> print -u2 code\ncode\n( a b ) y=1 +zsh:1> print in\nin\n",
		},
		{
			"two literals with code",
			`set -x; x=( [ab](e:"print -u2 X":) ) y=( [ab](e:"print -u2 Y":) ) print in`,
			"+zsh:1> x=+zsh:1> print -u2 X\nX\n+zsh:1> print -u2 X\nX\n( a b ) y=+zsh:1> print -u2 Y\nY\n+zsh:1> print -u2 Y\nY\n( a b ) +zsh:1> print in\nin\n",
		},
		{
			"a substitution ahead of it runs once",
			`set -x; y=$(print -u2 sub) x=( [ab](e:"print -u2 code":) ) print in`,
			"+zsh:1> y=+zsh:1> print -u2 sub\nsub\n+zsh:1> y='' x=+zsh:1> print -u2 code\ncode\n+zsh:1> print -u2 code\ncode\n( a b ) +zsh:1> print in\nin\n",
		},
		{
			"a substitution behind it inherits the rest",
			`set -x; x=( [ab](e:"true":) ) y=$(print -u2 s1) print in`,
			"+zsh:1> x=+zsh:1> true\n+zsh:1> true\n( a b ) y=+zsh:1> print -u2 s1\ns1\n( a b ) y='' +zsh:1> print in\nin\n",
		},
		{
			"a substitution inside the literal leaves the line pending",
			`set -x; x=( $(print -u2 sub) [ab](e:"print -u2 code":) ) print in`,
			"+zsh:1> x=+zsh:1> print -u2 sub\nsub\n+zsh:1> x=+zsh:1> print -u2 code\ncode\n+zsh:1> print -u2 code\ncode\n( a b ) +zsh:1> print in\nin\n",
		},
		{
			"a bare literal with a substitution after the code",
			`set -x; x=( [ab](e:"true":) $(print -u2 s1) )`,
			"+zsh:1> x=+zsh:1> print -u2 s1\ns1\n+zsh:1> x=+zsh:1> true\n+zsh:1> true\n( a b ) \n",
		},
		{
			"a function's lines inside the code",
			`g() { print -u2 g; }; set -x; x=( [ab](e:"g":) )`,
			"+zsh:1> x=+zsh:1> g\n+g:0> print -u2 g\ng\n+zsh:1> g\n+g:0> print -u2 g\ng\n( a b ) \n",
		},
		{
			"a frozen name after it",
			`readonly r; set -x; x=( [ab](e:"true":) ) r=2 print in; print st=$?`,
			"+zsh:1> x=+zsh:1> true\n+zsh:1> true\nzsh:1: read-only variable: r\n( a b ) r=2 ",
		},
		{
			"a value that fails after it",
			`set -x; x=( [ab](e:"true":) ) y=$((1/0)) print in`,
			"+zsh:1> x=+zsh:1> true\n+zsh:1> true\nzsh:1: division by zero\n( a b ) y=",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := globCodeFixture(t)
			want := strings.ReplaceAll(tc.want, "{later}", laterOf(t, dir, "a", "b"))
			out, _ := runZsh(t, dir, tc.src)
			if out != want {
				t.Errorf("out %q\nwant %q", out, want)
			}
		})
	}
}

// laterOf names whichever of x and y the directory lists second, in its own
// order rather than by name, which is the order qualifier code runs in: a
// `return` from the code on the first leaves the second to be kept. That
// order is the filesystem's, so a row naming the kept file names it through
// this. Measured 2026-10-28 on zsh 5.9.2 in the suite's pinned image, on a
// tmpfs that lists `b` ahead of `a`: `print X [ab](e:"return 3":N)` writes
// `X a`, where macOS lists `a` first and the same line writes `X b`.
func laterOf(t *testing.T, dir, x, y string) string {
	t.Helper()
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		switch name {
		case x:
			return y
		case y:
			return x
		}
	}
	t.Fatalf("%s holds neither %s nor %s", dir, x, y)
	return ""
}
