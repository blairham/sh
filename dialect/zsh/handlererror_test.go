// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An error the shell gives up over inside a trap handler: whether it ends the
// handler, so the code the handler interrupted runs on, or the shell. See
// interp.HandlerErrorReach for the grid across the panel.
//
// Measured 2026-10-02 on zsh 5.9.2 under `-f`, `env -i PATH=/usr/bin:/bin`, each row run as
// `-c` (#5360).
func TestAnErrorInsideAHandler(t *testing.T) {
	for _, c := range []struct {
		name, src string
		after     bool
	}{
		{"USR1, unset", "set -u; trap 'print T1; : $nope; print T2' USR1; kill -USR1 $$; print after", true},
		{"ERR, unset", "set -u; trap 'print T1; : $nope; print T2' ERR; false; print after", true},
		{"DEBUG, unset", "set -u; trap 'print T1; : $nope; print T2' DEBUG; :; print after", true},
		{"USR1, arith", "trap 'print T1; : $((1/0)); print T2' USR1; kill -USR1 $$; print after", true},
		{"ERR, arith", "trap 'print T1; : $((1/0)); print T2' ERR; false; print after", true},
		{"DEBUG, arith", "trap 'print T1; : $((1/0)); print T2' DEBUG; :; print after", true},
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
