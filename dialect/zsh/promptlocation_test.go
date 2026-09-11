// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A parse failure at a prompt names no line here, which is the same answer
// this shell gives a program on standard input.
//
// Measured 2026-09-11, `printf 'echo one\nif; then\necho three\n'` into `-i`:
// `zsh: parse error near `\n'`, where the same three lines in a file are
// `s.sh:3: parse error near `\n'`.
func TestAParseFailureAtAPromptNamesNoLine(t *testing.T) {
	_, err := syntax.Parse("if; then\necho three\n", zsh.Dialect())
	if err == nil {
		t.Fatal("the input parsed, want a refusal")
	}
	if got, want := zsh.Diagnostics().ForPrompt().ParseDiagnostic("zsh", "", err, ""), "zsh: parse error near `\\n'\n"; got != want {
		t.Errorf("at a prompt:\n got %q\nwant %q", got, want)
	}
	// The general answer still carries the line, tightly joined.
	if got, want := zsh.Diagnostics().ParseDiagnostic("zsh", "", err, ""), "zsh:3: parse error near `\\n'\n"; got != want {
		t.Errorf("elsewhere:\n got %q\nwant %q", got, want)
	}
}
