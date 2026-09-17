// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package histexpand

import (
	"errors"
	"testing"
)

// Rows measured 2026-09-16 on bash 5.3.20 from a script, with the history
// named in each table; the columns the other shells were measured in are
// named where a row is theirs too. Every "want" is the line bash echoed to
// standard error before running it.

func expandAll(t *testing.T, h List, c Chars, rows []struct{ in, want string }) {
	t.Helper()
	for _, row := range rows {
		got, err := Expand(row.in, h, c)
		if err != nil {
			t.Errorf("%q: unexpected error %v", row.in, err)
			continue
		}
		if got.Line != row.want {
			t.Errorf("%q = %q, want %q", row.in, got.Line, row.want)
		}
	}
}

// Ranges, over `echo a b c d e`. The rows bash, zsh and ksh93 agree on.
func TestAWordRangeRunsToTheLastWordAndFromWordZero(t *testing.T) {
	h := List{Lines: []string{"echo a b c d e"}, First: 1}
	expandAll(t, h, Default, []struct{ in, want string }{
		{"echo !!:2-$", "echo b c d e"},
		{"echo !!:-3", "echo echo a b c"},
		{"echo !!:-", "echo echo a b c d"},
		{"echo !!:-$", "echo echo a b c d e"},
		{"echo !!-3", "echo echo a b c"},
		{"echo !-1-3", "echo echo a b c"},
		{"echo !!:*-", "echo a b c d e-"},
		{"echo !ech-2", "echo echo a b"},
		{"echo !e-2x", "echo echo a bx"},
	})
}

// A word the event does not have stops the line, naming the designator.
func TestAWordTheEventDoesNotHaveIsRefused(t *testing.T) {
	h := List{Lines: []string{"echo a b c d e"}, First: 1}
	for _, c := range []struct{ in, ref string }{
		{"echo !!:9", ":9"},
		{"echo !!:2-9", ":2-9"},
		{"echo !!:3-2", ":3-2"},
		{"echo !!:9-", ":9-"},
		{"echo !!:9*", ":9*"},
		{"echo !!-9", "-9"},
	} {
		_, err := Expand(c.in, h, Default)
		var bad *BadWordSpecifier
		if !errors.As(err, &bad) || bad.Ref != c.ref {
			t.Errorf("%q: err = %v, want a bad word specifier naming %q", c.in, err, c.ref)
		}
	}
	// And the edge that is not: a range from the last word with no end is
	// empty rather than refused.
	expandAll(t, h, Default, []struct{ in, want string }{{"echo !!:5-", "echo "}})
}

// `&` on the right of a substitution is the text it replaced; `\&` is an `&`.
// bash, zsh and ksh93.
func TestAnAmpersandInAReplacementIsTheMatchedText(t *testing.T) {
	h := List{Lines: []string{"echo one two one"}, First: 1}
	expandAll(t, h, Default, []struct{ in, want string }{
		{"!!:s/o/&&/", "echoo one two one"},
		{`!!:s/o/\&/`, "ech& one two one"},
		{"^o^[&]^", "ech[o] one two one"},
	})
}

// The last substitution and search outlive the line that made them, where the
// caller keeps a Memory — and do not where it does not.
func TestTheLastSubstitutionOutlivesItsLine(t *testing.T) {
	h := List{Lines: []string{"echo one two one"}, First: 1}
	m := &Memory{}
	withMemory := List{Lines: h.Lines, First: 1, Memory: m}
	if _, err := Expand("!!:s/o/0/", withMemory, Default); err != nil {
		t.Fatal(err)
	}
	expandAll(t, withMemory, Default, []struct{ in, want string }{
		{"!!:&", "ech0 one two one"},
		{"!!:g&", "ech0 0ne tw0 0ne"},
		{"!!:s//X/", "echX one two one"},
		{"^^X^", "echX one two one"},
	})
	_, err := Expand("!!:&", h, Default)
	var none *NoPreviousSubstitution
	if !errors.As(err, &none) || none.Ref != ":&" {
		t.Errorf("a fresh expansion's :& = %v, want no previous substitution naming :&", err)
	}
	_, err = Expand("!!:g&", h, Default)
	if !errors.As(err, &none) || none.Ref != ":g&" {
		t.Errorf("a fresh expansion's :g& = %v, want no previous substitution naming :g&", err)
	}

	// With no substitution yet, an empty left side is the last search.
	searched := List{Lines: []string{"echo one two"}, First: 1, Memory: &Memory{}}
	if _, err := Expand("echo !?two?%", searched, Default); err != nil {
		t.Fatal(err)
	}
	expandAll(t, searched, Default, []struct{ in, want string }{{"!!:s//X/", "echo one X"}})
}

