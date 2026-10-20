// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A **subscript written inside `${!…}` is not swallowed by a reference.**
//
// `${!ref}` answers the name the reference points at — that is the whole of
// what the spelling means for a reference, and it is the same answer in both
// shells that have one. `${!ref[2]}` is not that spelling: it is an ordinary
// indirection of `ref[2]`, and the reference is followed for that read like
// any other. This shell took the first answer for both, so the subscript was
// dropped on the floor and the target's *name* came back where the element's
// value should have been indirected through.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over script files, against bash 5.3.20, with `declare -n foo=bar`:
//
//	                            bash 5.3.20                     before
//	bar=(x y z); z=ZZ
//	  ${!foo}                   bar                             bar
//	  ${!foo[2]}                ZZ                              bar
//	  ${!foo[0]}                (empty — `x` is not set)        bar
//	bar never declared
//	  ${!foo[2]}                foo[2]: invalid indirect …      bar
//	declare -a bar
//	  ${!foo[2]}                (empty — declared, so silent)   bar
//
// The last two are one rule: what decides the refusal is whether the cell the
// reference **points at** was ever declared, not whether the reference itself
// was. Asking the reference's own name found a declaration that was never the
// one in question — `foo` is always declared, that being what makes it a
// reference — so the refusal could not be reached at all (#4178).
func TestASubscriptInsideAnIndirectionSurvivesTheReference(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The row `nameref11.sub` grades, in the shape the file reaches
			// it: the reference is aimed at a name nothing declared.
			"an undeclared target is refused, named as written",
			"declare -n foo=bar\necho \"[${!foo[2]}]\"",
			"foo[2]: invalid indirect expansion",
		},
		{
			// And the indirection is really taken: element 2 holds `z`, and
			// `z` holds ZZ.
			"the element's value is what is indirected through",
			"declare -n foo=bar\nbar=(x y z)\nz=ZZ\nprintf '[%s]' \"${!foo[2]}\"",
			"[ZZ]",
		},
		{
			// The control that says the bare spelling is untouched: no
			// subscript, and the answer is still the target's name.
			"the bare spelling still answers the target's name",
			"declare -n foo=bar\nbar=(x y z)\nprintf '[%s]' \"${!foo}\"",
			"[bar]",
		},
		{
			// An element holding a name nothing set is silent and empty,
			// which is the ordinary indirection's answer rather than a
			// refusal.
			"an element naming an unset variable is empty and silent",
			"declare -n foo=bar\nbar=(x y z)\nprintf '[%s]' \"${!foo[0]}\"\nprintf '<%d>' $?",
			"[]<0>",
		},
		{
			// A target that **was** declared is never refused, however empty
			// it is. This is the control for the question moving from the
			// reference to the cell it points at.
			"a declared but empty target is silent",
			"declare -n foo=bar\ndeclare -a bar\nprintf '[%s]' \"${!foo[2]}\"\nprintf '<%d>' $?",
			"[]<0>",
		},
		{
			// A reference aimed at an **element**, with a subscript written
			// over the top of it, is refused however well declared the array
			// is — so the declaration is asked after the target's text whole
			// rather than the cell it lives in. Reducing it to `bar` answers
			// "declared" and goes silent, which is what a first draft did.
			"a reference aimed at an element refuses a second subscript",
			"declare -a bar=(p q r)\ndeclare -n foo='bar[1]'\nprintf '[%s]' \"${!foo[2]}\"",
			"foo[2]: invalid indirect expansion",
		},
		{
			// And the bare spelling over that same reference still answers
			// the whole target text, element and all.
			"and the bare spelling answers the whole target",
			"declare -a bar=(p q r)\ndeclare -n foo='bar[1]'\nprintf '[%s]' \"${!foo}\"",
			"[bar[1]]",
		},
		{
			// A plain name with a subscript never went through a reference
			// and is here so the change is known not to have moved it.
			"a plain name with a subscript is unchanged",
			"bar=(x y z)\nz=ZZ\nprintf '[%s]' \"${!bar[2]}\"",
			"[ZZ]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
		})
	}
}
