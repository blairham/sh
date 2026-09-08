// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// Reading a line the way a shell does.
//
// The terminal is in raw mode, so the bytes arrive as they are typed and
// nothing is echoed: this draws the line itself. That is not decoration. The
// moment the cursor can move, the terminal's own echo is wrong — it would
// print an arrow key rather than move — so a shell that offers editing at all
// has to own what is on the screen.
//
// Runes rather than bytes throughout, because a cursor sits between characters
// and not between the halves of one.

// ErrInterrupted is a line the user abandoned with ^C. The line is discarded
// and the prompt comes back; it is not an error the shell should report.
var ErrInterrupted = errors.New("interrupted")

// editor holds the line being typed and the history behind it.
type editor struct {
	in  io.Reader
	out io.Writer

	// line is what has been typed, and pos is where the cursor sits in it —
	// an index *between* runes, so 0 is before the first and len(line) is
	// after the last.
	line []rune
	pos  int

	// history is every line accepted so far, oldest first. browsing is where
	// Up and Down have walked to: len(history) means "not browsing, on the
	// line being typed".
	history  []string
	browsing int

	// drafts is what the walk has left behind, keyed by the index it was left
	// at — with len(history) standing for the line that was being typed.
	//
	// Because an entry recalled and then edited keeps the edit for as long as
	// the line lasts. Measured on 2026-09-05 in bash 5.3.15 and zsh 5.9.2
	// alike: recall `ls -la`, type `XX`, press Up and then Down, and `ls
	// -laXX` comes back. Accepting or abandoning the line throws all of them
	// away, which is why they live here and not in history — the entry itself
	// is never edited, and the next line starts from the file again.
	drafts map[int][]rune

	// comp answers what a prefix could become, and lastTab says the previous
	// keystroke was already a Tab — which is what makes the second one list
	// the matches rather than repeat a completion that changed nothing.
	comp    Completer
	lastTab bool

	// workingDir is the shell's own directory, asked when a completion is
	// built rather than held, because `cd` moves it under the editor. Nil is
	// a session with nothing to ask — the editor is usable without a Runner.
	workingDir func() string

	// highlighter colors the line as it is typed, and nil draws it plainly —
	// which is what every real shell does. Asked on every redraw; see
	// highlight.go for why that is affordable and why it is never a plugin.
	highlighter Highlighter

	// interrupt is what marks a line abandoned with ^C. Empty draws nothing,
	// which is what two of the four dialects do.
	interrupt string

	// killed is what the last kill took off the line, and ^Y puts it back.
	// Kills that follow one another go into it together, which is what
	// killing and killedBefore keep track of. It outlives the line: measured,
	// a word killed on one line yanks back on the next.
	killed                []rune
	killing, killedBefore bool

	// changes is the line as it was before each change, oldest first, and
	// `^_` walks back through it. typing and typedBefore say whether this
	// keystroke and the one before it were characters typed into the line,
	// which is what decides whether a dialect that groups a run of typing
	// puts another entry on the stack. See undo.go.
	changes              []snapshot
	typing, typedBefore  bool
	undoPerKeystroke     bool
	undoRestoresCursor   bool
	lastArg              lastArgWalk
	lastArgStaysOnOldest bool

	// What this dialect calls a word, and what its kills do with one. See
	// EditorStyle, which is where each of these was measured.
	wordChars                  string
	wholeLineKill              bool
	killBeforeCursorUsesWords  bool
	forwardWordStopsBeforeNext bool
	transposeAtStart           bool

	// What a reverse incremental search is drawn as, and where. See
	// HistoryStyle.
	searchPrompt string
	searchFailed string
	searchBelow  bool

	// pushed is the byte the search mode read and did not want, kept for the
	// next turn of the loop below. See pushBack.
	pushed    byte
	hasPushed bool

	// What to ask before printing a large listing, and how to read the
	// answer. See EditorStyle.
	listQuery       string
	listQueryEchoes bool
	listQueryStrict bool

	// bindings is what a person rebound, asked fresh for every key because
	// `bindkey` is a command run at the prompt as well as in an rc file. Nil,
	// or an empty table, is a session where nothing was rebound and every key
	// reaches the dispatch below. See bindings.go.
	bindings func() map[string]Binding

	// runFunc runs one of the shell's own actions over the line, where the
	// front end gave this session a way to. Nil is a session with no such
	// way, and a key bound to one then does nothing — see
	// runShellWidget.
	runFunc func(name string, in Line) (Line, bool)

	// watch is which descriptors the shell wants waited on beside the
	// terminal, descriptorReady is how one that woke is answered, and inFd is
	// the descriptor a key arrives on — negative where the session's input is
	// not one. Nil watch or nil descriptorReady is a session that waits on
	// the terminal alone. See watchfd.go.
	watch           func() []int
	descriptorReady func(fd int, in Line) (Line, bool)
	inFd            func() int

	// width is how many columns the terminal has, asked each time it is
	// needed; nil, or an answer of 0, means it will not say. row is which
	// screen row the last draw left the cursor on, counted from the row the
	// prompt starts in — 0 until the line is long enough to wrap.
	width func() int
	row   int
}

