// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"io"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// LineReaderForTest is lineReader, reachable from the package's external
// tests.
//
// What it leaves behind on the descriptor is the whole of its contract and
// only half of that is visible in a shell's output, so the other half is
// checked directly — against a descriptor that seeks, and against one that
// stops seeking half way, which nothing a Shell can be handed will do.
type LineReaderForTest struct{ lr lineReader }

// Read is lineReader.read.
func (l *LineReaderForTest) Read(in io.Reader, buf []byte) string { return l.lr.read(in, buf) }

// SayVerboseForTest is sayVerbose, reachable from the package's external
// tests, with the position it carries flattened to the two numbers it is.
//
// Exposed because the position is the whole of #580 and only half of it is
// visible in a shell's output: which lines come out is checked by running
// scripts, and where the walk *stops* — on a text that has not ended in a
// newline, or that a later piece is going to continue — decides whether the
// next call resumes on the right byte, and nothing a Shell can be handed makes
// that observable on its own.
func (sh Shell) SayVerboseForTest(src string, upTo, line, off int, echo bool) (int, int) {
	at := sh.sayVerbose(src, upTo, verbosePos{line: line, off: off}, echo)
	return at.line, at.off
}

// FrontEndForTest is frontEnd, reachable from the package's external tests.
//
// The assembly is what is being checked and it is not exported: a dialect's
// answers have to arrive, and the editor they arrive at is only built where
// there is a terminal.
func (sh Shell) FrontEndForTest(r *interp.Runner, name string, dg interp.Diagnostics) repl.Shell {
	return sh.frontEnd(r, name, dg)
}
