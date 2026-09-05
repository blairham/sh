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

// FrontEndForTest is frontEnd, reachable from the package's external tests.
//
// The assembly is what is being checked and it is not exported: a dialect's
// answers have to arrive, and the editor they arrive at is only built where
// there is a terminal.
func (sh Shell) FrontEndForTest(r *interp.Runner, name string, dg interp.Diagnostics) repl.Shell {
	return sh.frontEnd(r, name, dg)
}
