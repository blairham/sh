// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "io"

// keystrokes hands over one byte per Read, which is how a terminal delivers
// typing.
//
// It matters because the editor coalesces its drawing: input already in hand is
// drawn once, when it runs out, so a reader that answers a whole line in one
// Read produces one redraw where a person typing produces one per character
// (#1742). Every test here that asserts what is drawn *per keystroke* needs the
// typing rather than the paste — the wrap and wide-character cases especially,
// whose whole subject is the move back up to the prompt row that only a second
// and later draw can make.
//
// A test that wants the paste says so by handing the editor a reader that
// answers in one go, and asserts *bytes written* rather than sequences. See
// TestAPastedLineCostsTheLineAndNotItsSquare.
type keystrokes struct {
	s string
	i int
}

func typing(s string) *keystrokes { return &keystrokes{s: s} }

func (k *keystrokes) Read(p []byte) (int, error) {
	if k.i >= len(k.s) {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = k.s[k.i]
	k.i++
	return 1, nil
}

// The end is io.EOF and not a sentinel of its own: input running out *is* an end
// of file, and the editor reads it as one — `^D` on an empty line ends the
// session by returning it, so a named error here made that path report the wrong
// thing.
