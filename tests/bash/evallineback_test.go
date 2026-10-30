// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// The ERR trap an `eval` fires reads the `eval`'s own line, because the shell
// is back on it once the text is done — where the trap the text's own failing
// command fires reads that command's line, which the text's lines continue
// the caller's to reach (#6955).
//
// Measured 2026-10-30 on bash 5.3.20 over script files, the text held in `x`
// so its lines are not the file's: `eval "$x"` on line 6 with `false` on the
// text's line 3 fires at 8 for the `false` and at 6 for the `eval`, where this
// said 8 twice. The `eval` written across lines 3 to 5 with the `false` on the
// last fires at 5 and then 3. bash 3.2.57 reads 6 for the `eval` too.
func TestTheErrTrapAnEvalFiresReadsTheEvalsLine(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"text with lines of its own", "trap 'echo E=$LINENO' ERR\nx=\":\n:\nfalse\"\n:\neval \"$x\"\n", "E=8\nE=6\n"},
		{"an eval written across lines", "trap 'echo E=$LINENO' ERR\n:\neval ':\n:\nfalse'\n", "E=5\nE=3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := preset.Parse(t, tc.src)
			var buf strings.Builder
			r := preset.Runner(dialecttest.Base{
				Name: "bash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
				Stdout: &buf, Stderr: &buf,
			})
			r.Route = interp.RouteScriptFile
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			if buf.String() != tc.want {
				t.Errorf("wrote %q, want %q", buf.String(), tc.want)
			}
		})
	}
}
