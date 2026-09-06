// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runUnsetBelowBase runs src with both axes a subscript before the first
// element turns on: the base, which decides which spelling can reach the
// boundary, and the span policy, which decides whether `unset` removes or
// blanks and so whether the negative spelling reaches it at all.
func runUnsetBelowBase(t *testing.T, zeroBased Answer, p UnsetArraySpanPolicy, src string, set ...func(*Semantics)) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = zeroBased
		sem.UnsetArraySpan = p
		for _, f := range set {
			f(&sem)
		}
		r.Semantics = &sem
	})
}

// `unset a[0]` where the first element is 1 names nothing, and it was silent
// at status 0 — telling a script it had removed something out of reach. It is
// the boundary an assignment already refuses, reached from `unset`.
func TestUnsetBelowTheFirstElementIsRefused(t *testing.T) {
	const src = `a=(x y z); unset "a[0]"; echo "st=$?"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
	out, st := runUnsetBelowBase(t, No, UnsetArraySpanLeavesOneEmptyElement, src)
	if !strings.Contains(out, "a") || !strings.Contains(out, "st=1") {
		t.Errorf("one-based: %q, want a refusal reported at 1", out)
	}
	if !strings.Contains(out, "[x][y][z] n=3") {
		t.Errorf("one-based: %q, want the array left as it was", out)
	}
	if st != 0 {
		t.Errorf("one-based: script status = %d, want the refusal not to end it", st)
	}
	// The same numeral names the first element under the other base, so
	// nothing is refused and the element goes.
	if out, _ := runUnsetBelowBase(t, Yes, UnsetArraySpanRemovesTheElements, src); !strings.Contains(out, "st=0") ||
		!strings.Contains(out, "[y][z] n=2") {
		t.Errorf("zero-based: %q, want the first element removed at 0", out)
	}
}

// The refusal leaves a failed builtin behind rather than ending the script,
// which is where this route parts from the assignment's: that one stops. A
// script can therefore test it, and reusing the fatal path would have been a
// new bug rather than a fix.
func TestUnsetBelowTheFirstElementDoesNotEndTheScript(t *testing.T) {
	out, st := runUnsetBelowBase(t, No, UnsetArraySpanLeavesOneEmptyElement,
		`a=(x y z); unset "a[0]"; echo after`)
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("got %q (status %d), want the next command to run", out, st)
	}
}

// The other side of the same boundary: a negative subscript counting back
// past the start. Only the removing answer reaches it — blanking replaces a
// span that is there and a subscript past the start names none, so it is
// silent rather than refused, the same reading `unset a[-2]` records.
func TestUnsetPastTheStartIsRefusedOnlyWhereTheAnswerRemoves(t *testing.T) {
	const src = `a=(x y z); unset "a[-4]"; echo "st=$?"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
	out, _ := runUnsetBelowBase(t, Yes, UnsetArraySpanRemovesTheElements, src)
	if !strings.Contains(out, "st=1") || !strings.Contains(out, "[x][y][z] n=3") {
		t.Errorf("removing: %q, want a refusal at 1 with the array untouched", out)
	}
	out, _ = runUnsetBelowBase(t, No, UnsetArraySpanLeavesOneEmptyElement, src)
	if !strings.Contains(out, "st=0") || !strings.Contains(out, "[x][y][z] n=3") {
		t.Errorf("blanking: %q, want silence at 0 with the array untouched", out)
	}
	if strings.Contains(out, "subscript") {
		t.Errorf("blanking: %q, want nothing said", out)
	}
	// `-1` on an array with nothing in it is past the start too, and the
	// blanking answer is silent about that one as well — the boundary it
	// reaches is the non-negative spelling and only that one.
	out, _ = runUnsetBelowBase(t, No, UnsetArraySpanLeavesOneEmptyElement,
		`a=(); unset "a[-1]"; echo "st=$?"`)
	if strings.TrimSpace(out) != "st=0" {
		t.Errorf("blanking, empty array: %q, want %q", strings.TrimSpace(out), "st=0")
	}
}

// The boundary is *before* the first element and not at it: under the
// blanking answer a scalar's own element is reachable, and only what stands
// below it is refused.
func TestUnsetAtTheFirstElementOfAScalarIsNotRefused(t *testing.T) {
	out, _ := runUnsetBelowBase(t, No, UnsetArraySpanLeavesOneEmptyElement,
		`a=v; unset "a[1]"; echo "st=$?"`)
	if strings.TrimSpace(out) != "st=0" {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), "st=0")
	}
}

// A subscript written as an expression is named as it was written and not as
// the number it came to, which is the reason the text is carried this far.
func TestARefusedUnsetSubscriptIsNamedAsWritten(t *testing.T) {
	out, _ := runUnsetBelowBase(t, Yes, UnsetArraySpanRemovesTheElements,
		`a=(x y z); unset "a[x-9]"; echo "st=$?"`)
	if !strings.Contains(out, "x-9") {
		t.Errorf("got %q, want the subscript as written", out)
	}
	if strings.Contains(out, "-9]:") && !strings.Contains(out, "x-9") {
		t.Errorf("got %q, want the written text rather than the value", out)
	}
}

// A scalar has a first *character*, and a subscript can be before it. Which
// reading a subscripted string takes is ScalarSubscriptIsACharacter, and it is
// that reading and not the span policy that brings a scalar to this boundary:
// the readings that see an element there see the one element a scalar is, at
// the base, and every other subscript is UnsetSubscriptOnAScalarIsAnError's
// question rather than this one.
func TestUnsetBelowTheFirstCharacterOfAScalar(t *testing.T) {
	const src = `a=v; unset "a[0]"; echo "st=$? [${a-UNSET}]"`
	out, _ := runUnsetBelowBase(t, No, UnsetArraySpanLeavesOneEmptyElement, src,
		func(s *Semantics) { s.ScalarSubscriptIsACharacter = Yes })
	if out != "sh: unset: [0]: bad array subscript\nst=1 [v]\n" {
		t.Errorf("characters: %q, want a refusal at 1 with the value kept", out)
	}
	// The same subscript under the same base, read as an element: it names
	// nothing, because the one element a scalar is sits at the base.
	out, _ = runUnsetBelowBase(t, No, UnsetArraySpanLeavesOneEmptyElement, src,
		func(s *Semantics) { s.ScalarSubscriptIsACharacter = No })
	if out != "st=0 [v]\n" {
		t.Errorf("elements, base 1: %q, want the subscript to name nothing", out)
	}
	// And under the base that puts the element at 0, it names the scalar and
	// takes the name away.
	out, _ = runUnsetBelowBase(t, Yes, UnsetArraySpanRemovesTheElements, src,
		func(s *Semantics) { s.ScalarSubscriptIsACharacter = No })
	if out != "st=0 [UNSET]\n" {
		t.Errorf("elements, base 0: %q, want the whole name taken away", out)
	}
}

// A name holding nothing at all has no first element for a subscript to be
// before, so there is nothing to refuse under any answer.
func TestUnsetBelowTheFirstElementOfANameHoldingNothing(t *testing.T) {
	for _, p := range []UnsetArraySpanPolicy{
		UnsetArraySpanRemovesTheElements,
		UnsetArraySpanLeavesOneEmptyElement,
	} {
		out, st := runUnsetBelowBase(t, No, p, `unset "a[0]"; echo "st=$?"`)
		if strings.TrimSpace(out) != "st=0" || st != 0 {
			t.Errorf("%v: got %q (status %d), want %q at 0", p, strings.TrimSpace(out), st, "st=0")
		}
	}
}
