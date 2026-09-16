// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// TestTabKeepsCompletingWhenCompinitTakesTheWidget is #2770, and it is the
// whole of the blocker: a real `~/.zshrc` runs `compinit`, `compinit` puts a
// `zle -C` widget on Tab, and the key must go on completing.
//
// The two lines below are `compinit`'s own, in the order it writes them. They
// both run to completion here, which is what makes this a live failure rather
// than a missing feature: `^I` arrives in the table bound to a name that *is*
// defined, so without completionBinding the editor is handed
// `{Function: "complete-word"}` and calls `_main_complete`, which cannot
// complete because `compadd`, `compset` and `$compstate` are not in this
// shell.
//
// Measured 2026-09-14 through a pseudo-terminal against this machine's own
// `~/.zshrc` (Powerlevel10k, `zi` turbo), typing `print -r -- MARK uniquef`
// and pressing Tab in a directory whose only match is
// `uniquefile_marker.txt`:
//
//	ours, before | nothing completes, and the widget prints
//	             |   _main_complete:94: command not found: compset
//	             |   _setup:37: compstate: assignment to invalid subscript range
//	ours, after  | completes to `uniquefile_marker.txt`, no diagnostic
//	/bin/zsh     | completes to `uniquefile_marker.txt`
//
// **The assertion is the Widget and not merely "not the function".** A
// binding that reached the editor as nothing at all would also drop the
// diagnostics, and would leave Tab dead — which is the failure this issue
// says is worse than an absent completion system, arrived at from the other
// side.
func TestTabKeepsCompletingWhenCompinitTakesTheWidget(t *testing.T) {
	r := bindkeyRunner(t, "zle -C complete-word .complete-word _main_complete\n"+
		"bindkey '^i' complete-word\n")
	got, bound := zsh.KeyBindings(r, repl.KeymapMain)["\t"]
	want := repl.Binding{Widget: repl.WidgetComplete, Candidates: "complete-word"}
	if !bound || got != want {
		t.Errorf("^I after compinit = %v, %v, want %v — the editor's own completion", got, bound, want)
	}
}

// TestACompletionWidgetIsAnsweredByItsCompleterNotItsFunction is the rule
// under the case above, asked of the parts.
//
// The name a completion loader chooses is arbitrary — `_bash_complete-word`,
// `_correct_filename` and two hundred others are all `zle -C` widgets in a
// real session — so the question has to be asked of the *definition*. What
// the widget is worth here is its completer, which is the one of `zle -C`'s
// two claims this shell can keep.
func TestACompletionWidgetIsAnsweredByItsCompleterNotItsFunction(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want repl.Binding
	}{
		// A completer this editor has, under a name nothing would guess.
		{"arbitrary name", "zle -C _correct_filename .complete-word _cf\n" +
			"bindkey '^G' _correct_filename\n", repl.Binding{
			Widget: repl.WidgetComplete, Candidates: "_correct_filename",
		}},
		// The dotless spelling is the same action.
		{
			"no leading dot", "zle -C w complete-word f\nbindkey '^G' w\n",
			repl.Binding{Widget: repl.WidgetComplete, Candidates: "w"},
		},
		// zsh's own default completer, which `compinit` replaces.
		{
			"expand-or-complete", "zle -C w .expand-or-complete f\nbindkey '^G' w\n",
			repl.Binding{Widget: repl.WidgetComplete, Candidates: "w"},
		},
		// A completer this editor has not got: present and doing nothing,
		// which is what a key bound to `menu-select` does here already with
		// no `zle -C` in sight. Not the function, which would diagnose once
		// per keystroke.
		{
			"completer we lack", "zle -C w .menu-select f\nbindkey '^G' w\n",
			repl.Binding{Widget: repl.WidgetNone},
		},
		// **The control.** `zle -N` makes no claim about completion, so its
		// function is still what the key runs — the change must not swallow
		// every widget a startup file defines.
		{
			"plain widget is untouched", "zle -N w f\nbindkey '^G' w\n",
			repl.Binding{Function: "w"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := bindkeyRunner(t, c.src)
			got, bound := zsh.KeyBindings(r, repl.KeymapMain)["\a"]
			if !bound || got != c.want {
				t.Errorf("^G = %v, %v, want %v", got, bound, c.want)
			}
		})
	}
}
