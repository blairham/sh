// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// A subshell's EXIT trap after an `eval` is located at the line the text
// stopped on, in the caller's lines the text continues, and not back on the
// `eval`'s own — the other reading of Semantics.EvalPutsTheCallersLineBack,
// which ksh93 takes (#6955).
//
// Measured 2026-10-30 in the pinned alpine image (BusyBox v1.37.0) over script
// files, a three-line trap body whose second line fails and whose third writes
// `$LINENO`, the text three lines held in `x`: `eval "$x"` on line 9 is located
// at 11 whether the text ends `exit 0` or runs out, as the subshell's last
// command. Only `$LINENO` is held here: this harness does not word the
// location of a diagnostic the way the binary does.
func TestAnEvalLeavesTheExitTrapWhereItsTextStopped(t *testing.T) {
	const body = "trap \":\nnosuch_t\necho L=\\$LINENO\" EXIT\n"
	for _, tc := range []struct{ name, src, want string }{
		{"an exit in the text", "x=\":\n:\nexit 0\"\n(\n" + body + ":\neval \"$x\"\n:\n)\n:\n", "L=11\n"},
		{"the text running out", "x=\":\n:\n:\"\n(\n" + body + ":\neval \"$x\"\n)\n:\n", "L=11\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := preset.Parse(t, tc.src)
			var buf strings.Builder
			r := preset.Runner(dialecttest.Base{
				Name: "ash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
				Stdout: &buf, Stderr: &buf,
			})
			r.Route = interp.RouteScriptFile
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.HasSuffix(buf.String(), tc.want) {
				t.Errorf("wrote %q, want it to end %q", buf.String(), tc.want)
			}
		})
	}
}
