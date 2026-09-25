// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// The ground a session with an editor clears before every prompt goes on it,
// and a session without one never writes.
//
// The other thing the editor draws around a line — the request that the
// terminal mark a paste — is withheld on a pipe whatever the option says,
// because a paste is something a terminal does. It is asserted where it can be
// seen, in cmd/zsh's pseudo-terminal test.
const editorGround = "\x1b[0m\x1b[27m\x1b[24m\x1b[J"

// Whether this session has a line editor at all is the *shell's* answer, asked
// for by name and asked again for every line.
//
// One shell in the panel has an option for it — zsh's `zle`, which `+Z` turns
// off — and with it off an interactive shell still reads lines and still runs
// them. What goes away is the editor, and with it everything the editor draws:
// zsh's own `zpty`-driven test files start the shell as `-fiV +Z` and the
// reference writes nothing to the terminal but what the commands print, where
// this shell wrapped every line of those sessions in a redraw and a
// bracketed-paste toggle (#4472).
//
// Each row asserts both halves, and the first is the one that keeps the second
// honest: the line **ran** either way. A session that drew no escapes because
// it read no lines would pass an assertion about escapes alone.
func TestTheLineEditorRunsUnderTheNamedOption(t *testing.T) {
	for _, tc := range []struct {
		name, option string
		on           bool
		// drawing is whether the editor is expected to have drawn.
		drawing bool
	}{
		{
			// The majority, and what a caller without a dialect gets: four of
			// the five have no way to turn the editor off.
			name:    "no option is named, so the editor runs",
			drawing: true,
		},
		{
			name:    "the named option is off, so there is no editor",
			option:  "zle",
			drawing: false,
		},
		{
			name:    "the named option is on, so there is one",
			option:  "zle",
			on:      true,
			drawing: true,
		},
		{
			// The rule commentsAreOff states, on the other seam that reads an
			// option by name: a name this shell has never heard of is not a
			// name that is off. Reading it as off would take the line editor
			// away from every dialect that never installed the option.
			name:    "a name the shell does not have is not a name that is off",
			option:  "nobodyhasthis",
			drawing: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ran, drawn := editorOptionSession(t, tc.option, tc.on, "echo a\n")
			if want := "a\n"; ran != want {
				t.Fatalf("output %q, want %q — the line has to run either way", ran, want)
			}
			if got := strings.Contains(drawn, editorGround); got != tc.drawing {
				t.Errorf("the ground under the prompt drawn = %v, want %v (stderr %q)",
					got, tc.drawing, drawn)
			}
		})
	}
}

// And it is asked again per line, so a line that moves the option is obeyed
// from the next line on.
//
// Measured on the real shell, 2026-09-25, zsh 5.9.2 on a pseudo-terminal with
// `-fiV`: `unsetopt zle` is drawn by the editor and every line after it is
// plain, and in a `+Z` shell `setopt zle` is plain and the next line is drawn.
// So the state moves underneath the loop, and a session that settled this at
// startup would be a person typing `unsetopt zle`, watching it do nothing, and
// concluding the option is broken.
func TestTheEditorOptionIsReReadForEveryLine(t *testing.T) {
	// Three lines: one with the editor, one that turns it off, one without.
	// The stub's own switch is a variable the session sets, so this is the
	// option moving underneath the loop rather than three sessions.
	ran, drawn := editorOptionSession(t, "zle", true, "echo a\nZLE=\necho b\n")
	if want := "a\nb\n"; ran != want {
		t.Fatalf("output %q, want %q", ran, want)
	}
	// Two prompts were drawn by the editor — the first line's and the one the
	// line that turned the option off was read at — and the third was not.
	if got, want := strings.Count(drawn, editorGround), 2; got != want {
		t.Errorf("the editor drew %d of the 3 prompts, want %d (stderr %q)", got, want, drawn)
	}
}

// editorOptionSession runs typed through a session whose shell names option
// and holds it at on, and returns what the commands wrote and what was drawn.
//
// On a pipe with the editor given to it, which is the one shape that can reach
// the editor loop without a terminal — see
// interp.Semantics.EditorReadsKeysWhereThereIsNoTerminal. The question here is
// the option and not the terminal, and a test that needed a pseudo-terminal to
// ask it would be skipped on a platform that has none.
func editorOptionSession(t *testing.T, option string, on bool, typed string) (ran, drawn string) {
	t.Helper()
	var out, said bytes.Buffer
	vars := map[string]string{"PS1": "P> ", "PS2": "> ", "PATH": "/bin:/usr/bin"}
	if on {
		vars["ZLE"] = "on"
	}
	r := newTestRunner(vars)
	r.Stdout = &out
	// One name, answered from a variable so a line the session runs can move
	// it. Every other name is unknown, which is what the last row above is
	// about.
	r.SetOptionNamespace(func(r *interp.Runner, name string) (bool, bool) {
		if name != "zle" {
			return false, false
		}
		v, _ := r.GetVar("ZLE")
		return v != "", true
	})
	s := Shell{
		Runner: r,
		In:     strings.NewReader(typed),
		Out:    &out,
		Err:    &said,
		// The editor on a pipe, which is what puts this session in the loop
		// the option gates rather than in runPlain.
		EditorWithoutATerminal: true,
		Editor: EditorStyle{
			// What the editor draws that a session without one must not: with
			// the ground empty every row above would pass for a shell that had
			// never learned to withhold it. The paste offer is set beside it so
			// that a session which drew one here would be a finding rather than
			// a shell that was never asked.
			BracketedPaste:       true,
			ClearBeforeThePrompt: editorGround,
			RunsUnderTheOption:   option,
		},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatalf("run %q: %v", typed, err)
	}
	return out.String(), said.String()
}
