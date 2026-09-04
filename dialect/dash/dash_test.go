// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The measured answers for dash live here, in dash's own package, so that adding
// or correcting a shell never touches the substrate.

func parses(t *testing.T, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, dash.Dialect())
	return err == nil
}

func TestGrammar(t *testing.T) {
	// Expands aliases in a script, with no option to turn on.
	if got, want := dash.Dialect().ExpandAliases, true; got != want {
		t.Errorf("ExpandAliases = %v, want %v", got, want)
	}
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{`a=(x y)`, false},
		{`function f { echo x; }`, false},
		{`echo ${!x}`, false},
		// The array form too, which dash refuses twice over: it has no
		// indirection and no arrays either.
		{`echo ${!a[@]}`, false},
		// Not a syntax error: with the construct absent, `[[` is a command
		// name and this parses. Real dash agrees, and fails at runtime with
		// "[[: not found" and status 127 — the same trap as `&>`, where a
		// missing construct changes the meaning rather than rejecting it.
		{`[[ -n x ]]`, true},
		// It *is* a syntax error once a paren appears, because a paren after
		// a word is one in every shell in the panel.
		{`[[ ( -n x ) ]]`, false},
		{`echo "$((1+1))"`, true},
	} {
		if got := parses(t, tc.src); got != tc.want {
			t.Errorf("%q: parses = %v, want %v", tc.src, got, tc.want)
		}
	}
}

// TestSignalNamesHaveNoSIGPrefix is dash's answer where it can be seen.
//
// The prefix is not part of a signal's name here, and dash says so in three
// places with three wordings — including one that names only the first
// character, because that is where it stopped reading.
func TestSignalNamesHaveNoSIGPrefix(t *testing.T) {
	for _, tc := range []struct {
		src    string
		errs   string
		status int
	}{
		// No shell name in front of it: dash prints `trap`'s complaint bare
		// where it prefixes every `kill` diagnostic it has.
		{`trap 'echo x' SIGUSR1`, "trap: SIGUSR1: bad trap\n", 1},
		{`trap 'echo x' USR1`, "", 0},
		{`kill -SIGCONT $$`, "dash: 1: kill: Illegal option -S\n", 2},
		{`kill -s SIGCONT $$`, "dash: 1: kill: invalid signal number or name: SIGCONT\n", 2},
		{`kill -CONT $$`, "", 0},
	} {
		f, err := syntax.Parse(tc.src, dash.Dialect())
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		var errs bytes.Buffer
		sem, dg := dash.Semantics(), dash.Diagnostics()
		r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "dash"}
		st, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if got := errs.String(); got != tc.errs {
			t.Errorf("%s: stderr = %q, want %q", tc.src, got, tc.errs)
		}
		if st != tc.status {
			t.Errorf("%s: status = %d, want %d", tc.src, st, tc.status)
		}
	}
}

