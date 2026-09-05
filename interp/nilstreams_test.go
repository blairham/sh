// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runNilStreams runs src on r, which is a Runner some of whose streams the
// caller deliberately left unset.
func runNilStreams(t *testing.T, r *interp.Runner, src string) int {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	status, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// A stream nobody supplied is empty, not the process's own.
//
// The same answer Env gives, and the same reason: a Runner is embedded in
// other programs, and the process's streams belong to the program rather than
// to a shell inside it. Borrowing stdout is the worst of the three, because it
// is the one the shell *writes*.
func TestNilStreamsAreEmptyRatherThanTheProcess(t *testing.T) {
	r := newTestRunner(t, &interp.Runner{})

	if got := r.Out(); got == io.Writer(os.Stdout) {
		t.Error("a nil Stdout answered with the process's own output")
	}
	if got := r.Err(); got == io.Writer(os.Stderr) {
		t.Error("a nil Stderr answered with the process's own error stream")
	}
	if got := r.In(); got == io.Reader(os.Stdin) {
		t.Error("a nil Stdin answered with the process's own input")
	}

	// And empty means empty: a read finds the end straight away rather than
	// waiting on a descriptor the shell was never given.
	n, err := r.In().Read(make([]byte, 8))
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("reading a nil Stdin gave %d bytes, %v, want 0 and end of input", n, err)
	}
}

// The script-visible half of the reading side: a shell with no input of its
// own reads end of input.
//
// Which is a real answer rather than a failure — `read` says so with a status,
// exactly as it does at the end of a file — and it is the answer that does not
// depend on what the embedding program happened to have on its own descriptor.
func TestAShellWithNoInputReadsEndOfInput(t *testing.T) {
	var out strings.Builder
	r := newTestRunner(t, &interp.Runner{Stdout: &out, Stderr: &out})

	if status := runNilStreams(t, r, "if read line; then echo got:$line; else echo end; fi\n"); status != 0 {
		t.Errorf("status = %d, want the script itself to have run", status)
	}
	if got := out.String(); got != "end\n" {
		t.Errorf("out = %q, want %q", got, "end\n")
	}
}

// The writing side stays usable: output goes nowhere rather than failing, so a
// Runner nobody wired is still a Runner that runs.
//
// A write that *failed* would be the other way to read "no stream", and it is
// the wrong one — it would make every unwired embedder's script fail on its
// first `echo`, which is a worse surprise than silence.
func TestAShellWithNoOutputStillRuns(t *testing.T) {
	r := newTestRunner(t, &interp.Runner{})
	if status := runNilStreams(t, r, "echo one; echo two >&2; echo three\n"); status != 0 {
		t.Errorf("status = %d, want writes to a nil stream to succeed quietly", status)
	}
}

// An external command started by a Runner with no streams still runs, and its
// output goes to the same emptiness the shell's own does — nil reaches os/exec
// as nil, which is /dev/null to a child.
func TestAChildOfAShellWithNoStreamsStillRuns(t *testing.T) {
	const path = "/bin/echo"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no %s on this machine", path)
	}
	r := newTestRunner(t, &interp.Runner{})
	if status := runNilStreams(t, r, path+" out\n"); status != 0 {
		t.Errorf("status = %d, want the command to have run and succeeded", status)
	}
}
