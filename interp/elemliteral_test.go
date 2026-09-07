// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runElemLiteral runs src with the subscripted-literal axis answered, and with
// nothing else about arrays said: the base stays the core's so that no row
// here depends on which numeral names the first element.
func runElemLiteral(t *testing.T, p SubscriptedArrayLiteralPolicy, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.SubscriptedArrayLiteral = p
		r.Semantics = &sem
	})
}

// `a[i]=(p q)` is a third construct rather than either of the two it is spelled
// out of, and the axis is what says which reading applies.
//
// The bug it closes is the reading that is nobody's: the subscript was dropped
// and the literal became the whole array, silently, at status 0 and with an
// array on the other side — so a script that meant to replace one element
// replaced everything and nothing said so.
func TestASubscriptedArrayLiteralIsNotAWholeArrayAssignment(t *testing.T) {
	// Splicing. The subscript is `-1` so that the row asks about the
	// construct and not about the array base.
	out, st := runElemLiteral(t, SubscriptedArrayLiteralSplices,
		`a=(x y z); a[-1]=(p q); printf "[%s]" "${a[@]}"; echo " st=$?"`)
	if want := "[x][y][p][q] st=0"; strings.TrimSpace(out) != want {
		t.Errorf("splicing gave %q, want %q", strings.TrimSpace(out), want)
	}
	if st != 0 {
		t.Errorf("splicing: status %d, want 0", st)
	}

	// Refusing, which ends the script rather than reporting and running on.
	out, st = runElemLiteral(t, SubscriptedArrayLiteralRefused,
		`a=(x y z); a[-1]=(p q); echo ran-on`)
	if strings.Contains(out, "ran-on") {
		t.Errorf("refusing: %q ran on past the refusal", out)
	}
	if st == 0 {
		t.Error("refusing: status 0, want a failure")
	}
}

// An unanswered axis is refused by name rather than guessed at, because the
// two answers disagree about the array's length, its contents and the exit
// status — there is no reading that is nearly right.
func TestAnUnansweredSubscriptedArrayLiteralIsRefusedByName(t *testing.T) {
	out, _ := runElemLiteral(t, SubscriptedArrayLiteralUnspecified,
		`a=(x y z); a[-1]=(p q); printf "[%s]" "${a[@]}"`)
	if !strings.Contains(out, "a[i]=(p q)") {
		t.Errorf("%q does not name the construct it could not answer", out)
	}
	// And nothing was written on the way to saying so, which is the half a
	// refusal that only reported would have got wrong.
	if !strings.Contains(out, "[x][y][z]") {
		t.Errorf("%q, want the array left as it was", out)
	}
}

// The subscript decides which of the two operations `+=` is, exactly as it
// does for a scalar element: with one, the words join the element; without
// one, they are added after the last.
func TestASubscriptDecidesWhichLiteralAppend(t *testing.T) {
	out, _ := runElemLiteral(t, SubscriptedArrayLiteralSplices,
		`a=(x y); a+=(p); printf "[%s]" "${a[@]}"`)
	if want := "[x][y][p]"; out != want {
		t.Errorf("array append gave %q, want %q", out, want)
	}
	out, _ = runElemLiteral(t, SubscriptedArrayLiteralSplices,
		`a=(x y); a[-2]+=(p); printf "[%s]" "${a[@]}"`)
	if want := "[x][p][y]"; out != want {
		t.Errorf("element append gave %q, want %q", out, want)
	}
}

// An empty literal is a span of no words replacing a span of one, so the
// element goes and the array is one shorter. It is the row a real plugin
// writes, and the one that separates splicing from every reading that adds.
func TestAnEmptyLiteralThroughASubscriptRemovesTheElement(t *testing.T) {
	out, st := runElemLiteral(t, SubscriptedArrayLiteralSplices,
		`a=(x y z); a[-2]=(); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if want := "[x][z] n=2"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}
