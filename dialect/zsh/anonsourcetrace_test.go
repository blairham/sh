// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `${funcsourcetrace}` for a **nameless** function names the file it was
// written in, exactly as a named one's does (#5084).
//
// It named the shell instead. A nameless function has no name to record an
// origin under, so nothing recorded one, and the frame fell back to the
// stand-in [interp.Frame.NoFile] marks — which is the right answer for a
// function defined at the top level of `-c`, and the wrong one here.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
//
// **Every row sources a file**, and that is not decoration. The fallback
// answers the shell's own name, which in a script run by hand is plainly not
// the script — and in this harness is the *same string* the script is called
// by, so a row written at the top level passes whether the fault is there or
// not. I wrote three such rows first and the control mutant walked straight
// through all three. A sourced file has a name of its own that nothing else
// can coincide with, and it is the only shape here that can fail.
func TestANamelessFunctionsFrameNamesTheFileItWasWrittenIn(t *testing.T) {
	const read = `print -r -- "[${funcsourcetrace[1]}]"`
	dir := t.TempDir()
	for _, tc := range []struct{ name, call string }{
		{"an unbracketed body", `() ` + read},
		{"a bracketed one", `() { ` + read + ` }`},
		// The keyword spelling answers the same, which is worth a row of
		// its own: three issues running turned on which header was written,
		// and this is one of the places the two agree. See the note on
		// syntax.AnonFunc.
		{"and the keyword header", `function { ` + read + ` }`},
		// The named function in the same position, which answered the
		// library all along — so what the rows above grade is the nameless
		// call and not the sourcing.
		{"where a named one already did", `g() { ` + read + ` }` + "\n" + `g`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "print -l '" + tc.call + "' > lib.zsh\n. ./lib.zsh\n"
			if out, st := runZsh(t, dir, src); out != "[./lib.zsh:1]\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, "[./lib.zsh:1]\n")
			}
		})
	}
}

// The keyword standing entirely alone has a frame too, and it answers the same.
//
// It runs no body, so nothing inside it can be asked — except that a
// redirection written after it is a redirection with **no command**, and the
// null command runs in that frame. Naming a function as `NULLCMD` is what
// reaches in and reads the trace back.
//
// Without this the bare branch was the one place a nameless call still
// answered the shell's name, and no other row in this file could see it: the
// branch builds its own call and would have been left behind by a fix written
// only where the body is.
func TestTheBareKeywordsFrameNamesTheFileToo(t *testing.T) {
	const lib = `f() { print -r -- "[${funcsourcetrace[2]}]" }` + "\n" +
		"NULLCMD=f\n" + "function >out\n"
	const src = "print -l '" + lib + "' > lib.zsh\n. ./lib.zsh\n" +
		`print -r -- "got: $(<out)"` + "\n"
	dir := t.TempDir()
	if out, st := runZsh(t, dir, src); out != "got: [./lib.zsh:3]\n" || st != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, st, "got: [./lib.zsh:3]\n")
	}
}

// And the file is the one the **definition** was read from, not the one the
// call was made in.
//
// A sourced file names itself, from a script that names something else — the
// row that says this is an origin and not a look at where the shell is
// standing. Both entries are written out here because the names are the
// script's own and do not depend on the harness: the nameless call's frame
// reports the library, and the frame below it is the sourced file, which has
// no definition line and writes a literal nought.
func TestTheFileIsWhereTheBodyWasReadAndNotWhereItWasCalled(t *testing.T) {
	const read = `print -r -- "[${funcsourcetrace}]"`
	dir := t.TempDir()
	for _, tc := range []struct{ name, body string }{
		{"an unbracketed body", `() ` + read},
		{"a bracketed one", `() { ` + read + ` }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "print '" + tc.body + "' > lib.zsh\n\n\n. ./lib.zsh\n"
			want := "[./lib.zsh:1 ./lib.zsh:0]\n"
			if out, st := runZsh(t, dir, src); out != want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}

// The **line** comes from the text the body was read in, which is the other
// half of the same record.
//
// Borrowed text is where the two part: a nameless call written inside a
// command substitution is on the script's line 2, and the definition line has
// to carry the offset that text was running at or it answers 1 — its own line
// within the substitution. Nothing else in this file would notice, every other
// row being written where the offset is nought.
func TestTheDefinitionLineCarriesTheOffsetOfTheTextItWasReadIn(t *testing.T) {
	const line = "${funcsourcetrace[1]##*:}"
	dir := t.TempDir()
	for _, tc := range []struct{ name, call string }{
		{"an unbracketed body", `() print -r -- ` + line},
		{"a bracketed one", `() { print -r -- ` + line + ` }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "\nprint -r -- \"[$(" + tc.call + ")]\"\n"
			if out, st := runZsh(t, dir, src); out != "[2]\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, "[2]\n")
			}
		})
	}
}

// `${funcfiletrace}` is the control, and it was right all along.
//
// It reports where each frame was *entered from* rather than where its unit
// was written, so it never asked the function table and never saw the fault.
// The pair is what says the two parameters answer different questions off the
// same frames — and a fix that moved the wrong one would have broken this
// while making the rows above pass.
func TestTheCallSiteTraceWasAlreadyRightAndStaysRight(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"an unbracketed body",
			"\n" + `() print -r -- "n=${#funcfiletrace}"`, "n=1",
		},
		{
			"and the line it was entered at",
			"\n\n" + `() print -r -- "${funcfiletrace[1]##*:}"`, "3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}
