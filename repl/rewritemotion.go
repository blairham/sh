// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// Moving the way zsh's editor moves on a terminal's own sequences.
//
// terminalmotion.go spells a move with the description's sequences; this is
// the other half, which move to make, and it differs from the editor's
// default in five measured ways. 2026-10-07 through a pseudo-terminal, zsh
// 5.9.2 `-f -i`, typing one key at a time (#6325):
//
//	                         xterm        ansi          vt52           dumb
//	`c` after `e`            \bec         \e[Dec        \eDec          c
//	^B ×1                    \b           \e[D          \eD            \b
//	`X` mid-line, back 3     \b\b\b       \e[3D         \eD\eD\eD      \b\b\b
//	back 7 / back 8          \b ×7, \e[8D \e[7D         \eD ×7         \b ×7
//	^A on P> echo abc        \e[8D        \e[8D         \rP>           \r + 3 spaces
//	^E from col 4 to 11      \e[7C        \e[7C         \tabc          cho abc
//	Backspace mid-line       \bdef \b×4   \e[Ddef \e[4D \eDdef \eD ×4  \bdef \b×4
//	^K over 3                ' '×3 \b×3   ' '×3 \e[3D   ' '×3 \eD ×3   ' '×3 \b×3
//
// Read off those and the rows below:
//
//   - **Left, with a counted move** (`cub`): the single step repeated while
//     that is shorter than twice the counted move — 7 backspaces against
//     `\e[7D`'s 4 bytes, and `\e[D\e[D` against `\e[2D` — and the counted move
//     past that. To the row's first column it is a carriage return; measured
//     with an empty prompt, ^A on xterm is `\r` where it would be `\e[15D`.
//   - **Left, without one**: a carriage return and the move right again when
//     the target is no farther from the row's start than from the cursor, and
//     the single step otherwise. `PS1='abcdefghij> '` under vt52, ^A from
//     column 23 to 12 is eleven `\eD`, and from column 43 it is
//     `\r\t\eC\eC\eC\eC`; M-b from 6 to 3 under adm3a returns. A terminal the
//     database does not describe is not moved left at all: under a `$TERM`
//     with no entry ^B writes nothing.
//   - **Right, with a counted move** (`cuf`): always the counted move, one
//     column included (`\e[1C`).
//   - **Right, without one but with `hpa`**: the move to the column, counted
//     from the row's start — hp2621's `\E&a17C`, after a carriage return
//     for ^A as well (`\r\E&a3C`).
//   - **Right, with neither**: what is already on the screen is written again.
//     A tab (`ht`, where the description has one) wherever the next stop is
//     no farther than the target; then, inside the prompt, the single step
//     (`cuf1`) or — from the row's start to the prompt's end, where it is
//     fewer bytes — the prompt itself (vt52 `\rP> `, but wy50 `\r^L^L^L` at a
//     tie); a move that has to cross the prompt into the line writes the
//     prompt again from a carriage return (vt52, `abcdefghij> `: `\r\t\r`
//     and the prompt, the tab written and then abandoned), and a terminal
//     with no `cuf1` at all writes spaces over the prompt (`dumb`: `\r` and
//     three spaces). In the line it writes the line's own characters.
//   - **A shorter line** has its old tail covered with spaces on the row,
//     never erased with `el` or `ed`.
//   - **The first column is redrawn with the second.** Where the first
//     character that changed is the line's second, the draw starts from the
//     first: `e`, then `c`, writes `\bec`, and `ced` from `ce` writes
//     `\bcde\b`. Not on a terminal with no `cub1` (`dumb` writes `c`).

// rowView is what one row of the screen holds, for a move that writes it
// again rather than moving over it.
type rowView struct {
	// promptText is the prompt's row as it was written, and promptCells how
	// wide it is. Both are empty on a row the prompt is not on.
	promptText  string
	promptCells int

	// promptCost is what writing the prompt again is weighed at: its bytes,
	// two for each region of it said not to be drawn, and one for each
	// counted column. Measured 2026-10-07, zsh 5.9.2 under vt52 moves to the
	// end of `%{ab%G%}> ` with three `\eC` and to the end of `%{ab%G%}jn> `
	// by writing `abjn> ` again — six bytes weighed at nine against ten —
	// where `ab> ` weighed at seven is more than six; and under wy50 the
	// end of `x%3Gy> ` is seven `^L`, `xy> ` weighed at seven being a tie,
	// and a tie is the steps.
	promptCost int

	// promptFixed says the prompt has rows above this one, promptLead is
	// them as written and leadRows how many there are. Such a prompt is not
	// written again to move inside it, or across it from part way along —
	// measured, zsh under vt52 with `PS1=$'JNROW\njn> '` moves to the start
	// of the line with `\r` and four `\eC` where a one-row `P> ` is
	// written again — and is written again whole, upper rows and all, to
	// cross it from the row's start: `\r\eAJNROW\r\njn> ab `.
	promptFixed bool
	promptLead  string
	leadRows    int

	// cells is what draws the character starting in each column after the
	// prompt: wideHalf for the second column of a wide character, and empty
	// for a column the line does not reach, which is blank.
	cells []string
}

// wideHalf marks the column a wide character spills into.
const wideHalf = "\x00"

