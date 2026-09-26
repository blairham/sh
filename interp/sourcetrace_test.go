// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// One trace line as the shell enters a file it is sourcing — see
// Runner.TracesEachSourcedFile. A session switch and not an axis, so this
// names the switch and no shell.
//
// The Diagnostics here asks for the name-and-line trace prefix, because the
// substrate's own is a bare `+ ` and a test that let it fall through could
// not tell a line naming the file from one naming nothing. It names two
// fields and no shell, which is what a test in this package may do.
func TestTracesEachSourcedFileWritesOneLinePerFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "echo body\n")
	src := ". ./inc.sh\necho after\n"
	out, _ := sourceRunWith(t, dir, src, permissive(), tracedLocation(), func(r *Runner) {
		r.SetTracesEachSourcedFile(true)
	})
	if want := "+./inc.sh:1> <sourcetrace>\nbody\nafter\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The control, and the state a Runner is in with nothing said: nothing is
// written. It is the same source, so a green row here and a green row above
// cannot both come from a shell that traced nothing.
func TestASourcedFileIsNotTracedWithNothingSaid(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "echo body\n")
	src := ". ./inc.sh\necho after\n"
	out, _ := sourceRunWith(t, dir, src, permissive(), tracedLocation(), nil)
	if want := "body\nafter\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// Each file, and a file entered from inside another one is a file: two lines,
// outermost first, and each names the file the frame it opened is reading.
func TestTracesEachSourcedFileNamesTheOperandItWasGiven(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inner.sh", "echo inner\n")
	write(t, dir, "outer.sh", ". ./inner.sh\n")
	src := ". ./outer.sh\n"
	out, _ := sourceRunWith(t, dir, src, permissive(), tracedLocation(), func(r *Runner) {
		r.SetTracesEachSourcedFile(true)
	})
	want := "+./outer.sh:1> <sourcetrace>\n+./inner.sh:1> <sourcetrace>\ninner\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A file with nothing in it is still entered, and a file that is not there is
// not — the second is what says the line marks the entry rather than the
// builtin's call.
func TestTracesEachSourcedFileMarksTheEntryAndNotTheCall(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "empty.sh", "")
	src := ". ./empty.sh\n. ./nope.sh\n"
	out, _ := sourceRunWith(t, dir, src, permissive(), tracedLocation(), func(r *Runner) {
		r.SetTracesEachSourcedFile(true)
	})
	if !strings.Contains(out, "+./empty.sh:1> <sourcetrace>\n") {
		t.Errorf("got %q, want the empty file's entry traced", out)
	}
	if strings.Contains(out, "nope.sh:1> <sourcetrace>") {
		t.Errorf("got %q, want no entry traced for a file that could not be opened", out)
	}
}

// And `eval` is not a file: the switch reaches text read from disk and
// nothing else, which is what keeps it a *source* trace.
func TestTracesEachSourcedFileDoesNotReachEval(t *testing.T) {
	dir := t.TempDir()
	src := "eval 'echo hi'\n"
	out, _ := sourceRunWith(t, dir, src, permissive(), tracedLocation(), func(r *Runner) {
		r.SetTracesEachSourcedFile(true)
	})
	if want := "hi\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// tracedLocation is the Diagnostics these rows need: a trace prefix that is a
// location rather than the substrate's bare `+ `, so the file and the line on
// the traced entry are visible to assert on.
func tracedLocation() Diagnostics {
	return Diagnostics{TraceStyle: TraceNameLine, LocationNamesTheCurrentFile: true}
}
