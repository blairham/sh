// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// flagged turns on the grammar the `%` flag needs, by the construct's name.
func promptFlagged(d *syntax.Dialect) { d.ParamExpansionFlags = true }

// `%n` is the user this runner was told about, wherever it appears in the word.
func TestThePromptUserEscapeExpands(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"alone", `echo "[${(%):-%n}]"`, "[someone]\n"},
		{"with text around it", `echo "[${(%):-abc%ndef}]"`, "[abcsomeonedef]\n"},
		{"twice in one word", `echo "[${(%):-%n@%n}]"`, "[someone@someone]\n"},
		{"and a literal percent is still literal", `echo "[${(%):-%%n}]"`, "[%n]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runGrammar(t, tc.src, promptFlagged, func(r *Runner) {
				r.SetPromptStyle(promptEscapeTable())
				r.SetPromptUser("someone")
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// It is not read out of a variable, which is the fact that decides how it may
// be implemented.
//
// Measured on zsh 5.9.2 and on bash's `\u`: `%n` ignores `USER`, `LOGNAME` and
// `USERNAME`, assigned inside the shell or injected into the environment
// before it starts. An implementation that read one of them would make
// `env USER=someone-else zsh` draw the wrong person — so the runner is *told*
// who it is, and the variables are left standing where the script put them.
func TestThePromptUserEscapeIsNotAVariable(t *testing.T) {
	const src = `USER=impostor; LOGNAME=impostor; USERNAME=impostor; echo "[${(%):-%n}][$USER]"`
	out, _ := runGrammar(t, src, promptFlagged, func(r *Runner) {
		r.SetPromptStyle(promptEscapeTable())
		r.SetPromptUser("someone")
	})
	if want := "[someone][impostor]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A runner nobody told refuses the escape by name rather than expanding it to
// nothing.
//
// This is the row the first attempt failed. The refusal was written as a
// `break` out of the empty case, meaning to fall into the default — but a
// `break` inside a Go switch leaves the switch, so the escape silently
// produced an empty string and status 0. A wrong answer wearing a success is
// exactly what the construct's diagnostic exists to prevent, and only an
// assertion on the whole rendered line catches it: the *output* of the silent
// version and of a correct empty answer are the same string.
func TestThePromptUserEscapeIsRefusedWhenNobodyTold(t *testing.T) {
	// A second command on the line, to say that the refusal *abandons* the
	// list rather than only failing the one word: nothing after it runs, so
	// the whole assertion is the one diagnostic line and no more.
	const src = `echo "[${(%):-%n}]"; echo reached`
	out, st := runGrammar(t, src, promptFlagged, func(r *Runner) {
		r.SetPromptStyle(promptEscapeTable())
	})
	want := "sh: ${(%):-%n}: the %n prompt escape is not implemented\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st == 0 {
		t.Errorf("status = 0, want a failure — a refusal reported as success is the thing this guards")
	}
}

// An escape this shell does not carry is still refused by name, and the name
// is the escape the script wrote rather than the first one in the word.
func TestAnUnknownPromptEscapeIsRefusedByName(t *testing.T) {
	const src = `echo "[${(%):-%n%q}]"; echo reached`
	out, st := runGrammar(t, src, promptFlagged, func(r *Runner) {
		r.SetPromptStyle(promptEscapeTable())
		r.SetPromptUser("someone")
	})
	want := "sh: ${(%):-%n%q}: the %q prompt escape is not implemented\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st == 0 {
		t.Errorf("status = 0, want a failure")
	}
}

// The escapes that were already carried still are, so the new case did not
// displace them.
func TestThePromptEscapesAlreadyCarriedStillAre(t *testing.T) {
	out, _ := runGrammar(t, `echo "[${(%):-%%}]"`, promptFlagged, func(r *Runner) {
		r.SetPromptStyle(promptEscapeTable())
	})
	if want := "[%]\n"; out != want {
		t.Errorf("literal percent: got %q, want %q", out, want)
	}
}

