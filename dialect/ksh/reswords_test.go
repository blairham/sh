// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// TestTheClosingConditionalWordIsNotReported: this shell has `[[ ]]` and
// declines to name the word that closes it, which is a fact about the report
// rather than about what parses (#2981).
//
// `[[` is claimed by every column that has the construct and `]]` by bash
// alone, so the two ends of one construct are two answers — measured
// 2026-09-18 on ksh93u+ 2012-08-01, where `command -v ']]'` is silent at 1
// while `command -v '[['` names the word at 0 and the conditional itself
// runs.
//
// `in` is the other row of that measurement and is already right here: this
// shell names it, where zsh alone does not.
func TestTheClosingConditionalWordIsNotReported(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		word   string
		want   string
		status int
	}{
		{`']]'`, "", 1},
		{`'[['`, "[[", 0},
		{`in`, "in", 0},
		{`if`, "if", 0},
	} {
		out, st := runKsh(t, dir, "command -v "+tc.word+" 2>/dev/null")
		if strings.TrimSpace(out) != tc.want || st != tc.status {
			t.Errorf("command -v %s = %q at %d, want %q at %d", tc.word, out, st, tc.want, tc.status)
		}
	}
	// And the construct is still the construct: the word the report will not
	// name is one the grammar reads.
	if out, st := runKsh(t, dir, `[[ -n x ]] && echo yes`); strings.TrimSpace(out) != "yes" || st != 0 {
		t.Errorf("[[ -n x ]] = %q at %d, want yes at 0", out, st)
	}
	// `type` answers from the same lookup and splits the same way.
	if out, st := runKsh(t, dir, `type '[['`); strings.TrimSpace(out) != "[[ is a keyword" || st != 0 {
		t.Errorf("type '[[' = %q at %d", out, st)
	}
	if out, st := runKsh(t, dir, `type ']]' 2>/dev/null`); st == 0 {
		t.Errorf("type ']]' = %q at %d, want a refusal", out, st)
	}
}
