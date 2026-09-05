// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// The letters a shell starts with when it is interactive are a second startup
// vector, and the front end is the only thing that knows which of the two
// applies: being interactive is decided while the invocation is read and
// carried to the runner. A vector nothing reaches is a vector that is not
// there, which is the failure this package exists to make impossible — `-i`
// itself was dropped on the floor for every route with an operand (#472).
func TestTheInvocationChoosesBetweenTheTwoStartupVectors(t *testing.T) {
	sem := interp.PosixSemantics()
	// A set that keeps one letter, drops two and adds one, so nothing here
	// could pass by appending. The values are made up; which real shell
	// reports what is asserted in dialect/.
	sem.DefaultOptionLetters = "abc"
	sem.InteractiveOptionLetters = "aQ"

	script := writeScript(t, `echo "[$-]"`+"\n")
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		// `-i` is interactive on every route, prompt or no prompt — measured
		// unanimous — so the second vector applies to a script operand and to
		// a command string alike.
		{"a script operand", []string{"testsh", script}, "[abc]\n"},
		{"-i with a script operand", []string{"testsh", "-i", script}, "[aQi]\n"},
		// The `c` of the command-string route rides along on top of whichever
		// vector applied, which is the point: the vector says where `$-`
		// begins and the route letters follow it.
		{"a command string", []string{"testsh", "-c", `echo "[$-]"`}, "[abcc]\n"},
		{"-i with a command string", []string{"testsh", "-i", "-c", `echo "[$-]"`}, "[aQic]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			sh.Semantics = sem
			out, errs, code := runArgs(t, sh, tc.argv...)
			if code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// And a dialect with nothing separate to say keeps one answer for both, which
// is one of the four panel members' own.
func TestADialectWithOneStartupVectorUsesItEitherWay(t *testing.T) {
	sem := interp.PosixSemantics()
	sem.DefaultOptionLetters = "abc"
	sh := shell()
	sh.Semantics = sem
	out, errs, code := runArgs(t, sh, "testsh", "-i", "-c", `echo "[$-]"`)
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "[abcic]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The prompt route reaches it too, and by the same field: a prompt is
// interactive by definition, whether or not `-i` said so.
func TestAPromptTakesTheInteractiveStartupVector(t *testing.T) {
	sem := interp.PosixSemantics()
	sem.DefaultOptionLetters = "abc"
	sem.InteractiveOptionLetters = "aQ"
	sh := shell()
	sh.Semantics = sem
	out, _, _ := runPipedShell(t, sh, `echo "[$-]"`+"\n", "testsh", "-i")
	// `s` for the standard-input route rides along at a prompt, so this is a
	// containment check rather than an equality one.
	if want := "[aQis]"; !strings.Contains(out, want) {
		t.Errorf("out = %q, want %q in it", out, want)
	}
}
