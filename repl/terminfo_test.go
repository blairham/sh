// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"slices"
	"strconv"
	"testing"

	"github.com/blairham/sh/interp"
)

// The capability table, and what a test can honestly assert about it.
//
// One of the three claims in terminfo.go is not checkable here. That a value
// is what every terminal description spells it — the second of the two tests
// a capability has to pass — is a fact about a database on the machine the
// measurement was taken on, so it is recorded in the comment with the date
// and the seven descriptions rather than asserted; a test that read the
// database would be testing the database.
//
// What is asserted is what a later edit gets wrong: the table's exact
// contents, that no capability is answered twice or under two spellings, that
// a name the comment says is refused has stayed refused, and that the color
// count is one number rather than two that agree today.

// The table is exactly the capabilities measured, name for name and byte for
// byte.
//
// Written out here rather than read from the package, which is the convention
// a roster in this repository follows and the only thing that makes this an
// assertion: a test that asked TerminalCapabilities what it held would agree
// with it whatever it said. What it catches is the edit this table invites —
// a capability added because something asked for it, without the two tests in
// terminfo.go being applied to it, or a value quietly corrected to what a
// manual says rather than to what this shell does.
//
// The behavioral half of the proof is elsewhere and is stronger than anything
// this file could say: the corpus compares each of these against real zsh's
// own `$terminfo` and `$termcap` on the machine the record was made on.
func TestTheTableIsTheCapabilitiesMeasured(t *testing.T) {
	want := []TerminalCapability{
		{Terminfo: "colors", Termcap: "Co", Value: strconv.Itoa(interp.TerminalColors)},
		{Terminfo: "cub", Termcap: "LE", Value: "\x1b[%p1%dD"},
		{Terminfo: "cud", Termcap: "DO", Value: "\x1b[%p1%dB"},
		{Terminfo: "cuf", Termcap: "RI", Value: "\x1b[%p1%dC"},
		{Terminfo: "cuu", Termcap: "UP", Value: "\x1b[%p1%dA"},
		{Terminfo: "cr", Termcap: "cr", Value: "\r"},
		{Terminfo: "ed", Termcap: "cd", Value: "\x1b[J"},
		{Terminfo: "el", Termcap: "ce", Value: "\x1b[K"},
		{Terminfo: "home", Termcap: "ho", Value: "\x1b[H"},
		{Terminfo: "kcub1", Termcap: "kl", Value: "\x1bOD"},
		{Terminfo: "kcud1", Termcap: "kd", Value: "\x1bOB"},
		{Terminfo: "kcuf1", Termcap: "kr", Value: "\x1bOC"},
		{Terminfo: "kcuu1", Termcap: "ku", Value: "\x1bOA"},
	}
	if got := TerminalCapabilities(); !slices.Equal(got, want) {
		t.Errorf("TerminalCapabilities() = %q, want %q", got, want)
	}
}

// A capability appears once, under one terminfo name and one termcap name.
//
// The failure this catches is a merge: two entries for `ed`, or `ku` reused
// for a second capability, either of which makes one of the two views quietly
// lose a key while the other keeps both. It is not implied by the roster
// above — that one compares against a list somebody could paste a duplicate
// into as well.
func TestNoCapabilityIsAnsweredTwice(t *testing.T) {
	ti := map[string]bool{}
	tc := map[string]bool{}
	for _, c := range TerminalCapabilities() {
		if c.Terminfo == "" || c.Termcap == "" {
			t.Errorf("capability %+v is missing one of its two names, so one of the two views cannot answer for it", c)
		}
		if ti[c.Terminfo] {
			t.Errorf("terminfo name %q is answered twice", c.Terminfo)
		}
		if tc[c.Termcap] {
			t.Errorf("termcap name %q is answered twice", c.Termcap)
		}
		ti[c.Terminfo], tc[c.Termcap] = true, true
	}
}

// The names in terminfoRefused are refused, which is what makes that list a
// statement rather than a stale comment.
//
// It is the failure mode a table like this has: a capability gets added
// because something asked for it, and the paragraph explaining why it could
// not be answered stays behind, three lines above the value that contradicts
// it.
func TestEveryNameSaidToBeRefusedIsStillRefused(t *testing.T) {
	for _, c := range TerminalCapabilities() {
		if slices.Contains(terminfoRefused, c.Terminfo) {
			t.Errorf("%q is answered and terminfoRefused still says why it is not; one of the two is now wrong", c.Terminfo)
		}
		if slices.Contains(terminfoRefused, c.Termcap) {
			t.Errorf("termcap %q is answered and terminfoRefused still names it", c.Termcap)
		}
	}
}

// The color count is the renderer's, read from interp rather than written
// twice.
//
// One number in two places is the failure this prevents, and it is a quiet
// one: a capability reporting 256 on a shell whose `%F{200}` draws the
// default sends a theme down the 256-color branch and leaves it colorless.
// That the count matches what the renderer actually *paints* is asserted
// against the running shell in dialect/zsh/terminfo_test.go, where there is a
// `%F{}` to draw.
func TestTheColorCountComesFromTheRenderer(t *testing.T) {
	colors, found := "", false
	for _, c := range TerminalCapabilities() {
		if c.Terminfo == "colors" {
			colors, found = c.Value, true
		}
	}
	if !found {
		t.Fatal("no colors capability, and it is the one #1388 is about")
	}
	n, err := strconv.Atoi(colors)
	if err != nil {
		t.Fatalf("terminfo[colors] = %q, which is not a count: %v", colors, err)
	}
	if n != interp.TerminalColors {
		t.Errorf("terminfo[colors] = %d and the renderer paints %d indices; the two have to be one number", n, interp.TerminalColors)
	}
}
