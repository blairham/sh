// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// The per-shell answers the interp tests used to assert inline: the interp
// package proves what each axis value does, and this file pins which value
// this preset gives, so a preset edit cannot silently flip one.

// answersRun parses and runs one snippet as this dialect, under `sh` and with
// a PATH, which is the only thing it adds to the shared builder.
//
// The dialect goes to the runner as well as to the parser, and both halves are
// load-bearing. The parser decides what the source *is*; the runner asks
// Runner.Dialect what a pattern means, whether arithmetic has floats, and what
// grammar nested input — a command substitution, an `eval`, a trap body, a
// sourced file — is parsed with. A runner built without one falls back to the
// core, so a test whose whole purpose is to assert this dialect's answer was
// asserting the core's: `[[ $k == a(b|c) ]]` parsed here and then did not
// match (#849, found closing #826). dialecttest.Preset.Runner is now the one
// place that field is set, for every helper in this package.
func answersRun(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "sh", Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		return out + "unsupported: " + err.Error(), -1
	}
	return out, st
}

// TestTheHelperRunsUnderThisDialectAndNotTheCore guards the field answersRun
// sets, which nothing else in this package would miss.
//
// This is the dialect the omission flattered rather than weakened: dash has
// no `[[ ]]`, the core has one, and an `eval` reparses its argument with
// Runner.Dialect. So dash's own suite could run a construct dash does not
// have and see it succeed. Measured, dash on this machine:
//
//	$ dash -c 'eval "[[ a == a ]]"; echo st=$?'
//	dash: 1: eval: [[: not found
//	st=127
//
// 127 is the point: with no such keyword it is an ordinary command name, and
// there is no such command. Without the field it is a keyword and exits 0.
func TestTheHelperRunsUnderThisDialectAndNotTheCore(t *testing.T) {
	out, _ := answersRun(t, `eval '[[ a == a ]]'; echo st=$?`)
	if !strings.Contains(out, "st=127") {
		t.Errorf("answersRun = %q, want a status of 127: this shell has no [[ ]], "+
			"so the runner was not told the dialect", out)
	}
}

func TestAnswersTheInterpAxisTestsRelyOn(t *testing.T) {
	s := dash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"EqualsExpansion", s.EqualsExpansion, interp.No},
		{"LastPipelineElementInCurrentShell", s.LastPipelineElementInCurrentShell, interp.No},
		{"UnsetPositionalIsAllowed", s.UnsetPositionalIsAllowed, interp.No},
		{"LastBackgroundPidIsUnsetBeforeAnyJob", s.LastBackgroundPidIsUnsetBeforeAnyJob, interp.Yes},
		{"LastBackgroundPidIsZeroBeforeAnyJob", s.LastBackgroundPidIsZeroBeforeAnyJob, interp.No},
		{"ExitTrapIsFunctionLocal", s.ExitTrapIsFunctionLocal, interp.No},
		{"ArithNameValueRecurses", s.ArithNameValueRecurses, interp.No},
		{"ArrayBaseIsZero", s.ArrayBaseIsZero, interp.Yes},
		// dash has no subscript to read at all, so both are the preset it
		// inherits rather than a measurement of its own.
		{"SubscriptCommaIsARange", s.SubscriptCommaIsARange, interp.No},
		{"ScalarSubscriptIsACharacter", s.ScalarSubscriptIsACharacter, interp.No},
		{"SplitParamExpansion", s.SplitParamExpansion, interp.Yes},
		{"UnquotedListJoinsOnIFS", s.UnquotedListJoinsOnIFS, interp.No},
		// POSIX makes an unquoted `$@` behave as `$*` where nothing is
		// split, and this shell complies: `IFS=-; set -- x y z; v=${@}` is
		// `x-y-z` here and in zsh, against `x y z` in bash and ksh93.
		{"UnsplitAtListJoinsOnIFS", s.UnsplitAtListJoinsOnIFS, interp.Yes},
		{"TrailingSeparatorEndsAField", s.TrailingSeparatorEndsAField, interp.No},
		{"GlobNoMatchIsError", s.GlobNoMatchIsError, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.Yes},
		{"AssignThroughExpansionMayNameAPositional", s.AssignThroughExpansionMayNameAPositional, interp.No},
		{"ShiftPastEndFatal", s.ShiftPastEndFatal, interp.Yes},
		{"RedirectErrorOnSpecialBuiltinFatal", s.RedirectErrorOnSpecialBuiltinFatal, interp.Yes},
		{"DuplicationTargetErrorOnABuiltinIsFatal", s.DuplicationTargetErrorOnABuiltinIsFatal, interp.No},
		{"InteractiveMonitorNeedsATerminal", s.InteractiveMonitorNeedsATerminal, interp.Yes},
		{"InteractiveScriptAnnouncesJobs", s.InteractiveScriptAnnouncesJobs, interp.Yes},
		{"SubshellRunsOnAfterSignalingTheShell", s.SubshellRunsOnAfterSignalingTheShell, interp.Yes},
		{"UnsetReadonlyFatal", s.UnsetReadonlyFatal, interp.Yes},
		{"MultiDigitDuplicationTargetIsAnError", s.MultiDigitDuplicationTargetIsAnError, interp.Yes},
		{"TraceAssignmentsSeparately", s.TraceAssignmentsSeparately, interp.No},
		{"TraceShowsItsOwnDisabling", s.TraceShowsItsOwnDisabling, interp.Yes},
		{"LocalInheritsTheExportAttribute", s.LocalInheritsTheExportAttribute, interp.Yes},
		{"DeclarationAssignmentClearsTheExportAttribute", s.DeclarationAssignmentClearsTheExportAttribute, interp.No},
		{"UnsetSubscriptOnAScalarIsAnError", s.UnsetSubscriptOnAScalarIsAnError, interp.No},
		{"StdinOptionNamesTheOperands", s.StdinOptionNamesTheOperands, interp.No},
		{"HangupIsAnOrderlyExit", s.HangupIsAnOrderlyExit, interp.No},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
	if got, want := s.UnterminatedBracket, interp.BracketNoMatch; got != want {
		t.Errorf("UnterminatedBracket = %v, want %v", got, want)
	}
	if got, want := s.BackgroundJobInput, interp.BackgroundJobInputEmpty; got != want {
		t.Errorf("BackgroundJobInput = %v, want %v", got, want)
	}
	if got, want := s.StatusArgument, interp.StatusArgStrict; got != want {
		t.Errorf("StatusArgument = %v, want %v", got, want)
	}
	if got, want := s.SubshellJobTable, interp.SubshellJobsCleared; got != want {
		t.Errorf("SubshellJobTable = %v, want %v", got, want)
	}
}

