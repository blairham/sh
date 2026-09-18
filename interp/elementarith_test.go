// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// elementArithSemantics answers what an element write and an arithmetic read
// need and nothing else.
func elementArithSemantics() Semantics {
	s := testSemantics()
	s.DeclareOptions = "aAfFgilnprux"
	s.UnsetOptions = "vfn"
	s.CompoundElementsGoThroughTheAttribute = Yes
	s.ArithNameValueRecurses = Yes
	s.ArraysAreSparse = Yes
	s.DeclareListing = DeclareListingClustered
	s.DeclareValueQuoting = ListingQuoteAlwaysEscaped
	s.NamerefCycleIsRefused = No
	s.BadNameToDeclarationFatal = No
	s.NamerefArrayRefusal = NamerefArrayCheckedLastOnTheAttribute
	// A reference's target is the text the declaration was written with,
	// looked up again at every read — the other answer is its own subject,
	// in interp/namerefsettled_test.go.
	s.NamerefTargetResolvedWhenAimed = No
	return s
}

func runElementArith(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := elementArithSemantics()
	return run(t, src, func(r *Runner) { r.Semantics = &sem })
}

// An element written to an integer name is folded **before** it reaches the
// array, so an expression that names the element it is being stored in reads
// what the element held rather than what is about to replace it.
//
// The order was the whole of it: the value went into the array and
// `storeArray` folded afterwards, so `a[0]+4` was evaluated with `a[0]`
// already holding the text `a[0]+4` — an expression that reads itself, on a
// path with no depth bound, which ended the process with a Go stack overflow
// rather than answering. bash 5.3.20 and ksh93u+ both answer 6. See #3487.
func TestAnElementIsFoldedBeforeItIsStored(t *testing.T) {
	out, st := runElementArith(t, `typeset -i a
a[0]=2
a[0]=a[0]+4
echo "[${a[0]}] st=$?"`)
	if !strings.Contains(out, "[6] st=0") || st != 0 {
		t.Errorf("got %q at %d, want the element read before it was replaced", out, st)
	}

	// The keyed spelling of the same three lines, which was right
	// throughout — it is the control that says this is the *order* and not
	// the fold.
	out, st = runElementArith(t, `typeset -A m; typeset -i m
m[k]=2
m[k]=m[k]+4
echo "[${m[k]}] st=$?"`)
	if !strings.Contains(out, "[6] st=0") || st != 0 {
		t.Errorf("keyed: got %q at %d, want 6", out, st)
	}

	// And the scalar control, which shares neither path.
	out, _ = runElementArith(t, `typeset -i s=2; s=s+4; echo "[$s]"`)
	if !strings.Contains(out, "[6]") {
		t.Errorf("scalar: got %q, want 6", out)
	}

	// An append through the same attribute joins and then folds once.
	out, _ = runElementArith(t, `typeset -i b; b[0]=1; b[0]=b[0]*7; echo "[${b[0]}]"`)
	if !strings.Contains(out, "[7]") {
		t.Errorf("multiply: got %q, want 7", out)
	}
}

// The recursion bound reaches the **element** read, so a value that names its
// own element is a diagnostic the script survives rather than a crash.
//
// `Runner.arithValueOf` has checked the depth since the bound existed;
// `Runner.arithElemValue` went straight to the stored-value reader, which
// recurses through `arithValueAsExpression` — incrementing the counter that
// nothing on that path read. bash 5.3.20 reports `a[0]: expression recursion
// level exceeded` and ksh93u+ `a[0]: recursion too deep`.
//
// **What the refusal then costs the script is not this change's claim**: a
// bounded arithmetic failure is FailedExpansionAbandonsTheLine's question and
// the name path already answers it the same way, so these assert the sentence,
// the subject, and that there is a run left to assert on at all.
func TestAnElementWhoseValueNamesItselfIsBoundedRatherThanFatal(t *testing.T) {
	out, st := run(t, `a=('a[0]')
echo "v=$(( a[0] ))"
echo after`, func(r *Runner) {
		s := elementArithSemantics()
		r.Semantics = &s
		r.Diagnostics = &Diagnostics{ArithRecursionLimit: "deep: %[1]s"}
	})
	if !strings.Contains(out, "deep: a[0]") {
		t.Errorf("got %q, want the bound reported against the element as written", out)
	}
	if strings.Contains(out, "v=") {
		t.Errorf("got %q, want no value — the expression did not finish", out)
	}
	if st == 0 {
		t.Errorf("got %q at %d, want the read to have failed", out, st)
	}

	// A keyed element the same way, since the two kinds reach the reader by
	// different routes.
	out, _ = run(t, `typeset -A m; m[k]='m[k]'
echo "v=$(( m[k] ))"
echo after`, func(r *Runner) {
		s := elementArithSemantics()
		r.Semantics = &s
		r.Diagnostics = &Diagnostics{ArithRecursionLimit: "deep: %[1]s"}
	})
	if !strings.Contains(out, "deep: m[k]") {
		t.Errorf("keyed: got %q, want the bound reported against the key", out)
	}

	// The control: an element whose value is an ordinary expression still
	// re-reads, so the bound is a bound and not a wall.
	out, _ = runElementArith(t, `c=('1+1'); echo "[$(( c[0] * 3 ))]"`)
	if !strings.Contains(out, "[6]") {
		t.Errorf("got %q, want the stored expression re-read", out)
	}
}

