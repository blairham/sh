// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which `}` closes which `{`, in the column that reads the braces a value
// holds.
//
// The short rule — a brace pairs only with one of its own provenance — is
// right about half of it and too strong about the other half. A `}` the
// script **wrote** closes either kind; a `}` an expansion **produced** closes
// only what an expansion opened. See braceClassOf for the panel.
//
// Through the same field counter every row in this area uses, because `echo`
// joins its arguments with a blank and `[a] [b]` would print as `{a,b}`'s own
// text does.
func TestWhichClosingBraceClosesAProducedOne(t *testing.T) {
	for _, tc := range []struct{ name, src, reads, text string }{
		// The control: a group the script wrote, both braces written, which
		// the two readings answer alike.
		{"a written pair", `f {a,b}`, `2 | [a] [b]`, `2 | [a] [b]`},

		// The half the short rule had. A produced `}` closes nothing the
		// script wrote, and both readings leave the word alone.
		{
			"a produced closer closes no written opener", `e='}'; f {a,b$e`,
			`1 | [{a,b}]`, `1 | [{a,b}]`,
		},

		// The half it did not. A written `}` closes what a value opened.
		{
			"a written closer closes a produced opener", `e='{'; f ${e}a,b}`,
			`2 | [a] [b]`, `1 | [{a,b}]`,
		},
		{
			"and it is the first one, not the last", `e='{'; f ${e}a,b}c}`,
			`2 | [ac}] [bc}]`, `1 | [{a,b}c}]`,
		},
		{
			"with text on both sides of the group", `e='{'; f x${e}a,b}y`,
			`2 | [xay] [xby]`, `1 | [x{a,b}y]`,
		},
		{
			"the opener at the end of a longer value", `e='q{'; f ${e}a,b}`,
			`2 | [qa] [qb]`, `1 | [q{a,b}]`,
		},
		{
			"the opener out of a command substitution", `f $(printf '%s' '{')a,b}`,
			`2 | [a] [b]`, `1 | [{a,b}]`,
		},
		{
			"a produced closer still closes a produced opener",
			`e='{'; c='}'; f ${e}a,b${c}`, `2 | [a] [b]`, `1 | [{a,b}]`,
		},
		{
			"and a produced comma in between is still a separator",
			`e='{'; c=','; f ${e}a${c}b}`, `2 | [a] [b]`, `1 | [{a,b}]`,
		},

		// Depth is counted over every unquoted brace, whatever produced it,
		// which is what holds the two halves apart from "a written `}` closes
		// anything". Each of these three fails a different way under a scan
		// that counted one class alone.
		{
			"a written opener in between is counted", `e='{'; f $e{a,b}`,
			`1 | [{{a,b}]`, `2 | [{a] [{b]`,
		},
		{
			"and so is a second produced one", `e='{{'; f ${e}a,b}`,
			`1 | [{{a,b}]`, `1 | [{{a,b}]`,
		},
		{
			"a nested written pair does not close the outer group",
			`e='{'; f ${e}a,{b,c}d}`, `3 | [a] [bd] [cd]`, `2 | [{a,bd}] [{a,cd}]`,
		},
		{
			"a produced opener deepens a written group", `e='{'; f {a,${e}b}`,
			`1 | [{a,{b}]`, `2 | [a] [{b]`,
		},
		{
			"and the written `}` behind it closes", `e='{'; f {a,${e}b}c}`,
			`2 | [a] [{b}c]`, `2 | [ac}] [{bc}]`,
		},
		{
			"a produced closer spends no depth in a written group",
			`e='}'; f {a,b${e}c}`, `2 | [a] [b}c]`, `2 | [a] [b}c]`,
		},
		{
			"nor in that group's body, where a comma is behind it",
			`e='}'; f {a,b${e},c}`, `3 | [a] [b}] [c]`, `3 | [a] [b}] [c]`,
		},

		// The body is the same question one level down: a produced `{` in it
		// nests, and a produced `}` in it closes only behind a produced
		// opener.
		{
			"a produced opener in the body hides the commas behind it",
			`e='{{'; f ${e}a,b}}`, `1 | [{{a,b}}]`, `1 | [{{a,b}}]`,
		},
		{
			"a produced pair in a produced group's body closes",
			`e='{'; g='{}'; f ${e}a,${g}b,c}`, `3 | [a] [{}b] [c]`, `1 | [{a,{}b,c}]`,
		},
		{
			"a written pair in a produced group's body closes",
			`e='{'; f ${e}a,{}b,c}`, `3 | [a] [{}b] [c]`, `1 | [{a,{}b,c}]`,
		},
		{
			"a written list in a produced group's body still expands",
			`e='{'; f ${e}a,{x,y}b,c}`, `4 | [a] [xb] [yb] [c]`,
			`2 | [{a,xb,c}] [{a,yb,c}]`,
		},

		// Quoting is what says the pair is about provenance and not about
		// bytes: neither a quoted nor an escaped brace is one at all, on
		// either side and from either source.
		{
			"a quoted written closer closes nothing", `e='{'; f ${e}a,b"}"`,
			`1 | [{a,b}]`, `1 | [{a,b}]`,
		},
		{
			"nor an escaped one", `e='{'; f ${e}a,b\}`,
			`1 | [{a,b}]`, `1 | [{a,b}]`,
		},
		{
			"nor a quoted produced one", `e='{'; c='}'; f ${e}a,b"$c"`,
			`1 | [{a,b}]`, `1 | [{a,b}]`,
		},
		{
			"and a quoted produced opener opens nothing", `e='{'; f "${e}"a,b}`,
			`1 | [{a,b}]`, `1 | [{a,b}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := braceProduced(t, tc.src, Yes); out != tc.reads || st != 0 {
				t.Errorf("reads: %s = %q status %d, want %q", tc.src, out, st, tc.reads)
			}
			if out, st := braceProduced(t, tc.src, No); out != tc.text || st != 0 {
				t.Errorf("text: %s = %q status %d, want %q", tc.src, out, st, tc.text)
			}
		})
	}
}
