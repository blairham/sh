// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `BASH_XTRACEFD` moves the whole `set -x` trace to a descriptor of the
// script's own.
//
// Measured on bash 5.3.20 from a script file on 2026-09-22; see
// dialect/bash/xtracefd.go for the three facts the shape rests on.
func TestBashXtraceFdMovesTheTrace(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "trace")
	out, st := runBash(t, dir, `
exec 4>`+log+`
BASH_XTRACEFD=4
set -x
echo one
set +x
BASH_XTRACEFD=
echo two
`)
	if st != 0 {
		t.Fatalf("status %d", st)
	}
	// Nothing of the trace reached the shell's own streams.
	if strings.Contains(out, "+ echo one") {
		t.Errorf("the trace stayed on the shell's streams: %q", out)
	}
	if out != "one\ntwo\n" {
		t.Errorf("output %q, want the two commands' own", out)
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	// `set +x` is traced too: it is a command like any other, and the option
	// is not off until it has run.
	if want := "+ echo one\n+ set +x\n"; string(body) != want {
		t.Errorf("trace file %q, want %q", string(body), want)
	}
}

// A value naming no open descriptor complains **at the assignment** and still
// assigns, and a number that is open but cannot be written draws a sentence of
// its own.
func TestBashXtraceFdComplainsAtTheAssignment(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, `
BASH_XTRACEFD=4
echo "[$BASH_XTRACEFD]"
BASH_XTRACEFD=notanumber
echo "[$BASH_XTRACEFD]"
BASH_XTRACEFD=
echo "[$BASH_XTRACEFD]"
`)
	for _, want := range []string{
		"BASH_XTRACEFD: 4: invalid value for trace file descriptor",
		"BASH_XTRACEFD: notanumber: invalid value for trace file descriptor",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	// The store happens anyway: this is a complaint about the descriptor and
	// not a refused assignment.
	if !strings.Contains(out, "[4]\n") || !strings.Contains(out, "[notanumber]\n") {
		t.Errorf("the value did not read back: %q", out)
	}
	// And an empty value is silent — the trace simply goes back to standard
	// error.
	if strings.Count(out, "invalid value") != 2 {
		t.Errorf("an empty value was not silent: %q", out)
	}
}

// With no parameter, or with one naming nothing, the trace is on standard
// error as it always was.
func TestBashXtraceFdLeavesTheTraceWhereItWas(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, "set -x\necho one\n")
	if !strings.Contains(out, "+ echo one") {
		t.Errorf("the trace left standard error: %q", out)
	}
}
