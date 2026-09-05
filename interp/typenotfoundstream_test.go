// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which stream `type` and `command -V` report a name they could not account
// for on — Diagnostics.TypeNotFoundOnStdout.
//
// Half the panel treats the line as an answer and writes it where the answers
// go; half treats it as a complaint. Neither follows from the wording, so the
// tests below hold the wording still and move only the stream. They also pin
// the two cases the value must *not* move: a name that resolves, and `-t`'s
// silence.

func notFoundRun(t *testing.T, src string, dg Diagnostics) (string, string, int) {
	t.Helper()
	dg.TypeNotFound = "type: %[1]s: nothing here"
	dg.CommandVNotFound = "command: %[1]s: nothing here"
	return declRun(t, src, func(s *Semantics) {
		s.TypeEndsOptionsWithDashDash = Yes
		s.TypeNamesTheKindWithDashT = Yes
		s.TypeOptions = "afpP"
	}, dg)
}

// TestNotFoundOnStderrByDefault: unanswered, the line is a diagnostic, and it
// carries the location prefix every other diagnostic carries.
func TestNotFoundOnStderrByDefault(t *testing.T) {
	out, errs, _ := notFoundRun(t, `type nothing-at-all`, Diagnostics{})
	if out != "" {
		t.Errorf("stdout = %q, want nothing on it", out)
	}
	if !strings.Contains(errs, "nothing here") {
		t.Errorf("stderr = %q, want the report on it", errs)
	}
	if !strings.HasPrefix(errs, "testsh") {
		t.Errorf("stderr = %q, want the location prefix on it", errs)
	}
}

// TestNotFoundOnStdoutWhenAnswered: the same line, same wording, other stream.
func TestNotFoundOnStdoutWhenAnswered(t *testing.T) {
	dg := Diagnostics{TypeNotFoundOnStdout: true, TypeNotFoundUnprefixed: true}
	out, errs, _ := notFoundRun(t, `type nothing-at-all`, dg)
	if want := "type: nothing-at-all: nothing here\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want nothing on it", errs)
	}
}

// TestNotFoundStreamIsNotTheStatus: moving the report leaves the status where
// the status axis put it. The two travel together in the panel and are
// nonetheless separate answers.
func TestNotFoundStreamIsNotTheStatus(t *testing.T) {
	for _, c := range []struct {
		name string
		dg   Diagnostics
	}{
		{"complaint", Diagnostics{TypeNotFoundStatus: 127}},
		{"report", Diagnostics{TypeNotFoundStatus: 127, TypeNotFoundOnStdout: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, _ := notFoundRun(t, `type nothing-at-all; echo "st=$?"`, c.dg)
			if !strings.Contains(out, "st=127") {
				t.Errorf("stdout = %q / stderr = %q, want st=127 reported", out, errs)
			}
		})
	}
}

// TestNotFoundStreamIsNotThePrefix: the two questions do not constrain each
// other. A dialect that reports on standard output and still names itself is
// not in the panel, and the code must not assume it away.
func TestNotFoundStreamIsNotThePrefix(t *testing.T) {
	dg := Diagnostics{TypeNotFoundOnStdout: true}
	out, errs, _ := notFoundRun(t, `type nothing-at-all`, dg)
	if !strings.HasPrefix(out, "testsh") {
		t.Errorf("stdout = %q, want the location prefix on it", out)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want nothing on it", errs)
	}
}

// TestNotFoundOnStdoutKeepsOneOrder: the reason the stream is visible at all.
// With the report on standard output, a run of names reads in the order it
// was asked for — misses and hits in one stream — rather than splitting into
// two that a reader has to interleave.
func TestNotFoundOnStdoutKeepsOneOrder(t *testing.T) {
	dg := Diagnostics{TypeNotFoundOnStdout: true, TypeNotFoundUnprefixed: true}
	out, errs, _ := notFoundRun(t, `type -- nothing-at-all if`, dg)
	want := "type: nothing-at-all: nothing here\nif is a shell keyword\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want nothing on it", errs)
	}
}

// TestNotFoundStreamLeavesAResolvedNameAlone: the control. A name that is
// something was always an answer on standard output and stays one, whichever
// way the value goes.
func TestNotFoundStreamLeavesAResolvedNameAlone(t *testing.T) {
	for _, onStdout := range []bool{false, true} {
		dg := Diagnostics{TypeNotFoundOnStdout: onStdout, TypeNotFoundUnprefixed: onStdout}
		out, errs, st := notFoundRun(t, `type if`, dg)
		if want := "if is a shell keyword\n"; out != want || st != 0 {
			t.Errorf("TypeNotFoundOnStdout=%v: stdout = %q (status %d), want %q",
				onStdout, out, st, want)
		}
		if errs != "" {
			t.Errorf("TypeNotFoundOnStdout=%v: stderr = %q, want nothing on it", onStdout, errs)
		}
	}
}

// TestNotFoundStreamLeavesDashTSilent: the other control. `-t` says nothing
// about a name that is nothing, so there is no line for the stream to carry
// and neither stream gains one.
func TestNotFoundStreamLeavesDashTSilent(t *testing.T) {
	for _, onStdout := range []bool{false, true} {
		dg := Diagnostics{TypeNotFoundOnStdout: onStdout, TypeNotFoundUnprefixed: onStdout}
		out, errs, _ := notFoundRun(t, `type -t nothing-at-all`, dg)
		if out != "" || errs != "" {
			t.Errorf("TypeNotFoundOnStdout=%v: stdout = %q, stderr = %q, want both empty",
				onStdout, out, errs)
		}
	}
}

// TestCommandVNotFoundSharesTheStream: `command -V` asks `type`'s question
// with a complaint of its own, and the stream is the builtins' shared answer
// rather than one held twice.
func TestCommandVNotFoundSharesTheStream(t *testing.T) {
	dg := Diagnostics{TypeNotFoundOnStdout: true, TypeNotFoundUnprefixed: true}
	out, errs, _ := notFoundRun(t, `command -V nothing-at-all`, dg)
	if want := "command: nothing-at-all: nothing here\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want nothing on it", errs)
	}

	out, errs, _ = notFoundRun(t, `command -V nothing-at-all`, Diagnostics{})
	if out != "" {
		t.Errorf("stdout = %q, want nothing on it", out)
	}
	if !strings.Contains(errs, "command: nothing-at-all: nothing here") {
		t.Errorf("stderr = %q, want the complaint on it", errs)
	}
}