// viewOfRow is the row the line occupies at row, counted from the prompt's row,
// or nil where it holds something that cannot simply be written again — a
// control character, which is drawn as a styled caret, or a line break a
// paste put there. A tab is the spaces it was drawn as.
func (e *editor) viewOfRow(prompt drawnPrompt, line []rune, cols, row int) *rowView {
	if cols <= 0 {
		return nil
	}
	v := &rowView{cells: make([]string, cols+1)}
	if row == 0 {
		v.promptText, v.promptCells, v.promptFixed = prompt.text, prompt.cells, prompt.lead != ""
		v.promptCost = len(prompt.text) + 2*prompt.regions + prompt.counted
		v.promptLead, v.leadRows = prompt.lead, leadRows(prompt.lead)
	}
	for i, r := range line {
		at, col := placeAt(prompt.cells, line, i, cols)
		if at < row {
			continue
		}
		if at > row {
			break
		}
		if r == '\t' {
			// Drawn as spaces to the stop, so spaces are what is there.
			for w := range tabWidth(col, cols) {
				v.cells[col+w] = " "
			}
			continue
		}
		if r == '\n' || isControl(r) {
			return nil
		}
		v.cells[col] = string(r)
		if runeWidth(r) == 2 && col+1 < len(v.cells) {
			v.cells[col+1] = wideHalf
		}
	}
	return v
}

// rewriteRight moves right from col to to by writing what the row holds. See
// the comment at the top of this file.
func (m *terminalMotion) rewriteRight(b *strings.Builder, v *rowView, col, to int) {
	if v == nil {
		b.WriteString(m.repeated(m.right1, to-col))
		return
	}
	if m.tab != "" && m.tabWidth > 0 {
		for next := (col/m.tabWidth + 1) * m.tabWidth; next <= to; next = (col/m.tabWidth + 1) * m.tabWidth {
			b.WriteString(m.out(m.tab))
			col = next
		}
	}
	if pw := v.promptCells; col < pw {
		step := m.out(m.right1)
		if to <= pw {
			n := to - col
			switch {
			case step == "":
				b.WriteString(strings.Repeat(" ", n))
			case col == 0 && to == pw && !v.promptFixed && v.promptCost < n*len(step):
				b.WriteString(v.promptText)
			default:
				b.WriteString(strings.Repeat(step, n))
			}
			return
		}
		// Across the rest of the prompt into the line: the prompt written
		// again where that costs no more than the steps, counting only the
		// part of it still ahead and the return that a cursor inside it
		// needs — and then written whole, from the row's start.
		again := promptBytesFrom(v.promptText, col)
		if col == 0 {
			again = v.promptCost
		}
		if col > 0 {
			again++
		}
		if v.promptFixed {
			// And one step up, though neither the rows themselves nor
			// the steps after the first: measured, `jn> ` under `JNROW`
			// is written again under vt52, whose four steps are 8 bytes,
			// and stepped over under adm3a and wy50, whose four are 4; a
			// two-row lead counts the same as one.
			again += len(m.upBy(1))
		}
		switch {
		case step == "":
			b.WriteString(strings.Repeat(" ", pw-col))
		case v.promptFixed && col > 0:
			b.WriteString(strings.Repeat(step, pw-col))
		case again <= (pw-col)*len(step):
			if col > 0 {
				b.WriteString("\r")
			}
			if v.promptFixed {
				b.WriteString(m.upBy(v.leadRows))
				b.WriteString(v.promptLead)
			}
			b.WriteString(v.promptText)
		default:
			b.WriteString(strings.Repeat(step, pw-col))
		}
		col = pw
	}
	for col < to {
		s := ""
		if col < len(v.cells) {
			s = v.cells[col]
		}
		switch s {
		case wideHalf:
			// Half way through a wide character, which cannot be written
			// from its middle: stepped over instead.
			b.WriteString(m.repeated(m.right1, 1))
			col++
			continue
		case "":
			s = " "
		}
		b.WriteString(s)
		col += max(1, runeWidth([]rune(s)[0]))
	}
}

// columnAsTheScreenIs moves along one row the way the comment at the top of
// this file says.
func (m *terminalMotion) columnAsTheScreenIs(b *strings.Builder, fromCol, toCol int, row *rowView) {
	switch {
	case toCol == fromCol:
	case toCol > fromCol:
		if m.right != "" {
			b.WriteString(m.rightBy(toCol - fromCol))
			return
		}
		if m.column != "" {
			b.WriteString(m.counted(m.column, toCol))
			return
		}
		m.rewriteRight(b, row, fromCol, toCol)
	case m.left != "":
		steps := fromCol - toCol
		if toCol == 0 {
			b.WriteString("\r")
			return
		}
		single := strings.Repeat(m.stepLeft(), steps)
		if counted := m.counted(m.left, steps); len(single) >= 2*len(counted) {
			b.WriteString(counted)
			return
		}
		b.WriteString(single)
	case toCol <= fromCol-toCol:
		b.WriteString("\r")
		m.columnAsTheScreenIs(b, 0, toCol, row)
	default:
		b.WriteString(strings.Repeat(m.stepLeft(), fromCol-toCol))
	}
}

// promptBytesFrom is how many bytes of the prompt's row draw it from column
// col on: its escape sequences included, and the characters before col not.
func promptBytesFrom(text string, col int) int {
	at := 0
	for i := 0; i < len(text); {
		escape, size := nextToken(text, i)
		if !escape {
			if at >= col {
				return len(text) - i
			}
			r := []rune(text[i : i+size])[0]
			at += max(1, runeWidth(r))
		}
		i += size
	}
	return 0
}

// trailingBlanks is how many of the last at most n characters of a drawn line
// are spaces: cells a shorter line need not cover, since they are blank
// already. Measured 2026-10-07, zsh 5.9.2 under xterm, ^U on `echo ` writes
// four spaces and not five.
func trailingBlanks(styled string, n int) int {
	var plain []rune
	for i := 0; i < len(styled); {
		escape, size := nextToken(styled, i)
		if !escape {
			plain = append(plain, []rune(styled[i:i+size])...)
		}
		i += size
	}
	blanks := 0
	for i := len(plain) - 1; i >= 0 && blanks < n && plain[i] == ' '; i-- {
		blanks++
	}
	return blanks
}
