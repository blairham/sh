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
// The result is drawn into the line rather than printed. A widget that prints
// hands the terminal back to its line discipline for the length of the call
// (see repl's runHandler), so a key sent the moment printed output appears can
// land in cooked mode and be echoed and edited by the kernel instead of read —
// which is how this test first failed on Linux. What the editor draws, it
// draws after the widget has returned and the terminal is raw again.
func TestSelfInsertTypesWhatKeysSays(t *testing.T) {
	control, screen := widgetSession(t, `LC_ALL=en_US.UTF-8
w() { BUFFER=abcdefgh; CURSOR=1; zle .self-insert; BUFFER="SEQ [$BUFFER] $CURSOR END" }
zle -N w; bindkey '^Xm' w; bindkey '\eq' w
n() {
  local k said
  for k in 3 -3 0; do BUFFER=ab; CURSOR=1; NUMERIC=$k; zle .self-insert; said+=" ${k}=[$BUFFER]$CURSOR"; done
  BUFFER="CNT$said END"
}
zle -N n; bindkey '^Xn' n
si() { [[ $KEYS == [[:ascii:]] ]] || mb="MB n=${#KEYS} b=$(print -rn -- $KEYS | wc -c | tr -d ' ') END"; zle .self-insert }
zle -N self-insert si
r() { BUFFER=$mb }; zle -N r; bindkey '^Xr' r
c() { BUFFER= }; zle -N c; bindkey '^Xc' c
`)
	// Each row is assembled by the shell: the brackets and the counts are
	// not in anything typed, so the terminal's echo cannot satisfy a wait.
	for _, step := range []struct{ keys, want string }{
		{"\x18m", "SEQ [ambcdefgh] 2 END"},
		{"\x1bq", "SEQ [aqbcdefgh] 2 END"},
		{"\x18n", "CNT 3=[annnb]4 -3=[annnb]1 0=[ab]1 END"},
		{"é\x18r", "MB n=1 b=2 END"},
	} {
		if _, err := control.WriteString("\x18c" + step.keys); err != nil {
			t.Fatalf("typing %q: %v", step.keys, err)
		}
		if err := screen.Await(step.want, widgetBudget); err != nil {
			t.Fatalf("after %q, want\n%s\n%v\n%q", step.keys, step.want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
	// An empty line for the session's own `exit`.
	if _, err := control.WriteString("\x18c"); err != nil {
		t.Fatalf("clearing: %v", err)
	}
}
