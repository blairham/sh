// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// A session at a prompt and `$history` used to be two things that could not
// see each other, and the second of them did not exist.
//
// This is #4177 one list along. That issue joined the front end's remembered
// lines to the **bash** dialect's list; `dialect/zsh` was handed a reader and
// no adder at the same seam, so `interp.Runner.RecordHistoryEntry` did nothing
// here and a zsh session's typed lines reached neither `fc` nor the table
// `$history` views. With the parameter refusing by name on top of that, a
// prompt with zsh-autosuggestions loaded wrote
// `_zsh_autosuggest_strategy_history:22: history: parameter not implemented
// yet` over the line being typed, once per keystroke (#4408).
//
// Measured 2026-09-24 on zsh 5.9.2, `-f -i` with the lines on a pipe and a
// scratch HOME, one shape at a time. Every want below is what that shell
// printed for the same input.
//
// The **widget** reading — which is the one a suggestion is made in, and where
// the entry zsh leaves out is the line being typed rather than the command
// running — needs a terminal and is in cmd/sh/historywidgetpty_test.go.

// zshPrompt types lines into an interactive zsh and answers standard output.
func zshPrompt(t *testing.T, typed string) string {
	t.Helper()
	out, errs, code := prompt(t, typed, "zsh", "-f", "-i")
	if code != 0 {
		t.Fatalf("status %d, out %q, stderr %q", code, out, errs)
	}
	// stderr is where the prompt itself goes, so it is not asserted on.
	return out
}

// The lines a session types reach the table.
func TestAPromptsLinesReachTheHistoryTable(t *testing.T) {
	out := zshPrompt(t, "echo one\necho two\nprint -r -- \"${(v)history}\"\n")
	// Newest first, and without the line doing the asking: that one is the
	// event `$HISTCMD` names, which zsh's table leaves out.
	want := "one\ntwo\necho two echo one\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}

// And the search zsh-autosuggestions is built on finds the newest of them.
func TestASearchOfTheHistoryTableAtAPromptTakesTheNewestMatch(t *testing.T) {
	out := zshPrompt(t, "echo alpha\nls beta\necho gamma\nls delta\nprint -r -- \"${history[(r)echo*]}\"\n")
	want := "alpha\ngamma\necho gamma\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}

// The same lines reach `fc`, which is the half that says the adder is the
// thing that was missing rather than the parameter alone: this listing was
// empty for every zsh session this shell has ever had.
func TestAPromptsLinesReachTheFcListing(t *testing.T) {
	out := zshPrompt(t, "echo one\necho two\nfc -l\n")
	// `fc`'s default range ends at the command before itself, so the `fc -l`
	// line is not in its own listing.
	want := "one\ntwo\n    1  echo one\n    2  echo two\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}
