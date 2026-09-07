// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/blairham/sh/internal/tty"
	"github.com/blairham/sh/syntax"
)

// `select`, the menu loop.
//
// It is a for-loop's header over a different loop: the words are a menu rather
// than a sequence, the body runs once per *reply* rather than once per word,
// and it ends when the input does rather than when the list does.
//
// Everything the user sees goes to standard error — the menu and the prompt
// both — which is what lets a script's own output be redirected without taking
// the menu with it. That much is unanimous. What the menu looks like is not,
// and it is the widest presentation difference measured anywhere in the panel.

// selectClause runs `select name [in words] do … done`.
func (r *Runner) selectClause(ctx context.Context, c *syntax.SelectClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		var items []string
		r.beginHeading()
		if c.HasItems {
			for _, w := range c.Items {
				items = append(items, r.expandWord(w)...)
			}
		} else {
			items = r.Params
		}
		if r.failedHeading() {
			// Before the menu is drawn, which is the whole of it here: a
			// failed heading otherwise printed a numbered menu and then
			// *blocked at the prompt* for a choice among words the shell had
			// said it could not read (#1215).
			return nil
		}
		// An empty menu is not an endless prompt: the loop does not run, the
		// status is 0 and nothing is printed. Unanimous, and the alternative
		// would hang.
		if len(items) == 0 {
			r.status = 0
			return nil
		}

		menu := r.selectMenu(items)
		show := true
		for {
			if show {
				r.errf("%s", menu)
				show = false
			}
			r.selectPrompt()
			line, atEOF := r.readLine(false)
			switch {
			case atEOF && line == "":
				return r.selectEOF()
			case atEOF:
				// A final reply with no newline. bash and ksh93 ignore it
				// and end the loop; zsh takes it. Asked here rather than
				// once at the top, so a script whose input ends properly
				// never needs the answer at all.
				take := r.ask(r.sem().SelectTakesUnterminatedReply,
					"an unterminated final reply to `select`")
				if r.unspecified {
					return nil
				}
				if !take {
					return r.selectEOF()
				}
			}
			if strings.TrimSpace(line) == "" {
				// A blank reply reprints the menu and does not run the body,
				// which is the only way to see it again.
				show = true
				continue
			}
			// REPLY is the line as typed and the name is the item it chose,
			// which are different questions: a reply out of range leaves the
			// name empty and REPLY holding what was typed, and the body still
			// runs. A script tests the name to find out.
			r.setVar("REPLY", line)
			r.setVar(c.Name, selectChoice(items, line))
			if err := r.runList(ctx, c.Body); err != nil {
				return err
			}
			if stop := r.loopControl(); stop {
				return nil
			}
		}
	})
}

// selectChoice is the item a reply names, or empty if it names none.
func selectChoice(items []string, line string) string {
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(items) {
		return ""
	}
	return items[n-1]
}

// selectPrompt writes PS3, which is read fresh each iteration because the body
// may have changed it.
func (r *Runner) selectPrompt() {
	// ksh93 prints no prompt at all unless the input is a terminal, which is
	// why a script's transcript has menus and no `#?` in it there and does in
	// the other two.
	if r.ask(r.sem().SelectPromptNeedsTerminal, "the select prompt needing a terminal") && !r.stdinIsTerminal() {
		return
	}
	prompt, ok := r.getVar("PS3")
	if !ok {
		prompt = Wording(r.diag().SelectPrompt, "#? ")
	}
	r.errf("%s", prompt)
}

// selectEOF ends the loop when the input runs out.
//
// Two newlines and two questions, and no shell answers both the same way.
// zsh closes the prompt line where the prompt was, on standard error; bash
// writes one to standard *output* instead, the only thing this loop ever puts
// there; ksh93 writes neither, having printed no prompt to close.
//
// Splitting the streams to find that out needs care. Measuring it as
// `2>&1 >/dev/null` under zsh reports both newlines on stderr for every shell,
// because zsh's MULTIOS sends the output to both — the same contaminated-probe
// trap docs/spec/oracle.md records, from a shell that was the harness rather
// than the subject.
func (r *Runner) selectEOF() error {
	if r.ask(r.sem().SelectEofEndsPromptLine, "select closing the prompt line when the input ends") {
		r.errf("\n")
	}
	if r.ask(r.sem().SelectEofPrintsNewline, "select printing a newline on output when the input ends") {
		r.printf("\n")
	}
	if r.ask(r.sem().SelectEofIsSuccess, "the input running out being a success") {
		r.status = 0
		return nil
	}
	r.status = 1
	return nil
}

// stdinIsTerminal reports whether the shell's input is a terminal.
func (r *Runner) stdinIsTerminal() bool { return inputIsTerminal(r.Stdin) }

// inputIsTerminal reports whether a stream a builtin is about to read is a
// terminal — `select`'s prompt asks about the shell's input, `read -p`'s
// about whatever -u resolved to.
//
// The kernel is asked, through internal/tty, and that is the whole of it. It
// used to be a character-device test with a hand-rolled exception for the null
// device, because the exact answer lived in `repl` and `repl` imports this
// package. #525 is what the approximation cost: every character device that is
// neither a terminal nor `/dev/null` read as a terminal — `/dev/zero`,
// `/dev/random`, a serial port, a printer.
//
// Measured 2026-09-06 against bash 5.3.15, bash 3.2.57 and bash-as-sh, all
// three identical: `read -p 'PROMPT-42 ' v < /dev/random` reads a line, exits
// 0 and prints **no prompt**, where this shell printed one. `/dev/random` is
// the case that says the question is about terminals rather than about the
// null device — the read succeeds there, so there is every reason to have
// prompted, and no shell in the panel does.
//
// The null-device exception is gone rather than moved. The ioctl answers
// ENOTTY there without being told the path, so the `os.Stat(os.DevNull)` that
// used to be justified here — a fixed path, outside the boundary, which a
// `-deny /dev/null` policy could have turned into a terminal — is not needed
// by anything any more.
//
// Still a question a Runner may ask: it is about a descriptor the Runner was
// *handed*, not about the process. An embedded Runner may have been given a
// pipe while the program around it sits at a terminal, and this answers about
// the shell's input and not the program's.
func inputIsTerminal(in io.Reader) bool {
	f, ok := in.(*os.File)
	if !ok {
		return false
	}
	return tty.IsTerminal(f)
}

