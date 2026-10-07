// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A prompt refuses a token in the body of a substitution that is still open
// as the token is read, with the bare second sentence this shell writes for a
// body refused at its prompt, and the `)` typed after it is refused as a line
// of its own. Measured 2026-10-07 on bash 5.3.20 under `--norc -i`, at a
// terminal and on a pipe alike (#6319).
func TestAPromptRefusesASubstitutionBodyAtTheToken(t *testing.T) {
	for _, c := range []struct{ name, typed, sentence string }{
		{"a word that ends the read", "echo $(\nfi\n)\necho af''ter\n", "token `fi'"},
		{"a token the grammar refuses", "echo $(echo a\nif; then\n)\necho af''ter\n", "token `;'"},
		{"a process substitution", "cat <(\nfi\n)\necho af''ter\n", "token `fi'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, _ := prompt(t, c.typed, "bash", "--norc", "-i")
			want := "syntax error near unexpected " + c.sentence + " while looking for matching `)'\nbash: syntax error\n"
			if !strings.Contains(errs, want) {
				t.Errorf("stderr %q, want %q", errs, want)
			}
			if !strings.Contains(errs, "syntax error near unexpected token `)'\n") {
				t.Errorf("stderr %q, want the `)' refused on its own", errs)
			}
			if !strings.Contains(out, "after\n") {
				t.Errorf("stdout %q, want the line after it run", out)
			}
		})
	}
}
