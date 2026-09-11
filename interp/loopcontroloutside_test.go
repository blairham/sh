// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `break` and `continue` with no loop around them are a misuse the panel
// answers three ways, and the two halves of the answer are separable: whether
// anything is said, and whether the script goes on.
//
// It used to do neither. The control value was set whatever the depth was and
// nothing consumed it, so it unwound past the top and everything after the
// `break` vanished — silently, at status 0. A script testing `$?` after a
// misplaced `break` was told it had succeeded (#1236).
func TestLoopControlWithNoLoopAroundIt(t *testing.T) {
	const src = "echo t\nbreak\necho after\n"
	for _, c := range []struct {
		name    string
		fatal   Answer
		wording string
		want    []string
		gone    string
		status  int
	}{
		{
			// The answer that says something and carries on, which is what
			// leaves the misuse visible without costing the script.
			"reported, and the script carries on",
			No,
			"%[1]s: only meaningful in a loop",
			[]string{"t", "break: only meaningful in a loop", "after"},
			"",
			0,
		},
		{
			// The answer with no wording at all: the misuse is ignored and
			// there is nothing to read.
			"silent, and the script carries on",
			No,
			"",
			[]string{"t", "after"},
			"break",
			0,
		},
		{
			// And the answer that stops the script. The status is then the
			// dialect's own for a fatal error rather than a number of this
			// misuse's, which is why nothing here names one.
			"reported, and the script stops there",
			Yes,
			"%[1]s: not in a loop",
			[]string{"t", "break: not in a loop"},
			"after",
			1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := loopControlRun(t, src, c.fatal, c.wording)
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("said %q, want %q in it", out, w)
				}
			}
			if c.gone != "" && strings.Contains(out, c.gone) {
				t.Errorf("said %q, want %q not in it", out, c.gone)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// `continue` asks the same question and is worded with its own name, which is
// what the verb in the format is for — a wording that hardcoded `break` would
// have named the wrong builtin here and no row above would have seen it.
func TestTheMisusedBuiltinIsNamedInTheComplaint(t *testing.T) {
	out, _ := loopControlRun(t, "continue\n", No, "%[1]s: only meaningful in a loop")
	if !strings.Contains(out, "continue: only meaningful in a loop") {
		t.Errorf("said %q, want the builtin's own name in it", out)
	}
}

// With a loop around it the question is never put, under either answer: a
// `break` inside one is ordinary control flow in every shell in the panel.
func TestLoopControlInsideALoopIsNeverRefused(t *testing.T) {
	for _, fatal := range []Answer{Yes, No, Unspecified} {
		out, st := loopControlRun(t, "while :; do break; done\necho ok\n", fatal, "%[1]s: not in a loop")
		if !strings.Contains(out, "ok") || strings.Contains(out, "not in a loop") {
			t.Errorf("fatal=%v: said %q, want the loop left quietly", fatal, out)
		}
		if st != 0 {
			t.Errorf("fatal=%v: status = %d, want 0", fatal, st)
		}
	}
}

// The count is dynamic rather than lexical, which is what a `break` inside a
// *subshell* inside a loop turns on: it leaves that subshell, and a cloned
// Runner carries the count with it. A lexical reading would have complained
// here and left the subshell running.
func TestLoopControlInsideASubshellInsideALoop(t *testing.T) {
	out, st := loopControlRun(t,
		"for i in 1 2; do ( break; echo insub ); echo body; done\necho after\n",
		Yes, "%[1]s: not in a loop")
	if strings.Contains(out, "not in a loop") || strings.Contains(out, "insub") {
		t.Errorf("said %q, want the subshell left quietly", out)
	}
	if !strings.Contains(out, "body") || !strings.Contains(out, "after") {
		t.Errorf("said %q, want the loop and the line to carry on", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The two loops that are not `for`, `while` or `until` raise the count too.
// Both were counting nothing, so a `break` inside either would have been
// reported as having no loop around it — a complaint about correct script.
func TestTheCountedLoopAndTheMenuAreLoops(t *testing.T) {
	for _, c := range []struct{ name, src, stdin, ran string }{
		{"a counted loop", "repeat 3; do echo x; break; done\necho after\n", "", "x"},
		// A reply, so the body actually runs: a menu given nothing to read
		// ends at once and would have proved nothing about the count.
		{"a menu", "select v in a b; do echo got; break; done\necho after\n", "1\n", "got"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := loopControlRunWith(t, c.src, Yes, "%[1]s: not in a loop", func(d *syntax.Dialect) {
				d.Repeat = true
				d.Select = true
				d.ShortForm = true
			}, c.stdin)
			if strings.Contains(out, "not in a loop") {
				t.Errorf("said %q, want nothing refused", out)
			}
			if !strings.Contains(out, c.ran) {
				t.Errorf("said %q, want the body to have run", out)
			}
			if !strings.Contains(out, "after") {
				t.Errorf("said %q, want the script to carry on", out)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// A shell with no answer refuses rather than guessing, and says which
// question it could not answer.
func TestLoopControlWithNoAnswerRecordedIsRefused(t *testing.T) {
	out, _ := loopControlRun(t, "break\necho after\n", Unspecified, "")
	if !strings.Contains(out, "break") || !strings.Contains(out, "disagree") {
		t.Errorf("said %q, want the axis named", out)
	}
}

func loopControlRun(t *testing.T, src string, fatal Answer, wording string) (string, int) {
	t.Helper()
	return loopControlRunWith(t, src, fatal, wording, nil, "")
}

func loopControlRunWith(t *testing.T, src string, fatal Answer, wording string,
	enable func(*syntax.Dialect), stdin string,
) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.LoopControlOutsideALoopIsFatal = fatal
	// The answer that makes a fatal error 1 rather than 2, so that the status
	// a stopping row asserts is a number this test chose.
	sem.FatalErrorStatusIsOne = Yes
	dg := Diagnostics{LoopControlOutsideALoop: wording}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Stdin: strings.NewReader(stdin),
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
	})
	d := syntax.Core()
	if enable != nil {
		enable(&d)
	}
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}
