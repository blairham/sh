// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether a declaration's array-literal operand is stored when the command's
// redirection failed (#3815).
//
// bash 5.3.20 and 3.2.57 store it; zsh 5.9.2 does not; ksh93 cannot be asked,
// because a failed redirection on a declaration utility ends that shell.
//
// The two controls carry the weight. A **scalar** operand is not stored in
// bash either, so a reading that applied every operand would match the first
// row and be wrong about the shell it was copying. And a command whose
// redirection **succeeds** must store either way, or the axis has stopped
// being about the failure.
func TestArrayOperandIsStoredPastAFailedOpenAxis(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  Answer
		src     string
		want    string
		refused string
	}{
		{
			// bash 5.3.20 and bash 3.2.57.
			name: "the literal is stored anyway", answer: Yes,
			src:  `typeset a=(VAL) 2>/nope/x; echo "[${a[0]}]"`,
			want: "[VAL]\n",
		},
		{
			// zsh 5.9.2.
			name: "the command that did not run stored nothing", answer: No,
			src:  `typeset a=(VAL) 2>/nope/x; echo "[${a[0]}]"`,
			want: "[]\n",
		},
		{
			// A PIN, not a control, and the difference is worth stating
			// because it is easy to mistake one for the other. It records
			// bash's measured answer — a scalar operand is *not* stored
			// there — so a future change to the scalar path is caught here.
			//
			// It does not discriminate against this implementation. I
			// mutated the store to `assignOperands`, which applies every
			// operand including scalars, and this row still passed: on a
			// path whose redirection failed the scalar's own assign stores
			// nothing anyway, so there is no reading of this function that
			// can make the row fail. A row that cannot fail is not a
			// control, whatever it is named.
			name: "a scalar operand is not stored", answer: Yes,
			src:  `export s=$(echo VAL) 2>/nope/x; echo "[${s}]"`,
			want: "[]\n",
		},
		{
			// CONTROL. An open that succeeds stores under either answer —
			// otherwise the axis is deciding something about ordinary
			// declarations rather than about the failure.
			name: "a successful open still stores, answered No", answer: No,
			src:  `typeset a=(VAL) 2>/dev/null; echo "[${a[0]}]"`,
			want: "[VAL]\n",
		},
		{
			name: "a successful open still stores, answered Yes", answer: Yes,
			src:  `typeset a=(VAL) 2>/dev/null; echo "[${a[0]}]"`,
			want: "[VAL]\n",
		},
		{
			name: "unanswered", answer: Unspecified,
			src:     `typeset a=(VAL) 2>/nope/x; echo "[${a[0]}]"`,
			refused: "array-literal operand stored after its redirection failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			// Not this axis's question, but every row reads `${a[0]}` and
			// the runner refuses an unanswered one. Zero is bash's and
			// zsh's base, which are the two columns these rows are about.
			sem.ArrayBaseIsZero = Yes
			sem.ArrayOperandIsStoredPastAFailedOpen = tc.answer
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if tc.refused != "" {
				if !contains(out, tc.refused) {
					t.Fatalf("got %q, want it to carry %q", out, tc.refused)
				}
				return
			}
			// The open's own complaint goes to standard error and is the
			// dialect's wording, not this axis's; what is asserted is the
			// value the next command reads.
			if !contains(out, tc.want) {
				t.Errorf("got %q, want it to carry %q", out, tc.want)
			}
		})
	}
}
