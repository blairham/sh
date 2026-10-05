// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// Assigning `HISTNO` in a widget moves the history walk to that line, on a
// real terminal (#6050).
//
// Measured 2026-10-05 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file, after `: one` and `: two` and with `ab` typed: ^Xa
// (`HISTNO=1`) and then ^Xs draws `GOT[: one|5|1|integer-local-special]`;
// after one more command, ^Xb (`HISTNO=9`) from `ab` leaves
// `GOT[ab|2|4|integer-local-special]`; and a widget calling another sees the
// same number, `N[4]`; and `zle up-history` in a widget leaves `U[3]`.
// Before the fix the assignment was refused as read-only, and `$HISTNO`
// after `zle up-history` still said 4.
func TestAssigningHistnoMovesTheWalk(t *testing.T) {
	control, screen := widgetSession(t, `ha() { HISTNO=1 }; zle -N ha; bindkey '^Xa' ha
hb() { HISTNO=9 }; zle -N hb; bindkey '^Xb' hb
show() { BUFFER="GOT[$BUFFER|$CURSOR|$HISTNO|${(t)HISTNO}]" }
zle -N show; bindkey '^Xs' show
inner() { LBUFFER="N[$HISTNO]" }; zle -N inner
outer() { zle inner }; zle -N outer; bindkey '^Xn' outer
up() { zle up-history; LBUFFER="U[$HISTNO]" }; zle -N up; bindkey '^Xu' up
c() { BUFFER= }; zle -N c; bindkey '^Xc' c
`)
	steps := []struct{ keys, want string }{
		{": one\r", widgetMark},
		{": two\r", widgetMark},
		{"ab\x18a\x18s", "GOT[: one|5|1|integer-local-special]"},
		{"\x18cprint -r -- E$((1+1))\r", "E2"},
		{"", widgetMark},
		{"ab\x18b\x18s", "GOT[ab|2|4|integer-local-special]"},
		{"\x18c\x18n", "N[4]"},
		{"\x18c\x18u", "U[3]"},
		{"\x18cprint -r -- F$((1+2))\r", "F3"},
	}
	for _, step := range steps {
		if step.keys != "" {
			if _, err := control.WriteString(step.keys); err != nil {
				t.Fatalf("typing %q: %v", step.keys, err)
			}
		}
		if err := screen.Await(step.want, widgetBudget); err != nil {
			t.Fatalf("after %q, want\n%s\n%v\n%q", step.keys, step.want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
}
