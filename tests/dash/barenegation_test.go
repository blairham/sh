// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// This shell needs a pipeline after its `!`, wherever the `!` stands.
// Measured 2026-09-12; bash 3.2 answers the same way, so it is not one shell's
// eccentricity but the older reading (#948).
func TestABareNegationIsRefusedHere(t *testing.T) {
	d := dash.Dialect()
	if d.BareNegationReach != syntax.NoBareNegation {
		t.Errorf("reach = %v, want NoBareNegation", d.BareNegationReach)
	}
	if d.RepeatedNegationToggles {
		t.Error("this shell refuses a second `!`")
	}
	for _, tc := range []struct{ src, want string }{
		{"!", "Syntax error: end of file unexpected"},
		{"!\n", "Syntax error: newline unexpected"},
		{"! ; echo x\n", `Syntax error: ";" unexpected`},
		{"! & echo x\n", `Syntax error: "&" unexpected`},
		{"( ! )\n", `Syntax error: ")" unexpected`},
		{"! | cat\n", `Syntax error: "|" unexpected`},
		// A second `!` is named as the reserved word it is, rather than as
		// "word", which is this shell's own class distinction.
		{"! ! true\n", `Syntax error: "!" unexpected`},
	} {
		_, err := syntax.Parse(tc.src, d)
		if err == nil {
			t.Errorf("%q parsed, want a syntax error", tc.src)
			continue
		}
		if got := dash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