// `%` is the word the search matched in, read from the end of the event.
func TestTheMatchedWordIsReadFromTheEnd(t *testing.T) {
	h := List{Lines: []string{"echo two.three"}, First: 1}
	expandAll(t, h, Default, []struct{ in, want string }{{"echo !?e?%", "echo two.three"}})
}

// The three word readings. The first six rows are the measured table on
// Words, in all three columns; the rows after them were measured in bash
// alone, and the other two columns there are what their rule gives.
func TestAnEventIsCutIntoWordsTheWayTheDialectReadsThem(t *testing.T) {
	cases := []struct {
		event, ref       string
		quotes, shell, z string
	}{
		{`echo "a b"c d`, ":1", `"a b"c`, `"a b"c`, `"a b"c`},
		{`echo a\ b c`, ":1", `a\`, `a\ b`, `a\ b`},
		{`echo $(echo x y) z`, ":1", `$(echo`, `$(echo x y)`, `$(echo x y)`},
		{`echo ${v:-a b} z`, ":1", `${v:-a`, `${v:-a`, `${v:-a b}`},
		{`echo x;echo b`, ":2", `b`, `;`, `;`},
		{`echo a 2>/dev/null`, ":2", `2>/dev/null`, `2>`, `2>`},
		{`echo a 2>&1 b`, ":2", `2>&1`, `2>&1`, `2>&1`},
		{`echo a |& cat`, ":3", `cat`, `&`, `&`},
		{`echo 12>f b`, ":1", `12>f`, `12>`, `12>`},
		{`echo x<(echo y z) w`, ":1", `x<(echo`, `x<(echo y z)`, `x<(echo y z)`},
		{`echo /+(one|two)/x y`, ":1", `/+(one|two)/x`, `/+(one|two)/x`, `/+(one|two)/x`},
		{`case a in a) echo b;; esac`, ":4", `echo`, `)`, `)`},
	}
	for _, c := range cases {
		h := List{Lines: []string{c.event}, First: 1}
		for _, reading := range []struct {
			words Words
			want  string
		}{{WordsQuotes, c.quotes}, {WordsShell, c.shell}, {WordsShellBraces, c.z}} {
			chars := Default
			chars.Words = reading.words
			got, err := Expand("!!"+c.ref, h, chars)
			if err != nil || got.Line != reading.want {
				t.Errorf("%q !!%s under reading %d = %q, %v; want %q", c.event, c.ref, reading.words, got.Line, err, reading.want)
			}
		}
	}
}

// Where a `:q` applies is the dialect's.
func TestAQuoteModifierAppliesLastOrInPlace(t *testing.T) {
	h := List{Lines: []string{"echo two.three"}, First: 1}
	expandAll(t, h, Default, []struct{ in, want string }{{"echo !$:q:r", "echo 'two'"}})
	inPlace := Default
	inPlace.QuoteInPlace = true
	expandAll(t, h, inPlace, []struct{ in, want string }{{"echo !$:q:r", "echo 'two"}})
}

// A word beginning with the comment character ends expansion, where the
// dialect says so.
func TestACommentWordEndsExpansion(t *testing.T) {
	h := List{Lines: []string{"echo a"}, First: 1}
	stops := Default
	stops.CommentStops = true
	expandAll(t, h, stops, []struct{ in, want string }{
		{"echo ab c # !!", "echo ab c # !!"},
		{"echo a;#!!", "echo a;#!!"},
		{"echo ab c#!!", "echo ab c#echo a"},
		{`echo "#" !!`, `echo "#" echo a`},
	})
	expandAll(t, h, Default, []struct{ in, want string }{{"echo ab c # !!", "echo ab c # echo a"}})
}

// Double quotes protect a reference where the dialect says so, and an
// unquoted one on the same line still expands.
func TestDoubleQuotesProtectWhereTheDialectSaysSo(t *testing.T) {
	h := List{Lines: []string{"echo a"}, First: 1}
	protects := Default
	protects.DoubleQuotesProtect = true
	expandAll(t, h, protects, []struct{ in, want string }{{`echo "!!" !!`, `echo "!!" echo a`}})
	expandAll(t, h, Default, []struct{ in, want string }{{`echo "!!" !!`, `echo "echo a" echo a`}})
}
