// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"regexp"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// $TIMEFORMAT is how a script asks the `time` keyword for a shape other than
// the three-line block — most often for one machine-readable line. It was not
// read at all here, so every such script got four lines of something else at
// status 0, which is a wrong answer that says it succeeded.
//
// The figures are real timings and are matched by shape, which is the
// discipline the rest of timeclause_test.go already keeps.

// timeFormatDiag is the two dialects' shared reading of the variable, with
// bash's wording for the one thing they word differently.
func timeFormatDiag() Diagnostics {
	return Diagnostics{
		TimeFormatVariable:     "TIMEFORMAT",
		TimeFormatBadDirective: "%[1]s: `%[2]s': invalid format character",
	}
}

// formatted runs `time true` with the variable set to format and returns what
// landed on the shell's stderr.
func formatted(t *testing.T, format string) string {
	t.Helper()
	_, errs, st := timeRun(t, `time true`, syntax.Core(), timeSem(), timeFormatDiag(),
		func(r *Runner) { r.Vars = map[string]string{"TIMEFORMAT": format} })
	if st != 0 {
		t.Errorf("status = %d, want the pipeline's 0", st)
	}
	return errs
}

// TestTheTimeFormatVariableShapesTheReport, one directive at a time.
func TestTheTimeFormatVariableShapesTheReport(t *testing.T) {
	for _, tc := range []struct{ name, format, want string }{
		{"seconds, three decimals by default", `[%R]`, `^\[\d+\.\d{3}\]\n$`},
		{"a precision digit", `[%0R][%1R][%2R]`, `^\[\d+\]\[\d+\.\d\]\[\d+\.\d{2}\]\n$`},
		{"past six the digit says nothing more", `[%6R][%9R]`, `^\[\d+\.\d{6}\]\[\d+\.\d{6}\]\n$`},
		{"the long form always writes the minutes", `[%lR]`, `^\[\d+m\d+\.\d{3}s\]\n$`},
		{"a precision inside the long form", `[%1lR]`, `^\[\d+m\d+\.\ds\]\n$`},
		{"user and sys read the same way", `[%U][%S][%lU]`, `^\[\d+\.\d{3}\]\[\d+\.\d{3}\]\[\d+m\d+\.\d{3}s\]\n$`},
		{"the percentage carries two decimals", `[%P]`, `^\[\d+\.\d{2}\]\n$`},
		{"a doubled percent is one", `[%%]`, `^\[%\]\n$`},
		{"a trailing percent is a percent", `end %`, `^end %\n$`},
		{"literal text passes through", `a b\tc`, `^a b\\tc\n$`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := formatted(t, tc.format)
			if !regexp.MustCompile(tc.want).MatchString(got) {
				t.Errorf("stderr = %q, want %s", got, tc.want)
			}
		})
	}
}

// TestBackslashEscapesInTheFormatAreNotExpanded is the row above's last line
// pulled out, because it is the detail a reader would assume the other way: a
// format wanting a newline is written `$'\n…'`, which the *word* does before
// the variable ever holds it. The default value is spelled that way for
// exactly that reason, and a reader that expanded escapes here would expand
// them twice for every script that did.
func TestBackslashEscapesInTheFormatAreNotExpanded(t *testing.T) {
	if got := formatted(t, `a\nb`); got != "a\\nb\n" {
		t.Errorf("stderr = %q, want the backslash and the n as written", got)
	}
}

// TestAnEmptyFormatPrintsNothing — not a blank line, nothing. It is the state
// that says the variable is read rather than merely present: unset leaves the
// dialect's default block and set-and-empty silences the report.
func TestAnEmptyFormatPrintsNothing(t *testing.T) {
	if got := formatted(t, ""); got != "" {
		t.Errorf("stderr = %q, want nothing at all", got)
	}
	_, errs, _ := timeRun(t, `time true`, syntax.Core(), timeSem(), timeFormatDiag(), nil)
	if !defaultReport.MatchString(errs) {
		t.Errorf("unset: stderr = %q, want the default block", errs)
	}
}

// TestAnUnknownDirectiveRefusesTheWholeReport. The complaint is printed and
// nothing else is: a partial report would be worse than none, because a
// script parsing it would read the fields it did get.
func TestAnUnknownDirectiveRefusesTheWholeReport(t *testing.T) {
	got := formatted(t, `real %R and %Q`)
	if got != "testsh: TIMEFORMAT: `Q': invalid format character\n" {
		t.Errorf("stderr = %q, want the complaint and no report", got)
	}
}

// TestAFormatIsNotReadWhereTheDialectHasNoVariable. A dialect that names no
// variable must not pick up a name another dialect uses — the field is the
// only thing that makes the variable live.
func TestAFormatIsNotReadWhereTheDialectHasNoVariable(t *testing.T) {
	_, errs, _ := timeRun(t, `time true`, syntax.Core(), timeSem(), Diagnostics{},
		func(r *Runner) { r.Vars = map[string]string{"TIMEFORMAT": "[%R]"} })
	if !defaultReport.MatchString(errs) {
		t.Errorf("stderr = %q, want the default block and not the format", errs)
	}
}

// TestThePosixFlagIsAskedBeforeTheFormat: `time -p` prints the POSIX three
// lines whatever the variable holds, in both shells that read one.
func TestThePosixFlagIsAskedBeforeTheFormat(t *testing.T) {
	d := syntax.Core()
	d.TimePosixFlag = true
	_, errs, _ := timeRun(t, `time -p true`, d, timeSem(), timeFormatDiag(),
		func(r *Runner) { r.Vars = map[string]string{"TIMEFORMAT": "[%R]"} })
	posix := regexp.MustCompile(`^real \d+\.\d{2}\nuser \d+\.\d{2}\nsys \d+\.\d{2}\n$`)
	if !posix.MatchString(errs) {
		t.Errorf("stderr = %q, want the POSIX three lines", errs)
	}
}