// readLine reads one line, drawing it as it is typed.
//
// It returns io.EOF for ^D on an empty line — which is how a shell is told to
// exit — and ErrInterrupted for ^C, which abandons the line without exiting.
func (e *editor) readLine(prompt drawnPrompt) (string, error) {
	e.line, e.pos = e.line[:0], 0
	e.browsing = len(e.history)
	e.row = 0
	// Fresh for every line: an edit made to a recalled entry lasts as long as
	// the line does and no longer, which is what both shells do — see drafts.
	e.drafts = map[int][]rune{}
	// Nothing to take back yet. Measured, `^_` at a fresh prompt does nothing
	// in both shells however much was edited on the line before it, so the
	// stack belongs to the line rather than to the session.
	e.changes = nil
	e.lastArg = lastArgWalk{}
	e.write(prompt.text)

	var buf [1]byte
	for {
		// Whatever the shell asked to be told about goes first, because this
		// is where a session waits and there is nowhere else to notice from.
		// Here rather than inside nextByte, and that is the load-bearing
		// half: nextByte is also how the *rest* of a key sequence arrives —
		// see readByte — and a handler that printed between ESC and the byte
		// after it would print into the middle of a keystroke. Only the wait
		// for a key's *first* byte is an idle moment. It costs nothing in a
		// session with no descriptor armed, which is every session that has
		// not asked; see watchfd.go.
		e.serveDescriptors(prompt)
		n, err := e.nextByte(buf[:])
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		wasTab := e.lastTab
		e.lastTab = false
		// Whether the keystroke before this one was a kill, which is what
		// decides between joining onto what ^Y holds and replacing it.
		//
		// Nothing carries this across an accepted line: only a kill sets it,
		// no kill returns from here, and the next keystroke read is the next
		// line's first. So the text a kill took survives the line it came off
		// — measured — and the joining does not.
		e.killedBefore, e.killing = e.killing, false
		// The same for whether the keystroke before this one typed a
		// character into the line, and whether it was `M-.`. Both decide what
		// this keystroke does rather than what it is: a run of typing is one
		// change to undo, and a second `M-.` walks the history rather than
		// inserting a second copy.
		e.typedBefore, e.typing = e.typing, false
		e.lastArg.walkingBefore, e.lastArg.walking = e.lastArg.walking, false
		// What a person rebound comes first, and only for a byte that starts
		// something they bound: a session with no bindings reaches the switch
		// having asked one question of an empty map. It is after the bookkeeping
		// above and not before it, because those two lines are about the
		// keystroke that came *last* and hold whatever this one turns out to be.
		// See bindings.go.
		switch b, claimed, got := e.matchBinding(buf[0]); {
		case got == keyAbandoned:
			return e.abandon(prompt)
		case claimed && b.Function != "":
			// An action of the shell's own rather than one of this editor's.
			// See shellwidget.go.
			e.runShellWidget(b.Function, prompt)
			continue
		case claimed:
			e.runWidget(b.Widget, prompt)
			continue
		}
		switch c := buf[0]; c {
		case ctrlC:
			return e.abandon(prompt)
		case ctrlD:
			if len(e.line) == 0 {
				e.endLine(prompt, "")
				return "", io.EOF
			}
			// With something typed, ^D deletes forwards instead — which is
			// what it means everywhere but on an empty line.
			e.change(false, e.deleteForward)
		case '\r', '\n':
			e.endLine(prompt, "")
			return string(e.line), nil
		case ctrlA:
			e.moveTo(0, prompt)
		case ctrlE:
			e.moveTo(len(e.line), prompt)
		case ctrlB:
			e.moveTo(e.pos-1, prompt)
		case ctrlF:
			e.moveTo(e.pos+1, prompt)
		case ctrlK:
			e.change(false, func() { e.killForwardTo(len(e.line)) })
			e.redraw(prompt)
		case ctrlU:
			e.change(false, e.killToStart)
			e.redraw(prompt)
		case ctrlW:
			e.change(false, func() { e.killTo(e.wordStartBeforeCursor()) })
			e.redraw(prompt)
		case ctrlY:
			e.change(false, e.yank)
			e.redraw(prompt)
		case ctrlT:
			e.change(false, e.transpose)
			e.redraw(prompt)
		case ctrlUnderscore:
			e.undoLine()
			e.redraw(prompt)
		case ctrlL:
			// Clear the screen and put the line back at the top of it.
			e.write("\x1b[H\x1b[2J")
			e.row = 0
			e.redraw(prompt)
		case ctrlP:
			e.browse(-1, prompt)
		case ctrlN:
			e.browse(+1, prompt)
		case ctrlR:
			e.reverseSearch(prompt)
		case backspace, del:
			e.change(false, e.deleteBackward)
			e.redraw(prompt)
		case tab:
			var matches []string
			e.change(false, func() { matches = e.complete(e.comp) })
			if len(matches) > 0 && wasTab && e.confirmList(matches, prompt) {
				e.list(matches, prompt)
			}
			e.redraw(prompt)
			// Set after the redraw, and the only key that leaves it set: two
			// Tabs in a row are a request to see the matches, and anything
			// between them is not.
			e.lastTab = true
			continue
		case esc:
			if e.escape(prompt) == keyAbandoned {
				return e.abandon(prompt)
			}
		case ctrlX:
			// The other prefix, and the only sequence behind it that this
			// acts on is `^X^U`, which is a second spelling of `^_`. The byte
			// after it is read whatever it is, for the same reason an escape
			// sequence is read to its end: measured, `^X` then `q` puts
			// nothing in the line in either shell, and a prefix that gave the
			// byte back would type the `q`.
			b, got := e.readByte()
			switch {
			case got == keyAbandoned:
				return e.abandon(prompt)
			case got != keyContinues:
			case b == ctrlU:
				e.undoLine()
				e.redraw(prompt)
			}
		default:
			if c < 0x20 {
				// Any other control character is ignored rather than
				// inserted: a shell that put a raw byte in the line would
				// hand the parser something no one typed.
				continue
			}
			r, err := e.readRune(c)
			if err != nil {
				return "", err
			}
			e.change(e.typedBefore, func() { e.insert(r) })
			e.typing = true
			e.redraw(prompt)
		}
	}
}

