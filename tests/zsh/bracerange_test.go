// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A range between two single characters spans whatever those characters are,
// and a range that cannot be counted loses its **braces** rather than
// standing whole.
//
// Both were measured on zsh 5.9.2, 2026-09-14, and both were bash's answer
// here: `{1..x}` was the word as written and `@{1..}@` kept its braces
// (#1692, #1691). They are one file because they are one scan — the character
// reading is tried before a body with a gap in it is ever called a failure,
// which is what makes `{1...}` the range `1` to `.` and not a missing
// endpoint.
//
// The `@` on each side of the collapsing rows is load-bearing: without it a
// row cannot tell "the braces came off" from "the braces were never there",
// and `printf '[%s]'` is what shows the characters rather than the status —
// every column here is at 0.
func TestARangeCountsBetweenAnyTwoCharacters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"digits to a letter", `printf '[%s]' {5..A}`, "[5][6][7][8][9][:][;][<][=][>][?][@][A]"},
		{"and back down", `printf '[%s]' {A..5}`, "[A][@][?][>][=][<][;][:][9][8][7][6][5]"},
		{"punctuation at both ends", `printf '[%s]' {!..#}`, "[!][\"][#]"},
		// The body is counted in characters rather than cut at its first
		// `..`, which is the whole of the difference on these two.
		{"a body of four dots is the range from a dot to a dot", `printf '[%s]' {....}`, "[.]"},
		{"a body of five is left alone", `printf '[%s]' {.....}`, "[{.....}]"},
		{"three is left alone too", `printf '[%s]' {...}`, "[{...}]"},
		{"a trailing dot is an endpoint", `printf '[%s]' {1...}`, "[1][0][/][.]"},
		// The wider reading takes no step, where the letters-only one bash
		// and ksh93 have does.
		{"a step takes the body out of the reading", `printf '[%s]' {a..c..1}`, "[{a..c..1}]"},
		// And the spelling both readings share is unchanged.
		{"two letters still count", `printf '[%s]' {a..e}`, "[a][b][c][d][e]"},
		{"either direction", `printf '[%s]' {e..a}`, "[e][d][c][b][a]"},
		// A number written at both ends is read as a number first, which is
		// what keeps the padding: a character range would answer `1` here.
		{"two numbers are still numbers", `printf '[%s]' {01..03}`, "[01][02][03]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// What a body shaped like a numeric range and holding no range leaves behind.
//
// The last five rows are the boundary and they are not decoration: an
// implementation that dropped the braces from anything holding a `..` passes
// every row above them and fails all five. `{..2..}` against `{1..2..}` is
// the pair that says the rule is about *digits at the ends* rather than about
// emptiness, and it is the row a first reading of #1691 had backwards.
func TestARangeThatCannotBeCountedLosesItsBraces(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a missing second endpoint", `printf '[%s]' @{1..}@`, "[@1..@]"},
		{"a missing first endpoint", `printf '[%s]' @{..3}@`, "[@..3@]"},
		{"a missing step", `printf '[%s]' @{1..2..}@`, "[@1..2..@]"},
		{"a missing middle", `printf '[%s]' @{1....2}@`, "[@1....2@]"},
		{"leading zeros go the same way", `printf '[%s]' @{08..}@`, "[@08..@]"},
		{"a step of zero goes the same way", `printf '[%s]' @{1..2..0}@`, "[@1..2..0@]"},
		{"a step that is only a sign too", `printf '[%s]' @{1..2..-}@`, "[@1..2..-@]"},
		// The boundary.
		{"no digit at either end is left alone", `printf '[%s]' @{..}@`, "[@{..}@]"},
		{"and with a digit only in the middle", `printf '[%s]' @{..2..}@`, "[@{..2..}@]"},
		{"a sign on the first endpoint is left alone", `printf '[%s]' @{-1..}@`, "[@{-1..}@]"},
		{"a plus anywhere is left alone", `printf '[%s]' @{+1..2}@`, "[@{+1..2}@]"},
		{"a letter in the step is left alone", `printf '[%s]' @{1..2..x}@`, "[@{1..2..x}@]"},
		// And a range that counts is untouched by any of it.
		{"a range that counts still counts", `printf '[%s]' @{1..3}@`, "[@1@][@2@][@3@]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// What a range counted is data; what a list held is text the word still has
// to expand.
//
// Invisible until a character range is wider than the letters, and decisive
// the moment it is: `{5..A}` holds a `?` of its own, and a shell that put its
// elements back as ordinary text would match that `?` against the directory
// and then refuse the word for matching nothing.
func TestACountedElementIsNotAPattern(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"q", "z"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a range's metacharacter is a character", `printf '[%s]' {=..?}`, "[=][>][?]"},
		{"a list's is a pattern", `printf '[%s]' {?,x}`, "[q][z][x]"},
		// And the text a collapsed range leaves is a word like any other, so
		// it still matches. The pair is what keeps "quoted" from spreading
		// from the elements to everything braces produce.
		{"what the braces left is still a pattern", `: > 1..x; printf '[%s]' {1..}*`, "[1..x]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if strings.TrimRight(out, "\n") != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A range between two characters is the *locale's* characters, which is the
// same question a pattern's `?` asks and is asked the same way.
//
// Measured on zsh 5.9.2: `{α..γ}` is three words under a UTF-8 locale and the
// word as written under `LC_ALL=C`, where each endpoint is two bytes and
// neither is one character. Both rows, because an implementation that counted
// runes whatever the locale said passes the first and fails the second — and
// the corpus records this under `LC_ALL=C`, so the second is the one a
// run there would have graded.
func TestARangeBetweenTwoCharactersFollowsTheLocale(t *testing.T) {
	for _, tc := range []struct{ name, locale, want string }{
		{"a UTF-8 locale counts three characters", "C.UTF-8", "[α][β][γ]"},
		{"a single-byte locale counts none", "C", "[{α..γ}]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, "LC_ALL="+tc.locale+"; printf '[%s]' {α..γ}")
			if out != tc.want || st != 0 {
				t.Errorf("under %s gave %q at %d, want %q at 0", tc.locale, out, st, tc.want)
			}
		})
	}
}
