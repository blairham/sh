// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"
)

func TestLastPipelineElementRunsWhereTheDialectSays(t *testing.T) {
	// The axis had a value in every preset and nothing read it. Asserting
	// both sides is the point: taking the majority silently is what it did
	// before, and that looks identical to an implementation on one side.
	const src = `echo x | read v; echo "[$v]"`
	for _, tc := range []struct {
		name string
		sem  Semantics
		want string
	}{
		{"ksh93", ksh.Semantics(), "[x]\n"},
		{"zsh", zsh.Semantics(), "[x]\n"},
		{"dash", dash.Semantics(), "[]\n"},
		{"bash", bash.Semantics(), "[]\n"},
	} {
		if got, _ := run(t, src, withSem(tc.sem)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestPipelineAxisIsAskedOnlyWhenItShows is what keeps the core usable. The
// answer cannot be observed through an external command, so an ordinary
// pipeline must not be refused for want of a dialect.
func TestPipelineAxisIsAskedOnlyWhenItShows(t *testing.T) {
	if got, st := run(t, `echo x | cat`, withSem(CoreSemantics())); got != "x\n" || st != 0 {
		t.Errorf("a plain pipeline should need no answer: got %q status %d", got, st)
	}
	// A builtin at the end can touch the shell, so the core must refuse.
	out, st := run(t, `echo x | read v`, withSem(CoreSemantics()))
	if st != 2 || !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("an observable last element should be refused: got %q status %d", out, st)
	}
	// So can a brace group.
	if _, st := run(t, `echo x | { read v; }`, withSem(CoreSemantics())); st != 2 {
		t.Errorf("a group should be refused, status %d", st)
	}
	// A subshell is already one, so the axis changes nothing.
	if _, st := run(t, `echo x | (cat)`, withSem(CoreSemantics())); st != 0 {
		t.Errorf("a subshell needs no answer, status %d", st)
	}
}

// TestPipelineStillPipes guards the plumbing the axis rearranged: the last
// element reads the pipe whichever shell it runs in.
func TestPipelineStillPipes(t *testing.T) {
	for _, tc := range []struct {
		name string
		sem  Semantics
	}{{"subshell side", bash.Semantics()}, {"current-shell side", ksh.Semantics()}} {
		if got, _ := run(t, `printf 'a\nb\n' | grep b`, withSem(tc.sem)); got != "b\n" {
			t.Errorf("%s: got %q, want %q", tc.name, got, "b\n")
		}
		if got, _ := run(t, `echo one | cat | cat`, withSem(tc.sem)); got != "one\n" {
			t.Errorf("%s: three elements: got %q", tc.name, got)
		}
	}
}