// nextByte is the next byte of input, which is whatever the search mode handed
// back before the byte the terminal has.
//
// The whole of the pushback: a mode that ends on a key which still means
// something has to leave that key where the ordinary switch will see it, and
// there is never more than one of them.
func (e *editor) nextByte(buf []byte) (int, error) {
	if e.hasPushed {
		buf[0], e.hasPushed = e.pushed, false
		return 1, nil
	}
	return e.in.Read(buf)
}

// abandon ends a line the person gave up on with ^C.
//
// The line is discarded, not run. The newline is ours to print: the terminal
// echoes nothing in raw mode, so without it the next prompt would land on top
// of what was typed.
func (e *editor) abandon(prompt drawnPrompt) (string, error) {
	e.endLine(prompt, e.interrupt)
	return "", ErrInterrupted
}

// readRune completes a UTF-8 sequence whose first byte has arrived.
//
// The terminal delivers bytes, and a character outside ASCII arrives as
// several. Inserting them one at a time would put half a rune in the line and
// draw a cursor between the halves.
func (e *editor) readRune(first byte) (rune, error) {
	buf := []byte{first}
	for !utf8.FullRune(buf) && len(buf) < utf8.UTFMax {
		var next [1]byte
		n, err := e.in.Read(next[:])
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		buf = append(buf, next[0])
	}
	r, _ := utf8.DecodeRune(buf)
	return r, nil
}

