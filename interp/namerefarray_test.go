// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// NamerefArrayRefusal is the *shape* of a refusal both shells with the `n`
// letter give: `typeset -n r=v` over a name carrying an array. The refusal
// itself is unanimous and in the same words, so it is the core's; when the
// check runs, and what counts as an array, are the axis.
//
// The two halves co-vary across the only two columns that answer — bash asks
// it last and takes the attribute, ksh93 asks it first and wants the contents
// — which is why they are one field. Measured 2026-09-16, and every
// assertion here was checked against both real shells (#3103).
func namerefArraySemantics(shape NamerefArrayRefusal) Semantics {
	sem := PosixSemantics()
	// The letter, and the two answers the refusals beside it already need:
	// a self reference refused rather than warned about, and a refusal that
	// reports and carries on so the `echo` behind it still runs.
	sem.DeclareOptions = "aAfFgilnprux"
	sem.UnsetOptions = "vfn"
	sem.NamerefCycleIsRefused = Yes
	sem.BadNameToDeclarationFatal = No
	// Three axes the probes walk past on their way to this one, answered so
	// that what comes back is this field's answer and not another field's
	// complaint: a plain word declared over a container, `$a` reading the
	// whole of one, and whether a declaration over a container may be
	// refused for its type.
	sem.ScalarOverACompoundIsAnInconsistentType = No
	sem.ArrayScalarIsTheWholeArray = No
	sem.NamerefArrayRefusal = shape
	return sem
}

func TestWhereTheArrayRefusalSitsAmongTheOtherTwo(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		last      string
		first     string
	}{
		{
			// A bad target beside an array: the late answer reports the
			// target, the early one reports the array.
			name:  "a target that is not a name",
			src:   `r=(a b); typeset -n r=1bad; echo "st=$?"`,
			last:  "invalid variable name",
			first: "cannot be an array",
		},
		{
			// And a self reference beside one, the same way round.
			name:  "a reference to itself",
			src:   `r=(a b); typeset -n r=r; echo "st=$?"`,
			last:  "self reference",
			first: "cannot be an array",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := namerefArraySemantics(NamerefArrayCheckedLastOnTheAttribute)
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if !strings.Contains(out, tc.last) || strings.Contains(out, tc.first) {
				t.Errorf("checked last: got %q, want %q and not %q", out, tc.last, tc.first)
			}
			early := namerefArraySemantics(NamerefArrayCheckedFirstOnTheContents)
			out, _ = run(t, tc.src, func(r *Runner) { r.Semantics = &early })
			if !strings.Contains(out, tc.first) || strings.Contains(out, tc.last) {
				t.Errorf("checked first: got %q, want %q and not %q", out, tc.first, tc.last)
			}
		})
	}
}

// The other half: what counts as an array. A bare `typeset -a r` holding
// nothing is one under the attribute reading and is not under the contents
// reading, which is the whole of the difference between refusing this line
// and running it.
func TestWhatCountsAsAnArrayUnderTheTwoReadings(t *testing.T) {
	const src = `typeset -a r; v=VEE; typeset -n r=v; echo "st=$? [${r-GONE}]"`

	sem := namerefArraySemantics(NamerefArrayCheckedLastOnTheAttribute)
	out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
	if !strings.Contains(out, "cannot be an array") || !strings.Contains(out, "st=1 [GONE]") {
		t.Errorf("the attribute reading got %q, want the refusal and the array left standing", out)
	}

	early := namerefArraySemantics(NamerefArrayCheckedFirstOnTheContents)
	out, _ = run(t, src, func(r *Runner) { r.Semantics = &early })
	if strings.Contains(out, "cannot be an array") || out != "st=0 [VEE]\n" {
		t.Errorf("the contents reading got %q, want the reference made and reading through", out)
	}

	// One element in it and the two readings agree again, which is what
	// says the axis is about the empty attribute rather than about arrays.
	const filled = `typeset -a r; r[0]=x; v=VEE; typeset -n r=v; echo "st=$?"`
	for _, shape := range []NamerefArrayRefusal{
		NamerefArrayCheckedLastOnTheAttribute, NamerefArrayCheckedFirstOnTheContents,
	} {
		s := namerefArraySemantics(shape)
		out, _ := run(t, filled, func(r *Runner) { r.Semantics = &s })
		if !strings.Contains(out, "cannot be an array") {
			t.Errorf("%v over a filled array got %q, want the refusal", shape, out)
		}
	}

	// And the associative attribute makes an object on its own, so both
	// readings refuse that one too.
	const assoc = `typeset -A r; v=VEE; typeset -n r=v; echo "st=$?"`
	for _, shape := range []NamerefArrayRefusal{
		NamerefArrayCheckedLastOnTheAttribute, NamerefArrayCheckedFirstOnTheContents,
	} {
		s := namerefArraySemantics(shape)
		out, _ := run(t, assoc, func(r *Runner) { r.Semantics = &s })
		if !strings.Contains(out, "cannot be an array") {
			t.Errorf("%v over an associative attribute got %q, want the refusal", shape, out)
		}
	}
}

