// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// Where the input runs out with a construct open, the token that ended the
// last command made of redirections alone is what is named unmatched, rather
// than the construct's keyword (#6618). Measured 2026-10-15 on ksh93u+
// 2012-08-01 at /bin/ksh with `-c`, a script file and standard input; the
// table is on syntax.Dialect.RedirectionOnlyCommandNamesTheTokenAfterIt.
func TestARedirectionOnlyCommandNamesTheTokenAfterIt(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"if {v}<x\necho ok", "newline"},
		{"while {v}<x\necho ok", "newline"},
		{"until {v}<x\necho ok", "newline"},
		{"{ {v}<x\necho ok", "newline"},
		{"( {v}<x\necho ok", "newline"},
		{"if <x\necho ok", "newline"},
		{"if 3<x\necho ok", "newline"},
		{"if : | <x\necho ok", "newline"},
		{"if ! <x\necho ok", "newline"},
		{"if :; {v}<x\necho ok", "newline"},
		{"if :; then <x\necho ok", "newline"},
		{"for i in a; do <x\necho ok", "newline"},
		{"if {v}<x;\necho ok", ";"},
		{"if {v}<x; echo a\necho ok", ";"},
		{"if {v}<x |\ncat", "|"},
		{"if {v}<x &&\necho ok", "&&"},
		{"if {v}<x ||\necho ok", "||"},
		{"if <x; :; <y |\necho ok", "|"},
		// A clause displaces it, a construct closing drops it, and a word
		// or an assignment makes it an ordinary command.
		{"if {v}<x; then\necho ok", "then"},
		{"if { <x; }\necho ok", "if"},
		{"if (<x)\necho ok", "if"},
		{"if : {v}<x\necho ok", "if"},
		{"if x=1 {v}<x\necho ok", "if"},
		{"if echo <x\necho ok", "if"},
		{"if {v}<x", "if"},
	} {
		_, err := syntax.Parse(c.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", c.src)
			continue
		}
		want := "`" + c.want + "' unmatched"
		got := ksh.Diagnostics().ParseFailure(err)
		if len(got) < len(want) || got[len(got)-len(want):] != want {
			t.Errorf("%q: got %q, want it to end %q", c.src, got, want)
		}
	}
}

// The rest of the panel names the construct, so the mark is this dialect's
// alone: the grammar without the flag names `if`.
func TestARedirectionOnlyCommandIsNamedOnlyWhereTheDialectSaysSo(t *testing.T) {
	d := ksh.Dialect()
	d.RedirectionOnlyCommandNamesTheTokenAfterIt = false
	_, err := syntax.Parse("if {v}<x\necho ok", d)
	got := ksh.Diagnostics().ParseFailure(err)
	if want := "syntax error at line 2: `if' unmatched"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
