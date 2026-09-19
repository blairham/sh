// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strconv"
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

// The bare listing and the translating form are one shell answering one
// question, so the set of positions they say exist has to be the same set
// (#3792).
//
// It was not. The listing walked the *names* and the translation walks the
// kernel's range, and those come apart wherever a platform takes more signal
// numbers than this table has names for: `kill -l 40` wrote `40` on a machine
// whose bare listing had no row for 40 at all, so the same binary said a
// position both did and did not exist. The gap is thirty-three positions wide
// on the kernel that has a real-time range and zero wide on the one that does
// not, which is why this is written as an invariant over whatever the host
// takes rather than as a row — a table of numbers here would be a fact about
// one machine, and the machine that has the gap is not the one this is
// usually run on.
//
// The two axes are pinned to their narrow answers on purpose: a shell that
// writes a number back for anything it cannot name would answer for every
// number ever passed to it, and the question here is which positions *exist*.
func TestTheListingAndTheTranslationAgreeAboutWhichPositionsExist(t *testing.T) {
	narrow := func(s *Semantics) {
		// Only a real position answers, so "answered" means "is a signal
		// here" rather than "was echoed back".
		s.KillListPrintsANumberItCannotName = No
		s.KillListLeavesAnUnnamedSignalBlank = No
		// 0 is the trap table's pseudo-signal rather than a position of the
		// kernel's, and it is the other test's question.
		s.KillListNamesZeroAsExit = No
	}
	// The numbered shape, because it writes the position beside the word and
	// so says what it thinks exists without the reader counting lines.
	dg := Diagnostics{
		KillListingUnnamedPosition: "%[1]d",
		KillListing:                KillListingNumberedPerLine,
	}
	out, _ := optRunAs(t, narrow, dg, `kill -l`, RouteUnspecified)
	listed := map[int]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		pos, _, ok := strings.Cut(line, ")")
		if !ok {
			t.Fatalf("listing line %q: want `N) word`", line)
		}
		n, err := strconv.Atoi(strings.TrimSpace(pos))
		if err != nil {
			t.Fatalf("listing line %q: %v", line, err)
		}
		listed[n] = true
	}
	if len(listed) == 0 {
		t.Fatal("the listing wrote nothing, so this test is measuring nothing")
	}
	// Below 128, so that nothing here is the exit-status reduction — that is
	// a translation of `$?` rather than a claim that a position exists.
	for n := 1; n < 128; n++ {
		_, st := optRunAs(t, narrow, dg, fmt.Sprintf("kill -l %d", n), RouteUnspecified)
		if answered := st == 0; answered != listed[n] {
			t.Errorf("position %d: the translation answers %v and the listing has it %v, want the two to agree",
				n, answered, listed[n])
		}
	}
}

// And the rendering stays the dialect's: a shell that leaves a position it
// cannot name out of its listing leaves out the ones past the last name too,
// rather than growing rows it had never written.
func TestAnEmptyRenderingLeavesThePositionsPastTheTableOutToo(t *testing.T) {
	out, _ := optRunAs(t, nil, Diagnostics{KillListing: KillListingNumberedPerLine}, `kill -l`, RouteUnspecified)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		_, word, ok := strings.Cut(line, ") ")
		if !ok {
			t.Fatalf("listing line %q: want `N) word`", line)
		}
		if _, err := strconv.Atoi(word); err == nil {
			t.Errorf("listing line %q: a bare number, want every position written as a name", line)
		}
	}
}