// The valueless `typeset -n r` asks no axis: both shells refuse it on the
// attribute alone, so the answer is the same under either shape — and the
// unanswered field is never reached by it.
func TestTheValuelessFormRefusesTheAttributeUnderEitherShape(t *testing.T) {
	const src = `typeset -a r; typeset -n r; echo "st=$?"`
	for _, shape := range []NamerefArrayRefusal{
		NamerefArrayRefusalUnspecified,
		NamerefArrayCheckedLastOnTheAttribute,
		NamerefArrayCheckedFirstOnTheContents,
	} {
		sem := namerefArraySemantics(shape)
		out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
		if !strings.Contains(out, "cannot be an array") || strings.Contains(out, "disagree here") {
			t.Errorf("%v got %q, want the refusal and no axis asked", shape, out)
		}
	}
}

// An unanswered axis is refused by name, and only a declaration that really
// lands on an array reaches it: a reference over a scalar, or over a name
// that holds nothing at all, never asks.
func TestAnUnansweredArrayRefusalShapeIsRefused(t *testing.T) {
	sem := namerefArraySemantics(NamerefArrayRefusalUnspecified)
	for _, src := range []string{
		`v=VEE; typeset -n r=v; echo "[$r]"`,
		`r=SCALAR; v=VEE; typeset -n r=v; echo "[$r]"`,
	} {
		out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
		if out != "[VEE]\n" {
			t.Errorf("%s got %q, want the axis never to have been asked", src, out)
		}
	}
	out, _ := run(t, `r=(a b); v=VEE; typeset -n r=v`, func(r *Runner) { r.Semantics = &sem })
	if !strings.Contains(out, "a name reference over a name holding an array") {
		t.Errorf("got %q, want the unanswered axis named", out)
	}
}

// A refused declaration leaves the name **exactly as found**, which is the
// constraint #3105 put on the discard: the array is still there, still counts
// its elements, and still reads its first one — and `unset -n` finds no
// reference to take off.
func TestARefusedArrayDeclarationDisturbsNothing(t *testing.T) {
	sem := namerefArraySemantics(NamerefArrayCheckedLastOnTheAttribute)
	const src = `r=(a b c); v=VEE; typeset -n r=v; unset -n r; echo "[${r-GONE}] n=${#r[@]} one=${r[1]}"`
	out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
	if !strings.Contains(out, "[a] n=3 one=b") {
		t.Errorf("got %q, want the array untouched by the refusal", out)
	}
}

// And where the contents reading *takes* the declaration, the attribute goes
// with the value rather than surviving under the reference: ksh93's own
// listing writes no `-a` afterwards, and `unset -n` opens an empty cell.
func TestATakenDeclarationDiscardsTheBareAttribute(t *testing.T) {
	sem := namerefArraySemantics(NamerefArrayCheckedFirstOnTheContents)
	const src = `typeset -a r; v=VEE; typeset -n r=v; unset -n r; echo "[${r-GONE}] n=${#r[@]}"`
	out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
	if out != "[GONE] n=0\n" {
		t.Errorf("got %q, want the attribute discarded with the value", out)
	}
}
