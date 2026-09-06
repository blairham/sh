// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// What this shell says about a loop whose name is an expansion, and what it
// exits with. `n=x; for $n in a b` is refused by every shell in the panel and
// was taken here in every dialect, binding a variable literally called `n` at
// status 0 with nothing said (#1076).
//
// Measured 2026-09-06, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over a
// script file and through `-c` alike. Six panel columns give the one refusal
// **four** wordings — the three bash columns share theirs — at three statuses,
// which is why the detection was left out of #1057 and filed on its own.
//
// The whole rendered report is asserted rather than a substring of it: the
// location, the sentence and whether the offending line is echoed back are
// three separate answers, and a `Contains` check passes with any two of them
// wrong.
func TestALoopNamedByAnExpansionIsRefusedInThisShellsWords(t *testing.T) {
	const src = "n=x\nfor $n in a b; do :; done\n"
	const want = "s.sh: line 2: $n: invalid variable name\n"
	_, err := syntax.Parse(src, ksh.Dialect())
	if err == nil {
		t.Fatal("parsed; every shell in the panel refuses this")
	}
	d := ksh.Diagnostics().ForScript()
	if got := d.ParseDiagnostic("s.sh", "", err, src); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	if got, want := d.StatusForParseError(err), 1; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

// Every spelling of the expansion reaches the same sentence, with the word
// quoted back **as written** — the token's literal is `n` for the first three
// of these and no shell in the panel says `n`.
func TestTheNameIsNamedAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"$n", "$n: invalid variable name"},
		{"${n}", "${n}: invalid variable name"},
		{`"$n"`, "\"$n\": invalid variable name"},
		{"$(echo n)", "$(echo n): invalid variable name"},
	} {
		src := "for " + tc.name + " in a b; do :; done\n"
		_, err := syntax.Parse(src, ksh.Dialect())
		if err == nil {
			t.Fatalf("%q parsed; this shell refuses it", src)
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", src, got, tc.want)
		}
	}
}

// A *quoted* name is the axis beside it, and this shell is the one that
// removes the quoting and takes the name: `for "i" in a b` binds `i` here and
// is a refusal in the other four. The escape travels with the quotes rather
// than being a question of its own — all five spellings run here.
func TestAQuotedLoopNameIsTakenHere(t *testing.T) {
	if !ksh.Dialect().ForNameMayBeQuoted {
		t.Fatal("this shell takes a quoted loop name")
	}
	for _, tc := range []struct{ name, binds string }{
		{`"i"`, "i"},
		{`'i'`, "i"},
		{"i\"\"", "i"},
		{`"i"x`, "ix"},
		{`\i`, "i"},
	} {
		src := "for " + tc.name + " in a b; do :; done\n"
		f, err := syntax.Parse(src, ksh.Dialect())
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		c, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.ForClause)
		if !ok {
			t.Fatalf("%q: not a for", src)
		}
		if len(c.Names) != 1 || c.Names[0] != tc.binds {
			t.Errorf("%q: names = %v, want [%s]", src, c.Names, tc.binds)
		}
	}
	// And the quoting is the whole of what it widens: a word that is not a
	// name in any reading is still refused.
	for _, name := range []string{"1x", `"1x"`, `"a b"`, `""`, `"$n"`} {
		src := "for " + name + " in a b; do :; done\n"
		if _, err := syntax.Parse(src, ksh.Dialect()); err == nil {
			t.Errorf("%q parsed; this shell refuses it", src)
		}
	}
}