// browse walks the history. -1 is older, +1 is newer.
//
// Whatever is on the screen is left behind at the step it was on, so nothing
// the walk passes over is lost: the half-written line comes back when the walk
// reaches the end again, and an entry that was recalled and then edited comes
// back edited. Measured — both shells do the second as well as the first, and
// doing only the first makes a typo fixed on the way past a reason to retype
// the whole command.
func (e *editor) browse(dir int, prompt drawnPrompt) {
	if len(e.history) == 0 {
		return
	}
	to := e.browsing + dir
	if to < 0 || to > len(e.history) {
		return
	}
	// Copied rather than kept by reference, and no test can tell: every path
	// out of here reassigns e.line, so the array just stored is never written
	// through again. Left as a copy because the map outlives the assignment
	// and insert grows the line in place — a later branch that kept e.line
	// where it was would hand the map a live buffer, and that failure would
	// show up as history entries changing under the walk rather than as
	// anything a reader would look for here.
	e.drafts[e.browsing] = append([]rune(nil), e.line...)
	e.browsing = to
	if draft, kept := e.drafts[to]; kept {
		e.line = append([]rune(nil), draft...)
	} else {
		e.line = []rune(e.history[to])
	}
	e.pos = len(e.line)
	e.redraw(prompt)
}

// remember adds an accepted line to the history.
//
// A blank line is not kept, which every shell in the panel agrees about: a
// bare newline at the prompt is nothing happening.
//
// An immediate repeat *is* kept, which is measured rather than obvious.
// `echo a` twice in a row leaves two entries in bash 5.3.15 and in zsh 5.9.2
// alike, because dropping the second is what `HISTCONTROL=ignoredups` and
// `setopt HIST_IGNORE_DUPS` are for and neither is on by default. Doing it
// unconditionally here made the knob unobservable and the default wrong; see
// historyRules.
func (e *editor) remember(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	e.history = append(e.history, line)
}

// newest is the last line remembered, or nothing at all.
//
// What "a duplicate" is measured against, and it is the list rather than the
// file: in the dialect that keeps ignored lines, an ignored line is the one a
// duplicate is compared with, and in the dialect that drops them it is the
// last one kept. Reading the list gets both without asking which dialect this
// is.
func (e *editor) newest() string {
	if len(e.history) == 0 {
		return ""
	}
	return e.history[len(e.history)-1]
}

func (e *editor) insert(r rune) {
	e.line = append(e.line, 0)
	copy(e.line[e.pos+1:], e.line[e.pos:])
	e.line[e.pos] = r
	e.pos++
}

func (e *editor) deleteBackward() {
	if e.pos == 0 {
		return
	}
	e.line = append(e.line[:e.pos-1], e.line[e.pos:]...)
	e.pos--
}

func (e *editor) deleteForward() {
	if e.pos >= len(e.line) {
		return
	}
	e.line = append(e.line[:e.pos], e.line[e.pos+1:]...)
}

func (e *editor) moveTo(pos int, prompt drawnPrompt) {
	if pos < 0 || pos > len(e.line) {
		return
	}
	e.pos = pos
	e.redraw(prompt)
}

// redraw puts the whole line back on the screen.
//
// The whole line each time rather than the difference. A shell that tracked
// what changed would be faster and would be wrong the first time a character
// is wider than one cell or the line wraps; and at typing speed there is
// nothing to gain.
func (e *editor) redraw(prompt drawnPrompt) {
	cols := e.cols()
	if cols <= 0 {
		// Nothing known about the terminal, so the line is assumed to fit on
		// the row it started on. Wrong for a long line, and the best that can
		// be done without a width: guessing one would be wrong for every
		// line rather than only the long ones.
		var b strings.Builder
		b.WriteString("\r\x1b[K")
		b.WriteString(prompt.text)
		b.WriteString(e.styled())
		// Cells to come back over, not characters: the cursor moves by
		// columns, and one `日` to the right of it is two of them.
		if back := cells(e.line[e.pos:]); back > 0 {
			b.WriteString("\x1b[")
			b.WriteString(itoa(back))
			b.WriteString("D")
		}
		e.row = 0
		e.write(b.String())
		return
	}

	// A line wider than the terminal occupies several screen rows, and the
	// cursor is somewhere among them. `\r` returns to the start of the row it
	// is on and not to the start of the line, so getting back to the prompt
	// means going up as far as the last draw came down.
	var b strings.Builder
	b.WriteString("\r")
	if e.row > 0 {
		b.WriteString("\x1b[")
		b.WriteString(itoa(e.row))
		b.WriteString("A")
	}
	// Erase to the end of the *screen* rather than the end of the row: what
	// is being replaced may be several rows of it, and clearing only the
	// first leaves the rest of the old line below the new one.
	b.WriteString("\x1b[J")
	b.WriteString(prompt.text)
	b.WriteString(e.styled())

	curRow, curCol, endRow, endCol := place(prompt.cells, e.line, e.pos, cols)
	if endCol == cols {
		// The line ends exactly at the right-hand edge. A terminal does not
		// move to the next row until there is something to put there, so the
		// cursor is still on the old row and every count from here would be
		// one row out. A space makes it wrap, and the carriage return undoes
		// the space.
		b.WriteString(" \r")
		endRow++
	}
	if curCol == cols {
		// The cursor is at the edge with more line after it, so the terminal
		// has already wrapped and it is at the start of the next row.
		curRow, curCol = curRow+1, 0
	}
	if endRow > curRow {
		b.WriteString("\x1b[")
		b.WriteString(itoa(endRow - curRow))
		b.WriteString("A")
	}
	b.WriteString("\r")
	if curCol > 0 {
		b.WriteString("\x1b[")
		b.WriteString(itoa(curCol))
		b.WriteString("C")
	}
	e.row = curRow
	e.write(b.String())
}

