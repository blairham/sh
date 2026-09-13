// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// What this shell does about output that never ended its line.
//
// Measured 2026-09-12 through a pseudo-terminal with an rc file ending
// `printf 'LEFTOVER'`: a bold, inverse `%` where the output stopped, padded to
// the end of the row so the terminal wraps, and the prompt on the row below —
// where bash draws its prompt straight onto the output.
//
// Three answers, and they are asserted here because repl names no shell and a
// dialect that gave none would leave the whole thing off with nothing failing.
func TestThisShellMarksUnfinishedOutput(t *testing.T) {
	style := zsh.EditorStyle()
	if style.MarkUnfinishedOutputOption != "PROMPT_SP" {
		t.Errorf("marking option = %q, want PROMPT_SP", style.MarkUnfinishedOutputOption)
	}
	if style.ReturnBeforeThePromptOption != "PROMPT_CR" {
		t.Errorf("return option = %q, want PROMPT_CR", style.ReturnBeforeThePromptOption)
	}
	// The exact text, because it is what makes the four measured rows
	// byte-identical to the real shell rather than merely close.
	if got, want := style.ClearBeforeThePrompt, "\x1b[0m\x1b[27m\x1b[24m\x1b[J"; got != want {
		t.Errorf("before the prompt = %q, want %q", got, want)
	}
	// The mark is an inverse `%`, and it has to carry its own reset or the
	// prompt after it is drawn inverse too.
	if !strings.Contains(style.UnfinishedOutputMark, "%") {
		t.Errorf("mark = %q, want a %% in it", style.UnfinishedOutputMark)
	}
	if !strings.HasSuffix(style.UnfinishedOutputMark, "\x1b[0m") {
		t.Errorf("mark = %q, want it to end by resetting — the prompt follows it",
			style.UnfinishedOutputMark)
	}
	// Both options exist and both are on before anybody changes them, which is
	// what makes the marking this shell's default behavior rather than an
	// opt-in. A name the option table does not carry would read as off.
	for _, opt := range []string{"promptsp", "promptcr"} {
		out, st := runZsh(t, t.TempDir(), "[[ -o "+opt+" ]] && print -r -- on || print -r -- off\n")
		if st != 0 || strings.TrimSpace(out) != "on" {
			t.Errorf("%s = %q status %d, want on", opt, out, st)
		}
	}
}

// The join between the option table and the session that reads it.
//
// EditorStyle names two options; repl reads them through
// interp.Runner.DialectOption, which is this dialect's own namespace. A test
// that only asserted the two strings would pass for a name nothing could look
// up — and a name that does not resolve reads as *off*, which is a feature
// that is silently never on. The same shape as
// TestTheHistoryOptionsAreReadableThroughTheNamespace, and for the same
// reason.
//
// Every spelling this shell folds together, because the style spells them
// `PROMPT_SP` and `PROMPT_CR` and the option table records `promptsp` and
// `promptcr`.
func TestTheMarkingOptionsAreReadableThroughTheNamespace(t *testing.T) {
	style := zsh.EditorStyle()
	for _, tc := range []struct {
		named string
		on    string
		off   string
	}{
		{named: style.MarkUnfinishedOutputOption, on: "setopt promptsp", off: "unsetopt promptsp"},
		{named: style.ReturnBeforeThePromptOption, on: "setopt promptcr", off: "unsetopt promptcr"},
	} {
		if tc.named == "" {
			t.Fatal("EditorStyle names no option here; the marking would be off in every session")
		}
		for _, spelling := range []string{
			tc.named,
			strings.ToLower(tc.named),
			strings.ReplaceAll(strings.ToLower(tc.named), "_", ""),
		} {
			src := tc.on + `; [[ -o ` + spelling + ` ]] && echo on; ` +
				tc.off + `; [[ -o ` + spelling + ` ]] || echo off`
			out, st := runZsh(t, t.TempDir(), src)
			if want := "on\noff\n"; out != want || st != 0 {
				t.Errorf("%s: out %q status %d, want %q at 0", spelling, out, st, want)
			}
		}
	}
}
