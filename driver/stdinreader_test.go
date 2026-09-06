// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// Shell.Stdin is an io.Reader, so a front end whose input is not a descriptor
// can still be a shell. This is #787: an ACP session's input is a question put
// to the client when a script reads, and there is no file to hold one.
//
// The four routes are asked separately because they reach the reader by
// different paths — the program is fetched by lineReader, a `read` goes
// through the interpreter, and the two prompt questions are the only places
// the front end wanted a descriptor at all.

// runReaderShell invokes the front end with input that is a reader and not a
// file, which is the whole point: a strings.Reader is deliberately the kindest
// possible one, and the point is that the front end takes it at all.
func runReaderShell(t *testing.T, in io.Reader, argv ...string) (out, errs string, code int) {
	t.Helper()
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	sh.Stdin = in
	code = driver.MainArgs(sh, argv)
	return o.String(), e.String(), code
}

// The program itself arriving on a reader. `echo … | sh` with no pipe.
func TestAProgramOnStandardInputRunsFromAReaderThatIsNotAFile(t *testing.T) {
	out, errs, code := runReaderShell(t, strings.NewReader("echo one\necho two\n"), "testsh")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "one\ntwo\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
}

// And the script still shares the input with the program, which is the whole
// of #470 asked of a reader rather than of a descriptor: what the front end
// has not taken is what `read` finds. A strings.Reader can be rewound, so this
// takes lineReader's seeking arm; the byte-at-a-time arm is below.
func TestAProgramOnAReaderStillSharesItWithTheScript(t *testing.T) {
	const program = "read x\nDATA\necho \"[$x]\"\n"
	out, errs, code := runSharedReaderShell(t, strings.NewReader(program))
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "[DATA]\n"; out != want {
		t.Errorf("output %q, want %q — the data line was not left for the read", out, want)
	}
}

// The same, on a reader that cannot be rewound at all, which is the arm a pipe
// takes and the one an ACP session's input would. lineReader asks the value
// whether it can be rewound rather than asking its type, which is what makes
// both arms reachable through a field that is no longer an *os.File.
func TestAProgramOnAReaderThatCannotBeRewoundStillSharesIt(t *testing.T) {
	const program = "read x\nDATA\necho \"[$x]\"\n"
	out, errs, code := runSharedReaderShell(t, unseekable{strings.NewReader(program)})
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "[DATA]\n"; out != want {
		t.Errorf("output %q, want %q — the fetch swallowed the data line", out, want)
	}
}

// runSharedReaderShell is runReaderShell with the one axis these two are not
// about pinned: a dialect that takes the program in blocks has already
// swallowed the data line, and that is #470's answer rather than this one's.
func runSharedReaderShell(t *testing.T, in io.Reader) (out, errs string, code int) {
	t.Helper()
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	sh.Semantics.StdinProgramReadInBlocks = false
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	sh.Stdin = in
	code = driver.MainArgs(sh, []string{"testsh"})
	return o.String(), e.String(), code
}

// `-i` on a reader draws prompts and runs the lines, which is what the panel
// does with `-i` on a pipe. Without the widening reaching repl this route
// would have been handed a nil descriptor and read nothing at all — a session
// that ends at once and looks exactly like one that worked.
func TestTheForcedPromptReadsAReaderThatIsNotAFile(t *testing.T) {
	out, errs, code := runReaderShell(t, strings.NewReader("echo typed\n"), "testsh", "-i")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "typed\n"; out != want {
		t.Errorf("output %q, want %q — the prompt loop read nothing", out, want)
	}
	if errs == "" {
		t.Error("no prompt was drawn, and `-i` draws one wherever it is reached")
	}
}

// And the two terminal questions answer no, which is the cost of the widening
// stated as a test rather than left to be discovered. A reader is not a
// terminal, so there is no prompt without `-i` and no monitor with it.
func TestAReaderIsNotATerminal(t *testing.T) {
	sh := shell()
	sh.Stdin = strings.NewReader("")
	if driver.Interactively(sh, false) {
		t.Error("a reader that is not a file was taken for a terminal")
	}
	out, _, code := runReaderShell(t, strings.NewReader("case $- in *m*) echo monitor;; *) echo none;; esac\n"), "testsh", "-i")
	if code != 0 {
		t.Fatalf("status %d", code)
	}
	if !strings.Contains(out, "none") {
		t.Errorf("`$-` was %q, want no monitor: there is no terminal to announce a job to", out)
	}
}

// unseekable hides a reader's Seek, which is how lineReader tells a descriptor
// it can rewind from one it cannot — it asks the value, not the type.
type unseekable struct{ r io.Reader }

func (u unseekable) Read(p []byte) (int, error) { return u.r.Read(p) }

// What the widening costs at the other end, measured rather than assumed: a
// reader that is not a file is read *on a child's behalf*, whether or not the
// child ever reads it.
//
// os/exec connects a child straight to an *os.File and builds a pipe for
// anything else, and it fills that pipe from a goroutine that starts copying
// as soon as the command does. So one Read arrives for `/bin/echo`, which
// reads nothing.
//
// This is the fact #787 asked to have thought about — "anything that needs a
// descriptor number to hand to a child" — and it is the reason a reader that
// asks a *person* something cannot simply be dropped into this field: it would
// be asked once per external command rather than once per `read`. See
// docs/design/acp.md.
func TestAReaderIsReadOnAChildsBehalfWhetherOrNotTheChildReads(t *testing.T) {
	in := &countingReader{}
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	sh.Stdin = in
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	if code := driver.MainArgs(sh, []string{"testsh", "-c", "/bin/echo hi"}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, e.String())
	}
	if o.String() != "hi\n" {
		t.Fatalf("output %q, want %q", o.String(), "hi\n")
	}
	// A tripwire rather than an assertion about something we want. If this
	// ever reads zero, os/exec has stopped copying a stdin the child never
	// touched, and the paragraph in docs/design/acp.md that says an eliciting
	// reader cannot be a shell's standard input is out of date.
	if in.reads.Load() == 0 {
		t.Error("a child that reads nothing no longer causes a Read: " +
			"docs/design/acp.md's reason for not eliciting from standard input has expired")
	}
}

// countingReader is empty and counts who asked.
type countingReader struct{ reads atomic.Int64 }

func (c *countingReader) Read([]byte) (int, error) {
	c.reads.Add(1)
	return 0, io.EOF
}
