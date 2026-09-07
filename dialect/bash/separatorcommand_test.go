// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// A `;` written where a command belongs is a syntax error here, in every
// position, and this shell names the `;` it found. Measured 2026-09-07, bash
// 5.3.15 and bash 3.2.57 alike, from a script file under `env -i`:
//
//	$ bash -n s.sh          # true ; ; echo two
//	s.sh: line 1: syntax error near unexpected token `;'
//
// It is the half of the measurement that keeps this out of the core: only
// ksh93 and zsh take any of it, so the common denominator is the refusal.
func TestNoSeparatorWhereACommandBelongs(t *testing.T) {
	d := bash.Dialect()
	if d.SeparatorWhereACommandBelongs != syntax.NoSeparatorWhereACommandBelongs {
		t.Error("bash refuses a `;` where a command belongs")
	}
	if d.AbsentAndOrOperandIsAnEmptyCommand {
		t.Error("bash puts no command where an and-or's operand is missing")
	}
	for _, src := range []string{
		"; echo two\n",
		"true ; ; echo two\n",
		"true & ; echo two\n",
		"true ; ;\n",
		"true || ; echo two\n",
		"true && ; echo two\n",
		"echo one | ; cat\n",
		"false || ;\n",
	} {
		_, err := syntax.Parse(src, d)
		if err == nil {
			t.Errorf("%q parsed, want a refusal", src)
			continue
		}
		const want = "syntax error near unexpected token `;'"
		if got := bash.Diagnostics().ParseFailure(err); got != want {
			t.Errorf("%q:\n got %q\nwant %q", src, got, want)
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
		_, err := syntax.Parse(src, bash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", src)
			continue
		}
		if got := bash.Diagnostics().ParseFailure(err); got != "syntax error near unexpected token `;'" {
			t.Errorf("%q:\n got %q\nwant %q", src, got, "syntax error near unexpected token `;'")
		}
	}
}
