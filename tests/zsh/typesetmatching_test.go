// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// `typeset -m` and `typeset +m`, measured against zsh 5.9.2 (2026-09-10) with
// a scrubbed environment and no startup files.
//
// The operands are patterns rather than names, and every other letter on the
// line decides what matching *means* — the four readings are in
// interp/declarematching.go. This file pins the two that a real startup runs
// and the letter's reach across the builtins that spell it.
//
// It is `typeset` and `declare` alone here. `local -m`, `export -m`,
// `readonly -m`, `integer -m` and `float -m` are each `bad option: -m` in this
// shell, measured a builtin at a time, so the letter is in DeclareOptions and
// not in LocalOptions.

// The reading a completion dump collects its function names with: `typeset
// +fm '_*'` names the matching functions and writes no bodies. Refusing the
// letter is what left every interactive startup without a dump (#1674).
func TestTypesetPlusFMNamesTheMatchingFunctions(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `_b() { echo two; }
_a() { echo one; }
c() { echo three; }
typeset +fm '_*'`)
	want := "_a\n_b\n"
	if out != want || st != 0 {
		t.Errorf("typeset +fm = %q (status %d), want %q", out, st, want)
	}
}

// The same letter under the other sign of `f` writes the bodies, which is what
// makes the row above a statement about the sign rather than about `-m`.
func TestTypesetMinusFMWritesTheMatchingBodies(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `_a() { echo one; }
c() { echo three; }
typeset -fm '_*'`)
	want := "_a () {\n\techo one\n}\n"
	if out != want || st != 0 {
		t.Errorf("typeset -fm = %q (status %d), want %q", out, st, want)
	}
}

// Over parameters the two signs are two listings: the value under a minus, the
// attributes and the bare name under a plus.
func TestTypesetMatchingOverParameters(t *testing.T) {
	// `q*` rather than `p*`: this shell's own `path`, `psvar` and
	// `parameters` all begin with a p and a pattern reaches them, which is
	// true of the shell being modeled too — measured, and the reason these
	// names are the ones a test can state an expectation about.
	src := `qa=1
typeset -i qb=2
typeset -a qc=(x y)
typeset -A qd=(k v)
zz=9
`
	for _, tc := range []struct{ name, line, want string }{
		{"a minus writes the values", "typeset -m 'q*'", "qa=1\nqb=2\nqc=( x y )\nqd=( [k]=v )\n"},
		{"a plus writes the attributes and names", "typeset +m 'q*'", "qa\ninteger qb\narray qc\nassociation qd\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), src+tc.line)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.line, out, st, tc.want)
			}
		})
	}
}

// A minus-signed attribute letter alongside is a declaration over the matches,
// and a plus-signed one is a third listing — the matching names that carry the
// attribute. The pair has to be tested together: one reading of the sign gives
// both, and the wrong reading swaps them.
func TestTypesetMatchingDeclaresUnderMinusAndListsUnderPlus(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `qa=1
qb=2
typeset -mx 'qa'
typeset +mx 'q*'
typeset -p qa
typeset -p qb`)
	want := "qa\nexport qa=1\ntypeset qb=2\n"
	if out != want || st != 0 {
		t.Errorf("the -mx/+mx pair = %q (status %d), want %q", out, st, want)
	}
}

// The letter reaches `declare` too, because they are one builtin under two
// words in this shell.
func TestDeclareSpellsTheMatchingLetter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "qa=1\ndeclare +m 'q*'")
	if out != "qa\n" || st != 0 {
		t.Errorf("declare +m = %q (status %d), want %q", out, st, "qa\n")
	}
}

// And it does *not* reach `local`, where this shell has no such option. The
// refusal has to be the bad-option one and not the missing-feature one: saying
// a letter is on its way when the shell being modeled does not have it is the
// worse of the two answers.
func TestLocalHasNoMatchingLetter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "f() { local -m 'q*'; }\nf")
	if st == 0 || !strings.Contains(out, "bad option: -m") {
		t.Errorf("local -m = %q (status %d), want the bad-option refusal", out, st)
	}
	if strings.Contains(out, "not implemented") {
		t.Errorf("local -m = %q, want no claim that the letter is missing here", out)
	}
}

// The letter has left the missing list under every name that spells it, which
// is what keeps the two statements from drifting apart: a letter named as
// unimplemented *and* answered is a shell telling a script both things.
func TestTheMatchingLetterIsNoLongerCalledMissing(t *testing.T) {
	missing := zsh.Diagnostics().UnimplementedOptionLetters
	for _, name := range []string{"typeset", "declare", "local"} {
		if strings.ContainsRune(missing[name], 'm') {
			t.Errorf("UnimplementedOptionLetters[%s] = %q, which still claims -m is missing",
				name, missing[name])
		}
	}
	if !strings.ContainsRune(zsh.Semantics().DeclareOptions, 'm') {
		t.Errorf("DeclareOptions = %q, want the matching letter in it",
			zsh.Semantics().DeclareOptions)
	}
	if strings.ContainsRune(zsh.Semantics().LocalOptions, 'm') {
		t.Errorf("LocalOptions = %q, want no matching letter — this shell's `local` has none",
			zsh.Semantics().LocalOptions)
	}
}

