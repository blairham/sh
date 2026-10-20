// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
)

// The prompt-escape language travels with the dialect, not only with the front
// end.
//
// #1455, measured through a pty with `PS1='[\u][\h]END '` inherited and `USER`
// unset: `cmd/bash` drew `[bhamilton][Blairs-MacBook-Pro-5]END` and
// `cmd/sh -dialect bash` drew `[\u][\h]END` — the same dialect, reached two
// ways, drawing two different prompts, because the table was supplied by the
// binary and this dialect's Apply did not install it.
//
// Two halves of one escape, and only one of them was here: Apply asked
// interp.LoginName and interp.MachineName who the user and the machine are —
// the *answers* to `\u` and `\h` — and never said which letters ask the
// question. A runner told the answers and not the table draws the letters.
func TestApplyInstallsThePromptEscapeTable(t *testing.T) {
	preset.PromptTableInstalled(t, dialecttest.Base{Dir: t.TempDir()}, bash.PromptStyle())
}

// And the table is not the empty one, which would let the comparison above
// pass for the wrong reason: two shells that both have no prompt language
// agree perfectly.
func TestThisDialectHasAPromptEscapeLanguage(t *testing.T) {
	st := bash.PromptStyle()
	if st.Escape != '\\' {
		t.Errorf("Escape = %q, want a backslash: bash's prompt codes are written `\\u`, `\\h`, `\\w`", st.Escape)
	}
	for _, code := range []rune{'u', 'h', 'w', 'W', 's', '$'} {
		if _, ok := st.Codes[code]; !ok {
			t.Errorf("no row for %q: a prompt written `\\u@\\h:\\w\\$ ` reaches all of these", code)
		}
	}
}
