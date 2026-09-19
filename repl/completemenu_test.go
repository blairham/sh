// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
	"testing"
)

// The listing, the menu and the delete-or-list, as keys.
//
// Nothing here names a shell, for bindings_test.go's reason: a dialect package
// imports this one. What each action *is* was measured against a real shell
// through a pseudo-terminal and the table is in widgets.go; this pins what the
// editor does with it, and dialect/zsh pins which names reach it.

// menuMatches is the fixture every case here completes against, in the order a
// completer hands them over. Three that share a prefix and one that does not,
// so a single-match case and a several-match case can be reached from the same
// table by typing a different word.
var menuMatches = []string{"uniq_alpha", "uniq_beta", "uniq_gamma", "zzsolo"}

// typedCompleting runs a line through an editor with a table of bindings and a
// completer that offers the matches above, and hands back the line that was
// accepted and everything that was drawn.
//
// Both, because the two actions here divide on exactly that: a menu is visible
// in the line and a listing is visible only on the screen, and a test that
// looked at one of them could not tell them apart.
func typedCompleting(t *testing.T, table map[string]Binding, keys string) (line, screen string) {
	t.Helper()
	var out strings.Builder
	s := Shell{
		KeyBindings: func(Keymap) map[string]Binding { return table },
		Completers: []Completer{CompleterFunc(func(c Completion) []Candidate {
			var got []string
			for _, m := range menuMatches {
				if strings.HasPrefix(m, c.Word) {
					got = append(got, m)
				}
			}
			return Words(got...)
		})},
	}
	e := s.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	got, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return got, out.String()
}

// The listing widget draws the matches and does not touch the line.
//
// Both halves are the assertion, and the second one is what tells this action
// apart from the completion on Tab. Tab's first press fills in `uniq_`, which
// the three matches agree on, and lists nothing; this lists on the first press
// and leaves `uniq` exactly as it was typed. A listing implemented as "the
// completion, but also print" would pass an assertion about the printing alone.
func TestTheListingWidgetDrawsTheMatchesAndLeavesTheLine(t *testing.T) {
	table := map[string]Binding{"\a": {Widget: WidgetListChoices}}
	line, screen := typedCompleting(t, table, ": uniq\a\n")
	if want := ": uniq"; line != want {
		t.Errorf("line = %q, want %q — the listing must not complete the word", line, want)
	}
	for _, m := range []string{"uniq_alpha", "uniq_beta", "uniq_gamma"} {
		if !strings.Contains(screen, m) {
			t.Errorf("%q was not drawn; screen:\n%q", m, screen)
		}
	}
	// The control: the same keys with the same completer on Tab complete and
	// draw nothing, which is what makes the two assertions above discriminate.
	line, screen = typedCompleting(t, map[string]Binding{}, ": uniq\t\n")
	if want := ": uniq_"; line != want {
		t.Errorf("the control line = %q, want %q", line, want)
	}
	if strings.Contains(screen, "uniq_alpha") {
		t.Errorf("one press of Tab drew the matches; screen:\n%q", screen)
	}
}

// And it lists again on the next press, where Tab's second press is the one
// that lists at all.
func TestTheListingWidgetHasNoSecondKeystrokeRule(t *testing.T) {
	table := map[string]Binding{"\a": {Widget: WidgetListChoices}}
	_, screen := typedCompleting(t, table, ": uniq\a\a\n")
	if got := strings.Count(screen, "uniq_gamma"); got != 2 {
		t.Errorf("the matches were drawn %d time(s) for two presses, want 2; screen:\n%q", got, screen)
	}
}

// A single match is listed rather than inserted, which is the other half of
// "without touching the line".
func TestTheListingWidgetDoesNotInsertALoneMatch(t *testing.T) {
	table := map[string]Binding{"\a": {Widget: WidgetListChoices}}
	line, screen := typedCompleting(t, table, ": zzs\a\n")
	if want := ": zzs"; line != want {
		t.Errorf("line = %q, want %q", line, want)
	}
	if !strings.Contains(screen, "zzsolo") {
		t.Errorf("the lone match was not drawn; screen:\n%q", screen)
	}
}

// The menu walks the matches in the line, one keystroke at a time, and wraps.
func TestMenuCompletionWalksTheMatches(t *testing.T) {
	table := map[string]Binding{
		"\a":   {Widget: WidgetMenuComplete},
		"\x00": {Widget: WidgetMenuCompleteBackward},
	}
	for _, c := range []struct{ name, keys, want string }{
		{"the first press takes the first match", ": uniq\a\n", ": uniq_alpha"},
		{"then the second", ": uniq\a\a\n", ": uniq_beta"},
		{"then the third", ": uniq\a\a\a\n", ": uniq_gamma"},
		{"and round to the first again", ": uniq\a\a\a\a\n", ": uniq_alpha"},
		// Backwards starts at the *last* match rather than at the first,
		// which is the row a menu built as "forwards, reversed" gets wrong.
		{"backwards starts at the last", ": uniq\x00\n", ": uniq_gamma"},
		{"and steps back from there", ": uniq\x00\x00\n", ": uniq_beta"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got, _ := typedCompleting(t, table, c.keys); got != c.want {
				t.Errorf("line = %q, want %q", got, c.want)
			}
		})
	}
}

