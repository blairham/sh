// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// This shell reads the `[.` and `[=` delimiters as a sub-expression and finds
// an element in no body at all, which is neither of the two readings the axis
// carrying it had while it was a boolean.
//
// Measured 2026-09-18 inside the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// and the same case through `cmd/ash` in the same container (#3379).
//
// No BusyBox was reachable when the axis was first taken, so the preset kept
// the reading the shell already had — which was the wrong one of the two it
// could then hold, and would have been the wrong one of four in any case.
func TestACollatingDelimiterIsReadAndHoldsNothing(t *testing.T) {
	for _, tc := range []struct {
		pattern, subject string
		want             string
		why              string
	}{
		// The discriminating row. The columns that read an element answer
		// `a` here and the column with neither construct answers `a]`; this
		// one answers neither, because the set is empty.
		{"[[.a.]]", "a", "n", "the element is not a member"},
		{"[[.a.]]", "a]", "n", "and the delimiters are not ordinary characters"},
		{"[[.a.]]", "[", "n", "nor is any one of them"},
		// And the row that says the delimiters were read at all: the `]`
		// inside them did not end the bracket, so a member behind the
		// element still counts.
		{"[[.a.]x]", "x", "Y", "a member behind the element still counts"},
		{"[[.a.]x]", "a", "n", "while the element itself never does"},
		{"[x[=a=]]", "x", "Y", "the equivalence class answers alike"},
		// A body that is not an element is the unknown-body question, and
		// this shell answers that one inert — which here is every body.
		{"[a[.nosuch.]b]", "a", "Y", "a member before it survives"},
		{"[a[.nosuch.]b]", "b", "Y", "and one after it"},
		{"[a[.nosuch.]b]", "n", "n", "and no letter of the body is a member"},
		// The names one column reads for a longer body are not read here.
		{"[[.hyphen.]]", "-", "n", "a name is a body like any other"},
	} {
		src := `case "` + tc.subject + `" in ` + tc.pattern + `) printf Y;; *) printf n;; esac` + "\n"
		out, status := run(t, src)
		if status != 0 {
			t.Fatalf("%s vs %q: status %d, out %q", tc.pattern, tc.subject, status, out)
		}
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s vs %q = %q, want %q — %s",
				tc.pattern, tc.subject, out, tc.want, tc.why)
		}
	}
}