func TestSemantics(t *testing.T) {
	// The same answer bash gives.
	if got, want := dash.Semantics().ExitInTrapReportsEarlierStatus, interp.Yes; got != want {
		t.Errorf("ExitInTrapReportsEarlierStatus = %v, want %v", got, want)
	}
	s := dash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"SetFTurnsOffGlobbing", s.SetFTurnsOffGlobbing, interp.Yes},
		{"DeclaredNameWithoutValueIsEmpty", s.DeclaredNameWithoutValueIsEmpty, interp.No},
		// dash has no `typeset`, so TypesetLocalNeedsKeywordFunction is absent
		// rather than false — the axis does not arise.
		{"TypesetLocalNeedsKeywordFunction", s.TypesetLocalNeedsKeywordFunction, interp.Unspecified},
		{"EchoInterpretsEscapes", s.EchoInterpretsEscapes, interp.Yes},
		{"LengthOfSpecialIsCount", s.LengthOfSpecialIsCount, interp.No},
		{"BraceExpansion", s.BraceExpansion, interp.No},
		{"BracketCaretNegates", s.BracketCaretNegates, interp.No},
		{"FatalErrorStatusIsOne", s.FatalErrorStatusIsOne, interp.No},
		{"SIGPrefixAccepted", s.SIGPrefixAccepted, interp.No},
		// A `jobs` listing: which end it starts from, and whether a job that
		// has already ended appears in it at all. Both split the panel two
		// and two, which is why both are fields.
		{"AnnouncesBackgroundJob", s.AnnouncesBackgroundJob, interp.No},
		{"ReportsACommandKilledBySignal", s.ReportsACommandKilledBySignal, interp.Yes},
		{"ReportsAnyKilledPipelineElement", s.ReportsAnyKilledPipelineElement, interp.Yes},
		{"ChildInterruptEndsTheScript", s.ChildInterruptEndsTheScript, interp.No},
		{"CdRefusesUnknownOption", s.CdRefusesUnknownOption, interp.Yes},
		{"CdLastPathOptionWins", s.CdLastPathOptionWins, interp.Yes},
		{"BadSetOptionNameFatal", s.BadSetOptionNameFatal, interp.Yes},
		{"ReturnOutsideAFunctionIsRefused", s.ReturnOutsideAFunctionIsRefused, interp.No},
		{"LoneDashIsAnOption", s.LoneDashIsAnOption, interp.No},
		{"ReadonlyReassignmentFatalFromCommandString", s.ReadonlyReassignmentFatalFromCommandString, interp.Yes},
		{"ReadonlyReassignmentByDeclarationFatal", s.ReadonlyReassignmentByDeclarationFatal, interp.Yes},
		{"UnsetFunctionChecksTheName", s.UnsetFunctionChecksTheName, interp.No},
		{"UnsetFunctionReportsMissing", s.UnsetFunctionReportsMissing, interp.No},
		// What `type` does: whether it follows the sentence with the
		// function itself, and whether `--` ends its options.
		{"TypePrintsFunctionBody", s.TypePrintsFunctionBody, interp.No},
		{"TypeEndsOptionsWithDashDash", s.TypeEndsOptionsWithDashDash, interp.No},
		{"JobsShowBackgroundCommand", s.JobsShowBackgroundCommand, interp.No},
		{"JobsListNewestFirst", s.JobsListNewestFirst, interp.Yes},
		{"JobsListFinishedJobs", s.JobsListFinishedJobs, interp.Yes},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

func TestDiagnostics(t *testing.T) {
	// The four lines `type` prints, each of them this shell's own words.
	if got, want := dash.Diagnostics().TypeFunction, "%[1]s is a shell function"; got != want {
		t.Errorf("TypeFunction = %q, want %q", got, want)
	}
	if got, want := dash.Diagnostics().TypeNotFound, "%[1]s: not found"; got != want {
		t.Errorf("TypeNotFound = %q, want %q", got, want)
	}
	// A missing name gets no shell name in front of it, and a missing
	// command's status rather than a plain failure.
	if !dash.Diagnostics().TypeNotFoundUnprefixed {
		t.Error("TypeNotFoundUnprefixed = false, want the line written bare")
	}
	if got, want := dash.Diagnostics().TypeNotFoundStatus, 127; got != want {
		t.Errorf("TypeNotFoundStatus = %d, want %d", got, want)
	}
	if got, want := dash.Diagnostics().SyntaxStatus(), 2; got != want {
		t.Errorf("syntax-error status = %d, want %d", got, want)
	}
	// A failed open is reported where the command began, compound or not.
	if got, want := dash.Diagnostics().RedirectFailureLine, interp.LineOfCommand; got != want {
		t.Errorf("RedirectFailureLine = %v, want %v", got, want)
	}
}

// TestDerivesFromTheStandardNotFromASibling is the property the package
// comment promises. A preset that inherits from another shell inherits its
// future mistakes; this one starts from POSIX and overrides only what was
// measured.
func TestDerivesFromTheStandardNotFromASibling(t *testing.T) {
	posix := interp.PosixSemantics()
	s := dash.Semantics()
	if s == posix {
		t.Error("the preset overrides nothing, which cannot be right")
	}
}

// TestUnterminatedNamesWhatWouldHaveClosedIt is dash's view: the expected word
// and the class of what it found instead, never the construct.
func TestUnterminatedNamesWhatWouldHaveClosedIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if", `Syntax error: end of file unexpected (expecting "then")`},
		{"if true; then echo x", `Syntax error: end of file unexpected (expecting "fi")`},
		{"case a in a) echo x", `Syntax error: end of file unexpected (expecting ";;")`},
		{"{ echo x", `Syntax error: end of file unexpected (expecting "}")`},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if got := dash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestArithmeticFailuresAreDashsOwnShape covers the four dash reaches, and the
