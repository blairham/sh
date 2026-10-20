// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

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
		// The declaring call unsetting its own private takes the entry with
		// it, which is the held binding being read rather than remembered.
		{
			"the declarer unsetting it takes the entry too",
			mod + "inner(){ " + showV + " }\n(){ private -x v=q; unset v; inner }", "none",
		},
		// Two frames in **with an outer exported name underneath**, which is
		// the row that says the *outermost* seal is the one read: the second
		// seal holds what the first installed, and that is the outer name.
		// With one body between the child and the declaration the two
		// readings agree, so this needs the depth and the outer name at once.
		{
			"two frames in, over an outer exported name",
			mod + "typeset -x v=outer\ndeep(){ " + showV + " }\ninner(){ deep }\n" +
				"(){ private -x v=q; inner }", "v=q",
		},
		// And exactly one entry, which is what says the passes over the
		// runner's tables step past the name rather than writing a second.
		{
			"and it is written once",
			mod + "typeset -x v=outer\n" +
				`inner(){ print -r -- "n=$(/usr/bin/env | /usr/bin/grep -c "^v=")" }` + "\n" +
				"(){ private -x v=q; inner }", "n=1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}

// A name the shell **inherited** rather than assigned is answered by the
// first pass over the environment it was handed, which the second pass never
// sees — so the private has to step past that one too.
//
// The runner is given the name in its environment rather than assigning it,
// because an assignment moves it into the shell's own table and the row would
// then be the one above wearing a different spelling.
func TestAPrivateExportBeatsAnInheritedName(t *testing.T) {
	const src = "zmodload zsh/param/private\n" +
		"inner(){ " + showV + " }\n(){ private -x v=q; inner }\n"
	dir := t.TempDir()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir}, Env: []string{"v=inherited"},
	}, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "v=q\n" || st != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, st, "v=q\n")
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
