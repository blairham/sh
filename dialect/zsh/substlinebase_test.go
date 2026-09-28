// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `=(…)` body counts its lines from where it was written, the way the other
// substitutions already did (#5085).
//
// It counted from one instead: the body is parsed on its own, and the offset
// that says how far into the script that text begins was set at the two call
// sites that fork a pipe and not at the one that writes a temporary file.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
//
// The rows move the substitution down the file, because an offset that is
// simply absent and one that is off by a constant read the same at any single
// depth. `<(…)` and `$(…)` at the same depths are the controls, and both were
// right throughout — which is what says this is one spelling's offset rather
// than the parameter or the parse.
func TestATempFileSubstitutionsBodyCountsFromTheScript(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"on line 2",
			"\n" + `read v <=(print -r -- $LINENO)` + "\n" + `print -r -- "[$v]"`, "[2]",
		},
		{
			"and on line 4",
			"\n\n\n" + `read v <=(print -r -- $LINENO)` + "\n" + `print -r -- "[$v]"`, "[4]",
		},
		// The controls, at the same depth.
		{
			"where the pipe spelling already did",
			"\n" + `read v < <(print -r -- $LINENO)` + "\n" + `print -r -- "[$v]"`, "[2]",
		},
		{
			"and so did a command substitution",
			"\n" + `print -r -- "[$(print -r -- $LINENO)]"`, "[2]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}

// The offset is not `$LINENO` alone: **three readers carry it**, and one
// missing assignment moved all of them.
//
// A diagnostic raised in the body names the line, and a function *defined* in
// the body records it as the origin its own frame reports ever after. That
// last one is the half a fix aimed at `$LINENO` would have left behind — it
// was found by asking what travels with the offset rather than by reading the
// symptom.
func TestTheOffsetReachesADiagnosticAndADefinitionToo(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a command that is not there",
			"\n: =(nosuchcmd-xyz)\n", "zsh:2: command not found: nosuchcmd-xyz\n",
		},
		{
			"an expression that will not compute",
			"\n: =((( 1/0 )))\n", "zsh:2: division by zero\n",
		},
		{
			"and a function defined in the body",
			"\n" + `read v <=(f() { print -r -- ${funcsourcetrace[1]##*:} }; f)` + "\n" +
				`print -r -- "[$v]"` + "\n", "[2]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// And it is the offset **wherever the substitution was written**, which every
// row at the top level hides: there the script's numbering and the enclosing
// unit's are the same, so an offset taken from the wrong place still lands.
//
// A function body, a sourced file and a `=(…)` inside another one each move
// the two apart.
func TestTheOffsetIsTakenWhereTheSubstitutionStands(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"inside a function body",
			"f() {\n\n\n" + `read v <=(print -r -- $LINENO)` + "\n" +
				`print -r -- "[$v]"` + "\n}\nf\n", "[3]",
		},
		{
			"inside a sourced file",
			"print -l '' '' " + `'read v <=(print -r -- $LINENO)' 'print -r -- "[$v]"'` +
				" > lib.zsh\n. ./lib.zsh\n", "[3]",
		},
		{
			"and inside another one of the same",
			"\n" + `read v <=(read w <=(print -r -- $LINENO); print -r -- $w)` + "\n" +
				`print -r -- "[$v]"` + "\n", "[2]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}
