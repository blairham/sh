// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// Output that never ended its line, and where the next prompt goes.
//
// Nothing here names a shell. What is asserted is what each of the three
// answers does; which shell gives which is in dialect/zsh and dialect/bash,
// next to the measurements.

// promptedAfter draws one prompt with the given answers and hands back what
// went to the terminal before the prompt itself.
func promptedAfter(t *testing.T, mark string, returns, clears bool, cols int) string {
	t.Helper()
	var out strings.Builder
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing("\n"), &out
	e.unfinishedMark, e.returnsFirst, e.clearsBelow = mark, returns, clears
	if cols > 0 {
		e.width = func() int { return cols }
	}
	if _, err := e.readLine(drawPrompt("P> ")); err != nil {
		t.Fatal(err)
	}
	before, _, _ := strings.Cut(out.String(), "P> ")
	return before
}

const inverse = "\x1b[1m\x1b[7m%\x1b[27m\x1b[1m\x1b[0m"

func TestWhatIsWrittenBeforeAPrompt(t *testing.T) {
	for _, c := range []struct {
		name            string
		mark            string
		returns, clears bool
		cols            int
		want            string
	}{
		{
			// All three: the mark, the row filled so the terminal wraps, the
			// return, and the erase.
			"a mark, a return and an erase",
			inverse, true, true, 80,
			inverse + strings.Repeat(" ", 79) + "\r \r\r\x1b[J",
		},
		{
			// The marking turned off leaves the return and the erase.
			"no mark",
			"", true, true, 80, "\r\x1b[J",
		},
		{
			// The return turned off takes the marking with it, because the
			// mark is written *at* the cursor and stepped off by the wrap —
			// there is nothing to step off to without the return. Measured
			// in the shell that has both.
			"no return takes the mark with it",
			inverse, false, true, 80, "\x1b[J",
		},
		{
			// A dialect that does none of it writes nothing at all, which is
			// what the other shell with an editor does.
			"none of it",
			inverse, false, false, 80, "",
		},
		{
			// Without a width the row cannot be filled, so the terminal will
			// not wrap and a mark would be painted over by the prompt rather
			// than marking anything. The return and the erase still happen.
			"no width, no mark",
			inverse, true, true, 0, "\r\x1b[J",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := promptedAfter(t, c.mark, c.returns, c.clears, c.cols); got != c.want {
				t.Errorf("wrote %q before the prompt, want %q", got, c.want)
			}
		})
	}
}

// The padding is one short of the width, and that is the whole trick.
//
// From column 0 the mark takes column 0 and the padding fills to the
// right-hand edge, where a terminal has not wrapped yet — it wraps lazily,
// when there is another character to put — so the `\r` returns to the *same*
// row and the space paints the mark out. From any other column the padding
// overflows, the terminal does wrap, and the `\r` lands on the row below with
// the mark left standing above it.
//
// One sequence, two outcomes, and the terminal decides which. A padding of
// `cols` would wrap even from column 0 and leave a mark on every prompt; a
// padding of `cols-2` would not wrap from column 1 and the prompt would be
// drawn over the output it had just marked.
func TestThePaddingIsOneShortOfTheWidth(t *testing.T) {
	got := promptedAfter(t, inverse, true, false, 40)
	want := inverse + strings.Repeat(" ", 39) + "\r \r\r"
	if got != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
	if n := strings.Count(got, " ") - 1; n != 39 {
		t.Errorf("padded %d columns, want %d — one short of the width", n, 39)
	}
}
