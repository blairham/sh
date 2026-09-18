// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
	"testing"
)

// The seam a shell's own completion system answers through: Binding.Candidates
// and Shell.RunCompletion.
//
// Nothing here names a shell, for shellwidget_test.go's reason — what the
// names are and what a completion function reads the word out of belongs in
// dialect/zsh, next to the measurement. This pins what the *seam* does with
// whatever comes back.

// completedThroughShell types keys into an editor whose Tab is bound to a
// completion the shell supplies, and hands back the line and everything drawn.
func completedThroughShell(t *testing.T, answer func(Completion) []Candidate, keys string) (string, string) {
	t.Helper()
	var out strings.Builder
	s := Shell{
		KeyBindings: func(Keymap) map[string]Binding {
			return map[string]Binding{"\t": {Widget: WidgetComplete, Candidates: "w"}}
		},
		// A completer of this package's own behind it, so that "the shell
		// answered" and "the editor answered" are distinguishable — a test
		// where the editor had nothing to say could not tell an answer that
		// won from an answer that was merely the only one.
		Completers: []Completer{CompleterFunc(func(Completion) []Candidate {
			return Words("editors-own")
		})},
	}
	if answer != nil {
		s.RunCompletion = func(_ context.Context, name string, c Completion) []Candidate {
			if name != "w" {
				t.Errorf("completion name = %q, want %q", name, "w")
			}
			return answer(c)
		}
	}
	e := s.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line, out.String()
}

// TestAShellsOwnCompletionAnswersTheKeyItWasBoundTo is the capability: a key
// the shell configured completes what the shell says it completes.
func TestAShellsOwnCompletionAnswersTheKeyItWasBoundTo(t *testing.T) {
	var saw Completion
	line, _ := completedThroughShell(t, func(c Completion) []Candidate {
		saw = c
		return Words("checkout")
	}, "git che\t\n")
	if want := "git checkout "; line != want {
		t.Errorf("line = %q, want %q", line, want)
	}
	// And it was asked about the word under the cursor rather than the line,
	// which is what a completer needs and what the seam promises.
	if saw.Word != "che" || saw.Start != 4 || saw.Point != 7 {
		t.Errorf("asked about %+v, want the word `che` at 4", saw)
	}
}

// TestAShellThatAnswersNothingLeavesThisEditorsCompletionStanding is the
// ordering rule, and it is the one that makes the seam safe to wire to a
// startup file's completion system: an answer of no matches is "no opinion"
// and not "this word cannot be completed".
//
// The three shapes are the three ways a real one comes back empty — it has
// nothing to say about this word, there is no such action, and the front end
// never wired a shell in at all.
func TestAShellThatAnswersNothingLeavesThisEditorsCompletionStanding(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer func(Completion) []Candidate
	}{
		{"nothing to say", func(Completion) []Candidate { return nil }},
		{"an empty list", func(Completion) []Candidate { return []Candidate{} }},
		{"no shell wired in", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			line, _ := completedThroughShell(t, c.answer, "x\t\n")
			if want := "editors-own "; line != want {
				t.Errorf("line = %q, want %q — this editor's own answer", line, want)
			}
		})
	}
}

// TestAKeyBoundToCompletionStillListsOnTheSecondPress is the gap this seam
// uncovered, and it is older than the seam: completion is the one action this
// editor performs over *two* keystrokes, and a key that reached it through a
// binding ran the completion and threw the matches away.
//
// It matters because the bound key is the ordinary case rather than the
// exotic one. Every startup file with a completion system rebinds Tab, so
// before this the second Tab listed nothing on any real shell session.
func TestAKeyBoundToCompletionStillListsOnTheSecondPress(t *testing.T) {
	matches := []string{"checkout", "cherry", "cherry-pick"}
	line, drawn := completedThroughShell(t,
		func(Completion) []Candidate { return Words(matches...) }, "git che\t\t\n")
	// The first press filled in as far as the three agree, which is no
	// further than what was typed, so the line is unchanged.
	if want := "git che"; line != want {
		t.Errorf("line = %q, want %q", line, want)
	}
	for _, name := range matches {
		if !strings.Contains(drawn, name) {
			t.Errorf("the second press did not print %q; it drew %q", name, drawn)
		}
	}
}

// And one press does not list, which is the other half of the same rule — a
// test that only asserted the listing would pass against an editor that
// listed on every press and never filled anything in.
func TestOnePressOfABoundCompletionKeyDoesNotList(t *testing.T) {
	_, drawn := completedThroughShell(t,
		func(Completion) []Candidate { return Words("checkout", "cherry") }, "git che\t\n")
	if strings.Contains(drawn, "cherry") {
		t.Errorf("one press listed; it drew %q", drawn)
	}
}
