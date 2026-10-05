// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// send-break and recursive-edit, on a real terminal (#5913, #5899).
//
// Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file:
//
//	ab ^Xr cd Return       REC st=0 B=[abcd] K=^M END
//	ab ^Xr cd ^G           REC st=1 B=[abcd] K=^G END
//	ran=line ^G, then ^Xb  CHECK s=1 ran=none END
//	ran=line ^Xw, then ^Xb CHECK s=1 ran=none END
//
// The edit ends on Return without running the line and on `^G` at status 1,
// and the widget goes on; `^G` at the prompt gives the line up with `$?` 1;
// and `zle send-break` does the same and stops the widget that asked. Before
// the fix `recursive-edit` was no widget (status 1, the line untouched), `^G`
// did nothing, and `zle send-break` was refused and the widget ran on.
func TestSendBreakAndRecursiveEdit(t *testing.T) {
	control, screen := widgetSession(t, `ran=none
r() { zle recursive-edit; local st=$?; BUFFER="REC st=$st B=[$BUFFER] K=${(V)KEYS} END" }; zle -N r; bindkey '^Xr' r
w() { zle send-break; ran=widget }; zle -N w; bindkey '^Xw' w
b() { BUFFER="CHECK s=$? ran=$ran END" }; zle -N b; bindkey '^Xb' b
c() { BUFFER= }; zle -N c; bindkey '^Xc' c
precmd() { print -rn -- "PRE$((1+1))" }
`)
	await := func(want string) {
		t.Helper()
		if err := screen.Await(want, widgetBudget); err != nil {
			t.Fatalf("want %q: %v\n%q", want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
	send := func(keys string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
	}
	send("ab\x18rcd\r")
	await("REC st=0 B=[abcd] K=^M END")
	send("\x18cab\x18rcd\x07")
	await("REC st=1 B=[abcd] K=^G END")
	// A line given up ends with a fresh prompt, and the next keys wait for
	// it — for the precmd's mark and then the prompt after it — because keys
	// typed while the editor starts the next line could be read by the
	// terminal's line discipline instead.
	//
	// The key rings the bell and the widget's call does not, measured: zsh
	// writes `\a` after `^G` and nothing after `zle send-break`, and gives
	// the line up without drawing it again either way.
	for _, row := range []struct {
		breaker string
		bell    bool
	}{{"\x07", true}, {"\x18w", false}} {
		from := len(screen.Text())
		send("\x18cran=line" + row.breaker)
		await("PRE2")
		if got := strings.Contains(screen.Text()[from:], "\a"); got != row.bell {
			t.Errorf("after %q the bell rang %v, want %v\n%q", row.breaker, got, row.bell, screen.Text()[from:])
		}
		await(widgetMark)
		send("\x18b")
		await("CHECK s=1 ran=none END")
	}
	send("\x18c")
}