// The menu draws no listing. Measured: the shell this was taken from listed on
// the first press and the listing was its `autolist` option rather than the
// menu — with the option off, the same keystroke inserted and drew nothing.
func TestMenuCompletionDrawsNoListing(t *testing.T) {
	table := map[string]Binding{"\a": {Widget: WidgetMenuComplete}}
	_, screen := typedCompleting(t, table, ": uniq\a\n")
	// `uniq_alpha` is in the line, so the discriminating name is one of the
	// two a listing would have drawn beside it.
	if strings.Contains(screen, "uniq_gamma") {
		t.Errorf("the menu drew a listing; screen:\n%q", screen)
	}
}

// One match is an ordinary completion, trailing space and all.
func TestAMenuOverOneMatchIsAnOrdinaryCompletion(t *testing.T) {
	table := map[string]Binding{"\a": {Widget: WidgetMenuComplete}}
	if got, _ := typedCompleting(t, table, ": zzs\a\n"); got != ": zzsolo " {
		t.Errorf("line = %q, want %q — a lone match gets the suffix Tab gives it", got, ": zzsolo ")
	}
}

// Anything else between two menu keystrokes ends the walk, and the next press
// starts a fresh completion.
//
// `^B` is the key in the middle: it moves the cursor back one, so the fresh
// completion that follows is over `uniq_alph` and offers exactly one match.
// A walk that survived the interruption would step to `uniq_beta` instead,
// which is what makes this discriminate rather than merely pass.
func TestAMenuBrokenByAnotherKeyStartsAgain(t *testing.T) {
	table := map[string]Binding{"\a": {Widget: WidgetMenuComplete}}
	if got, _ := typedCompleting(t, table, ": uniq\a\x02\a\n"); got != ": uniq_alpha a" {
		t.Errorf("line = %q, want %q", got, ": uniq_alpha a")
	}
}

// Deleting where there is a character under the cursor, and listing where
// there is not.
func TestDeleteCharOrListDividesOnTheCursor(t *testing.T) {
	table := map[string]Binding{"\a": {Widget: WidgetDeleteCharOrList}}
	// Two `^B`s put the cursor before `i`, so there is a character to delete
	// and the line loses it.
	line, screen := typedCompleting(t, table, ": uniq\x02\x02\a\n")
	if want := ": unq"; line != want {
		t.Errorf("line = %q, want %q — a character under the cursor is deleted", line, want)
	}
	if strings.Contains(screen, "uniq_alpha") {
		t.Errorf("it listed with a character under the cursor; screen:\n%q", screen)
	}
	// At the end of the line there is nothing to delete, and it lists.
	line, screen = typedCompleting(t, table, ": uniq\a\n")
	if want := ": uniq"; line != want {
		t.Errorf("line = %q, want %q — listing must not change the line", line, want)
	}
	if !strings.Contains(screen, "uniq_gamma") {
		t.Errorf("it did not list at the end of the line; screen:\n%q", screen)
	}
}

// The empty line lists rather than ending the session.
//
// This is the case a guess gets wrong: the key this action is bound to in one
// real shell ends input on an empty line, and the action does not. Measured
// through a pseudo-terminal with the action on a key that is not `^D` — it
// offered the whole command list. Here the completer answers the empty word
// with everything it has, and the line survives to be accepted.
func TestDeleteCharOrListOnAnEmptyLineLists(t *testing.T) {
	table := map[string]Binding{"\a": {Widget: WidgetDeleteCharOrList}}
	line, screen := typedCompleting(t, table, "\a: done\n")
	if want := ": done"; line != want {
		t.Errorf("line = %q, want %q — the session ended or the line was lost", line, want)
	}
	if !strings.Contains(screen, "zzsolo") {
		t.Errorf("nothing was listed on the empty line; screen:\n%q", screen)
	}
}

// A shell's own widget can run the editor's completion by name, which it could
// not before: Perform refused it, so a plugin falling back to the standard
// completion reached a refusal.
//
// Two calls, because the interesting half is that the two-keystroke rule
// survives the round trip — the first fills in what the matches agree on and
// the second lists them, exactly as two presses of Tab do.
func TestAWidgetCanRunTheEditorsCompletion(t *testing.T) {
	var out strings.Builder
	var performed bool
	s := Shell{
		KeyBindings: func(Keymap) map[string]Binding {
			return map[string]Binding{"\a": {Function: "w"}}
		},
		RunWidget: func(ctx context.Context, _ string, in Line) (Line, bool) {
			ed, inside := ActionsFrom(ctx)
			if !inside {
				return in, false
			}
			out, ok := ed.Perform(WidgetComplete, in)
			performed = ok
			return out, true
		},
		Completers: []Completer{CompleterFunc(func(c Completion) []Candidate {
			var got []string
			for _, m := range menuMatches {
				if strings.HasPrefix(m, c.Word) {
					got = append(got, m)
				}
			}
			return Words(got...)
		})},
	}
	e := s.newEditor(t.Context(), nil)
	e.in, e.out = typing(": uniq\a\a\n"), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatal(err)
	}
	if !performed {
		t.Fatal("the editor refused to complete from inside a widget")
	}
	if want := ": uniq_"; line != want {
		t.Errorf("line = %q, want %q — the first call fills in what the matches agree on", line, want)
	}
	if !strings.Contains(out.String(), "uniq_gamma") {
		t.Errorf("the second call did not list; screen:\n%q", out.String())
	}
}
