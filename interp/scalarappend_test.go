// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// `a+=(x)` over a name holding a scalar keeps that value as the first element
// rather than building a fresh array from the words alone.
//
// It did not, and nothing said so: `a=1; a+=(2)` was the single element `2` at
// status 0, so a script that pushed onto a name a plain assignment had set
// lost the value it started from and read back a plausible array. Unanimous
// across every shell in the panel that has arrays at all -- see the corpus
// rows under `core/appending-an-array-literal-to-a-scalar`.
func TestAppendingALiteralToAScalarKeepsIt(t *testing.T) {
	out, st := runArray(t, `a=1; a+=(2); printf "[%s]" "${a[@]}"; echo " keys=${!a[@]} n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "[1][2] keys=0 1 n=2" {
		t.Errorf("got %q, want the scalar kept as the first element", got)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The empty string is a value like any other and is kept, so the array is two
// elements with an empty one in front.
//
// Its own test rather than a line in the one above, because an implementation
// that promoted only a non-empty scalar answers this exactly as it answers the
// unset name below -- and a count alone cannot tell those two states apart.
func TestAppendingALiteralToAnEmptyScalarKeepsIt(t *testing.T) {
	out, _ := runArray(t, `a=; a+=(2); printf "[%s]" "${a[@]}"; echo " keys=${!a[@]} n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "[][2] keys=0 1 n=2" {
		t.Errorf("got %q, want an empty first element in front of the appended one", got)
	}
}

// A name holding nothing has nothing to keep, so the literal's words are the
// whole array.
//
// The half a fix is likeliest to break: the natural way to keep a scalar reads
// the name and puts whatever came back in front of the words, and for an unset
// name that is an empty string nobody asked for.
func TestAppendingALiteralToAnUnsetNameAddsNothingInFront(t *testing.T) {
	out, _ := runArray(t, `unset a; a+=(2); printf "[%s]" "${a[@]}"; echo " keys=${!a[@]} n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "[2] keys=0 n=1" {
		t.Errorf("got %q, want the appended element alone", got)
	}
}

// The kept value is one element however many words it looks like.
//
// The specific wrong turn a fix takes by handing the old value back through
// the field splitting the literal's own bare elements go through: `x y` would
// become two elements and the count would be three.
func TestAppendingALiteralToAScalarDoesNotSplitIt(t *testing.T) {
	out, _ := runArray(t, `a="x y"; a+=(2); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "[x y][2] n=2" {
		t.Errorf("got %q, want the scalar kept whole as one element", got)
	}
}

// An append with no words still promotes: the name is left holding the one
// element it already had, as an array.
func TestAppendingAnEmptyLiteralToAScalarKeepsIt(t *testing.T) {
	out, _ := runArray(t, `a=1; a+=(); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "[1] n=1" {
		t.Errorf("got %q, want the scalar kept with nothing added", got)
	}
}

// A subscripted element in the appended literal still places where it says,
// and the kept scalar is at the first element rather than being pushed along
// by it.
//
// The combination, because each half works alone: promotion happens before the
// literal is placed, so the gap between them is a gap and not padding.
func TestAppendingASubscriptedLiteralToAScalarKeepsIt(t *testing.T) {
	out, _ := runArray(t, `a=1; a+=([3]=z); printf "[%s]" "${a[@]}"; echo " keys=${!a[@]} n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "[1][z] keys=0 3 n=2" {
		t.Errorf("got %q, want the scalar at the first element and z at 3", got)
	}
}

// The whole-array spelling still *replaces* the scalar, which is the half that
// was already right and the half a rule placed at the store would have broken.
//
// It is the negative that makes the rule above about the operator rather than
// about "an array store finding a scalar": `a=(2)` is one element and `a+=(2)`
// is two, from the same starting state.
func TestAssigningALiteralOverAScalarStillReplacesIt(t *testing.T) {
	out, _ := runArray(t, `a=1; a=(2); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "[2] n=1" {
		t.Errorf("got %q, want the scalar replaced rather than kept", got)
	}
}

// Appending to a name that is already an array is unchanged: the words go
// after the highest subscript and nothing is promoted in front of them.
func TestAppendingALiteralToAnArrayIsUnchanged(t *testing.T) {
	out, _ := runArray(t, `a=(1 2); a+=(3); printf "[%s]" "${a[@]}"; echo " keys=${!a[@]} n=${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "[1][2][3] keys=0 1 2 n=3" {
		t.Errorf("got %q, want the append unchanged over an array", got)
	}
}
