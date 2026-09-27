// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A file sourced from pinned text has lines of its own, and they are its own
// line plus one.
//
// A pin stands for the text that took it — see
// Semantics.TrapBodyLine and TrapBodyLineWhereItFired, where every line of a
// trap body reports the line the trap fired on. A *file* the body sources is
// not that text, and its lines were reported as the firing line all the same.
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go version
// -m` → *not a Go executable*, `-f` over a script file. A three-line file
// each of whose lines reads `$LINENO`, sourced from a trap body, reads
// `2 3 4`; the same file sourced at the top level reads `1 2 3` in both
// shells. The `+1` is constant — a three-line body whose `.` stands on its
// last line reads `2 3 4` as well — so it is neither the body's length nor
// where in the body the `.` was written (#4757).
//
// The axis is named and no shell is. Only a dialect that pins a trap body's
// lines can reach this at all, and the control below is the same file under
// the other answer.
func pinnedSourceSem() Semantics {
	s := permissive()
	s.TrapBodyLine = TrapBodyLineWhereItFired
	s.ExitTrapFiresPastTheEnd = Yes
	return s
}

const threeLinesReadingTheLine = "echo S1=$LINENO\necho S2=$LINENO\necho S3=$LINENO\n"

func TestAFileSourcedFromAPinnedBodyIsNumberedOnFromItsOwnFirstLine(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", threeLinesReadingTheLine)
	src := "echo pad\ntrap '. ./inc.sh' EXIT\necho done\n"
	out, _ := sourceRunWith(t, dir, src, pinnedSourceSem(), Diagnostics{}, nil)
	if want := "pad\ndone\nS1=2\nS2=3\nS3=4"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
	}
}

// The control the row rests on, and the one a fix that simply added one
// everywhere would fail: the same file at the top level is unmoved.
func TestTheSameFileAtTheTopLevelKeepsItsOwnLines(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", threeLinesReadingTheLine)
	out, _ := sourceRunWith(t, dir, "echo pad\n. ./inc.sh\n", pinnedSourceSem(), Diagnostics{}, nil)
	if want := "pad\nS1=1\nS2=2\nS3=3"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
	}
}

// And the second control, which is the one that says the offset belongs to
// the *pin* rather than to being inside a trap at all: a body that calls a
// function which sources the file reads the file's own lines, because the pin
// ends at the call.
func TestAFileSourcedThroughACallFromAPinnedBodyKeepsItsOwnLines(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", threeLinesReadingTheLine)
	src := "h() { . ./inc.sh; }\ntrap 'h' EXIT\necho done\n"
	out, _ := sourceRunWith(t, dir, src, pinnedSourceSem(), Diagnostics{}, nil)
	if want := "done\nS1=1\nS2=2\nS3=3"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
	}
}

// A file that sources another is what tells "in force" from "applied again":
// the inner file reads `2 3 4` as well and not `3 4 5`.
func TestTheOffsetIsInForceRatherThanAppliedPerFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", threeLinesReadingTheLine)
	write(t, dir, "outer.sh", "echo A1=$LINENO\n. ./inc.sh\necho A3=$LINENO\n")
	src := "trap '. ./outer.sh' EXIT\necho done\n"
	out, _ := sourceRunWith(t, dir, src, pinnedSourceSem(), Diagnostics{}, nil)
	if want := "done\nA1=2\nS1=2\nS2=3\nS3=4\nA3=4"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
	}
}

// It is the whole location and not only the parameter. A refusal raised on
// the file's third line is located at its fourth, which is the same number
// the parameter reads there — two readers of one location.
func TestADiagnosticFromAPinnedSourcedFileCarriesTheSameLine(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "echo one\necho two\n${NOPE?}\n")
	src := "echo pad\necho pad\necho pad\ntrap '. ./inc.sh' EXIT\n"
	dg := Diagnostics{Location: LocationTightLine, LocationNamesTheCurrentFile: true}
	out, _ := sourceRunWith(t, dir, src, pinnedSourceSem(), dg, nil)
	if !strings.Contains(out, "inc.sh:4:") {
		t.Errorf("got %q, want the refusal located at `inc.sh:4:`", out)
	}
}
