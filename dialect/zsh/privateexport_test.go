// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// showV is a child command that prints the environment entry for `v`, or a
// word when there is none. An absolute path, so the rows need nothing on
// PATH.
const showV = `/usr/bin/env | /usr/bin/grep "^v=" || print -r -- none`

// A `private -x` name is in the environment a **callee's** children inherit,
// for as long as the declaring call is on the stack (#5091).
//
// It was not. The hiding and the export were one mechanism here and are two
// rules in the shell being modeled: the seal takes the private binding out of
// the runner's tables for the length of a callee's frame, which is how the
// name becomes invisible to the callee — and the environment is built from
// those same tables, so taking it out took it out of both.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device, each side
// starting its own binary where a child shell is wanted.
func TestAPrivateExportReachesAChildStartedByACallee(t *testing.T) {
	const mod = "zmodload zsh/param/private\n"
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a child in the declaring call",
			mod + "(){ private -x v=q; " + showV + " }", "v=q",
		},
		{
			"and a child in a callee",
			mod + "inner(){ " + showV + " }\n(){ private -x v=q; inner }", "v=q",
		},
		{
			"two frames in",
			mod + "deep(){ " + showV + " }\ninner(){ deep }\n(){ private -x v=q; inner }", "v=q",
		},
		// The letter spelling of the same request.
		{
			"the letter spelling",
			mod + "inner(){ " + showV + " }\n(){ local -Px v=q; inner }", "v=q",
		},
		// The value is the one the name holds when the callee is entered,
		// not the one the declaration wrote.
		{
			"the value it holds at the call",
			mod + "inner(){ " + showV + " }\n(){ private -x v; v=set; inner }", "v=set",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}

// The other half of the same mechanism, and it was right: the **parameter**
// stays hidden from the callee.
//
// This is the row that says the fix reads the seal back rather than lifting
// it. A change that simply left the private in the tables would pass every
// row above and break this one.
func TestThePrivateNameStaysHiddenFromTheCallee(t *testing.T) {
	const mod = "zmodload zsh/param/private\n"
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"the callee cannot see the name",
			mod + `inner(){ print -r -- "seen=$+v" }` + "\n(){ private -x v=q; inner }", "seen=0",
		},
		{
			"nor read a value through it",
			mod + `inner(){ print -r -- "v=[$v]" }` + "\n(){ private -x v=q; inner }", "v=[]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}

// What the entry loses to, what it beats, and when it stops.
//
// Three rules, each measured, and each one a row a fix written to the
// headline alone would get wrong: it sits **between** an outer binding and
// the callee's own.
func TestWhatThePrivateExportBeatsAndLosesTo(t *testing.T) {
	const mod = "zmodload zsh/param/private\n"
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"it beats an outer exported name",
			mod + "typeset -x v=outer\ninner(){ " + showV + " }\n(){ private -x v=q; inner }", "v=q",
		},
		{
			"it loses to one the callee declared",
			mod + "inner(){ local -x v=own; " + showV + " }\n(){ private -x v=q; inner }", "v=own",
		},
		{
			"an unset in the callee does not remove it",
			mod + "inner(){ unset v; " + showV + " }\n(){ private -x v=q; inner }", "v=q",
		},
		{
			"a private with no export letter is not there at all",
			mod + "inner(){ " + showV + " }\n(){ private v=q; inner }", "none",
		},
		{
			"and it stops when the declaring call returns",
			mod + "(){ private -x v=q }\n" + showV, "none",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}

// And the ordinary environment, which this walks through and must not move.
//
// The entry is written by stepping the two passes over the runner's tables
// past the names it covers, so every row those passes already answered is a
// row this could have broken.
func TestTheOrdinaryEnvironmentIsUnmoved(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"an exported name", "typeset -x v=q\n" + showV, "v=q"},
		{"one that was never exported", "v=q\n" + showV, "none"},
		{"one the letter came off", "typeset -x v=q\ntypeset +x v\n" + showV, "none"},
		{"one that was unset", "typeset -x v=q\nunset v\n" + showV, "none"},
		{"an empty exported value", "typeset -x v=\n" + showV, "v="},
		{
			"a local export inside a function",
			"typeset -x v=outer\nf(){ local -x v=loc; " + showV + " }\nf", "v=loc",
		},
		{
			"and an exported name seen from a callee",
			"inner(){ " + showV + " }\ntypeset -x v=q\ninner", "v=q",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}
