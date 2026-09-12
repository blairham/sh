// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"slices"
	"unicode"

	"github.com/blairham/sh/internal/fdset"
)

// The second state a line editor can be in.
//
// Everything else in this package is one state: a key either puts a character
// in the line or performs an action, and which it does is a property of the
// key. A command mode is the other arrangement — the same `w` is a character
// in one state and a motion in the other — and it is what `set -o vi` and
// `bindkey -v` have been asking for in both dialects while neither had it.
//
// **It is the editor's and not a shell's**, which is the rule widgets.go
// states and the reason this file is here rather than in either dialect. A
// motion is not a vocabulary: `dw` is one action made of two keystrokes, and
// the grammar that joins them — an operator, an optional count, a motion, and
// the *range* the motion names as opposed to the place it lands — is the same
// grammar whichever shell is asking for it. Written in one dialect it would be
// written twice, and this tree has been bitten by two copies of one rule five
// times; repl.DefaultBindings exists because it had already happened to the
// key table.
//
// ## What a motion answers
//
// Two things, and the whole grammar turns on their being different. A bare `w`
// leaves the cursor at the start of the next word; `dw` deletes from where the
// cursor was up to but not including it. A bare `e` leaves the cursor on the
// last character of the word; `de` deletes that character too. So a motion
// answers with a landing place *and* with a half-open range, and an operator
// reads the range. See viMove.
//
// Three cases make that more than bookkeeping, and each was measured rather
// than reasoned about:
//
//   - `dw` on the last word of the line deletes to the end of the line,
//     although a bare `w` there does not move at all.
//   - `cw` changes to the *end* of the word rather than to the start of the
//     next one, so it is `ce` and not `dw` with an insert after it. On the
//     last character of a line it changes that character alone.
//   - `x` at the end of a line leaves the cursor on the character before it,
//     because in command mode the cursor sits *on* a character rather than
//     between two. That is viClamp, and it is why every path here ends in one.
//
// ## Where the measurements came from
//
// A pseudo-terminal, one keystroke at a time, against bash 5.3.15, bash 3.2.57
// and zsh 5.9.2 on 2026-09-12, with no startup files and `set -o vi` or
// `bindkey -v` typed at the first prompt. The cursor was read back rather than
// guessed at: the keys under test, then `i` and a marker character, then
// Return, and `fc -ln -1` printed the line the shell had accepted — so the
// marker says exactly which character the cursor was on, and the line says
// exactly what the edit did. docs/spec/editing.md holds the table.
//
// The three shells agree about nearly all of it. Where two of them do not,
// editing.md says so and says which answer is here.

// viOutcome is what one command-mode key did to the read loop.
type viOutcome int

const (
	// viContinues is a key acted on, with the line still being edited.
	viContinues viOutcome = iota
	// viAccepted is Return: the line is finished.
	viAccepted
	// viAbandoned is ^C.
	viAbandoned
	// viStopped is the input ending part-way through a command.
	viStopped
)

// viFind is the last `f`, `F`, `t` or `T`, which `;` and `,` repeat.
type viFind struct {
	key  byte
	char rune
	made bool
}

// viEditing reports whether this session edits the vi way at all.
//
// Asked rather than held, because `set -o vi` and `bindkey -v` are commands a
// person runs at the prompt as much as ones an rc file runs — the reason
// KeyBindings is a function. A session whose front end has not said is a
// session with no command mode, which is every session in the two dialects
// that have no line editor and every session in the other two until somebody
// asks.
func (e *editor) viEditing() bool { return e.vi != nil && e.vi() }

