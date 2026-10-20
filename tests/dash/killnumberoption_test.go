// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// TestThisShellHasNoNumberOption records the second column without `kill -n`.
//
// Measured 2026-09-17 on dash 0.5.12, macOS arm64, under a matching
// `argv[0]`, from a script file:
//
//	kill -n 99 999999   kill: Illegal option -n         2
//	kill -s 99 999999   kill: invalid signal number or name: 99    2
//	kill -s             kill: No arg for -s option      2
//
// The first row is the whole of it: `-n` is not an option, so the word is a
// dash-word like any other and `n` is its first option letter. This shell
// read it as bash's `-n` and refused the *99* instead, which named a number
// nobody typed at a status the real shell does reach by another road — so the
// two looked alike and were not.
//
// `interp/killbuiltin.go`'s header used to record this as deliberately not
// built, on the reasoning that only one column wanted it. BusyBox ash is the
// second and refuses it differently again, which is what made it an axis
// rather than a gap (#3165).
func TestThisShellHasNoNumberOption(t *testing.T) {
	if got := dash.Semantics().KillReadsTheNumberOption; got != interp.No {
		t.Errorf("KillReadsTheNumberOption is %v, want No", got)
	}
	// And `-s` with nothing after it is still an option missing its argument
	// here, which is the control: the two questions are separate and only
	// BusyBox answers the second one the other way.
	if got := dash.Semantics().KillOptionWithNoArgumentIsASignalName; got != interp.No {
		t.Errorf("KillOptionWithNoArgumentIsASignalName is %v, want No", got)
	}
	for _, c := range []struct{ src, want string }{
		{`kill -n 99 999999`, "Illegal option -n"},
		{`kill -s 99 999999`, "invalid signal number or name: 99"},
		{`kill -s`, "No arg for -s option"},
	} {
		out, _ := answersRun(t, c.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s said %q, want %q in it", c.src, out, c.want)
		}
		if !strings.Contains(out, "st=2") {
			t.Errorf("%s said %q, want status 2", c.src, out)
		}
	}
}
