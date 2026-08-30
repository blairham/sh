// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"path/filepath"
	"testing"
)

// These three were found in the *wording* bucket of the conformance run:
// cases whose exit status matched, so the behavioural score called them
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
	for _, tc := range []struct {
		name string
		sem  Semantics
		want string
	}{
		{"bash", BashSemantics(), "1 2 3\n"},
		{"ksh93", KshSemantics(), "1 2 3\n"},
		{"zsh", ZshSemantics(), "1 2 3\n"},
		// dash has no brace expansion, so the word is a literal.
		{"dash", DashSemantics(), "{1..3}\n"},
	} {
		if got, _ := run(t, `echo {1..3}`, withSem(tc.sem)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
	if _, st := run(t, `echo {1..3}`, withSem(CoreSemantics())); st != 2 {
		t.Error("the core should refuse brace expansion")
	}
}

func TestBracketCaretIsADialectQuestion(t *testing.T) {
	const src = `case d in [^abc]) echo caret;; *) echo no-caret;; esac`
	for _, tc := range []struct {
		name string
		sem  Semantics
		want string
	}{
		{"bash", BashSemantics(), "caret\n"},
		{"zsh", ZshSemantics(), "caret\n"},
		// dash reads `^` as an ordinary member, so `[^abc]` matches a caret
		// and `d` falls through. Both answers are matches, on different
		// inputs, with nothing to warn on.
		{"dash", DashSemantics(), "no-caret\n"},
	} {
		if got, _ := run(t, src, withSem(tc.sem)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
	// `!` is the portable negation and needs no answer from anyone.
	for _, sem := range []Semantics{DashSemantics(), BashSemantics(), CoreSemantics()} {
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
