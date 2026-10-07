// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// Moving the cursor and erasing with the sequences of the terminal `$TERM`
// names.
//
// The editor used to speak ANSI to every terminal: `\e[nD`, `\e[nC`, `\e[nA`,
// `\e[nB`, `\e[K` and `\e[J` whatever the description said. A terminal with
// sequences of its own is drawn on with those, by both line editors measured.
// 2026-10-07 through a pseudo-terminal, `PS1='P> '`, `echo abc` then ^A, then
// `#`:
//
//	bash 5.3.20, TERM=vt52   \r\eC\eC\eC, then #echo abc\r\eC\eC\eC\eC
//	                         (vt52's cuf1 is \eC, and it has no cub or cuf)
//	bash 5.3.20, TERM=wy50   \r^L^L^L                   (wy50's cuf1 is ^L)
//	zsh 5.9.2, TERM=vt52     Left is \eD, its cub1
//	zsh 5.9.2, TERM=ansi     Left is \e[D, its cub1, and three left \e[3D
//
// So every sequence is read from the description: `cub1`/`cub` to go left,
// `cuf1`/`cuf` right, `cuu1`/`cuu` up, `cud1`/`cud` down, `el` and `ed` to
// erase, and `sgr0` for whether there are attributes to reset at all. What the
// editor *decides* to draw is unchanged by this — a shell's own choices are in
// EditorStyle — and on a terminal whose description holds the ANSI spellings,
// which is xterm's and every terminal emulator's, every byte is what it was
// (#6324).
//
// A delay in a sequence (`$<2>`, vt100's) is taken off, which is what bash
// writes; see PaddedCapability for the shell that writes it as NULs.

// terminalMotion is the sequences one terminal moves and erases with, still in
// terminfo's own language: a counted move carries its `%p1%d`.
type terminalMotion struct {
	left1, left   string
	right1, right string
	up1, up       string
	down1, down   string
	eraseRow      string
	eraseBelow    string
	clear         string

	// resets says the terminal has attributes to turn off, so the reset
	// before an erase is worth writing. A terminal without `sgr0` draws
	// nothing in color and gets none.
	resets bool

	// insert and insert1 are `ich` and `ich1`, insertOn and insertOff
	// `smir` and `rmir`, and delete and delete1 `dch` and `dch1`. See
	// inplace.go.
	insert, insert1     string
	insertOn, insertOff string
	delete, delete1     string

	// column is `hpa`, a move to a column counted from the row's start.
	column string

	// tab is `ht`, and tabWidth the distance between the stops it moves to:
	// `it`, or 8 where the description has `ht` and does not say.
	tab      string
	tabWidth int

	// speed is the terminal's output speed, for writing a delay as NULs; 0
	// takes delays out. See PaddedCapability.
	speed int

	// undescribed says the database has no description of the terminal at
	// all. See stepLeft.
	undescribed bool

	// asTheScreenIs chooses each move the way rewritemotion.go says rather
	// than by the fewest bytes. See EditorStyle.MovesAsTheScreenIs.
	asTheScreenIs bool

	// inPlace draws a change the way inplace.go says. See
	// EditorStyle.DrawsChangesInPlace.
	inPlace bool
}

// ansiMotion is what the editor speaks to a terminal it was not told about —
// a session with no `$TERM` to read, or a shell that does not read one. It is
// xterm's description, which every terminal emulator in use understands.
var ansiMotion = terminalMotion{
	left1: "\b", left: "\x1b[%p1%dD",
	right1: "\x1b[C", right: "\x1b[%p1%dC",
	up1: "\x1b[A", up: "\x1b[%p1%dA",
	down1: "\x1b[B", down: "\x1b[%p1%dB",
	eraseRow: "\x1b[K", eraseBelow: "\x1b[J",
	clear:  clearScreenSequence,
	resets: true,
}

