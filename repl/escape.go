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
func (e *editor) escape(prompt drawnPrompt) {
	b, ok := e.readByte()
	if !ok {
		return
	}
	switch b {
	case '[':
		e.controlSequence(prompt)
	case 'O':
		// SS3. A terminal in application cursor mode sends the arrows and
		// Home and End this way, and it is the mode a full-screen program
		// leaves behind, so a shell sees it constantly. One byte names the
		// key and there are no parameters.
		if f, ok := e.readByte(); ok {
			e.namedKey(f, 0, prompt)
		}
	case 'b', 'B':
		e.moveTo(e.backwardWord(), prompt)
	case 'f', 'F':
		e.moveTo(e.forwardWord(), prompt)
	case 'd', 'D':
		e.killForwardTo(e.endOfWord())
		e.redraw(prompt)
	case del, backspace:
		// M-Delete kills the word before the cursor. Both spellings, because
		// a terminal sends whichever of the two its erase character is, and
		// both shells act on both.
		e.killTo(e.backwardWord())
		e.redraw(prompt)
	}
	// Anything else is a key this does not act on, and the ESC and the byte
	// after it are dropped together. Measured: both shells do nothing at all
	// for `M-z`, and neither puts the `z` in the line.
}

// controlSequence reads a CSI — everything after `\e[` — and acts on it.
//
// The shape is fixed even when the meaning is unknown: any number of parameter
// bytes, any number of intermediate bytes, then exactly one final byte. Reading
// to the final byte is the whole point; the switch afterwards is allowed to
// recognize nothing.
func (e *editor) controlSequence(prompt drawnPrompt) {
	var params [maxParams]byte
	kept := 0
	b, ok := e.readByte()
	for ok && b >= 0x30 && b <= 0x3f {
		if kept < len(params) {
			// A fixed room for them rather than a growing one: the bytes come
			// from a terminal, and a sequence longer than this is not a
			// keystroke. Past the cap they are still read — reaching the final
			// byte is the point — and no longer kept.
			params[kept] = b
			kept++
		}
		b, ok = e.readByte()
	}
	for ok && b >= 0x20 && b <= 0x2f {
		b, ok = e.readByte()
	}
	if !ok || b < 0x40 || b > 0x7e {
		return
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
			if _, ok := e.readByte(); !ok {
				return
			}
		}
		return
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
			e.deleteForward()
			e.redraw(prompt)
		}
		return
	}
	e.namedKey(b, modifier, prompt)
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

// readByte is one byte of input, or false once there is no more.
func (e *editor) readByte() (byte, bool) {
	var b [1]byte
	for {
		n, err := e.in.Read(b[:])
		if err != nil {
			return 0, false
		}
		if n > 0 {
			return b[0], true
		}
	}
}

// maxParams is how many parameter bytes of a control sequence are kept. Longer
// than any key sends and short enough that a terminal cannot make this grow.
const maxParams = 32