// one it does not: a digit too great for its base is not a diagnosis dash has
// — the literal ends there and what follows is left over, so it says the same
// thing it says about `$((1 2))`.
func TestArithmeticFailuresAreDashsOwnShape(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo $((1 2))", `arithmetic expression: expecting EOF: "1 2"`},
		{"echo $((1+))", `arithmetic expression: expecting primary: "1+"`},
		{"echo $((1,2))", `arithmetic expression: expecting EOF: "1,2"`},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if got := dash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}

	// `08` parses here and fails when it is evaluated, so it never reaches
	// ParseFailure — and it is the case that says a bad digit is not a
	// diagnosis dash has.
	f, err := syntax.Parse("echo $((08))", dash.Dialect())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var errs bytes.Buffer
	sem, dg := dash.Semantics(), dash.Diagnostics()
	r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "dash"}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := `arithmetic expression: expecting EOF: "08"`; !strings.Contains(errs.String(), want) {
		t.Errorf("got %q, want it to contain %q", errs.String(), want)
	}
}

// TestUnexpectedTokensAreNamedByClass is dash's rule and the reason the
// wording is two fields: an ordinary word is not quoted and not named — it is
// "word" — where a reserved word or an operator is quoted as itself.
func TestUnexpectedTokensAreNamedByClass(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo )", `Syntax error: ")" unexpected`},
		{"if true; echo x; fi", `Syntax error: "fi" unexpected (expecting "then")`},
		{"for i in a b; echo $i; done", `Syntax error: word unexpected (expecting "do")`},
		{"case a in a) echo x;& esac", `Syntax error: "&" unexpected`},
		// The exception to the rule above, and dash's alone: where the
		// unexpected token is itself a redirection operator it is not named
		// at all. Reached by `cat < <(cmd)` — process substitution is not in
		// this dialect, so what dash reads is a redirection whose target is
		// another redirection.
		{"cat < < x", `Syntax error: redirection unexpected`},
		{"cat <(echo hi)", `Syntax error: "(" unexpected`},
		{"echo x > >(cat)", `Syntax error: redirection unexpected`},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if got := dash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestDashSpellsItsOwnReasons covers two answers that are dash's alone.
func TestDashSpellsItsOwnReasons(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			// "No such file", not the C string's "No such file or
			// directory" — dash shortens it, and the reason is quoted by
			// every message that reports a file it could not open.
			"a file that is not there", "cat < nope", "cannot open nope: No such file",
		},
		{
			// The *first* word, where the other three name the one that
			// should have been an operator.
			"a malformed three-argument test", "test a b c", "test: a: unexpected operator",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, dash.Dialect())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var errs bytes.Buffer
			sem, dg := dash.Semantics(), dash.Diagnostics()
			r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "dash", Dir: dir}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.Contains(errs.String(), tc.want) {
				t.Errorf("got %q, want it to contain %q", errs.String(), tc.want)
			}
		})
	}
}

// TestAParenAfterAWordIsAFunctionDefinition is dash committing at the paren,
// which is what decides the token it blames — and it reaches this most often
// through a construct it does not have.
func TestAParenAfterAWordIsAFunctionDefinition(t *testing.T) {
	for _, src := range []string{"f ( x )", "[[ ( -n x ) ]]"} {
		_, err := syntax.Parse(src, dash.Dialect())
		want := `Syntax error: word unexpected (expecting ")")`
		if got := dash.Diagnostics().ParseFailure(err); got != want {
			t.Errorf("%q: got %q, want %q", src, got, want)
		}
	}
}

