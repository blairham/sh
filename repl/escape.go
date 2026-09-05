// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// What arrives after an ESC, and why the whole of it has to be read.
//
// A terminal names its arrows, Home, End and Delete with a sequence of bytes,
// and there is no one spelling: the same key is `\e[H` on one terminal, `\eOH`
// on the same terminal in application cursor mode, and `\e[1~` or `\e[7~` on
// others. An editor that recognizes some spellings and abandons the rest
// half-read does something worse than ignoring the key — the bytes it did not
// read arrive at the next Read and are typed into the line. Home becomes a
// stray `~`, and Ctrl-Right becomes `;5C` in the middle of a command.
//
// That is not hypothetical, and it is not only ours. Measured under a pty:
// real zsh 5.9 with no startup files puts `~` in the line for `\e[1~` and
// `;5C` for `\e[1;5C`; real bash 5.3 handles most of them and still leaves a
// `~` behind for `\e[4~`, `\e[7~` and `\e[8~`, and a `C` for `\e[1;2C`. Both
// are keymaps looking a spelling up and giving the rest of it back to the
// reader. Reading the sequence by its *shape* instead — parameters, then
// intermediates, then one final byte, which is how ECMA-48 says a control
// sequence is built — means an unknown key is dropped whole and can never be
// typed into the line.

// escape reads and acts on the key that follows an ESC.
func (e *editor) escape(prompt drawnPrompt) keyRead {
	b, got := e.readByte()
	if got != keyContinues {
		return got
	}
	switch b {
	case '[':
		return e.controlSequence(prompt)
	case 'O':
		// SS3. A terminal in application cursor mode sends the arrows and
		// Home and End this way, and it is the mode a full-screen program
		// leaves behind, so a shell sees it constantly. One byte names the
		// key and there are no parameters.
		f, got := e.readByte()
		if got != keyContinues {
			return got
		}
		e.namedKey(f, 0, prompt)
	case 'b', 'B':
		e.moveTo(e.backwardWord(), prompt)
	case 'f', 'F':
		e.moveTo(e.forwardWord(), prompt)
	case 'd', 'D':
		e.change(false, func() { e.killForwardTo(e.endOfWord()) })
		e.redraw(prompt)
	case '.', '_':
		// `M-.` and `M-_`, which are the same key in both shells: the last
		// argument of the line before. See lastarg.go.
		e.insertLastArg(prompt)
	case del, backspace:
		// M-Delete kills the word before the cursor. Both spellings, because
		// a terminal sends whichever of the two its erase character is, and
		// both shells act on both.
		e.change(false, func() { e.killTo(e.backwardWord()) })
		e.redraw(prompt)
	}
	// Anything else is a key this does not act on, and the ESC and the byte
	// after it are dropped together. Measured: both shells do nothing at all
	// for `M-z`, and neither puts the `z` in the line.
	return keyContinues
}

// controlSequence reads a CSI — everything after `\e[` — and acts on it.
//
// The shape is fixed even when the meaning is unknown: any number of parameter
// bytes, any number of intermediate bytes, then exactly one final byte. Reading
// to the final byte is the whole point; the switch afterwards is allowed to
// recognize nothing.
func (e *editor) controlSequence(prompt drawnPrompt) keyRead {
	var params [maxParams]byte
	kept := 0
	b, got := e.readByte()
	for got == keyContinues && b >= 0x30 && b <= 0x3f {
		if kept < len(params) {
			// A fixed room for them rather than a growing one: the bytes come
			// from a terminal, and a sequence longer than this is not a
			// keystroke. Past the cap they are still read — reaching the final
			// byte is the point — and no longer kept.
			params[kept] = b
			kept++
		}
		b, got = e.readByte()
	}
	for got == keyContinues && b >= 0x20 && b <= 0x2f {
		b, got = e.readByte()
	}
	if got != keyContinues {
		return got
	}
	if b < 0x40 || b > 0x7e {
		return keyContinues
	}
	first, modifier := parameters(params[:kept])
	if b == 'M' && kept == 0 {
		// A mouse click, in the encoding xterm sent before it had a better
		// one: three bytes of button and coordinates after the final byte,
		// which is the one input sequence that is not shaped like a control
		// sequence. They have to be counted off by hand or they are typed
		// into the line — and a terminal left reporting the mouse by a
		// full-screen program that did not turn it off again is not rare.
		// The newer encoding puts its numbers in the parameters, so it is
		// already read by the loop above; this is the one without them.
		for range 3 {
			if _, got := e.readByte(); got != keyContinues {
				return got
			}
		}
		return keyContinues
	}
	if b == '~' {
		// The keypad and editing keys, which name themselves with a number.
		// Two spellings of Home and two of End, because terminals disagree:
		// `\e[1~` and `\e[7~` are both Home, `\e[4~` and `\e[8~` are both End.
		switch first {
		case 1, 7:
			e.moveTo(0, prompt)
		case 4, 8:
			e.moveTo(len(e.line), prompt)
		case 3:
			e.change(false, e.deleteForward)
			e.redraw(prompt)
		}
		return keyContinues
	}
	e.namedKey(b, modifier, prompt)
	return keyContinues
}

