// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// A `;` written where a command belongs is a syntax error here, in every
// position. Measured 2026-09-07 from a script file under `env -i`:
//
//	$ dash -n s.sh          # true ; ; echo two
//	s.sh: 1: Syntax error: ";" unexpected
//
// The whole rendered line, location included, because a blame that stayed on
// the operator's line would report `1` for a `;` on line 2.
func TestNoSeparatorWhereACommandBelongs(t *testing.T) {
	d := dash.Dialect()
	if d.SeparatorWhereACommandBelongs != syntax.NoSeparatorWhereACommandBelongs {
		t.Error("dash refuses a `;` where a command belongs")
	}
	if d.AbsentAndOrOperandIsAnEmptyCommand {
		t.Error("dash puts no command where an and-or's operand is missing")
	}
	for _, tc := range []struct{ src, want string }{
		{"; echo two\n", "s.sh: 1: Syntax error: \";\" unexpected\n"},
		{"true ; ; echo two\n", "s.sh: 1: Syntax error: \";\" unexpected\n"},
		{"true & ; echo two\n", "s.sh: 1: Syntax error: \";\" unexpected\n"},
		{"true || ; echo two\n", "s.sh: 1: Syntax error: \";\" unexpected\n"},
		{"echo one | ; cat\n", "s.sh: 1: Syntax error: \";\" unexpected\n"},
		{"echo one |\n; cat\n", "s.sh: 2: Syntax error: \";\" unexpected\n"},
	} {
		_, err := syntax.Parse(tc.src, d)
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		dg := dash.Diagnostics().ForScript()
		if got := dg.ParseDiagnostic("s.sh", "", err, tc.src); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// And at the head of a compound body, which is its own position in the
// grammar and refused here like every other.
func TestASeparatorAtTheHeadOfACompoundBodyIsRefused(t *testing.T) {
	for _, src := range []string{
		"{ ; echo two; }\n",
		"( ; echo two )\n",
		"if :; then ; echo two; fi\n",
		"while :; do ; echo two; break; done\n",
		"case x in x) ; echo two;; esac\n",
	} {
		_, err := syntax.Parse(src, dash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", src)
			continue
		}
		if got := dash.Diagnostics().ForScript().ParseDiagnostic("s.sh", "", err, src); got != "s.sh: 1: Syntax error: \";\" unexpected\n" {
			t.Errorf("%q:\n got %q\nwant %q", src, got, "s.sh: 1: Syntax error: \";\" unexpected\n")
		}
	}
}
