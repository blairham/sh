// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A value's end is not the field's end, and a backslash that runs out of value
// quotes the **field's** next character.
//
// `bs='\'; echo x${bs}?` is one word of three spans: the literal `x`, the value's
// backslash, and a live `?`. Under the reading that has a value's backslash
// quote what follows it there is then no metacharacter left in the field at all
// and nothing is matched — where this shell wrote the backslash as an ordinary
// literal, left the `?` live, matched two files and handed them on at status 0.
// That is the quietest of the rows: a wrong answer with no diagnostic anywhere
// (#4234).
//
// Asserted under each of the three readings of Semantics.ValueBackslashInAPattern
// rather than against one shell, because the mark is what lets the axis be asked
// at all: with the backslash written as a literal there is nothing for the three
// readings to differ about, and all three answered the same wrong word.
func TestABackslashThatEndsAValueQuotesTheFieldsNextCharacter(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"x*", `x\y`, "xy", "xz"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name    string
		reading ValueBackslashPolicy
		// what `x${bs}?` and `x${bs}*` come to
		question, star string
	}{
		// The `?` is quoted, so the field is no pattern and the word stands as
		// it was written — backslash and all, since no shell removes an
		// expansion's quoting.
		{"quotes what follows", ValueBackslashQuotesWhatFollows, `[x\?]`, `[x\*]`},
		// The backslash is a character of the pattern and what follows it is
		// not live, so the field spells the name `x\?`, which nothing here is
		// called.
		{"disarms what follows", ValueBackslashDisarmsWhatFollows, `[x\?]`, `[x\*]`},
		// The backslash is a character and the metacharacter stays live, so
		// the pattern is a literal backslash and then any name.
		{"data", ValueBackslashIsData, `[x\y]`, `[x\y]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ask := func(s *Semantics) { s.ValueBackslashInAPattern = tc.reading }
			for _, row := range []struct{ src, want string }{
				{`bs='\'; printf '[%s]' x${bs}?`, tc.question},
				{`bs='\'; printf '[%s]' x${bs}*`, tc.star},
			} {
				out, st := axisRunIn(t, dir, row.src, ask)
				if st != 0 || out != row.want {
					t.Errorf("%s: got %q status %d, want %q", row.src, out, st, row.want)
				}
			}
		})
	}
}

