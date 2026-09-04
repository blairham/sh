// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// hash keeps no cache, so the honest answers are: bare and -r succeed,
// a name that could run succeeds silently, and a name that could not is
// the dialect's question.

func TestHashBareAndResetSucceed(t *testing.T) {
	out, st := run(t, `hash; hash -r; echo "st=$?"`, nil)
	if st != 0 || !strings.Contains(out, "st=0") {
		t.Errorf("out=%q st=%d, want quiet success", out, st)
	}
}

func TestHashEmptyTableWordingIsTheDialects(t *testing.T) {
	out, _ := run(t, `hash`, func(r *Runner) {
		r.Diagnostics = &Diagnostics{HashEmptyTable: "hash: hash table empty"}
	})
	if !strings.Contains(out, "hash table empty") {
		t.Errorf("out=%q, want the table announced where the dialect says so", out)
	}
}

func TestHashAMissingNameIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		status int
		said   bool
	}{
		{"reported", Yes, 1, true},
		{"silent success", No, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, `hash nosuchcmd-xyz`, func(r *Runner) {
				sem := CoreSemantics()
				sem.HashReportsAMissingName = tc.answer
				sem.HashSearchesPathAlone = No
				r.Semantics = &sem
			})
			if st != tc.status {
				t.Errorf("status = %d, want %d (out %q)", st, tc.status, out)
			}
			if said := strings.Contains(out, "not found"); said != tc.said {
				t.Errorf("said = %v, want %v (out %q)", said, tc.said, out)
			}
		})
	}
}

func TestHashCountsABuiltinUnlessPathAlone(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		status int
	}{
		{"a builtin hashes", No, 0},
		{"only PATH counts", Yes, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, st := run(t, `hash shift`, func(r *Runner) {
				sem := CoreSemantics()
				sem.HashSearchesPathAlone = tc.answer
				sem.HashReportsAMissingName = Yes
				r.Semantics = &sem
			})
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
}
