// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// POSIX mode makes an expansion this shell cannot perform end the shell rather
// than cost the line, and a **bracketed** expression is the one shape it does
// not reach.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/bash` — `GNU bash, version
// 5.3.20(1)-release (aarch64-apple-darwin25.6.0)` — and against the 3.2.57
// macOS ships at `/bin/bash`, which agrees row for row, so this is not a
// version line. `go version -m` says *not a Go executable* for both. Each
// program was run as a script file and as one `-c` string, with `a=(1 2)` and
// `echo B4` in front and `echo "after st=$?"` behind (#4784):
//
//	                 mode  file              -c
//	echo ${a[1+]}    on    after st=1, 0     nothing, 1
//	echo ${a[1+]}    off   after st=1, 0     nothing, 1
//	echo $((1/0))    on    nothing, 1        nothing, 127
//	echo $((1/0))    off   after st=1, 0     after st=1, 0
//	echo ${#+}       on    nothing, 1        nothing, 127
//	echo ${#+}       off   after st=1, 0     after st=1, 0
//
// The first pair carries it: the two mode rows are identical for a subscript
// and differ for everything else. The `-c` column is the second half — the
// route's own 127 is not a bad subscript's number either, in either mode.
func TestPosixModeDoesNotSharpenABadSubscript(t *testing.T) {
	for _, c := range []struct {
		name     string
		probe    string
		sharpens bool
	}{
		{"a subscript that will not evaluate", "echo ${a[1+]}", false},
		{"the same subscript in a store", "v=${a[1+]}", false},
		{"the same subscript inside a function", "g() { echo ${a[1+]}; }\ng", false},
		{"a division by zero", "echo $((1/0))", true},
		{"a word with no expansion to perform", "echo ${#+}", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "a=(1 2)\n" + c.probe + "\necho AFTER\n"
			inMode := posixRouteRun(t, "set -o posix\n"+src, interp.RouteScriptFile)
			without := posixRouteRun(t, src, interp.RouteScriptFile)
			if !strings.Contains(without, "AFTER") {
				t.Fatalf("%q without the mode: got %q, want the next line to run", c.probe, without)
			}
			if c.sharpens && strings.Contains(inMode, "AFTER") {
				t.Errorf("%q in the mode: got %q, want the shell ended", c.probe, inMode)
			}
			if !c.sharpens && !strings.Contains(inMode, "AFTER") {
				t.Errorf("%q in the mode: got %q, want the next line to run", c.probe, inMode)
			}
		})
	}
}

// And the number. A bad subscript from a command string costs this shell its
// ordinary fatal status in either mode, where a bare failure costs the 127
// this column gives that route — and without the mode a bare failure costs
// nothing but the line.
func TestABadSubscriptFromACommandStringKeepsTheFatalStatus(t *testing.T) {
	for _, posix := range []bool{true, false} {
		src := "a=(1 2)\necho ${a[1+]}\necho AFTER\n"
		if posix {
			src = "set -o posix\n" + src
		}
		out, st := posixRouteStatus(t, src, interp.RouteCommandString)
		if st != 1 {
			t.Errorf("posix=%v: status %d, want 1", posix, st)
		}
		if strings.Contains(out, "AFTER") {
			t.Errorf("posix=%v: got %q, want the command string given up whole", posix, out)
		}
	}
	if _, st := posixRouteStatus(t, "set -o posix\necho $((1/0))\necho AFTER\n", interp.RouteCommandString); st != 127 {
		t.Errorf("a bare failure in the mode: status %d, want 127", st)
	}
	if out, _ := posixRouteStatus(t, "echo $((1/0))\necho AFTER\n", interp.RouteCommandString); !strings.Contains(out, "AFTER") {
		t.Errorf("a bare failure without the mode: got %q, want the next line to run", out)
	}
}

func posixRouteRun(t *testing.T, src string, route interp.Route) string {
	t.Helper()
	out, _ := posixRouteStatus(t, src, route)
	return out
}

func posixRouteStatus(t *testing.T, src string, route interp.Route) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "bash", Env: []string{"PATH=/usr/bin:/bin"}, Route: route,
	}, src)
	if err != nil {
		return out + "unsupported: " + err.Error(), -1
	}
	return out, st
}

// The second shape the mode does not reach: a **substring range**.
//
// Measured 2026-09-27 the same way, against the same two builds, which agree
// on every row here — with `a=(1 2 3); v=abcdef` in front (#4804):
//
//	                     mode  file              -c
//	echo ${v:1+}         on    after st=1, 0     after st=1, 0
//	echo ${v:0:1+}       on    after st=1, 0     after st=1, 0
//	echo ${a[@]:1+}      on    after st=1, 0     after st=1, 0
//	echo ${a[@]:0:1+}    on    after st=1, 0     after st=1, 0
//	echo ${a[@]:1:-1}    on    after st=1, 0     after st=1, 0
//	echo ${v:$((1/0))}   on    nothing, 1        nothing, 127
//	echo ${v:$((1/0))}   off   after st=1, 0     after st=1, 0
//
// The last pair is the one that matters and is why this is a shape of its own
// rather than the bracket's rule widened: an arithmetic expansion written
// *inside* the range is sharpened on both routes, so what escapes the mode is
// the range's reader and not "an expression inside a parameter expansion".
//
// The `-c` column is the other difference: a range prints the next line's
// `after st=1` where a subscript prints nothing at all.
func TestPosixModeDoesNotSharpenASubstringRange(t *testing.T) {
	for _, c := range []struct {
		name     string
		probe    string
		sharpens bool
	}{
		{"an offset that will not evaluate", "echo ${v:1+}", false},
		{"a length that will not evaluate", "echo ${v:0:1+}", false},
		{"a list's offset", "echo ${a[@]:1+}", false},
		{"a list's length", "echo ${a[@]:0:1+}", false},
		{"a list's negative length", "echo ${a[@]:1:-1}", false},
		{"the same range in a store", "x=${v:1+}", false},
		{"an arithmetic expansion inside the range", "echo ${v:$((1/0))}", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "a=(1 2 3)\nv=abcdef\n" + c.probe + "\necho AFTER\n"
			inMode := posixRouteRun(t, "set -o posix\n"+src, interp.RouteScriptFile)
			without := posixRouteRun(t, src, interp.RouteScriptFile)
			if !strings.Contains(without, "AFTER") {
				t.Fatalf("%q without the mode: got %q, want the next line to run", c.probe, without)
			}
			if c.sharpens && strings.Contains(inMode, "AFTER") {
				t.Errorf("%q in the mode: got %q, want the shell ended", c.probe, inMode)
			}
			if !c.sharpens && !strings.Contains(inMode, "AFTER") {
				t.Errorf("%q in the mode: got %q, want the next line to run", c.probe, inMode)
			}
			// And the command-string route, where the two shapes' unwinding
			// parts: a range costs the line there too.
			if !c.sharpens {
				out := posixRouteRun(t, "set -o posix\n"+src, interp.RouteCommandString)
				if !strings.Contains(out, "AFTER") {
					t.Errorf("%q from a command string: got %q, want the next line to run", c.probe, out)
				}
			}
		})
	}
}