// The attribute words `+m` writes, and the order they come in — every row
// measured on zsh 5.9.2 (2026-09-10) with no startup files, one row per seam.
// `+m` is what found them: it writes the words and no value, so a missing word
// is the whole of a row rather than a detail at the front of one.
func TestTypesetPlusMWritesTheMeasuredAttributeWords(t *testing.T) {
	for _, tc := range []struct{ name, decl, want string }{
		{"a plain scalar", "qa=1", "qa\n"},
		{"an integer", "typeset -i qa=1", "integer qa\n"},
		{"an integer with a base", "typeset -i16 qa=255", "integer 16 qa\n"},
		{"a float", "typeset -F qa=1.5", "float qa\n"},
		{"an array", "typeset -a qa=(x y)", "array qa\n"},
		{"an association", "typeset -A qa=(k v)", "association qa\n"},
		{"uppercase", "typeset -u qa=a", "uppercase qa\n"},
		{"lowercase", "typeset -l qa=A", "lowercase qa\n"},
		{"unique", "typeset -U qa=abc", "unique qa\n"},
		{"readonly", "typeset -r qa=1", "readonly qa\n"},
		{"a readonly with a base", "typeset -i16 -r qa=255", "integer 16 readonly qa\n"},
		{"readonly before unique", "typeset -rU qa=(x y)", "array readonly unique qa\n"},
		{"exported before unique", "typeset -aU qa=(x)\ntypeset -x qa", "array exported unique qa\n"},
		{"a case before readonly", "typeset -lr qa=A", "lowercase readonly qa\n"},
		{"a case before unique", "typeset -uU qa=(a b)", "array uppercase unique qa\n"},
		// The hiding attribute is not a word in any shape: it takes the
		// *value* away, and `+m` has already taken that away.
		{"hidden", "typeset -H qa=1", "qa\n"},
		{"a hidden integer", "typeset -Hi qa=1", "integer qa\n"},
		// An exported scalar at the top level is an entry in the
		// environment and this listing says nothing about it. The two
		// kinds that cannot be one keep the word, and so does a local.
		{"an exported scalar", "typeset -x qa=1", "qa\n"},
		{"an exported readonly scalar", "typeset -rx qa=1", "readonly qa\n"},
		{"an exported array", "typeset -xa qa=(x)", "array exported qa\n"},
		{"an exported association", "typeset -xA qa=(k v)", "association exported qa\n"},
		{"an exported local", "local -x qa=1", "local exported qa\n"},
		{"an exported local array", "local -xa qa=(x)", "array local exported qa\n"},
		{"a local", "local qa=1", "local qa\n"},
		{"a local readonly", "local -r qa=1", "local readonly qa\n"},
		{"a local unique", "typeset -U qa=abc", "local unique qa\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A declaration naming a scope runs inside a function, which is
			// the only place `local` can be one of the words.
			src := tc.decl + "\ntypeset +m 'q*'"
			if strings.Contains(tc.want, "local ") {
				src = "f() {\n" + src + "\n}\nf"
			}
			out, st := runZsh(t, t.TempDir(), src)
			if out != tc.want || st != 0 {
				t.Errorf("+m after %q = %q (status %d), want %q", tc.decl, out, st, tc.want)
			}
		})
	}
}

// The tie is the last word and the one that carries a second name: it names
// the *other* half, so either end reads as the pair. Measured on both halves,
// alone and with another attribute in front of it.
func TestTypesetPlusMNamesTheOtherHalfOfATie(t *testing.T) {
	for _, tc := range []struct{ name, decl, want string }{
		{"both halves", "typeset -T QS qs", "array tied QS qs\ntied qs QS\n"},
		{"a readonly tie", "typeset -rT QS qs", "array readonly tied QS qs\nreadonly tied qs QS\n"},
		{"a separator of its own", "typeset -T QS qs '#'", "array tied QS qs\ntied qs QS\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.decl+"\ntypeset +m 'qs'\ntypeset +m 'QS'")
			if out != tc.want || st != 0 {
				t.Errorf("+m after %q = %q (status %d), want %q", tc.decl, out, st, tc.want)
			}
		})
	}
}

// A based integer lists as the decimal number it stands for, and bare — where
// the *value* the name holds is the based text. The `-m` half is what a script
// reads back, so the two are pinned together.
func TestTypesetMatchingWritesABasedIntegerInDecimal(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"typeset -i16 qa=255\ntypeset -i8 qb=8\ntypeset -i16 qc=-255\n"+
			"typeset -m 'q*'\necho $qa")
	want := "qa=255\nqb=8\nqc=-255\n16#FF\n"
	if out != want || st != 0 {
		t.Errorf("-m over based integers = %q (status %d), want %q", out, st, want)
	}
}

// The sign is read per *letter* and not per option word. Measured on zsh 5.9.2
// as a pair, because one reading of the sign gives both rows and the wrong
// reading swaps them: `typeset -m +x` takes the attribute off every match and
// `typeset +mx` lists the matches that carry it, unchanged.
func TestTypesetMatchingReadsEachLettersSign(t *testing.T) {
	src := "qa=1\ntypeset -x qa\n"
	for _, tc := range []struct{ name, line, want string }{
		{"a plus letter behind a minus m removes it", "typeset -m +x 'q*'", "typeset qa=1\n"},
		{"the two letters in one plus word list", "typeset +mx 'q*'", "qa\nexport qa=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), src+tc.line+"\ntypeset -p qa")
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.line, out, st, tc.want)
			}
		})
	}
}

// An assignment outranks `-p`: the value reaches every match and the listing is
// only the shape of the answer. Measured — and the same line *without* the
// matching letter assigns nothing at all, which is why this is a row about the
// pair rather than about `-p`.
func TestTypesetMatchingUnderThePrintLetterStillAssigns(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "qa=1\nqb=2\nzz=3\ntypeset -pm 'q*'=9\ntypeset -p zz")
	want := "typeset qa=9\ntypeset qb=9\ntypeset zz=3\n"
	if out != want || st != 0 {
		t.Errorf("typeset -pm 'q*'=9 = %q (status %d), want %q", out, st, want)
	}
}
