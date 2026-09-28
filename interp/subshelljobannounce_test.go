// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subshell has nobody to tell that a job started.
//
// The prompt belongs to the shell that owns the terminal, and a body a real
// shell would have forked draws none — so a background job started inside one
// is not announced, however loudly the shell around it would have announced
// its own.
//
// **Five columns to nothing, so it is not an axis.** Measured 2026-09-28
// through a pseudo-terminal, `( sleep 2 & )` typed at an interactive prompt
// with no other job running, counting `[n] pid` lines: bash 5.3.20, bash
// 3.2.57, ksh93, dash and zsh 5.9.2 all write **none**, and this shell wrote
// **one** at every boundary below before #5021.
//
// The four rows are four kinds of clone and they are the point: the noun is
// *being a copy*, not being parentheses. A rule written against `( … )` alone
// would have agreed with the reference on the first row and gone on announcing
// in the other three.
func TestASubshellDoesNotAnnounceTheJobsItStarts(t *testing.T) {
	for _, tc := range []struct {
		name, src string
	}{
		{"a parenthesised subshell", `( sleep 0.3 & )`},
		{"a command substitution", `V=$( sleep 0.3 & echo x )`},
		{"a backquoted substitution", "V=`sleep 0.3 & echo x`"},
		{"a compound pipeline element", `{ sleep 0.3 & } | cat`},
		{"and a subshell nested in one", `( ( sleep 0.3 & ) )`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out := notices(t, tc.src, true, Yes, Diagnostics{}, nil); strings.Contains(out, "[1]") {
				t.Errorf("out = %q, want a subshell to announce nothing", out)
			}
		})
	}
	// The control, and it is what stops this passing for the wrong reason: a
	// shell that had simply stopped announcing would pass every row above.
	// The same source at the top level still announces.
	t.Run("while the shell around it still announces its own", func(t *testing.T) {
		if out := notices(t, `sleep 0.3 &`, true, Yes, Diagnostics{}, nil); !strings.Contains(out, "[1] ") {
			t.Errorf("out = %q, want the shell's own job announced", out)
		}
	})
	// And the job is still *started* — silence is about the notice and not
	// about the work. `$!` is set inside the subshell, so something was
	// backgrounded there.
	t.Run("and the job it did not announce still ran", func(t *testing.T) {
		out := notices(t, `( sleep 0.3 & echo "started=${!:+yes}" )`, true, Yes, Diagnostics{}, nil)
		if !strings.Contains(out, "started=yes") {
			t.Errorf("out = %q, want the subshell to have backgrounded something", out)
		}
		if strings.Contains(out, "[1]") {
			t.Errorf("out = %q, want no announcement", out)
		}
	})
}
