// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// A parse failure at a prompt names no line here, where the same failure in a
// script is `s.sh: line 1: …`.
//
// Measured 2026-09-11, `printf 'echo one\nif; then\n'` into `-i`:
// `bash: syntax error near unexpected token `;'`, against
// `s.sh: line 1: syntax error near unexpected token `;'` for the same two
// lines in a file.
//
// The line *inside* a sentence stays, which is the discriminating half and
// the reason this is the location rather than a second wording: an unfinished
// `if` at a prompt is `bash: syntax error: unexpected end of file from `if'
// command on line 1` there too.
func TestAParseFailureAtAPromptNamesNoLine(t *testing.T) {
	for _, tc := range []struct{ src, prompt string }{
		{"if; then\n", "bash: syntax error near unexpected token `;'\n"},
		{"if true; then\n", "bash: syntax error: unexpected end of file from `if' command on line 1\n"},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := bash.Diagnostics().ForPrompt().ParseDiagnostic("bash", "", err, ""); got != tc.prompt {
			t.Errorf("%q at a prompt:\n got %q\nwant %q", tc.src, got, tc.prompt)
		}
	}
	// The script route still names the line, which is what says the prompt
	// answer is the route's and not a wording that lost it.
	_, err := syntax.Parse("if; then\n", bash.Dialect())
	if got := bash.Diagnostics().ForScript().ParseDiagnostic("s.sh", "", err, "if; then\n"); got == "" ||
		got == bash.Diagnostics().ForPrompt().ParseDiagnostic("s.sh", "", err, "") {
		t.Errorf("a script says %q, which is the prompt's answer", got)
	}
}
