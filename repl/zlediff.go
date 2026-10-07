// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// Drawing a change the way zsh's editor does: the old row and the new
// compared cell by cell.
//
// The default repaint writes everything from the first changed character to
// the end of the line again. zsh 5.9.2 writes only the cells that differ,
// steps over runs that already match where stepping is cheaper than writing
// them, and closes or opens room with the terminal's delete and insert
// sequences where that is cheaper still. Measured 2026-10-07 through a
// pseudo-terminal, `zsh -f -i`, `PS1='P> '`, 80 columns (#6341):
//
//	                                 xterm                      tvi912 (ich1 \EQ, dch1 \EW)
//	X into `a bb … ffffff ggggggg`   X \e[5Cf \e[6Cg\e[15D       —
//	^T there                         \b e                        —
//	Backspace before a long tail     \b\e[P\e[41C \e[42D         \b\EW, tabs and the tail's
//	                                                             last cells to the end, ' '
//	^W of 3 before `cd efghijklmnopq` \b\b\b\e[3P\e[16C   \e[19D \b\b\b\EW\EW\EW\t\t   \r\t
//	X typed before `cd efghijklmnopq` Xcd efghijklmnopq\e[16D    \EQX\t\tnopq\EW\r\tab X
//	Backspace with `pq` after it     \bpq \b\b\b                 the same
//
// Read off those and the rows that pin their edges:
//
//   - **Writing** goes from the first cell that differs, cell by cell, to the
//     last that does. A run of two or more cells that already hold what they
//     should is moved over the way any move right is made — `\e[2C`, hp2621's
//     `\E&a14C`, or a tab and the run's last cells under vt52 — though it is
//     not always fewer bytes; a single such cell is written.
//   - **Closing room**: at the first differing cell whose neighbor differs
//     too, where the rest of the old line lines up with the rest of the new
//     once the k extra characters are taken out — `dch` with the count, else
//     `dch1` k times (`\e[P` for one, `\e[3P` for three), then the move to
//     the new end and k spaces. Backspace on the X of `Xabc` is `\e[P` at
//     once; on the X of `Xaab`, whose neighbor already matches, it is `a`
//     and then `\e[P` one column on — and where a single matching cell is
//     the same character as the one to go, that one goes instead.
//   - **Opening room** the same way with `ich1` only (xterm, which has `ich`
//     and no `ich1`, writes the cells): k of it and the characters, the move
//     to the new end, and there `dch1` k times or `el`, whichever is shorter
//     — tvi912 writes `\EW` for one and `\ET` for three.
//   - Whichever is fewest bytes, counting the move back to the cursor and a
//     delay as the text of the delay (wy50's `dch1` is `\EW$<1>`: one
//     character is closed with it, three are written), is what is drawn; at
//     a tie, room is closed or opened.
//   - A row left with nothing on it at all is erased with `el`: ^U with an
//     empty prompt is `\r\e[K`.
//
// The first column is still redrawn with the second, as rewritemotion.go
// says, and then the change is drawn from the second.

