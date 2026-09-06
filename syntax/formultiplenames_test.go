// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// `for key value ( a 1 b 2 ) { … }` — a loop that names more than one
// variable, which one shell in the panel has. The grammar half; what the
// names are then bound to is interp/formultiplenames_test.go.

// manyNames is the short-form dialect with the name list turned on, which is
// the combination zsh has. Both flags are set separately here on purpose: the
// point of the pair is that they are independent, and a helper that turned
// them on together would hide the four combinations the next test walks.
func manyNames() syntax.Dialect {
	d := short()
	d.ForMultipleNames = true
	d.ForBraceBody = true
	d.Foreach = true
	return d
}

func forNames(t *testing.T, src string, d syntax.Dialect) []string {
	t.Helper()
	c, ok := onlyCommand(t, src, d).(*syntax.ForClause)
	if !ok {
		t.Fatalf("parse %q: not a for", src)
	}
	return c.Names
}

// The four combinations of the name count and the body spelling, each parsed
// under the flag that should decide it and refused under the dialect without.
// Measured in zsh: all four run there, which is what says these are two
// features and not one.
func TestTheNameListAndTheBraceBodyAreIndependent(t *testing.T) {
	for _, tc := range []struct {
		src   string
		names int
	}{
		{"for a b in x 1 y 2; do echo $a$b; done", 2},
		{"for a b ( x 1 y 2 ) { echo $a$b }", 2},
		{"for a ( x y ) { echo $a }", 1},
		{"for a b ( x 1 y 2 ); do echo $a$b; done", 2},
		{"for a in x y; { echo $a }", 1},
		{"for a b in x 1 y 2; { echo $a$b }", 2},
		{"for a b; do echo $a$b; done", 2},
		{"for a b { echo $a$b }", 2},
	} {
		if got := forNames(t, tc.src, manyNames()); len(got) != tc.names {
			t.Errorf("%q: names = %v, want %d of them", tc.src, got, tc.names)
		}
		// Without the flag, the one-name spellings still parse and the
		// others do not — the flag is the name count and nothing else.
		d := manyNames()
		d.ForMultipleNames = false
		if tc.names == 1 {
			mustParseHere(t, tc.src, d, "one name needs no flag")
		}
	}
}

// A dialect without the flag refuses the second name rather than binding one
// and running on — which is what it did before the flag existed, and the half
// worth pinning: a loop that bound nothing and printed one empty pair would
// have been the silent version of this.
func TestASecondNameNeedsTheFlag(t *testing.T) {
	d := manyNames()
	d.ForMultipleNames = false
	mustFailHere(t, "for a b in x 1 y 2; do echo $a; done", d, "no flag, no second name")
	mustFailHere(t, "for a b ( x 1 y 2 ) { echo $a }", d, "and not in the short spelling either")
}

// The names come back in the order they were written, and the header quotes
// all of them.
func TestEveryNameIsKeptInOrder(t *testing.T) {
	c, ok := onlyCommand(t, "for a b c ( 1 2 3 ) { : }", manyNames()).(*syntax.ForClause)
	if !ok {
		t.Fatal("not a for")
	}
	if got := strings.Join(c.Names, ","); got != "a,b,c" {
		t.Errorf("Names = %q, want a,b,c", got)
	}
	if c.Header != "for a b c ( 1 2 3 )" {
		t.Errorf("Header = %q, want the whole header", c.Header)
	}
}

// What ends the name list. Each of these binds exactly one name, because the
// second word is the thing that ends the header rather than another name.
func TestTheNameListEndsAtTheHeader(t *testing.T) {
	for _, tc := range []struct{ src, why string }{
		{"for a in b c; do : ; done", "`in` ends it and is then an item"},
		{"for a ( b c ) { : }", "a parenthesis ends it"},
		{"for a; do : ; done", "a separator ends it"},
		{"for a\ndo : \ndone", "a newline ends it"},
		{"for a do : ; done", "`do` ends it"},
		{"for a { : }", "a brace ends it"},
	} {
		if got := forNames(t, tc.src, manyNames()); len(got) != 1 || got[0] != "a" {
			t.Errorf("%s: %q gave names %v, want just a", tc.why, tc.src, got)
		}
	}
}

