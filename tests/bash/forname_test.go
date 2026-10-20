// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What this shell says about a loop whose name is an expansion, what it exits
// with, and **when it says it**. `n=x; for $n in a b` is refused by every
// shell in the panel and was taken here in every dialect, binding a variable
// literally called `n` at status 0 with nothing said (#1076).
//
// The stage is the second question and is #1110's: this shell **parses** it
// and complains when the loop is reached, so `bash -n` accepts the script and
// everything after the loop still runs. Measured 2026-09-06 and re-measured
// 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over a script
// file and through `-c` alike:
//
//	bash -n s.sh                        accepts, silent, status 0
//	bash s.sh                           the complaint, then `after st=1`
//	the script's own status              0
//
// The whole rendered line is asserted rather than a substring of it: the
// location, the sentence and whether the script carried on are three separate
// answers, and a `Contains` check passes with any two of them wrong.
//
// This replaces the assertion that the *parse* fails. That assertion was
// right about the wording and the status and wrong about the stage, so what
// it was pinning is kept and moved rather than dropped.
func TestALoopNamedByAnExpansionIsRefusedWhenTheLoopRuns(t *testing.T) {
	const src = "n=x\nfor $n in a b; do :; done\necho \"after st=$?\""
	if _, err := syntax.Parse(src+"\n", bash.Dialect()); err != nil {
		t.Fatalf("refused while parsing: %v — this shell accepts it and `bash -n` is silent", err)
	}
	out, st := runBash(t, t.TempDir(), src)
	const want = "bash: line 2: `$n': not a valid identifier\nafter st=1\n"
	if out != want || st != 0 {
		t.Errorf("\n got %q (status %d)\nwant %q at 0", out, st, want)
	}
}

// `for 1x` behaves identically, which is what says the *expansion* is not
// what moved the stage: it is the whole check that moved, and it has been
// this way since long before #1076 added the expansion half.
func TestALoopNamedByANonNameIsRefusedWhenTheLoopRuns(t *testing.T) {
	const src = "for 1x in a b; do :; done\necho \"after st=$?\""
	out, st := runBash(t, t.TempDir(), src)
	const want = "bash: line 1: `1x': not a valid identifier\nafter st=1\n"
	if out != want || st != 0 {
		t.Errorf("\n got %q (status %d)\nwant %q at 0", out, st, want)
	}
}

// `select` answers the same as `for`, which #1110 had recorded as differing
// in ksh93 and which does not differ in either shell that gets here.
func TestTheMenuLoopAnswersAsTheForLoopDoes(t *testing.T) {
	const src = "n=x\nselect $n in a b; do :; done\necho \"after st=$?\""
	out, st := runBash(t, t.TempDir(), src)
	const want = "bash: line 2: `$n': not a valid identifier\nafter st=1\n"
	if out != want || st != 0 {
		t.Errorf("\n got %q (status %d)\nwant %q at 0", out, st, want)
	}
}

// Every spelling of the expansion reaches the same sentence, with the word
// quoted back **as written** — the token's literal is `n` for the first three
// of these and no shell in the panel says `n`.
func TestTheNameIsNamedAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"$n", "bash: line 1: `$n': not a valid identifier\n"},
		{"${n}", "bash: line 1: `${n}': not a valid identifier\n"},
		{`"$n"`, "bash: line 1: `\"$n\"': not a valid identifier\n"},
		{"$(echo n)", "bash: line 1: `$(echo n)': not a valid identifier\n"},
	} {
		out, _ := runBash(t, t.TempDir(), "for "+tc.name+" in a b; do :; done")
		if out != tc.want {
			t.Errorf("for %s:\n got %q\nwant %q", tc.name, out, tc.want)
		}
	}
}

// A *quoted* name is the axis beside it, and this shell is on the refusing
// side: `for "i" in a b` names the word here where one shell in the panel
// removes the quoting and binds `i`. The escape is refused with the quotes.
//
// Refused at the same stage as every other bad name, which is the half this
// row gained: it used to assert a parse failure and now asserts the loop's.
func TestAQuotedLoopNameIsRefusedHere(t *testing.T) {
	if bash.Dialect().ForNameMayBeQuoted {
		t.Error("this shell wants a loop name written plainly")
	}
	for _, name := range []string{`"i"`, `'i'`, "i\"\"", `"i"x`, `\i`} {
		src := "for " + name + " in a b; do :; done\necho \"after st=$?\""
		if _, err := syntax.Parse(src+"\n", bash.Dialect()); err != nil {
			t.Errorf("%q refused while parsing: %v", src, err)
			continue
		}
		out, st := runBash(t, t.TempDir(), src)
		want := "bash: line 1: `" + name + "': not a valid identifier\nafter st=1\n"
		if out != want || st != 0 {
			t.Errorf("for %s:\n got %q (status %d)\nwant %q at 0", name, out, st, want)
		}
	}
}

// POSIX mode moves it, and that is the whole of the bash-as-`sh` column: the
// same complaint, and then the script stops at 2.
//
// A mode rather than a build, which is what makes it a value of the axis and
// not a preset of its own: `set -o posix` in bash 5.3 answers exactly as the
// same binary invoked as `sh` does, and bash 3.2 agrees with 5.3 under its own
// name. Measured 2026-09-07, and `set +o posix` puts the dialect's own answer
// back rather than the standard's opposite.
func TestPosixModeEndsTheScriptInstead(t *testing.T) {
	if got := bash.Semantics().ForNameWhenTheLoopRuns; got != interp.ForNameFailsTheLoop {
		t.Errorf("under its own name = %v, want ForNameFailsTheLoop", got)
	}
	out, st := runBash(t, t.TempDir(), "set -o posix\nfor 1x in a b; do :; done\necho after")
	const want = "bash: line 2: `1x': not a valid identifier\n"
	if out != want || st != 2 {
		t.Errorf("in POSIX mode:\n got %q (status %d)\nwant %q at 2", out, st, want)
	}
	// And out again, which needs a saved value rather than the standard's
	// opposite: a shell POSIX already agreed with would otherwise lose its
	// own answer on the way out.
	out, st = runBash(t, t.TempDir(),
		"set -o posix\nset +o posix\nfor 1x in a b; do :; done\necho \"after st=$?\"")
	const wantBack = "bash: line 3: `1x': not a valid identifier\nafter st=1\n"
	if out != wantBack || st != 0 {
		t.Errorf("after leaving POSIX mode:\n got %q (status %d)\nwant %q at 0", out, st, wantBack)
	}
}