// The invocation answers this preset gives that are not Answers.
func TestPlusSignedCommandStringNaming(t *testing.T) {
	if got := dash.Semantics().PlusSignedCommandStringIsDollarZero; got != false {
		t.Errorf("PlusSignedCommandStringIsDollarZero = %v, want false", got)
	}
}

func TestDiagnosticAnswersTheInterpTestsRelyOn(t *testing.T) {
	d := dash.Diagnostics()
	if got, want := d.TraceQuoting, interp.QuoteNever; got != want {
		t.Errorf("TraceQuoting = %v, want %v", got, want)
	}
	if got, want := d.TraceStyle, interp.TracePlain; got != want {
		t.Errorf("TraceStyle = %v, want %v", got, want)
	}
	if got, want := d.TraceForHeader, interp.TraceForNone; got != want {
		t.Errorf("TraceForHeader = %v, want %v", got, want)
	}
	// This dialect has neither `[[ ]]` nor `(( ))`, so the only compound
	// trace answer it can reach is `case`, and it prints nothing for one.
	if got, want := d.TraceCaseHeader, interp.TraceCaseNone; got != want {
		t.Errorf("TraceCaseHeader = %v, want %v", got, want)
	}
	if got, want := d.Location, interp.LocationColonLine; got != want {
		t.Errorf("Location = %v, want %v", got, want)
	}
	if got := d.SyntaxStatus(); got != 2 {
		t.Errorf("SyntaxStatus() = %d, want 2", got)
	}
	// The two Report routes agree here; only one dialect splits them.
	if d.Report("s", 2, "m") != d.ForScript().Report("s", 2, "m") {
		t.Error("Report should not change between -c and a script")
	}
}

