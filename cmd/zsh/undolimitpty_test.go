// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// `UNDO_LIMIT_NO` stops a key's undo at the change a widget named, and is
// the line's and not the session's, on a real terminal (#5898).
//
// Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file: typing `ab c`, ^Xl, `d e`, five ^_ and ^Xs draws
// `SHOW[ab c] t=integer-local-special set=1 END` — the undo walks back to the
// line as it stood when the limit was set and no further. On the next line
// ^Xs draws `SHOW[] t=integer-local-special set=0 END`, and at the prompt
// `${UNDO_LIMIT_NO-unset}` is `unset`, though ^Xl assigned it without
// `local`. Before the fix this shell undid to `ab`, read the name as a
// plain `scalar` that kept its value on the next line, and left ^Xl's
// assignment behind as a global.
func TestTheUndoLimitStopsAnUndoAndBelongsToTheLine(t *testing.T) {
	control, screen := widgetSession(t, `lim() { UNDO_LIMIT_NO=$UNDO_CHANGE_NO }
zle -N lim; bindkey '^Xl' lim
show() { BUFFER="SHOW[$BUFFER] t=${(t)UNDO_LIMIT_NO} set=$(( UNDO_LIMIT_NO > 0 )) END" }
zle -N show; bindkey '^Xs' show
c() { BUFFER= }; zle -N c; bindkey '^Xc' c
`)
	// A step's keys go only once the one before has been seen through, and
	// an empty line is a step of its own so that the keys after it wait for
	// the prompt it ends with: keys typed while a line runs can be read by
	// the terminal's line discipline rather than by the editor.
	steps := []struct{ keys, want string }{
		{"ab c\x18ld e\x1f\x1f\x1f\x1f\x1f\x18s", "SHOW[ab c] t=integer-local-special set=1 END"},
		{"\x18cprint -r -- NEXT$((1+1))\r", "NEXT2"},
		{"", widgetMark},
		{"\x18s", "SHOW[] t=integer-local-special set=0 END"},
		{"\x18cprint -r -- \"G=${UNDO_LIMIT_NO-unset}\"\r", "G=unset"},
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
