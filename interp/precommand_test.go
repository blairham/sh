// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The precommand-modifier table, from the core's side. A runner with no table
// scans nothing and the words are ordinary command names, which is what the
// four shells without the family do; a runner given a table reads the leading
// words before any of them is matched.
//
// The names here are the test's own rather than any dialect's — the core has
// no table of its own and cannot see one — so what is checked is the rule and
// not the roster. See dialect/zsh/precommand_test.go for the measured words.

func TestAnUnregisteredWordIsAnOrdinaryCommandName(t *testing.T) {
	dir := fileDir(t, "a.txt")
	inDir := func(r *Runner) { r.Dir = dir }
	// No table: the word is a command name, and the words behind it were
	// matched on the way to it — which is the bug this class exists to fix,
	// kept here as the control that says the table is doing the work.
	out, st := run(t, `nomatch echo *.txt`, inDir)
	if !strings.Contains(out, "nomatch") || st == 0 {
		t.Errorf("with no table: %q (status %d), want a refusal naming nomatch", out, st)
	}
	// And the word carries nothing with it: written where its result can be
	// seen, it is one more argument and the pattern beside it still matched.
	if out, _ := run(t, `echo nomatch *.txt`, inDir); strings.TrimSpace(out) != "nomatch a.txt" {
		t.Errorf("with no table: %q, want %q", out, "nomatch a.txt")
	}
}

func TestASuppressingModifierIsTakenAwayAndStopsTheMatch(t *testing.T) {
	dir := fileDir(t, "a.txt")
	withTable := func(r *Runner) {
		r.Dir = dir
		r.SetPrecommand("nomatch", PrecommandNoGlob)
		// The core's own `builtin`, named as transparent: it keeps its
		// work and the scan carries on past it, which is the difference
		// between the two constants.
		r.SetPrecommand("builtin", PrecommandTransparent)
	}
	for _, tc := range []struct{ src, want string }{
		// Taken away, and the words behind it stand as written.
		{`nomatch echo *.txt`, "*.txt"},
		{`nomatch echo *.txt b*`, "*.txt b*"},
		// The control, one line along: the same runner still matches.
		{`echo *.txt`, "a.txt"},
		// It is read after expansion, so an expansion can produce it and
		// quoting does not take it away.
		{`c=nomatch; $c echo *.txt`, "*.txt"},
		{`"nomatch" echo *.txt`, "*.txt"},
		// Repeats, and a transparent word the scan reads past: the word
		// stays and does its own work, and a modifier may stand behind it.
		{`nomatch nomatch echo *.txt`, "*.txt"},
		{`builtin nomatch echo *.txt`, "*.txt"},
		{`builtin echo *.txt`, "a.txt"},
		// An unregistered word ends the scan, so a modifier behind it is an
		// ordinary word again and the match happens.
		{`echo nomatch *.txt`, "nomatch a.txt"},
		// The scan is over fields and not over words: one word can produce
		// several, and the front of *that* list is what carries it.
		{`set -- nomatch echo; $@ *.txt`, "*.txt"},
		{`set -- nomatch nomatch echo; $@ *.txt`, "*.txt"},
		// The control for the two rows above, which is what says the rule
		// is about fields rather than about text: one field holding both
		// words is a command name and not a modifier.
		{`set -- "nomatch echo"; "$@" *.txt`, "sh: nomatch echo: not found"},
		// Nothing behind it: a command that runs nothing.
		{`nomatch; echo done`, "done"},
	} {
		if out, _ := run(t, tc.src, withTable); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// TestASuppressedMatchIsTheWordsAndNotTheirInsides: the modifier covers the
// command's own words. What a substitution inside one of them does is that
// command's business, and a redirection target is expanded by another route
// entirely.
func TestASuppressedMatchIsTheWordsAndNotTheirInsides(t *testing.T) {
	dir := fileDir(t, "a.txt")
	withTable := func(r *Runner) {
		r.Dir = dir
		r.SetPrecommand("nomatch", PrecommandNoGlob)
	}
	for _, tc := range []struct{ src, want string }{
		{`nomatch echo "$(echo *.txt)"`, "a.txt"},
		{`f() { echo *.txt; }; nomatch f`, "a.txt"},
		{`nomatch eval 'echo *.txt'`, "a.txt"},
	} {
		if out, _ := run(t, tc.src, withTable); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// TestAModifierDoesNotMoveTheCommandOutOfThisShell. The second place that
// asks what a command's name is reads the written words rather than the
// expanded ones, and it has to strip the same table — otherwise the last
// element of a pipeline stops looking like a builtin and runs where its
// assignments cannot be seen.
//
// The axis has to be answered here because the core does not answer it, and
// the pair is the assertion: the same runner, with and without the modifier,
// has to reach the same variable.
func TestAModifierDoesNotMoveTheCommandOutOfThisShell(t *testing.T) {
	withTable := func(r *Runner) {
		r.SetPrecommand("nomatch", PrecommandNoGlob)
		s := *r.Semantics
		s.LastPipelineElementInCurrentShell = Yes
		r.Semantics = &s
	}
	for _, tc := range []struct{ src, want string }{
		{`echo a | read x; echo "x=[$x]"`, "x=[a]"},
		{`echo a | nomatch read x; echo "x=[$x]"`, "x=[a]"},
	} {
		if out, _ := run(t, tc.src, withTable); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}
