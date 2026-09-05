// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// TestFatalErrorsAbandonTheScript covers the three unrelated failures that
// share one status axis. Each is asserted under a vector that calls it fatal
// and one that does not, because asserting only the fatal side would pass
// against an implementation that made everything fatal.
func TestFatalErrorsAbandonTheScript(t *testing.T) {
	shiftFatal := permissive()
	shiftFatal.FatalErrorStatusIsOne = Yes
	shiftSurvives := permissive()
	shiftSurvives.ShiftPastEndFatal = No
	readonlySurvives := permissive()
	readonlySurvives.ReadonlyReassignmentFatal = No
	readonlySurvives.ReadonlyReassignmentFatalFromCommandString = No

	tests := []struct {
		name, src string
		fatal     Semantics // aborts, and with which status
		status    int
		survives  Semantics // reaches the second command
	}{
		{
			"readonly reassignment",
			// `echo after` on its own line on purpose. The vector that
			// survives this abandons the rest of the *line* the refusal was
			// on, so with all three on one line nothing after it runs there
			// either and the two sides of this test stop differing.
			"readonly r=1; r=2\necho after",
			PosixSemantics(), 2, readonlySurvives,
		},
		{
			"shift past the end",
			`shift 5; echo after`,
			shiftFatal, 1, shiftSurvives,
		},
		{
			"arithmetic error",
			// Fatal in every measured vector, so the surviving side is not
			// asserted.
			`echo $((1/0)); echo after`,
			PosixSemantics(), 2,
			Semantics{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, withSem(tc.fatal))
			if strings.Contains(out, "after") {
				t.Errorf("the script continued: %q", out)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
			if tc.survives == (Semantics{}) {
				return
			}
			out, st = run(t, tc.src, withSem(tc.survives))
			if !strings.Contains(out, "after") {
				t.Errorf("the script stopped where the vector survives: %q", out)
			}
			if st != 0 {
				t.Errorf("surviving status = %d, want 0", st)
			}
		})
	}
}

// TestFatalStatusIsOneAxis is the evidence for having one axis rather than
// three: unrelated errors, same split, so FatalErrorStatusIsOne is asked once.
func TestFatalStatusIsOneAxis(t *testing.T) {
	one := permissive()
	one.FatalErrorStatusIsOne = Yes
	for _, src := range []string{`readonly r=1; r=2`, `shift 5`, `echo $((1/0))`} {
		if _, st := run(t, src, withSem(PosixSemantics())); st != 2 {
			t.Errorf("%s with the axis No: status = %d, want 2", src, st)
		}
		if _, st := run(t, src, withSem(one)); st != 1 {
			t.Errorf("%s with the axis Yes: status = %d, want 1", src, st)
		}
	}
}

func TestUnmatchedGlobIsFatalOnlyWhereTheDialectSaysSo(t *testing.T) {
	// GlobNoMatchIsError. Reporting the error and then passing the pattern
	// through was the bug: the diagnostic appeared and the command ran anyway.
	fatal := permissive()
	fatal.GlobNoMatchIsError = Yes
	fatal.FatalErrorStatusIsOne = Yes
	out, st := run(t, `echo /zzz_no_such_dir_*; echo after`, withSem(fatal))
	if strings.Contains(out, "after") {
		t.Errorf("Yes: the script continued: %q", out)
	}
	// The diagnostic names the pattern, so the check is that `echo` never
	// wrote it as a line of its own.
	for _, line := range strings.Split(out, "\n") {
		if line == "/zzz_no_such_dir_*" {
			t.Errorf("Yes: the pattern was passed through: %q", out)
		}
	}
	if st != 1 {
		t.Errorf("Yes: status = %d, want 1", st)
	}
	passes := permissive()
	passes.GlobNoMatchIsError = No
	out, st = run(t, `echo /zzz_no_such_dir_*`, withSem(passes))
	if !strings.Contains(out, "/zzz_no_such_dir_*") {
		t.Errorf("No: the pattern should pass through, got %q", out)
	}
	if st != 0 {
		t.Errorf("No: status = %d, want 0", st)
	}
}

