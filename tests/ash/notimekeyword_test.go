// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
)

// This shell has no `time` keyword, and the thing that made it look like one
// is a program.
//
// Measured 2026-09-18 inside the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with stdin `/dev/null`, against `cmd/ash` cross-compiled into the same
// container (#2982).

// The discriminator is a **compound command**, which only a keyword can take.
// `time echo hi` prints a three-row summary in this shell and says nothing
// about a keyword: `/usr/bin/time` is a BusyBox applet on that image, so the
// line runs an ordinary command through an ordinary program. Reading the
// summary as evidence is how the flag came to be set without being measured.
//
// Written through `eval` because the refusal is the *parser's*: a snippet that
// will not parse never reaches a runner, and the helper here parses before it
// runs. The measurement is the same either way — `eval 'time for i in 1; do
// :; done'` is `eval: line 1: syntax error: unexpected "do"` in both shells,
// and the script ends at 2.
func TestTimeDoesNotTakeACompoundCommand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a loop", "eval 'time for i in 1; do :; done'\n", `unexpected "do"`},
		{"a subshell", "eval 'time ( : )'\n", `unexpected word`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := run(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want a syntax error containing %q", out, tc.want)
			}
			if status != 2 {
				t.Errorf("status %d, want 2", status)
			}
		})
	}
}

// And `type` says what it is: a file rather than a keyword. The reserved-word
// report is derived from the grammar (#2918), so the flag and this answer
// cannot drift apart — which is what makes this row worth asserting beside the
// grammar one rather than instead of it.
func TestTypeCallsTimeAProgram(t *testing.T) {
	out, status := run(t, "type time\n")
	if status != 0 {
		t.Fatalf("status %d: %s", status, out)
	}
	if strings.Contains(out, "keyword") {
		t.Errorf("type time = %q, want the program it resolves to, not a keyword", out)
	}
}

// The flag itself, asserted on the vector rather than through a snippet: a
// grammar flag set for a whole column is the kind of answer that reads as
// measured whether or not it was, and this one was inherited for the life of
// the dialect.
func TestTheGrammarHasNoTimeKeyword(t *testing.T) {
	if ash.Dialect().TimeKeyword {
		t.Error("TimeKeyword is set, and this shell refuses `time` in front of a compound command")
	}
}
