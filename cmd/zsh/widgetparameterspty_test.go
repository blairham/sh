// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A widget writes to the parameters it is given, and what it wrote reaches
// the widgets and actions it calls, on a real terminal.
//
// Three issues meet here: `CURSOR+=2` appended a digit, because the cursor
// did not carry the integer attribute (#5929); `NUMERIC=2` did nothing, so no
// count could be handed on (#5941); and an `unset` of one of these names
// lasted the rest of the session, so the next widget found it gone (#5917).
//
// Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file, pressing ^T, ^Y, then ESC 2 ^O:
//
//	PONE c=3 e=4 n=2 t=integer-local-special w2=2 fw=4 w2=unset ENDONE
//	PTHREE t=scalar-local-special n=2 fw=4 ENDTHREE
//
// Before the fix this shell wrote `c=10 e=5 n= t= w2=unset fw=2` on the first
// row and `t= n=unset fw=2` on the second: the earlier `unset NUMERIC` had
// taken the typed count away too, and `zle forward-word` was never given one.
func TestAWidgetsParameterWritesReachWhatItCalls(t *testing.T) {
	control, screen := widgetSession(t, `w2() { said+=" w2=${NUMERIC-unset}" }
zle -N w2
p1() {
  local said
  BUFFER=abcdefghij; CURSOR=1; CURSOR+=2; said="c=$CURSOR"
  CURSOR=5; CURSOR=CURSOR-1; said+=" e=$CURSOR"
  NUMERIC=2; said+=" n=$NUMERIC t=${(t)NUMERIC}"; zle w2
  BUFFER='a b c d'; CURSOR=0; zle forward-word; said+=" fw=$CURSOR"
  unset NUMERIC; zle w2
  BUFFER=
  print -r -- "PONE $said ENDONE"
}
zle -N p1; bindkey '^T' p1
p2() { unset POSTDISPLAY }
zle -N p2; bindkey '^Y' p2
p3() {
  BUFFER='a b c d e'; CURSOR=0; zle forward-word
  local said="t=${(t)POSTDISPLAY} n=${NUMERIC-unset} fw=$CURSOR"
  BUFFER=
  print -r -- "PTHREE $said ENDTHREE"
}
zle -N p3; bindkey '^O' p3
`)
	// Every row is the shell's own output — the keys typed are control keys
	// and echo nothing — so neither can be satisfied by the terminal.
	if _, err := control.WriteString("\x14"); err != nil {
		t.Fatalf("pressing ^T: %v", err)
	}
	const one = "PONE c=3 e=4 n=2 t=integer-local-special w2=2 fw=4 w2=unset ENDONE"
	if err := screen.Await(one, widgetBudget); err != nil {
		t.Fatalf("the widget's writes did not land as\n%s\n%v\n%q", one, err, smoke.LastLines(screen.Text(), 6))
	}
	if _, err := control.WriteString("\x19\x1b2\x0f"); err != nil {
		t.Fatalf("pressing ^Y, ESC 2 ^O: %v", err)
	}
	const three = "PTHREE t=scalar-local-special n=2 fw=4 ENDTHREE"
	if err := screen.Await(three, widgetBudget); err != nil {
		t.Fatalf("an earlier widget's unset outlived it, or the typed count went missing\n%s\n%v\n%q", three, err,
			smoke.LastLines(screen.Text(), 6))
	}
}
