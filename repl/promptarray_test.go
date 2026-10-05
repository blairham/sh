// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// `%1v` drawn in a prompt is an element of the array the script holds, asked
// of the interpreter as `print -P` asks it. Measured 2026-10-05 through a pty
// against zsh 5.9.2: `psvar=(9 8); PS1='Q%1vQ%-v> '` draws `Q9Q8> `; this
// drawer drew `QQ> ` (#5965).
func TestAPromptArrayElementIsDrawn(t *testing.T) {
	r := newTestRunner(nil)
	r.SetArray("psvar", []string{"9", "8"})
	s := Shell{Runner: r, Style: PromptStyle{
		Escape: '%', NumericArgument: true,
		Codes: map[rune]PromptField{'v': FieldPromptArrayElement},
	}}
	if got := s.render(`Q%1vQ%-v>`); got != "Q9Q8>" {
		t.Errorf("drew %q, want %q", got, "Q9Q8>")
	}
}
