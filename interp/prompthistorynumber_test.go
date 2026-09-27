// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// historyNumberTable is the escape table above with a history-number code in
// it, so the two halves below differ only in whether the runner was told how
// to count.
func historyNumberTable() PromptStyle {
	st := promptEscapeTable()
	st.Codes['h'] = FieldHistoryNumber
	return st
}

// A runner nobody told refuses the history-number escape by name.
//
// This is the half that would rot into a stub. The list is a fact a Runner
// can hold, so it is tempting to answer the escape from it here — and the
// arithmetic is the dialect's, because the shells count from different ends
// of the same list. A core answer would be a plausible number at status 0 for
// whichever dialect it got wrong, which is the failure the by-name refusal
// exists to prevent.
func TestTheHistoryNumberEscapeIsRefusedWhenNobodyTold(t *testing.T) {
	// A second command on the line, to say the refusal abandons the list:
	// the whole output is the one diagnostic and no more.
	const src = `echo "[${(%):-%h}]"; echo reached`
	out, st := runGrammar(t, src, promptFlagged, func(r *Runner) {
		r.SetPromptStyle(historyNumberTable())
	})
	want := "sh: ${(%):-%h}: the %h prompt escape is not implemented\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st == 0 {
		t.Errorf("status = 0, want a failure — a refusal reported as success is the thing this guards")
	}
}

// And a runner that was told draws what the dialect's arithmetic answers.
//
// The number is carried in whole rather than derived here, which the two rows
// say by answering differently for the same empty list: one counts from the
// newest event and one from the line after it, and this package holds
// neither reading.
func TestTheHistoryNumberEscapeDrawsWhatTheDialectCounts(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int
		want string
	}{
		{"the newest event", 0, "[0]\n"},
		{"or the line after it", 1, "[1]\n"},
		{"and a list with entries behind it", 42, "[42]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, `echo "[${(%):-%h}]"`, promptFlagged, func(r *Runner) {
				r.SetPromptStyle(historyNumberTable())
				r.SetPromptHistoryNumber(func(*Runner) int { return tc.n })
			})
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
