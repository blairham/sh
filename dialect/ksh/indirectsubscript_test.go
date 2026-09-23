// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// The column that answers `${!x}` with a **name** carries a written subscript
// onto the name it answers with.
//
// `${!foo}` over `typeset -n foo=bar` is `bar` here as everywhere, and
// `${!foo[2]}` is `bar[2]`: the reference is followed and the subscript rides
// along. This shell answered `bar` to both, so the subscript was dropped — the
// same loss the indirection-taking column showed, in the one other column that
// spells a reference at all.
//
// Measured 2026-09-23 against ksh93u+ 2012-08-01, `env -i` from a script file:
//
//	typeset -n foo=bar; typeset -a bar=(x y z)
//	  ${!foo}                      bar
//	  ${!foo[2]}                   bar[2]        was bar
//	typeset -A bar=([k]=v)
//	  ${!foo[k]}                   bar[k]        was bar
//	typeset -a a=(x y z)
//	  ${!a[0]}                     a[0]          already right
//
// **What is deliberately not claimed here**: where the target is a scalar, or
// was never declared, this shell answers `foo[2]` rather than `bar[2]` — and
// `${!foo[k]}` over an undeclared target is a bare `bar`. So whether the
// subscript rides depends on the target being a compound, and the undeclared
// row does not fit even that. Those are a second question with their own
// panel, in a column no suite here grades; the rows below are the ones where
// the answer is not in doubt (#4178).
func TestAnIndirectionYieldingANameKeepsItsSubscript(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an indexed target takes the subscript",
			"typeset -n foo=bar\ntypeset -a bar=(x y z)\nprintf '[%s]' \"${!foo[2]}\"",
			"[bar[2]]",
		},
		{
			"a table target takes the subscript",
			"typeset -n foo=bar\ntypeset -A bar=([k]=v)\nprintf '[%s]' \"${!foo[k]}\"",
			"[bar[k]]",
		},
		{
			// The control: no subscript, and the answer is the target's name
			// exactly as before.
			"the bare spelling is unchanged",
			"typeset -n foo=bar\ntypeset -a bar=(x y z)\nprintf '[%s]' \"${!foo}\"",
			"[bar]",
		},
		{
			// And a name that is not a reference keeps answering itself,
			// which this change must not have moved.
			"a plain name answers itself with its subscript",
			"typeset -a a=(x y z)\nprintf '[%s]' \"${!a[0]}\"",
			"[a[0]]",
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
