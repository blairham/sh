// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A parse failure at a prompt names no line here, where the same failure in a
// script names one — and this shell is the one that carries the line *inside*
// its sentence, so the route has to replace the sentence rather than the
// location.
//
// Measured 2026-09-11 under `-i` with PS1 and PS2 set, against the same input
// in a file, the shell's own path normalized:
//
//	input             at a prompt                             in a file
//	if; then          syntax error: `;' unexpected             syntax error at line 1: …
//	if true; then     syntax error: `then' unmatched           syntax error at line 2: …
//	x='never closed   syntax error: `'' unmatched              syntax error at line 2: …
//	v=$(echo hi       syntax error: `(' unmatched              syntax error at line 1: …
//	echo one &&       syntax error: `end of file' unexpected   syntax error at line 2: …
func TestAParseFailureAtAPromptNamesNoLine(t *testing.T) {
	for _, tc := range []struct{ src, prompt string }{
		{"if true; then\n", "syntax error: `then' unmatched"},
		{"x='never closed\n", "syntax error: `'' unmatched"},
		{"v=$(echo hi\n", "syntax error: `(' unmatched"},
		{"echo one &&\n", "syntax error: `end of file' unexpected"},
		{"echo $((1+\n", "syntax error: `(' unmatched"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ForPrompt().ParseFailure(err); got != tc.prompt {
			t.Errorf("%q at a prompt:\n got %q\nwant %q", tc.src, got, tc.prompt)
		}
		// And the script sentence is the same one with the line in it, which
		// is the rule the pair is built from.
		script := ksh.Diagnostics().ParseFailure(err)
		if script == tc.prompt {
			t.Errorf("%q: the script sentence is the prompt's, so no line is named anywhere", tc.src)
		}
	}
}

// The location goes with it: name only, whatever line the construct reached.
func TestAPromptLocationIsTheNameAlone(t *testing.T) {
	d := ksh.Diagnostics().ForPrompt()
	for _, line := range []int{1, 2, 7} {
		if got, want := d.Report("ksh", line, "m"), "ksh: m"; got != want {
			t.Errorf("line %d: %q, want %q", line, got, want)
		}
	}
}
