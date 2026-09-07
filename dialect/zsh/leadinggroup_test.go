// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A parenthesised group at the front of a loop item and of a `case` arm's
// pattern, run rather than only parsed. Measured 2026-09-06 on zsh 5.9.2,
// `env -i PATH=/usr/bin:/bin` with a scratch HOME, ZDOTDIR and HISTFILE, over
// a script file (#1161).

// The arm's leading paren and the pattern's group are the same character, and
// the reading has to be the one that *matches*: `((#i)*.zip)` is the arm's
// paren in front of a case-insensitive pattern, which is what
// `~/.zi/bin/lib/zsh/install.zsh:1580` is written as.
func TestACaseArmsPatternMayBeginWithAGroup(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The flag itself is read and not implemented — it is the `(#q…)`
		// family of #1053 — so the case-insensitive rows answer `miss` here
		// where that shell answers `hit`. That is the honest half and it is
		// what this issue was about: the arm *parses*, and the flag's
		// meaning is a separate open item rather than something this change
		// pretends to have. A row asserting `hit` would have been asserting
		// a feature nothing here builds — and the flag is not a no-op
		// either: `(#i)` reaches the matcher as five literal characters, so
		// even `f.zip` misses `((#i)*.zip)`. What each row *does* show is
		// that the arm was read and dispatched, since the `*)` arm ran.
		{`setopt extendedglob; case F.ZIP in ((#i)*.zip) echo hit;; *) echo miss;; esac`, "miss"},
		{`setopt extendedglob; case A in (#i)a) echo hit;; *) echo miss;; esac`, "miss"},
		{`setopt extendedglob; case A in ((#i)a) echo hit;; *) echo miss;; esac`, "miss"},
		// No `#` anywhere: the arm's paren in front of an alternation group,
		// and that group *is* implemented, so this row matches.
		{`case b in ((a|b)) echo hit;; *) echo miss;; esac`, "hit"},
		{`case a in ((a)) echo hit;; *) echo miss;; esac`, "hit"},
		// And the group really is a group: it does not match its own text.
		{`case "(a)" in ((a)) echo hit;; *) echo miss;; esac`, "miss"},
		// The arm's own paren in front of a plain pattern, unchanged.
		{`case a in (a) echo hit;; *) echo miss;; esac`, "hit"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// The arithmetic-command suspension belongs to the arm and not to the
// construct: an arm's *body* is ordinary commands, and an expression in one
// really is an expression.
func TestAnArmsBodyStillTakesAnArithmeticCommand(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `case x in x) ((1+1)) && echo ran;; esac`)
	if strings.TrimSpace(out) != "ran" || st != 0 {
		t.Errorf("an arithmetic command in an arm's body = %q (status %d), want ran",
			out, st)
	}
}

// A loop item may begin with a group too, and the item is one word.
func TestALoopItemMayBeginWithAGroup(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt extendedglob
for x in '(#i)a' b; do echo "got=[$x]"; done`)
	want := "got=[(#i)a]\ngot=[b]\n"
	if out != want || st != 0 {
		t.Errorf("a quoted flag as an item = %q (status %d), want %q", out, st, want)
	}
	// Unquoted, the flag is a pattern that reaches the filesystem — the row
	// is that the *item* is read at all, which is what refused before.
	out, _ = runZsh(t, t.TempDir(), `for x in (#i)zz; do :; done`)
	if strings.Contains(out, "parse error") {
		t.Errorf("for x in (#i)zz = %q, want it parsed", out)
	}
}

// Argument position ends with the list, and this can only be seen by
// **running** it.
//
// The flag is set before the list's first word is read and restored after,
// and the token after the list is read while it is still on — so a `(` there
// would be folded into a word. Every one of these *parses* either way: the
// folded group is a perfectly good word, and `-n` cannot tell them apart. It
// is the run that shows it — a mutant that left the flag on answers
// `unknown file attribute:` where the loop should print `hi`, and every
// parse-only row passed it.
func TestArgumentPositionEndsWithTheList(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"in list, subshell body", `for x in a; do ( echo hi ); done`},
		{"in list, newline before do", "for x in a\ndo ( echo hi ); done"},
		{"parenthesised list", `for x (a); do ( echo hi ); done`},
		// The menu loop needs a choice on its input or the body never runs;
		// `1` picks the only item.
		{"select's list", "select x in a; do ( echo hi ); break; done <<< 1"},
		{"case arm's pattern", `case a in (a) ( echo hi );; esac`},
		{"case arm, group pattern", `case b in ((a|b)) ( echo hi );; esac`},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if !strings.Contains(out, "hi") || strings.Contains(out, "file attribute") {
			t.Errorf("%s: %s = %q (status %d), want the subshell to have run — a "+
				"leaked argument position folds the `(` into a word instead",
				tc.name, tc.src, out, st)
		}
	}
}

