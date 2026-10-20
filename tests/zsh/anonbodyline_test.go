// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A nameless function whose body was written **without brackets** is not
// renumbered: its lines stay the caller's (#5080).
//
// This engine numbered every anonymous call's body from the construct, so a
// non-bracketed body reported 0 where the reference reports the line it
// stands on. Three readers give the same number and all three moved together:
// `$LINENO`, the `%i` of a trace prefix, and the line a diagnostic names.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
//
// Every row carries its bracketed twin, because 0 is not wrong in general —
// it is what the reference writes for a brace body, byte for byte. The two
// spellings differ and only one of them was modeled.
func TestANonBracketedBodyKeepsTheCallersLineNumbering(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a simple command reports the line it stands on",
			"\n\n" + `() print -r -- "L=$LINENO"`, "L=3",
		},
		{
			"where the brace spelling reports nought",
			"\n\n" + `() { print -r -- "L=$LINENO" }`, "L=0",
		},
		{
			"and so does the subshell spelling",
			"\n\n" + `() ( print -r -- "L=$LINENO" )`, "L=0",
		},
		// A body on a later line answers the **header's** line and not its
		// own, which is the row that says the number is the construct's
		// position rather than the body's.
		{
			"a body on a later line answers the header's",
			"\n\n()\n\n" + `print -r -- "L=$LINENO"`, "L=3",
		},
		// The other two commands with no statement list of their own.
		{
			"a test clause is one of the same",
			"\n\n" + "() [[ -n \"$LINENO\" ]]\n" + `print -r -- "st=$?"`, "st=0",
		},
		{
			"and an assignment",
			"\n\n" + `() y=$LINENO` + "\n" + `print -r -- "[$y]"`, "[3]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}

// The same number reaches the trace prefix and a diagnostic, which is what
// says it is one fact and not three.
//
// `PS4` is set to `%i` alone so the trace rows grade the number and nothing
// around it.
func TestTheBodysLineReachesTheTraceAndTheDiagnostic(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			"the trace prefix, unbracketed",
			"PS4='@%i '\nsetopt xtrace\n\n() print A\n",
			"@4 '(anon)'\n@4 print A\nA\n", 0,
		},
		{
			"and bracketed",
			"PS4='@%i '\nsetopt xtrace\n\n() { print A }\n",
			"@4 '(anon)'\n@0 print A\nA\n", 0,
		},
		{
			"a command that is not there, unbracketed",
			"\n\n() nosuchcmd-xyz\n",
			"(anon):3: command not found: nosuchcmd-xyz\n", 127,
		},
		{
			"and bracketed",
			"\n\n() { nosuchcmd-xyz }\n",
			"(anon): command not found: nosuchcmd-xyz\n", 127,
		},
		{
			"an expression that will not compute, unbracketed",
			"\n\n() (( 1/0 ))\n", "(anon):3: division by zero\n", 2,
		},
		{
			"and bracketed",
			"\n\n() { (( 1/0 )) }\n", "(anon): division by zero\n", 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != tc.status {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// The **definition's own line does not move with the numbering**, and this is
// the row that says the two are separate facts rather than one written twice.
//
// `$funcsourcetrace` reports where the function was defined, which is the
// construct's line for every spelling: 3 for the unbracketed body, 3 for the
// bracketed one, 3 for a body written two lines below the header, and 3 for a
// call made from inside another function. A change that moved the frame's
// line along with the body's numbering would answer 0 for the first of those
// and pass every other row in this file.
//
// Only the line is read here. The file half of that trace names the shell
// where the reference names the script, for both spellings alike, and that is
// a fault of its own rather than anything this measures.
func TestTheDefinitionsOwnLineDoesNotMoveWithIt(t *testing.T) {
	const read = `print -r -- "D=${funcsourcetrace[1]##*:}"`
	dir := t.TempDir()
	for _, tc := range []struct{ name, src string }{
		{"an unbracketed body", "\n\n() " + read + "\n"},
		{"a bracketed one", "\n\n() { " + read + " }\n"},
		{"a body two lines below the header", "\n\n()\n\n" + read + "\n"},
		{"and a call made inside a function", "f() {\n\n() " + read + "\n}\nf\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != "D=3\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, "D=3\n")
			}
		})
	}
}

// It is the **caller's** numbering that is kept, and not the script's.
//
// The two agree for every call made at the top level, which is where a grid
// would naturally be written — so these rows move the call inside something
// that is itself renumbered, and the answer is nought in each, because nought
// is what the frame around it calls that line.
func TestItIsTheCallersNumberingAndNotTheScripts(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"inside a named function",
			"f() {\n\n" + `() print -r -- "L=$LINENO"` + "\n}\nf\n", "L=2",
		},
		{
			"inside a bracketed nameless one",
			"\n\n" + `() { () print -r -- "L=$LINENO" }` + "\n", "L=0",
		},
		// And borrowed text, where the offset the caller was running under
		// has to be kept as well as the origin: this answered one line short
		// while only the origin was carried.
		{
			"inside a command substitution",
			"\n\n" + `print -r -- "[$(() print -r -- $LINENO)]"` + "\n", "[3]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}

// Only the parenthesised header keeps it. The keyword's own non-bracketed
// body is renumbered exactly as a bracketed one is.
//
// Same body, same line, and only the word that opened the construct moves.
// A rule read off the `()` rows alone would have said "an unbracketed body is
// never renumbered", which is wrong in half the grammar.
func TestOnlyTheParenthesisedHeaderKeepsTheCallersNumbering(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"the keyword with a body behind a redirection",
			"\n\n" + `function >f print -r -- "L=$LINENO"` + "\n" + `print -r -- "[$(<f)]"` + "\n",
			"[L=0]",
		},
		{
			"and the keyword with the body on the next line",
			"\n\nfunction\n" + `print -r -- "L=$LINENO"` + "\n", "L=1",
		},
		// The pair, spelled out: the parenthesised header on the same line
		// answers its own line where the keyword answers nought.
		{
			"where the parenthesised header answers its own line",
			"\n\n" + `() print -r -- "L=$LINENO"` + "\n", "L=3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}