// motionOf is the motion caps describes. A capability the description
// does not hold is empty.
func motionOf(caps []TerminalCapability, speed int) terminalMotion {
	m := terminalMotion{speed: speed}
	for _, c := range caps {
		if c.Extended {
			continue
		}
		if c.Kind == NumericCapability && c.Terminfo == "it" {
			if n, ok := atoiStrict(c.Value); ok && n > 0 {
				m.tabWidth = n
			}
			continue
		}
		if c.Kind != StringCapability {
			continue
		}
		switch c.Terminfo {
		case "cub1":
			m.left1 = c.Value
		case "cub":
			m.left = c.Value
		case "cuf1":
			m.right1 = c.Value
		case "cuf":
			m.right = c.Value
		case "cuu1":
			m.up1 = c.Value
		case "cuu":
			m.up = c.Value
		case "cud1":
			m.down1 = c.Value
		case "cud":
			m.down = c.Value
		case "el":
			m.eraseRow = c.Value
		case "ed":
			m.eraseBelow = c.Value
		case "clear":
			m.clear = c.Value
		case "sgr0":
			m.resets = true
		case "ht":
			m.tab = c.Value
		case "hpa":
			m.column = c.Value
		case "ich":
			m.insert = c.Value
		case "ich1":
			m.insert1 = c.Value
		case "smir":
			m.insertOn = c.Value
		case "rmir":
			m.insertOff = c.Value
		case "dch":
			m.delete = c.Value
		case "dch1":
			m.delete1 = c.Value
		}
	}
	if m.tab == "" {
		m.tabWidth = 0
	} else if m.tabWidth == 0 {
		m.tabWidth = 8
	}
	return m
}

// out is a sequence ready to write: its delays written for the terminal's
// speed or taken off.
func (m *terminalMotion) out(s string) string {
	return PaddedCapability(s, m.speed)
}

// counted is a counted sequence for n, or empty where the terminal has none.
func (m *terminalMotion) counted(seq string, n int) string {
	if seq == "" {
		return ""
	}
	return m.out(ParameterizedString(seq, []int{n}))
}

// repeated is a single step n times, or empty where the terminal has none.
func (m *terminalMotion) repeated(seq string, n int) string {
	if seq == "" || n <= 0 {
		return ""
	}
	return strings.Repeat(m.out(seq), n)
}

// stepLeft is one column left: `cub1`, or a backspace where the description
// has none, which is what both shells write on `dumb`. A terminal with no
// description at all is not stepped left on where the moves are chosen as
// the screen is: measured 2026-10-07, zsh 5.9.2 under a `$TERM` the database
// has no entry for writes nothing for ^B (#6325).
func (m *terminalMotion) stepLeft() string {
	if m.undescribed && m.asTheScreenIs {
		return ""
	}
	if m.left1 == "" {
		return "\b"
	}
	return m.out(m.left1)
}

// upBy and downBy move n rows: the counted sequence where there is one, and
// the single step n times otherwise.
func (m *terminalMotion) upBy(n int) string {
	if s := m.counted(m.up, n); s != "" {
		return s
	}
	return m.repeated(m.up1, n)
}

func (m *terminalMotion) downBy(n int) string {
	if s := m.counted(m.down, n); s != "" {
		return s
	}
	return m.repeated(m.down1, n)
}

// rightBy moves n columns right: the counted sequence where there is one,
// and the single step n times otherwise. Empty for a terminal that cannot.
func (m *terminalMotion) rightBy(n int) string {
	if n <= 0 {
		return ""
	}
	if s := m.counted(m.right, n); s != "" {
		return s
	}
	return m.repeated(m.right1, n)
}

// canMoveRight reports whether the terminal can move the cursor right at all.
func (m *terminalMotion) canMoveRight() bool {
	return m.right != "" || m.right1 != ""
}

// eraseToRowEnd and eraseToScreenEnd are `el` and `ed`, empty where the
// terminal has none.
func (m *terminalMotion) eraseToRowEnd() string    { return m.out(m.eraseRow) }
func (m *terminalMotion) eraseToScreenEnd() string { return m.out(m.eraseBelow) }

// clearScreen is `clear`: home, and the whole screen erased. A terminal whose
// description has none is sent the ANSI sequence, as it always was — measured
// 2026-10-07 under `TERM=dumb`, which has none, bash 5.3.20 starts a new row
// for ^L and zsh 5.9.2 writes a form feed, and neither is a clear.
func (m *terminalMotion) clearScreen() string {
	if m.clear == "" {
		return clearScreenSequence
	}
	return m.out(m.clear)
}

// resetBeforeErase is the reset an erase is preceded by, for a cursor that
// may be inside a highlighted run: highlightReset where the terminal has
// attributes at all, and nothing where it has none to reset.
func (m *terminalMotion) resetBeforeErase() string {
	if !m.resets {
		return ""
	}
	return highlightReset
}

// moves is the terminal motion this read draws with.
func (e *editor) moves() *terminalMotion {
	if e.motion == nil {
		return &ansiMotion
	}
	return e.motion
}

// screenView answers what a row of the screen holds, for a move that writes
// it again rather than stepping over it; nil where nothing is known.
type screenView func(row int) *rowView
