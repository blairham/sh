// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// substFailureSem answers where a substitution body's refusal stops.
func substFailureSem(a Answer) Semantics {
	s := permissive()
	s.SubstitutionParseErrorIsFatal = a
	return s
}

// The older substitution's body is unescaped and re-lexed when the word is
// expanded, so a body that is not a program is a failure arriving mid-run.
// Where it stops is the axis: one column reports it, expands the word to the
// empty string and carries the statement and the script on, and the rest
// abandon the input (#2703).
func TestWhereASubstitutionBodysRefusalStopsFollowsTheAxis(t *testing.T) {
	// The nesting written wrong: the escaped backquote opens an inner body
	// nothing closes, so the text the outer substitution hands on runs out
	// mid-substitution.
	const src = "echo A; echo \"`echo \\`echo n`\"; echo B"
	out, st := run(t, src, withSem(substFailureSem(No)))
	if st != 0 {
		t.Errorf("scoped to the word: status %d, want 0", st)
	}
	if !strings.HasPrefix(out, "A\n") || !strings.HasSuffix(out, "\n\nB\n") {
		t.Errorf("scoped to the word: got %q, want `A`, the complaint, an empty line and `B`", out)
	}
	out, st = run(t, src, withSem(substFailureSem(Yes)))
	if st == 0 {
		t.Errorf("fatal: status %d, want a refusal", st)
	}
	if strings.Contains(out, "\nB\n") {
		t.Errorf("fatal: got %q, want the script abandoned before `B`", out)
	}
}

// The word the refusal belongs to expands to the empty string rather than
// being dropped, which is what a reused conversion can see: a field that is
// gone prints no `[]` at all.
func TestAWordWhoseSubstitutionWasRefusedIsEmptyAndNotGone(t *testing.T) {
	const src = "printf '[%s]' a \"`echo \\`echo n`\" b; echo"
	out, st := run(t, src, withSem(substFailureSem(No)))
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if !strings.HasSuffix(out, "[a][][b]\n") {
		t.Errorf("got %q, want the refused word as an empty field between `a` and `b`", out)
	}
}

// And the status is the command's own rather than the failure's: nothing here
// sets a syntax status behind the statement that goes on to run.
func TestAScopedSubstitutionFailureLeavesTheStatusToTheCommand(t *testing.T) {
	out, st := run(t, "false; echo \"`echo \\`echo n`\" >/dev/null; echo \"st=$?\"",
		withSem(substFailureSem(No)))
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if !strings.HasSuffix(out, "st=0\n") {
		t.Errorf("got %q, want the `echo` that held the word to report its own 0", out)
	}
}

// A body refused inside the *newer* spelling is not this axis. The column
// that scopes the older one reads a `$( … )` body with the script, so that
// refusal is the line's there and is fatal — asking this axis for both would
// take one row and lose the other.
func TestTheNewerSpellingsRefusalIsFatalUnderEitherAnswer(t *testing.T) {
	const src = "echo one; echo $(if; then :; fi); echo two"
	for _, a := range []Answer{Yes, No} {
		out, st := run(t, src, withSem(substFailureSem(a)))
		if st == 0 {
			t.Errorf("%v: status %d, want a refusal", a, st)
		}
		if strings.Contains(out, "two") {
			t.Errorf("%v: got %q, want the script abandoned before `two`", a, out)
		}
	}
}
