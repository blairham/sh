// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Brace expansion copies the **names**; whether it copies the **work** with
// them is [Semantics.BraceFanExpandsEachNameOnItsOwn]. Every name a fan makes
// carries the same `$( … )` and the same `$(( … ))` as its siblings, so
// expanding each on its own runs all of it once per name.
//
// The rule is keyed on the **fan** and not on where the word stands. The same
// doubling was in a redirection target and in an ordinary argument, and the
// argument is the control that says so — which is why both roads go through
// one helper here. The trap this seam keeps setting is the neighboring noun:
// "a word that comes to several words" is not it, because a word split into
// several by anything *other* than braces is expanded once under both
// readings.
//
// Measured 2026-09-26, `-c`, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// each case in a directory of its own, against `/opt/homebrew/bin/bash`
// 5.3.20, `/bin/bash` 3.2.57, `/opt/homebrew/bin/zsh -f` 5.9.2, `/bin/ksh`
// 93u+ 2012-08-01 and `/bin/dash` 0.5.12 — `go version -m` says *not a Go
// executable* for each — and BusyBox 1.37.0 in the pinned alpine image:
//
//	i=0; echo {x,y,w}$((i++)); echo "i=$i"
//	bash 5.3.20   x0 y1 w2   i=3
//	bash 3.2.57   x0 y1 w2   i=3
//	zsh 5.9.2     x0 y0 w0   i=1
//	ksh93u+       x0 y0 w0   i=1
//
//	echo {x,y}$(echo TICK >&2; echo z)   TICK lines   words
//	bash 5.3.20                                   2   xz yz
//	zsh 5.9.2                                     1   xz yz
//	ksh93u+                                       1   xz yz
//	dash 0.5.12                                   1   {x,y}z
//	BusyBox ash 1.37.0                            1   {x,y}z
//
// The **words agree in every column**, which is why this is counted rather
// than read: a doubled expansion produces the value a single one produces,
// and only the count and the variable it moved say it ran twice (#4694).

// fanSemantics is a vector with braces on, at the given answer for the axis
// under test, and nothing else that could decide these rows.
func fanSemantics(t *testing.T, each Answer) func(*Runner) {
	t.Helper()
	return func(r *Runner) {
		sem := permissive()
		sem.BraceExpansion = Yes
		sem.BraceFanExpandsEachNameOnItsOwn = each
		sem.BraceOutputRereadAsText = No
		sem.BraceRescanEntersFailedGroup = Yes
		sem.SplitParamExpansion = Yes
		sem.SplitCommandSubstitution = Yes
		sem.GlobExpansionResults = Yes
		sem.GlobNoMatchIsError = No
		sem.RedirectTargetIsAnOrdinaryWord = No
		sem.RedirectTargetTakesPathnameExpansion = Yes
		sem.RedirectsUseEveryTarget = Yes
		sem.FailedExpansionAbandonsTheLine = No
		sem.FatalErrorStatusIsOne = Yes
		r.Semantics, r.Dir = &sem, t.TempDir()
	}
}

// countTicks is why these rows are counted at all: the two readings produce
// the same words, so a comparison of the words cannot tell them apart.
func countTicks(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "TICK" {
			n++
		}
	}
	return n
}

const tickSubst = `$(echo TICK >&2; echo z)`

// The axis, both ways, on the shape that shows it as a *side effect* rather
// than as a count: `i` is moved once per name under one reading and once for
// the word under the other, and the value substituted moves with it.
func TestABraceFanMovesAVariableOncePerNameOrOnceForTheWord(t *testing.T) {
	for _, tc := range []struct {
		name string
		each Answer
		want string
	}{
		{"each name expands the word again", Yes, "x0 y1 w2\ni=3\n"},
		{"the names share one expansion", No, "x0 y0 w0\ni=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, "i=0\necho {x,y,w}$((i++))\necho \"i=$i\"\n", fanSemantics(t, tc.each))
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// And on the shape that shows it as a *run*: the words are `xz yz` under both
// readings, and the count on standard error is the whole of the difference.
func TestABraceFanRunsASubstitutionOncePerNameOrOnceForTheWord(t *testing.T) {
	for _, tc := range []struct {
		name  string
		each  Answer
		ticks int
	}{
		{"each name expands the word again", Yes, 2},
		{"the names share one expansion", No, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, "echo {x,y}"+tickSubst+"\n", fanSemantics(t, tc.each))
			if n := countTicks(out); n != tc.ticks {
				t.Errorf("out = %q: %d runs, want %d", out, n, tc.ticks)
			}
			// The words, asserted beside the count so that a reading which
			// shared the *wrong* value cannot pass as a reading that shared.
			if !strings.Contains(out, "xz yz") {
				t.Errorf("out = %q, want the two names `xz yz` either way", out)
			}
		})
	}
}

