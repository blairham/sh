// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
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
