// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runDisowning runs src with `&!` and `&|` in the grammar, and hands back the
// runner it ran on so a test can ask what the job table holds.
func runDisowning(t *testing.T, src string) (out string, status int, r *Runner) {
	t.Helper()
	out, status = runGrammar(t, src,
		func(d *syntax.Dialect) { d.BackgroundAndDisown = true },
		func(got *Runner) { r = got })
	return out, status, r
}

// TestADisownedJobIsNotInTheTable — `&!` starts the job and lets go of it, so
// nothing lists it and the next ordinary background job is `[1]` rather than
// `[2]`. Measured on zsh 5.9.2, 2026-09-06.
//
// Asserted against the job table rather than against a `jobs` listing,
// because what a listing looks like is four other tests' question and this
// one is about whether the job is in it at all.
func TestADisownedJobIsNotInTheTable(t *testing.T) {
	out, st, r := runDisowning(t, "true &!")
	if st != 0 || out != "" {
		t.Errorf("out %q status %d, want a silent 0 — starting a job succeeds", out, st)
	}
	if n := len(r.Jobs()); n != 0 {
		t.Errorf("a disowned job left %d entries in the table, want 0", n)
	}
	// The same statement with a plain `&`, which is the pair this change
	// could have collapsed: without it the assertion above passes for a
	// shell that lost the table altogether.
	_, _, r = runDisowning(t, "true &")
	if n := len(r.Jobs()); n != 1 {
		t.Errorf("an ordinary background job left %d entries, want 1", n)
	}
	// And the number the next job gets, which is a position in the table
	// rather than a counter: a disowned job never took one.
	_, _, r = runDisowning(t, "true &!\ntrue &")
	if n := len(r.Jobs()); n != 1 {
		t.Errorf("after a disowned job and an ordinary one the table holds %d, want 1", n)
	}
}

// TestADisownedJobIsStillTheLastOne — the half that does not change. `$!` is
// the disowned job's process, which is what a script meaning to signal it
// later reads: letting go of a job is about the table and not the process.
func TestADisownedJobIsStillTheLastOne(t *testing.T) {
	out, st, _ := runDisowning(t, "sleep 0 &!\necho \"pid=${!:+set}\"")
	if st != 0 || !strings.Contains(out, "pid=set") {
		t.Errorf("out %q status %d, want $! set after a disowned job", out, st)
	}
	// And it moves on to the next one, disowned or not. Real commands
	// rather than builtins on both sides: a builtin's job has no process,
	// so `$!` is 0 for each of them and the two would compare equal for a
	// reason that has nothing to do with the disowning.
	out, _, _ = runDisowning(t, "sleep 0 &!\na=$!\nsleep 0 &\nb=$!\n"+
		`if [ "$a" != "$b" ]; then echo moved; else echo same; fi`)
	if !strings.Contains(out, "moved") {
		t.Errorf("out %q, want $! to have moved to the second job", out)
	}
}

// TestBothSpellingsDisown — `&!` and `&|` are one operator, so nothing after
// the parser can tell them apart.
func TestBothSpellingsDisown(t *testing.T) {
	for _, op := range []string{"&!", "&|"} {
		_, st, r := runDisowning(t, "true "+op)
		if st != 0 {
			t.Errorf("%s: status %d, want 0", op, st)
		}
		if n := len(r.Jobs()); n != 0 {
			t.Errorf("%s: left %d jobs in the table, want 0", op, n)
		}
	}
}
