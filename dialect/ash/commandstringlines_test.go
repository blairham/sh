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

// runCommandString runs src the way `-c` does, which is the one route this
// shell numbers differently.
//
// A Runner built by hand rather than through Preset.Combined, because the
// route is the whole subject: interp.Runner.Route is what says the program
// arrived as an argument, and the shared builder leaves it unspecified on
// purpose — every other test here is about a shell reading a file.
func runCommandString(t *testing.T, src string) string {
	t.Helper()
	f := preset.Parse(t, src)
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{
		Name: "ash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
		Stdout: &buf, Stderr: &buf,
	})
	r.Route = interp.RouteCommandString
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return buf.String()
}

// A program handed to `-c` is numbered from 0 in this shell: its first line is
// line 0 (#2799).
//
// The counter and not the rendering, which is what `$LINENO` says — and the
// only reason the rest of it is visible at all is #2761, since this shell's
// *own* diagnostics carry no line on this route. A builtin's location and a
// borrowed text's are the two places the digit can be read.
//
// Measured 2026-09-14, BusyBox v1.37.0 in the pinned alpine image, under
// `env -i`. bash 3.2 answers `$LINENO` the same way here and every other
// column in the panel says 1; a script file and standard input are 1-based in
// all seven.
func TestACommandStringIsNumberedFromZero(t *testing.T) {
	for _, tc := range []struct{ name, src, want, why string }{
		{
			"the parameter the route counts with",
			`echo "L=$LINENO"`, "L=0\n",
			"the fact itself: `ash -c 'echo $LINENO'` is 0 and the same line in a file is 1",
		},
		{
			"the second line of the program",
			"\n" + `echo "L=$LINENO"`, "L=1\n",
			"one behind throughout rather than a nought written for the first line alone",
		},
		{
			"a builtin's own location",
			"shift -1\n", ": shift: line 0: Illegal number: -1",
			"the location #2761 gave this shell, with the route's digit in it",
		},
		{
			"a builtin three lines in",
			"echo a\necho b\nshift -1\n", ": shift: line 2: Illegal number: -1",
			"the offset is the route's and not a nought floor",
		},
		{
			"a borrowed text's location",
			`eval "nosuchcmd"`, ": eval: line 0: nosuchcmd: not found",
			"the offset survives into text that resets the *other* offset — an `eval`'s " +
				"program is line 1 of itself, and it is still written as 0 here",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out := runCommandString(t, tc.src); !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q — %s", out, tc.want, tc.why)
			}
		})
	}
}

// The control the rule above needs: the other two routes are 1-based, so
// nothing here may shift a line a file or standard input wrote.
func TestAFileAndStandardInputAreStillNumberedFromOne(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the parameter", `echo "L=$LINENO"`, "L=1\n"},
		{"a builtin's location", "echo a\necho b\nshift -1\n", ": shift: line 3: Illegal number: -1"},
		{"a borrowed text", `eval "nosuchcmd"`, ": eval: line 1: nosuchcmd: not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runIn(t, tc.src); !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
		})
	}
}
