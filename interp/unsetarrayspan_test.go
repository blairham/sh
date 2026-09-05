// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runUnsetArraySpan runs src with the UnsetArraySpan axis set to p.
func runUnsetArraySpan(t *testing.T, p UnsetArraySpanPolicy, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.UnsetArraySpan = p
		r.Semantics = &sem
	})
}

// `unset a[@]` names every element rather than one, and it was doing nothing
// at all: `@` is not an arithmetic expression, so the subscript failed to
// evaluate and the element nobody named was quietly not removed. The array
// came back whole at status 0 — the silent kind of wrong, in the spelling a
// script uses to start a list over.
func TestUnsetOfEveryElementClearsWhereThatIsTheReading(t *testing.T) {
	for _, c := range []struct {
		name string
		p    UnsetArraySpanPolicy
		want string
	}{
		{"removes every element", UnsetArraySpanRemovesTheElements, "[] n=0"},
		{"leaves one empty element", UnsetArraySpanLeavesOneEmptyElement, "[] n=1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Both spellings, because the star and the at are one question
			// here where they part company in an expansion.
			for _, sub := range []string{"@", "*"} {
				src := `a=(p q r); unset "a[` + sub + `]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
				out, st := runUnsetArraySpan(t, c.p, src)
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
		p    UnsetArraySpanPolicy
		want string
	}{
		{UnsetArraySpanRemovesTheElements, "[z] n=1"},
		{UnsetArraySpanLeavesOneEmptyElement, "[][z] n=2"},
	} {
		if out, _ := runUnsetArraySpan(t, c.p, src); strings.TrimSpace(out) != c.want {
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
		p    UnsetArraySpanPolicy
		want string
	}{
		{UnsetArraySpanRemovesTheElements, "st=1 [hello]"},
		{UnsetArraySpanLeavesOneEmptyElement, "st=0 []"},
	} {
		out, _ := runUnsetArraySpan(t, c.p, src)
		if got := lastLine(out); got != c.want {
			t.Errorf("%v: got %q, want %q", c.p, got, c.want)
		}
	}
}

// The dialect with no whole-array reading treats `@` as the expression it is
// not, and reports it — the same complaint, and the same two answers about
// what follows, as any other subscript that will not evaluate. Both halves are
// asserted here because #648 left this column silent: the array survived, and
// nothing said why.
func TestUnsetOfEveryElementIsAnOrdinaryBadSubscript(t *testing.T) {
	for _, c := range []struct {
		name  string
		fatal Answer
		want  string
	}{
		{"fatal", Yes, ""},
		{"a failed builtin", No, "st=1 n=3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runGrammar(t, `a=(p q r); unset "a[@]"; echo "st=$? n=${#a[@]}"`, nil,
				func(r *Runner) {
					sem := *r.Semantics
					sem.UnsetArraySpan = UnsetArraySpanIsAnExpression
					sem.BadSubscriptToUnsetFatal = c.fatal
					r.Semantics = &sem
				})
			if !strings.Contains(out, "@") {
				t.Errorf("output = %q, want the subscript named", out)
			}
			if c.want == "" {
				// The script stopped, so nothing after the `unset` ran.
				if strings.Contains(out, "st=") {
					t.Errorf("output = %q, want nothing after the unset", out)
				}
				return
			}
			if got := lastLine(out); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// A name holding nothing at all is quiet under the two answers that have a
// whole-array reading, and gains no array: there is nothing to empty and
// nothing to complain about. The third has no such reading and reports the
// subscript instead, which is its own test.
func TestUnsetOfEveryElementOfANameThatHoldsNothing(t *testing.T) {
	const src = `unset "a[@]"; echo "st=$? n=${#a[@]}"`
	for _, p := range []UnsetArraySpanPolicy{
		UnsetArraySpanRemovesTheElements,
		UnsetArraySpanLeavesOneEmptyElement,
	} {
		out, st := runUnsetArraySpan(t, p, src)
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
	for _, p := range []UnsetArraySpanPolicy{
		UnsetArraySpanRemovesTheElements,
		UnsetArraySpanLeavesOneEmptyElement,
	} {
		if out, _ := runUnsetArraySpan(t, p, src); strings.TrimSpace(out) != "n=0" {
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
	for _, p := range []UnsetArraySpanPolicy{
		UnsetArraySpanRemovesTheElements,
		UnsetArraySpanLeavesOneEmptyElement,
		UnsetArraySpanIsAnExpression,
	} {
		if out, _ := runUnsetArraySpan(t, p, src); strings.TrimSpace(out) != "n=2" {
			t.Errorf("%v: got %q, want %q", p, strings.TrimSpace(out), "n=2")
		}
	}
}

// A single subscript is the same axis at a span of one, which is why there is
// one field and not two. Two of the answers take the element away and the
// third replaces the span with an empty string in the place it stood, so the
// array keeps its length.
//
// The subscript here is the last element, because that is the only place the
// difference shows: a removed subscript in the middle reads back empty under
// a dense reading anyway, so the two answers are indistinguishable there and
// the bug hid behind it.
func TestUnsetOfOneElementFollowsTheSameAxis(t *testing.T) {
	const src = `a=(p q r); unset "a[2]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
	for _, c := range []struct {
		p    UnsetArraySpanPolicy
		want string
	}{
		{UnsetArraySpanUnspecified, "[p][q] n=2"},
		{UnsetArraySpanRemovesTheElements, "[p][q] n=2"},
		{UnsetArraySpanIsAnExpression, "[p][q] n=2"},
		{UnsetArraySpanLeavesOneEmptyElement, "[p][q][] n=3"},
	} {
		out, st := runUnsetArraySpan(t, c.p, src)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%v: got %q, want %q", c.p, got, c.want)
		}
		if st != 0 {
			t.Errorf("%v: status = %d, want 0", c.p, st)
		}
	}
}

// An unanswered axis is not refused at a single subscript, where it is at
// `[@]`: the removal reading is what a preset with no arrays of its own has
// already committed to, so `unset a[2]` is answerable everywhere.
func TestUnsetOfOneElementRefusesNothing(t *testing.T) {
	out, st := runUnsetArraySpan(t, UnsetArraySpanUnspecified,
		`a=(p q r); unset "a[2]"; echo "st=$?"`)
	if strings.Contains(out, "no dialect was chosen") {
		t.Errorf("output = %q, want no refusal", out)
	}
	if got := lastLine(out); got != "st=0" || st != 0 {
		t.Errorf("got %q (status %d), want %q", got, st, "st=0")
	}
}

// Blanking is what the *span* does, so it reaches only a subscript that names
// an element already there. One past the end has no span to replace and the
// array does not grow.
func TestUnsetOfOneElementPastTheEndLeavesTheArrayAlone(t *testing.T) {
	const src = `a=(p q r); unset "a[9]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
	for _, p := range []UnsetArraySpanPolicy{
		UnsetArraySpanRemovesTheElements,
		UnsetArraySpanLeavesOneEmptyElement,
	} {
		if out, _ := runUnsetArraySpan(t, p, src); strings.TrimSpace(out) != "[p][q][r] n=3" {
			t.Errorf("%v: got %q, want %q", p, strings.TrimSpace(out), "[p][q][r] n=3")
		}
	}
}

// The end-relative subscripts, where the blanking answer reaches `-1` and no
// other: `unset a[-2]` leaves the array as it was where the removing answer
// takes the middle element away. Measured rather than reasoned — the two
// readings agree about what `-2` *names* and disagree about whether `unset`
// acts on it.
func TestUnsetOfOneElementFromTheEnd(t *testing.T) {
	for _, c := range []struct {
		sub             string
		removes, blanks string
	}{
		{"-1", "[p][q] n=2", "[p][q][] n=3"},
		{"-2", "[p][r] n=2", "[p][q][r] n=3"},
	} {
		src := `a=(p q r); unset "a[` + c.sub + `]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
		if out, _ := runUnsetArraySpan(t, UnsetArraySpanRemovesTheElements, src); strings.TrimSpace(out) != c.removes {
			t.Errorf("[%s] removing: got %q, want %q", c.sub, strings.TrimSpace(out), c.removes)
		}
		if out, _ := runUnsetArraySpan(t, UnsetArraySpanLeavesOneEmptyElement, src); strings.TrimSpace(out) != c.blanks {
			t.Errorf("[%s] blanking: got %q, want %q", c.sub, strings.TrimSpace(out), c.blanks)
		}
	}
}

// The keyed attribute takes the subscript away under every answer, exactly as
// it does for `[@]`: a key is removed from the table and nothing is left
// standing in its place.
func TestUnsetOfOneKeyRemovesItUnderEveryAnswer(t *testing.T) {
	const src = `typeset -A m; m[k]=v; m[j]=w; unset "m[k]"; echo "n=${#m[@]} [${m[k]}]"`
	for _, p := range []UnsetArraySpanPolicy{
		UnsetArraySpanUnspecified,
		UnsetArraySpanRemovesTheElements,
		UnsetArraySpanLeavesOneEmptyElement,
		UnsetArraySpanIsAnExpression,
	} {
		if out, _ := runUnsetArraySpan(t, p, src); strings.TrimSpace(out) != "n=1 []" {
			t.Errorf("%v: got %q, want %q", p, strings.TrimSpace(out), "n=1 []")
		}
	}
}

// An axis nobody answered is refused by name rather than guessed — and, since
// the bug was that the spelling did nothing quietly, refusing it has to be
// loud too.
func TestUnsetOfEveryElementRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := runUnsetArraySpan(t, UnsetArraySpanUnspecified,
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
