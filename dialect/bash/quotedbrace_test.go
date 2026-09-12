// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

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