// No word in the item list is a reserved word — unanimous across the panel,
// so this is core seen through one dialect rather than a dialect's answer.
func TestNoWordInTheItemListIsReserved(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `for x in do; do echo "1=$x"; done
for x in done; do echo "2=$x"; done
for x (a done); do echo "3=$x"; done`)
	want := "1=do\n2=done\n3=a\n3=done\n"
	if out != want || st != 0 {
		t.Errorf("reserved words as items = %q (status %d), want %q", out, st, want)
	}
	// And the other direction: with no separator the `do` is an item, so
	// `done` stands where `do` belongs and the line is refused. Asked of the
	// parser rather than of a run, because the run helper treats a refused
	// program as a harness failure.
	if _, err := parseZsh(`for x in a b do echo "$x"; done`); err == nil {
		t.Error("`for x in a b do …` parsed; every shell in the panel refuses it")
	} else if !strings.Contains(err.Error(), `"done" unexpected`) {
		t.Errorf("`for x in a b do …` = %v, want the refusal to name `done`", err)
	}
}

// A `case` arm's leading `(` read as the *pattern's*, run rather than only
// parsed — and behavioral because parsing is not the question here. A group
// and a two-element pattern list match identically, so a reading that picked
// the wrong one would still parse and still answer `hit` on most subjects;
// the rows that separate them are the ones where the group carries text of
// its own, and the `miss` subjects beside each one (#1218).
func TestACaseArmsLeadingParenMayBeThePatternsOwn(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The shape the issue was filed on.
		{`case ab in (ab|cd)) echo hit;; *) echo miss;; esac`, "hit"},
		{`case xy in (ab|cd)) echo hit;; *) echo miss;; esac`, "miss"},
		// The group carrying pattern text: `(a)b` matches `ab` and not `a`,
		// which a two-element list `a`,`b` would have the other way round.
		// This is the row that says the group reading is real.
		{`case ab in (a)b) echo hit;; *) echo miss;; esac`, "hit"},
		{`case a in (a)b) echo hit;; *) echo miss;; esac`, "miss"},
		{`case b in (a)b) echo hit;; *) echo miss;; esac`, "miss"},
		{`case ax in (a|b)x) echo hit;; *) echo miss;; esac`, "hit"},
		{`case a in (a|b)x) echo hit;; *) echo miss;; esac`, "miss"},
		{`case ab in (a)(b)) echo hit;; *) echo miss;; esac`, "hit"},
		{`case ab in (a)*) echo hit;; *) echo miss;; esac`, "hit"},
		{`case b in (a)*) echo hit;; *) echo miss;; esac`, "miss"},
		// A list of groups, and a subject for each alternative plus one
		// outside both.
		{`case a in (a|b)|(c|d)) echo hit;; *) echo miss;; esac`, "hit"},
		{`case c in (a|b)|(c|d)) echo hit;; *) echo miss;; esac`, "hit"},
		{`case x in (a|b)|(c|d)) echo hit;; *) echo miss;; esac`, "miss"},
		{`case a in (a|b) | (c|d)) echo hit;; *) echo miss;; esac`, "hit"},
		// An alternative written as nothing *after* the group. Distinct
		// from the empty alternative *inside* one, which already worked:
		// this needs the `|` to settle the reading on its own, and while
		// it required a word after it the line was refused. Found by
		// mutation, not by a case anyone wrote.
		{`case a in (a|b)|) echo hit;; *) echo miss;; esac`, "hit"},
		{`case "" in (a|b)|) echo hit;; *) echo miss;; esac`, "hit"},
		{`case x in (a|b)|) echo hit;; *) echo miss;; esac`, "miss"},
		{`case c in (a|b)|c) echo hit;; *) echo miss;; esac`, "hit"},
		{`case d in (a|b)|c|d) echo hit;; *) echo miss;; esac`, "hit"},
		// Nested, so three `)` in a row.
		{`case a in ((a|b))) echo hit;; *) echo miss;; esac`, "hit"},
		{`case ab in ((a|b))) echo hit;; *) echo miss;; esac`, "miss"},
		// Blanks before the arm's `)`.
		{`case a in (a|b) ) echo hit;; *) echo miss;; esac`, "hit"},
		{`case ab in (a|b) ) echo hit;; *) echo miss;; esac`, "miss"},
		{`case ab in (a)b ) echo hit;; *) echo miss;; esac`, "hit"},
		// An empty alternative in the group, this dialect's own.
		{`case a in (|a)) echo hit;; *) echo miss;; esac`, "hit"},
		{`case "" in (|a)) echo hit;; *) echo miss;; esac`, "hit"},
		// The arm's own paren, which is the reading a rule that always
		// chose the group would have broken. Kept beside the rest so the
		// boundary is one table rather than two.
		{`case a in (a|b) echo hit;; *) echo miss;; esac`, "hit"},
		{`case ab in (a|b) echo hit;; *) echo miss;; esac`, "miss"},
		{`case a in ((a|b)) echo hit;; *) echo miss;; esac`, "hit"},
		{`case a in (a) echo hit;; *) echo miss;; esac`, "hit"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}

// And the shapes neither reading can take, refused here as in zsh 5.9.2.
// `(a) b)` is the discriminating one: the blank means the group cannot go on
// being a word, so only the arm reading is left, and it then finds `b` where
// the arm's `)` belongs.
func TestACaseArmNeedsAParenLeftToCloseIt(t *testing.T) {
	for _, src := range []string{
		`case ab in (a) b) echo hit;; *) echo miss;; esac`,
		`case ab in (a|b) c) echo hit;; *) echo miss;; esac`,
		`case a in (a|b)|c echo hit;; *) echo miss;; esac`,
		`case a in (a|b)|c d) echo hit;; *) echo miss;; esac`,
	} {
		if !parseFails(t, src) {
			t.Errorf("%s: parsed, where no parenthesis is left to close the arm", src)
		}
	}
}

// A glob flag that is not followed by a closing `)` of its own has to be read
// as the *pattern's* paren all the same, and the `#` is the whole of what
// says so: `#` where a word may begin opens a comment, so the arm reading
// swallows the rest of the line and the refusal ends up blaming the
// parenthesis the script legitimately wrote.
//
// Asserted as the whole rendered line, position included, because the
// position is the entire content of the claim — both readings refuse this
// shape and they differ in nothing else. Dropping the `#` clause moves the
// blame to “ `(' “ and is caught here and nowhere else.
//
// The refusal itself is not this dialect's final answer: zsh 5.9.2 reads the
// pattern and complains about it at match time, `bad pattern: #i*`. That gap
// is #1244, and a row asserting it would be asserting a feature nothing here
// builds.
func TestAGlobFlagsRefusalNamesTheTokenAfterThePattern(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`case abc in (#i*) echo Y;; esac`, "parse error near `echo'"},
		{`case x in (#q) echo Y;; esac`, "parse error near `echo'"},
		// A `|` commits to the group reading, and the position is the whole
		// of what that buys: each of these is refused at the token after
		// the pattern list, exactly as zsh 5.9.2 refuses it, where falling
		// back to the arm's paren blamed the `|` instead.
		{`case a in (a|b)|c echo Y;; esac`, "parse error near `echo'"},
		{`case a in (a|b)| echo Y;; esac`, "parse error near `Y'"},
		{`case a in (a|b) | echo Y;; esac`, "parse error near `Y'"},
		{`case a in (a|b)|(c) echo Y;; esac`, "parse error near `echo'"},
		{`case a in (a|b)|c d) echo Y;; esac`, "parse error near `d'"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%s: parsed, want a refusal", tc.src)
			continue
		}
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}
