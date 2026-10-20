// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// A single quote written inside a double-quoted `${ … }` protects the closing
// brace in **every** operand here, which no other panel column does in a word
// operand. bash 3.2.57 answers the same.
//
// The substrate's tests name the flag — `QuoteProtectsTheClosingBrace` — and
// this one names the shell, which is the only place that is allowed: the core
// protects a pattern operand alone, so the wider reading belongs to a preset.
func TestAQuoteProtectsTheClosingBraceInEveryOperand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The headline: one expansion running to the second brace, with
			// nothing of the operand left in the enclosing word.
			"a word operand",
			`v=Vx}y; printf '[%s]' "${v-'a}b'}" "${u-'a}b'}"`,
			`[Vx}y]['a}b']`,
		},
		{
			// A pattern operand agrees with the other five columns, so this
			// row is what keeps the flag from reading as "bash is different
			// about quotes in expansions".
			"a pattern operand",
			`s=a}b; printf '[%s]' "${s#'a}'}" "${s%'}b'}"`,
			`[b][a]`,
		},
		{
			// Unquoted every column protects, so this is the boundary rather
			// than the rule.
			"unquoted",
			`v=Vx}y; printf '[%s]' ${v-'a}b'} ${u-'a}b'}`,
			`[Vx}y][a}b]`,
		},
		{
			// A double quote protects in every column, in both kinds.
			"a double quote",
			`v=SET; printf '[%s]' "${v-"a}b"}" "${u-"a}b"}"`,
			`[SET][a}b]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runBash(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The quote running the scan off the end of the input is refused here, where
// the five columns that read it as an ordinary character stop at the `}` and
// print `'bar` and the line after it. The refusal arrives at the line the
// construct is on — `one` is printed and `two` is not — which is what bash
// 5.3.15 and 3.2.57 do from a script file. It is the control on the deferral
// as well: nothing about *when* an operand is read can reach a file whose
// scan never found the closing brace.
func TestAnUnbalancedQuoteInAWordOperandIsRefused(t *testing.T) {
	out, errs, st := runScript(t, scriptFile(t, "echo one\necho \"${v+'bar}\"\necho two\n"))
	if out != "one\n" || st != 2 || errs == "" {
		t.Errorf("out %q errs %q status %d, want the file refused at the line, with `two` never reached", out, errs, st)
	}
}

// TestASecondReadingThatRunsOffTheEndBlamesTheBrace is #2969. A word this
// shell's POSIX mode divided one way and the run divides the other, where the
// `${` no longer closes.
//
// It is a **run-time** failure belonging to the word rather than a syntax
// error belonging to the line — the words beside it on the same line expand
// normally — and this shell blames the **brace**, not the quote the word
// opened with. Measured 2026-09-18 on bash 5.3.20 invoked as `sh`.
//
// The two sentences are what make this a wording of its own. The same shell
// words the *parse-time* case against the quote, which the second assertion
// below is: identical text, one binary, two readings, and the failure landing
// in a different phase under each.
func TestASecondReadingThatRunsOffTheEndBlamesTheBrace(t *testing.T) {
	var out, errs bytes.Buffer
	run := func(argv ...string) (string, string, int) {
		out.Reset()
		errs.Reset()
		code := driver.MainArgs(bashShell(&out, &errs), argv)
		return out.String(), errs.String(), code
	}
	for _, tc := range []struct {
		word string
		want string
	}{
		{`"${v-'a}"`, "sh: line 1: bad substitution: no closing `}' in \"${v-'a}\"\n"},
		{`"x${v-'a}y"`, "sh: line 1: bad substitution: no closing `}' in \"x${v-'a}y\"\n"},
		{`"${v:-'a}"`, "sh: line 1: bad substitution: no closing `}' in \"${v:-'a}\"\n"},
	} {
		o, e, st := run("sh", "-c",
			`v=V; f(){ printf '[%s]' `+tc.word+`; echo; }; set +o posix; f; echo AFTER=$?`)
		if e != tc.want {
			t.Errorf("%s: err %q, want %q", tc.word, e, tc.want)
		}
		if o != "" || st != 1 {
			t.Errorf("%s: out %q status %d, want nothing at 1", tc.word, o, st)
		}
	}
	// The control, and the whole reason the sentence is its own field: the
	// same text under this shell's own name, which has only the protecting
	// reading to read it with, is refused **while it is read** — against the
	// quote, at 2, with the words beside it never reached.
	o, e, st := run("bash", "-c", `v=V; printf '[%s]' "${v-'a}"; echo`)
	if want := "bash: -c: line 1: unexpected EOF while looking for matching `''\n"; e != want {
		t.Errorf("the parse-time reading: err %q, want %q", e, want)
	}
	if o != "" || st != 2 {
		t.Errorf("the parse-time reading: out %q status %d, want nothing at 2", o, st)
	}
	// And the row that says nothing else on the line moved: under the name
	// that starts in the mode, the same text parses and the word beside it
	// expands, because the division the parse made is the one that closes.
	o, e, st = run("sh", "-c", `v=V; printf '[%s]' "${v-'a}"; echo`)
	if o != "[V]\n" || e != "" || st != 0 {
		t.Errorf("the mode's own reading: out %q err %q status %d", o, e, st)
	}
}
