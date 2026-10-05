// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A keymap a startup file makes with `bindkey -N` and selects with `bindkey
// -A … main` is the one the editor reads keys in, on a real terminal (#5969).
//
// Measured 2026-10-05 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file: `abc` and ^Xw draws `GOT[abc|3|main]`; `abcd`,
// Escape, `0`, `iZ` and ^Xw draws `GOT[Zabcd|1|main]`, because the copy of
// viins is vi editing; and `xyz`, Escape and `q` draws `GOT[xyz|2|vicmd]`.
// Before the fix `bindkey -N` was refused, main stayed emacs, and ^Xw was
// bound nowhere.
func TestAMadeKeymapIsTheOneTheEditorReadsIn(t *testing.T) {
	control, screen := widgetSession(t, `w() { BUFFER="GOT[$BUFFER|$CURSOR|$KEYMAP]" }
zle -N w
c() { BUFFER= }; zle -N c
bindkey -N myvi viins
bindkey -A myvi main
bindkey -M myvi '^Xw' w '^Xc' c
bindkey -M vicmd 'q' w '^Xc' c
`)
	steps := []struct{ keys, want string }{
		{"abc\x18w", "GOT[abc|3|main]"},
		// Escape alone, and then the backspace this editor draws on entering
		// command mode before the next key: a key already waiting behind an
		// Escape makes it the start of a sequence instead (see
		// repl.editor.escapeIsTheModeSwitch).
		{"\x18cabcd", "abcd"},
		{"\x1b", "\x08"},
		{"0", ""},
		{"iZ\x18w", "GOT[Zabcd|1|main]"},
		{"\x18cxyz", "xyz"},
		{"\x1b", "\x08"},
		{"q", "GOT[xyz|2|vicmd]"},
		{"\x18ci", ""},
		{"print -r -- END$((1+1))\r", "END2"},
	}
	for _, step := range steps {
		if _, err := control.WriteString(step.keys); err != nil {
			t.Fatalf("typing %q: %v", step.keys, err)
		}
		if step.want == "" {
			continue
		}
		if err := screen.Await(step.want, widgetBudget); err != nil {
			t.Fatalf("after %q, want\n%s\n%v\n%q", step.keys, step.want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
}
