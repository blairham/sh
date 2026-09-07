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
