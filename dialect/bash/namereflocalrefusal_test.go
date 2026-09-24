// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// **A reference the running call made local is the declaration's to refuse,
// and the declaration keeps it.** A reference it merely inherited is the
// assignment's, and that refusal takes the name away.
//
// The same two lines part on nothing but whose binding the reference is.
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over script files, against bash 5.3.20 and bash 5.3.15, which agree:
//
//	f() { typeset -n q; typeset q=42; typeset -p q; }; f
//	  `typeset: `42': invalid variable name for name reference`, and
//	  `declare -n q` still standing
//	typeset -n q; typeset q=42; typeset -p q          (at the top level)
//	  `typeset: `42': not a valid identifier`, and no `q` at all
//
// This narrows a rule that shipped too wide: the take-away was made the
// declaration spelling's everywhere, which is right for a reference the call
// inherited and wrong for one it made. The top-level rows are unchanged; what
// is added is that the call's own reference takes the `n` letter's sentence
// and survives (#4178).
func TestACallsOwnReferenceIsTheDeclarationsToRefuse(t *testing.T) {
	const letter = "invalid variable name for name reference"
	const builtin = "not a valid identifier"
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row `nameref13.sub` grades.
			"the call's own reference takes the letter's sentence",
			"f() { typeset -n q; typeset q=42; }\nf", letter, builtin,
		},
		{
			"and survives the refusal",
			"f() { typeset -n q; typeset q=42; typeset -p q; }\nf 2>/dev/null",
			"declare -n q", "not found",
		},
		{
			// The control on the other side: a reference this call did not
			// make takes the assignment's sentence and goes.
			"a reference the call did not make is taken away",
			"declare -n q\ndeclare q=42\ndeclare -p q", "q: not found", letter,
		},
		{
			// `-g` inside a function reaches the caller's reference, so it
			// takes the top-level answer — which says the rule is about the
			// binding and not about being in a function.
			"the caller's reference under -g takes the top-level answer",
			"declare -n q\nf() { declare -g q=42; declare -p q; }\nf",
			"q: not found", letter,
		},
		{
			// A **plain assignment** over the call's own reference is the
			// bare spelling's refusal, not the declaration's.
			"a plain assignment over it is the bare refusal",
			"f() { typeset -n q; q=42; }\nf", builtin, letter,
		},
		{
			// A letter that predates the refused line stays on the name.
			"a letter written before the refused line stays",
			"f() { typeset -in q; typeset q=42; typeset -p q; }\nf 2>/dev/null",
			"declare -in q", "not found",
		},
		{
			// And a second function declaring the name makes a fresh local
			// and writes into it, reaching no refusal at all.
			"a nested call makes a fresh local instead",
			"outer() { typeset -n q; inner; }\ninner() { typeset q=42; typeset -p q; }\nouter",
			`declare -- q="42"`, "not a valid identifier",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
			if tc.absent != "" && strings.Contains(out, tc.absent) {
				t.Errorf("= %q, want it not to contain %q", out, tc.absent)
			}
		})
	}
}
