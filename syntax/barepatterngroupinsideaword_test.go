// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The three readings of one construct, as three dialects that differ in
// nothing else. See [Dialect.BarePatternGroupInsideAWord] for the rows they
// were taken from.

// groupsAnywhere opens a bare `(a|b)` wherever it stands.
func groupsAnywhere() Dialect {
	d := Core()
	d.DoubleBracket = true
	// A leading `(` reaches a word scan at all only where the grammar has
	// the qualifier suffix, which is the same dialect that has bare groups.
	// Without it the argument rows below would be refused by all three
	// fixtures and would say nothing about the flag.
	d.GlobQualifiers = true
	d.PatternAlternation = true
	return d
}

// groupsNowhere is the same grammar with the construct gone.
func groupsNowhere() Dialect {
	d := groupsAnywhere()
	d.PatternAlternation = false
	return d
}

// groupsInsideAWord is the narrowed reading, and it differs from the one
// above in exactly one field — which is what makes every row below a
// statement about that field.
func groupsInsideAWord() Dialect {
	d := groupsNowhere()
	d.BarePatternGroupInsideAWord = true
	return d
}

func TestABareGroupOpensWhereTheFlagSaysAWordHasBegun(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		// want is refused-or-not under each of the three, in the order
		// anywhere, nowhere, inside-a-word.
		want [3]bool
	}{
		{"a group where a word begins", "[[ a == (a|b) ]]\n", [3]bool{false, true, true}},
		{"a group inside a word", "[[ ab == a(b|c) ]]\n", [3]bool{false, true, false}},
		{"a group where an argument begins", "echo (a|b)\n", [3]bool{false, true, true}},
		{"a group inside an argument", "echo a(b|c)\n", [3]bool{false, true, false}},
		// The controls. Neither has a bare parenthesis in it at all, so a
		// flag that had broken the word rules rather than narrowed them
		// would show here and in none of the rows above.
		{"no group at all", "[[ ab == a*c ]]\n", [3]bool{false, false, false}},
		{"a quoted parenthesis", "echo 'a(b|c)'\n", [3]bool{false, false, false}},
		{"a subshell where a command begins", "(echo hi)\n", [3]bool{false, false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i, d := range []Dialect{groupsAnywhere(), groupsNowhere(), groupsInsideAWord()} {
				_, err := Parse(tc.src, d)
				if got := err != nil; got != tc.want[i] {
					t.Errorf("dialect %d: refused=%v (%v), want %v", i, got, err, tc.want[i])
				}
			}
		})
	}
}

// How much text the group takes is [Dialect.UnterminatedPatternGroupIsAWord]'s
// rule and not a second one, which is the half a narrowed reading could have
// got wrong by scanning differently: the two dialects that open a group at all
// have to produce the same word.
func TestTheNarrowedGroupIsLexedLikeTheWiderOne(t *testing.T) {
	const src = "print -r -- a(b ; print x\n"
	for _, d := range []Dialect{groupsAnywhere(), groupsInsideAWord()} {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if len(f.Stmts) != 2 {
			t.Fatalf("%d statements, want 2 — the `;` is what ends the unterminated group's word",
				len(f.Stmts))
		}
	}

	// And the control: with no group to open, the parenthesis is the shell's
	// own and the line does not parse at all.
	if _, err := Parse(src, groupsNowhere()); err == nil {
		t.Error("the line parsed with bare groups gone, so the rows above say nothing about the group")
	}
}

// A group the *narrowed* reading opened is a bare one and not a quantified
// one, which decides how much text it takes: a quantified group holds a `;`
// and a bare one ends the word at it.
//
// This row exists because the reading was written twice and the second copy
// was wrong. `scanGroupSpans` asked the complement — "no bare group opens
// here, so a quantifier must have opened this one" — and the narrowed reading
// made that false wherever a `@` stood in front of the parenthesis, so
// `case "x;y" in x@([;])y)` parsed where the reference refuses it under every
// mode. The whole-corpus figure *fell* on the change that introduced it.
func TestANarrowedGroupIsBareAndNotQuantified(t *testing.T) {
	const src = "case \"x;y\" in x@([;])y) echo semi;; *) echo no;; esac\n"
	for i, d := range []Dialect{groupsAnywhere(), groupsNowhere(), groupsInsideAWord()} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("dialect %d parsed the line, so the `;` was taken as the group's text", i)
		}
	}

	// And the control on the other side: where the dialect really does have
	// quantified groups, the same line parses. Without it this test would
	// pass against a grammar that had lost them altogether.
	d := groupsNowhere()
	d.ExtendedPattern = true
	if _, err := Parse(src, d); err != nil {
		t.Errorf("a dialect with quantified groups refused the line: %v", err)
	}
}
