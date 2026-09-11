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

// A dialect's option seams are handed the shell that is *running*, not the one
// they were installed on (#1855).
//
// A subshell is a cloned Runner and the three fields are copied into it, so a
// dialect that closed over the runner it registered against would read and
// write the parent's options from inside every subshell — which is what the
// dialect with an option namespace of its own did. It is the same rule a
// builtin already follows, and it is the reason these are functions taking a
// runner rather than plain closures.
//
// The store here is keyed by the runner handed in, which is the only way the
// test can tell the two apart: a seam reading the installing runner would find
// the top-level entry from inside the subshell and every assertion below would
// read the wrong shell's state.

// seamShell is a miniature dialect with one option, `gamma`, stored per
// runner. Registered once on the top-level shell, exactly as a dialect
// registers: nothing re-registers when a subshell is cloned.
type seamShell struct {
	state map[*Runner]bool
	asked []*Runner
}

func (s *seamShell) get(r *Runner) bool {
	s.asked = append(s.asked, r)
	return s.state[r]
}

func (s *seamShell) register(r *Runner) {
	s.state = map[*Runner]bool{}
	r.SetOptionNamespace(func(r *Runner, name string) (on, known bool) {
		if name != "gamma" {
			return false, false
		}
		return s.get(r), true
	})
	r.SetOptionTable(
		func(r *Runner) []ListedOption {
			return []ListedOption{{Name: "gamma", On: s.get(r)}}
		},
		func(r *Runner, name string, on bool) (moved, known bool) {
			if name != "gamma" {
				return false, false
			}
			s.asked = append(s.asked, r)
			s.state[r] = on
			return true, true
		},
	)
}

// runSeamShell runs src in a shell with the miniature dialect installed, and
// hands back its output and the runners its seams were asked about.
func runSeamShell(t *testing.T, src string) (out string, top *Runner, asked []*Runner) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh"})
	shell := &seamShell{}
	shell.register(r)
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return buf.String(), r, shell.asked
}

// TestTheOptionSeamsAreHandedTheRunningShell: the condition operator, the
// listing and the mover, each asked inside a subshell, are handed a runner
// that is not the one the dialect registered against.
func TestTheOptionSeamsAreHandedTheRunningShell(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"the condition operator", `( [[ -o gamma ]] )`},
		{"the listing", `( set +o >/dev/null )`},
		{"the mover", `( set -o gamma )`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, top, asked := runSeamShell(t, tc.src)
			if len(asked) == 0 {
				t.Fatal("the seam was never asked")
			}
			for _, r := range asked {
				if r == top {
					t.Errorf("the seam was handed the shell it was installed on, want the subshell")
				}
			}
		})
	}
}

// And the state that follows from it: an option moved inside a subshell is the
// subshell's, so the listing inside reports it on and the one after reports it
// off.
//
// This is the half a seam that merely reported the wrong shell would still get
// wrong in the other direction — the mover wrote the *parent's* option and left
// the subshell's alone, so the change both failed to apply where it was asked
// and escaped to where it was not.
func TestAnOptionMovedInASubshellIsTheSubshellsOwn(t *testing.T) {
	out, _, _ := runSeamShell(t, "( set -o gamma; set +o ); set +o\n")
	const want = "set -o gamma\nset +o gamma\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
