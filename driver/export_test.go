// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"io"
	"os"

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
	at := sh.sayVerbose(sh.Stderr, src, upTo, verbosePos{line: line, off: off}, echo)
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

// ReplaceProcessForTest is replaceProcess, reachable from the package's
// external tests.
//
// Exposed because what it does to the *process* — stopping the collector for
// the window before the execve and starting it again if the execve fails — is
// not visible in any shell's output, and the exec it is named for does not
// return. Handed a path that cannot be executed, it takes the failure path,
// which is the one a test can be in the same process as.
func ReplaceProcessForTest(path string, argv, env []string, files []*os.File) error {
	return replaceProcess(path, argv, env, files)
}

// AtPlacementForTest fills in the seam that stands on both sides of the
// descriptor placement.
func AtPlacementForTest(f func(where string)) { atPlacement = f }

// InstallPrefixForTest and FunctionSearchDirsForTest are the two halves of
// where a shell looks for function definition files, reachable from the
// package's external tests.
//
// Exposed because the derivation is a fact about *layouts* and the only way to
// see it through a running shell is to install a binary into each of them —
// which puts a build step in the middle of what is a string transformation,
// and could only ever exercise the layout this checkout happens to be in.
func InstallPrefixForTest(exe string) string { return installPrefix(exe) }

// FunctionSearchDirsForTest is functionSearchDirs.
func FunctionSearchDirsForTest(prefix string) []string { return functionSearchDirs(prefix) }
