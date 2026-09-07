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
// The stage is the second question and is #1110's: this shell **parses** it
// and complains when the loop is reached, so `ksh -n` accepts the script — and
// then ends it, where bash carries on. Measured 2026-09-06 and re-measured
// 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over a script
// file and through `-c` alike:
//
//	ksh -n s.sh              accepts, silent, status 0
//	ksh s.sh                 the complaint, and stops
//	the script's own status   1
//
// The whole rendered line is asserted rather than a substring of it: the
// location, the sentence and whether the script carried on are three separate
// answers, and a `Contains` check passes with any two of them wrong.
//
// This replaces the assertion that the *parse* fails. That assertion was
// right about the wording and the status and wrong about the stage, so what
// it was pinning is kept and moved rather than dropped.
func TestALoopNamedByAnExpansionIsRefusedWhenTheLoopRuns(t *testing.T) {
	const src = "n=x\nfor $n in a b; do :; done\necho after"
	if _, err := syntax.Parse(src+"\n", ksh.Dialect()); err != nil {
		t.Fatalf("refused while parsing: %v — this shell accepts it and `ksh -n` is silent", err)
	}
	out, st := runKsh(t, t.TempDir(), src)
	const want = "ksh: line 2: $n: invalid variable name\n"
	if out != want || st != 1 {
		t.Errorf("\n got %q (status %d)\nwant %q at 1", out, st, want)
	}
}

// `for 1x` behaves identically, which is what says the *expansion* is not
// what moved the stage: the whole check moved.
func TestALoopNamedByANonNameIsRefusedWhenTheLoopRuns(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "for 1x in a b; do :; done\necho after")
	const want = "ksh: 1x: invalid variable name\n"
	if out != want || st != 1 {
		t.Errorf("\n got %q (status %d)\nwant %q at 1", out, st, want)
	}
}

// `select` answers exactly as `for` does here, and this is the row #1110 had
// the other way round.
//
// It recorded ksh93 as fatal for `for` and not for `select`. Re-measured
// 2026-09-07 on 93u+ 2012-08-01 with stdin closed, both spellings end the
// script at 1 with the line after the loop unreached — so the loop keyword is
// not an axis and the fatality half has two answers rather than three.
func TestTheMenuLoopAnswersAsTheForLoopDoes(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "n=x\nselect $n in a b; do :; done\necho after")
	const want = "ksh: line 2: $n: invalid variable name\n"
	if out != want || st != 1 {
		t.Errorf("\n got %q (status %d)\nwant %q at 1", out, st, want)
	}
}

// A redirection on the clause makes the refusal **not** fatal, which is this
// shell's alone and is measured rather than tolerated.
//
// `for 1x in a b; do :; done > mf; echo after` reports the same sentence,
// prints `after` and exits 0, where the same loop without a redirection ends
// the script at 1. Any redirection does it — `2>&1` behaves as `> mf` does —
// and the `select` spelling behaves the same way. bash-as-`sh` stops at 2 with
// a redirection and without one, so it is not a rule about clauses in general.
func TestARedirectionMakesTheRefusalNotFatal(t *testing.T) {
	for _, src := range []string{
		"for 1x in a b; do :; done > mf\necho \"after st=$?\"",
		"for 1x in a b; do :; done 2>&1\necho \"after st=$?\"",
		"select 1x in a b; do :; done > mf\necho \"after st=$?\"",
	} {
		out, st := runKsh(t, t.TempDir(), src)
		want := "ksh: 1x: invalid variable name\nafter st=1\n"
		if out != want || st != 0 {
			t.Errorf("%q:\n got %q (status %d)\nwant %q at 0", src, out, st, want)
		}
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
		out, _ := runKsh(t, t.TempDir(), "for "+tc.name+" in a b; do :; done")
		if want := "ksh: " + tc.want + "\n"; out != want {
			t.Errorf("for %s:\n got %q\nwant %q", tc.name, out, want)
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
	// name in any reading is still refused — when the loop runs, which is
	// the stage this shell checks at, and with the word named as written.
	for _, name := range []string{"1x", `"1x"`, `"a b"`, `""`, `"$n"`} {
		out, st := runKsh(t, t.TempDir(), "for "+name+" in a b; do :; done\necho after")
		want := "ksh: " + name + ": invalid variable name\n"
		if out != want || st != 1 {
			t.Errorf("for %s:\n got %q (status %d)\nwant %q at 1", name, out, st, want)
		}
	}
}
