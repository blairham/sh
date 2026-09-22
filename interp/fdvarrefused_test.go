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

// runFdVarRefused runs src in a shell that picks the descriptor for a
// `{name}` redirection and leaves it open past the command, which is the pair
// of answers the cases below are written against.
func runFdVarRefused(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.FdVariableOutlivesTheCommand = Yes
	// The refusal under test is the one a readonly name makes, so the axis
	// that decides whether such an assignment gives up the function has to be
	// answered: the point of these cases is that a *redirection* does not
	// carry that transfer however it is answered.
	sem.ReadonlyReassignmentFatal = Yes
	var out strings.Builder
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "testsh",
		Dir: dir, Stdout: &out, Stderr: &out,
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), st
}

// A `{name}` redirection whose name will not take the number must not leave
// the descriptor it picked behind: the number is unreachable — nothing
// published it — so a descriptor kept there is one the shell can never give
// back, and every number a script picks afterwards moves.
func TestARefusedFdVariableStoreGivesTheNumberBack(t *testing.T) {
	dir := t.TempDir()
	out, _ := runFdVarRefused(t, dir, `readonly v=42
exec {v}> one
exec {v}> two
exec {q}> three
echo "q=$q"`)
	if !strings.Contains(out, "q=10") {
		t.Errorf("two refused redirections moved the next allocation: %q", out)
	}
}

// And the refusal belongs to the redirection rather than to the shell: a
// store the name refuses fails the command in front of it and leaves what
// follows to run, where the same assignment written out would give up the
// function it is in.
func TestARefusedFdVariableStoreDoesNotGiveUpTheFunction(t *testing.T) {
	dir := t.TempDir()
	out, _ := runFdVarRefused(t, dir, `readonly v=42
f() { exec {v}> one; echo reached; }
f
echo after`)
	if !strings.Contains(out, "reached") {
		t.Errorf("the function was given up by a refused redirection: %q", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("the script was given up by a refused redirection: %q", out)
	}
}

// The session switch takes such a descriptor back when the command ends,
// which is the permission a dialect names over
// Semantics.FdVariableOutlivesTheCommand. It can only turn the axis down, so
// the number is free again and a later read through it finds nothing.
func TestTheSessionSwitchClosesAPickedDescriptorWithItsCommand(t *testing.T) {
	dir := t.TempDir()
	d := syntax.Core()
	f, err := syntax.Parse(`echo one {a}> first
echo "a=$a"
echo two {b}> second
echo "b=$b"`, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sem := permissive()
	sem.FdVariableOutlivesTheCommand = Yes
	var out strings.Builder
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "testsh",
		Dir: dir, Stdout: &out, Stderr: &out,
	})
	if !r.FdVariableDescriptorOutlivesTheCommand() {
		t.Fatal("a Runner that was never told should leave the descriptor open")
	}
	r.SetFdVariableDescriptorOutlivesTheCommand(false)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	// The second redirection takes the first one's number back, which is the
	// whole observable: with the descriptor left open it would be 11.
	if o := out.String(); !strings.Contains(o, "a=10") || !strings.Contains(o, "b=10") {
		t.Errorf("the descriptor outlived its command with the switch off: %q", out.String())
	}
}
