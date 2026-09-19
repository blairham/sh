// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The bare listing walks the *platform's* table and asks the dialect what to
// write where its own table has no name (#3536).
//
// It walked the dialect's before, so a name the shell lacks took the position
// with it. Two shells have that gap and render it differently — ksh93 writes
// `SIG29` for the macOS signal it cannot name and dash on Linux writes a bare
// `16` for STKFLT — so the rendering is a field and the empty value is the
// third answer: leave the position out, which is what a shell whose table is
// the platform's never has to decide.
//
// ALRM rather than either of the measured pairs, because both of those are
// facts about one machine: signal 29 is INFO on macOS and a signal the shared
// table names on Linux, and 16 is the other way about. ALRM is on the table
// of every platform this builds for, so declaring it missing asks the
// rendering question and nothing else.
func TestAnUnnamedPositionIsWrittenTheDialectsWay(t *testing.T) {
	const src = `kill -l`
	listing := func(t *testing.T, unnamed string) string {
		t.Helper()
		out, _ := optRunAs(t, func(s *Semantics) {
			s.SignalNamesTheShellLacks = "ALRM"
		}, Diagnostics{KillListingUnnamedPosition: unnamed}, src, RouteUnspecified)
		return out
	}
	// `%[1]d` is the number alone, which is dash's rendering.
	if out := listing(t, "%[1]d"); !strings.Contains(out, "\n14\n") {
		t.Errorf("the number alone: wrote %q, want a bare 14 where ALRM would be", out)
	}
	// `SIG%[1]d` is ksh93's.
	if out := listing(t, "SIG%[1]d"); !strings.Contains(out, "\nSIG14\n") {
		t.Errorf("SIG and the number: wrote %q, want SIG14 where ALRM would be", out)
	}
	// And empty leaves the position out, which is what the listing did with
	// every such position before there was a field.
	out := listing(t, "")
	// Anchored, because VTALRM holds ALRM and would answer for it.
	if strings.Contains(out, "\n14\n") || strings.Contains(out, "\nALRM\n") {
		t.Errorf("empty: wrote %q, want the position left out entirely", out)
	}
	if !strings.Contains(out, "TERM") {
		t.Errorf("empty: wrote %q, want the rest of the table still there", out)
	}
}

// And the older name a shell answers to is written in the listing only where
// the dialect says so — Diagnostics.SignalListingWritesTheAlias, which is
// ksh93 alone, against Semantics.SignalNamesTheShellAlsoReads, which ksh93
// and zsh share.
func TestTheListingWritesTheAliasOnlyWhereTheDialectPrefersIt(t *testing.T) {
	read := func(t *testing.T, writes bool, src string) string {
		t.Helper()
		out, _ := optRunAs(t, func(s *Semantics) {
			s.SignalNamesTheShellAlsoReads = "IOT=ABRT"
			// The translating form takes a name at all, which is its own
			// axis and not this one's question.
			s.KillListAcceptsName = Yes
		}, Diagnostics{SignalListingWritesTheAlias: writes}, src, RouteUnspecified)
		return out
	}
	out := read(t, true, `kill -l`)
	if !strings.Contains(out, "\nIOT\n") || strings.Contains(out, "\nABRT\n") {
		t.Errorf("preferred: wrote %q, want IOT in the listing and no ABRT", out)
	}
	out = read(t, false, `kill -l`)
	if !strings.Contains(out, "\nABRT\n") || strings.Contains(out, "\nIOT\n") {
		t.Errorf("not preferred: wrote %q, want ABRT in the listing and no IOT", out)
	}
	// Either way the word is *read*, and the number still translates back to
	// the table's own name — which is what makes the listing a preference
	// rather than a second name for the signal.
	for _, writes := range []bool{true, false} {
		if out := read(t, writes, `kill -l IOT; kill -l 6`); out != "6\nABRT\n" {
			t.Errorf("writes=%v: wrote %q, want `kill -l IOT` 6 and `kill -l 6` ABRT", writes, out)
		}
	}
}
