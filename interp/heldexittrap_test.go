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

// heldTrapSemantics scopes EXIT to a function — which is what makes a call
// hold its caller's trap at all — and answers the axis under test.
func heldTrapSemantics(scoped, lose Answer) Semantics {
	s := permissive()
	s.ExitTrapIsFunctionLocal = scoped
	s.StopInsideACallLosesTheHeldExitTrap = lose
	s.ArithDivisionByZeroYieldsAValue = No
	return s
}

// A stop that ends the shell from inside a call, where the axis says so,
// leaves the trap the call held unrun; and the stops that do not end the shell
// where they stand, or a shell that holds nothing, keep it.
//
// Each row is run under both answers, because a row the axis cannot move is a
// control and says so: `moves` is false for exactly those. See
// Semantics.StopInsideACallLosesTheHeldExitTrap.
func TestAStopInsideACallLosesTheHeldExitTrapWhereTheAxisSays(t *testing.T) {
	const handler = "trap 'printf TRAP_RAN' EXIT\n"
	for _, c := range []struct {
		name   string
		src    string
		scoped Answer
		moves  bool
		// own is output the row writes under either answer.
		own string
	}{
		{"errexit inside the call", handler + "set -e\nf() { false; }\nf\nprintf after", Yes, true, ""},
		{"errexit two calls down", handler + "set -e\nf() { g; }\ng() { false; }\nf\nprintf after", Yes, true, ""},
		{"a parameter error inside the call", handler + "f() { : ${q?b}; }\nf\nprintf after", Yes, true, ""},
		{"the call's own trap still fires", handler + "set -e\nf() { trap 'printf OWN' EXIT; false; }\nf\nprintf after", Yes, true, "OWN"},
		// Controls.
		{"errexit at the top level", handler + "set -e\nfalse\nprintf after", Yes, false, ""},
		{"an error the shell reported inside the call", handler + "f() { : $((1/0)); }\nf\nprintf after", Yes, false, ""},
		{"exit inside the call", handler + "f() { exit 3; }\nf\nprintf after", Yes, false, ""},
		{"a shell that holds nothing", handler + "set -e\nf() { false; }\nf\nprintf after", No, false, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, lose := range []Answer{Yes, No} {
				out, _ := run(t, c.src, func(r *Runner) {
					sem := heldTrapSemantics(c.scoped, lose)
					r.Semantics = &sem
					r.Diagnostics = &Diagnostics{}
				})
				ran := strings.Contains(out, "TRAP_RAN")
				want := !c.moves || lose != Yes
				if ran != want {
					t.Errorf("lose=%v: trap ran=%v, want %v — got %q", lose, ran, want, out)
				}
				if c.own != "" && !strings.Contains(out, c.own) {
					t.Errorf("lose=%v: the call's own trap should fire either way, got %q", lose, out)
				}
				if strings.Contains(out, "after") {
					t.Errorf("lose=%v: the shell should have ended, got %q", lose, out)
				}
			}
		})
	}
}

// A stop that is caught gets the held trap back: a typed line given up over
// a parameter error inside a call leaves the session open, and the trap the
// call held is standing again and runs when the session ends.
//
// This is the half that keeps the axis from taking a person's handler away
// for the rest of a session over one line — measured at a prompt, where the
// same line in the same shell leaves `trap` listing the handler afterwards.
func TestACaughtStopPutsTheHeldExitTrapBack(t *testing.T) {
	var buf bytes.Buffer
	sem := heldTrapSemantics(Yes, Yes)
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: t.TempDir(), Name: "testsh",
	})
	ctx := context.Background()
	for _, line := range []string{"trap 'printf TRAP_RAN' EXIT\nf() { : ${q?b}; }\n", "f\n"} {
		f, err := syntax.Parse(line, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", line, err)
		}
		if err := r.RunPart(ctx, f); err != nil {
			t.Fatalf("run %q: %v", line, err)
		}
	}
	if !r.GiveUpTheLine() {
		t.Fatal("the parameter error was not there to give up; the row tests nothing")
	}
	if r.Exited() {
		t.Fatal("the session ended; a caught stop should leave it open")
	}
	r.Finish(ctx)
	if !strings.Contains(buf.String(), "TRAP_RAN") {
		t.Errorf("the held trap should be back once the line was given up, got %q", buf.String())
	}
}
