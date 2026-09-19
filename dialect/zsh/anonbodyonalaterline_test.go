// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A nameless function's body may stand on a later line here, and the words
// after it are the call's positional parameters exactly as they are when the
// two are written together (#3778).
//
// Measured 2026-09-19 on zsh 5.9.2 with `-f`, each row a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null
// device. This engine refused every one of them at a parse error, because the
// keyword standing alone is already a whole command here — so the `{ … }`
// under it read as an ordinary group and the words after that group had
// nowhere to go.
func TestANamelessFunctionsBodyMayStandOnALaterLineHere(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a newline between the two", "function\n" + `{ printf "n=%d" $#; } a b`, "n=2"},
		{"a blank line as well", "function\n\n" + `{ printf "n=%d" $#; } a b`, "n=2"},
		{"a comment line", "function\n# why\n" + `{ printf "n=%d" $#; } a b`, "n=2"},
		{"a semicolon", `function; { printf "n=%d" $#; } a b`, "n=2"},
		{"the other header over a semicolon", `() ; { printf "n=%d" $#; } a b`, "n=2"},
		{"a subshell as the body", "function\n" + `( printf "n=%d" $# ) a b`, "n=2"},
		{"inside a construct", "if true; then function\n" + `{ printf "n=%d" $#; } a b; fi`, "n=2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// And the row that looks as though it already agreed.
//
// With no arguments the two readings print the same bytes, which is why the
// issue that filed this had them down as controls — a bare keyword followed
// by a group runs the group either way. They are not the same program. A
// function body has a scope of its own, so a name declared in it is gone
// afterwards, where a group leaves it behind; and `$#` inside the body is the
// call's and not the script's.
//
// Measured the same day: the reference prints `in=2 out=` and `n=0` for these
// two, and this engine printed `out=2` and the script's own count. The script
// is given positional parameters of its own so that the count row can tell an
// empty call from an inherited one — with none, both readings answer 0.
func TestTheNoArgumentSpellingIsACallAndNotAGroup(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a name declared in the body does not outlive it",
			"function\n{ typeset y=2; printf 'in=%s ' $y; }\n" + `printf "out=%s" $y`,
			"in=2 out=",
		},
		{
			"and the body's positional parameters are the call's",
			"set -- p q\nfunction\n" + `{ printf "n=%d" $#; }`,
			"n=0",
		},
		// The discriminator against both: written with no keyword at all the
		// group really is a group, and both answers come back.
		{
			"where a plain group leaves the name behind",
			"{ typeset y=2; printf 'in=%s ' $y; }\n" + `printf "out=%s" $y`,
			"in=2 out=2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
