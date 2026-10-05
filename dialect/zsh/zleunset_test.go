// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/blairham/sh/repl"
)

// TestAnUnsetInOneWidgetIsGoneByTheNext: each widget call is given its
// parameters afresh, so a widget that unsets them leaves the next call
// nothing missing. Measured 2026-10-04 through a pseudo-terminal against zsh
// 5.9.2: a widget running `unset` over the line parameters, then another
// reading each one's type, gets every type word back (#5917). This shell kept
// the removal for the rest of the session and the second widget read every
// one of them empty.
func TestAnUnsetInOneWidgetIsGoneByTheNext(t *testing.T) {
	r, out := zleRunner(t, `c() { unset POSTDISPLAY CUTBUFFER BUFFER LBUFFER RBUFFER CURSOR region_highlight NUMERIC; print -r -- "c ${+POSTDISPLAY}${+CUTBUFFER}${+BUFFER}${+LBUFFER}${+RBUFFER}${+CURSOR}" }
zle -N c
b() { print -r -- "b ${(t)POSTDISPLAY} ${(t)CUTBUFFER} ${(t)BUFFER} ${(t)LBUFFER} ${(t)RBUFFER} ${(t)CURSOR} ${(t)region_highlight} n=${NUMERIC-unset}" }
zle -N b
`)
	if _, _, said := runWidget(t, r, out, "c", repl.Line{Buffer: "ab", Cursor: 1}); said != "c 000000\n" {
		t.Errorf("the unsetting widget said %q, want %q", said, "c 000000\n")
	}
	n := 4
	_, _, said := runWidget(t, r, out, "b", repl.Line{Buffer: "ab", Cursor: 1, Numeric: &n})
	const want = "b scalar-local-special scalar-local-special scalar-local-special scalar-local-special scalar-local-special " +
		"integer-local-special array-local-special n=4\n"
	if said != want {
		t.Errorf("the next widget said %q, want %q", said, want)
	}
}

// TestAWidgetsCountReachesTheActionsItCalls: the count the line carries, or
// the one the widget assigned to `NUMERIC`, is what an action performed from
// the widget is given — and an `unset NUMERIC` is none (#5941).
func TestAWidgetsCountReachesTheActionsItCalls(t *testing.T) {
	r, out := zleRunner(t, `w() { zle forward-word; NUMERIC=2; zle forward-word; unset NUMERIC; zle forward-word }
zle -N w
`)
	ed := &stubEditor{}
	n := 3
	runWidgetWatching(t, r, out, "w", repl.Line{Buffer: "a b c d e", Numeric: &n}, ed)
	var got []string
	for _, l := range ed.lines {
		if l.Numeric == nil {
			got = append(got, "none")
		} else {
			got = append(got, strconv.Itoa(*l.Numeric))
		}
	}
	if want := []string{"3", "2", "none"}; !slices.Equal(got, want) {
		t.Errorf("the actions were given %v, want %v", got, want)
	}
}

// TestTheCursorIsAnIntegerOnlyInsideAWidget: `CURSOR+=2` adds, because the
// cursor carries the integer attribute while a widget runs (#5929), and
// between keystrokes the names are ordinary again — measured 2026-10-04
// against zsh 5.9.2, `CURSOR=1+1 NUMERIC=2+2 KEYS_QUEUED_COUNT=3+3` at the
// prompt after a widget has run keeps all three as text. This shell made
// the queue counter 6, its attribute outliving the call.
func TestTheCursorIsAnIntegerOnlyInsideAWidget(t *testing.T) {
	r, out := zleRunner(t, `w() { CURSOR=1; CURSOR+=2; a=$CURSOR; CURSOR=5; CURSOR=CURSOR-1; CURSOR+=x; print -r -- "c=$a e=$CURSOR" }
zle -N w
`)
	if _, _, said := runWidget(t, r, out, "w", repl.Line{Buffer: "abcdefghij"}); said != "c=3 e=4\n" {
		t.Errorf("the widget said %q, want %q", said, "c=3 e=4\n")
	}
	got, _ := runZshVars(t, r, `CURSOR=1+1 NUMERIC=2+2 KEYS_QUEUED_COUNT=3+3; print -r -- "$CURSOR $NUMERIC $KEYS_QUEUED_COUNT"`)
	if want := "1+1 2+2 3+3\n"; got != want {
		t.Errorf("after the widget: %q, want %q", got, want)
	}
}
