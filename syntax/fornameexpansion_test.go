// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// `n=x; for $n in a b` — a loop whose *first* name is an expansion. Refused by
// every shell in the panel, and taken here in every dialect: the loop bound a
// variable literally called `n`, so the script's own `$n` read the list's
// words and the name it meant to reach stayed empty, at status 0 with nothing
// said. The wordings are dialect/*/forname_test.go; this is the predicate.

// loops is the core with `select` and `foreach` on, so all three spellings of
// the header are reachable in one place.
func loops() syntax.Dialect {
	d := syntax.Core()
	d.Select = true
	d.Foreach = true
	return d
}

// forNameError parses src and returns the refusal, failing if there is none.
func forNameError(t *testing.T, src string, d syntax.Dialect) *syntax.Error {
	t.Helper()
	_, err := syntax.Parse(src, d)
	if err == nil {
		t.Fatalf("%q parsed; every shell in the panel refuses it", src)
	}
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("%q: err = %v, want a *syntax.Error", src, err)
	}
	return se
}

// Every spelling of an expansion in the name position, in all three loops.
// Unanimous across the panel, so it is refused whatever the dialect — and the
// quoting axis below does not reach it: the shell that removes quoting still
// refuses `"$n"`.
func TestANameMayNotComeOutOfAnExpansion(t *testing.T) {
	for _, name := range []string{"$n", "${n}", `"$n"`, "$(echo n)", "`echo n`", "$((1))"} {
		for _, loop := range []struct{ head, tail string }{
			{"for " + name + " in a b; do :; done", "for"},
			{"select " + name + " in a b; do :; done", "select"},
			{"foreach " + name + " ( a b )\n:\nend", "foreach"},
		} {
			se := forNameError(t, loop.head, loops())
			if se.Kind != syntax.ErrForName {
				t.Errorf("%q: kind = %v, want ErrForName", loop.head, se.Kind)
			}
			// Reported as written, which is what three of the four dialects
			// quote back. The literal would be `n` for the first four of
			// these, and no shell says `n`.
			if se.Token != name {
				t.Errorf("%q: blamed %q, want %q", loop.head, se.Token, name)
			}
			// And with the quoting flag on, so the axis is not what is
			// refusing it.
			d := loops()
			d.ForNameMayBeQuoted = true
			if se := forNameError(t, loop.head, d); se.Kind != syntax.ErrForName {
				t.Errorf("%q with quoting allowed: kind = %v, want ErrForName", loop.head, se.Kind)
			}
		}
	}
}

// The name the loop would otherwise have bound is the reading this replaces,
// and it is worth naming: the token for `$n` reports its literal as `n`, so
// [isName] was satisfied by a word that names nothing yet. A plain name still
// parses, which is the control.
func TestAPlainNameStillParses(t *testing.T) {
	for _, src := range []string{
		"for i in a b; do :; done",
		"select i in a b; do :; done",
		"foreach i ( a b )\n:\nend",
		"for i_2 in a; do :; done",
		"for _x in a; do :; done",
	} {
		mustParseHere(t, src, loops(), "a plain name needs nothing")
	}
}

// Whether the word may be *quoted* is an axis and not the rule above. One
// shell removes the quoting and takes the name; the other four want it plain.
// The escape travels with the quotes rather than being its own question —
// measured, all five spellings run in that one shell and none in the rest.
func TestWhetherAQuotedNameIsOneIsAnAxis(t *testing.T) {
	quoted := loops()
	quoted.ForNameMayBeQuoted = true
	for _, tc := range []struct{ name, binds string }{
		{`"i"`, "i"},
		{`'i'`, "i"},
		{`i""`, "i"},
		{`"i"x`, "ix"},
		{`\i`, "i"},
	} {
		src := "for " + tc.name + " in a b; do :; done"
		c, ok := onlyCommand(t, src, quoted).(*syntax.ForClause)
		if !ok {
			t.Fatalf("%q: not a for", src)
		}
		if len(c.Names) != 1 || c.Names[0] != tc.binds {
			t.Errorf("%q: names = %v, want [%s] — the quoting comes off before the name is read",
				src, c.Names, tc.binds)
		}
		// Without the flag the same word is refused, and blamed as written.
		se := forNameError(t, src, loops())
		if se.Token != tc.name {
			t.Errorf("%q: blamed %q, want %q", src, se.Token, tc.name)
		}
	}
}

// A word that is not a name in any reading is still refused under the axis, so
// the flag is about the *quoting* and not about what counts as a name.
func TestTheQuotingAxisDoesNotWidenWhatCountsAsAName(t *testing.T) {
	quoted := loops()
	quoted.ForNameMayBeQuoted = true
	for _, name := range []string{"1x", `"1x"`, `"a b"`, `""`, `"a-b"`} {
		src := "for " + name + " in a b; do :; done"
		if se := forNameError(t, src, quoted); se.Token != name {
			t.Errorf("%q: blamed %q, want %q", src, se.Token, name)
		}
	}
}

// The refusal is located at the word rather than at the keyword, so a
// diagnostic that names a line names the loop's.
func TestTheRefusalIsLocatedAtTheWord(t *testing.T) {
	const src = "echo one\nfor $n in a b; do :; done\n"
	se := forNameError(t, src, loops())
	if se.Pos.Line != 2 {
		t.Errorf("line = %d, want 2", se.Pos.Line)
	}
	if want := len("echo one\nfor "); se.Pos.Offset != want {
		t.Errorf("offset = %d, want %d — the word, not the keyword", se.Pos.Offset, want)
	}
}
