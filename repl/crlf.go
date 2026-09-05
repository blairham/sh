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
}

func (c *crlf) Write(p []byte) (int, error) {
	out := make([]byte, 0, len(p)+8)
	for _, b := range p {
		if b == '\n' && !c.afterR {
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

// translating wraps a stream in that rule, leaving a nil one nil — a session
// without an error stream has nothing to translate.
func translating(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}
	return &crlf{w: w}
}
