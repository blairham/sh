// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Every shell in the panel judges its option words before it looks at the
// operand behind them, and this front end did the opposite: it resolved the
// source first, so a script that would not open was answered at 127 before any
// option had been applied.
//
// Measured 2026-09-18 against a path that is not there, with standard input on
// /dev/null — `-o nosuchoption /nope/x.sh` names the option in bash 5.3.20,
// zsh 5.9.2, ksh93u+ 2012-08-01, dash 0.5.12 and BusyBox ash 1.37.0 alike, and
// `-q /nope/x.sh` names the letter in all five. Unanimous, so it is a rule and
// not an axis (#3284).
//
// Here rather than in a dialect package because it is the front end's order
// that was wrong, and the fix is the front end's: the operand is resolved
// lazily and the error carried on the source. The dialect binaries have the
// end-to-end rows.
func TestAnOptionIsJudgedBeforeTheOperandIsOpened(t *testing.T) {
	shell := func() driver.Shell {
		sem := interp.PosixSemantics()
		return driver.Shell{
			Name: "testsh", Dialect: syntax.Core(), Semantics: sem,
			Diagnostics: interp.Diagnostics{
				SetInvalidOptionName: "set: %[1]s: invalid option name",
				ScriptNotFound:       "%[1]s: No such file or directory",
			},
		}
	}
	for _, c := range []struct {
		name   string
		argv   []string
		errs   string
		status int
	}{
		{
			// The option, not the file — which is the whole of the issue.
			name:   "a refused name beats a missing operand",
			argv:   []string{"testsh", "-o", "nosuchoption", "/nope/x.sh"},
			errs:   "testsh: nosuchoption: invalid option name\n",
			status: 2,
		},
		{
			// And with nothing wrong in the options the operand is still
			// answered, in the same words and at the same status it was
			// answered in before: only where in the sequence it happens
			// moved.
			name:   "and a good one still lets the operand speak",
			argv:   []string{"testsh", "-x", "/nope/x.sh"},
			errs:   "testsh: /nope/x.sh: No such file or directory\n",
			status: 127,
		},
		{
			name:   "with no options at all",
			argv:   []string{"testsh", "/nope/x.sh"},
			errs:   "testsh: /nope/x.sh: No such file or directory\n",
			status: 127,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs strings.Builder
			sh := shell()
			sh.Stdout, sh.Stderr = &out, &errs
			if got := driver.MainArgs(sh, c.argv); got != c.status {
				t.Errorf("status %d, want %d (out %q, err %q)", got, c.status, out.String(), errs.String())
			}
			if !strings.Contains(errs.String(), c.errs) {
				t.Errorf("stderr %q, want %q in it", errs.String(), c.errs)
			}
		})
	}
}
