// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// `backward-kill-line` kills back to the line's start and keeps the rest,
// on a real terminal, from a widget and from a key (#5960).
//
// Measured 2026-10-05 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file: ^Xk on `aa bb cc dd ee` at 7 draws
// `GOT[c dd ee] 0 [aa bb c]`, and typing `aa bb cc dd ee`, eight ^B and the
// rebound ^U, then ^Xs, draws `GOT[cc dd ee] 0 [aa bb ]`. With a count of -1
// from the widget it draws `GOT[aa bb c] 7 [c dd ee]`. Before the fix this
// shell took the whole line every time.
func TestBackwardKillLineKeepsWhatIsAheadOfTheCursor(t *testing.T) {
	control, screen := widgetSession(t, `bindkey '^U' backward-kill-line
k() { BUFFER='aa bb cc dd ee'; CURSOR=7; CUTBUFFER=; zle .backward-kill-line "$@"; show }
show() { BUFFER="GOT[$BUFFER] $CURSOR [$CUTBUFFER]" }
kn() { k -n -1 }
zle -N k; bindkey '^Xk' k; zle -N kn; bindkey '^Xn' kn
zle -N show; bindkey '^Xs' show
c() { BUFFER= CUTBUFFER= }; zle -N c; bindkey '^Xc' c
`)
	steps := []struct{ keys, want string }{
		{"\x18k", "GOT[c dd ee] 0 [aa bb c]"},
		{"\x18caa bb cc dd ee\x02\x02\x02\x02\x02\x02\x02\x02\x15\x18s", "GOT[cc dd ee] 0 [aa bb ]"},
		{"\x18c\x18n", "GOT[aa bb c] 7 [c dd ee]"},
		// An empty line for the session to exit from.
		{"\x18cprint -r -- END$((1+1))\r", "END2"},
	}
	for _, step := range steps {
		if _, err := control.WriteString(step.keys); err != nil {
			t.Fatalf("typing %q: %v", step.keys, err)
		}
		if err := screen.Await(step.want, widgetBudget); err != nil {
			t.Fatalf("after %q, want\n%s\n%v\n%q", step.keys, step.want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
}