// A reference aimed at an **element** is read in arithmetic, which it was
// not: the store resolves a reference to a plain name and leaves the element
// half to the expansion, so every caller that is not an expansion has to ask
// — and this one did not.
//
// `a=(5 7); typeset -n b=a[0]; echo $(( b ))` is 5 in bash 5.3.20 and ksh93u+
// alike and was 0 here. A write through the same reference already landed, so
// an arithmetic that read and wrote used 0 as the old value: `(( b += 5 ))`
// left 5 where both shells leave 14, silently, at status 0.
func TestArithmeticReadsAReferenceAimedAtAnElement(t *testing.T) {
	out, _ := runElementArith(t, `a=(5 7)
typeset -n b=a[0]
typeset -n c=a[1]
v=9; typeset -n d=v
echo "[$(( b ))] [$(( b + 1 ))] [$(( c ))] [$(( d ))]"`)
	if !strings.Contains(out, "[5] [6] [7] [9]") {
		t.Errorf("got %q, want the elements read through the references", out)
	}

	// The read-and-write shapes, which are what the silence cost.
	out, _ = runElementArith(t, `a=(1 2)
typeset -n b=a[0]
(( b = 9 )); (( b += 5 )); (( b++ ))
echo "[${a[0]}] [${a[1]}]"`)
	if !strings.Contains(out, "[15] [2]") {
		t.Errorf("got %q, want 9 then 14 then 15 in the first element alone", out)
	}
}

// A `-n` declaration discards the attributes that exist to **fold a value**,
// because a reference holds none — and keeps the ones that do not.
//
// Measured 2026-09-17 with `tgt=T`: `typeset -i k; typeset -n k=tgt` is
// `declare -n k="tgt"` in bash 5.3.20 with no `i` left in it, and the `-l`
// and `-u` spellings likewise, while `typeset -x k` keeps its `x`. The
// behavior agrees with the listing in both shells: `typeset -i k; typeset -n
// k=tgt; k=3+4` leaves `tgt` holding the text `3+4`.
func TestADeclarationOfAReferenceDiscardsTheFoldingAttributes(t *testing.T) {
	for _, tc := range []struct{ letter, want string }{
		{"-i", `declare -n k='tgt'`},
		{"-l", `declare -n k='tgt'`},
		{"-u", `declare -n k='tgt'`},
		{"-x", `declare -nx k='tgt'`},
	} {
		out, _ := runElementArith(t,
			`tgt=T; typeset `+tc.letter+` k; typeset -n k=tgt; typeset -p k`)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: got %q, want %q", tc.letter, out, tc.want)
		}
	}

	// The behavior behind the listing: an integer attribute that survived
	// would evaluate what is written through the reference.
	out, _ := runElementArith(t, `tgt=zz; typeset -i k; typeset -n k=tgt; k=3+4; echo "[$tgt]"`)
	if !strings.Contains(out, "[3+4]") {
		t.Errorf("got %q, want the text stored rather than evaluated", out)
	}

	// And an attribute that arrives **after** the reference is the
	// reference's own and is kept.
	out, _ = runElementArith(t, `typeset -n y; typeset -i y; typeset -p y`)
	if !strings.Contains(out, "declare -in y") {
		t.Errorf("got %q, want the later attribute kept, i before n", out)
	}
}

// The clustered listing's letter order, which `n` used to stand first in.
//
// Only one pair can tell: `a` and `A` cannot stand with `n` at all — a `-n`
// declaration over an array is refused — so `i` is the whole of the
// measurement, and bash 5.3.20 writes `declare -in y`.
func TestTheClusteredListingPutsTheIntegerLetterBeforeTheReference(t *testing.T) {
	out, _ := runElementArith(t, `typeset -n y; typeset -i y; typeset -x y; typeset -p y`)
	if !strings.Contains(out, "declare -inx y") {
		t.Errorf("got %q, want declare -inx y", out)
	}

	// The letters measured either side of the pair, unchanged: the order is
	// one string and moving `n` must not move anything else.
	for _, tc := range []struct{ src, want string }{
		{`typeset -ai z; typeset -p z`, "declare -ai z"},
		{`typeset -ir z; typeset -p z`, "declare -ir z"},
		{`typeset -xl z=Q; typeset -p z`, `declare -xl z='q'`},
		{`typeset -rx z=1; typeset -p z`, `declare -rx z='1'`},
		{`typeset -Ax z; typeset -p z`, "declare -Ax z"},
	} {
		out, _ := runElementArith(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%q: got %q, want %q", tc.src, out, tc.want)
		}
	}
}
