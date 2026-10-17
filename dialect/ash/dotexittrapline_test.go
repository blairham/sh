// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// An EXIT trap after a program that ends on a `.` is located at the line the
// file stopped on, as the file counts it (#6715).
//
// Measured 2026-10-16 in the pinned alpine image (BusyBox v1.37.0), with
// b.inc three lines of `:` and b5.inc five: the trap's `cd` names the file's
// last line by `-c` and from a script file alike, and an `exit` inside the
// file names its own line. A command after the `.` is where the script
// stopped instead, and so is the call of a function whose body was the `.`.
func TestAnExitTrapAfterADotIsLocatedInTheFile(t *testing.T) {
	for _, tc := range []struct {
		route     interp.Route
		src, want string
	}{
		{interp.RouteScriptFile, "trap \"cd /nxT\" EXIT\n. ./b.inc\n", "cd: line 3:"},
		{interp.RouteScriptFile, "trap \"cd /nxT\" EXIT; . ./b.inc\n", "cd: line 3:"},
		{interp.RouteScriptFile, "trap \"cd /nxT\" EXIT\n:\n:\n. ./b5.inc\n", "cd: line 5:"},
		{interp.RouteScriptFile, "trap \"cd /nxT\" EXIT\n. ./bx.inc\n:\n", "cd: line 2:"},
		{interp.RouteScriptFile, "trap \"cd /nxT\" EXIT\n. ./b.inc\n:\n:\n", "cd: line 4:"},
		{interp.RouteScriptFile, "f() { . ./b5.inc; }\ntrap \"cd /nxT\" EXIT\nf\n", "cd: line 3:"},
		{interp.RouteCommandString, "trap \"cd /nxT\" EXIT; . ./b.inc", "cd: line 3:"},
		{interp.RouteCommandString, "trap \"cd /nxT\" EXIT\n:\n. ./b5.inc", "cd: line 5:"},
	} {
		dir := t.TempDir()
		for name, body := range map[string]string{
			"b.inc": ":\n:\n:\n", "b5.inc": ":\n:\n:\n:\n:\n", "bx.inc": ":\nexit 4\n:\n",
		} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		f := preset.Parse(t, tc.src)
		var buf strings.Builder
		r := preset.Runner(dialecttest.Base{
			Name: "ash", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
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
