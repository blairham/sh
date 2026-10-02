// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/repl"
)

// TestZleCallWithWSetsTheCalledWidgetsName: `zle inner -w` runs inner with
// `$WIDGET` its own name for the call, where a plain `zle inner` keeps the
// caller's; `-N` and `-Nw` are read as options, and a `--` after them ends
// the list. Measured 2026-10-02 through a pseudo-terminal against zsh 5.9.2
// (#5398) — and it is how add-zle-hook-widget's dispatcher calls each widget.
func TestZleCallWithWSetsTheCalledWidgetsName(t *testing.T) {
	r, out := zleRunner(t, `inner() { print -r -- "W=$WIDGET n=$# [$*]" }; zle -N inner
outer() { zle inner -w; print -r -- "after W=$WIDGET"; zle inner -Nw -- a b; zle inner -N x; zle inner -wN; zle inner }; zle -N outer`)
	_, ok, said := runWidget(t, r, out, "outer", repl.Line{})
	want := "W=inner n=0 []\nafter W=outer\nW=inner n=2 [a b]\nW=outer n=1 [x]\nW=inner n=0 []\nW=outer n=0 []\n"
	if !ok || said != want {
		t.Errorf("ok=%v said %q, want %q", ok, said, want)
	}
}
