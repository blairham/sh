// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestAnUnclosedBraceInAHeredocBodyQuotesTheBody pins how an unterminated
// `${` in a here-document body is refused: at the command's line, as the
// expansion's own failure, quoting the body's whole text — `bad
// substitution` after it where the expansion stopped before an operator, and
// `bad substitution: no closing` before it where it was reading an operand.
// A bare `${` is the command form and stays a sub-parse. Measured 2026-10-02
// on bash 5.3.20 (#5379).
func TestAnUnclosedBraceInAHeredocBodyQuotesTheBody(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ body, want string }{
		{"a ${x b\nc", "line 1: a ${x b\nc\n: bad substitution\nst=1\n"},
		{"${#", "line 1: ${#\n: bad substitution\nst=1\n"},
		{"pre\na ${x:-y\nc", "line 1: bad substitution: no closing `}' in pre\na ${x:-y\nc\n\nst=1\n"},
		{"${", "command substitution: line 3: unexpected EOF while looking for matching `}'\nst=1\n"},
	} {
		out, _ := runBash(t, t.TempDir(), "cat <<E\n"+tc.body+"\nE\necho st=$?\n")
		if !strings.HasSuffix(out, tc.want) || strings.Count(out, "st=") != 1 {
			t.Errorf("body %q\n got %q\nwant it ending %q", tc.body, out, tc.want)
		}
	}
}
