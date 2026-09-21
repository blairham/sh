// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// This shell's history file carries both of the things a file can carry.
//
// Two answers rather than one, because they were measured separately and only
// one of them is the option's doing: a multi-line command is stored across
// lines joined by a trailing backslash **whether or not** `EXTENDED_HISTORY`
// is set, and the `: <start>:<elapsed>;` header appears when it is.
//
// Asserted here rather than left to the decoder's own tests, which name no
// shell: a decoder that reads both perfectly is still useless if this dialect
// does not say its files are written that way. A mutation run is what asked —
// turning either answer off broke nothing in repl (#2452).
func TestThisShellsHistoryFileIsContinuedAndStamped(t *testing.T) {
	style := zsh.HistoryStyle()
	if !style.EntriesContinueOnABackslash {
		t.Error("entries do not continue on a backslash, so a multi-line command reads as several")
	}
	if !style.EntriesMayCarryATimestampHeader {
		t.Error("entries carry no timestamp header, so `EXTENDED_HISTORY` leaks into the line")
	}
}

// And a blank line in this shell's history file is an entry of its own.
//
// The third answer about the same file, and the one where this shell and bash
// part: measured 2026-09-21 on zsh 5.9.2 with `env -i` and a scratch HOME, a
// file with a blank line in it lists the blank — first, in the middle, and
// among EXTENDED_HISTORY headers — through `fc -R` and through the file
// `$HISTFILE` names alike. bash drops it and so does the substrate, so this
// dialect has to say so or it silently takes the other shell's answer (#4024).
//
// Asserted through the decoder rather than off the field, because the field
// being true is only interesting if it changes what a file reads as.
func TestABlankLineInThisShellsHistoryFileIsAnEntry(t *testing.T) {
	style := zsh.HistoryStyle()
	if !style.EmptyLinesAreEntries {
		t.Error("an empty line is not an entry, so a blank in the file silently disappears")
	}
	lines := []string{"echo one", "", "echo two"}
	got := repl.HistoryEntries(style, lines)
	if len(got) != 3 || got[0] != "echo one" || got[1] != "" || got[2] != "echo two" {
		t.Errorf("decoded %q, want the blank kept as an entry of its own", got)
	}
}
