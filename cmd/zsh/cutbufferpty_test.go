// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A kill a widget assigns to `$CUTBUFFER` is what the yank *key* inserts
// afterwards: the round trip back into the editor, not only the parameter
// (#5916). Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2:
// a widget doing `CUTBUFFER=hello`, then `^Y`, puts `hello` in the line.
//
// The line upper-cases what it is given, so the mark — `HELLO` — is in the
// output and never in anything typed or echoed: the yanked text is echoed in
// lower case, and the startup file spells it in two halves.
func TestAKillAWidgetAssignedIsWhatTheYankKeyInserts(t *testing.T) {
	control, screen := widgetSession(t,
		`setcut() { CUTBUFFER=hel${:-lo} }`,
		`zle -N setcut`,
		`bindkey '^G' setcut`,
	)
	for _, keys := range []string{"print -r -- ${(U):-", "\a", "\x19", "}\n"} {
		if _, err := control.WriteString(keys); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
	}
	if err := screen.Await("HELLO", widgetBudget); err != nil {
		t.Fatalf("the yank did not insert the kill the widget set: %v\n%s", err,
			smoke.Readable(smoke.LastLines(screen.Text(), 8)))
	}
}
