// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A `[sub]=value` element of an array literal places its value at that
// subscript.
//
// It used to become an element holding the text, so `a=([2]=c [0]=a)` was two
// elements reading `[2]=c` and `[0]=a` — the array is the right length and
// only its contents are nonsense, which is why nothing noticed.
func TestALiteralPlacesItsSubscripts(t *testing.T) {
	out, st := runArray(t, `a=([2]=c [1]=b); printf "[%s]" "${a[@]}"; echo " keys=${!a[@]} n=${#a[@]}"`)
	if strings.TrimSpace(out) != "[b][c] keys=1 2 n=2" {
		t.Errorf("got %q, want the values placed at 1 and 2", strings.TrimSpace(out))
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if strings.Contains(out, "[2]=c") {
		t.Errorf("got %q, want the brackets read rather than stored", out)
	}
}

// Placing above what has been filled leaves a gap rather than padding one:
// the store is sparse and only the reading of it is a question.
func TestALiteralLeavesAGap(t *testing.T) {
	out, _ := runArray(t, `a=([2]=c); echo "keys=${!a[@]} n=${#a[@]}"`)
	if strings.TrimSpace(out) != "keys=2 n=1" {
		t.Errorf("got %q, want one element at subscript 2", strings.TrimSpace(out))
	}
}

// A bare element after a subscripted one continues from that subscript rather
// than from where the count had reached.
func TestALiteralContinuesFromTheSubscript(t *testing.T) {
	out, _ := runArray(t, `a=(x [3]=y z); echo "keys=${!a[@]} n=${#a[@]}"`)
	if strings.TrimSpace(out) != "keys=0 3 4 n=3" {
		t.Errorf("got %q, want the bare element one past the subscripted one", strings.TrimSpace(out))
	}
}

// The same subscript twice is one element holding the later value, which is
// the only answer under which the elements are placed in the order written.
func TestALiteralRepeatingASubscript(t *testing.T) {
	out, _ := runArray(t, `a=([2]=c [2]=d); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if strings.TrimSpace(out) != "[d] n=1" {
		t.Errorf("got %q, want the later value alone", strings.TrimSpace(out))
	}
}

// `+=` keeps what is there and a subscripted element still places, rather than
// landing after the end — the combination, because each half works alone.
func TestAppendingALiteralWithASubscript(t *testing.T) {
	out, _ := runArray(t, `a=([2]=c); a+=([5]=f); echo "keys=${!a[@]} n=${#a[@]}"`)
	if strings.TrimSpace(out) != "keys=2 5 n=2" {
		t.Errorf("got %q, want the appended element at its own subscript", strings.TrimSpace(out))
	}
	// And a bare element in an appended literal still goes after the end.
	out, _ = runArray(t, `a=([2]=c); a+=(f); echo "keys=${!a[@]}"`)
	if strings.TrimSpace(out) != "keys=2 3" {
		t.Errorf("got %q, want the bare element one past the highest subscript", strings.TrimSpace(out))
	}
}

// The value of a subscripted element is an assignment's value and is not
// split; a bare element in the same parentheses is a word and is. One set of
// parentheses, two expansion rules, and the subscript chooses.
func TestALiteralValueIsAnAssignmentValue(t *testing.T) {
	out, _ := runArray(t, `x="p q"; a=([2]=$x); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if strings.TrimSpace(out) != "[p q] n=1" {
		t.Errorf("subscripted element gave %q, want one unsplit element", strings.TrimSpace(out))
	}
	out, _ = runArray(t, `x="p q"; a=($x); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if strings.TrimSpace(out) != "[p][q] n=2" {
		t.Errorf("bare element gave %q, want it split", strings.TrimSpace(out))
	}
}

// The written subscript goes through the array base, so the same literal
// fills the same *positions* whichever number the first element answers to.
func TestALiteralSubscriptInheritsTheBase(t *testing.T) {
	// Sparse answered because the literal leaves a gap and how a gap reads is
	// a different axis from where the values went.
	vectors := baseVectors()
	for i := range vectors {
		vectors[i].ArraysAreSparse = Yes
	}
	src := `a=(x [3]=y z); printf "[%s]" "${a[@]}"`
	if out, _ := run(t, src, withSem(vectors[0])); out != "[x][y][z]" {
		t.Errorf("zero-based gave %q, want three elements", out)
	}
	// One-based fills the same three positions, one lower in the store, and
	// reads them back under the numbers a script would have written.
	if out, _ := run(t, src, withSem(vectors[1])); out != "[x][y][z]" {
		t.Errorf("one-based gave %q, want three elements", out)
	}
	indices := func(d *syntax.Dialect) { d.ParamIndirection = true }
	if out, _ := runGrammar(t, `a=(x [3]=y z); echo "${!a[@]}"`, indices, withSem(vectors[1])); strings.TrimSpace(out) != "1 3 4" {
		t.Errorf("one-based subscripts are %q, want the subscripts as written", strings.TrimSpace(out))
	}
}

// A subscript a script assigns *through* is an arithmetic expression and not
// only a numeral, in the literal and in a plain element assignment alike.
func TestAWrittenSubscriptIsAnExpression(t *testing.T) {
	out, _ := runArray(t, `i=2; a=([1+1]=c); echo "keys=${!a[@]} v=${a[2]}"`)
	if strings.TrimSpace(out) != "keys=2 v=c" {
		t.Errorf("literal gave %q, want the expression evaluated", strings.TrimSpace(out))
	}
	out, _ = runArray(t, `i=1; a=(x y); a[i+0]=Q; printf "[%s]" "${a[@]}"`)
	if strings.TrimSpace(out) != "[x][Q]" {
		t.Errorf("element assignment gave %q, want the expression evaluated", strings.TrimSpace(out))
	}
}

// ArrayLiteralSubscriptIsAKey, both answers. Under one the text is evaluated
// and two spellings of 2 are the same element; under the other it is the key
// and they are two.
func TestALiteralSubscriptAsAKey(t *testing.T) {
	src := `i=2; a=([1+1]=c [i]=d); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`

	expr := permissive()
	expr.ArrayBaseIsZero = Yes
	expr.ArraysAreSparse = Yes
	expr.ArrayLiteralSubscriptIsAKey = No
	if out, _ := run(t, src, withSem(expr)); strings.TrimSpace(out) != "[d] n=1" {
		t.Errorf("as an expression gave %q, want one element", strings.TrimSpace(out))
	}

	key := permissive()
	key.ArrayBaseIsZero = Yes
	key.ArraysAreSparse = Yes
	key.ArrayLiteralSubscriptIsAKey = Yes
	if out, _ := run(t, src, withSem(key)); strings.TrimSpace(out) != "[c][d] n=2" {
		t.Errorf("as a key gave %q, want two elements", strings.TrimSpace(out))
	}
}

// Unanswered, the axis refuses and names itself rather than picking a side —
// but only where the two answers differ. A plain numeral evaluates to itself,
// so the ordinary sparse literal is available in a core that has chosen no
// shell.
func TestALiteralSubscriptAsksOnlyWhereItMatters(t *testing.T) {
	none := permissive()
	none.ArrayBaseIsZero = Yes
	none.ArraysAreSparse = Yes
	out, _ := run(t, `a=([2]=c [0]=a); printf "[%s]" "${a[@]}"`, withSem(none))
	if out != "[a][c]" {
		t.Errorf("a numeral subscript gave %q, want it placed without asking", out)
	}
	out, _ = run(t, `a=([1+1]=c); printf "[%s]" "${a[@]}"`, withSem(none))
	if !strings.Contains(out, "disagree") {
		t.Errorf("an expression subscript gave %q, want the axis named", out)
	}
}
