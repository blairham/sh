// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runUnsetArrayAt runs src with the UnsetArrayAt axis set to p.
func runUnsetArrayAt(t *testing.T, p UnsetArrayAtPolicy, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.UnsetArrayAt = p
		r.Semantics = &sem
	})
}

// `unset a[@]` names every element rather than one, and it was doing nothing
// at all: `@` is not an arithmetic expression, so the subscript failed to
// evaluate and the element nobody named was quietly not removed. The array
// came back whole at status 0 — the silent kind of wrong, in the spelling a
// script uses to start a list over.
func TestUnsetOfEveryElementIsThreeAnswers(t *testing.T) {
	for _, c := range []struct {
		name string
		p    UnsetArrayAtPolicy
		want string
	}{
		{"removes every element", UnsetArrayAtRemovesEveryElement, "[] n=0"},
		{"leaves one empty element", UnsetArrayAtLeavesOneEmptyElement, "[] n=1"},
		// No such reading: the brackets hold an expression, `@` is not one,
		// and the array is left as it was.
		{"a subscript", UnsetArrayAtIsASubscript, "[p][q][r] n=3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Both spellings, because the star and the at are one question
			// here where they part company in an expansion.
			for _, sub := range []string{"@", "*"} {
				src := `a=(p q r); unset "a[` + sub + `]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
				out, st := runUnsetArrayAt(t, c.p, src)
				if got := strings.TrimSpace(out); got != c.want {
					t.Errorf("[%s] = %q, want %q", sub, got, c.want)
				}
				if st != 0 {
					t.Errorf("[%s]: status = %d, want 0", sub, st)
				}
			}
		})
	}
}

// Where the next append lands, which is what tells an emptied array from one
// holding a single empty string. A count cannot say it and a script that
// resets a list and pushes onto it reads it on the first element.
func TestUnsetOfEveryElementDecidesWhereAnAppendLands(t *testing.T) {
	const src = `a=(p q); unset "a[@]"; a+=(z); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
	for _, c := range []struct {
		p    UnsetArrayAtPolicy
		want string
	}{
		{UnsetArrayAtRemovesEveryElement, "[z] n=1"},
		{UnsetArrayAtLeavesOneEmptyElement, "[][z] n=2"},
	} {
		if out, _ := runUnsetArrayAt(t, c.p, src); strings.TrimSpace(out) != c.want {
			t.Errorf("%v: got %q, want %q", c.p, strings.TrimSpace(out), c.want)
		}
	}
}

// The same spelling on a name that is no array is where the two readings show
// what they mean. Taking every element away from a scalar is refused, because
// it has none; making the span it names one empty string empties it, because a
// scalar is one such span. Neither turns the name into an array.
func TestUnsetOfEveryElementOfAScalar(t *testing.T) {
	const src = `a=hello; unset "a[@]"; echo "st=$? [$a]"`
	for _, c := range []struct {
		p    UnsetArrayAtPolicy
		want string
	}{
		{UnsetArrayAtRemovesEveryElement, "st=1 [hello]"},
		{UnsetArrayAtLeavesOneEmptyElement, "st=0 []"},
	} {
		out, _ := runUnsetArrayAt(t, c.p, src)
		if got := lastLine(out); got != c.want {
			t.Errorf("%v: got %q, want %q", c.p, got, c.want)
		}
	}
}

// A name holding nothing at all is quiet under every answer, and gains no
// array: there is nothing to empty and nothing to complain about.
func TestUnsetOfEveryElementOfANameThatHoldsNothing(t *testing.T) {
	const src = `unset "a[@]"; echo "st=$? n=${#a[@]}"`
	for _, p := range []UnsetArrayAtPolicy{
		UnsetArrayAtRemovesEveryElement,
		UnsetArrayAtLeavesOneEmptyElement,
		UnsetArrayAtIsASubscript,
	} {
		out, st := runUnsetArrayAt(t, p, src)
		if got := strings.TrimSpace(out); got != "st=0 n=0" {
			t.Errorf("%v: got %q, want %q", p, got, "st=0 n=0")
		}
		if st != 0 {
			t.Errorf("%v: status = %d, want 0", p, st)
		}
	}
}

// An array that is already empty has no span to replace, so the answer that
// leaves one empty element does not invent one.
func TestUnsetOfEveryElementOfAnEmptyArray(t *testing.T) {
	const src = `a=(); unset "a[@]"; echo "n=${#a[@]}"`
	for _, p := range []UnsetArrayAtPolicy{
		UnsetArrayAtRemovesEveryElement,
		UnsetArrayAtLeavesOneEmptyElement,
	} {
		if out, _ := runUnsetArrayAt(t, p, src); strings.TrimSpace(out) != "n=0" {
			t.Errorf("%v: got %q, want %q", p, strings.TrimSpace(out), "n=0")
		}
	}
}

// The whole-array reading belongs to the indexed array alone. With the keyed
// attribute on, `@` is a key like any other and nothing was stored under it,
// so both elements stay — under every answer, including the two that clear an
// indexed array through the same spelling.
func TestUnsetOfEveryElementIsAKeyWhereTheAttributeIsOn(t *testing.T) {
	const src = `typeset -A m; m[k]=v; m[j]=w; unset "m[@]"; echo "n=${#m[@]}"`
	for _, p := range []UnsetArrayAtPolicy{
		UnsetArrayAtRemovesEveryElement,
		UnsetArrayAtLeavesOneEmptyElement,
		UnsetArrayAtIsASubscript,
	} {
		if out, _ := runUnsetArrayAt(t, p, src); strings.TrimSpace(out) != "n=2" {
			t.Errorf("%v: got %q, want %q", p, strings.TrimSpace(out), "n=2")
		}
	}
}

// One element is not the question, and no answer may change it: `unset a[1]`
// names a single subscript in all three shells with arrays.
func TestUnsetOfOneElementAsksNothing(t *testing.T) {
	const src = `a=(p q r); unset "a[1]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
	for _, p := range []UnsetArrayAtPolicy{
		UnsetArrayAtUnspecified,
		UnsetArrayAtRemovesEveryElement,
		UnsetArrayAtLeavesOneEmptyElement,
		UnsetArrayAtIsASubscript,
	} {
		out, st := runUnsetArrayAt(t, p, src)
		if got := strings.TrimSpace(out); got != "[p][r] n=2" {
			t.Errorf("%v: got %q, want %q", p, got, "[p][r] n=2")
		}
		if st != 0 {
			t.Errorf("%v: status = %d, want 0", p, st)
		}
	}
}

// An axis nobody answered is refused by name rather than guessed — and, since
// the bug was that the spelling did nothing quietly, refusing it has to be
// loud too.
func TestUnsetOfEveryElementRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := runUnsetArrayAt(t, UnsetArrayAtUnspecified,
		`a=(p q r); unset "a[@]"; echo "st=$? n=${#a[@]}"`)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("output = %q, want a refusal naming the axis", out)
	}
	// Refused rather than done one shell's way, and the array is left as it
	// was — which is the same store the answer that reads `@` as a subscript
	// leaves, reached for the opposite reason.
	if got := lastLine(out); got != "st=2 n=3" {
		t.Errorf("got %q, want %q", got, "st=2 n=3")
	}
}
