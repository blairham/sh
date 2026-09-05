// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runBelowBase runs src with the array base and the negative-subscript axis
// both answered, since a case about the first element turns on both.
func runBelowBase(t *testing.T, zeroBased, inserts Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = zeroBased
		sem.NegativeSubscriptPastTheStartInserts = inserts
		r.Semantics = &sem
	})
}

// An assignment whose subscript lands before the first element is refused, and
// the refusal ends the script. It reported at status 0 and ran on, so the
// element was not written and the next command read the array as though it
// had been.
//
// Which subscript reaches it is the array base and nothing else, which is why
// the two answers need different spellings to show one rule: where the first
// element is 1, `a[0]` is below it; where the first element is 0, no
// non-negative subscript can be.
func TestASubscriptBelowTheFirstElementIsRefused(t *testing.T) {
	const src = `a=(p q); a[0]=x; printf "[%s]" "${a[@]}"; echo " ok"`
	out, st := runBelowBase(t, No, No, src)
	if !strings.Contains(out, "a[0]") {
		t.Errorf("one-based: %q does not name the subscript", out)
	}
	if strings.Contains(out, "ok") {
		t.Errorf("one-based: %q ran on past the refusal", out)
	}
	if st == 0 {
		t.Errorf("one-based: status 0, want a failure")
	}
	// The same numeral names the first element under the other answer, so
	// nothing is refused and nothing stops.
	if out, st := runBelowBase(t, Yes, No, src); strings.TrimSpace(out) != "[x][q] ok" || st != 0 {
		t.Errorf("zero-based: %q (status %d), want %q at 0", out, st, "[x][q] ok")
	}
}

// `+=` is the same assignment and reaches the same refusal, rather than
// joining a position that does not exist.
func TestAppendingBelowTheFirstElementIsRefused(t *testing.T) {
	out, st := runBelowBase(t, No, No, `a=(p q); a[0]+=Q; printf "[%s]" "${a[@]}"; echo " ok"`)
	if !strings.Contains(out, "a[0]") || strings.Contains(out, "ok") || st == 0 {
		t.Errorf("got %q (status %d), want a refusal that stops", out, st)
	}
}

// A subscript written as an expression is named as it was written, not as the
// number it came to — which is the one dialect that names it at all, and the
// reason the text is carried this far.
func TestARefusedSubscriptIsNamedAsWritten(t *testing.T) {
	out, _ := runBelowBase(t, Yes, No, `x=1; a[x-2]=v; echo ok`)
	if !strings.Contains(out, "a[x-2]") {
		t.Errorf("got %q, want the subscript as written", out)
	}
}

// A *negative* subscript that counts back past the first element is the one
// spelling the panel disagrees about: refused under one answer, and under the
// other it places an element in front of every other. However far past, the
// array grows by exactly one.
func TestANegativeSubscriptPastTheStartIsAnAxis(t *testing.T) {
	for _, src := range []string{
		`a=(p q); a[-3]=x; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		`a=(p q); a[-5]=x; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
	} {
		if out, st := runBelowBase(t, Yes, Yes, src); strings.TrimSpace(out) != "[x][p][q] n=3" || st != 0 {
			t.Errorf("inserting: %s = %q (status %d), want %q", src, out, st, "[x][p][q] n=3")
		}
		out, st := runBelowBase(t, Yes, No, src)
		if strings.Contains(out, "n=") || st == 0 {
			t.Errorf("refusing: %s = %q (status %d), want a refusal that stops", src, out, st)
		}
	}
	// On an array with nothing in it the same subscript makes the first
	// element, under the answer that places one.
	if out, st := runBelowBase(t, Yes, Yes, `a[-1]=x; printf "[%s]" "${a[@]}"; echo " ok"`); strings.TrimSpace(out) != "[x] ok" || st != 0 {
		t.Errorf("empty array = %q (status %d), want %q at 0", out, st, "[x] ok")
	}
	// And appending through one places the value with nothing to join.
	if out, _ := runBelowBase(t, Yes, Yes, `a=(p q); a[-3]+=Q; printf "[%s]" "${a[@]}"`); out != "[Q][p][q]" {
		t.Errorf("appending = %q, want %q", out, "[Q][p][q]")
	}
}

// The axis is asked only where a negative subscript actually ran past the
// start: one that lands on a real element needs no answer from anyone.
func TestANegativeSubscriptInsideTheArrayAsksNothing(t *testing.T) {
	for _, base := range []Answer{Yes, No} {
		out, st := runBelowBase(t, base, Unspecified, `a=(p q); a[-1]=x; a[-2]=y; printf "[%s]" "${a[@]}"`)
		if out != "[y][x]" || st != 0 {
			t.Errorf("base %v: got %q (status %d), want %q at 0", base, out, st, "[y][x]")
		}
	}
}

// An unanswered axis is refused by name rather than guessed, and refusing
// leaves the array alone.
func TestANegativeSubscriptPastTheStartRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := runBelowBase(t, Yes, Unspecified,
		`a=(p q); a[-3]=x; echo "st=$?"; printf "[%s]" "${a[@]}"`)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("output = %q, want a refusal naming the axis", out)
	}
	// Refused rather than placed, and the refusal is a failure: an
	// assignment that did not happen must not report success.
	if !strings.Contains(out, "st=2") {
		t.Errorf("output = %q, want the refusal's status", out)
	}
	if !strings.Contains(out, "[p][q]") {
		t.Errorf("output = %q, want the array untouched", out)
	}
}

// The same refusal reached through an array literal, which two of the three
// word differently from the plain form — so it is a wording of its own rather
// than the same sentence in a second place.
func TestASubscriptBelowTheFirstElementInALiteral(t *testing.T) {
	const src = `a=([0]=p); printf "[%s]" "${a[@]}"; echo " ok"`
	out, st := runBelowBase(t, No, No, src)
	if strings.Contains(out, "ok") || st == 0 {
		t.Errorf("one-based: got %q (status %d), want a refusal that stops", out, st)
	}
	if out, st := runBelowBase(t, Yes, No, src); strings.TrimSpace(out) != "[p] ok" || st != 0 {
		t.Errorf("zero-based: got %q (status %d), want %q at 0", out, st, "[p] ok")
	}
}
