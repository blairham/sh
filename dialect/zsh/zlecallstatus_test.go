// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// widgetsReturning is three widgets whose functions end three ways, and the
// table of what `zle NAME` answers for each, measured 2026-10-04 through a
// pseudo-terminal against zsh 5.9.2 (#5939): the function's own status,
// with a `false` run just before a call that ends 0 not leaking through.
const widgetsReturning = `w2() { return 3 }; w3() { false }; w4() { true }
zle -N w2; zle -N w3; zle -N w4
`

const widgetsReturningSaid = "w2=3 w3=1 w4=0 after-false=0\n"

// TestCallingAWidgetAnswersWhatItsFunctionReturned: from inside another
// widget, `zle w` is the status w's function returned. It was always 0, so a
// widget built from others — `zle backward-word || …` — read a move that
// did not happen as one that did.
func TestCallingAWidgetAnswersWhatItsFunctionReturned(t *testing.T) {
	r, out := zleRunner(t, widgetsReturning+
		`k() { zle w2; a=$?; zle w3; b=$?; zle w4; c=$?; false; zle w4; print -r -- "w2=$a w3=$b w4=$c after-false=$?" }
zle -N k
`)
	_, ok, said := runWidget(t, r, out, "k", repl.Line{})
	if !ok || said != widgetsReturningSaid {
		t.Errorf("ok=%v said %q, want %q", ok, said, widgetsReturningSaid)
	}
}

// TestAHandlersWidgetCallAnswersWhatItsFunctionReturned is the same table
// from a plain `zle -F` handler, which reaches the widget by a different
// path — runWidgetFunction rather than a nested call — and was 0 there too.
func TestAHandlersWidgetCallAnswersWhatItsFunctionReturned(t *testing.T) {
	shellFd, systemFd, write, inherited := pipeAt(t)
	if _, err := write.WriteString("x\n"); err != nil {
		t.Fatal(err)
	}
	src := widgetsReturning +
		`h() { IFS= read -u $1 -r _; zle w2; a=$?; zle w3; b=$?; zle w4; c=$?; false; zle w4; print -r -- "w2=$a w3=$b w4=$c after-false=$?" }
zle -F ` + strconv.Itoa(shellFd) + " h"
	r, out := watchRunnerWith(t, src, func(rr *interp.Runner) { rr.InheritedFiles = inherited })
	zsh.DescriptorReady(r, context.Background(), systemFd, repl.Line{Buffer: "b", Cursor: 1})
	if got := out.String(); got != widgetsReturningSaid {
		t.Errorf("handler said %q, want %q", got, widgetsReturningSaid)
	}
}
