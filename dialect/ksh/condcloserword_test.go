// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A `]]` standing where a condition **term** belongs is an ordinary word
// here, so the closer is only a closer once the condition has something to
// close over.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, `-c` and script files. bash and zsh refuse the token in every
// row below; this shell runs the first three of them (#2964).
func TestTheConditionCloserIsAWordWhereATermBegins(t *testing.T) {
	for _, tc := range []struct {
		src    string
		status int
		why    string
	}{
		// The word is two characters and non-empty, so a condition over it
		// alone holds.
		{`[[ ]] ]]`, 0, "a bare word is a test for non-emptiness"},
		{`[[ ]] == x ]]`, 1, "and it compares as the string it is"},
		{`[[ ]] && x ]]`, 0, "a connective joins it like any other word"},
		// An operand's position is a separate question, and this shell
		// answers it the way the other two do.
		{`[[ -n ]]`, 3, "the closer behind a unary operator is still the closer"},
		{`[[ x == ]]`, 3, "and behind a binary one"},
	} {
		out, st := runKshCloser(t, tc.src)
		if st != tc.status {
			t.Errorf("%s: status %d, want %d — %s (%q)", tc.src, st, tc.status, tc.why, out)
		}
	}
	// And the condition that never closes: the word is read, the input runs
	// out, and the `[[` is what is named.
	out, st := runKshCloser(t, `[[ ]]`)
	if !strings.Contains(out, "`[[' unmatched") || st != 3 {
		t.Errorf("an empty condition: out %q status %d, want the opener named at 3", out, st)
	}
}

// runKshCloser runs one line through the front end and hands back everything
// it said with the status, which is what these rows are: half of them are
// silent and the status is the whole answer, and half are refusals the parse
// makes rather than errors a run returns.
func runKshCloser(t *testing.T, src string) (string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	code := driver.MainArgs(kshShell(&out, &errs), []string{"ksh", "-c", src})
	return out.String() + errs.String(), code
}