// cols is the terminal's width, or 0 when there is nothing to ask.
func (e *editor) cols() int {
	if e.width == nil {
		return 0
	}
	return e.width()
}

// toLastRow puts the cursor below everything drawn, so what comes next starts
// on a clean row.
//
// The cursor sits wherever it was left, which for a wrapped line is usually
// not the last row of it. A newline from there scrolls the rest of the line
// out of the way of nothing and the next thing printed lands on top of it.
func (e *editor) toLastRow(prompt drawnPrompt) {
	cols := e.cols()
	if cols <= 0 {
		return
	}
	_, _, endRow, endCol := place(prompt.cells, e.line, e.pos, cols)
	if endCol == cols {
		// Sitting at the right-hand edge with the wrap still pending is being
		// on the row already, not below it.
		endRow++
	}
	if endRow > e.row {
		e.write("\x1b[" + itoa(endRow-e.row) + "B")
	}
}

// endLine finishes the line on the screen: down past the last row of it, then
// a newline, and the next draw starts from the top again.
func (e *editor) endLine(prompt drawnPrompt, before string) {
	e.toLastRow(prompt)
	e.write(before + "\r\n")
	e.row = 0
}

// displayWidth is how many columns a string takes on the screen.
//
// Escape sequences are skipped: they instruct the terminal rather than
// putting anything in a cell, and counting them would push every calculation
// here to the right by however many bytes it took to say "in green". One cell
// per rune otherwise — which is not true of a character the terminal draws
// double width, and is the remaining gap in this arithmetic.
func displayWidth(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == esc {
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && (s[i] < '@' || s[i] > '~') {
					i++
				}
			}
			if i < len(s) {
				i++
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		// Cells, not runes: a terminal draws `日` in two and a combining
		// mark in none, and every calculation built on this — which row the
		// cursor lands on, which row a redraw comes back up to, whether the
		// line wrapped at all — is wrong by however many of those are to the
		// left of it.
		n += runeWidth(r)
	}
	return n
}

// cells is how many columns the line takes on the screen.
//
// Separate from displayWidth because a line carries no escape sequences — the
// editor never puts a control character in it — so there is nothing to skip,
// and this runs on every keystroke.
func cells(rs []rune) int {
	n := 0
	for _, r := range rs {
		n += runeWidth(r)
	}
	return n
}