// TestACasePatternMayBeAnOperator is dash's grammar, not its recovery: the
// first of these runs and prints miss, and the second is the error that comes
// of dash already being past the `&` when it complains.
func TestACasePatternMayBeAnOperator(t *testing.T) {
	if _, err := syntax.Parse("case a in & ) echo hit;; *) echo miss;; esac", dash.Dialect()); err != nil {
		t.Errorf("an operator should stand where a pattern belongs: %v", err)
	}
	_, err := syntax.Parse("case a in a) echo x;;& esac", dash.Dialect())
	want := `Syntax error: word unexpected (expecting ")")`
	if got := dash.Diagnostics().ParseFailure(err); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestPrintfAnswers: dash has no `%q` at all, and its statuses are not one
// answer — a directive it does not have is 2 and an operand that is not a
// number is 1.
func TestPrintfAnswers(t *testing.T) {
	if got, want := dash.Semantics().PrintfQuote, interp.PrintfQuoteAbsent; got != want {
		t.Errorf("PrintfQuote = %v, want %v", got, want)
	}
	d := dash.Diagnostics()
	if got, want := d.PrintfBadVerbStatus, 2; got != want {
		t.Errorf("PrintfBadVerbStatus = %d, want %d", got, want)
	}
	if d.PrintfBadNumberStatus != 0 {
		t.Errorf("PrintfBadNumberStatus = %d, want the substrate's 1", d.PrintfBadNumberStatus)
	}
}

// TestCdAnswers: dash gives no reason and reports 2 where the others report 1.
func TestCdAnswers(t *testing.T) {
	d := dash.Diagnostics()
	if got, want := d.CdCannotChange, "cd: can't cd to %[1]s"; got != want {
		t.Errorf("CdCannotChange = %q, want %q", got, want)
	}
	if got, want := d.CdStatus, 2; got != want {
		t.Errorf("CdStatus = %d, want %d", got, want)
	}
}

// TestGetoptsAnswers: dash prints these with neither a name nor a line, which
// is the only diagnostic in the panel with nothing in front of it.
func TestGetoptsAnswers(t *testing.T) {
	d := dash.Diagnostics()
	if !d.GetoptsUnprefixed {
		t.Error("getopts should print with nothing in front of it")
	}
	if got, want := d.GetoptsBadOption, "Illegal option -%[1]s"; got != want {
		t.Errorf("GetoptsBadOption = %q, want %q", got, want)
	}
}

// TestParametersDashDoesNotProvide is the other half: dash has neither
// RANDOM nor SECONDS, and a script tests for them exactly this way.
func TestParametersDashDoesNotProvide(t *testing.T) {
	for _, name := range []string{"RANDOM", "SECONDS", "UID"} {
		f, err := syntax.Parse(`[ -n "${`+name+`-}" ] && echo have || echo none`, dash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		sem := dash.Semantics()
		r := &interp.Runner{Stdout: &out, Semantics: &sem}
		dash.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out.String()) != "none" {
			t.Errorf("%s: got %q, want dash not to provide it", name, out.String())
		}
	}
}

// TestDashHasNeitherDeclarationName: the core provides `typeset` because
// three of the four have it, and the one that does not takes it away.
func TestDashHasNeitherDeclarationName(t *testing.T) {
	for _, name := range []string{"typeset", "declare"} {
		f, err := syntax.Parse(name+` x=1`, dash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		s, d := dash.Semantics(), dash.Diagnostics()
		r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d}
		dash.Apply(r)
		status, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		if status != 127 {
			t.Errorf("%s: status %d, want 127 for a command dash does not have", name, status)
		}
	}
}

// The words for the signal and nothing else — no process id, no command, and
// alone among everything this shell prints, no name and no line in front.
func TestAKilledCommandIsSaidWithNoLocation(t *testing.T) {
	dg := dash.Diagnostics()
	if got, want := dg.KilledCommandNotice, "%[2]s"; got != want {
		t.Errorf("KilledCommandNotice = %q, want %q", got, want)
	}
	if !dg.KilledCommandNoticeUnprefixed {
		t.Error("this notice carries no location, unlike every other message here")
	}
	if dg.SignalDescriptions != nil {
		t.Error("this shell takes the words for a signal from the machine")
	}
}

// This shell numbers a backquoted body from one and a `$( … )` body from the
// file — two answers for the two spellings of one construct, and it is alone
// in giving them.
func TestABackquotedBodyIsNumberedFromOne(t *testing.T) {
	if !dash.Diagnostics().BackquotedSubstitutionRestartsLines {
		t.Error("a backquoted body is numbered from one here")
	}
}
