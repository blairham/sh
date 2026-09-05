// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// remedy is what a binary with a dialect flag would say. It names no shell,
// because this package may not: what matters here is that whatever the binary
// supplied comes out the other end, not what it happens to contain.
const remedy = "choose one with -whatever"

// TestARefusalCarriesTheRemedyTheBinarySupplied is the half of the diagnostic
// that used to be missing. Refusing an axis nothing answered is correct and
// stays; saying so without saying what to do about it is half a message, and
// the half only the front end can write — this package has no flags and interp
// may not name a shell.
//
// Both routes to a refusal are checked, because they are written in different
// places: the front end refuses the invocation itself for `-c` with `-s`, and
// the interpreter refuses the twenty-odd axes a script can reach.
func TestARefusalCarriesTheRemedyTheBinarySupplied(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{
			// The front end's own: which operand is $0 when both `-c` and
			// `-s` are given. Refused before anything runs, so there is no
			// runner to have carried the string.
			name: "the invocation itself",
			argv: []string{"testsh", "-sc", "echo hi", "name", "a"},
		},
		{
			// The interpreter's: `test a == a` is an axis, and reaching it
			// means the string traveled through the runner this front end
			// built.
			name: "an axis a script reached",
			argv: []string{"testsh", "-c", "test a == a"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sh := shell()
			sh.AxisRemedy = remedy
			_, errs, _ := runArgs(t, sh, tc.argv...)
			if !strings.Contains(errs, "no dialect was chosen") {
				t.Fatalf("stderr = %q, want the unanswered axis named", errs)
			}
			if !strings.Contains(errs, remedy) {
				t.Errorf("stderr = %q, want it to carry the remedy %q", errs, remedy)
			}

			// And silent by default. A dialect binary sets nothing here, and
			// a refusal from one is a hole in its vector rather than a flag
			// its user forgot — so there is nothing honest for it to say.
			_, bare, _ := runArgs(t, shell(), tc.argv...)
			if !strings.Contains(bare, "no dialect was chosen") {
				t.Fatalf("stderr = %q, want the unanswered axis named", bare)
			}
			if strings.Contains(bare, remedy) {
				t.Errorf("stderr = %q: a shell that supplied no remedy must invent none", bare)
			}
			if strings.Contains(bare, ";") {
				t.Errorf("stderr = %q: the separator belongs to the remedy, not to the refusal", bare)
			}
		})
	}
}

// TestTheRemedyReachesASubshell pins that the string is a fact about the
// invocation rather than about one runner. A subshell is a cloned Runner, and
// a refusal inside one reads to a person exactly as it does outside.
func TestTheRemedyReachesASubshell(t *testing.T) {
	t.Parallel()
	sh := shell()
	sh.AxisRemedy = remedy
	_, errs, _ := runArgs(t, sh, "testsh", "-c", "( test a == a )")
	if !strings.Contains(errs, remedy) {
		t.Errorf("stderr = %q, want the remedy inside a subshell too", errs)
	}
}

// TestTheRemedyIsCarriedNotComposed is the structural half. driver has no
// flags, so it cannot know what the fix is spelled like; a front end that
// composed one here would be composing it for every binary that imports this
// package, including the dialect binaries for which there is no fix.
func TestTheRemedyIsCarriedNotComposed(t *testing.T) {
	t.Parallel()
	var sh driver.Shell
	if sh.AxisRemedy != "" {
		t.Errorf("the zero Shell should offer no remedy, got %q", sh.AxisRemedy)
	}
}
