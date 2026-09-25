// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "io"

// crlf is the newline translation the terminal is no longer doing.
//
// Raw mode turns OPOST off, and with it the kernel's ONLCR — the rule that a
// newline written to a terminal also returns the carriage. So a line the shell
// writes between one prompt and the next ends by moving down and not back, and
// the next line starts under the end of the last one:
//
//	[1]+  Done                       sleep 2
//	                                         $
//
// The editor spells `\r\n` itself, because it has always drawn straight to the
// terminal. Everything else the session says — a job that ended, a line that
// would not parse, the warning about a stopped job at exit — is a message
// written with a plain newline by code that has no business knowing the
// terminal's mode. This is that knowledge, in one place: the loop that turned
// OPOST off owes the translation, and puts it on its own streams.
//
// A newline that already follows a return is left alone, which is what makes
// it safe to wrap a writer the editor also draws through.
type crlf struct {
	w      io.Writer
	afterR bool
	// while is when the translation is owed, and nil is "always". A session
	// whose line editor can be turned off spends part of its life with the
	// terminal in its own discipline, and there the kernel is doing this
	// again — so a stream that translated unconditionally would put the
	// return in twice. The memory is kept either way: what the terminal last
	// saw does not depend on which of the two wrote it.
	while func() bool
}

func (c *crlf) Write(p []byte) (int, error) {
	owed := c.while == nil || c.while()
	out := make([]byte, 0, len(p)+8)
	for _, b := range p {
		if b == '\n' && !c.afterR && owed {
			out = append(out, '\r')
		}
		c.afterR = b == '\r'
		out = append(out, b)
	}
	if _, err := c.w.Write(out); err != nil {
		return 0, err
	}
	// The count is the caller's bytes and not the written ones: a writer that
	// claims to have written more than it was given is a short-write error to
	// everything that checks.
	return len(p), nil
}

// forget drops what this stream remembers about the byte it last wrote.
//
// The memory is only good for as long as this writer is the only thing
// reaching the terminal, and between one prompt and the next it is not. A
// command runs with the terminal in its own line discipline and writes to it
// directly; the terminal itself echoes what was typed, and a ^C leaves a `^C`
// two columns in. Neither of those bytes comes through here, so what this
// stream wrote *before* them is no longer what the screen ends with — and a
// newline written afterwards is then left bare on the strength of a carriage
// return that scrolled past several lines ago.
//
// That is #2861. The editor ends its line with `\x1b[?2004l\r`, the command
// runs and is interrupted, and the newline the loop writes to get off the
// echoed `^C` moves down without returning: the next prompt is drawn two
// columns in. It was read as a job-control notice written bare, because the
// two shells that hold an exit for a running job print one shortly before —
// but those notices are whole, and this one newline was not.
//
// Cheap to be wrong in this direction. A return added where one was already
// there costs nothing at all, and one left out costs the start of every row
// after it.
func (c *crlf) forget() { c.afterR = false }

// translating wraps a stream in that rule, leaving a nil one nil — a session
// without an error stream has nothing to translate.
func translating(w io.Writer) io.Writer { return translatingWhile(w, nil) }

// translatingWhile is translating with the rule applied only while while
// answers true. A nil while is "always", which is what a caller with one mode
// for the whole session gets.
func translatingWhile(w io.Writer, while func() bool) io.Writer {
	if w == nil {
		return nil
	}
	return &crlf{w: w, while: while}
}
