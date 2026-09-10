// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `a+=x` over a name holding an array joins the array instead of replacing it
// with a string.
//
// It replaced it, at status 0 and with no diagnostic: `a=(1 2); a+=x` left the
// single scalar `1x` under bash's answers and `1 2x` under zsh's — the scalar
// *view* of the whole array with the value stuck on the end — where every
// shell in the panel with arrays leaves an array standing. Every later
// `${a[@]}`, `${#a[@]}` and append then read a different thing from what the
// script built (#1571).
//
// Where the value joins is a real disagreement and is answered on both sides
// here rather than picked: see Semantics.ScalarAppendedToAnArrayBecomesANewElement.
func appendSem(addsAnElement Answer) Semantics {
	s := testSemantics()
	s.ScalarAppendedToAnArrayBecomesANewElement = addsAnElement
	return s
}

func runScalarAppend(t *testing.T, sem Semantics, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamIndirection = true
	}, withSem(sem))
}

func TestAppendingAScalarToAnArrayKeepsTheArray(t *testing.T) {
	const src = `a=(1 2); a+=x; printf "[%s]" "${a[@]}"; echo " keys=${!a[@]} n=${#a[@]}"`
	for _, c := range []struct {
		addsAnElement Answer
		want          string
	}{
		// bash 5.3.15, the same binary under argv[0] of `sh`, bash 3.2.57 and
		// ksh93: the first element joined and the rest left standing.
		{No, "[1x][2] keys=0 1 n=2"},
		// zsh 5.9.2: a third element after the last.
		{Yes, "[1][2][x] keys=0 1 2 n=3"},
	} {
		out, st := runScalarAppend(t, appendSem(c.addsAnElement), src)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("addsAnElement=%v: got %q, want %q", c.addsAnElement, got, c.want)
		}
		if st != 0 {
			t.Errorf("addsAnElement=%v: status = %d, want 0", c.addsAnElement, st)
		}
	}
}

// Which element the joining answer joins: the base, and not the lowest
// subscript that has anything in it.
//
// Measured — `a=([5]=q); a+=x` lists `declare -a a=([0]="x" [5]="q")` in bash,
// so an element grows at the base where there was none and `q` does not move.
// It is the only probe that tells `a[0]+=x` from "append to the first element
// standing", which are the same operation on a dense array and different ones
// here. The subscripts are printed because the values alone read the same
// under either.
func TestAppendingAScalarToASparseArrayJoinsTheBase(t *testing.T) {
	out, _ := runScalarAppend(t, appendSem(No),
		`a=([5]=q); a+=x; printf "[%s]" "${a[@]}"; echo " keys=${!a[@]} n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "[x][q] keys=0 5 n=2" {
		t.Errorf("got %q, want the value at the base with the old element where it was", got)
	}
}

// The adding answer lands one past the highest subscript, which is where an
// appended array literal's first word goes — one rule about where the end of
// an array is rather than two.
func TestAppendingAScalarToASparseArrayAddsPastTheEnd(t *testing.T) {
	out, _ := runScalarAppend(t, appendSem(Yes),
		`a=([5]=q); a+=x; echo "keys=${!a[@]} n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "keys=5 6 n=2" {
		t.Errorf("got %q, want the value one past the highest subscript", got)
	}
}