// And the character it quotes may be the `/` that closes a piece of a path,
// which is the one character a quote cannot take the meaning off: the separator
// still separates, so the backslash stays at the end of the piece in front of it
// and that piece decides what becomes of it.
//
// A piece that **spells** a name has it removed with the rest of that piece's
// quoting — unanimous — and a piece that **describes** one is
// Semantics.ValueBackslashSurvivesAPatternPiece. The tree holds `x\` and `y`,
// each with a file `e`, so the two readings are each other's control: one
// matches the directory whose name ends in a backslash and misses on the one
// that does not, and the other does the reverse.
func TestABackslashEndingAValueInFrontOfASeparatorStaysInItsPiece(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{`x\`, "y"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, d, "e"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const bs = `bs='\'; `
	for _, tc := range []struct {
		name     string
		survives Answer
		// `./[x]${bs}/e` and `./[y]${bs}/e`
		hit, miss string
	}{
		{"it stays in the piece", Yes, `[./x\/e]`, `[./[y]\/e]`},
		{"it goes with the quoting", No, `[./[x]\/e]`, `[./y/e]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ask := func(s *Semantics) {
				s.ValueBackslashInAPattern = ValueBackslashQuotesWhatFollows
				s.ValueBackslashSurvivesAPatternPiece = tc.survives
			}
			for _, row := range []struct{ src, want string }{
				{bs + `printf '[%s]' ./[x]${bs}/e`, tc.hit},
				{bs + `printf '[%s]' ./[y]${bs}/e`, tc.miss},
			} {
				out, st := axisRunIn(t, dir, row.src, ask)
				if st != 0 || out != row.want {
					t.Errorf("%s: got %q status %d, want %q", row.src, out, st, row.want)
				}
			}
		})
	}
	// The unanimous half, which is what says the axis is about a pattern piece
	// and not about the separator: a piece that spells a name loses the
	// backslash under either answer.
	for _, survives := range []Answer{Yes, No} {
		ask := func(s *Semantics) {
			s.ValueBackslashInAPattern = ValueBackslashQuotesWhatFollows
			s.ValueBackslashSurvivesAPatternPiece = survives
		}
		if out, st := axisRunIn(t, dir, bs+`printf '[%s]' ./y${bs}/[e]`, ask); st != 0 || out != `[./y/e]` {
			t.Errorf("a spelled piece under %v: got %q status %d, want %q", survives, out, st, `[./y/e]`)
		}
	}
}

// The axis is asked only where a value's backslash ran out of value in front of
// a separator **and** the piece it ends describes a name. Every other shape is
// one word under both answers, which is nearly every word anybody writes.
func TestTheValueBackslashPieceAxisIsNotAskedWhereBothAnswersAgree(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tmp", "a", "b"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmp", "a", "b", "c"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ask := func(s *Semantics) { s.ValueBackslashInAPattern = ValueBackslashQuotesWhatFollows }
	for _, tc := range []struct{ name, src, want string }{
		// The piece in front of the quoted separator spells a name.
		{"a spelled piece", `bs='\'; printf '[%s]' ./tmp${bs}/a/b/*`, `[./tmp/a/b/c]`},
		{"a spelled piece mid-word", `bs='\'; printf '[%s]' ./t${bs}mp/a/b/*`, `[./tmp/a/b/c]`},
		// No separator behind the backslash at all.
		{"no separator behind it", `bs='\'; printf '[%s]' tmp${bs}a`, `[tmp\a]`},
		// No value backslash at all.
		{"no value backslash", `printf '[%s]' ./tmp/a/b/*`, `[./tmp/a/b/c]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := axisRunIn(t, dir, tc.src, ask); st != 0 || out != tc.want {
				t.Errorf("%s: got %q status %d, want %q with no question asked", tc.src, out, st, tc.want)
			}
		})
	}
}

// axisRunIn runs a snippet in dir under the axes these rows reach, with
// testSemantics rather than the standard's preset for the reason vector_test.go
// gives: the core refuses what the shells disagree about, and these rows are
// full of it.
func axisRunIn(t *testing.T, dir, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	sem := testSemantics()
	set(&sem)
	return run(t, src, func(r *Runner) {
		r.Semantics, r.Dir = &sem, dir
	})
}

// The doubled mark is spelled out as a literal, because a Go constant cannot be
// built from another byte constant, so nothing but this holds the two together.
//
// A cheap test for a cheap mistake: changing valueBackslashMark and leaving the
// pair behind would put a mark in every field that reads as data and take the
// end-of-value backslash out of every field that has one, silently.
func TestTheTwoValueBackslashMarksAgree(t *testing.T) {
	if want := string([]byte{ValueBackslashMarkForTest, ValueBackslashMarkForTest}); ValueBackslashRanOutOfValueForTest != want {
		t.Errorf("the doubled mark is %q, want two of %q", ValueBackslashRanOutOfValueForTest, ValueBackslashMarkForTest)
	}
}

// A value's trailing backslash has nothing to quote when the script had already
// quoted what follows, so it is a character of the pattern like any other.
//
// Under the reading that has such a backslash quote what follows, this shell
// spent it on a character that had nothing live about it — the `\*` the script
// wrote — and the two collapsed into one. Every column in the panel produces one
// field there and this shell produced two, which is what says it is a defect
// rather than a reading: see dialect/bash/valuebackslashquotedalready_test.go
// for the seven-column measurement (#4158).
//
// Asserted under all three readings, because the fix is about the *absence* of
// something to quote and no reading has anything to quote here.
func TestAValueBackslashBeforeSomethingAlreadyQuotedIsACharacter(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a*b", `a\*b`, `a\\*b`, "ab"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, reading := range []ValueBackslashPolicy{
		ValueBackslashQuotesWhatFollows, ValueBackslashDisarmsWhatFollows, ValueBackslashIsData,
	} {
		ask := func(s *Semantics) { s.ValueBackslashInAPattern = reading }
		// `v` ends in a backslash and `\\` is a backslash the script quoted, so
		// the pattern spells `a`, two backslashes, a live `*` and a `b` — one
		// name, the one with two backslashes in it.
		out, st := axisRunIn(t, dir, `v='a\'; printf '[%s]' $v\\*b`, ask)
		if want := `[a\\*b]`; out != want || st != 0 {
			t.Errorf("%v: said %q (status %d), want %q at 0", reading, out, st, want)
		}
	}
}
