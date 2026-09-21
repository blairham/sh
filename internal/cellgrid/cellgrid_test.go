// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cellgrid

import (
	"strings"
	"testing"
)

// The grid, and the two properties the fidelity comparison rests on: two
// spellings of one appearance are the same screen, and a colorless render is
// not.

func gridOf(t *testing.T, cols int, writes ...string) *Grid {
	t.Helper()
	g := New(cols)
	for _, w := range writes {
		if _, err := g.Write([]byte(w)); err != nil {
			t.Fatalf("writing %q: %v", w, err)
		}
	}
	return g
}

// The claim the whole comparison rests on: two spellings of one color paint
// the same screen.
//
// A byte diff fails on every one of these pairs, which is why this package
// exists rather than a `bytes.Equal`.
func TestTwoSpellingsOfOneAppearanceAreTheSameScreen(t *testing.T) {
	for _, tc := range []struct{ name, a, b string }{
		{
			"an index against its terse spelling",
			"\x1b[38;5;1mX", "\x1b[31mX",
		},
		{
			"a bright index against its terse spelling",
			"\x1b[38;5;9mX", "\x1b[91mX",
		},
		{
			"two attributes in either order",
			"\x1b[1;4mX", "\x1b[4;1mX",
		},
		{
			"a reset spelled out against a bare reset",
			"\x1b[1mA\x1b[0mB", "\x1b[1mA\x1b[mB",
		},
		{
			"a color set twice, the second winning",
			"\x1b[31m\x1b[32mX", "\x1b[32mX",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if diffs := Compare(gridOf(t, 20, tc.a), gridOf(t, 20, tc.b)); len(diffs) != 0 {
				t.Errorf("%q and %q compare as different screens: %v", tc.a, tc.b, diffs)
			}
		})
	}
}

// And the mirror, which is the property a harness that stripped ANSI would
// lose: a colorless render differs from a colored one in every colored cell.
func TestAColorlessRenderIsNotTheSameScreen(t *testing.T) {
	colored := gridOf(t, 20, "\x1b[38;5;31mbranch\x1b[0m")
	plain := gridOf(t, 20, "branch")
	diffs := Compare(colored, plain)
	if len(diffs) != len("branch") {
		t.Fatalf("a colorless render differed in %d cells, want %d: %v",
			len(diffs), len("branch"), diffs)
	}
	if !strings.Contains(diffs[0].String(), "fg=31") {
		t.Errorf("the difference does not say what the color was: %s", diffs[0])
	}
	// The text is identical, so anything comparing text alone sees nothing.
	if colored.String() != plain.String() {
		t.Errorf("the two renders differ in text as well, so this proves less than it claims")
	}
}

// A nearby color is a difference, because a prompt that quietly substitutes
// is the failure this repository treats as its worst.
func TestANeighboringColorIsADifference(t *testing.T) {
	if diffs := Compare(gridOf(t, 10, "\x1b[38;5;31mX"), gridOf(t, 10, "\x1b[38;5;32mX")); len(diffs) != 1 {
		t.Errorf("31 and 32 compared as %d differences, want 1", len(diffs))
	}
	if diffs := Compare(gridOf(t, 10, "\x1b[38;2;30;102;245mX"), gridOf(t, 10, "\x1b[38;5;31mX")); len(diffs) != 1 {
		t.Errorf("a triple and an index compared as the same color")
	}
}

// An untouched cell and a space are the same screen; a space with a
// background is not, because that is paint and it is what a framed prompt
// is made of.
func TestABlankCellIsPaintOnlyWhenItIsPainted(t *testing.T) {
	if diffs := Compare(gridOf(t, 10, "A"), gridOf(t, 10, "A ")); len(diffs) != 0 {
		t.Errorf("a trailing space compared as a difference: %v", diffs)
	}
	diffs := Compare(gridOf(t, 10, "A "), gridOf(t, 10, "A\x1b[48;5;4m "))
	if len(diffs) != 1 {
		t.Fatalf("a painted blank compared as %d differences, want 1: %v", len(diffs), diffs)
	}
	if !strings.Contains(diffs[0].String(), "bg=4") {
		t.Errorf("the difference does not name the background: %s", diffs[0])
	}
}

// The cursor moves a redraw is written with, so a screen assembled out of
// order is the screen a person sees.
//
// An incremental redraw writes whichever of several equivalent sequences is
// shortest, so a model that read them literally would report a difference in
// the *route* rather than in the result.
func TestTheScreenIsWhatTheMovesLeaveBehind(t *testing.T) {
	// Written in one go, and then written again out of order with the same
	// destination: a carriage return, a move up, an erase and a rewrite.
	straight := gridOf(t, 20, "one\r\ntwo")
	assembled := gridOf(t, 20, "XXX\r\nYYY", "\r\x1b[1A\x1b[Kone\r\n\x1b[Ktwo")
	if diffs := Compare(straight, assembled); len(diffs) != 0 {
		t.Errorf("the same screen reached two ways compared as different: %v\n%q vs %q",
			diffs, straight.String(), assembled.String())
	}
}

// A grapheme is a cell, so a combining mark does not shift everything after
// it.
func TestACombiningMarkJoinsTheCellBeforeIt(t *testing.T) {
	g := gridOf(t, 10, "éx")
	if got := g.Cell(0, 0).Text; got != "é" {
		t.Errorf("cell 0 = %q, want the letter and its accent", got)
	}
	if got := g.Cell(0, 1).Text; got != "x" {
		t.Errorf("cell 1 = %q, want the next character", got)
	}
}

// Bytes arriving in pieces are the ordinary case on a pipe, and a rune split
// across two writes must not become a replacement character.
func TestARuneSplitAcrossTwoWritesSurvives(t *testing.T) {
	whole := "❯"
	g := New(10)
	if _, err := g.Write([]byte(whole)[:1]); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Write([]byte(whole)[1:]); err != nil {
		t.Fatal(err)
	}
	if got := g.Cell(0, 0).Text; got != whole {
		t.Errorf("cell 0 = %q, want %q", got, whole)
	}
	// And an escape sequence split the same way.
	split := New(10)
	if _, err := split.Write([]byte("\x1b[38;5;")); err != nil {
		t.Fatal(err)
	}
	if _, err := split.Write([]byte("31mX")); err != nil {
		t.Fatal(err)
	}
	if got := split.Cell(0, 0); got.Fg.String() != "31" {
		t.Errorf("a split escape lost its color: %s", describe(got))
	}
}

// A sequence this model does not know paints nothing rather than being
// guessed at, and the text around it still lands where a terminal would put
// it.
func TestAnUnknownSequencePaintsNothing(t *testing.T) {
	g := gridOf(t, 20, "A\x1b]0;a title\x07B\x1b[?2004hC")
	if got := g.Text(0); got != "ABC" {
		t.Errorf("row = %q, want ABC — a sequence that paints nothing drew something", got)
	}
}

// The deferred wrap: a terminal that has filled a row stays on it until
// there is another character to put somewhere.
func TestTheWrapIsDeferredUntilThereIsSomethingToPut(t *testing.T) {
	g := gridOf(t, 3, "abc")
	if g.Rows() != 1 {
		t.Errorf("a row filled exactly wrapped early: %d rows", g.Rows())
	}
	g = gridOf(t, 3, "abcd")
	if g.Rows() != 2 || g.Cell(1, 0).Text != "d" {
		t.Errorf("the fourth character did not land on the second row: %q", g.String())
	}
}
