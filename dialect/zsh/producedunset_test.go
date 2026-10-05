// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/repl"
)

// TestAnUnsetRegionHighlightIsUnsetUntilAssigned: in the widget that runs
// it, `unset region_highlight` takes the name away, and an assignment after
// it brings it back with its type. Measured 2026-10-05 through a
// pseudo-terminal against zsh 5.9.2 (#5961). Before the fix this shell
// emptied the array but still counted it as set, and after the assignment
// `${(t)region_highlight}` was empty and `typeset -p` listed nothing.
func TestAnUnsetRegionHighlightIsUnsetUntilAssigned(t *testing.T) {
	r, out := zleRunner(t, `w() {
  region_highlight=('0 1 bold')
  unset region_highlight
  print -r -- "A ${+region_highlight} [${(t)region_highlight}] [$region_highlight] n=${#region_highlight}"
  region_highlight=('0 2 standout')
  print -r -- "B ${+region_highlight} [${(t)region_highlight}] [$region_highlight]"
  typeset -p region_highlight
}
zle -N w
`)
	_, _, said := runWidget(t, r, out, "w", repl.Line{Buffer: "abcd", Cursor: 4})
	const want = "A 0 [] [] n=0\n" +
		"B 1 [array-local-special] [0 2 standout]\n" +
		"typeset -a region_highlight=( '0 2 standout' )\n"
	if said != want {
		t.Errorf("the widget said\n%q\nwant\n%q", said, want)
	}
}

// TestAnUnsetArgvIsUnsetUntilAssigned: the same for the other produced array
// a script may write. Measured on zsh 5.9.2: `unset argv` leaves `${+argv}`
// and `$#` at 0 and no type, and `argv=(x y)` after it is `array-special`
// again with both elements.
func TestAnUnsetArgvIsUnsetUntilAssigned(t *testing.T) {
	got, _, errs := runZshUTF8(t, `set -- a b c; unset argv; print -r -- "${+argv} $# [${(t)argv}]"
argv=(x y); print -r -- "${+argv} $# [${(t)argv}] $*"`)
	if want := "0 0 []\n1 2 [array-special] x y\n"; got != want || errs != "" {
		t.Errorf("got %q (stderr %q), want %q", got, errs, want)
	}
}
