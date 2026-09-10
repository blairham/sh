// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The substrate behind `typeset -m` and `typeset +m`: a letter the dialect
// hands the declaration builtins through Semantics.DeclareOptions, under which
// the operands are patterns rather than names. Tests name the letter and never
// a shell — see interp/declarematching.go for the four readings and where they
// were measured.

// withMatching is declRun's setter for a dialect that spells the letter. The
// attribute-word listing comes with it because two of the four readings write
// that listing's halves, and a dialect with the letter and no such listing
// could not be asked what `+m` writes.
func withMatching(s *Semantics) {
	s.DeclareOptions = "aAfgilmprux"
	s.BareTypesetListing = BareLocalListsEveryParameter
	s.DeclaredNameWithoutValueIsEmpty = Yes
}

// The minus sense: each match's *value*, and no attribute word in front of it.
func TestMatchingWithAMinusWritesEachMatchsValue(t *testing.T) {
	src := "pa=1\npb=2\nqq=3\ntypeset -i pb\ntypeset -m 'p*'"
	out, errs, st := declRun(t, src, withMatching, Diagnostics{})
	want := "pa=\"1\"\npb=\"2\"\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("typeset -m 'p*' = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The plus sense: each match's attributes and its *name*, with no value. The
// row above is the control — a `+m` that wrote the whole listing row would
// answer this one with `integer pb="2"`.
func TestMatchingWithAPlusWritesEachMatchsAttributesAndName(t *testing.T) {
	src := "pa=1\npb=2\nqq=3\ntypeset -i pb\ntypeset +m 'p*'"
	out, errs, st := declRun(t, src, withMatching, Diagnostics{})
	want := "pa\ninteger pb\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("typeset +m 'p*' = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The function form, and the sign that decides it is the `f` letter's own:
// `+f` names the matches and `-f` writes their bodies, whichever sign the word
// carrying the `m` was given. This is the reading a completion-system dump
// collects its function names with.
func TestMatchingOverFunctionsIsNamedOrWrittenByTheFLetter(t *testing.T) {
	src := "_b() { echo two; }\n_a() { echo one; }\nc() { echo three; }\n"
	for _, tc := range []struct {
		name, line, want string
	}{
		{"named by a plus f", "typeset +fm '_*'", "_a\n_b\n"},
		{"named though the m rode a minus", "typeset +f -m '_*'", "_a\n_b\n"},
		{"written by a minus f", "typeset -fm '_a'", "_a () \n{ \n  echo one\n}\n"},
		{"written though the m rode a plus", "typeset -f +m '_a'", "_a () \n{ \n  echo one\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, src+tc.line, withMatching, Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.line, out, errs, st, tc.want)
			}
		})
	}
}

