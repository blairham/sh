// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// The bare `kill -l` listing in this applet is its own shape: the number in a
// two-wide column, `) `, then the **bare** name, one entry per line.
//
// Measured 2026-09-18 against BusyBox v1.37.0 in the pinned Alpine image
// (alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b),
// `cmd/ash` cross-compiled for linux/arm64 and run in the same container, a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`. The first three
// lines as bytes are ` 1) HUP\n 2) INT\n 3) QUIT\n`, and the tail is
// `31) SYS`, `35) RTMIN`, `64) RTMAX` — the unnamed positions between them
// skipped outright.
//
// This column held dash's zero-first shape, which is the one thing about the
// listing it does not share with dash (#3655). It is none of the four values
// that existed: bash's numbered shape writes `SIG` and packs five to a row,
// ksh93's and zsh's carry no number at all.
//
// The listing is the host's table, so the rows themselves cannot be written
// down here — a case holding one machine's numbering cannot pass on the
// other. What is asserted is the vector's pick; the bytes are checked against
// the applet in the container, above.
func TestTheListingIsNumberedOnePerLine(t *testing.T) {
	if got := ash.Diagnostics().KillListing; got != interp.KillListingNumberedPerLine {
		t.Errorf("KillListing is %v, want KillListingNumberedPerLine", got)
	}
	// And the unnamed positions are left out rather than written as numbers,
	// which is the empty value of the field beside it: `31) SYS` is followed
	// by `35) RTMIN` with nothing between.
	if got := ash.Diagnostics().KillListingUnnamedPosition; got != "" {
		t.Errorf("KillListingUnnamedPosition is %q, want it empty", got)
	}
}

// Two spellings this applet reads that the platform's table does not name,
// measured the same day in the same container: `kill -l POLL` and `kill -l
// IO` are both `29`, and `kill -l IOT` is `6` where `kill -l CLD` is refused.
// Both were refused here.
//
// They are not the same *kind* of word, which is what #3655 could not yet
// say and #3684 measured. The listing writes `29) POLL` and ` 6) ABRT` — and
// so does the translating form, `kill -l 29` being `POLL` and `kill -l 6`
// being `ABRT`. A word written in both places is this shell's **name** for
// the signal, so `POLL` is `SignalNamesTheShellSpellsItsOwnWay` and `IOT` is
// the reading-only `SignalNamesTheShellAlsoReads`.
//
// The measurement that separates them is a second column:
// Diagnostics.SignalListingWritesTheAlias is what ksh93u+ needs, where the
// listing writes `IOT` and `kill -l 6` writes `ABRT` on one binary. It stays
// off here, because this applet does not hold a preference for its listing —
// it has a different name for one signal.
func TestTheAppletReadsTwoSpellingsTheTableDoesNotName(t *testing.T) {
	if got := ash.Semantics().SignalNamesTheShellAlsoReads; got != "IOT=ABRT" {
		t.Errorf("SignalNamesTheShellAlsoReads is %q, want the reading-only pair", got)
	}
	if got := ash.Semantics().SignalNamesTheShellSpellsItsOwnWay; got != "IO=POLL" {
		t.Errorf("SignalNamesTheShellSpellsItsOwnWay is %q, want IO=POLL", got)
	}
	if ash.Diagnostics().SignalListingWritesTheAlias {
		t.Error("SignalListingWritesTheAlias is on, which would write `6) IOT`")
	}
}