// The noun, held fixed. Both rows below come to two words and run one
// substitution, and only the braced one is a fan — so a rule keyed on "a word
// that comes to several words" gets the second row wrong under either answer.
//
// This is the pair the campaign's wrong-noun trap is made of: braces and
// several-words agree on every row unless something *else* makes the several.
func TestSeveralWordsWithoutBracesIsNotAFan(t *testing.T) {
	for _, each := range []Answer{Yes, No} {
		t.Run("each="+answerName(each), func(t *testing.T) {
			out, _ := run(t, "e='p q'\necho $e"+tickSubst+"\n", fanSemantics(t, each))
			if n := countTicks(out); n != 1 {
				t.Errorf("out = %q: %d runs, want 1 — splitting is not a fan", out, n)
			}
			// The positive that makes the row falsifiable: the same
			// substitution in a real fan does move with the answer.
			braced, _ := run(t, "echo {p,q}"+tickSubst+"\n", fanSemantics(t, each))
			want := 1
			if each == Yes {
				want = 2
			}
			if n := countTicks(braced); n != want {
				t.Errorf("braced = %q: %d runs, want %d — the probe cannot fire", braced, n, want)
			}
		})
	}
}

// A fan of one name is not a fan, and a fan whose word held nothing for the
// names to share does not reach the question either. Both rows run with the
// axis **unanswered**, which is what says they do not ask it: an axis that was
// consulted would refuse the script by name.
func TestAFanWithNothingToShareDoesNotAskTheAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"one name", "echo {x}" + tickSubst + "\n", "{x}z\n"},
		{"literals only", "echo {x,y}tail\n", "xtail ytail\n"},
		{"a parameter the names only read", "v=V\necho {x,y}$v\n", "xV yV\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, fanSemantics(t, Unspecified))
			clean := strings.ReplaceAll(out, "TICK\n", "")
			if clean != tc.want || st != 0 {
				t.Errorf("out = %q (status %d), want %q at 0 with nothing refused", out, st, tc.want)
			}
		})
	}
}

// And the sharpest of that family, which is a fan whose word *does* run
// something: the substitution lives in **one alternative**, so no second name
// carries it and both readings run it once. Asking between names rather than
// at the moment a span is found twice refused this one over a disagreement it
// does not have.
func TestAnExpansionInOneAlternativeDoesNotAskTheAxis(t *testing.T) {
	src := "f() { echo ran >&2; echo 3; }\nprintf '[%s]' {1..$(f),5}\n"
	out, st := run(t, src, func(r *Runner) {
		fanSemantics(t, Unspecified)(r)
		sem := *r.Semantics
		sem.BraceRangeEndpointsExpanded = Yes
		r.Semantics = &sem
	})
	if n := strings.Count(out, "ran"); n != 1 {
		t.Errorf("out = %q: %d runs, want 1", out, n)
	}
	if !strings.Contains(out, "[1..3][5]") || st != 0 {
		t.Errorf("out = %q (status %d), want the names at 0 with nothing refused", out, st)
	}
}

// And the row that makes the four above falsifiable: a fan where a second name
// really does carry the same span asks, and an unanswered vector refuses the
// script rather than picking a reading.
func TestAFanWithWorkToShareRefusesAnUnansweredVector(t *testing.T) {
	out, st := run(t, "echo {x,y}"+tickSubst+"\n", fanSemantics(t, Unspecified))
	if !strings.Contains(out, "brace fan") {
		t.Errorf("out = %q, want the axis named", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2 — an unanswered axis", st)
	}
	if strings.Contains(out, "xz yz") {
		t.Errorf("out = %q, want the command left unrun", out)
	}
}

// A failure ends the fan, and that is **core**: it is the same in bash
// 5.3.20, bash 3.2.57, zsh 5.9.2 and ksh93u+, each of which answers the axis
// its own way. Counted rather than contained, because the bug was a *second*
// sentence after the first.
func TestAFailedExpansionEndsTheFanUnderEitherAnswer(t *testing.T) {
	for _, each := range []Answer{Yes, No} {
		t.Run("each="+answerName(each), func(t *testing.T) {
			out, _ := run(t, "echo {x,y,w}$(( 1/0 ))\n", fanSemantics(t, each))
			if n := strings.Count(out, "division by zero"); n != 1 {
				t.Errorf("out = %q: %d diagnostics, want exactly 1", out, n)
			}
		})
	}
}

// Every shape a fan comes in, because the doubling was in the fan and reached
// all of them: two alternatives, three, a nested group, a range, an empty
// alternative, and two groups multiplying. One run of the word under the
// sharing reading, however many names come out.
func TestEveryShapeOfFanSharesOneRunOfTheWord(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"two alternatives", "echo {x,y}" + tickSubst},
		{"three alternatives", "echo {x,y,w}" + tickSubst},
		{"a nested group", "echo {a,{b,c}}" + tickSubst},
		{"a range", "echo {1..3}" + tickSubst},
		{"an empty alternative", "echo {,x}" + tickSubst},
		{"two groups", "echo {a,b}{c,d}" + tickSubst},
		{"the group behind the substitution", "echo " + tickSubst + "{x,y}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := fanSemantics(t, No)
			out, _ := run(t, tc.src+"\n", sem)
			if n := countTicks(out); n != 1 {
				t.Errorf("out = %q: %d runs, want 1", out, n)
			}
			// The positive beside it: the other reading really does repeat,
			// so a zero here would be a dead probe rather than a shared run.
			each, _ := run(t, tc.src+"\n", fanSemantics(t, Yes))
			if n := countTicks(each); n < 2 {
				t.Errorf("each-name = %q: %d runs, want more than 1 — the probe cannot fire", each, n)
			}
		})
	}
}