// `%m` and `%M` are the machine, and a runner nobody told refuses them by
// name rather than drawing a prompt that describes no machine.
//
// The same rule `%n` follows and for the same reason: the host name comes
// from the system, and a Runner embedded in another program must not go
// asking. Both halves are asserted — told, they answer; untold, they refuse —
// because either alone passes with the condition ignored in one direction.
func TestThePromptHostIsToldOrRefused(t *testing.T) {
	told := func(r *Runner) {
		r.SetPromptStyle(promptHostTable())
		r.SetPromptHost("box.example.test")
	}
	out, st := runGrammar(t, `echo "[${(%):-%m}][${(%):-%M}]"`, promptFlagged, told)
	if want := "[box][box.example.test]\n"; out != want || st != 0 {
		t.Errorf("told = %q (status %d), want %q at 0", out, st, want)
	}
	untold := func(r *Runner) { r.SetPromptStyle(promptHostTable()) }
	out, st = runGrammar(t, `echo "[${(%):-%m}]"; echo reached`, promptFlagged, untold)
	if want := "sh: ${(%):-%m}: the %m prompt escape is not implemented\n"; out != want {
		t.Errorf("untold = %q, want %q", out, want)
	}
	if st == 0 {
		t.Error("status = 0, want a failure — a refusal reported as success is the thing this guards")
	}
	out, st = runGrammar(t, `echo "[${(%):-%M}]"; echo reached`, promptFlagged, untold)
	if want := "sh: ${(%):-%M}: the %M prompt escape is not implemented\n"; out != want || st == 0 {
		t.Errorf("untold, long form = %q (status %d), want %q at a failure", out, st, want)
	}
}

// promptHostTable is the two host rows and nothing else.
func promptHostTable() PromptStyle {
	return PromptStyle{
		Escape: '%',
		Codes:  map[rune]PromptField{'m': FieldHost, 'M': FieldHostFull},
	}
}

// Told no table at all, the flag transforms nothing.
//
// The zero value of the whole table, and the substrate's own answer rather
// than a borrowed one: with no escape character there is no escape language,
// so there is nothing to refuse by name either. Reachable only through this
// package's API — the one dialect whose grammar reads the flag supplies the
// table in its Apply — and asserted because the alternative reading, an
// escape language with no rows in it, would refuse every `%` in the word.
func TestThePercentFlagWithNoTableTransformsNothing(t *testing.T) {
	out, st := runGrammar(t, `echo "[${(%):-%n%%x}]"`, promptFlagged, nil)
	if want := "[%n%%x]\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// The question is not asked until the escape is drawn, which is the whole of
// #1403's zsh column.
//
// Whoever is allowed to ask the system who the user is may find that
// expensive — measured at 0.83-1.10 ms for one such answer — and a shell
// running `-c` never draws a prompt at all. So the runner is handed a
// *question* rather than an answer, and this is the assertion that keeps it
// one: an implementation that called the function while installing it, or from
// anywhere on the way to running a command, would pass every other test in
// this file and put the cost straight back.
//
// A counter rather than a flag, because "asked once" and "asked at all" are
// different failures and the next test needs the count anyway.
func TestThePromptUserIsNotAskedUntilTheEscapeIsDrawn(t *testing.T) {
	var asked int
	// A whole command runs, and nothing in it mentions the escape.
	out, _ := runGrammar(t, `echo reached`, promptFlagged, func(r *Runner) {
		r.SetPromptStyle(promptEscapeTable())
		r.SetPromptUserFunc(func() string {
			asked++
			return "someone"
		})
	})
	if out != "reached\n" {
		t.Fatalf("got %q, want %q", out, "reached\n")
	}
	if asked != 0 {
		t.Errorf("the login name was asked for %d times by a script that never drew %%n; want 0 — installing the question must not ask it", asked)
	}
}

// And it is asked once however many times it is drawn.
//
// A prompt is redrawn on every keystroke that redraws the line, so a question
// asked per draw would move the cost from startup to typing rather than
// removing it. Three draws in one word, so a per-word memo would pass and a
// per-escape one would not.
func TestThePromptUserIsAskedOnlyOnce(t *testing.T) {
	var asked int
	out, _ := runGrammar(t, `echo "[${(%):-%n@%n}]"; echo "[${(%):-%n}]"`, promptFlagged, func(r *Runner) {
		r.SetPromptStyle(promptEscapeTable())
		r.SetPromptUserFunc(func() string {
			asked++
			return "someone"
		})
	})
	if want := "[someone@someone]\n[someone]\n"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	if asked != 1 {
		t.Errorf("the login name was asked for %d times across three draws; want 1", asked)
	}
}

// A question that answers nothing is a different fact from never being asked,
// and is answered with the dialect's word for it.
//
// The two were one empty string until #1451 and both took the refusal above.
// That reads right and is a measurement of nothing: a uid with no
// password-database entry is a real state of a real machine — a container
// started `--user 99999` — and the panel has an answer for it. Measured
// 2026-09-12 at uid 99999, bash draws `I have no name!` for `\u` and zsh draws
// nothing at all for `%n`, both at status 0. So this is PromptStyle's to say
// and not the mechanism's, and a shell that refused here would refuse a prompt
// that both real shells draw.
//
// Both halves in one test because what is being claimed is that the answer
// comes from the table: a shell whose field was hard-wired to either sentence
// would pass one row and fail the other.
func TestAPromptUserQuestionThatAnswersNothingDrawsTheDialectsWord(t *testing.T) {
	for _, tc := range []struct {
		name, noLoginName, want string
	}{
		{"a dialect with words for it", "I have no name!", "[I have no name!]\nreached\n"},
		{"a dialect with none", "", "[]\nreached\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const src = `echo "[${(%):-%n}]"; echo reached`
			out, st := runGrammar(t, src, promptFlagged, func(r *Runner) {
				table := promptEscapeTable()
				table.NoLoginName = tc.noLoginName
				r.SetPromptStyle(table)
				r.SetPromptUserFunc(func() string { return "" })
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0 — the shell drew what it had to draw", st)
			}
		})
	}
}

// And the refusal is still there for the other empty, which is what makes the
// two separate answers worth having.
//
// The counter-case to the test above, and the one that says #1451 un-collapsed
// the pair rather than dropping the refusal: a Runner nobody told is a gap in
// how it was set up, not a fact about the machine, and drawing a dialect's
// no-name sentence for it would be inventing a measurement. Asserted with the
// same table that draws the sentence above, so only the *question* differs.
func TestNobodyTellingTheRunnerIsStillARefusalEvenWithAWordForNoName(t *testing.T) {
	const src = `echo "[${(%):-%n}]"; echo reached`
	out, st := runGrammar(t, src, promptFlagged, func(r *Runner) {
		table := promptEscapeTable()
		table.NoLoginName = "I have no name!"
		r.SetPromptStyle(table)
	})
	want := "sh: ${(%):-%n}: the %n prompt escape is not implemented\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st == 0 {
		t.Errorf("status = 0, want a failure")
	}
}

