// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// runWithExecFdAnswer runs src with one answer to whether a descriptor
// `exec` parked reaches what the shell runs, and hands back what the script
// printed.
func runWithExecFdAnswer(t *testing.T, dir, src string, reaches Answer, inherited []*os.File) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.ExecOpenedFdReachesACommand = reaches
	var out strings.Builder
	r := &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "testsh",
		Dir: dir, Env: testPATH(), Stdout: &out, Stderr: &out,
		InheritedFiles: inherited,
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out.String()
}

// The axis itself: one answer hands the descriptor over and the other keeps
// it, for the same script. The flock and shared-log idioms are the first
// answer; a shell that closes anything above 2 that `exec` opened before it
// runs a program is the second.
func TestADescriptorExecParkedIsHandedOverOrNotByTheAxis(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  Answer
		wantsIt bool
	}{
		{"handed over", Yes, true},
		{"kept by the shell", No, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "f")
			out := runWithExecFdAnswer(t, dir,
				`exec 3>`+target+`; /bin/sh -c 'echo child >&3' 2>/dev/null; exec 3>&-`,
				tc.answer, nil)
			b, _ := os.ReadFile(target)
			if got := string(b) == "child\n"; got != tc.wantsIt {
				t.Errorf("file = %q, want the child's line to be there: %v (output %q)",
					b, tc.wantsIt, out)
			}
		})
	}
}

// The boundary, which is what makes this an axis about `exec` rather than
// about handing descriptors over at all. Under the answer that keeps them:
//
//   - a command's own redirection still crosses, because it is the command's
//     and not `exec`'s;
//   - and restating the number on the command brings an `exec` descriptor
//     back, which is the same rule read from the other side.
//
// Both are unanimous across the panel, so neither may move with the axis.
func TestACommandsOwnRedirectionCrossesWhicheverWayTheAxisIsAnswered(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		dir := t.TempDir()
		own := filepath.Join(dir, "own")
		out := runWithExecFdAnswer(t, dir,
			`/bin/sh -c 'echo own >&3' 3>`+own+` 2>/dev/null`, answer, nil)
		if b, _ := os.ReadFile(own); string(b) != "own\n" {
			t.Errorf("answer %v: a command's own redirection did not cross: %q (output %q)",
				answer, b, out)
		}

		// And the number goes back to being `exec`'s when that command
		// ends, which is what makes this "for that command" rather than a
		// mark the script can knock off for good. Measured: under the
		// answer that keeps them, a plain child after the restating one
		// finds 3 closed again.
		restated := filepath.Join(dir, "restated")
		out = runWithExecFdAnswer(t, dir,
			`exec 3>`+restated+`; /bin/sh -c 'echo restated >&3' 3>&3 2>/dev/null
/bin/sh -c 'echo after >&3' 2>/dev/null
exec 3>&-`,
			answer, nil)
		b, _ := os.ReadFile(restated)
		if !strings.Contains(string(b), "restated") {
			t.Errorf("answer %v: restating the number did not hand the descriptor over: %q (output %q)",
				answer, b, out)
		}
		if got := strings.Contains(string(b), "after"); got != (answer == Yes) {
			t.Errorf("answer %v: the command after the restating one wrote: %v, want %v (%q)",
				answer, got, answer == Yes, b)
		}
	}
}

// And a descriptor the *caller* opened crosses whichever way it is answered,
// because it was never `exec`'s. Unanimous across the panel, this shell's
// dissenter included, which is what says the axis is about what `exec`
// opened rather than about what the table holds.
func TestAnInheritedDescriptorCrossesWhicheverWayTheAxisIsAnswered(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		dir := t.TempDir()
		target := filepath.Join(dir, "inherited")
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		// Entry 0 is descriptor 3, exactly as a front end hands one in.
		out := runWithExecFdAnswer(t, dir,
			`/bin/sh -c 'echo inherited >&3' 2>/dev/null`, answer, []*os.File{f})
		_ = f.Close()
		if b, _ := os.ReadFile(target); string(b) != "inherited\n" {
			t.Errorf("answer %v: an inherited descriptor did not cross: %q (output %q)",
				answer, b, out)
		}
	}
}
