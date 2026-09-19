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

// And a word this shell spells its own way is a *name* rather than a
// preference: it is written wherever a number comes back as a name, and it
// reads back to that number (#3684).
//
// The discriminator against the field above is the pair of routes. One column
// writes its older spelling in the bare listing and the table's own name for
// `kill -l 6` on the same binary — a preference the listing holds — while
// another writes one word in both places, which is that shell's name for the
// signal. A field that answered either question would get the other wrong,
// so the rows here assert both routes each time.
//
// ALRM rather than the pair this was measured on, for the reason the test
// above uses ALRM: signal 29 is one thing on one kernel and another on the
// next, and 14 is ALRM everywhere this builds for. So the rows ask the
// question and nothing about a machine.
func TestASignalTheShellSpellsItsOwnWayIsWrittenAndReadBack(t *testing.T) {
	run := func(t *testing.T, spells, src string) string {
		t.Helper()
		out, _ := optRunAs(t, func(s *Semantics) {
			s.SignalNamesTheShellSpellsItsOwnWay = spells
			// The translating form takes a name at all, which is its own
			// axis and not this one's question.
			s.KillListAcceptsName = Yes
		}, Diagnostics{}, src, RouteUnspecified)
		return out
	}
	const spelled = "ALRM=WAKEUP"
	// Both writing routes take the shell's word.
	if out := run(t, spelled, `kill -l 14`); out != "WAKEUP\n" {
		t.Errorf("the number translated back: wrote %q, want WAKEUP", out)
	}
	if out := run(t, spelled, `kill -l`); !strings.Contains(out, "\nWAKEUP\n") ||
		strings.Contains(out, "\nALRM\n") {
		t.Errorf("the listing: wrote %q, want WAKEUP in it and no ALRM", out)
	}
	// And both spellings read back to the same number, which is what makes
	// it a name and not a rendering: the shell's word, and the one the table
	// carries it under.
	if out := run(t, spelled, `kill -l WAKEUP; kill -l ALRM`); out != "14\n14\n" {
		t.Errorf("read back: wrote %q, want 14 for each spelling", out)
	}
	// A trap takes it too, and writes the table's name into the trap table,
	// so one word reaches everything downstream.
	if out := run(t, spelled, `trap 'echo hit' WAKEUP; kill -s WAKEUP $$`); !strings.Contains(out, "hit") {
		t.Errorf("a trap on the shell's word: wrote %q, want the handler to fire", out)
	}
	// With nothing said the table's own name stands in both places and the
	// shell's word names nothing — the mutation that says every row above is
	// the field's doing.
	if out := run(t, "", `kill -l 14`); out != "ALRM\n" {
		t.Errorf("unspelled: wrote %q, want ALRM", out)
	}
	if out := run(t, "", `kill -l`); !strings.Contains(out, "\nALRM\n") ||
		strings.Contains(out, "WAKEUP") {
		t.Errorf("unspelled listing: wrote %q, want ALRM in it and no WAKEUP", out)
	}
	if out := run(t, "", `kill -l WAKEUP`); !strings.Contains(out, "WAKEUP") ||
		strings.Contains(out, "14") {
		t.Errorf("unspelled read back: wrote %q, want the word refused", out)
	}
}

// The two fields answer different questions, and this is the probe that can
// tell them apart: the listing preference moves the *listing* alone and the
// spelling moves both routes.
//
// Without it a reader could fold one into the other and every row of either
// test above would still pass — which is what the measurement says is wrong,
// since one column writes `IOT` in its listing and `ABRT` for `kill -l 6` on
// the same binary.
func TestTheListingPreferenceAndTheShellsOwnSpellingAreNotOneField(t *testing.T) {
	run := func(t *testing.T, tweak func(*Semantics), dg Diagnostics, src string) string {
		t.Helper()
		out, _ := optRunAs(t, func(s *Semantics) {
			s.KillListAcceptsName = Yes
			tweak(s)
		}, dg, src, RouteUnspecified)
		return out
	}
	// The preference: the listing writes the older word, the translation
	// does not.
	preference := func(s *Semantics) { s.SignalNamesTheShellAlsoReads = "IOT=ABRT" }
	if out := run(t, preference, Diagnostics{SignalListingWritesTheAlias: true}, `kill -l`); !strings.Contains(out, "\nIOT\n") {
		t.Errorf("preference listing: wrote %q, want IOT", out)
	}
	if out := run(t, preference, Diagnostics{SignalListingWritesTheAlias: true}, `kill -l 6`); out != "ABRT\n" {
		t.Errorf("preference translation: wrote %q, want the table's own ABRT", out)
	}
	// The spelling: both.
	spelling := func(s *Semantics) { s.SignalNamesTheShellSpellsItsOwnWay = "ABRT=IOT" }
	if out := run(t, spelling, Diagnostics{}, `kill -l`); !strings.Contains(out, "\nIOT\n") {
		t.Errorf("spelling listing: wrote %q, want IOT", out)
	}
	if out := run(t, spelling, Diagnostics{}, `kill -l 6`); out != "IOT\n" {
		t.Errorf("spelling translation: wrote %q, want IOT", out)
	}
}
