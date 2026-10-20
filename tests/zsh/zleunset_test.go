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
// `NUMERIC` carries it too — `NUMERIC=2; NUMERIC+=3` is 5 and `NUMERIC=x` is
// 0, measured. Between keystrokes the names are ordinary again: measured
// 2026-10-04 against zsh 5.9.2, `CURSOR=1+1 NUMERIC=2+2
// KEYS_QUEUED_COUNT=3+3` at the prompt after a widget has run keeps all three
// as text. This shell made the queue counter 6, its attribute outliving the
// call.
func TestTheCursorIsAnIntegerOnlyInsideAWidget(t *testing.T) {
	r, out := zleRunner(t, `w() { CURSOR=1; CURSOR+=2; a=$CURSOR; CURSOR=5; CURSOR=CURSOR-1; CURSOR+=x; NUMERIC=2; NUMERIC+=3; b=$NUMERIC; NUMERIC=x; print -r -- "c=$a e=$CURSOR n=$b x=$NUMERIC" }
zle -N w
`)
	if _, _, said := runWidget(t, r, out, "w", repl.Line{Buffer: "abcdefghij"}); said != "c=3 e=4 n=5 x=0\n" {
		t.Errorf("the widget said %q, want %q", said, "c=3 e=4 n=5 x=0\n")
	}
	got, _ := runZshVars(t, r, `CURSOR=1+1 NUMERIC=2+2 KEYS_QUEUED_COUNT=3+3; print -r -- "$CURSOR $NUMERIC $KEYS_QUEUED_COUNT"`)
	if want := "1+1 2+2 3+3\n"; got != want {
		t.Errorf("after the widget: %q, want %q", got, want)
	}
}

// TestAnUnsetLineParameterEmptiesWhatItStandsFor: in the widget that runs
// it, `unset` of a line parameter empties that part of the line, as well as
// taking the name away. Measured 2026-10-05 through a pseudo-terminal
// against zsh 5.9.2, on `abcd` with the cursor at 2 (#6038).
func TestAnUnsetLineParameterEmptiesWhatItStandsFor(t *testing.T) {
	r, out := zleRunner(t, `w() { unset $1; print -r -- "[${(P)+1}][$BUFFER][$CURSOR][$POSTDISPLAY][$CUTBUFFER]" }
for n in BUFFER LBUFFER RBUFFER CURSOR POSTDISPLAY CUTBUFFER; do
  eval "u$n() { POSTDISPLAY=pp; CUTBUFFER=kk; w $n }; zle -N u$n"
done
`)
	for _, c := range []struct {
		name, said, buffer string
		cursor             int
	}{
		{"BUFFER", "[0][][0][pp][kk]\n", "", 0},
		{"LBUFFER", "[0][cd][0][pp][kk]\n", "cd", 0},
		{"RBUFFER", "[0][ab][2][pp][kk]\n", "ab", 2},
		{"CURSOR", "[0][abcd][][pp][kk]\n", "abcd", 2},
		{"POSTDISPLAY", "[0][abcd][2][][kk]\n", "abcd", 2},
		{"CUTBUFFER", "[0][abcd][2][pp][]\n", "abcd", 2},
	} {
		line, _, said := runWidget(t, r, out, "u"+c.name, repl.Line{Buffer: "abcd", Cursor: 2})
		if said != c.said || line.Buffer != c.buffer || line.Cursor != c.cursor {
			t.Errorf("unset %s: said %q, line %q at %d; want %q, %q at %d",
				c.name, said, line.Buffer, line.Cursor, c.said, c.buffer, c.cursor)
		}
	}
	// And outside a widget the names are ordinary: an unset there is only
	// an unset.
	if got, _ := runZshVars(t, r, `LBUFFER=x; unset LBUFFER; print -r -- "[${LBUFFER-gone}]"`); got != "[gone]\n" {
		t.Errorf("outside a widget: %q", got)
	}
}
