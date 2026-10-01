// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// **A `coproc` after a bar is a parse error at the word** (#5273). Measured
// 2026-10-01 over `-c` on zsh 5.9.2, which writes these lines byte for byte;
// the coprocess started here before. See syntax.Dialect.CoprocAfterABar.
func TestACoprocAfterABarIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo | coproc true; echo st=$?", "zsh:1: parse error near `coproc'\n"},
		{"echo |\ncoproc cat; echo st=$?", "zsh:2: parse error near `coproc'\n"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Fatalf("%q parsed, want a refusal", tc.src)
		}
		if got := zsh.Diagnostics().ParseDiagnostic("zsh", "-c", err, tc.src); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
	if _, err := syntax.Parse("coproc true | cat", zsh.Dialect()); err != nil {
		t.Errorf("a leading coproc: %v, want it to parse", err)
	}
}
