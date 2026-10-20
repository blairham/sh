// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A subscript the **reference** supplied is not the script's, and a name that
// is no array says nothing about it.
//
// `unset "x[2]"` over a scalar is `x: not an array variable` at 1 — that was
// already right, and stays. An `unset` through a reference aimed at `x[2]` is
// silent at 0, because the subscript is the reference's own and the script
// never asked about an element.
//
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh` over
// script files, against bash 5.3.20 and bash 5.3.15, which agree, with `x=42`:
//
//	unset "x[2]"                  `x: not an array variable`, 1
//	typeset -n foo='x[2]'
//	  unset foo                   silent, 0, and `x` still `42`
//
// The control that says it is the **route** and not the subscript is element
// zero: `unset "x[0]"` and an `unset` through a reference aimed at `x[0]` both
// take the whole scalar away, in both shells. So the two spellings agree
// wherever a scalar has the element and part only where it does not.
//
// This is `nameref4.sub`'s row, and it stood unreproduced for most of the
// campaign because every spelling of it written by hand used the direct
// subscript — the one that does refuse (#4178).
func TestAnUnsetThroughAReferenceDoesNotRefuseAScalar(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row.
			"the reference's subscript is silent over a scalar",
			"typeset -n foo='x[2]'\nx=42\nunset foo\nprintf '<%d>' $?",
			"<0>", "not an array variable",
		},
		{
			"and the scalar is untouched",
			"typeset -n foo='x[2]'\nx=42\nunset foo\ndeclare -p x",
			`declare -- x="42"`, "",
		},
		{
			// The control on the other side: the script's own subscript
			// still refuses.
			"the script's own subscript still refuses",
			"x=42\nunset 'x[2]'\nprintf '<%d>' $?",
			"x: not an array variable", "<0>",
		},
		{
			// Element zero agrees either way, which is what says the route
			// and not the subscript is the discriminator.
			"element zero takes the scalar away through a reference",
			"typeset -n foo='x[0]'\nx=42\nunset foo\ndeclare -p x",
			"x: not found", "",
		},
		{
			"and takes it away written directly too",
			"x=42\nunset 'x[0]'\ndeclare -p x",
			"x: not found", "",
		},
		{
			// A real array is unaffected by any of this.
			"a real array loses just the element",
			"typeset -n foo='x[2]'\nx=(a b c)\nunset foo\ndeclare -p x",
			`declare -a x=([0]="a" [1]="b")`, "not an array variable",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
			if tc.absent != "" && strings.Contains(out, tc.absent) {
				t.Errorf("= %q, want it not to contain %q", out, tc.absent)
			}
		})
	}
}
