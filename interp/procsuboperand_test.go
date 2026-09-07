// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"path/filepath"
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
	tmp := t.TempDir()
	// `${u:-<(:)}` rather than a pattern operand, because the *value* is
	// what a path is visible in — a pattern operand's path simply fails to
	// match, which is true of the literal reading too.
	out, st := runGrammar(t, `printf "[%s]" ${nosuch:-<(:)}`, procsubOperand,
		func(r *Runner) { r.Env = append(withoutTMPDIR(r.Env), "TMPDIR="+tmp) })
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if strings.Contains(out, "<(") {
		t.Errorf("got %q, want the path rather than the text", out)
	}
	if !strings.Contains(out, tmp) {
		t.Errorf("got %q, want a path under the scratch directory %q", out, tmp)
	}
	made, err := filepath.Glob(filepath.Join(tmp, "sh-procsub*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(made) == 0 {
		t.Error("no process substitution ran")
	}
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
		out, st := runGrammar(t, `[[ x == <(:) ]] && printf "[hit]" || printf "[miss]"`,
			procsubPlain, func(r *Runner) {
				sem := testSemantics()
				sem.ProcessSubstitutionInCondition = Yes
				r.Semantics = &sem
				r.Env = append(withoutTMPDIR(r.Env), "TMPDIR="+tmp)
			})
		if want := "[miss]"; out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
		made, _ := filepath.Glob(filepath.Join(tmp, "sh-procsub*"))
		if len(made) == 0 {
			t.Error("no process substitution ran")
		}
	})

	t.Run("refused where it does not, and the command never runs", func(t *testing.T) {
		tmp := t.TempDir()
		out, st := runGrammar(t, `[[ x == <(:) ]] && printf "[hit]" || printf "[miss]"; printf "[after]"`,
			procsubPlain, func(r *Runner) {
				sem := testSemantics()
				sem.ProcessSubstitutionInCondition = No
				sem.FatalErrorStatusIsOne = No
				d := Diagnostics{ProcessSubstitutionNotInCondition: "no substitution here: %[1]s"}
				r.Semantics, r.Diagnostics = &sem, &d
				r.Env = append(withoutTMPDIR(r.Env), "TMPDIR="+tmp)
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
		made, _ := filepath.Glob(filepath.Join(tmp, "sh-procsub*"))
		if len(made) != 0 {
			t.Errorf("the command ran before the refusal: %v", made)
		}
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