// The empty string is a value on both sides of the axis: the joining answer
// leaves the array exactly as it stands and the adding answer grows an empty
// element.
//
// Its own test because an implementation that took an empty append for a
// no-op passes the joining side and fails only the other, and because the
// brackets alone read the same either way — the count is what says which.
func TestAppendingAnEmptyScalarToAnArray(t *testing.T) {
	for _, c := range []struct {
		addsAnElement Answer
		want          string
	}{
		{No, "[1][2] n=2"},
		{Yes, "[1][2][] n=3"},
	} {
		out, _ := runScalarAppend(t, appendSem(c.addsAnElement),
			`a=(1 2); a+=""; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("addsAnElement=%v: got %q, want %q", c.addsAnElement, got, c.want)
		}
	}
}

// And an empty *element* is joined like any other, which is the same claim
// from the other side: emptiness is never a special case on either half.
func TestAppendingAScalarToAnEmptyFirstElement(t *testing.T) {
	out, _ := runScalarAppend(t, appendSem(No),
		`a=("" 2); a+=x; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "[x][2] n=2" {
		t.Errorf("got %q, want the empty first element joined", got)
	}
}

// The appended value is one value however many words it looks like.
//
// The wrong turn is handing it through the field splitting an array literal's
// own words go through, which would leave `[1p][q][2]` here and `[p][q]` on
// the other side of the axis.
func TestAppendingAScalarWithASpaceToAnArray(t *testing.T) {
	for _, c := range []struct {
		addsAnElement Answer
		want          string
	}{
		{No, "[1p q][2] n=2"},
		{Yes, "[1][2][p q] n=3"},
	} {
		out, _ := runScalarAppend(t, appendSem(c.addsAnElement),
			`a=(1 2); a+="p q"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("addsAnElement=%v: got %q, want %q", c.addsAnElement, got, c.want)
		}
	}
}

// The boundary the axis stops at: a name holding no array is the ordinary
// string append and stays a plain scalar, unanimously and on both sides.
//
// Recorded because a fix that reached for the array store whenever `+=` was
// written would make an array here and nothing in the values would say so — a
// one-element array prints exactly the same. The listing is what tells them
// apart, and so does an assignment through a subscript afterwards.
func TestAppendingAScalarToAnUnsetNameStaysAScalar(t *testing.T) {
	for _, adds := range []Answer{No, Yes} {
		out, _ := runScalarAppend(t, appendSem(adds), `unset a; a+=x; typeset -p a`)
		if got := strings.TrimSpace(out); got != `declare -- a="x"` {
			t.Errorf("addsAnElement=%v: got %q, want a plain scalar", adds, got)
		}
	}
	// And the same of a name that was holding a scalar all along, which is
	// the half #1502 already answers for the array-literal spelling.
	out, _ := runScalarAppend(t, appendSem(No), `a=1; a+=x; typeset -p a`)
	if got := strings.TrimSpace(out); got != `declare -- a="1x"` {
		t.Errorf("over a scalar: got %q, want a plain scalar", got)
	}
}

// A plain assignment is not the append and must not reach it.
//
// Where the value goes is a second disagreement on the same pair of stores —
// ScalarAssignedOverACompoundReplacesTheName — so both of its answers are
// here, and neither of them is what the append does. The append's own axis is
// set to the *adding* answer throughout, so a mutant that let `a=x` fall into
// the `Append` branch would grow a third element and be caught on both rows.
func TestAssigningAScalarOverAnArrayIsNotTheAppend(t *testing.T) {
	for _, c := range []struct {
		replaces Answer
		want     string
	}{
		// The value is the whole of the name: one element, and no array
		// attribute left on the listing.
		{Yes, "declare -- a=\"x\"\nn=1"},
		// The first element written and the second left standing.
		{No, "declare -a a=([0]=\"x\" [1]=\"2\")\nn=2"},
	} {
		sem := appendSem(Yes)
		sem.ScalarAssignedOverACompoundReplacesTheName = c.replaces
		out, _ := runScalarAppend(t, sem, `a=(1 2); a=x; typeset -p a; echo "n=${#a[@]}"`)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("replaces=%v: got %q, want %q", c.replaces, got, c.want)
		}
	}
}

// The array-literal spelling is not this question either: `a+=(3)` adds an
// element in every shell that has arrays and asks nothing.
//
// Here so that the two appends cannot be folded into one: they share a
// spelling and the parentheses are the whole of what tells them apart.
func TestAppendingAnArrayLiteralIsUnchangedByTheAxis(t *testing.T) {
	for _, adds := range []Answer{No, Yes} {
		out, _ := runScalarAppend(t, appendSem(adds),
			`a=(1 2); a+=(3); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
		if got := strings.TrimSpace(out); got != "[1][2][3] n=3" {
			t.Errorf("addsAnElement=%v: got %q, want the literal append unchanged", adds, got)
		}
	}
}

// And so is a subscripted append, which reaches the element it names on both
// sides.
func TestAppendingThroughASubscriptIsUnchangedByTheAxis(t *testing.T) {
	for _, adds := range []Answer{No, Yes} {
		out, _ := runScalarAppend(t, appendSem(adds),
			`a=(1 2); a[1]+=x; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
		if got := strings.TrimSpace(out); got != "[1][2x] n=2" {
			t.Errorf("addsAnElement=%v: got %q, want the named element joined", adds, got)
		}
	}
}

// An unanswered dialect is refused by name rather than given one shell's
// answer, and the array is left as it was.
func TestAppendingAScalarToAnArrayRefusesAnUnansweredDialect(t *testing.T) {
	out, _ := runScalarAppend(t, appendSem(Unspecified),
		`a=(1 2); a+=x; echo "st=$?"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if !strings.Contains(out, "a scalar appended to an array becoming a new element") {
		t.Errorf("got %q, want the axis named", out)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("got %q, want the refusal to leave a status", out)
	}
	if !strings.Contains(out, "[1][2] n=2") {
		t.Errorf("got %q, want the array left exactly as it was", out)
	}
}