// TestWordings runs the failures whose sentences are this shell's own.
func TestWordings(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"readonly", `readonly r=1; r=2`, "r: is read only"},
		{"not found", `nosuchcommand_xyz`, "nosuchcommand_xyz: not found"},
		{"arithmetic", `echo $((1/0))`, `arithmetic expression: division by zero: "1/0"`},
		{"invalid number", `x=abc; echo $((x+1))`, "Illegal number: abc"},
		{"shift", `shift 5`, "shift: can't shift that many"},
		{"unbound", `set -u; echo "$NOPE"`, "parameter not set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

// A `test` diagnostic is written by the builtin under whichever of its two
// names was typed, so no wording may spell one of them itself.
func TestNoTestWordingSpellsItsOwnName(t *testing.T) {
	d := dash.Diagnostics()
	for _, w := range []struct{ field, text string }{
		{"TestUnaryExpected", d.TestUnaryExpected},
		{"TestBinaryExpected", d.TestBinaryExpected},
		{"TestIntegerExpected", d.TestIntegerExpected},
		{"TestTooManyArguments", d.TestTooManyArguments},
		{"TestOperandExpected", d.TestOperandExpected},
	} {
		if strings.HasPrefix(w.text, "test:") || strings.HasPrefix(w.text, "[:") {
			t.Errorf("%s = %q: names a builtin that may have been called by its other name", w.field, w.text)
		}
	}
	if got := d.TestMissingBracket; got != "" && !strings.ContainsAny(got, "[]") {
		t.Errorf("TestMissingBracket = %q: want a bracket in it", got)
	}
}

// TestALocalCarriesTheExportAttribute: the same answer as bash, reached with
// this shell's own reading of a valueless declaration — `local FOO` leaves the
// caller's value showing through, and it is exported, so the child is told
// `FOO=bar` where bash tells it nothing.
func TestALocalCarriesTheExportAttribute(t *testing.T) {
	out, _ := answersRun(t, `export FOO=bar; f() { local FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`)
	if !strings.Contains(out, "FOO=baz") {
		t.Errorf("got %q, want the child told the local's value", out)
	}
	out, _ = answersRun(t, `export FOO=bar; f() { local FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`)
	if !strings.Contains(out, "FOO=bar") {
		t.Errorf("valueless: got %q, want the outer value showing through", out)
	}
}

// The POSIX rule about a redirection error on a special builtin, asserted as
// behavior rather than only as a field: this is the panel's strictest column
// and the one the standard describes, and it keeps the answer with no mode to
// be in. Status 2, because every fatal error here is 2.
func TestAFailedRedirectionOnASpecialBuiltinEndsTheScript(t *testing.T) {
	out, st := answersRun(t, "exec 3>/nope/x\necho after\n")
	if strings.Contains(out, "after") {
		t.Errorf("out %q, want the script to have stopped at the redirection", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want this shell's fatal status", st)
	}

	// And a command POSIX does not mark special is unaffected, which is the
	// boundary the whole panel agrees on.
	out, st = answersRun(t, "true 3>/nope/x\necho after\n")
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("out %q status %d, want an ordinary command to carry on", out, st)
	}
}

// The one shell in the panel that will not take a duplication target wider
// than one digit. It words the refusal as a syntax error and stops, though
// the parse itself succeeded — `sh -n -c 'echo hi >&10'` accepts the input —
// so what is asserted here is the behavior and not the sentence's category.
func TestAWideDuplicationTargetIsRefused(t *testing.T) {
	out, st := answersRun(t, "echo hi >&10\necho after\n")
	if !strings.Contains(out, "Syntax error: Bad fd number") {
		t.Errorf("out %q, want this shell's wording", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("out %q, want nothing after the redirection to have run", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want this shell's fatal status", st)
	}

	// And a single digit is left to the ordinary descriptor failure, which
	// is survivable here as it is everywhere.
	out, st = answersRun(t, "echo hi >&9\necho after\n")
	if strings.Contains(out, "Bad fd number") {
		t.Errorf("out %q, want one digit past the width question", out)
	}
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("out %q status %d, want the script to have carried on", out, st)
	}
}

// The one shell in the panel with no multibyte decoder: a length is bytes
// here whatever the locale names, where bash, ksh93 and zsh switch on it.
//
// Asserted under a UTF-8 locale, because that is the only one the answer is
// visible in — under `LC_ALL=C` every panel member counts bytes and this
// preset's answer changes nothing. Measured 2026-09-05: dash gives 6 for
// `s=héllo; echo ${#s}` under LC_ALL, LC_CTYPE and LANG alike, and 9 for
// `s=日本語`.
func TestALengthIsBytesInEveryLocale(t *testing.T) {
	for _, locale := range []string{"C", "C.UTF-8", "en_US.UTF-8"} {
		src := "LC_ALL=" + locale + `; s=héllo; t=日本語; printf "[%s][%s]" "${#s}" "${#t}"`
		out, st := answersRun(t, src)
		if out != "[6][9]" || st != 0 {
			t.Errorf("under %s: out %q status %d, want %q at 0", locale, out, st, "[6][9]")
		}
	}
}

// TestAnAssignmentThroughAnExpansionCannotNameAListOrAPositional is #1541.
//
// Measured 2026-09-11. The bare name without a sigil, this shell's own
// `bad variable name` sentence, and the one status in the panel that is not
// 1 — 2, which FatalErrorStatusIsOne already answers rather than a number of
// this refusal's own.
func TestAnAssignmentThroughAnExpansionCannotNameAListOrAPositional(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set --; printf "<%s>" ${@:=abc}`, "@: bad variable name"},
		{`set --; printf "<%s>" ${*:=abc}`, "*: bad variable name"},
		{`set --; printf "<%s>" ${1:=abc}`, "1: bad variable name"},
	} {
		out, st := answersRun(t, tc.src+"\necho AFTER")
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want %q in it", tc.src, out, tc.want)
		}
		if strings.Contains(out, "AFTER") || st != 2 {
			t.Errorf("%s = %q (status %d), want the shell ended at 2", tc.src, out, st)
		}
	}
	out, st := answersRun(t, `set -- p; printf "<%s>" ${@:=abc}`)
	if out != "<p>" || st != 0 {
		t.Errorf("a parameter that is there = %q (status %d), want <p> at 0", out, st)
	}
}
