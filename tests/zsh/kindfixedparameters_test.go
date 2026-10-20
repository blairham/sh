// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A kind letter aimed at one of the shell's own parameters is refused, and
// the refusal **ends the shell**: nothing after the line runs.
//
// Measured on zsh 5.9.2, 2026-09-27, under `-f` from a script file with a
// scratch `HOME` (#4834). The `after` that never arrives is half of what is
// being pinned here — this engine took the line at 0 and carried on.
func TestAKindLetterOverAShellParameterEndsTheShell(t *testing.T) {
	out, st := answersRun(t, `f() { typeset -i path; print "  st=$?" }
f
print after`)
	want := "f:typeset: path: can't change type of a special parameter\n"
	if out != want || st != 1 {
		t.Errorf("the refusal = %q (status %d), want %q at 1", out, st, want)
	}
}

// Every declaration word reaches it, and the written word is in the
// *location* rather than in the sentence — the same shape the inconsistent-type
// refusal already has in this shell.
//
// `export` and the two numeric words are here because they are the ones a
// reading of "this is `typeset`'s rule" would have missed, and the top-level
// row because this refusal — unlike `private`'s — is not about taking a
// scope.
func TestEveryDeclarationWordRefusesTheKindLetter(t *testing.T) {
	for _, row := range []struct{ src, want string }{
		{"f() { local -A path }\nf", "f:local: path: can't change type of a special parameter\n"},
		{"f() { declare -F path }\nf", "f:declare: path: can't change type of a special parameter\n"},
		{"f() { typeset -g -i path }\nf", "f:typeset: path: can't change type of a special parameter\n"},
	} {
		out, st := answersRun(t, row.src)
		if out != row.want || st != 1 {
			t.Errorf("%q = %q (status %d), want %q at 1", row.src, out, st, row.want)
		}
	}
	for _, row := range []struct{ word, src string }{
		{"typeset", "typeset -i path"},
		{"export", "export -i path"},
		{"integer", "integer path"},
		{"float", "float path"},
	} {
		out, st := answersRun(t, row.src+"\nprint after")
		if !strings.Contains(out, row.word+":1: path: can't change type of a special parameter") || st != 1 {
			t.Errorf("%q = %q (status %d), want the %s refusal at 1", row.src, out, st, row.word)
		}
	}
}

// The controls, and the first of them is what makes the rest evidence: the
// same letters over an ordinary name are taken in both shells, so it is the
// name that decides and not the letter.
//
// The rest say what the rule is *keyed on*. It is not the kind the name is
// holding — `SECONDS` is an integer and takes the float letter, because the
// slot takes two kinds — and it is not "every parameter the shell provides",
// since `EPOCHSECONDS` is produced on being read and takes an association
// letter. A slot's own kind is taken under a minus and refused under a
// **plus**, which is the half a table of minus letters alone cannot see.
func TestWhatTheKindRefusalIsKeyedOn(t *testing.T) {
	out, st := answersRun(t, `f1() { typeset -i v;           print "  ordinary=${(t)v}" }
f2() { typeset -F SECONDS;     print "  seconds=${(t)SECONDS}" }
f3() { typeset -i RANDOM;      print "  random=${(t)RANDOM}" }
f4() { typeset -A EPOCHSECONDS; print "  produced=$?" }
f5() { typeset -a path;        print "  ownkind=${(t)path}" }
f6() { typeset +i RANDOM;      print "  neverhere=$?" }
f1; f2; f3; f4; f5; f6`)
	want := "  ordinary=integer-local\n  seconds=float-local-special\n" +
		"  random=integer-local-special\n  produced=0\n" +
		"  ownkind=array-local-tied-special\n" +
		"f6:typeset: RANDOM: can't change type of a special parameter\n"
	if out != want || st != 1 {
		t.Errorf("the controls = %q (status %d), want %q at 1", out, st, want)
	}
}

// A hidden shadow is the one exemption, and only where a shadow stands.
//
// The letter on this line and a hiding declaration earlier in the same call
// both reach it, and the **top-level** row is the control that shapes the
// rule: there the letter detaches no shadow and the refusal is the ordinary
// one. `unset` is the other control — the slot is there whether or not a
// value is.
func TestOnlyAHiddenShadowExemptsTheKindLetter(t *testing.T) {
	out, st := answersRun(t, `f1() { typeset -h -i path; print "  letterhere=$?" }
f2() { typeset -h path; typeset -i path; print "  shadowalready=$?" }
f1; f2`)
	want := "  letterhere=0\n  shadowalready=0\n"
	if out != want || st != 0 {
		t.Errorf("the exemption = %q (status %d), want %q at 0", out, st, want)
	}
	out, st = answersRun(t, "typeset -h -i path\nprint after")
	if !strings.Contains(out, "typeset:1: path: can't change type of a special parameter") || st != 1 {
		t.Errorf("the top level = %q (status %d), want the refusal at 1", out, st)
	}
	out, st = answersRun(t, "unset path\ntypeset -i path\nprint after")
	if !strings.Contains(out, "typeset:2: path: can't change type of a special parameter") || st != 1 {
		t.Errorf("after unset = %q (status %d), want the refusal at 1", out, st)
	}
}

// And it is asked ahead of `private`'s own refusals, which is measured rather
// than chosen: the pair below is the discriminator, since a rule in either
// order agrees with the reference on one of these two rows.
//
// `private -A path` is the *type* sentence and is fatal. `private -i RANDOM`
// — a letter that slot takes — is the *scope* sentence at 1 with the script
// carrying on, and `RANDOM` is in one table and not the other.
func TestTheKindRefusalComesBeforeThePrivateOne(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f() { private -A path }
f
print after`)
	want := "f:private: path: can't change type of a special parameter\n"
	if out != want || st != 1 {
		t.Errorf("private with a kind letter = %q (status %d), want %q at 1", out, st, want)
	}
	out, st = answersRun(t, `zmodload zsh/param/private
f() { private -i RANDOM }
f
print after`)
	want = "f:private: can't change scope of existing param: RANDOM\nafter\n"
	if out != want || st != 0 {
		t.Errorf("private with a taken letter = %q (status %d), want %q at 0", out, st, want)
	}
}
