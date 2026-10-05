// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// `zle .self-insert` types what `$KEYS` says, on a real terminal.
//
// Three ways it did not: from a widget bound to a key sequence it typed the
// printable key before the sequence rather than the sequence's last character
// (#5926); with a count it typed once (the count half of #5941's table that
// self-insert had left over); and a typed character of more than one byte
// reached a self-insert widget with only its first byte in `$KEYS` (#5900).
//
// Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file in a UTF-8 locale, pressing ^Xm, ESC q, ^Xn and
// typing `é`:
//
//	SEQ [ambcdefgh] 2 END
//	SEQ [aqbcdefgh] 2 END
//	CNT 3=[annnb]4 -3=[annnb]1 0=[ab]1 END
//	MB n=1 b=2 END
//
// Before the fix this shell wrote `[abcdefgh] 1` for both sequences, `[ab]1`
// for every count, and `MB n=1 b=1`.
func TestSelfInsertTypesWhatKeysSays(t *testing.T) {
	control, screen := widgetSession(t, `LC_ALL=en_US.UTF-8
w() { BUFFER=abcdefgh; CURSOR=1; zle .self-insert; local said="[$BUFFER] $CURSOR"; BUFFER=; print -r -- "SEQ $said END" }
zle -N w; bindkey '^Xm' w; bindkey '\eq' w
n() {
  local k said
  for k in 3 -3 0; do BUFFER=ab; CURSOR=1; NUMERIC=$k; zle .self-insert; said+=" ${k}=[$BUFFER]$CURSOR"; done
  BUFFER=
  print -r -- "CNT$said END"
}
zle -N n; bindkey '^Xn' n
si() { [[ $KEYS == [[:ascii:]] ]] || print -r -- "MB n=${#KEYS} b=$(print -rn -- $KEYS | wc -c | tr -d ' ') END"; zle .self-insert }
zle -N self-insert si
`)
	// Each row is assembled by the shell: the brackets and the counts are
	// not in anything typed, so the terminal's echo cannot satisfy a wait.
	for _, step := range []struct{ keys, want string }{
		{"\x18m", "SEQ [ambcdefgh] 2 END"},
		{"\x1bq", "SEQ [aqbcdefgh] 2 END"},
		{"\x18n", "CNT 3=[annnb]4 -3=[annnb]1 0=[ab]1 END"},
		{"é", "MB n=1 b=2 END"},
	} {
		if _, err := control.WriteString(step.keys); err != nil {
			t.Fatalf("typing %q: %v", step.keys, err)
		}
		if err := screen.Await(step.want, widgetBudget); err != nil {
			t.Fatalf("after %q, want\n%s\n%v\n%q", step.keys, step.want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
	// The `é` is still on the line, and the session's own `exit` follows.
	if _, err := control.WriteString("\x7f"); err != nil {
		t.Fatalf("erasing: %v", err)
	}
}
