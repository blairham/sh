// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// The six builtin completion widgets that reached nothing until the editor
// grew a listing and a menu (#3043).
//
// `zle -C name completer function` names one of eight, and two of them —
// `complete-word` and `expand-or-complete` — were the only two this shell
// answered. The other six left the key bound to WidgetNone: nothing happened,
// in the open, which is honest and is still a dead key.
//
// What each action *is* was measured against zsh 5.9.2 through a
// pseudo-terminal and the measurements are in repl/widgets.go, where the
// actions are. This is the half that belongs to this package: which of this
// shell's names reach them.
func TestTheCompletionWidgetNamesReachTheEditor(t *testing.T) {
	for _, c := range []struct {
		name string
		want repl.Widget
	}{
		// The two that were already here, unchanged.
		{"complete-word", repl.WidgetComplete},
		{"expand-or-complete", repl.WidgetComplete},
		// Completion that ignores what is after the cursor, which is what
		// this editor's completion does and always did: it completes the text
		// before the cursor and replaces exactly that. Measured with the
		// cursor put after `uniq` in `cat uniqXYZ` — this name inserted the
		// `_` the matches agree on and left `XYZ` alone, where
		// `complete-word` found nothing at all.
		{"expand-or-complete-prefix", repl.WidgetComplete},
		{"list-choices", repl.WidgetListChoices},
		{"delete-char-or-list", repl.WidgetDeleteCharOrList},
		{"menu-complete", repl.WidgetMenuComplete},
		{"menu-expand-or-complete", repl.WidgetMenuComplete},
		{"reverse-menu-complete", repl.WidgetMenuCompleteBackward},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := bindkeyRunner(t, "bindkey '^G' "+c.name+"\n")
			got, bound := zsh.KeyBindings(r, repl.KeymapMain)["\a"]
			if !bound || got.Widget != c.want {
				t.Errorf("^G bound to %s = %v, %v, want widget %d",
					c.name, got, bound, c.want)
			}
		})
	}
}

// And `zle -C` over one of them carries the function that supplies the
// candidates, which it did not: the question asked was `== WidgetComplete`, so
// a widget whose completer was `menu-complete` reached the editor with its
// function dropped.
//
// The control is the last row: a completer outside the eight still resolves to
// nothing, and naming a source of candidates for a completion that will not
// happen would be a table saying something untrue.
func TestACompletionWidgetOverTheNewCompletersKeepsItsFunction(t *testing.T) {
	for _, c := range []struct {
		completer string
		want      repl.Binding
	}{
		{"list-choices", repl.Binding{Widget: repl.WidgetListChoices, Candidates: "w"}},
		{"menu-complete", repl.Binding{Widget: repl.WidgetMenuComplete, Candidates: "w"}},
		{"reverse-menu-complete", repl.Binding{Widget: repl.WidgetMenuCompleteBackward, Candidates: "w"}},
		{"delete-char-or-list", repl.Binding{Widget: repl.WidgetDeleteCharOrList, Candidates: "w"}},
		{"expand-or-complete-prefix", repl.Binding{Widget: repl.WidgetComplete, Candidates: "w"}},
		// Not one of the eight, and not this editor's: the key does nothing
		// and says so by holding no candidates.
		{".menu-select", repl.Binding{Widget: repl.WidgetNone}},
	} {
		t.Run(c.completer, func(t *testing.T) {
			r := bindkeyRunner(t, "zle -C w "+c.completer+" f\nbindkey '^G' w\n")
			got, bound := zsh.KeyBindings(r, repl.KeymapMain)["\a"]
			if !bound || got != c.want {
				t.Errorf("^G = %v, %v, want %v", got, bound, c.want)
			}
		})
	}
}

// The listing prints the name back, which is what a person who ran `bindkey`
// then asks for.
func TestTheNewCompletionWidgetsAreNamedBackByAListing(t *testing.T) {
	for _, name := range []string{
		"list-choices", "delete-char-or-list", "menu-complete",
		"reverse-menu-complete", "expand-or-complete-prefix",
	} {
		out, _ := runZsh(t, t.TempDir(), "bindkey '^G' "+name+"\nbindkey '^G'\n")
		if want := `"^G" ` + name + "\n"; out != want {
			t.Errorf("bindkey '^G' after binding %s printed %q, want %q", name, out, want)
		}
	}
}