// selectMenu lays the items out the way the dialect does.
//
// Three engines, and the difference is not cosmetic detail that could be
// averaged: the same nine items are nine lines in two shells and one line in
// the third.
func (r *Runner) selectMenu(items []string) string {
	switch r.sem().SelectLayout {
	case SelectMenuColumns:
		return columnMenu(items, r.selectWidth(), spacePad)
	case SelectMenuVerticalThenColumns:
		width := r.selectWidth()
		if cells := numberedCells(items, false); fits(cells, tabPad, width) {
			return verticalMenu(items)
		}
		return columnMenu(items, width, tabPad)
	default:
		// Vertical and nothing else. ksh93 columnizes too, but on the
		// terminal's *height* rather than its width — measured and not built,
		// because a menu long enough to reach it cannot be graded by the
		// corpus, whose cases do not control LINES.
		return verticalMenu(items)
	}
}

const tabStop = 8

// verticalMenu is one item per line with the numbers right-aligned, which is
// what makes `9)` and `10)` line up their parentheses.
func verticalMenu(items []string) string {
	var b strings.Builder
	digits := len(strconv.Itoa(len(items)))
	for i, it := range items {
		fmt.Fprintf(&b, "%*d) %s\n", digits, i+1, it)
	}
	return b.String()
}

// numberedCells renders each entry. Right-aligning the number is the vertical
// layout's rule and, oddly, a columnized layout's rule for every column but
// the first — measured, and reproduced rather than explained.
func numberedCells(items []string, align bool) []string {
	digits := len(strconv.Itoa(len(items)))
	cells := make([]string, len(items))
	for i, it := range items {
		if align {
			cells[i] = fmt.Sprintf("%*d) %s", digits, i+1, it)
			continue
		}
		cells[i] = fmt.Sprintf("%d) %s", i+1, it)
	}
	return cells
}

// fits reports whether the whole list would go on one line.
//
// It is the same question in both columnizing layouts and they do opposite
// things with the answer: the tab layout falls back to *vertical* when the
// list fits, and the space layout puts it all on the one line.
func fits(cells []string, pad padder, width int) bool {
	return len(cells)*cellWidth(cells, pad) <= width
}

// cellWidth is the width one entry occupies under a layout's own rule.
func cellWidth(cells []string, pad padder) int {
	longest := 0
	for _, c := range cells {
		longest = max(longest, len(c))
	}
	return pad.width(longest)
}

type padder struct {
	// width is the space one entry occupies, which the two layouts compute
	// differently: the tab layout rounds up to a tab stop, and the space
	// layout adds a fixed two-space gutter and does not round at all.
	width func(longest int) int
	// join renders one row from its cells.
	join func(cells []string, width int) string
}

// tabPad separates entries with a tab and does not align them. The columns in
// this layout only look aligned when the entries happen to be the same width.
var tabPad = padder{
	width: func(longest int) int { return (longest/tabStop + 1) * tabStop },
	join: func(cells []string, _ int) string {
		return strings.Join(cells, "\t")
	},
}

// spacePad pads every entry to the column width, the last one included — so a
// row ends in trailing spaces.
var spacePad = padder{
	width: func(longest int) int { return longest + 2 },
	join: func(cells []string, width int) string {
		var b strings.Builder
		for _, c := range cells {
			b.WriteString(c)
			b.WriteString(strings.Repeat(" ", width-len(c)))
		}
		return b.String()
	},
}

// aligns reports whether this layout right-aligns the numbers in every column
// but the first, which the tab layout does and the space layout does not.
func (p padder) aligns() bool { return p.width(0) == tabStop }

// columnMenu fills column-major: reading *down* the first column gives 1, 2, 3.
func columnMenu(items []string, width int, pad padder) string {
	// The space layout does not right-align at all, and the tab layout aligns
	// every column but the first — so the cells are rendered both ways and
	// each column takes the one it wants.
	plain, aligned := numberedCells(items, false), numberedCells(items, true)
	cw := cellWidth(plain, pad)
	perRow := max(1, width/cw)
	rows := (len(items) + perRow - 1) / perRow
	cols := (len(items) + rows - 1) / rows

	var b strings.Builder
	for row := range rows {
		var line []string
		for col := range cols {
			i := col*rows + row
			if i >= len(items) {
				continue
			}
			if pad.aligns() && col > 0 {
				line = append(line, aligned[i])
				continue
			}
			line = append(line, plain[i])
		}
		b.WriteString(pad.join(line, cw))
		b.WriteString("\n")
	}
	return b.String()
}

// selectWidth is the terminal width the layout works to.
//
// COLUMNS when it is set, because that is what a shell reads and what a Runner
// can be told. What to do when it is not is the axis: bash works to 80 and
// zsh, with no terminal to ask, works to no limit at all and puts forty items
// on one line. Only the layouts that use a width ask, so the shell whose menu
// is always vertical never has to answer.
func (r *Runner) selectWidth() int {
	if v, ok := r.getVar("COLUMNS"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	if r.ask(r.sem().SelectAssumesUnboundedWidth, "an unset COLUMNS meaning no limit") {
		return math.MaxInt32
	}
	return 80
}
