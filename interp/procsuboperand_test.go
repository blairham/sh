// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// procsubOperand is the grammar where a `${…}` operand may carry a process
// substitution, and procsubPlain is the same grammar without that one answer.
func procsubOperand(d *syntax.Dialect) {
	d.ProcessSubstitutionInParamOperand = true
	d.PatternAlternation = true
}

func procsubPlain(d *syntax.Dialect) {
	d.PatternAlternation = true
}

// Whether `<(cmd)` opens a process substitution inside a `${…}` operand is a
// grammar question, and the panel answers it by position rather than by
// dialect: bash says yes, and ksh93, zsh and dash say no — none of them for
// want of the construct, since ksh93 and zsh have it in an ordinary word.
//
// Where the answer is no the characters are ordinary, so they are *pattern*
// text: `<(x)` is a `<` and, where the grammar has bare groups, the group
// `(x)`, which matches `<x`. Measured on zsh 5.9.2 and ksh93u+.
func TestAProcessSubstitutionInAParamOperandIsAGrammarQuestion(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"without it the text is a pattern", `v='<x'; printf "[%s]" "${v#<(x)}"`, "[]"},
		{"and the group takes alternatives", `v='<y'; printf "[%s]" "${v#<(x|y)}"`, "[]"},
		{"and more pattern may follow it", `v='<xy'; printf "[%s]" "${v#<(x)y}"`, "[]"},
		// The wrong answer this replaced: the substitution's *inner* text was
		// the pattern, so a bare `x` matched and the value came back trimmed.
		{"the inner text is never the pattern", `v=x; printf "[%s]" "${v#<(x)}"`, "[x]"},
		{"nor on the replacement side", `v=x; printf "[%s]" "${v/<(x)/Z}"`, "[x]"},
		{"nor for the output direction", `v=x; printf "[%s]" "${v#>(x)}"`, "[x]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, procsubPlain, tellTheRunner(procsubPlain))
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// With the grammar answer the other way the operand really is a substitution:
// the command runs and the operand is the path it made.
//
// Written **unquoted**, which is the whole of what the grammar flag reaches:
// re-measured 2026-09-07, `p=${nosuch:-<(:)}` is a path in bash 5.3.15 and
// 3.2.57 and `p="${nosuch:-<(:)}"` is the five characters, because a `${ }`
// body inside double quotes is double-quoted content and no process
// substitution is written there (#1150). The quoted spelling this row used to
// carry measured the wrong thing and agreed with the binary by accident, since
// the flag was the only reason it produced a path at all.
func TestAProcessSubstitutionInAParamOperandIsPerformedWhereTheGrammarHasOne(t *testing.T) {
	// `${u:-<(:)}` rather than a pattern operand, because the *value* is
	// what a path is visible in — a pattern operand's path simply fails to
	// match, which is true of the literal reading too.
	var r *Runner
	out, st := runGrammar(t, `printf "[%s]" ${nosuch:-<(:)}`, procsubOperand,
		func(rr *Runner) { r = rr })
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if strings.Contains(out, "<(") {
		t.Errorf("got %q, want the path rather than the text", out)
	}
	if !strings.HasPrefix(out, "[/dev/fd/") {
		t.Errorf("got %q, want the descriptor path the word expands to", out)
	}
	pipesMade(t, r, 1)
}

// And without it, the same operand is the five characters it was written as —
// as it also is *with* it once the expansion is quoted, which is the other way
// to reach the same text and a different reason for it.
func TestAProcessSubstitutionInAParamOperandIsTextWithoutTheGrammar(t *testing.T) {
	out, st := runGrammar(t, `printf "[%s]" ${nosuch:-<(:)}`, procsubPlain, tellTheRunner(procsubPlain))
	if want := "[<(:)]"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
	out, st = runGrammar(t, `printf "[%s]" "${nosuch:-<(:)}"`, procsubOperand, tellTheRunner(procsubOperand))
	if want := "[<(:)]"; out != want || st != 0 {
		t.Errorf("quoted, with the grammar: got %q (status %d), want %q at 0", out, st, want)
	}
}

// A condition's operand is a third answer again: one dialect performs it, and
// the rest refuse the word — in their own sentence, at status 2, and without
// having started the command.
func TestAProcessSubstitutionInAConditionIsAnAxis(t *testing.T) {
	t.Run("performed where the axis says so", func(t *testing.T) {
		tmp := t.TempDir()
		var r *Runner
		out, st := runGrammar(t, `[[ x == <(:) ]] && printf "[hit]" || printf "[miss]"`,
			procsubPlain, func(rr *Runner) {
				r = rr
				sem := testSemantics()
				sem.ProcessSubstitutionInCondition = Yes
				rr.Semantics = &sem
				rr.Env = append(withoutTMPDIR(rr.Env), "TMPDIR="+tmp)
			})
		if want := "[miss]"; out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
		pipesMade(t, r, 1)
	})

	t.Run("refused where it does not, and the command never runs", func(t *testing.T) {
		var r *Runner
		out, st := runGrammar(t, `[[ x == <(:) ]] && printf "[hit]" || printf "[miss]"; printf "[after]"`,
			procsubPlain, func(rr *Runner) {
				r = rr
				sem := testSemantics()
				sem.ProcessSubstitutionInCondition = No
				sem.FatalErrorStatusIsOne = No
				d := Diagnostics{ProcessSubstitutionNotInCondition: "no substitution here: %[1]s"}
				rr.Semantics, rr.Diagnostics = &sem, &d
			})
		// The whole line is abandoned: neither arm of the `||` runs and
		// neither does what came after it. Measured — the shell that refuses
		// this prints its sentence and stops reading.
		if want := "sh: no substitution here: <(:)\n"; out != want {
			t.Errorf("output = %q, want %q", out, want)
		}
		if st != 2 {
			t.Errorf("status %d, want 2", st)
		}
		// The half that needed rewriting most. This used to glob the shell's
		// TMPDIR and want it empty, and a shell that cleans up after itself
		// leaves it empty whether or not it ran the substitution — so the
		// assertion would have gone on passing against exactly the shell it
		// is here to forbid. What the shell made is counted rather than
		// looked for, and a shell that made nothing has nothing to count.
		pipesMade(t, r, 0)
	})

	t.Run("and an unanswered axis is refused by name", func(t *testing.T) {
		out, st := runGrammar(t, `[[ x == <(:) ]]; printf "[st=%d]" "$?"`,
			procsubPlain, func(r *Runner) {
				sem := CoreSemantics()
				r.Semantics = &sem
			})
		want := "sh: a process substitution standing as a condition's operand: " +
			"the shells disagree here and no dialect was chosen\n[st=2]"
		if out != want {
			t.Errorf("output = %q, want %q", out, want)
		}
		if st != 0 {
			t.Errorf("status %d, want 0 — the printf after it is what ran last", st)
		}
	})
}

// tellTheRunner hands the same grammar to the Runner that the parser was
// given. The parser decides what the source *is* and the runner decides what
// a pattern means, so a test that turns on bare groups and tells only the
// parser asserts the core's answer with the dialect's source (#849).
func tellTheRunner(enable func(*syntax.Dialect)) func(*Runner) {
	return func(r *Runner) {
		d := syntax.Core()
		enable(&d)
		r.Dialect = &d
	}
}

// TestTheConditionRefusalReachesEveryOperand is #3280. The axis is asked at
// every operand of every operator and not only at the one a comparison holds:
// a shell that refuses a process substitution in a condition refuses it in a
// file test, in a bare word and on either side of any operator, and the
// command is never started in any of them.
//
// The status it leaves is a property of the **expression** rather than of the
// word the sentence names, which is the second half and is what the last two
// rows pin: the same refusal, the same word named, and a different number
// behind it because of what stands on the other side of the operator.
func TestTheConditionRefusalReachesEveryOperand(t *testing.T) {
	refusing := func(rr *Runner) {
		sem := testSemantics()
		sem.ProcessSubstitutionInCondition = No
		sem.FatalErrorStatusIsOne = No
		d := Diagnostics{ProcessSubstitutionNotInCondition: "no substitution here: %[1]s"}
		rr.Semantics, rr.Diagnostics = &sem, &d
	}
	for _, tc := range []struct {
		src    string
		named  string
		status int
		why    string
	}{
		// Every operand, and the command started in none of them.
		{`[[ -e <(:) ]]`, "<(:)", 1, "a file test's operand"},
		{`[[ -n <(:) ]]`, "<(:)", 1, "a string test's"},
		{`[[ <(:) ]]`, "<(:)", 1, "a bare word, which is the same test written short"},
		{`[[ <(:) == x ]]`, "<(:)", 1, "the left side of a comparison"},
		{`[[ x -nt <(:) ]]`, "<(:)", 1, "the right side of one that is not a pattern"},
		{`[[ ! -e <(:) ]]`, "<(:)", 1, "under a negation"},
		{`[[ ( -e <(:) ) ]]`, "<(:)", 1, "inside a group"},
		// The pattern comparison's right side, which is the one shape whose
		// status differs — and only for the input spelling.
		{`[[ x == <(:) ]]`, "<(:)", 2, "the right side of a pattern comparison"},
		{`[[ x != <(:) ]]`, "<(:)", 2, "which the negated spelling is too"},
		{`[[ x == >(:) ]]`, ">(:)", 1, "where the output spelling is not"},
		// The file spelling is one grammar's alone and is asserted in that
		// dialect rather than here, the grammar this test runs on having
		// only the two.
		// And the pair that says the status belongs to the expression: both
		// refuse the **left** word by name and differ only in what stands
		// behind the operator.
		{`[[ <(:) == <(:) ]]`, "<(:)", 2, "an input substitution behind the operator"},
		{`[[ <(:) == >(:) ]]`, "<(:)", 1, "and one of the other spelling there"},
	} {
		var r *Runner
		out, st := runGrammar(t, tc.src+`; printf "[after]"`, procsubPlain, func(rr *Runner) {
			r = rr
			refusing(rr)
		})
		want := "sh: no substitution here: " + tc.named + "\n"
		if out != want {
			t.Errorf("%s: output = %q, want %q — %s", tc.src, out, want, tc.why)
		}
		if st != tc.status {
			t.Errorf("%s: status %d, want %d — %s", tc.src, st, tc.status, tc.why)
		}
		pipesMade(t, r, 0)
	}
}

// And the refusal is lazy, because the condition is: a short-circuit that
// never reaches the operand never refuses it either.
func TestAShortCircuitStillHidesARefusedSubstitution(t *testing.T) {
	var r *Runner
	out, st := runGrammar(t, `[[ x == y && -e <(:) ]]; printf "[st=%d]" "$?"`,
		procsubPlain, func(rr *Runner) {
			r = rr
			sem := testSemantics()
			sem.ProcessSubstitutionInCondition = No
			sem.FatalErrorStatusIsOne = No
			d := Diagnostics{ProcessSubstitutionNotInCondition: "no substitution here: %[1]s"}
			rr.Semantics, rr.Diagnostics = &sem, &d
		})
	if want := "[st=1]"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
	pipesMade(t, r, 0)
}
