// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A call's assignment prefix standing over a **name reference** writes the
// reference's *target*, and everything else the prefix does goes to the target
// with it: the export attribute, the entry a child is handed, and the
// take-back. The reference itself is left exactly as it was.
//
// Measured 2026-09-21 on bash 5.3.20 from a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME, `target=T; declare -n
// foo=target` in front of every row (#4110).
//
// bash 3.2.57 cannot be asked at all: `declare -n` is `invalid option` there,
// so the outer declaration never happens and the row measures nothing. This is
// a 5.3-only question and not a split between the two builds.

// The listing half, which is the first of the issue's two defects: the `x` the
// prefix applies belongs to the **target** and this shell was putting it on
// the reference — `declare -nx foo="target"` where bash writes `declare -n
// foo="target"`.
func TestAPrefixOverANameReferenceLeavesTheReferenceAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const decl = `target=T; declare -n foo=target; `
	for _, row := range []struct{ name, src, want string }{
		{
			"the reference keeps its letter and takes no other",
			decl + `ff() { declare -p foo; }; foo=bar ff`,
			`declare -n foo="target"`,
		},
		{
			"and the target is the one carrying the prefix",
			decl + `ff() { declare -p target; }; foo=bar ff`,
			`declare -x target="bar"`,
		},
		// Through the builtin route, which is a different piece of code
		// answering the same question.
		{
			"at a builtin, the reference is untouched",
			decl + `foo=bar declare -p foo`,
			`declare -n foo="target"`,
		},
		{
			"at a builtin, the target carries it",
			decl + `foo=bar declare -p target`,
			`declare -x target="bar"`,
		},
	} {
		out, st := runBash(t, dir, row.src)
		if st != 0 {
			t.Errorf("%s: %s answered %d: %q", row.name, row.src, st, out)
			continue
		}
		if got := strings.TrimSpace(out); got != row.want {
			t.Errorf("%s: %s = %q, want %q", row.name, row.src, got, row.want)
		}
	}
}

// The reading half, with no `declare -p` in it at all — so this is what the
// body is *handed* rather than how a listing spells it.
//
// Both shells already agreed on this row before #4110, which is exactly why it
// is here: it is the control that says a fix for the take-back below did not
// buy itself by stopping the prefix reaching the body at all.
func TestAPrefixOverANameReferenceIsReadThroughIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ name, src, want string }{
		{
			"the body reads the prefix through the reference",
			`target=T; declare -n foo=target; ff() { echo "[$foo]"; }; foo=bar ff`,
			"[bar]\n",
		},
		{
			"and reads it under the target's own name",
			`target=T; declare -n foo=target; ff() { echo "[$target]"; }; foo=bar ff`,
			"[bar]\n",
		},
		// An append joins what the reference was showing, so the old value is
		// reached through the reference and the join lands on the target.
		{
			"an appended prefix joins the target's value",
			`target=T; declare -n foo=target; ff() { echo "[$foo]"; }; foo+=bar ff`,
			"[Tbar]\n",
		},
	} {
		out, st := runBash(t, dir, row.src)
		if out != row.want || st != 0 {
			t.Errorf("%s: %s = %q status %d, want %q", row.name, row.src, out, st, row.want)
		}
	}
}