// The lazy form and the eager one answer identically, including the shortening
// `%m` does and the full name `%M` keeps.
//
// Asserted together rather than in two tests, because what is being claimed is
// that the pair is one behavior reached two ways — and a claim about a pair
// that is checked one half at a time is how the host escapes came to have an
// eager half and a lazy one.
func TestTheLazyPromptIdentityAnswersLikeTheEagerOne(t *testing.T) {
	const src = `echo "[${(%):-%n|%m|%M}]"`
	const want = "[someone|machine|machine.example]\n"
	// A table with all three rows, since the two shared ones each carry
	// half of what this asserts about and the claim is about the pair.
	table := func() PromptStyle {
		return PromptStyle{
			Escape: '%',
			Codes:  map[rune]PromptField{'n': FieldUser, 'm': FieldHost, 'M': FieldHostFull},
		}
	}
	eager, _ := runGrammar(t, src, promptFlagged, func(r *Runner) {
		r.SetPromptStyle(table())
		r.SetPromptUser("someone")
		r.SetPromptHost("machine.example")
	})
	lazy, _ := runGrammar(t, src, promptFlagged, func(r *Runner) {
		r.SetPromptStyle(table())
		r.SetPromptUserFunc(func() string { return "someone" })
		r.SetPromptHostFunc(func() string { return "machine.example" })
	})
	if eager != want {
		t.Errorf("told: got %q, want %q", eager, want)
	}
	if lazy != eager {
		t.Errorf("asked: got %q, but being told gave %q — the two forms must be one behavior", lazy, eager)
	}
}

// The machine's name is asked on the same terms, and this is the test that
// says so rather than assuming the pair moved together.
func TestThePromptHostIsNotAskedUntilTheEscapeIsDrawn(t *testing.T) {
	var asked int
	out, _ := runGrammar(t, `echo reached`, promptFlagged, func(r *Runner) {
		r.SetPromptStyle(promptHostTable())
		r.SetPromptHostFunc(func() string {
			asked++
			return "machine.example"
		})
	})
	if out != "reached\n" {
		t.Fatalf("got %q, want %q", out, "reached\n")
	}
	if asked != 0 {
		t.Errorf("the host name was asked for %d times by a script that never drew %%m; want 0", asked)
	}
}
