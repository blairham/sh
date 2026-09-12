// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// Keys a person has rebound, and how the editor finds out.
//
// The table is an **override layer** and not the editor's key handling: it
// holds only what somebody changed, and every key nobody mentioned reaches the
// editor's own dispatch untouched. That is the whole reason this is cheap. A
// full keymap here would be a second copy of what the editor already does,
// which is two places to fix a key and one of them silently ahead of the
// other; an override layer is empty in the ordinary case and costs a map
// lookup on a byte that starts nothing.
//
// It is read through a function rather than copied in at the start of a
// session, because `bindkey` is a command a person runs *at the prompt* as
// much as one an rc file runs. A snapshot would mean the binding took effect
// on the next shell rather than the next line.
//
// Two decisions in matchBinding are worth stating, because both are the
// answer to a hang rather than a preference:
//
//   - **A byte that starts no binding is not read past.** The lookup asks
//     first whether anything at all begins with the byte just read, and gives
//     up before touching the terminal if nothing does. Without that, every
//     ordinary keystroke would wait to find out whether a longer sequence was
//     coming.
//   - **A key sequence can be given up on part-way through.** ^C arriving
//     between the bytes of a binding abandons the line rather than being read
//     as part of the key, which is the wedge escape.go describes: a sequence
//     that has begun and cannot finish is the one failure that makes an editor
//     look broken rather than incomplete.
//   - **A prefix that leads nowhere asks the editor's own table before giving
//     up.** This layer is consulted *before* the editor's dispatch, so a
//     partial match here would otherwise shadow a complete match there — and
//     it did, on every macOS machine, the moment this shell started reading
//     `/etc/zshrc`. See the give-up branch below, which is where the up arrow
//     went.
//   - **An exact match wins immediately over a longer one that might follow.**
//     Bind both `^X` and `^X^T` and `^X` acts at once. A shell with a timer
//     waits to see which was meant; this reader blocks, so waiting is not
//     waiting — it is a key that does nothing until another is pressed.

// matchBinding reads as much of a bound sequence as has been started and
// returns what it is bound to.
//
// The first byte has already been read and is passed in. The second return is
// whether a binding claimed the key at all: false leaves the editor's own
// dispatch to handle the byte, which is what happens for every key nobody
// rebound.
func (e *editor) matchBinding(first byte) (Binding, bool, keyRead) {
	if e.bindings == nil {
		return Binding{}, false, keyContinues
	}
	table := e.bindings(e.keymap())
	if len(table) == 0 {
		return Binding{}, false, keyContinues
	}
	seq := string(first)
	if !anyBindingStartsWith(table, seq) {
		return Binding{}, false, keyContinues
	}
	for {
		if b, bound := table[seq]; bound {
			return b, true, keyContinues
		}
		// Nothing the person rebound answers to what has been read. Two
		// questions decide whether to read on, and they are asked of two
		// tables because this one is an **override layer**: a partial match
		// here must not shadow a complete match in the editor's own.
		//
		// That shadowing is the bug this shape exists to stop (#2435). macOS's
		// `/etc/zshrc` binds the arrows by `$terminfo[kcuu1]`, which is the
		// application-cursor spelling `\eOA`, while a terminal in normal
		// cursor mode sends `\e[A`. Reading `\e`, then `[`, then giving up
		// dropped what it had read and left the `A` to be typed into the line
		// — and the `\e[A` in defaultkeys.go, which walks history, was never
		// reached. The up arrow printed `A` on every macOS machine from the
		// day this shell started reading the system rc files.
		//
		// In zsh nothing is shadowed because its keymap holds both spellings
		// and the rc file replaces only one of them. This asks for that
		// completeness rather than copying it, which is what keeps the
		// override layer empty in the ordinary case — see the file comment.
		if anyBindingStartsWith(table, seq) {
			// A rebinding is still reachable, and it wins: a longer sequence
			// somebody bound is what they meant by pressing this prefix.
			next, got := e.readByte()
			if got != keyContinues {
				// The key is claimed either way: the bytes behind it are
				// gone, so handing the first one back to the dispatch would
				// run a key nobody finished pressing.
				return Binding{}, true, got
			}
			seq += string(next)
			continue
		}
		// The override layer has given up. An **exact** entry in the editor's
		// own table runs, and only an exact one — falling back on the first
		// byte alone would be wrong and was tried: binding `^A^B` makes `^A` a
		// prefix and takes its single-key meaning away, so `^A` followed by a
		// byte nothing continues to must leave the line alone rather than run
		// beginning-of-line. Measured in zsh, and pinned by
		// TestAnAbandonedSequenceDoesNotRunItsFirstKey.
		if w, known := defaultKeys[seq]; known {
			return Binding{Widget: w}, true, keyContinues
		}
		if !anyDefaultStartsWith(seq) {
			// Nothing anywhere answers to it, and nothing longer could. The
			// bytes are dropped rather than typed into the line, which is what
			// the editor does with any escape sequence it does not recognize —
			// see escape.go, where reading a key whole is the point.
			return Binding{}, true, keyContinues
		}
		next, got := e.readByte()
		if got != keyContinues {
			return Binding{}, true, got
		}
		seq += string(next)
	}
}

// keymap is which of the editor's two key tables is current.
//
// **This is the structural half of a command mode.** Before it, a shell's
// binding builtin kept a table per keymap and the editor read one of them: a
// binding written into the command map was stored and never fired, because the
// map was never current. The mode is what makes a keymap current, and this is
// where the editor says which.
func (e *editor) keymap() Keymap {
	if e.viCommand {
		return KeymapViCommand
	}
	return KeymapMain
}

// anyDefaultStartsWith is anyBindingStartsWith over the editor's own table.
//
// Separate rather than one function over two maps because the maps hold
// different things — a Binding can name an action of the *shell's*, and a
// default never can — and a single generic walker would have to be told which
// it was looking at anyway.
func anyDefaultStartsWith(seq string) bool {
	for bound := range defaultKeys {
		if len(bound) >= len(seq) && bound[:len(seq)] == seq {
			return true
		}
	}
	return false
}

// anyBindingStartsWith reports whether the table holds a sequence beginning
// with the one given, the exact sequence included.
func anyBindingStartsWith(table map[string]Binding, seq string) bool {
	for bound := range table {
		if len(bound) >= len(seq) && bound[:len(seq)] == seq {
			return true
		}
	}
	return false
}
