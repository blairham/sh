// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
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