// A letter written with a *minus* alongside is a declaration over the matches
// rather than a listing, and it reaches only the names that already exist.
func TestMatchingWithAMinusLetterDeclaresOverTheMatches(t *testing.T) {
	src := "pa=1\npb=2\nqq=3\ntypeset -mx 'p*'\ntypeset -p pa\ntypeset -p pb\ntypeset -p qq"
	out, errs, st := declRun(t, src, withMatching, Diagnostics{})
	want := "declare -x pa=\"1\"\ndeclare -x pb=\"2\"\ndeclare -- qq=\"3\"\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("typeset -mx 'p*' left %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And with every letter written with a *plus* it is a third listing: the
// matching names that carry those attributes, named and nothing else. The row
// above is what makes the distinction load-bearing — the same letters under
// the other sign take the attribute away.
func TestMatchingWithPlusLettersListsTheMatchesCarryingThem(t *testing.T) {
	src := "pa=1\npb=2\nqq=3\ntypeset -x pb\ntypeset +mx 'p*' 'q*'\ntypeset -p pb"
	out, errs, st := declRun(t, src, withMatching, Diagnostics{})
	want := "pb\ndeclare -x pb=\"2\"\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("typeset +mx = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// An assignment takes the line out of the listing under either sign: the value
// reaches every parameter the pattern picked out.
func TestMatchingWithAnAssignmentSetsEveryMatch(t *testing.T) {
	for _, sign := range []string{"-", "+"} {
		t.Run(sign+"m", func(t *testing.T) {
			src := "pa=1\npb=2\nqq=3\ntypeset " + sign + "m 'p*'=9\necho \"[$pa][$pb][$qq]\""
			out, errs, st := declRun(t, src, withMatching, Diagnostics{})
			want := "[9][9][3]\n"
			if out != want || errs != "" || st != 0 {
				t.Errorf("typeset %sm 'p*'=9 = %q (stderr %q, status %d), want %q",
					sign, out, errs, st, want)
			}
		})
	}
}

// A pattern that matches nothing declares nothing and is silent at 0 — a
// listing that found nothing has answered the question.
func TestMatchingNothingIsASilentSuccess(t *testing.T) {
	for _, line := range []string{"typeset -m 'z*'", "typeset +m 'z*'", "typeset -mx 'z*'", "typeset +fm 'z*'"} {
		out, errs, st := declRun(t, "pa=1\n"+line, withMatching, Diagnostics{})
		if out != "" || errs != "" || st != 0 {
			t.Errorf("%s = %q (stderr %q, status %d), want nothing at 0", line, out, errs, st)
		}
	}
}

// A name two patterns reach is written once per pattern, which is the rule the
// same walk follows for functions.
func TestMatchingWritesANameOncePerPatternThatReachesIt(t *testing.T) {
	out, errs, st := declRun(t, "pa=1\ntypeset +m 'p*' 'pa'", withMatching, Diagnostics{})
	want := "pa\npa\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("two patterns over one name = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// `-p` outranks the *listings* wherever it is written, so the matches come back
// in the listing form rather than in either of the letter's own.
func TestMatchingUnderThePrintLetterUsesThePrintForm(t *testing.T) {
	for _, line := range []string{"typeset -pm 'p*'", "typeset +pm 'p*'"} {
		out, errs, st := declRun(t, "pa=1\nqq=2\n"+line, withMatching, Diagnostics{})
		want := "declare -- pa=\"1\"\n"
		if out != want || errs != "" || st != 0 {
			t.Errorf("%s = %q (stderr %q, status %d), want %q", line, out, errs, st, want)
		}
	}
}

// An assignment outranks `-p` in turn: the value reaches every match and the
// listing is only the shape of the answer. Reading `-p` first sent `p*=9` to
// the listing as a *name*, which matched nothing — and a listing asked for no
// names writes the whole table, so one operand wrote the table out and
// assigned nothing.
func TestMatchingUnderThePrintLetterStillTakesAnAssignment(t *testing.T) {
	for _, sign := range []string{"-", "+"} {
		t.Run(sign+"pm", func(t *testing.T) {
			src := "pa=1\npb=2\nqq=3\ntypeset " + sign + "pm 'p*'=9"
			out, errs, st := declRun(t, src, withMatching, Diagnostics{})
			want := "declare -- pa=\"9\"\ndeclare -- pb=\"9\"\n"
			if out != want || errs != "" || st != 0 {
				t.Errorf("typeset %spm 'p*'=9 = %q (stderr %q, status %d), want %q",
					sign, out, errs, st, want)
			}
		})
	}
}

// With no pattern at all the letter is *ignored*, under either sign: the bare
// listing is what the word writes, values and all. A reading that treated no
// patterns as no matches would answer with nothing.
func TestMatchingWithNoPatternIsIgnored(t *testing.T) {
	for _, line := range []string{"typeset -m", "typeset +m"} {
		out, errs, st := declRun(t, "pa=1\n"+line, withMatching, Diagnostics{})
		if errs != "" || st != 0 {
			t.Fatalf("%s: stderr %q, status %d", line, errs, st)
		}
		// The whole table with values, which is the bare word's listing —
		// the script's own name in it and the shell's parameters around it.
		// Not `pa` alone, and not the name-only shape `+m` writes when it
		// has a pattern to match.
		if !strings.Contains(out, "pa=\"1\"\n") || strings.Count(out, "\n") < 2 {
			t.Errorf("%s = %q, want the bare listing", line, out)
		}
	}
}

// The letter belongs to whichever builtins the dialect hands it to, and to no
// others: a dialect whose `local` is not given the letter refuses it there as
// an option it does not have, rather than accepting it as a pattern.
func TestTheMatchingLetterIsRefusedWhereTheDialectWithholdsIt(t *testing.T) {
	set := func(s *Semantics) {
		withMatching(s)
		s.LocalOptions = "ailrx"
	}
	_, errs, st := declRun(t, "f() { local -m 'p*'; }\nf", set, Diagnostics{})
	if st == 0 || errs == "" {
		t.Errorf("local -m = stderr %q status %d, want a refusal", errs, st)
	}
}

// The words a `+m` row is preceded by, and the order they come in. The order
// is a property of the listing form rather than of this letter — the same head
// the bare listing writes — but `+m` is where a caller sees it undiluted, so
// it is pinned from here. See interp/localbuiltin.go for where each seam was
// measured.
func TestMatchingWithAPlusWritesTheAttributeWordsInOrder(t *testing.T) {
	set := func(s *Semantics) {
		withMatching(s)
		s.DeclareOptions = "aAfgilmpruUxT"
	}
	for _, tc := range []struct{ name, decl, want string }{
		{"readonly before unique", "typeset -rU qa=(x y)", "array readonly unique qa\n"},
		{"exported before unique", "typeset -aU qa=(x y)\ntypeset -x qa", "array exported unique qa\n"},
		{"case before readonly", "typeset -lr qa=A", "lowercase readonly qa\n"},
		{"uppercase", "typeset -u qa=a", "uppercase qa\n"},
		{"association", "typeset -A qa=(k v)", "association qa\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.decl+"\ntypeset +m 'q*'", set, Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("+m after %q = %q (stderr %q, status %d), want %q",
					tc.decl, out, errs, st, tc.want)
			}
		})
	}
}