// escapeIsTheModeSwitch reports whether an Escape that has just arrived should
// leave insert mode rather than begin a key sequence.
//
// **This is the one question a vi command mode asks that an emacs one does
// not, and both real shells answer it with a timer.** Escape is the mode
// switch and also the first byte of every arrow key, so something has to tell
// them apart. Measured under a pty: with `\e[D` typed as one burst both shells
// move the cursor left, and with a 1.2-second pause after the Escape both
// shells leave insert mode and then read `[` and `D` as two command-mode keys
// — `D` deleting to the end of the line. bash calls the wait `keyseq-timeout`
// and defaults it to half a second; zsh calls it `KEYTIMEOUT` and defaults it
// to four tenths.
//
// This asks whether a byte is *there*, and waits no time at all for one. A
// terminal writes an escape sequence in a single write, so the bytes after the
// Escape have already been delivered by the time this is asked; a person
// pressing Escape delivers one byte and nothing follows it. That needs no
// number to be invented — and there is no honest one to invent, since the two
// shells that have the wait disagree about its length. What it costs is a
// sequence split across two writes by something slow in between, which is what
// the shells' timers cover and this does not; #1427 filed it.
//
// Two sources, because input reaches this editor two ways and asking only the
// kernel would miss the commoner one: bytes already taken off the terminal sit
// in e.held, where fdset cannot see them — the same split inputPending exists
// for, and the same mistake serveDescriptors documents having made once.
func (e *editor) escapeIsTheModeSwitch() bool {
	if e.inputPending() {
		return false
	}
	if e.inFd == nil {
		return true
	}
	fd := e.inFd()
	if fd < 0 {
		return true
	}
	return !fdset.ReadableNow(fd)
}

// enterViCommand leaves insert mode.
//
// The cursor steps back one, which is measured in all three shells: with
// `echo onetwo` typed and Escape pressed, `i` then a marker puts the marker
// before the last character rather than after it. At the start of the line
// there is nowhere to step back to and it stays.
func (e *editor) enterViCommand(prompt drawnPrompt) {
	e.viCommand = true
	if e.pos > 0 {
		e.pos--
	}
	e.redraw(prompt)
}

// leaveViCommand enters insert mode with the cursor at `at`.
func (e *editor) leaveViCommand(at int, prompt drawnPrompt) {
	e.viCommand = false
	e.pos = min(max(at, 0), len(e.line))
	e.redraw(prompt)
}

// viClamp keeps the cursor on a character.
//
// In insert mode the cursor sits between characters and len(line) is a legal
// place for it — after the last one, which is where typing leaves it. In
// command mode it sits *on* a character, so the last legal place is one
// earlier and an empty line has only the one. Measured: `$` then `l` three
// times does not move, and `x` on the last character of a line leaves the
// cursor on the character now last.
func (e *editor) viClamp() {
	e.pos = min(max(e.pos, 0), max(len(e.line)-1, 0))
}

// firstNonBlank is where `^` goes.
func (e *editor) firstNonBlank() int {
	i := 0
	for i < len(e.line) && unicode.IsSpace(e.line[i]) {
		i++
	}
	return min(i, max(len(e.line)-1, 0))
}

// viKey acts on one key in command mode, reading whatever else the key needs.
//
// The whole command is read here rather than a byte at a time through the read
// loop, which is how `escape` and the `^X` prefix already work: a count, an
// operator, a second count and a motion are one keystroke as far as a person
// is concerned, and spreading them over the loop would put the grammar in the
// loop. ^C arriving part-way through is the rescue readByte already provides.
func (e *editor) viKey(first byte, prompt drawnPrompt) viOutcome {
	count, key, got := e.viCount(first)
	if out, done := viRead(got); done {
		return out
	}
	return e.viAct(count, key, prompt)
}

// viRead turns a failed read into an outcome.
func viRead(got keyRead) (viOutcome, bool) {
	switch got {
	case keyAbandoned:
		return viAbandoned, true
	case keyStopped:
		return viStopped, true
	default:
		return viContinues, false
	}
}

// viCount reads the digits in front of a command and then the command's own
// key.
//
// `0` is the motion to the start of the line and not a count, until a count
// has been begun by one of the other nine — measured, `10l` moves ten
// characters right. Zero is returned for no count at all, which every caller
// reads as one.
func (e *editor) viCount(first byte) (int, byte, keyRead) {
	if first < '1' || first > '9' {
		return 0, first, keyContinues
	}
	n := int(first - '0')
	for {
		b, got := e.readByte()
		if got != keyContinues {
			return 0, 0, got
		}
		if b < '0' || b > '9' {
			return n, b, keyContinues
		}
		if n < 1<<20 {
			// A count from a terminal, bounded so that a key held down cannot
			// make this grow without end. Past the cap the digits are read and
			// no longer counted, which is what maxParams does for a control
			// sequence.
			n = n*10 + int(b-'0')
		}
	}
}

