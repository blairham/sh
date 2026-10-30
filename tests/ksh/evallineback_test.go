// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// Once `eval`'s text is done the shell is back on the `eval`'s own line, as it
// is after a function call, so whatever reads the line before the next command
// sets it reads the `eval`'s line and not the text's last (#6955).
//
// Measured 2026-10-30 on ksh93u+ 2012-08-01 over script files, with the
// three-line trap body the neighboring test uses: its second line fails and its
// third writes `$LINENO`, so a subshell's EXIT trap is counted on from the last
// line the subshell ran. The text is held in `x` where its lines would
// otherwise be the file's, because an `eval` written across lines 7 to 11
// starts on 7 and its last line is 11, and only a text with lines of its own
// tells "the `eval`'s line" from "the `eval`'s line plus the text's, less one":
// `eval "$x"` on line 9 with `exit 0` on the text's line 3 is counted from 9,
// not 11, and not the text's own 3, which is what this said.
func TestEvalPutsTheShellBackOnItsOwnLine(t *testing.T) {
	const body = "trap \":\nnosuch_t\necho L=\\$LINENO\" EXIT\n"
	const three = "x=\":\n:\nexit 0\"\n"
	for _, tc := range []struct{ name, src, want string }{
		{"an exit in the text ends the subshell", ":\n(\n" + body + ":\neval 'exit 0'\n:\n)\n:\n", "line 8: nosuch_t: not found\nL=9\n"},
		{"text with lines of its own", three + "(\n" + body + ":\neval \"$x\"\n:\n)\n:\n", "line 10: nosuch_t: not found\nL=11\n"},
		{"an eval written across lines", ":\n(\n" + body + ":\neval ':\n:\n:\n:\nexit 0'\n:\n)\n:\n", "line 8: nosuch_t: not found\nL=9\n"},
		{"an eval in an eval", "y=\":\n:\nexit 0\"\nx=\":\neval \\\"\\$y\\\"\"\n(\n" + body + ":\neval \"$x\"\n:\n)\n:\n", "line 12: nosuch_t: not found\nL=13\n"},
		{"the text running out", "x=\":\n:\n:\"\n(\n" + body + ":\neval \"$x\"\n)\n:\n", "line 10: nosuch_t: not found\nL=11\n"},
		{"the ERR trap the eval fires", "trap 'echo E=$LINENO' ERR\nx=\":\n:\nfalse\"\n:\neval \"$x\"\n", "E=3\nE=6\n"},
		{"a case subject after it", "x=\":\n:\n:\"\n:\n:\n:\neval \"$x\"\ncase $LINENO in 3) echo 3;; 7) echo 7;; *) echo other;; esac\n", "7\n"},
		{"a function call, the control", "f() {\n:\nexit 0\n}\n(\n" + body + ":\nf\n:\n)\n:\n", "line 11: nosuch_t: not found\nL=12\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := preset.Parse(t, tc.src)
			var buf strings.Builder
			r := preset.Runner(dialecttest.Base{
				Name: "ksh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
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
