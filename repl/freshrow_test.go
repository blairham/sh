// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// Output that never ended its line, and where the next prompt goes.
//
// Nothing here names a shell. What is asserted is what each of the answers
// does; which shell gives which is in dialect/zsh and dialect/bash, next to
// the measurements.

const (
	inverse = "\x1b[1m\x1b[7m%\x1b[27m\x1b[1m\x1b[0m"
	// What one dialect writes on the ground the prompt goes on.
	clearing = "\x1b[0m\x1b[27m\x1b[24m\x1b[J"
)

// answers is one dialect's say in this, as the editor carries it.
type answers struct {
	mark    string
	returns bool
	clears  bool
	cols    int
}

// prompted plays out a whole prompt in the order the session does it — the
// mark for the previous command's output, then whatever the prompt hooks
// printed, then the ground and the prompt — and hands back everything written
// before the prompt itself.
//
// The hook's output is a parameter because where it falls is the point: it
// goes between the two halves, and a shell that wrote them as one sequence
// would mark the hook's line instead of the command's.
func prompted(t *testing.T, a answers, hook string) string {
	t.Helper()
	var out strings.Builder
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing("\n"), &out
	e.unfinishedMark, e.returnsFirst = a.mark, a.returns
	if a.clears {
		e.clearBefore = clearing
	}
	if a.cols > 0 {
		e.width = func() int { return a.cols }
	}
	e.markUnfinished()
	out.WriteString(hook)
	if _, err := e.readLine(drawPrompt("P> ")); err != nil {
		t.Fatal(err)
	}
	before, _, _ := strings.Cut(out.String(), "P> ")
	return before
}

// marking is the whole sequence for a mark of the given width in cols columns.
func marking(mark string, cols int) string {
	w := displayWidth(mark)
	return mark + strings.Repeat(" ", cols-w) + "\r" + strings.Repeat(" ", w) + "\r"
}

func TestWhatIsWrittenBeforeAPrompt(t *testing.T) {
	for _, c := range []struct {
		name string
		a    answers
		want string
	}{
		{
			// All of it: the mark, the row filled so the terminal wraps, the
			// return, and the erase.
			"a mark, a return and an erase",
			answers{inverse, true, true, 80},
			marking(inverse, 80) + "\r" + clearing,
		},
		{
			// The marking turned off leaves the return and the erase — and no
			// padding either, which is measured rather than assumed: the
			// shell that marks writes nothing but the return with the marking
			// option off.
			"no mark",
			answers{"", true, true, 80},
			"\r" + clearing,
		},
		{
			// The return turned off takes the marking with it, because the
			// mark is written *at* the cursor and stepped off by the wrap —
			// there is nothing to step off to without the return. Measured in
			// the shell that has both.
			"no return takes the mark with it",
			answers{inverse, false, true, 80},
			clearing,
		},
		{
			// A dialect that does none of it writes nothing at all, which is
			// what the other shell with an editor does.
			"none of it",
			answers{inverse, false, false, 80},
			"",
		},
		{
			// Without a width the row cannot be filled, so the terminal will
			// not wrap and a mark would be painted over by the prompt rather
			// than marking anything. The return and the erase still happen.
			"no width, no mark",
			answers{inverse, true, true, 0},
			"\r" + clearing,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := prompted(t, c.a, ""); got != c.want {
				t.Errorf("wrote %q before the prompt, want %q", got, c.want)
			}
		})
	}
}