// place reports where the cursor and the end of the line land on the screen,
// counted in rows from the row the prompt starts on.
//
// It walks the line rather than dividing its width by the terminal's, and the
// reason is the wide characters: a terminal will not split `日` across the
// right-hand edge, so it wraps early and leaves the last cell of that row
// blank. Dividing counts that blank cell as used, and from the first wrapped
// line onwards every row this returns is wrong — the redraw comes back up to
// the wrong row and paints the prompt into the middle of the line.
//
// A column of `cols` is the cursor at the edge with the wrap still pending: a
// terminal stays on the row it filled until there is another character to put
// somewhere. The caller decides what to do about it, because the answer
// differs between the end of the line and the cursor.
//
// The cursor is recorded before the character it sits in front of is laid
// down, which is where writing the line up to that point would leave a
// terminal. In front of a wide character the wrap skipped over, that is the
// blank cell at the end of the row above rather than the character itself.
func place(promptWidth int, line []rune, pos, cols int) (curRow, curCol, endRow, endCol int) {
	row, col := promptWidth/cols, promptWidth%cols
	for i, r := range line {
		if i == pos {
			curRow, curCol = row, col
		}
		w := runeWidth(r)
		if col+w > cols {
			row, col = row+1, 0
		}
		col += w
		if col > cols {
			// A character wider than the whole terminal. Nothing sensible is
			// on the screen at that point; keeping the count inside the row
			// at least keeps the arithmetic after it honest.
			col = cols
		}
	}
	if pos >= len(line) {
		curRow, curCol = row, col
	}
	return curRow, curCol, row, col
}

// list prints the matches above the line, the way a shell does — the line is
// then drawn again below them, because it is still being typed.
//
// In columns, filled downwards. The note here used to say one per line was
// honest and never wrong, because columns need the terminal's width and
// asking for it meant an ioctl and a resize signal to keep it current. The
// width arrived with the wrapped redraw and needs neither, and one per line
// was never what a shell does: a directory of a hundred files became a
// hundred rows and scrolled the prompt away.
//
// Measured, bash and zsh lay them out the same way — as many columns as fit,
// each as wide as the longest match plus two, filled down one column before
// starting the next, so that reading in sorted order means reading downwards.
func (e *editor) list(matches []string, prompt drawnPrompt) {
	e.endLine(prompt, "")
	for _, row := range columns(matches, e.cols()) {
		e.write(row)
		e.write("\r\n")
	}
	// The prompt and the line are not written back here: the caller redraws,
	// and the redraw now knows it is starting from a fresh row.
}

// columns arranges the matches into the rows to print.
//
// Down each column rather than across each row: sorted matches read in order
// down the first column, then the second. Both shells with a line editor do
// it this way, and reading across would put `b` beside `a` and `z` below it.
//
// A width of zero is a terminal that will not say how wide it is, and one
// match per row is the only arrangement that cannot be wrong on it.
func columns(matches []string, width int) []string {
	if len(matches) == 0 {
		return nil
	}
	widest := 0
	for _, m := range matches {
		if w := displayWidth(m); w > widest {
			widest = w
		}
	}
	// Two spaces between columns, which is what both draw.
	cell := widest + 2
	perRow := 1
	if width > 0 {
		perRow = width / cell
	}
	if perRow < 1 {
		// Wider than the screen: one to a row, and it wraps rather than
		// being cut.
		perRow = 1
	}
	rows := (len(matches) + perRow - 1) / perRow
	out := make([]string, 0, rows)
	for r := range rows {
		var b strings.Builder
		for c := range perRow {
			i := c*rows + r
			if i >= len(matches) {
				break
			}
			b.WriteString(matches[i])
			// No padding after the last one on a row: trailing spaces are
			// invisible until something copies them.
			if i+rows < len(matches) {
				for n := displayWidth(matches[i]); n < cell; n++ {
					b.WriteString(" ")
				}
			}
		}
		out = append(out, b.String())
	}
	return out
}

func (e *editor) write(s string) { _, _ = io.WriteString(e.out, s) }

// itoa without importing strconv for one call on the keystroke path.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// The control bytes this editor acts on, named so the switch above reads as
// what it does rather than as a table of numbers.
const (
	ctrlA     = 0x01
	ctrlB     = 0x02
	ctrlC     = 0x03
	ctrlD     = 0x04
	ctrlE     = 0x05
	ctrlF     = 0x06
	ctrlG     = 0x07
	ctrlK     = 0x0b
	ctrlL     = 0x0c
	ctrlN     = 0x0e
	ctrlP     = 0x10
	ctrlR     = 0x12
	ctrlT     = 0x14
	ctrlU     = 0x15
	ctrlW     = 0x17
	ctrlX     = 0x18
	ctrlY     = 0x19
	tab       = 0x09
	esc       = 0x1b
	backspace = 0x08
	del       = 0x7f
	// `^_` is what a terminal sends for Ctrl and the underscore key, which on
	// most layouts is Ctrl-Shift-minus. `^X^U` is the same command spelled
	// with two keystrokes, for a keyboard where the first is awkward.
	ctrlUnderscore = 0x1f
)
