// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"
)

// The three letters left after the two kinds landed, and the plus words
// beside them (#2097). Measured against zsh 5.9.2 on 2026-09-12, with
//
//	alias -g UP='| tr a-z A-Z'; alias cmd='echo hi'; alias -s txt=cat
//
// Every listing is an exact-byte assertion, because a listing is a measured
// artifact rather than a formatting choice.

// **`-L` writes each line as a command that would define the alias back**,
// which the plain listing cannot be: it is `name=value` in this dialect and
// says nothing about which kind an entry is.
//
// The kind letter comes from the *entry* and not from the letter that was
// asked for, which is what makes a plain `alias -L` tell its global rows from
// its regular ones.
func TestListingAsDefinitions(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"both kinds at once",
			`alias -g G='b b'; alias r=y; alias -L`,
			"alias -g G='b b'\nalias r='y'\n",
		},
		{"the global kind alone", `alias -g G='b b'; alias r=y; alias -g -L`, "alias -g G='b b'\n"},
		{"the suffix kind alone", `alias -s t=z; alias -s -L`, "alias -s t='z'\n"},
		{"a named operand", `alias r=y; alias -L r`, "alias r='y'\n"},
		{"the regular kind alone", `alias -g G=x; alias r=y; alias -rL`, "alias r='y'\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := kindsRun(t, c.src); out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// **`-r` is the kind letter for "neither global nor suffix"**, and it is a
// kind rather than a modifier: two kind letters in one call is the same
// refusal `-g` and `-s` already earned.
func TestTheRegularKindIsAKind(t *testing.T) {
	out, st := kindsRun(t, `alias -g G=x; alias r=y; alias -s t=z; alias -r`)
	if out != "r='y'\n" || st != 0 {
		t.Errorf("alias -r = %q (status %d), want %q at 0", out, st, "r='y'\n")
	}
	// Found by the table and filtered by the letter, the same rule `-g` has:
	// a global named under `-r` is success and silence.
	out, st = kindsRun(t, `alias -g G=x; alias r=y; alias -r G r`)
	if out != "r='y'\n" || st != 0 {
		t.Errorf("alias -r G r = %q (status %d), want %q at 0", out, st, "r='y'\n")
	}
	for _, src := range []string{`alias -r -g`, `alias -rs`, `alias -gs`} {
		out, st := kindsRun(t, src)
		if want := "testsh: alias: illegal combination of options\n"; out != want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", src, out, st, want)
		}
	}
}

// **`-m` reads every operand as a pattern**, and the two builtins differ on
// what an *absent* pattern means — which is the row that says why one axis
// serving both is still one question about the letter and not about the
// builtin.
func TestOperandsAsPatterns(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"a pattern picks names out", `alias -g UP=x; alias cmd=y; alias -m 'U*'`, "UP='x'\n", 0},
		{"restricted by the kind letter too", `alias -g UP=x; alias cmd=y; alias -g -m '*'`, "UP='x'\n", 0},
		{"a pattern matching nothing is not a failure", `alias cmd=y; alias -m 'z*'`, "", 0},
		// The discriminating pair: `alias -m` with nothing after it lists,
		// and `unalias -m` with nothing after it refuses. A removal with no
		// pattern would be a removal of everything, which is `-a`.
		{"no pattern lists", `alias cmd=y; alias -m`, "cmd='y'\n", 0},
		{"no pattern refuses to remove", `alias cmd=y; unalias -m`, "testsh: unalias: not enough arguments\n", 1},
		{"a pattern removes", `alias a=1; alias b=2; unalias -m 'a*'; alias`, "b='2'\n", 0},
		{"removing nothing is 1", `alias a=1; unalias -m 'z*'; echo "st=$?"`, "st=1\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := kindsRun(t, c.src); out != c.want || st != c.status {
				t.Errorf("%s = %q (status %d), want %q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}

// **A `+` word prints the names without the values**, and a bare `+` does
// that *and ends the option list*.
//
// The two halves need separate rows because either alone would pass a test
// that had only the other: `alias +` and `alias -L +` agree under a
// "just names" reading and under an "ends the options" reading only if both
// are true, and `alias + -L` is what tells them apart — the `-L` is an
// operand there, so nothing is listed and the status is 1.
func TestThePlusWordsPrintNamesOnly(t *testing.T) {
	const defs = `alias -g UP=x; alias cmd=y; alias -s t=z; `
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"a kind letter", defs + `alias +g`, "UP\n", 0},
		{"the regular kind", defs + `alias +r`, "cmd\n", 0},
		{"the suffix kind", defs + `alias +s`, "t\n", 0},
		{"a bare plus is every name", defs + `alias +`, "UP\ncmd\n", 0},
		// Discriminating: the bare plus ended the options, so `-L` is a name
		// the table does not hold.
		{
			"a bare plus ends the options",
			defs + `alias + -L; echo "st=$?"`,
			"testsh: alias: -L: not found\nst=1\n", 0,
		},
		// And the words before it were still options.
		{"the options before it still count", defs + `alias -L +`, "alias -g UP='x'\nalias cmd='y'\n", 0},
		// `-L` wins over the names-only reading, measured.
		{"a definition line beats names only", defs + `alias -L +g`, "alias -g UP='x'\n", 0},
		// The refusal is the dialect's ordinary bad-option one, reached
		// through the same helper the `-` words use — so a letter is either
		// in the accepted set or refused, whichever sign was written.
		{"an unknown plus letter is refused", defs + `alias +q`, "testsh: alias: +q: invalid option\n", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := kindsRun(t, c.src); out != c.want || st != c.status {
				t.Errorf("%s = %q (status %d), want %q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}
