// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `disown` answering 1 for every call and saying nothing (#3187).
//
// The axis is moved both ways over the same four spellings, because a status
// that is always 1 cannot be told from one nobody consults. The declining
// column is the reading every other shell has: 0 for a job it let go of, and
// 1 with a complaint for one it could not find.
func TestDisownAnsweringOneForEveryCall(t *testing.T) {
	for _, tc := range []struct {
		name   string
		always Answer
		src    string
		st     int
		says   string
	}{
		{name: "a live job, always", always: Yes, src: "sleep 0 & disown %1", st: 1},
		{name: "a live job, not always", always: No, src: "sleep 0 & disown %1", st: 0},
		{name: "a jobspec that is not there, always", always: Yes, src: "disown %9", st: 1},
		{
			name: "a jobspec that is not there, not always", always: No,
			src: "disown %9", st: 1, says: "no such job",
		},
		{name: "no operand and no job, always", always: Yes, src: "disown", st: 1},
		{name: "no operand and no job, not always", always: No, src: "disown", st: 1},
		{
			// A bad option is refused before the axis is reached, so the
			// shell that fails every call still says which letter it was.
			name: "a bad option, always", always: Yes,
			src: "disown -a", st: 2, says: "-a",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.DisownAlwaysFails = tc.always
			sem.DisownRemovesTheJob = Yes
			dg := Diagnostics{NoSuchJob: "%[1]s: %[2]s: no such job"}
			out, st := run(t, tc.src, func(r *Runner) {
				r.Semantics = &sem
				r.Diagnostics = &dg
			})
			if st != tc.st {
				t.Errorf("status %d, want %d (%q)", st, tc.st, out)
			}
			if tc.says == "" && out != "" {
				t.Errorf("out %q, want nothing said", out)
			}
			if tc.says != "" && !strings.Contains(out, tc.says) {
				t.Errorf("out %q, want %q in it", out, tc.says)
			}
		})
	}
}