// repaintAsTheScreenIs is repaint for a shell that draws a change the way the
// comment above says, for a line on one row with nothing styled in it. It
// reports false where it does not apply.
func (e *editor) repaintAsTheScreenIs(m *terminalMotion, prompt drawnPrompt, cols int) bool {
	d := e.drawn
	if d.endRow != 0 || d.right || e.listingBelow || strings.ContainsRune(d.styled, esc) {
		return false
	}
	cur := e.displayed()
	styled := e.styled()
	if styled != string(cur) {
		return false
	}
	old := []rune(d.styled)
	for _, line := range [][]rune{old, cur} {
		for _, r := range line {
			if r < ' ' || isControl(r) || runeWidth(r) != 1 {
				return false
			}
		}
		if prompt.cells+len(line) >= cols {
			return false
		}
	}
	pc := prompt.cells
	row := e.viewOfRow(prompt, cur, cols, 0)
	// A cell past the end of the new line is to be blank; one past the end
	// of the old line holds nothing known, so a space typed there is
	// written rather than stepped over.
	newAt := func(i int) rune {
		if i < len(cur) {
			return cur[i]
		}
		return ' '
	}
	oldAt := func(i int) rune {
		if i < len(old) {
			return old[i]
		}
		return 0
	}

	p := 0
	for p < len(old) && p < len(cur) && old[p] == cur[p] {
		p++
	}
	to := pc + e.pos

	var b strings.Builder
	col := d.col
	changed := p < len(old) || p < len(cur)
	if changed && p == 1 && m.left1 != "" {
		// The first column again, and then the change. See
		// rewritemotion.go.
		m.columnAsTheScreenIs(&b, col, pc, row)
		b.WriteRune(cur[0])
		col = pc + 1
	}
	if changed {
		m.columnAsTheScreenIs(&b, col, pc+p, row)

		// What is left of the old line past the new end, up to its last
		// cell that is not blank already — on a terminal that can step
		// right; one that cannot covers all of it.
		end := len(old)
		if m.canMoveRight() {
			for end > len(cur) && old[end-1] == ' ' {
				end--
			}
		}
		end = max(end, len(cur))

		type way struct {
			bytes string
			col   int
			// extra is what the way costs beyond its bytes: a delay in a
			// sequence counts as the text of the delay, not the NULs it
			// is written as. Measured, wy50's `dch1` is `\EW$<1>`, and
			// zsh closes one character with it in front of a long tail
			// but writes the tail again for three, where tvi912, whose
			// `\EW` carries no delay, closes all three.
			extra int
		}
		var ways []way
		// same says a cell already holds what it should. Past the new end
		// a terminal that cannot step right writes over every cell, blank
		// or not: under dumb, ^U on `echo ` writes five spaces.
		same := func(i int) bool {
			if i >= len(cur) && !m.canMoveRight() {
				return false
			}
			return newAt(i) == oldAt(i)
		}
		// A run that already holds what it should is stepped over, two cells
		// or more of it — measured, `\e[2C` over two under xterm and
		// `\E&a14C` under hp2621, though neither is fewer bytes, and a tab
		// and the run's last cells under vt52 — and written where it is one.
		//
		// scan writes the cells from i up to stop and answers where the
		// cursor is and how many cells before stop it left unwritten: a
		// single matching cell just before stop is left, so that what
		// happens at stop can happen there instead where that is the same.
		scan := func(w *strings.Builder, i, stop int) (int, int) {
			for i < stop {
				if !same(i) {
					w.WriteRune(newAt(i))
					i++
					continue
				}
				j := i
				for j < stop && same(j) {
					j++
				}
				switch {
				case j-i >= 2:
					m.columnAsTheScreenIs(w, pc+i, pc+j, row)
				case j == stop:
					return pc + i, j - i
				default:
					w.WriteRune(newAt(i))
				}
				i = j
			}
			return pc + i, 0
		}

		var w strings.Builder
		i := p
		if el := m.eraseToRowEnd(); pc == 0 && len(cur) == 0 && p == 0 && el != "" {
			// A row with nothing left on it at all is erased rather than
			// covered: measured, ^U with an empty prompt is `\r\e[K`
			// under xterm and `\r\eK` under vt52.
			w.WriteString(el)
			ways = append(ways, way{w.String(), pc, 0})
		} else {
			// The cells written to the end, with the matching runs at the
			// end left as they are.
			last := end
			for last > p && same(last-1) {
				last--
			}
			c, left := scan(&w, i, last)
			ways = append(ways, way{w.String(), c + left, 0})
		}

		// Closing or opening room is tried at the first cell, from the
		// front, where the rest of the old line lines up with the rest of
		// the new once the difference in their lengths is taken out — and
		// not at a cell whose neighbor already holds what it should, which
		// is written instead: measured, Backspace on the X of `Xabc` is
		// `\e[P`, and on the X of `Xaab` it is `a` and then `\e[P` one
		// column on.
		k := len(old) - len(cur)
		at := -1
		for x := p; x < len(old) && x < len(cur)+max(0, -k); x++ {
			if same(x) {
				continue
			}
			if x+1 < len(cur) && x+1 < len(old) && same(x+1) {
				continue
			}
			if k > 0 && string(old[x+k:]) == string(cur[x:]) || k < 0 && x-k <= len(cur) && string(cur[x-k:]) == string(old[x:]) {
				at = x
			}
			break
		}
		if at >= 0 && k > 0 && m.canDelete() {
			var del strings.Builder
			c, left := scan(&del, p, at)
			if left == 1 && old[at-1] == old[at-1+k] {
				// The one matching cell left is the same character as the
				// one to go, so it goes instead, where the cursor is.
				c = pc + at - 1
			} else if left == 1 {
				del.WriteRune(newAt(at - 1))
				c = pc + at
			}
			raw := ""
			before := del.Len()
			if one := m.repeated(m.delete1, k); one != "" && (m.delete == "" || len(one) <= len(m.counted(m.delete, k))) {
				// One step at a time where that is no longer: `\e[P` for
				// one, `\e[3P` for three.
				del.WriteString(one)
				raw = strings.Repeat(m.delete1, k)
			} else {
				m.closeUp(&del, k)
				raw = ParameterizedString(m.delete, []int{k})
			}
			extra := len(raw) - (del.Len() - before)
			m.columnAsTheScreenIs(&del, c, pc+len(cur), row)
			del.WriteString(strings.Repeat(" ", k))
			ways = append(ways, way{del.String(), pc + len(cur) + k, extra})
		}
		if at >= 0 && k < 0 && m.insert1 != "" {
			n := -k
			drop := m.repeated(m.delete1, n)
			if el := m.eraseToRowEnd(); el != "" && (drop == "" || len(el) < len(drop)) {
				drop = el
			}
			if drop != "" {
				var ins strings.Builder
				c, left := scan(&ins, p, at)
				from := at
				switch {
				case left == 1 && cur[at-1] == cur[at-1+n]:
					// Opened in front of the one matching cell instead,
					// which comes to the same: `X` before `aa` under
					// tvi912 is `X\EQa`.
					from = at - 1
				case left == 1:
					ins.WriteRune(newAt(at - 1))
					c = pc + at
				}
				ins.WriteString(m.repeated(m.insert1, n))
				ins.WriteString(string(cur[from : from+n]))
				m.columnAsTheScreenIs(&ins, c+n, pc+len(cur), row)
				ins.WriteString(drop)
				ways = append(ways, way{ins.String(), pc + len(cur), 0})
			}
		}

		best, bestCost := 0, -1
		for k, wy := range ways {
			var back strings.Builder
			m.columnAsTheScreenIs(&back, wy.col, to, row)
			// At a tie the room is closed or opened: measured, wy50
			// closes one character with `\EW` where writing the cells
			// costs the same.
			if c := len(wy.bytes) + wy.extra + back.Len(); bestCost < 0 || c <= bestCost {
				best, bestCost = k, c
			}
		}
		b.WriteString(ways[best].bytes)
		col = ways[best].col
	}
	m.columnAsTheScreenIs(&b, col, to, row)

	e.row = 0
	if b.Len() > 0 {
		e.write(b.String())
	}
	e.drawn = drawnLine{
		valid:  true,
		styled: styled,
		prompt: prompt.text,
		cells:  prompt.cells,
		cols:   cols,
		row:    0, col: to,
		endRow: 0, endCol: pc + len(cur),
	}
	return true
}
