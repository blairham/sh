// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
	"testing"
)

// The listing widget and the menu, on a real terminal, under a two-row prompt.
//
// completemenu_test.go grades what each action does to the line and to the
// bytes the editor wrote; this grades the *screen*, which is the half a
// reader-driven test cannot see. Where the listing lands, what is left on the
// row above it and what the line looks like underneath are the terminal's
// answer rather than the function's.
//
// **Two rows of prompt, deliberately**, for completelistpty_test.go's reason:
// a one-row `PS1` cannot tell a listing drawn in the right place from one that
// ate the row above it, and this package has a standing finding that
// components can each be right while the nesting is broken (#2467, #3222).
//
// Both halves of the binding are exercised — the key names a completion the
// shell supplies, so this is also the check that Binding.Candidates reaches an
// action that is not WidgetComplete.
func TestTheListingWidgetReachesTheTerminal(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Runner.Vars["PS1"] = "UPPER\n[\\#]"
		sh.KeyBindings = func(Keymap) map[string]Binding {
			return map[string]Binding{"\a": {Widget: WidgetListChoices, Candidates: "w"}}
		}
		sh.RunCompletion = func(_ context.Context, _ string, c Completion) []Candidate {
			var got []string
			for _, m := range menuMatches {
				if strings.HasPrefix(m, c.Word) {
					got = append(got, m)
				}
			}
			return Words(got...)
		}
	})

	s.typeLine(": uniq\a")
	waitFor(t, s.screen, "uniq_gamma", "the listing")

	if got := s.row(0); got != "UPPER" {
		t.Errorf("row 0 is %q, want the upper prompt row untouched", got)
	}
	if got := strings.Fields(s.row(2)); len(got) != 3 ||
		got[0] != "uniq_alpha" || got[1] != "uniq_beta" || got[2] != "uniq_gamma" {
		t.Errorf("the listing row is %v, want the three matches\nscreen:\n%s", got, s.shown().styledText())
	}
	// And the line underneath is what was typed, not what a completion would
	// have left. This is the assertion the whole action turns on: `uniq_` is
	// the prefix the three matches agree on, so a listing built out of the
	// completion draws the same rows and leaves a different line.
	if got := s.row(3); got != "[1]: uniq" {
		t.Errorf("row 3 is %q, want %q — the listing must not complete the word\nscreen:\n%s",
			got, "[1]: uniq", s.shown().styledText())
	}

	s.typeKeys("\n")
	s.end()
}

// The menu puts each match in the line in turn, on the terminal, and draws no
// listing while it does.
func TestMenuCompletionReachesTheTerminal(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Runner.Vars["PS1"] = "UPPER\n[\\#]"
		sh.KeyBindings = func(Keymap) map[string]Binding {
			return map[string]Binding{"\a": {Widget: WidgetMenuComplete}}
		}
		sh.Completers = []Completer{CompleterFunc(func(c Completion) []Candidate {
			var got []string
			for _, m := range menuMatches {
				if strings.HasPrefix(m, c.Word) {
					got = append(got, m)
				}
			}
			return Words(got...)
		})}
	})

	s.typeLine(": uniq\a")
	waitFor(t, s.screen, "uniq_alpha", "the first match")
	if got := s.row(1); got != "[1]: uniq_alpha" {
		t.Errorf("row 1 is %q, want %q\nscreen:\n%s", got, "[1]: uniq_alpha", s.shown().styledText())
	}
	// The wait is on `beta` and not on `uniq_beta`: the redraw is
	// incremental, so what reaches the terminal is a jump back and the four
	// characters that differ. A wait on the whole word is a wait on a
	// whole-line draw, which is a path no session with a terminal takes.
	s.typeKeys("\a")
	waitFor(t, s.screen, "beta", "the second match")
	if got := s.row(1); got != "[1]: uniq_beta" {
		t.Errorf("row 1 is %q, want %q\nscreen:\n%s", got, "[1]: uniq_beta", s.shown().styledText())
	}
	// Nothing was drawn below the line, which is what "the menu draws no
	// listing" means on a screen: there is no row under the prompt with
	// anything on it.
	for i, row := range strings.Split(s.shown().styledText(), "\n")[2:] {
		if strings.TrimSpace(row) != "" {
			t.Errorf("row %d is %q, want nothing drawn under the line\nscreen:\n%s",
				2+i, row, s.shown().styledText())
		}
	}
	if got := s.row(0); got != "UPPER" {
		t.Errorf("row 0 is %q, want the upper prompt row untouched", got)
	}

	s.typeKeys("\n")
	s.end()
}
