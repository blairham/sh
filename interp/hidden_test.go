// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// These three were found in the *wording* bucket of the conformance run:
// cases whose exit status matched, so the behavioral score called them
// agreements, and whose output differed for reasons that had nothing to do
// with diagnostics. A score that compares only the status cannot see them.

func TestRedirectionWithNoCommandStillOpensTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b")
	if _, st := run(t, `> `+path, nil); st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("`> file` did not create the file: %v", err)
	}
	// And it truncates an existing one.
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, `> `+path, nil)
	if b, _ := os.ReadFile(path); len(b) != 0 {
		t.Errorf("`> file` did not truncate: %q", b)
	}
}

func TestBraceExpansionIsADialectQuestion(t *testing.T) {
	// The BraceExpansion axis: on one side the word expands, on the other it
	// is a literal. Both answers are quiet, so both are asserted.
	expands := permissive()
	expands.BraceExpansion = Yes
	if got, _ := run(t, `echo {1..3}`, withSem(expands)); got != "1 2 3\n" {
		t.Errorf("Yes: got %q, want %q", got, "1 2 3\n")
	}
	literal := permissive()
	literal.BraceExpansion = No
	if got, _ := run(t, `echo {1..3}`, withSem(literal)); got != "{1..3}\n" {
		t.Errorf("No: got %q, want %q", got, "{1..3}\n")
	}
	if _, st := run(t, `echo {1..3}`, withSem(CoreSemantics())); st != 2 {
		t.Error("the core should refuse brace expansion")
	}
}

func TestBracketCaretIsADialectQuestion(t *testing.T) {
	// The BracketCaretNegates axis: `^` negates the class, or is an ordinary
	// member — so `[^abc]` against `d` matches on one side and falls through
	// on the other. Both answers are matches, on different inputs, with
	// nothing to warn on.
	const src = `case d in [^abc]) echo caret;; *) echo no-caret;; esac`
	negates := permissive()
	negates.BracketCaretNegates = Yes
	if got, _ := run(t, src, withSem(negates)); got != "caret\n" {
		t.Errorf("Yes: got %q, want %q", got, "caret\n")
	}
	member := permissive()
	member.BracketCaretNegates = No
	if got, _ := run(t, src, withSem(member)); got != "no-caret\n" {
		t.Errorf("No: got %q, want %q", got, "no-caret\n")
	}
	// `!` is the portable negation and needs no answer from anyone.
	for _, sem := range []Semantics{negates, member, CoreSemantics()} {
		if got, _ := run(t, `case d in [!abc]) echo neg;; *) echo no;; esac`, withSem(sem)); got != "neg\n" {
			t.Errorf("[!abc] should negate everywhere, got %q", got)
		}
	}
	// A class without a caret is never questioned, so the core can match it.
	if got, _ := run(t, `case b in [abc]) echo in;; *) echo out;; esac`, withSem(CoreSemantics())); got != "in\n" {
		t.Errorf("core: got %q, want %q", got, "in\n")
	}
	if _, st := run(t, src, withSem(CoreSemantics())); st != 2 {
		t.Error("the core should refuse a caret class")
	}
}
