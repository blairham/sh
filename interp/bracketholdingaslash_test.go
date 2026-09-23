// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A bracket expression written across a separator, under both readings of
// Semantics.BracketHoldingASlashIsStillABracket.
//
// No metacharacter matches a `/`, so such a bracket can never match. One
// reading takes that as the bracket not being a bracket — its `[` is an
// ordinary character, and a word holding nothing else live is not a pattern at
// all — and the other takes it as a bracket that matches nothing. Nothing tells
// the two apart until something happens to a pattern that *matched nothing*,
// which is why the rows below turn on UnmatchedPatternIsEmpty rather than
// looking at the expansion.
//
// The rows name the axis and no shell. What each column prints is in
// dialect/bash/bracketholdingaslash_test.go and its zsh sibling.
func TestABracketHoldingASlashIsStillABracket(t *testing.T) {
	dir := t.TempDir()
	// A name `[zQ]` could match and a name `[a/b]` cannot exist, so the second
	// field of each row is the control: a bracket that is a bracket under both
	// readings, and has nothing to match.
	if err := os.WriteFile(filepath.Join(dir, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		reading Answer
		// `[a/b]` and `[a\/b]`, each beside the control, with an unmatched
		// pattern deleted.
		live, quoted string
	}{
		// A bracket that holds a separator is one, so the word is a pattern,
		// it matches nothing, and the deletion takes it.
		{"a separator leaves it a bracket", Yes, `[0]`, `[0]`},
		// It is not one, so the word carries no live metacharacter, is not a
		// pattern, and the deletion has nothing to delete. The quoted
		// separator is the control that keeps this about the *live* one.
		{"a separator is not a bracket's", No, `[1][[a/b]]`, `[0]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ask := func(s *Semantics) { s.BracketHoldingASlashIsStillABracket = tc.reading }
			for _, row := range []struct{ src, want string }{
				{`set -- [a/b]; printf '[%s]' "$#" "$@"`, tc.live},
				{`set -- [a\/b]; printf '[%s]' "$#" "$@"`, tc.quoted},
			} {
				out, st := axisGlobRun(t, dir, row.src, ask)
				if out != row.want || st != 0 {
					t.Errorf("%v: %q said %q (status %d), want %q at 0",
						tc.reading, row.src, out, st, row.want)
				}
			}
		})
	}
}

// The separator still separates, whichever way the axis is answered — which is
// what the mark on it commits to and the reason splitFieldParts exists.
//
// A quoted `/` is marked in the escaped field so that the bracket question can
// see it, and a mark elsewhere in that form means "this was quoted, so it is a
// character". A separator cannot be made a character, so a reader that took the
// mark at face value would lose the component boundary — and every one of these
// words would become a single name with a backslash in it.
func TestAQuotedSeparatorStillSeparates(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "qwe"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qwe", "rty"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ src, want string }{
		{`printf '[%s]' "qwe"/rty`, `[qwe/rty]`},
		{`printf '[%s]' "qwe/rty"`, `[qwe/rty]`},
		{`printf '[%s]' qwe"/"rty`, `[qwe/rty]`},
		{`v=qwe; printf '[%s]' "$v/rty"`, `[qwe/rty]`},
		{`printf '[%s]' "qwe"/[r]ty`, `[qwe/rty]`},
		{`printf '[%s]' "qwe"/*`, `[qwe/rty]`},
		{`printf '[%s]' "qwe"/`, `[qwe/]`},
	} {
		for _, reading := range []Answer{Yes, No} {
			ask := func(s *Semantics) { s.BracketHoldingASlashIsStillABracket = reading }
			out, st := axisGlobRun(t, dir, tc.src, ask)
			if out != tc.want || st != 0 {
				t.Errorf("%v: %q said %q (status %d), want %q at 0", reading, tc.src, out, st, tc.want)
			}
		}
	}
}

// axisGlobRun is axisRunIn with an unmatched pattern deleted, which is the one
// arrangement that can see this axis at all.
func axisGlobRun(t *testing.T, dir, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	sem := testSemantics()
	set(&sem)
	return run(t, src, func(r *Runner) {
		r.Semantics, r.Dir = &sem, dir
		r.SetMatchOption(UnmatchedPatternIsEmpty, true)
	})
}
