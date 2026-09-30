// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// Measured under a pseudo-terminal on 2026-09-05, bash 5.3.15, with the screen
// reconstructed from the bytes rather than read off the raw stream — `C-r` is
// drawn incrementally, so the stream is the optimisation and the screen is the
// behavior. With four lines of history and `C-r e c h o` typed:
//
//	(reverse-i-search)`echo': echo two
//
// and after two more `C-r`, once nothing older matches:
//
//	(failed reverse-i-search)`echo': echo one
func TestHistoryStyleDrawsTheSearchWhereThePromptWas(t *testing.T) {
	s := bash.HistoryStyle()
	if got, want := s.SearchPrompt, "(reverse-i-search)`%s': "; got != want {
		t.Errorf("SearchPrompt = %q, want %q", got, want)
	}
	if got, want := s.SearchFailedPrompt, "(failed reverse-i-search)`%s': "; got != want {
		t.Errorf("SearchFailedPrompt = %q, want %q", got, want)
	}
	if s.SearchBelowTheLine {
		t.Error("this shell replaces the prompt rather than drawing under the line")
	}
}

// The two variables, and the answer that makes them a conflict rather than a
// spelling difference.
//
// Measured: `HISTCONTROL=ignorespace` and a line typed with a leading space
// leaves the file without it *and* leaves the session without it — the up
// arrow at the next prompt recalls the line before, and bash's own `history`
// cannot see the hidden one. zsh, asked the same thing, keeps it.
func TestHistoryStyleForgetsWhatItIgnores(t *testing.T) {
	s := bash.HistoryStyle()
	if got, want := s.Control, "HISTCONTROL"; got != want {
		t.Errorf("Control = %q, want %q", got, want)
	}
	if got, want := s.Ignore, "HISTIGNORE"; got != want {
		t.Errorf("Ignore = %q, want %q", got, want)
	}
	if s.IgnoreIsOnePattern {
		t.Error("HISTIGNORE is a colon-separated list, not one pattern")
	}
	if s.PatternIgnoredStaysInSession {
		t.Error("this shell drops an ignored line from the session as well as from the file")
	}
	// bash spells the other two rules in HISTCONTROL and has no option for
	// either. Naming one would give the dialect a `shopt` real bash ignores.
	if s.IgnoreSpaceOption != "" || s.IgnoreDupsOption != "" {
		t.Errorf("bash keeps these in HISTCONTROL, got %q and %q",
			s.IgnoreSpaceOption, s.IgnoreDupsOption)
	}
}

// A backslash in a history substitution's replacement is copied through as
// written here, which is the other reading of
// Semantics.HistorySubstitutionUnescapesTheReplacement.
//
// Inherited from the substrate rather than set in this file, so this row is
// what says the substrate's answer is still No: flipping the preset would
// change this shell and nothing in the zsh tests would notice.
//
// Measured 2026-09-30 at a prompt on 5.3.20 and 3.2.57 alike: after `echo one
// two one`, `!!:s/o/\\\\X/` written with four backslashes comes back with
// four, where zsh comes back with one.
func TestBashKeepsBackslashesInASubstitutionReplacement(t *testing.T) {
	if got := bash.Semantics().HistorySubstitutionUnescapesTheReplacement; got != interp.No {
		t.Errorf("HistorySubstitutionUnescapesTheReplacement is %v, want No", got)
	}
	sem, diag, d := bash.Semantics(), bash.Diagnostics(), bash.Dialect()
	r := &interp.Runner{
		Semantics: &sem, Diagnostics: &diag, Name: "bash",
		// Set because the guard in internal/dialecttest asks for it: a nil
		// Dialect is the core, so any nested parse would run as a shell this
		// row is not about.
		Dialect: &d,
	}
	bash.Apply(r)
	res, err := r.ExpandHistoryAlways(`echo !!:s/o/\\\\X/`, []string{"echo one two one"}, 1)
	if err != nil {
		t.Fatalf("expanding: %v", err)
	}
	if want := `echo ech\\\\X one two one`; res.Line != want {
		t.Errorf("expanded to %q, want %q", res.Line, want)
	}
}

// A `:h` or `:t` takes no count here: the bare modifier runs and the digits
// are ordinary text.
//
// Inherited from the substrate rather than set in this file, so this row is
// what holds the preset's No in place — flipping it would change this shell
// and nothing in the zsh tests would notice. Measured 2026-09-30 on 5.3.20
// and 3.2.57 alike: `!!:1:h2` over `/my/path/for/testing` is
// `/my/path/for2`.
func TestBashHeadAndTailTakeNoCount(t *testing.T) {
	if got := bash.Semantics().HistoryHeadAndTailTakeACount; got != interp.No {
		t.Errorf("HistoryHeadAndTailTakeACount is %v, want No", got)
	}
	sem, diag, d := bash.Semantics(), bash.Diagnostics(), bash.Dialect()
	r := &interp.Runner{
		Semantics: &sem, Diagnostics: &diag, Name: "bash",
		Dialect: &d,
	}
	bash.Apply(r)
	res, err := r.ExpandHistoryAlways("echo !!:1:h2", []string{"echo /my/path/for/testing"}, 1)
	if err != nil {
		t.Fatalf("expanding: %v", err)
	}
	if want := "echo /my/path/for2"; res.Line != want {
		t.Errorf("expanded to %q, want %q", res.Line, want)
	}
}

// bash has no `:P` modifier, and the letter is unrecognized.
//
// Inherited from the substrate, so this row holds the preset's No in place.
// Measured 2026-09-30 on 5.3.20 and 3.2.57 alike: `!!:1:P` is
// `P: unrecognized history modifier`.
func TestBashHasNoAbsolutePathModifier(t *testing.T) {
	if got := bash.Semantics().HistoryAbsolutePathModifier; got != interp.No {
		t.Errorf("HistoryAbsolutePathModifier is %v, want No", got)
	}
	sem, diag, d := bash.Semantics(), bash.Diagnostics(), bash.Dialect()
	r := &interp.Runner{Semantics: &sem, Diagnostics: &diag, Name: "bash", Dialect: &d}
	bash.Apply(r)
	_, err := r.ExpandHistoryAlways("echo !!:1:P", []string{"echo /a/b"}, 1)
	if err == nil {
		t.Fatal("the modifier was accepted")
	}
	if got, want := r.HistoryExpansionRefusal(err), "P: unrecognized history modifier"; got != want {
		t.Errorf("the refusal reads %q, want %q", got, want)
	}
}
