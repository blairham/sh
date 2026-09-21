// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package promptfidelity compares the prompt this shell draws with the
// prompt the program it was imported from draws.
//
// docs/spec/prompt-theme.md asks for exactly this and says why:
//
//	"It looks the same" is an opinion until something can fail. So fidelity
//	is defined as a comparison against the real program, and it needs an
//	instrument in the family of `make oracle`, `make acp` and `make
//	sandbox`: drive the real prompt, drive ours, compare.
//
// It is the one claim in that document a person can check by looking, and
// until this existed it was asserted and not shown.
//
// # The four rules it inherits, each from a mistake already made here
//
// **Compare a cell grid, not a byte string.** Two SGR spellings paint the
// same screen, so a byte diff fails on prompts that are identical to look
// at. internal/cellgrid resolves the parameters and the comparison is over
// the grapheme, the foreground, the background and the attributes.
//
// **Never strip ANSI.** A harness comparing plain text cannot tell a working
// theme from a colorless one, which is the blind spot that has hidden broken
// rendering in this tree before. Nothing here discards appearance, and the
// grid's own tests prove a colorless render differs in every colored cell.
//
// **Drive it through a pty with a multi-row prompt.** A one-row prompt
// cannot catch a frame that is correct in its pieces and wrong in its
// nesting, and a shell declines to draw a prompt at all without a terminal.
//
// **Pin the context on both sides or the diff is noise.** Working directory,
// exit status, command duration, job count, terminal width — every one is an
// input to some segment.
//
// # The clock, and everything else that moves
//
// One input cannot be pinned across two processes: the clock. So instead of
// a special case for it there is a general one, and it is the discipline
// internal/suite already applies to a reference shell — **each side is
// rendered twice and a cell that differs between a side's own two renders is
// unstable.** Unstable cells are left out of the comparison and *counted in
// the report*, because a row that agreed by excluding half its cells is a
// row that says nothing.
//
// The renders are **interleaved** — ours, theirs, ours, theirs — and that
// ordering is what makes the rule work rather than a detail of it. See
// Compare.
//
// That covers the clock, a duration measured in the shell, a pid, and
// whatever else a configuration reaches for. It is also the only mechanism
// here that could quietly weaken a row, which is why the count is printed on
// every run rather than only when it is nonzero.
package promptfidelity

import (
	"fmt"
	"strings"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
)

// Context is what both sides are pinned to.
//
// Deliberately the facts internal/prompttheme lets a segment read, because a
// fact a segment could reach around this struct would be a fact the harness
// could not pin — which is the sentence that struct's own documentation
// already carries, read from this end.
type Context struct {
	// Dir is where the prompt is drawn, and Home is what a segment
	// abbreviates it against.
	Dir  string
	Home string

	// Status is what the last command exited with and Duration is how long
	// it took.
	Status   int
	Duration time.Duration

	// Jobs is how many background jobs the shell is looking after.
	Jobs int

	// Columns is the terminal's width. A prompt with a right side is a
	// different prompt at a different width, so this is pinned and not
	// inherited.
	Columns int
}

// Source is one prompt program under comparison.
type Source interface {
	// Name is what the report calls it.
	Name() string

	// Available says whether this source can be driven here, and why not
	// where it cannot.
	//
	// A reason and not a boolean, because a source that is not run has to
	// be **printed** rather than dropped: a table listing three sources
	// where four were asked for, with nothing explaining the difference,
	// reads as a source that agreed.
	Available() (bool, string)

	// Render draws one prompt at this context and answers what a terminal
	// of Context.Columns would be showing.
	Render(Context) (*cellgrid.Grid, error)
}

// Row is one comparison.
type Row struct {
	// Name is the source that was compared against ours.
	Name string

	// Ran says whether the comparison happened, and Why says what stopped
	// it where it did not.
	Ran bool
	Why string

	// Cells is how many cells were compared and Unstable is how many were
	// left out because a side did not draw them the same way twice.
	Cells    int
	Unstable int

	// Differences is every cell the two sides drew differently.
	Differences []cellgrid.Difference

	// Ours and Theirs are the two screens, for a report that has to show
	// what it is talking about.
	Ours, Theirs *cellgrid.Grid
}

// Agrees reports whether the two prompts draw the same screen.
func (r Row) Agrees() bool { return r.Ran && len(r.Differences) == 0 }