// `in` is only a keyword after the first word: a loop must have a name, so the
// word right after `for` is one whatever it spells. Measured in zsh —
// `for in ( 1 2 ) { print $in }` prints 1 and 2 there.
func TestTheFirstWordIsANameEvenWhenItSpellsAKeyword(t *testing.T) {
	if got := forNames(t, "for in ( 1 2 ) { : }", manyNames()); len(got) != 1 || got[0] != "in" {
		t.Errorf("names = %v, want just in", got)
	}
}

// The subtractive half, and the reason the flag cannot be a pure addition: a
// short body may no longer stand directly after the names, because the words
// there are names. Measured in zsh, `set -- p q; for a print -r -- "[$a]"` is
// a parse error near `-r`.
func TestAShortBodyMayNotFollowTheNamesDirectly(t *testing.T) {
	mustFailHere(t, `for a print -r -- x`, manyNames(),
		"`print` is a second name and `-r` is not a name")
	// The dialect without the flag keeps the older, wider reading.
	d := manyNames()
	d.ForMultipleNames = false
	mustParseHere(t, `for a print -r -- x`, d, "without the flag the word begins the body")
	// And a header that ended itself still takes a short body.
	mustParseHere(t, `for a b ( 1 2 ) print -r -- x`, manyNames(), "a parenthesized list ends the header")
	mustParseHere(t, `for a b; print -r -- x`, manyNames(), "a separator ends the header")
}

// A name is a plain unquoted name, and anything else is refused where it
// stands rather than quietly becoming a body. Both spellings are parse errors
// in zsh.
func TestAForNameIsNeitherQuotedNorExpanded(t *testing.T) {
	mustFailHere(t, `for a 1x ( 1 2 ) { : }`, manyNames(), "1x is not a name")
	mustFailHere(t, `for a "b" ( 1 2 ) { : }`, manyNames(), "a quoted word is not a name")
	mustFailHere(t, `for a $n ( 1 2 ) { : }`, manyNames(), "a name is not expanded")
}

// `select` does not take the flag, however much of the rest of the header it
// shares. Measured: `select a b (x y) { … }` is a parse error in the shell
// that accepts every other spelling here.
func TestSelectTakesOneNameEvenWithTheFlagOn(t *testing.T) {
	d := manyNames()
	d.Select = true
	mustFailHere(t, `select a b ( x y ) { : }`, d, "select has one name")
	mustParseHere(t, `select a ( x y ) { : }`, d, "and still takes it")
}

// `foreach` does take it — the same loop under two other words.
func TestForeachTakesTheNameList(t *testing.T) {
	c, ok := onlyCommand(t, "foreach a b ( 1 2 3 4 )\n:\nend", manyNames()).(*syntax.ForClause)
	if !ok {
		t.Fatal("not a for")
	}
	if got := strings.Join(c.Names, ","); got != "a,b" {
		t.Errorf("Names = %q, want a,b", got)
	}
}

// Printed back with every name, which is the only spelling that says what the
// loop does. It reads back as the same names.
func TestALoopWithSeveralNamesPrintsThemAll(t *testing.T) {
	c := onlyCommand(t, "for a b ( 1 2 3 4 ) { echo $a }", manyNames())
	got := syntax.PrintCommand(c)
	if !strings.HasPrefix(got, "for a b ") {
		t.Fatalf("printed as %q, want it to begin `for a b `", got)
	}
	if names := forNames(t, got, manyNames()); strings.Join(names, ",") != "a,b" {
		t.Errorf("printed source read back with names %v", names)
	}
}

func mustParseHere(t *testing.T, src string, d syntax.Dialect, why string) {
	t.Helper()
	if _, err := syntax.Parse(src, d); err != nil {
		t.Errorf("%s: %q: %v", why, src, err)
	}
}

func mustFailHere(t *testing.T, src string, d syntax.Dialect, why string) {
	t.Helper()
	if _, err := syntax.Parse(src, d); err == nil {
		t.Errorf("%s: %q parsed, want a syntax error", why, src)
	}
}