// **The one that matters**, and asserted on its own rather than as the tail of
// a row above: the write does not outlive the call.
//
// Every other prefix row is confined to the displaced name and given back on
// return. This one landed on a *different variable* — one the call never named
// — and there was nothing to restore it from, so `foo=bar ff` silently and
// permanently rewrote whatever `foo` happened to point at (#4110).
func TestAPrefixOverANameReferenceDoesNotOutliveTheCall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ name, src, want string }{
		{
			"the target holds what it held",
			`target=T; declare -n foo=target; ff() { :; }; foo=bar ff; declare -p target`,
			`declare -- target="T"`,
		},
		{
			"read rather than listed",
			`target=T; declare -n foo=target; ff() { :; }; foo=bar ff; echo "[$target]"`,
			`[T]`,
		},
		{
			"an append does not outlive it either",
			`target=T; declare -n foo=target; ff() { :; }; foo+=bar ff; echo "[$target]"`,
			`[T]`,
		},
		// The kind and the letters come back with the value, which is the
		// take-back's own claim reached through a reference. The prefix makes
		// the target a fresh exported scalar for the length of the call — see
		// Semantics.AssignmentPrefixMakesAFreshCell — so what is given back
		// here is an array the body never saw.
		{
			"an array target comes back whole",
			`declare -a target=(a b); declare -n foo=target; ff() { :; }; ` +
				`foo=bar ff; declare -p target`,
			`declare -a target=([0]="a" [1]="b")`,
		},
		{
			"and a letter the target carried comes back on it",
			`declare -i target=7; declare -n foo=target; ff() { :; }; ` +
				`foo=bar ff; declare -p target`,
			`declare -i target="7"`,
		},
		{
			"an export the target did not have is not left on it",
			`target=T; declare -n foo=target; ff() { :; }; foo=bar ff; declare -p target`,
			`declare -- target="T"`,
		},
		// A target the shell had never heard of is *removed* again rather
		// than left holding the prefix's value, which is the same rule where
		// there was no binding to put back.
		{
			"a target that did not exist does not start existing",
			`declare -n foo=target; ff() { :; }; foo=bar ff; ` +
				`declare -p target 2>/dev/null || echo "(not there)"`,
			`(not there)`,
		},
		// Through the builtin route, and through a chain of two references.
		{
			"after a builtin",
			`target=T; declare -n foo=target; foo=bar declare -p foo >/dev/null; declare -p target`,
			`declare -- target="T"`,
		},
		{
			"through a chain of two references",
			`t2=T; declare -n mid=t2; declare -n foo=mid; ff() { :; }; ` +
				`foo=bar ff; declare -p t2`,
			`declare -- t2="T"`,
		},
	} {
		out, st := runBash(t, dir, row.src)
		if st != 0 {
			t.Errorf("%s: %s answered %d: %q", row.name, row.src, st, out)
			continue
		}
		if got := strings.TrimSpace(out); got != row.want {
			t.Errorf("%s: %s = %q, want %q", row.name, row.src, got, row.want)
		}
	}
}

// A reference that points at no name a write can land on is **its own**
// target, and so reaches the fresh cell carrying the `n` letter — which the
// fresh cell takes off, making the store that follows an ordinary store to the
// reference rather than the assignment that would otherwise have *aimed* it.
//
// Measured 2026-09-21 on bash 5.3.20. These two rows are what say the
// resolution is the assignment's rule and not "follow the reference always":
// following it would have aimed the unaimed one at `bar` and written through
// the element in the second.
func TestAPrefixOverAReferenceWithNoTargetIsItsOwnCell(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ name, src, want string }{
		{
			"a reference aimed at nothing is a plain scalar for the call",
			`declare -n foo; ff() { declare -p foo; }; foo=bar ff`,
			`declare -x foo="bar"`,
		},
		{
			"and it is the reference again afterwards",
			`declare -n foo; ff() { :; }; foo=bar ff; declare -p foo`,
			`declare -n foo`,
		},
		{
			"a reference aimed at an element does not write the element",
			`a=(x y z); declare -n foo="a[1]"; ff() { echo "[${a[*]}]"; }; foo=bar ff`,
			`[x y z]`,
		},
		{
			"the element's prefix is a plain scalar on the reference",
			`a=(x y z); declare -n foo="a[1]"; ff() { declare -p foo; }; foo=bar ff`,
			`declare -x foo="bar"`,
		},
		{
			"and the array is untouched afterwards",
			`a=(x y z); declare -n foo="a[1]"; ff() { :; }; foo=bar ff; echo "[${a[*]}]"`,
			`[x y z]`,
		},
	} {
		out, st := runBash(t, dir, row.src)
		if st != 0 {
			t.Errorf("%s: %s answered %d: %q", row.name, row.src, st, out)
			continue
		}
		if got := strings.TrimSpace(out); got != row.want {
			t.Errorf("%s: %s = %q, want %q", row.name, row.src, got, row.want)
		}
	}
}

// And the entry a **child** is handed is named for the target too: `foo=bar
// env` shows a child `target=bar`, where this shell showed it `foo=bar` — a
// name holding the target's *name* everywhere else in the script.
//
// Measured 2026-09-21 on bash 5.3.20, and ksh93u+ 2012 answers it identically
// — see dialect/ksh's row, which is the same assertion in the column whose
// prefix persists. The filtering is done in the shell rather than with `grep`,
// because the run's PATH is the scratch directory alone.
func TestAPrefixOverANameReferenceReachesAChildAsTheTarget(t *testing.T) {
	t.Parallel()
	const src = `target=T; declare -n foo=target; ` +
		`foo=bar /usr/bin/env | while IFS= read -r l; do ` +
		`case $l in foo=*|target=*) echo "$l" ;; esac; done`
	out, st := runBash(t, t.TempDir(), src)
	const want = `target=bar`
	if got := strings.TrimSpace(out); got != want || st != 0 {
		t.Errorf("%s = %q status %d, want %q", src, got, st, want)
	}
}
