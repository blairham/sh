// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A `{name}` redirection after a compound command is a parse error in this
// shell, naming the brace after a group-like compound and the whole word
// after one closed by a reserved word (#6507). Measured 2026-10-15 against
// zsh 5.9.2 at `/opt/homebrew/bin/zsh` under `-fc`; the table is on
// syntax.Dialect.FdVariableRefusedAfterACompound.
func TestAnFdVariableAfterACompoundIsAParseErrorHere(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"{ :; } {v}</dev/null", "{"},
		{"case x in x) ;; esac {v}</dev/null", "{"},
		{"function f { :; } {v}</dev/null", "{"},
		{"if :; then :; fi {v}</dev/null", "{v}"},
		{"foreach i (1) : ; end {v}</dev/null", "{v}"},
		{"for i (1) { : } {v}</dev/null", "{v}"},
	} {
		_, err := syntax.Parse(c.src, zsh.Dialect())
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Errorf("%q: err %v, want a parse error", c.src, err)
			continue
		}
		if se.Token != c.want {
			t.Errorf("%q: blamed %q, want %q", c.src, se.Token, c.want)
		}
	}
	// The controls: a numbered redirection behind the same compound, and a
	// `{name}` on a simple command, are both read.
	for _, src := range []string{"{ :; } 3</dev/null", "true {v}</dev/null"} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
