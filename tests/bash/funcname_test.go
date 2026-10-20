// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// What this shell says about a `function` definition whose name holds an
// expansion, what it exits with, and **when it says it**.
//
// The stage is the question. #1256 removed a silent wrong answer here — the
// token's literal text was taken, so `function _p_${w} { … }` defined `_p_w`,
// a different function, at status 0 — and left a parse refusal in its place.
// This shell does not refuse the parse: it reads the definition, complains
// where the definition *runs*, naming the word as written, and carries on.
// Measured 2026-09-10 through `-c`, `w=foo; function _p_${w} { echo HI; };
// echo st=$?; echo after`:
//
//	bash -n s.sh            accepts, silent, status 0
//	bash s.sh               the complaint, then `st=1` and `after`
//	the script's own status 0
//
// The whole rendered line is asserted rather than a substring: the location,
// the sentence and whether the script carried on are three separate answers,
// and a `Contains` check passes with any two of them wrong (#1296).
func TestAFunctionNameHoldingAnExpansionIsRefusedWhenTheDefinitionRuns(t *testing.T) {
	const src = "w=foo\nfunction _p_${w} { echo HI; }\necho \"st=$?\"\necho after"
	if _, err := syntax.Parse(src+"\n", bash.Dialect()); err != nil {
		t.Fatalf("refused while parsing: %v — this shell accepts it and `bash -n` is silent", err)
	}
	out, st := runBash(t, t.TempDir(), src)
	const want = "bash: line 2: `_p_${w}': not a valid identifier\nst=1\nafter\n"
	if out != want || st != 0 {
		t.Errorf("\n got %q (status %d)\nwant %q at 0", out, st, want)
	}
}

// Every spelling of the expansion reaches the same sentence, with the word
// quoted back **as written** — the token's literal for the first three is
// `_p_` plus the name inside, and no shell in the panel says that.
func TestTheFunctionNameIsNamedAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"_p_$w", "bash: line 1: `_p_$w': not a valid identifier\n"},
		{"_p_${w}", "bash: line 1: `_p_${w}': not a valid identifier\n"},
		{"_p_$@", "bash: line 1: `_p_$@': not a valid identifier\n"},
		{"_p_$(echo n)", "bash: line 1: `_p_$(echo n)': not a valid identifier\n"},
	} {
		out, _ := runBash(t, t.TempDir(), "function "+tc.name+" { echo HI; }")
		if out != tc.want {
			t.Errorf("function %s:\n got %q\nwant %q", tc.name, out, tc.want)
		}
	}
}

// And nothing is defined, which is the half the wording cannot show: the
// literal text of `_p_${w}` is `_p_w`, a perfectly good name for a different
// function, and defining it silently at status 0 is exactly what #1256
// removed.
func TestARefusedFunctionNameDefinesNothingHere(t *testing.T) {
	const src = "w=foo\nfunction _p_${w} { echo HI; }\n" +
		"_p_foo 2>/dev/null || echo noasked\n_p_w 2>/dev/null || echo nolit"
	out, st := runBash(t, t.TempDir(), src)
	const want = "bash: line 2: `_p_${w}': not a valid identifier\nnoasked\nnolit\n"
	if out != want || st != 0 {
		t.Errorf("\n got %q (status %d)\nwant %q at 0", out, st, want)
	}
}

// A name this shell *does* accept is untouched, which is the control: the
// punctuation the other keyword shell refuses is an ordinary name here, so
// the stage flag did not widen or narrow what a name is.
func TestAPunctuatedFunctionNameIsStillDefinedHere(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "function f-g { echo HI; }\nf-g\necho \"st=$?\"")
	if want := "HI\nst=0\n"; out != want || st != 0 {
		t.Errorf("\n got %q (status %d)\nwant %q at 0", out, st, want)
	}
}
