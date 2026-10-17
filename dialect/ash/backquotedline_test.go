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

// A backquoted body's first line is the line above the one it opens on, never
// below 0, and its later lines count up from there (#6716).
//
// Measured 2026-10-16 in the pinned alpine image (BusyBox v1.37.0), a failure
// inside the body and `$LINENO` read inside it, on a script file and under
// `-c`, which counts from 0 — so its floor is the same first line of 0. Every
// row the old reading was taken from had the substitution on line 2, where
// the line above it and a restart at 1 are the same number; the rows below
// put it everywhere else. `$( … )` is the control, numbered from the file.
func TestABackquotedBodyStartsOnTheLineAboveItsOpener(t *testing.T) {
	for _, tc := range []struct {
		route     interp.Route
		src, want string
	}{
		{interp.RouteScriptFile, "x=`cd /nx`\n", "line 0:"},
		{interp.RouteScriptFile, ":\n:\n:\nx=`cd /nx`\n", "line 3:"},
		{interp.RouteScriptFile, ":\n:\nx=`:\ncd /nx`\n", "line 3:"},
		{interp.RouteScriptFile, ":\n:\n:\n:\nx=`:\n:\ncd /nx`\n", "line 6:"},
		{interp.RouteScriptFile, ":\n:\nx=`echo $LINENO`; echo L=$x\n", "L=2\n"},
		{interp.RouteScriptFile, ":\n:\nx=$(:\ncd /nx)\n", "line 4:"},
		{interp.RouteCommandString, "x=`cd /nx`", "line 0:"},
		{interp.RouteCommandString, "x=`:\ncd /nx`", "line 1:"},
		{interp.RouteCommandString, ":\n:\n:\nx=`cd /nx`", "line 2:"},
		{interp.RouteCommandString, ":\n:\n:\nx=`:\n:\ncd /nx`", "line 4:"},
	} {
		f := preset.Parse(t, tc.src)
		var buf strings.Builder
		r := preset.Runner(dialecttest.Base{
			Name: "ash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
			Stdout: &buf, Stderr: &buf,
		})
		r.Route = tc.route
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if !strings.Contains(buf.String(), tc.want) {
			t.Errorf("%v %q: wrote %q, want %q in it", tc.route, tc.src, buf.String(), tc.want)
		}
	}
}