// An exported *scalar* at the top level carries no word, where an exported
// array or association does and so does any exported local. The three have to
// be tested together: one rule gives all three, and "exported always" — which
// is what this listing did before — put the word on most of the environment.
func TestMatchingWithAPlusNamesAnExportOnlyWhereItIsNotAPlainEnvironmentEntry(t *testing.T) {
	set := func(s *Semantics) {
		withMatching(s)
		s.DeclareOptions = "aAfgilmpruUxT"
		s.LocalOptions = "aAgilpruUx"
	}
	for _, tc := range []struct{ name, decl, want string }{
		{"a top-level scalar", "typeset -x qa=1", "qa\n"},
		{"a top-level array", "typeset -xa qa=(x)", "array exported qa\n"},
		{"a top-level association", "typeset -xA qa=(k v)", "association exported qa\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.decl+"\ntypeset +m 'q*'", set, Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("+m after %q = %q (stderr %q, status %d), want %q",
					tc.decl, out, errs, st, tc.want)
			}
		})
	}
	out, errs, st := declRun(t, "f() { local -x qa=1; typeset +m 'q*'; }\nf", set, Diagnostics{})
	if want := "local exported qa\n"; out != want || errs != "" || st != 0 {
		t.Errorf("+m over a local export = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The tie names the *other* half, from either end, and comes last of all the
// words. It is the one word that carries a second name with it.
func TestMatchingWithAPlusNamesTheOtherHalfOfATie(t *testing.T) {
	set := func(s *Semantics) {
		withMatching(s)
		s.DeclareOptions = "aAfgilmpruUxT"
	}
	out, errs, st := declRun(t, "typeset -T QS qs\ntypeset +m 'qs'\ntypeset +m 'QS'", set, Diagnostics{})
	if want := "array tied QS qs\ntied qs QS\n"; out != want || errs != "" || st != 0 {
		t.Errorf("+m over a tie = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The sign is read per *letter* and not per option word: a line whose `m` sits
// under a minus and whose attribute letter sits under a plus takes the
// attribute *off* the matches, where the same two letters in one plus word
// list them. A reading that asked which sign the last word carried would swap
// the two, and both rows would still pass on their own.
func TestMatchingReadsEachLettersSignAndNotTheLastWords(t *testing.T) {
	set := func(s *Semantics) {
		withMatching(s)
		s.DeclareOptions = "aAfgilmpruUxT"
	}
	src := "qa=1\ntypeset -x qa\n"
	out, errs, st := declRun(t, src+"typeset -m +x 'q*'\ntypeset -p qa", set, Diagnostics{})
	if want := "declare -- qa=\"1\"\n"; out != want || errs != "" || st != 0 {
		t.Errorf("typeset -m +x = %q (stderr %q, status %d), want %q — the attribute taken off",
			out, errs, st, want)
	}
	out, errs, st = declRun(t, src+"typeset +mx 'q*'\ntypeset -p qa", set, Diagnostics{})
	if want := "qa\ndeclare -x qa=\"1\"\n"; out != want || errs != "" || st != 0 {
		t.Errorf("typeset +mx = %q (stderr %q, status %d), want %q — a listing that changed nothing",
			out, errs, st, want)
	}
}

// A pattern *lists* a produced table and must not *declare* over one: there is
// nothing behind a name the shell makes up on each read for an attribute to
// change, and this engine marks such names readonly — so a declaration that
// swept them up answered with a read-only refusal for each, where the same
// line over a script's own names is silent.
func TestMatchingDeclaresOverNothingTheShellProduces(t *testing.T) {
	set := func(s *Semantics) {
		withMatching(s)
		s.DeclareOptions = "aAfgilmpruUxT"
	}
	// Both kinds of produced parameter, because the guard has to name both
	// and a test that knew only one would let the other through.
	before := func(r *Runner) {
		r.SetDynamicAssoc("qtable", func(*Runner) AssocArray { return AssocArray{"k": "v"} })
		r.MarkReadonly("qtable")
		r.SetDynamicArray("qlist", func(*Runner) []string { return []string{"x"} })
		r.MarkReadonly("qlist")
	}
	out, errs, st := declRunWith(t, "qa=1\ntypeset -mx 'q*'\ntypeset -p qa", set, Diagnostics{}, nil, before)
	if want := "declare -x qa=\"1\"\n"; out != want || errs != "" || st != 0 {
		t.Errorf("typeset -mx over produced tables = %q (stderr %q, status %d), want %q and no refusal",
			out, errs, st, want)
	}
	// The listing still reaches them, which is what makes the line above a
	// statement about declaring rather than about matching.
	out, errs, st = declRunWith(t, "typeset +m 'qtable' 'qlist'", set, Diagnostics{}, nil, before)
	if want := "association readonly qtable\narray readonly qlist\n"; out != want || errs != "" || st != 0 {
		t.Errorf("typeset +m over produced tables = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}
