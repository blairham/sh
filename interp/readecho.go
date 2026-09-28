// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// readEcho is what `read`'s `-e` and `-E` letters ask of one call: whether
// each value is written to standard output as it is produced, and whether it
// is written to the name as well.
//
// Two fields rather than one, because the two letters compose and the pair is
// not a single switch: `-E` echoes and assigns, `-e` echoes and does not, and
// `read -eE x` and `read -Ee x` both leave `x` unset — **`-e` wins whichever
// order the letters were written in**, measured on zsh 5.9.2 in both
// spellings. So the suppression is a property of the letter having appeared
// at all and not of it having appeared last, which is why this is read off
// the option string rather than off the last sign seen.
//
// See Semantics.ReadEchoLettersWriteTheValues, which is what says the letters
// mean this here rather than bash's line editor.
type readEcho struct {
	// writes says each value goes to standard output, one line each.
	writes bool
	// withholds says the value does **not** reach the name. `-e`'s, and the
	// write is then never *attempted* rather than attempted and undone —
	// which is what keeps a frozen name silent. See the field's own row in
	// Semantics.ReadEchoLettersWriteTheValues.
	//
	// Spelled as the *withholding* and not as the assigning, so that the
	// zero value is an ordinary `read`. It was `assigns` for one build and
	// the zero value then meant "write to no name at all" — which is what
	// every `read` in every dialect that never reaches this got, and what
	// the bash control caught: `read -E x y` there left both names not
	// found where the reference assigns them. A field whose zero value is
	// the wrong answer is a field every caller has to remember to set.
	withholds bool
}

// readEchoLetters reads the pair off one call's options, asking the dialect
// what they mean and asking it only where one of them was written.
//
// The zero value is the answer for every call that spelled neither, which is
// nearly every `read` in every script: no question is put and nothing about
// the builtin changes. That is the rule this is written to — an axis reported
// where it decides nothing is a refusal a script cannot act on.
func (r *Runner) readEchoLetters(opts string) readEcho {
	lower, upper := containsByte(opts, 'e'), containsByte(opts, 'E')
	if !lower && !upper {
		return readEcho{}
	}
	if !r.ask(r.sem().ReadEchoLettersWriteTheValues,
		"`read -e` and `-E` writing the values read to standard output") {
		// The other reading: the letters open a line editor, which off a
		// terminal is nothing at all. bash's, and it is why they sit in its
		// ReadOptions doing nothing.
		return readEcho{}
	}
	return readEcho{writes: true, withholds: lower}
}

// value writes one of `read`'s values where the letters asked for it, and
// reports whether it may still be written to the name.
//
// **Ahead of the assignment and not behind it**, which is measured rather
// than chosen: `typeset -r fz=keep; read -E fz` on zsh 5.9.2 writes the
// read-only refusal *and* the line, so the echo is not something the write
// earns. A bad name is the same shape — `read -E 1bad x` complains about the
// name and still writes both lines.
//
// One line per call, with the newline, and on **standard output**: measured,
// `read -E x 2>/dev/null` still shows the line and `read -E x 1>/dev/null`
// shows nothing, and the line reaches a pipe.
func (e readEcho) value(r *Runner, v string) bool {
	if e.writes {
		r.printf("%s\n", v)
	}
	return !e.withholds
}

// elements is value over the whole of an array target: one line per element
// of the array as it would be assigned, which is the same sentence read of a
// container.
//
// The elements the *assignment* would make and not the fields the splitter
// found, which is the distinction one row settles: `printf '\n' | read -E -A
// arr` leaves one empty element in zsh 5.9.2 and echoes one empty line, so
// the answer that decides the array's length decides the echo's too. Taking
// the splitter's list would have written nothing there.
func (e readEcho) elements(r *Runner, values []string) bool {
	if e.writes {
		for _, v := range values {
			r.printf("%s\n", v)
		}
	}
	return !e.withholds
}
