// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An error the shell gives up over inside a trap handler: whether it ends the
// handler, so the code the handler interrupted runs on, or the shell. See
// interp.HandlerErrorReach for the grid across the panel.
//
// Measured 2026-10-02 on bash 5.3.20, `env -i PATH=/usr/bin:/bin`, each row run as
// `-c` (#5360).
func TestAnErrorInsideAHandler(t *testing.T) {
	for _, c := range []struct {
		name, src string
		after     bool
	}{
		{"USR1, unset", "set -u; trap 'echo T1; : $nope; echo T2' USR1; kill -USR1 $$; echo after", false},
		{"ERR, unset", "set -u; trap 'echo T1; : $nope; echo T2' ERR; false; echo after", false},
		{"DEBUG, unset", "set -u; trap 'echo T1; : $nope; echo T2' DEBUG; :; echo after", false},
		{"USR1, arith", "trap 'echo T1; : $((1/0)); echo T2' USR1; kill -USR1 $$; echo after", true},
		{"ERR, arith", "trap 'echo T1; : $((1/0)); echo T2' ERR; false; echo after", true},
		{"DEBUG, arith", "trap 'echo T1; : $((1/0)); echo T2' DEBUG; :; echo after", true},
		{"RETURN, arith", "f() { trap 'echo T1; : $((1/0)); echo T2' RETURN; }; f; echo after", true},
		// A heredoc body is a boundary that catches an unset parameter, and
		// what it caught is not the handler's error: measured the same day,
		// the handler after it still ends at its own division by zero.
		{"ERR, arith, after a caught unset", "set -u\ncat <<E\n$nope\nE\ntrap 'echo T1; : $((1/0)); echo T2' ERR\nfalse\necho after", true},
		{"USR1, arith, after a caught unset", "set -u\ncat <<E\n$nope\nE\ntrap 'echo T1; : $((1/0)); echo T2' USR1\nkill -USR1 $$\necho after", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			if err != nil {
				t.Fatalf("%s: %v", c.src, err)
			}
			if !strings.Contains(out, "T1") || strings.Contains(out, "T2") {
				t.Fatalf("%s\n got %q, want T1 and no T2", c.src, out)
			}
			if got := strings.Contains(out, "after"); got != c.after {
				t.Errorf("%s\n got %q, want after written: %v", c.src, out, c.after)
			}
		})
	}
}