// atLeastOne is what a missing count means.
func atLeastOne(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// viAct performs one command-mode key that is not a count.
func (e *editor) viAct(count int, key byte, prompt drawnPrompt) viOutcome {
	switch key {
	case '\r', '\n':
		// Return finishes the line from command mode as much as from insert
		// mode. Measured in all three.
		return viAccepted
	case ctrlC:
		return viAbandoned
	case esc:
		// Already in command mode. Measured: nothing happens, and in
		// particular the cursor does not step back a second time.
		return viContinues

	// Getting back to insert mode. These four are the vi-only actions both
	// shells name as widgets; see the Widget constants in widgets.go, which is
	// where the decision about naming them is written down.
	case 'i':
		e.leaveViCommand(e.pos, prompt)
	case 'a':
		e.leaveViCommand(e.pos+1, prompt)
	case 'I':
		e.leaveViCommand(e.viLineStart(), prompt)
	case 'A':
		e.leaveViCommand(len(e.line), prompt)

	// The edits that are their own key rather than an operator and a motion.
	case 'x':
		return e.viOperate('d', viMove{from: e.pos, end: min(e.pos+atLeastOne(count), len(e.line)), to: e.pos, ok: true}, prompt)
	case 'X':
		return e.viOperate('d', viMove{from: max(e.pos-atLeastOne(count), 0), end: e.pos, to: e.pos, ok: true}, prompt)
	case 'D':
		return e.viOperate('d', e.viWholeRest(), prompt)
	case 'C':
		return e.viOperate('c', e.viWholeRest(), prompt)
	case 'S':
		return e.viOperate('c', e.viWholeLine(), prompt)
	case 'r':
		return e.viReplace(atLeastOne(count), prompt)
	case '~':
		e.viToggleCase(atLeastOne(count), prompt)
	case 'p':
		e.viPut(e.pos+1, prompt)
	case 'P':
		e.viPut(e.pos, prompt)

	// Taking a change back. The same stack `^_` walks and the same answer
	// about where the cursor lands, which is a dialect's — see
	// EditorStyle.UndoRestoresTheCursorToWhereItWas, whose measurement in
	// emacs mode and this one in command mode agree.
	case 'u':
		e.undoLine()
		e.viClamp()
		e.redraw(prompt)

	// The history, which `k` and `j` walk the way the arrows do.
	case 'k':
		e.browse(-1, prompt)
		e.viClamp()
		e.redraw(prompt)
	case 'j':
		e.browse(+1, prompt)
		e.viClamp()
		e.redraw(prompt)

	case 'd', 'c', 'y':
		return e.viOperatorKey(key, count, prompt)

	default:
		m, got := e.viMotion(key, count)
		if out, done := viRead(got); done {
			return out
		}
		if !m.ok {
			// A key command mode does not act on. Nothing happens and nothing
			// is typed into the line — measured, `z` and `q` leave both shells
			// exactly as they were, which is the honest answer for a key
			// nobody bound and the one thing this mode must never get wrong.
			return viContinues
		}
		e.moveTo(m.to, prompt)
	}
	return viContinues
}

// viLineStart is where `I` puts the cursor, and the dialects disagree.
//
// Measured on `   ab` from the end of the line: bash inserts at column 0 and
// zsh inserts at the first character that is not a blank. See
// EditorStyle.ViInsertAtStartOfLineSkipsLeadingBlanks.
func (e *editor) viLineStart() int {
	if e.viInsertSkipsBlanks {
		return e.firstNonBlank()
	}
	return 0
}

// viWholeRest is the range `D` and `C` take: the cursor to the end of the
// line.
func (e *editor) viWholeRest() viMove {
	return viMove{from: e.pos, end: len(e.line), to: e.pos, ok: true}
}

// viWholeLine is the range `dd`, `cc` and `S` take.
func (e *editor) viWholeLine() viMove {
	return viMove{from: 0, end: len(e.line), to: 0, ok: true}
}

// viMove is one motion's answer.
//
// `to` is where a bare press of the motion leaves the cursor. `from` and `end`
// are the half-open range an operator in front of it takes, which is not the
// same thing: see the file comment, where the three cases that make them
// differ are measured.
type viMove struct {
	to        int
	from, end int
	ok        bool
}

// forward is the range a motion that went right names.
func (e *editor) forward(to int, inclusive bool) viMove {
	end := to
	if inclusive {
		end = min(to+1, len(e.line))
	}
	return viMove{to: min(to, max(len(e.line)-1, 0)), from: e.pos, end: max(end, e.pos), ok: true}
}

// backward is the range a motion that went left names.
func (e *editor) backward(to int) viMove {
	to = max(to, 0)
	return viMove{to: to, from: to, end: max(e.pos, to), ok: true}
}

// viMotion is where one motion key lands, and what an operator in front of it
// takes.
//
// A vocabulary of its own rather than Widgets, which is the decision
// widgets.go states: a motion is half an action, and a Widget is what a key
// does on its own.
func (e *editor) viMotion(key byte, count int) (viMove, keyRead) {
	n := atLeastOne(count)
	switch key {
	case 'h', backspace:
		return e.backward(e.pos - n), keyContinues
	case 'l', ' ':
		return e.forward(min(e.pos+n, len(e.line)), false), keyContinues
	case '0':
		return e.backward(0), keyContinues
	case '^':
		return e.viToward(e.firstNonBlank(), false), keyContinues
	case '$':
		return viMove{to: max(len(e.line)-1, 0), from: e.pos, end: len(e.line), ok: true}, keyContinues
	case '|':
		return e.viToward(min(n-1, max(len(e.line)-1, 0)), false), keyContinues
	case 'w', 'W':
		return e.viWordForward(n, key == 'W'), keyContinues
	case 'b', 'B':
		at := e.pos
		for range n {
			at = viBackwardWord(e.line, at, key == 'B')
		}
		return e.backward(at), keyContinues
	case 'e', 'E':
		at := e.pos
		for range n {
			at = viEndOfWord(e.line, at, key == 'E')
		}
		return e.forward(at, true), keyContinues
	case 'f', 'F', 't', 'T':
		c, got := e.viChar()
		if got != keyContinues {
			return viMove{}, got
		}
		e.find = viFind{key: key, char: c, made: true}
		return e.viFindMove(key, c, n), keyContinues
	case ';', ',':
		if !e.find.made {
			return viMove{}, keyContinues
		}
		k := e.find.key
		if key == ',' {
			k = viOppositeFind(k)
		}
		return e.viFindMove(k, e.find.char, n), keyContinues
	default:
		return viMove{}, keyContinues
	}
}

// viToward is a motion whose direction depends on where the cursor already is.
func (e *editor) viToward(to int, inclusive bool) viMove {
	if to >= e.pos {
		return e.forward(to, inclusive)
	}
	return e.backward(to)
}

// viWordForward is `w`, and it is the motion whose two answers differ most.
//
// Measured on `true alpha beta gamma` with the cursor on the last character:
// `w` does not move, and `dw` deletes that character — so with no next word to
// go to, an operator takes the rest of the line where the bare motion stays
// put. On `true   alpha` from the start, `dw` takes the three spaces with it.
func (e *editor) viWordForward(n int, big bool) viMove {
	at := e.pos
	for range n {
		at = viForwardWord(e.line, at, big)
	}
	if at >= len(e.line) {
		return viMove{to: max(len(e.line)-1, 0), from: e.pos, end: len(e.line), ok: true}
	}
	return e.forward(at, false)
}

// viFindMove is where `f`, `F`, `t` and `T` land.
//
// `f` and `t` are inclusive — `dfa` deletes the `a` it found — and `F` and `T`
// are not, because a backward range already ends before the cursor.
func (e *editor) viFindMove(key byte, c rune, n int) viMove {
	at := e.pos
	for range n {
		next, found := viFindChar(e.line, at, key, c)
		if !found {
			// Nothing to find. Measured, the whole command is abandoned and
			// the cursor does not move — and an operator in front of it takes
			// nothing.
			return viMove{}
		}
		at = next
	}
	switch key {
	case 'f':
		return e.forward(at, true)
	case 't':
		return e.forward(max(at-1, e.pos), true)
	case 'F':
		return e.backward(at)
	default:
		return e.backward(min(at+1, e.pos))
	}
}

// viFindChar is the next or previous occurrence of c.
func viFindChar(line []rune, from int, key byte, c rune) (int, bool) {
	switch key {
	case 'f', 't':
		for i := from + 1; i < len(line); i++ {
			if line[i] == c {
				return i, true
			}
		}
	default:
		for i := from - 1; i >= 0; i-- {
			if line[i] == c {
				return i, true
			}
		}
	}
	return 0, false
}

// viOppositeFind is what `,` turns the last find into.
//
// `;` repeats it as it was and `,` repeats it the other way. Both are measured
// for `f` and `F`; after a `t` the two shells part company about whether the
// repeat moves at all, and editing.md records which answer is here and why it
// is not a field.
func viOppositeFind(key byte) byte {
	switch key {
	case 'f':
		return 'F'
	case 'F':
		return 'f'
	case 't':
		return 'T'
	default:
		return 't'
	}
}

// viChar reads the character a key like `f` or `r` is waiting for.
//
// A rune and not a byte, for the reason readRune exists: a character outside
// ASCII arrives as several bytes and `f` on the first half of one would find
// nothing. Escape abandons the command, measured — `r` then Escape leaves the
// line exactly as it was.
func (e *editor) viChar() (rune, keyRead) {
	b, got := e.readByte()
	if got != keyContinues {
		return 0, got
	}
	if b == esc {
		return 0, keyStopped
	}
	r, err := e.readRune(b)
	if err != nil {
		return 0, keyStopped
	}
	return r, keyContinues
}

// viOperatorKey reads what follows `d`, `c` or `y` and performs it.
//
// The doubled letter is the whole line — `dd`, `cc`, `yy` — and a count may
// appear on either side of the operator, which multiply: `2dw` and `d2w` both
// take two words.
func (e *editor) viOperatorKey(op byte, count int, prompt drawnPrompt) viOutcome {
	b, got := e.readByte()
	if out, done := viRead(got); done {
		return out
	}
	second, key, got := e.viCount(b)
	if out, done := viRead(got); done {
		return out
	}
	if key == op {
		if op == 'y' {
			// `yy` and `Y` are a *line* in one shell and a span of characters
			// in the other, which shows up the moment `p` puts one back. Not
			// built rather than averaged; #1427 records it.
			return viContinues
		}
		return e.viOperate(op, e.viWholeLine(), prompt)
	}
	if op == 'c' {
		// `cw` is `ce`: measured on `true alpha` from the start, `cw` leaves
		// ` alpha` rather than `alpha`, so it changes to the end of the word
		// and not to the start of the next one.
		switch key {
		case 'w':
			key = 'e'
		case 'W':
			key = 'E'
		}
	}
	m, got := e.viMotion(key, atLeastOne(count)*atLeastOne(second))
	if out, done := viRead(got); done {
		return out
	}
	if !m.ok {
		// A key that is not a motion after an operator. Measured, `dz` and `d`
		// then Escape both leave the line alone.
		return viContinues
	}
	return e.viOperate(op, m, prompt)
}

// viOperate performs one operator over one range.
func (e *editor) viOperate(op byte, m viMove, prompt drawnPrompt) viOutcome {
	from, end := max(m.from, 0), min(m.end, len(e.line))
	if from >= end {
		if op == 'c' {
			// An empty change is still a change of mode, which is what `cw` on
			// an empty line does.
			e.leaveViCommand(from, prompt)
		}
		return viContinues
	}
	if op == 'y' {
		e.viTake(e.line[from:end])
		e.moveTo(from, prompt)
		return viContinues
	}
	e.change(false, func() {
		e.viTake(e.line[from:end])
		e.line = append(e.line[:from], e.line[end:]...)
		e.pos = from
	})
	if op == 'c' {
		e.leaveViCommand(from, prompt)
		return viContinues
	}
	e.viClamp()
	e.redraw(prompt)
	return viContinues
}

// viTake is what a delete or a yank leaves for `p` to put back.
//
// The same buffer `^Y` holds, so a word killed with `^W` is a word `p` puts
// back — and it outlives the line, measured: `x` on one line and `p` on the
// next puts the character back. Set outright rather than joined onto: joining
// consecutive kills is what `^K^K` means in emacs mode and there is no vi key
// that asks for it.
func (e *editor) viTake(text []rune) {
	e.killed = slices.Clone(text)
}

// viPut is `p` and `P`.
//
// The cursor lands on the last character put back, which is measured in all
// three shells: `x` then `p` swaps two characters and leaves the cursor on the
// second of them.
func (e *editor) viPut(at int, prompt drawnPrompt) {
	if len(e.killed) == 0 {
		return
	}
	at = min(max(at, 0), len(e.line))
	e.change(false, func() {
		line := make([]rune, 0, len(e.line)+len(e.killed))
		line = append(line, e.line[:at]...)
		line = append(line, e.killed...)
		line = append(line, e.line[at:]...)
		e.line = line
		e.pos = at + len(e.killed) - 1
	})
	e.viClamp()
	e.redraw(prompt)
}

// viReplace is `r`: the next character typed replaces the one under the
// cursor, and the count replaces that many.
//
// With fewer characters left than the count, nothing happens at all — the
// command needs the whole run to replace and vi does not replace part of it.
func (e *editor) viReplace(count int, prompt drawnPrompt) viOutcome {
	c, got := e.viChar()
	if got == keyAbandoned {
		return viAbandoned
	}
	if got != keyContinues {
		// Escape, or the input ending. Either way the line is untouched;
		// Escape is measured as abandoning the command rather than replacing
		// with an Escape.
		return viContinues
	}
	if e.pos+count > len(e.line) {
		return viContinues
	}
	e.change(false, func() {
		for i := range count {
			e.line[e.pos+i] = c
		}
		e.pos += count - 1
	})
	e.viClamp()
	e.redraw(prompt)
	return viContinues
}

// viToggleCase is `~`: the character under the cursor changes case and the
// cursor steps past it, count times.
func (e *editor) viToggleCase(count int, prompt drawnPrompt) {
	if e.pos >= len(e.line) {
		return
	}
	e.change(false, func() {
		for i := 0; i < count && e.pos < len(e.line); i++ {
			e.line[e.pos] = viSwapCase(e.line[e.pos])
			e.pos++
		}
	})
	e.viClamp()
	e.redraw(prompt)
}

// viSwapCase turns one case into the other and leaves everything else alone.
func viSwapCase(r rune) rune {
	switch {
	case unicode.IsUpper(r):
		return unicode.ToLower(r)
	case unicode.IsLower(r):
		return unicode.ToUpper(r)
	default:
		return r
	}
}

// What a word is here, and it is not what a word is anywhere else in this
// package.
//
// The editor's other word keys — `M-b`, `M-f`, `^W` — ask a *dialect* what
// counts as a word, because the two shells answer differently and one of them
// answers differently for `^W` than for `M-b` (see words.go and EditorStyle).
// The vi motions ask nobody: measured on `true a-b.c def`, `w` from the start
// of `a-b.c` lands on the `-` in all three shells, and `_` and the digits stay
// inside a word while `$` and `/` do not. Three runs of one kind: blanks, the
// letters and digits and `_`, and everything else. A shell's WORDCHARS does
// not reach it.
const (
	viBlank = iota
	viWord
	viPunct
)

// viClassOf is which of the three runs a character belongs to. `big` is the
// upper-case motions, for which there are only two: blank and not.
func viClassOf(r rune, big bool) int {
	switch {
	case unicode.IsSpace(r):
		return viBlank
	case big:
		return viWord
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return viWord
	default:
		return viPunct
	}
}

// viForwardWord is the start of the next word, or the end of the line where
// there is none.
func viForwardWord(line []rune, i int, big bool) int {
	n := len(line)
	if i >= n {
		return n
	}
	if c := viClassOf(line[i], big); c != viBlank {
		for i < n && viClassOf(line[i], big) == c {
			i++
		}
	}
	for i < n && viClassOf(line[i], big) == viBlank {
		i++
	}
	return i
}

// viBackwardWord is the start of the word before the cursor.
func viBackwardWord(line []rune, i int, big bool) int {
	i--
	for i >= 0 && viClassOf(line[i], big) == viBlank {
		i--
	}
	if i < 0 {
		return 0
	}
	c := viClassOf(line[i], big)
	for i > 0 && viClassOf(line[i-1], big) == c {
		i--
	}
	return i
}

// viEndOfWord is the last character of the word the cursor is in or of the
// next one.
func viEndOfWord(line []rune, i int, big bool) int {
	n := len(line)
	if n == 0 {
		return 0
	}
	i++
	for i < n && viClassOf(line[i], big) == viBlank {
		i++
	}
	if i >= n {
		return n - 1
	}
	c := viClassOf(line[i], big)
	for i+1 < n && viClassOf(line[i+1], big) == c {
		i++
	}
	return i
}
