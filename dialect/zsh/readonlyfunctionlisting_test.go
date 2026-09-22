// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// zsh has no readonly *function* attribute, so `readonly -f` is `typeset -fr`
// under another name and the `r` is inert on the `f` line — what is left is
// an ordinary function listing. Measured 2026-09-22 on zsh 5.9.2 at
// /opt/homebrew/bin/zsh under LC_ALL=C, from a script file:
//
//	b(){ :; }; c(){ :; }
//	readonly -f b       ->  b's body, 0
//	readonly -pf b      ->  the same bytes — `-p` changes nothing here
//	readonly -f         ->  every function's body, 0
//	readonly -f nosuch  ->  nothing, 1
//	readonly -f b nosuch -> b's body, then 1 — the missing name does not
//	                        stop the rest of the listing
//	readonly -f b c     ->  both bodies, 0
//
// Before this, `readonly -f` reached nothing here: the `f` branch in
// biReadonly only fired where Semantics.FunctionAttributeLetters is
// non-empty, which is bash alone, so a dialect with the letter in
// ReadonlyOptions but no function attribute fell through to the ordinary
// variable path and either froze a variable of that name (with an operand)
// or listed nothing at all (bare), always at status 0 — #4215.
func TestReadonlyFIsAPlainFunctionListingHere(t *testing.T) {
	setup := "b(){ :; }\nc(){ :; }\n"
	body := func(n string) string { return n + " () {\n\t:\n}\n" }
	for _, tc := range []struct {
		name, line, want string
		status           int
	}{
		{"named", "readonly -f b", body("b"), 0},
		{"named with -p", "readonly -pf b", body("b"), 0},
		{"bare", "readonly -f", body("b") + body("c"), 0},
		{"bare with -p", "readonly -pf", body("b") + body("c"), 0},
		{"missing name", "readonly -f nosuch", "", 1},
		{"missing name with -p", "readonly -pf nosuch", "", 1},
		{"one of each", "readonly -f b c", body("b") + body("c"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), setup+tc.line+"\n")
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q (status %d), want %q (status %d)",
					tc.line, out, st, tc.want, tc.status)
			}
		})
	}
}

// A name that is not there beside one that is: the found name's body is
// still written and the listing does not stop there, but the status
// reflects the miss. Measured the same way as the table above.
func TestReadonlyFOnAMissingNameBesideAFoundOne(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "b(){ :; }\nreadonly -f b nosuch\n")
	want := "b () {\n\t:\n}\n"
	if !strings.HasPrefix(out, want) {
		t.Errorf("readonly -f b nosuch = %q, want it to start with %q", out, want)
	}
	if st != 1 {
		t.Errorf("readonly -f b nosuch left %d behind, want 1", st)
	}
}

// `typeset -fr` already carried this dialect's whole story about a frozen
// function — nothing about it changes here, and `readonly -f` reaching the
// same listing is the whole of the fix. A control for the row above rather
// than new coverage.
func TestTypesetFrIsUnaffectedByReadonlyFsFix(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "b(){ :; }\ntypeset -fr b\n")
	want := "b () {\n\t:\n}\n"
	if out != want || st != 0 {
		t.Errorf("typeset -fr b = %q (status %d), want %q (status 0)", out, st, want)
	}
}
