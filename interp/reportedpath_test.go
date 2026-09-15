// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// What a builtin asked *where* a command is writes for an operand that was
// already written with a slash — see
// Semantics.APathnameOperandIsReportedAbsolute.
//
// These name no shell. The axis has two readings and both are held by real
// binaries; which dialect holds which is asserted in dialect/, and the
// measurements are in the axis's own doc comment.

// reportRun runs src with the axis set, in a directory holding `bb/tool`.
//
// Its own helper rather than lookRun because the working directory is the
// thing under test: one reading writes it out and the other never mentions
// it, so a test that could not name it could not tell them apart.
func reportRun(t *testing.T, answer Answer, src string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	script(t, dir+"/bb", "tool", "hi", true)
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.APathnameOperandIsReportedAbsolute = answer
	sem.TypeOptions = "taPp"
	sem.TypeEndsOptionsWithDashDash = Yes
	sem.TypePSearchesPathPastTheShell = No
	sem.TypePathAnswerIsASentence = No
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
		Vars: map[string]string{"PATH": "/usr/bin:/bin"},
		Env:  []string{"PATH=/usr/bin:/bin"},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	// The directory this run invented, written back as `<dir>` so an
	// assertion names the shape rather than a path that moves every run.
	// Substituted only where the shell really wrote it: an answer that names
	// no directory has nothing replaced, so a missing join still fails.
	out := strings.TrimRight(buf.String(), "\n")
	return strings.ReplaceAll(out, dir, "<dir>"), st
}

// The two readings, over the shapes that tell them apart.
//
// `<dir>` stands for the working directory, which the test substitutes back
// in: the shell that joins writes the directory it is in, and the shell that
// does not never mentions it.
func TestAPathnameOperandIsReportedAbsolute(t *testing.T) {
	for _, tc := range []struct {
		name, src, joined, asWritten string
	}{
		{
			name:      "a dot-slash operand",
			src:       `command -v ./bb/tool`,
			joined:    "<dir>/./bb/tool",
			asWritten: "./bb/tool",
		},
		{
			// The relative operand with no `./` in front of it, which is the
			// shape where the join is visible and the cleaning is not.
			name:      "a bare relative operand",
			src:       `command -v bb/tool`,
			joined:    "<dir>/bb/tool",
			asWritten: "bb/tool",
		},
		{
			// **Neither reading cleans.** The dot-dot survives the join, and
			// it is what says the joining shell performs a prefix join and
			// not a path normalization: a cleaned answer would be
			// `<dir>/bb/tool` in the first column and `bb/tool` in the
			// second, which is what this shell used to write in every
			// dialect (#2931).
			name:      "an operand that a cleaning would shorten",
			src:       `command -v ./bb/../bb/tool`,
			joined:    "<dir>/./bb/../bb/tool",
			asWritten: "./bb/../bb/tool",
		},
		{
			// An absolute operand has nothing to join to, so both readings
			// write it back byte for byte — the `/.` included.
			name:      "an absolute operand is untouched by either reading",
			src:       `command -v /bin/./ls`,
			joined:    "/bin/./ls",
			asWritten: "/bin/./ls",
		},
		{
			// The same question through every other builtin that asks it, so
			// a fix applied at one print site and not the others is visible
			// here rather than in one dialect's suite.
			name:      "the sentence `command -V` writes",
			src:       `command -V ./bb/tool`,
			joined:    "./bb/tool is <dir>/./bb/tool",
			asWritten: "./bb/tool is ./bb/tool",
		},
		{
			name:      "the sentence `type` writes",
			src:       `type ./bb/tool`,
			joined:    "./bb/tool is <dir>/./bb/tool",
			asWritten: "./bb/tool is ./bb/tool",
		},
		{
			name:      "the bare path `type -p` writes",
			src:       `type -p ./bb/tool`,
			joined:    "<dir>/./bb/tool",
			asWritten: "./bb/tool",
		},
		{
			name:      "the bare path `type -P` writes",
			src:       `type -P ./bb/tool`,
			joined:    "<dir>/./bb/tool",
			asWritten: "./bb/tool",
		},
		{
			name:      "every line `type -a` lists",
			src:       `type -a ./bb/tool`,
			joined:    "./bb/tool is <dir>/./bb/tool",
			asWritten: "./bb/tool is ./bb/tool",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.joined}, {No, tc.asWritten}} {
				out, st := reportRun(t, side.answer, tc.src)
				if st != 0 {
					t.Fatalf("%v: status %d: %s", side.answer, st, out)
				}
				if out != side.want {
					t.Errorf("%v: out = %q, want %q", side.answer, out, side.want)
				}
			}
		})
	}
}

// A pathname operand refuses where no dialect has answered, because the
// difference is one the command itself writes: a script reading
// `cmd=$(command -v ./helper)` gets a different string under each reading.
func TestAnUnansweredPathnameOperandIsRefused(t *testing.T) {
	out, st := reportRun(t, Unspecified, `command -v ./bb/tool`)
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out = %q, want the unanswered-axis refusal", out)
	}
	if strings.Contains(out, "bb/tool\n") {
		t.Errorf("out = %q, want no answer written alongside the refusal", out)
	}
}

// And a *bare* name is not this question at all: it was found by searching
// PATH, the resolved path is the answer in every column, and a shell with no
// answer here must still report one.
func TestABareNameIsNotThePathnameQuestion(t *testing.T) {
	out, st := reportRun(t, Unspecified, `command -v ls`)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if !strings.HasSuffix(out, "/ls") || !strings.HasPrefix(out, "/") {
		t.Errorf("out = %q, want the absolute path PATH resolved", out)
	}
}

// The kind is the same word under both readings, so asking there would refuse
// a command that has nothing to disagree about.
//
// This is the over-refusal the axis is deliberately not asked for: `type -t`
// writes `file` for a pathname operand in every column, and an earlier draft
// that asked inside the search made a bare core refuse it.
func TestTheKindOfAPathnameOperandIsNotRefused(t *testing.T) {
	for _, src := range []string{
		`type -t ./bb/tool`,
		`type -at ./bb/tool`,
		`type -pt ./bb/tool`,
	} {
		out, st := reportRun(t, Unspecified, src)
		if st != 0 || out != "file" {
			t.Errorf("%s: out = %q status %d, want %q and 0", src, out, st, "file")
		}
	}
}