// The mark goes before what the prompt hooks print, and only the return and
// the erase go after.
//
// Measured 2026-09-12 through a pseudo-terminal, with a `precmd` that prints
// without a newline and a command that does the same:
//
//	CMD  mark <79 spaces> \r <space> \r  HOOK  \r  <erase>  P>
//
// With no hook the two groups are adjacent and read as one sequence, which is
// how this was first written down. The hook is what tells them apart — and it
// is the case the whole thing exists for, since the progress line a plugin
// manager prints while it loads is exactly such a hook.
//
// A shell that wrote the sequence as one block after the hooks would mark the
// *hook's* half-written line and leave the command's output with the prompt
// drawn against it, which is the bug with an extra step.
func TestTheMarkGoesBeforeWhatTheHooksPrint(t *testing.T) {
	const cols = 80
	a := answers{inverse, true, true, cols}
	got := prompted(t, a, "HOOK")
	want := marking(inverse, cols) + "HOOK" + "\r" + clearing
	if got != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
	// Said the other way around, so that a rearrangement that kept every byte
	// still fails: the mark is on the far side of the hook's output from the
	// return that follows it.
	markAt := strings.Index(got, inverse)
	hookAt := strings.Index(got, "HOOK")
	if markAt < 0 || hookAt < 0 || markAt > hookAt {
		t.Errorf("the mark is not before the hook's output: %q", got)
	}
	if strings.Index(got, clearing) < hookAt {
		t.Errorf("the erase is not after the hook's output: %q", got)
	}
}

// A continuation prompt marks nothing, and the return still happens.
//
// The newline the terminal echoed when the line was accepted has already put
// the cursor at column 0, so there is nothing part-way along to mark. The
// session decides this by not calling for a mark at all, which is what is
// asserted here: the ground under the prompt is the same either way.
func TestAContinuationPromptIsOnlyGround(t *testing.T) {
	var out strings.Builder
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing("\n"), &out
	e.unfinishedMark, e.returnsFirst, e.clearBefore = inverse, true, clearing
	e.width = func() int { return 80 }
	// No markUnfinished, which is what a continuation prompt gets.
	if _, err := e.readLine(drawPrompt("C> ")); err != nil {
		t.Fatal(err)
	}
	before, _, _ := strings.Cut(out.String(), "C> ")
	if want := "\r" + clearing; before != want {
		t.Errorf("before a continuation prompt wrote %q, want %q", before, want)
	}
}

// The padding is the width of the screen less the width of the mark, and that
// is the whole trick.
//
// From column 0 the mark takes its own columns, the padding fills the rest to
// the right-hand edge, and a terminal has not wrapped yet — it wraps lazily,
// when there is another character to put — so the `\r` returns to the *same*
// row and the spaces paint the mark out. From any other column the padding
// overflows, the terminal does wrap, and the `\r` lands on the row below with
// the mark left standing above it.
//
// One sequence, two outcomes, and the terminal decides which. A padding of
// `cols` would wrap even from column 0 and leave a mark on every prompt; one
// column short of that and the prompt would be drawn over the output it had
// just marked.
func TestThePaddingIsTheScreenLessTheMark(t *testing.T) {
	got := prompted(t, answers{inverse, true, false, 40}, "")
	want := inverse + strings.Repeat(" ", 39) + "\r \r" + "\r"
	if got != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}

// And that it is the mark's width and not one column, measured a width at a
// time against the shell that marks by giving it a longer mark. In eighty
// columns: `%` pads 79 and paints out 1, `abc` pads 77 and paints out 3.
//
// The default mark is one column wide, so this is the difference between a
// rule and a coincidence — and it is the mark's width *on the screen*, which
// is not its length in bytes: the default one is six escape sequences around a
// single `%`.
func TestThePaddingFollowsTheMarksWidth(t *testing.T) {
	for _, c := range []struct {
		mark  string
		cells int
	}{
		{inverse, 1},
		{"%", 1},
		{"abc", 3},
		{"\x1b[7mabc\x1b[0m", 3},
	} {
		if n := displayWidth(c.mark); n != c.cells {
			t.Errorf("mark %q is %d columns, want %d", c.mark, n, c.cells)
			continue
		}
		got := prompted(t, answers{c.mark, true, false, 80}, "")
		want := c.mark + strings.Repeat(" ", 80-c.cells) +
			"\r" + strings.Repeat(" ", c.cells) + "\r" + "\r"
		if got != want {
			t.Errorf("mark %q: wrote %q, want %q", c.mark, got, want)
		}
	}
}
