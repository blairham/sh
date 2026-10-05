// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/repl"
)

// TestAFatalErrorInAWidgetGivesTheLineUp: the widget comes back Broken with
// status 1, and the error is taken back, so the next widget's body runs
// (#5959). A completion widget keeps its line — measured 2026-10-04 against
// zsh 5.9.2, `cf() { BUFFER=zz }` behind `zle -C` refuses the assignment and
// the line stays — but its error is taken back all the same.
func TestAFatalErrorInAWidgetGivesTheLineUp(t *testing.T) {
	r, out := zleRunner(t, `d() { : ${nosuch?gone}; print -r -- unreached }
zle -N d
cf() { BUFFER=zz; print -r -- unreached }
zle -C cw complete-word cf
b() { print -r -- "b ran" }
zle -N b
`)
	for _, row := range []struct {
		widget string
		broken bool
		status int
	}{
		{"d", true, 1},
		// The refused call's own status, which a `zle cw` from another
		// widget answers (#5939).
		{"cw", false, 1},
	} {
		line, ok, said := runWidget(t, r, out, row.widget, repl.Line{Buffer: "ab", Cursor: 2})
		if !ok || line.Broken != row.broken || line.Status != row.status {
			t.Errorf("%s: ok=%v broken=%v status=%d, want broken=%v status=%d (said %q)",
				row.widget, ok, line.Broken, line.Status, row.broken, row.status, said)
		}
		if _, _, said := runWidget(t, r, out, "b", repl.Line{}); said != "b ran\n" {
			t.Errorf("after %s the next widget said %q, want it to run", row.widget, said)
		}
	}
}