// namedKey acts on a key whose final byte names it — the arrows, Home and End,
// however they were spelled.
//
// The modifier is what a terminal puts after the semicolon, one more than a
// bitmask of the keys held down. Only the sideways arrows do anything
// different when one is held: measured, bash moves by a word for both
// `\e[1;5C` (Ctrl) and `\e[1;3C` (Alt), which is the pair of spellings the
// common terminals send for the same finger movement.
func (e *editor) namedKey(final byte, modifier int, prompt drawnPrompt) {
	held := 0
	if modifier > 1 {
		held = modifier - 1
	}
	const (
		alt  = 2
		ctrl = 4
	)
	byWord := held&(alt|ctrl) != 0
	switch final {
	case 'A':
		if held == 0 {
			e.browse(-1, prompt)
		}
	case 'B':
		if held == 0 {
			e.browse(+1, prompt)
		}
	case 'C':
		if byWord {
			e.moveTo(e.forwardWord(), prompt)
			return
		}
		e.moveTo(e.pos+1, prompt)
	case 'D':
		if byWord {
			e.moveTo(e.backwardWord(), prompt)
			return
		}
		e.moveTo(e.pos-1, prompt)
	case 'H':
		e.moveTo(0, prompt)
	case 'F':
		e.moveTo(len(e.line), prompt)
	}
}

// parameters reads the first number of a control sequence and the modifier
// after the semicolon. A missing number is zero, which no key uses.
func parameters(params []byte) (first, modifier int) {
	n, index := 0, 0
	for _, b := range params {
		switch {
		case b >= '0' && b <= '9':
			if n < 1<<20 {
				n = n*10 + int(b-'0')
			}
		case b == ';':
			if index == 0 {
				first = n
			}
			index, n = index+1, 0
		default:
			// A private-use or otherwise unexpected byte. The sequence is
			// still read to its end by the caller; it just names nothing.
			return 0, 0
		}
	}
	switch index {
	case 0:
		return n, 0
	case 1:
		return first, n
	default:
		return first, 0
	}
}

// keyRead is how the read of one byte of a key sequence came out.
type keyRead int

const (
	// keyContinues is a byte of the sequence, and the sequence goes on.
	keyContinues keyRead = iota
	// keyStopped is the input ending part-way through a sequence. There is no
	// key to act on and no line to go back to.
	keyStopped
	// keyAbandoned is ^C arriving part-way through a sequence, which is not a
	// byte of the key at all: the line is given up on, as though nothing had
	// been typed.
	keyAbandoned
)

// readByte is the next byte of a key sequence.
//
// A key that has begun and not finished is where an editor can wedge, and it
// is the one failure that makes a shell look broken rather than incomplete:
// press Escape by accident and the next keystroke is swallowed naming a key
// nobody meant to press.
//
// **There is no timeout, and that is measured rather than assumed.** With
// `echo one two` on the line, an ESC on its own and then a `b` typed six
// seconds later is still `M-b` in bash 5.3.15, bash 3.2.57 and zsh 5.9.2, and
// `\e[` with three seconds before the `A` is still Up. None of the three gives
// up on a half-read sequence, so a shell that did would be the odd one out —
// and there would be no honest number to pick, because every measurement says
// the wait is unbounded.
//
// What all three do have is ^C, and they have it without deciding to: their
// editors leave the terminal's ISIG on, so the kernel turns that one keystroke
// into a signal wherever the editor happens to be. This editor takes the
// terminal fully raw — ^C has to arrive as a byte for the line to be abandoned
// without racing a read already in progress, see makeRaw — so the same rescue
// has to be written down here rather than inherited. Measured, ESC then ^C
// abandons the line in all three shells and the next prompt is a fresh one.
func (e *editor) readByte() (byte, keyRead) {
	var b [1]byte
	for {
		// Through nextByte and not the reader: the rest of a key sequence has
		// to come from the same place its first byte did. Nothing pushes a
		// byte back part-way through a sequence today — the search mode does
		// it on its way out, and the read loop takes it as the next key's
		// first byte — so this is the invariant rather than a live case, and
		// the reader is the one place a second source would be missed.
		n, err := e.nextByte(b[:])
		if err != nil {
			return 0, keyStopped
		}
		if n == 0 {
			continue
		}
		if b[0] == ctrlC {
			return 0, keyAbandoned
		}
		return b[0], keyContinues
	}
}

// maxParams is how many parameter bytes of a control sequence are kept. Longer
// than any key sends and short enough that a terminal cannot make this grow.
const maxParams = 32