func TestUnterminatedBracketIsNotAGlob(t *testing.T) {
	// `[` is the test builtin's name. Treating it as a pattern reported
	// "no matches found: [" on every use of `test`, which went unnoticed
	// until an unmatched pattern became fatal and the builtin stopped
	// running. Every bracket policy agrees a lone `[` is literal.
	out, st := run(t, `[ a = a ] && echo yes`, withSem(bracketSem(BracketBadPattern)))
	if out != "yes\n" || st != 0 {
		t.Errorf("got %q status %d, want %q status 0", out, st, "yes\n")
	}
	if got, _ := run(t, `echo [`, withSem(bracketSem(BracketLiteral))); got != "[\n" {
		t.Errorf("echo [: got %q", got)
	}
	// A closed bracket is still a pattern.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	src := `cd ` + dir + `; echo [a]`
	if got, _ := run(t, src, withSem(bracketSem(BracketLiteral))); got != "a\n" {
		t.Errorf("[a] should have matched the file, got %q", got)
	}
}

func TestArithmeticAxesAreAsked(t *testing.T) {
	// ArithNameValueRecurses: a name-shaped value is re-evaluated where the
	// axis says Yes and an error where it says No.
	recurses := permissive()
	recurses.ArithNameValueRecurses = Yes
	if got, _ := run(t, `x=abc; echo $((x+1))`, withSem(recurses)); got != "1\n" {
		t.Errorf("Yes: got %q, want %q", got, "1\n")
	}
	if _, st := run(t, `x=abc; echo $((x+1))`, withSem(PosixSemantics())); st == 0 {
		t.Error("No: a name-shaped value should be an error")
	}
	// ArithInvalidOctalDigitIsError: the permissive base reads a leading
	// zero as octal and refuses a digit past 7.
	if _, st := run(t, `echo $((08))`, withSem(permissive())); st == 0 {
		t.Error("Yes: 08 should be an error")
	}
	tolerant := permissive()
	tolerant.ArithInvalidOctalDigitIsError = No
	if got, _ := run(t, `echo $((08))`, withSem(tolerant)); got != "8\n" {
		t.Errorf("No: got %q, want %q", got, "8\n")
	}
	// Octal *and* tolerant is a measured combination — the reason one bool
	// could not say it.
	if got, _ := run(t, `echo $((0100))`, withSem(tolerant)); got != "64\n" {
		t.Errorf("octal and tolerant 0100: got %q, want %q", got, "64\n")
	}
	decimal := permissive()
	decimal.ArithLeadingZeroIsOctal = No
	if got, _ := run(t, `echo $((0100))`, withSem(decimal)); got != "100\n" {
		t.Errorf("decimal 0100: got %q, want %q", got, "100\n")
	}
}

func TestCoreRefusesTheNewAxes(t *testing.T) {
	for _, src := range []string{`x=abc; echo $((x+1))`, `echo $((08))`} {
		if _, st := run(t, src, withSem(CoreSemantics())); st != 2 {
			t.Errorf("%s under the core: status = %d, want a refusal", src, st)
		}
	}
}

// TestIndirectionMeaningIsAnAxis is the semantics half of the three-way
// `${!x}` divergence; the grammar half is asserted in the syntax package.
// Together they express three answers with two binary questions.
func TestIndirectionMeaningIsAnAxis(t *testing.T) {
	const src = `x=y; y=V; printf "[%s]" "${!x}"`
	indirect := func(d *syntax.Dialect) { d.ParamIndirection = true }
	name := permissive()
	name.IndirectionYieldsName = Yes
	if got, _ := runGrammar(t, src, indirect, withSem(name)); got != "[x]" {
		t.Errorf("Yes yields the name: got %q, want %q", got, "[x]")
	}
	// The other answer reads through it, which is the whole disagreement.
	value := permissive()
	value.IndirectionYieldsName = No
	if got, _ := runGrammar(t, src, indirect, withSem(value)); got != "[V]" {
		t.Errorf("No indirects: got %q, want %q", got, "[V]")
	}
}