// The redirection target is the same fan by another road, which is the whole
// reason there is one helper: `: > {x,y}$(f)` names two files from one run of
// `f` under the sharing reading, and the files are the same either way.
func TestARedirectionTargetsFanSharesOneRunOfTheWord(t *testing.T) {
	for _, tc := range []struct {
		name  string
		each  Answer
		ticks int
	}{
		{"each name expands the word again", Yes, 2},
		{"the names share one expansion", No, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := fanSemantics(t, tc.each)
			out, st := run(t, ": > {x,y}"+tickSubst+"\nls\n", sem)
			if n := countTicks(out); n != tc.ticks {
				t.Errorf("out = %q: %d runs, want %d", out, n, tc.ticks)
			}
			// The files, asserted beside the count: sharing the run must not
			// cost a name. `ls` is the core's own builtin here.
			if !strings.Contains(out, "xz") || !strings.Contains(out, "yz") {
				t.Errorf("out = %q (status %d), want both names opened", out, st)
			}
		})
	}
}

// And every operator that takes a target, because the target road reached all
// of them. A here-string is deliberately absent: its word is a *body* and
// goes through another door.
func TestEveryRedirectionOperatorsFanSharesOneRunOfTheWord(t *testing.T) {
	for _, op := range []string{`>`, `>>`, `<>`, `2>`, `&>`} {
		t.Run(op, func(t *testing.T) {
			out, _ := run(t, ": "+op+" {x,y}"+tickSubst+"\n", fanSemantics(t, No))
			if n := countTicks(out); n != 1 {
				t.Errorf("out = %q: %d runs, want 1", out, n)
			}
		})
	}
	t.Run("exec", func(t *testing.T) {
		out, _ := run(t, "exec 3> {x,y}"+tickSubst+"\n", fanSemantics(t, No))
		if n := countTicks(out); n != 1 {
			t.Errorf("out = %q: %d runs, want 1", out, n)
		}
	})
}

// A substitution's body is a **program of its own**, so the hold must not
// answer for a span inside it: the body is parsed separately and its spans sit
// at offsets into *its* text, where a `$( … )` can land on the very position
// one in this script occupies.
//
// The case below is that collision, built rather than hoped for. The first
// `$( … )` of the word starts at offset 6 of the script and the `$( … )` in
// the `${ … ;}` body that follows it starts at offset 6 of the body — so a
// hold left standing over the body hands the body's substitution the value the
// word's already produced, and `B` never runs at all.
//
// Measured on `/bin/ksh` 93u+ 2012-08-01, which has the `${ … ;}` spelling and
// is one of the two columns that share the run: `Xaabbx Xaabby`. With the hold
// left standing this shell writes `Xaaaax Xaaaay`. The one `X` of padding is
// what aligns the two offsets — four neighboring lengths do not collide and
// say nothing, which is why it is written out rather than hoped for.
func TestTheFansHoldDoesNotReachIntoASubstitutionsOwnBody(t *testing.T) {
	src := "echo X$(echo A >&2; echo aa)${ echo $(echo B >&2; echo bb);}{x,y}\n"
	out, _ := runGrammar(t, src, func(d *syntax.Dialect) {
		d.CurrentShellSubstitution = true
	}, fanSemantics(t, No))
	if !strings.Contains(out, "Xaabbx Xaabby") {
		t.Errorf("out = %q, want `Xaabbx Xaabby` — the body took the word's value", out)
	}
	// And each of the two ran once, which is what says the names still share.
	for _, mark := range []string{"A", "B"} {
		if n := strings.Count(out, mark+"\n"); n != 1 {
			t.Errorf("out = %q: %s ran %d times, want 1", out, mark, n)
		}
	}
}

// answerName is for subtest names, since Answer has no spelling of its own
// that reads well in one.
func answerName(a Answer) string {
	switch a {
	case Yes:
		return "yes"
	case No:
		return "no"
	}
	return "unanswered"
}