// Compare drives both sides and grades one row.
//
// Each side twice, for the stability rule above, and **interleaved**: ours,
// theirs, ours, theirs. The order is the whole of whether the rule works.
//
// Taken back to back per side, a clock is stable *within* each side and
// differs *between* them — the two sides are seconds apart, because each
// render is a process — so the rule would exclude nothing and the row would
// differ in every digit. Interleaved, a side's own two renders span the same
// wall-clock window the comparison does, so anything moving at the clock's
// rate moves within a side too and is excluded. Measured: the first run of
// this against a real prompt reported 46 differences with 0 unstable, and
// four of them were the seconds hand.
func Compare(name string, ours, theirs Source, ctx Context) Row {
	row := Row{Name: name}
	if ok, why := ours.Available(); !ok {
		row.Why = "this shell: " + why
		return row
	}
	if ok, why := theirs.Available(); !ok {
		row.Why = why
		return row
	}

	ourFirst, err := ours.Render(ctx)
	if err != nil {
		row.Why = "this shell: " + err.Error()
		return row
	}
	theirFirst, err := theirs.Render(ctx)
	if err != nil {
		row.Why = err.Error()
		return row
	}
	ourSecond, err := ours.Render(ctx)
	if err != nil {
		row.Why = "this shell: " + err.Error()
		return row
	}
	theirSecond, err := theirs.Render(ctx)
	if err != nil {
		row.Why = err.Error()
		return row
	}

	unstable := union(moved(ourFirst, ourSecond), moved(theirFirst, theirSecond))
	row.Ran = true
	row.Ours, row.Theirs = ourSecond, theirSecond
	row.Unstable = len(unstable)
	for _, d := range cellgrid.Compare(theirSecond, ourSecond) {
		if unstable[at{d.Row, d.Col}] {
			continue
		}
		row.Differences = append(row.Differences, d)
	}
	row.Cells = cells(ourSecond, theirSecond) - row.Unstable
	return row
}

// at is one position.
type at struct{ row, col int }

// moved is the cells a source did not draw the same way twice.
func moved(a, b *cellgrid.Grid) map[at]bool {
	out := map[at]bool{}
	for _, d := range cellgrid.Compare(a, b) {
		out[at{d.Row, d.Col}] = true
	}
	return out
}

func union(a, b map[at]bool) map[at]bool {
	out := make(map[at]bool, len(a)+len(b))
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
}

// cells is how many positions the two screens cover between them.
func cells(a, b *cellgrid.Grid) int {
	rows := max(a.Rows(), b.Rows())
	cols := max(a.Cols(), b.Cols())
	return rows * cols
}

// Report renders a row the way the instrument prints it.
//
// The two screens are printed under a row that differs, because a list of
// cell coordinates is not something a person can act on and the screens are.
func (r Row) Report() string {
	var b strings.Builder
	switch {
	case !r.Ran:
		fmt.Fprintf(&b, "%-16s not run — %s\n", r.Name, r.Why)
		return b.String()
	case r.Agrees():
		fmt.Fprintf(&b, "%-16s AGREES over %d cells (%d unstable, left out)\n",
			r.Name, r.Cells, r.Unstable)
		return b.String()
	}
	fmt.Fprintf(&b, "%-16s DIFFERS in %d of %d cells (%d unstable, left out)\n",
		r.Name, len(r.Differences), r.Cells, r.Unstable)
	for i, d := range r.Differences {
		if i == differencesShown {
			fmt.Fprintf(&b, "    … and %d more\n", len(r.Differences)-i)
			break
		}
		fmt.Fprintf(&b, "    %s\n", d)
	}
	b.WriteString("  the other program drew:\n")
	writeScreen(&b, r.Theirs)
	b.WriteString("  this shell drew:\n")
	writeScreen(&b, r.Ours)
	return b.String()
}

// differencesShown bounds the per-row list. A prompt that disagrees
// everywhere produces a difference per cell, and eighty lines of them is not
// a report anybody reads — the two screens underneath are.
const differencesShown = 12

func writeScreen(b *strings.Builder, g *cellgrid.Grid) {
	if g == nil {
		return
	}
	for row := range g.Rows() {
		fmt.Fprintf(b, "    | %s\n", g.Text(row))
	}
}
