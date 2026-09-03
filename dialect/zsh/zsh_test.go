// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The measured answers for zsh live here, in zsh's own package, so that adding
// or correcting a shell never touches the substrate.

func parses(t *testing.T, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, zsh.Dialect())
	return err == nil
}

func TestGrammar(t *testing.T) {
	// Does not expand them in a script; the prompt is a different
	// question and the front end answers it.
	if got, want := zsh.Dialect().ExpandAliases, false; got != want {
		t.Errorf("ExpandAliases = %v, want %v", got, want)
	}
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{`case a in a) echo x;;& esac`, false},
		{`echo ${x^^}`, false},
		{`echo ${!x}`, false},
		// The array form is the one scripts reach for — iterating an array by
		// index — and it takes a name and a subscript where the scalar takes
		// only a name. zsh refuses both, and refuses them at *parse* time, so
		// nothing after the line runs. Its own spelling is `${(k)a}`.
		{`echo ${!a[@]}`, false},
		{`echo ${!a[*]}`, false},
		{`function f() { echo x; }`, true},
		{`a=(x y)`, true},
	} {
		if got := parses(t, tc.src); got != tc.want {
			t.Errorf("%q: parses = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestSemantics(t *testing.T) {
	// zsh alone: the trap's own last command decides, so
	// `trap "false; exit" 0` exits 1.
	if got, want := zsh.Semantics().ExitInTrapReportsEarlierStatus, interp.No; got != want {
		t.Errorf("ExitInTrapReportsEarlierStatus = %v, want %v", got, want)
	}
	s := zsh.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"SetFTurnsOffGlobbing", s.SetFTurnsOffGlobbing, interp.No},
		{"ArithIntegerOperatorRefusesFloat", s.ArithIntegerOperatorRefusesFloat, interp.No},
		{"AssignmentUpdatesPipelineStatus", s.AssignmentUpdatesPipelineStatus, interp.No},
		{"UnsetEndsTheProducedPipelineStatus", s.UnsetEndsTheProducedPipelineStatus, interp.Yes},
		{"ArrayScalarIsTheWholeArray", s.ArrayScalarIsTheWholeArray, interp.Yes},
		{"SelectPromptNeedsTerminal", s.SelectPromptNeedsTerminal, interp.No},
		{"SelectEofIsSuccess", s.SelectEofIsSuccess, interp.Yes},
		{"SelectTakesUnterminatedReply", s.SelectTakesUnterminatedReply, interp.Yes},
		{"SelectEofPrintsNewline", s.SelectEofPrintsNewline, interp.No},
		{"SelectEofEndsPromptLine", s.SelectEofEndsPromptLine, interp.Yes},
		{"SelectAssumesUnboundedWidth", s.SelectAssumesUnboundedWidth, interp.Yes},
		{"DeclaredNameWithoutValueIsEmpty", s.DeclaredNameWithoutValueIsEmpty, interp.Yes},
		{"TypesetLocalNeedsKeywordFunction", s.TypesetLocalNeedsKeywordFunction, interp.No},
		{"SplitParamExpansion", s.SplitParamExpansion, interp.No},
		{"ArrayBaseIsZero", s.ArrayBaseIsZero, interp.No},
		{"GlobNoMatchIsError", s.GlobNoMatchIsError, interp.Yes},
		{"ArithLeadingZeroIsOctal", s.ArithLeadingZeroIsOctal, interp.No},
		{"ReadonlyReassignmentFatal", s.ReadonlyReassignmentFatal, interp.Yes},
		{"DollarZeroInFunctionIsFunctionName", s.DollarZeroInFunctionIsFunctionName, interp.Yes},
		// A `jobs` listing: which end it starts from, and whether a job that
		// has already ended appears in it at all. Both split the panel two
		// and two, which is why both are fields.
		// Whether an unassigned subscript is an element.
		{"ArraysAreSparse", s.ArraysAreSparse, interp.No},
		{"AnnouncesBackgroundJob", s.AnnouncesBackgroundJob, interp.Yes},
		{"ReportsACommandKilledBySignal", s.ReportsACommandKilledBySignal, interp.No},
		{"CdRefusesUnknownOption", s.CdRefusesUnknownOption, interp.No},
		{"CdLastPathOptionWins", s.CdLastPathOptionWins, interp.No},
		// What `type` does: whether it follows the sentence with the
		// function itself, and whether `--` ends its options.
		{"TypePrintsFunctionBody", s.TypePrintsFunctionBody, interp.No},
		{"TypeEndsOptionsWithDashDash", s.TypeEndsOptionsWithDashDash, interp.Yes},
		{"JobsShowBackgroundCommand", s.JobsShowBackgroundCommand, interp.Yes},
		{"JobsListNewestFirst", s.JobsListNewestFirst, interp.No},
		{"JobsListFinishedJobs", s.JobsListFinishedJobs, interp.No},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

func TestDiagnostics(t *testing.T) {
	// The four lines `type` prints, each of them this shell's own words.
	if got, want := zsh.Diagnostics().TypeKeyword, "%[1]s is a reserved word"; got != want {
		t.Errorf("TypeKeyword = %q, want %q", got, want)
	}
	if got, want := zsh.Diagnostics().TypeFunction, "%[1]s is a shell function from zsh"; got != want {
		t.Errorf("TypeFunction = %q, want %q", got, want)
	}
	if !zsh.Diagnostics().TypeNotFoundUnprefixed {
		t.Error("TypeNotFoundUnprefixed = false, want the line written bare")
	}
	if got, want := zsh.Diagnostics().SyntaxStatus(), 1; got != want {
		t.Errorf("syntax-error status = %d, want %d", got, want)
	}
	// The same answer dash gives: where the command began.
	if got, want := zsh.Diagnostics().RedirectFailureLine, interp.LineOfCommand; got != want {
		t.Errorf("RedirectFailureLine = %v, want %v", got, want)
	}
}

// TestDerivesFromTheStandardNotFromASibling is the property the package
// comment promises. A preset that inherits from another shell inherits its
// future mistakes; this one starts from POSIX and overrides only what was
// measured.
// TestUnknownSignalIsNamedWithOnePrefix pins a wording that only reads right
// because of which verb it is given.
//
// zsh names a signal it does not know with exactly one SIG in front of it,
// however many the operand arrived with: `Q` is SIGQ and `SIGNOPE` is
// SIGNOPE. A format that added one to the operand as written produced
// SIGSIGNOPE for the second.
func TestUnknownSignalIsNamedWithOnePrefix(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`kill -Q 1`, "unknown signal: SIGQ"},
		{`kill -SIGNOPE 1`, "unknown signal: SIGNOPE"},
		{`kill -s signope 1`, "unknown signal: SIGNOPE"},
	} {
		f, err := syntax.Parse(tc.src, zsh.Dialect())
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		var errs bytes.Buffer
		sem, dg := zsh.Semantics(), zsh.Diagnostics()
		r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "zsh"}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if got := errs.String(); !strings.Contains(got, tc.want) {
			t.Errorf("%s: got %q, want it to contain %q", tc.src, got, tc.want)
		}
	}
}

// TestWhatZshSaysAndWhereItSaysIt covers three wordings that were each wrong
// in a different way, and are only checkable against zsh's own vector.
func TestWhatZshSaysAndWhereItSaysIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// Measured against the zsh the panel resolves. 5.9.2 from
			// Homebrew says -L; Apple's /bin/zsh 5.9 says -l, and probing
			// whichever came first on PATH is how the wrong one shipped.
			"the hint names -L", `kill -Q 1`,
			"type kill -L for a list of signals",
		},
		{
			// zsh is the only shell in the panel that says anything about a
			// shift it survives.
			"a survivable shift still complains", `set -- a; shift 5`,
			"shift count must be <= $#",
		},
		{
			// No `exec` segment: a command that could not be found is the
			// shell's failure rather than the builtin's, and zsh reports it
			// exactly as it reports a bare command word.
			"exec does not name itself", `exec nosuchcmd-xyz`,
			"zsh:1: command not found: nosuchcmd-xyz",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, zsh.Dialect())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var errs bytes.Buffer
			sem, dg := zsh.Semantics(), zsh.Diagnostics()
			r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "zsh"}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := errs.String(); !strings.Contains(got, tc.want) {
				t.Errorf("got %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

func TestDerivesFromTheStandardNotFromASibling(t *testing.T) {
	posix := interp.PosixSemantics()
	s := zsh.Semantics()
	if s == posix {
		t.Error("the preset overrides nothing, which cannot be right")
	}
}

// TestUnterminatedNamesTheLastTokenRead is zsh's view, which mentions neither
// the construct nor what would have closed it.
func TestUnterminatedNamesTheLastTokenRead(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if", "parse error near `if'"},
		{"if true; then echo x", "parse error near `x'"},
		{"case a in a) echo hi", "parse error near `hi'"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestBorrowedTextReplacesTheShellName is zsh's answer to both halves of the
// naming question, and it calls eval's text something of its own.
func TestBorrowedTextReplacesTheShellName(t *testing.T) {
	d := zsh.Diagnostics()
	for _, tc := range []struct {
		what string
		got  interp.SourceNaming
	}{{"eval", d.EvalNaming}, {"a sourced file", d.SourceFileNaming}} {
		if tc.got != interp.SourceReplacesShell {
			t.Errorf("%s: got %v, want it to replace the shell's name", tc.what, tc.got)
		}
	}
	if got, want := d.EvalSourceName, "(eval)"; got != want {
		t.Errorf("eval is called %q, want %q", got, want)
	}
}

// TestArithmeticFailuresQuoteNothing is zsh's shape: it does not quote the
// expression at all, and it puts the token inside the reason.
func TestArithmeticFailuresQuoteNothing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo $((1 2))", "bad math expression: operator expected at `2'"},
		{"echo $((1+))", "bad math expression: operand expected at end of string"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestWhoIsSpeakingInAConditionSplitsByWhenItFailed pins a distinction that
// looks like an inconsistency until the mechanism shows.
//
// zsh names the builtin in the location for `test -Q x` and does not for
// `test a b c` — the same builtin, two lines apart. `[[ a b c ]]` gives the
// identical wording with no name, and `[[ ]]` is not a builtin at all: an
// expression that never *parsed* is the condition parser's complaint, and one
// that failed while being *evaluated* is the builtin's.
func TestWhoIsSpeakingInAConditionSplitsByWhenItFailed(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"test a b c", "zsh:1: condition expected: b"},
		{"test -Q x", "zsh:test:1: unknown condition: -Q"},
		{"test 1 -eq a", "zsh:test:1: integer expression expected: a"},
		{"[ x", "zsh:[:1: ']' expected"},
	} {
		f, err := syntax.Parse(tc.src, zsh.Dialect())
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		var errs bytes.Buffer
		sem, dg := zsh.Semantics(), zsh.Diagnostics()
		r := &interp.Runner{Stderr: &errs, Semantics: &sem, Diagnostics: &dg, Name: "zsh"}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if got := strings.TrimSpace(errs.String()); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestAPatternThatIsNotOneNamesTheToken is the case-arm half of the
// unexpected-token family: `;;&` is not zsh's, so it stops at the `&`.
func TestAPatternThatIsNotOneNamesTheToken(t *testing.T) {
	_, err := syntax.Parse("case a in a) echo x;;& esac", zsh.Dialect())
	if got, want := zsh.Diagnostics().ParseFailure(err), "parse error near `&'"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestEveryTargetIsWritten is zsh's multios, and the axis nobody else sets.
func TestEveryTargetIsWritten(t *testing.T) {
	if got, want := zsh.Semantics().RedirectsWriteToEveryTarget, interp.Yes; got != want {
		t.Errorf("got %v, want a command's output in every file it names", got)
	}
}

// TestPrintfAnswers: zsh stops the output at `\c`, which is the answer that
// looks like ksh93's until the bytes are read.
func TestPrintfAnswers(t *testing.T) {
	s := zsh.Semantics()
	if got, want := s.PrintfBackslashC, interp.PrintfBackslashCStops; got != want {
		t.Errorf("PrintfBackslashC = %v, want %v", got, want)
	}
	if got, want := s.PrintfReportsBadNumber, interp.No; got != want {
		t.Errorf("PrintfReportsBadNumber = %v, want %v", got, want)
	}
}

// TestCdAnswers: zsh moves silently and names the reason before the operand,
// which is the reverse of everyone else.
func TestCdAnswers(t *testing.T) {
	s, d := zsh.Semantics(), zsh.Diagnostics()
	if got, want := s.CdDashPrintsTheDirectory, interp.No; got != want {
		t.Errorf("CdDashPrintsTheDirectory = %v, want %v", got, want)
	}
	if got, want := s.CdWithoutHomeIsAnError, interp.No; got != want {
		t.Errorf("CdWithoutHomeIsAnError = %v, want %v", got, want)
	}
	if got, want := d.CdCannotChange, "%[2]s: %[1]s"; got != want {
		t.Errorf("CdCannotChange = %q, want %q", got, want)
	}
}

// TestGetoptsAnswers: zsh is the one that empties OPTARG rather than unsetting
// it, and the one that carries on inside a word when OPTIND is assigned.
func TestGetoptsAnswers(t *testing.T) {
	s := zsh.Semantics()
	if got, want := s.GetoptsClearsOptarg, interp.Yes; got != want {
		t.Errorf("GetoptsClearsOptarg = %v, want %v", got, want)
	}
	if got, want := s.GetoptsAssignmentRestartsWord, interp.No; got != want {
		t.Errorf("GetoptsAssignmentRestartsWord = %v, want %v", got, want)
	}
}

// TestSelectMenuLayout: zsh packs the menu into columns always, so even three
// items share a line, and it asks with the same two characters as the others
// in the other order.
func TestSelectMenuLayout(t *testing.T) {
	if got, want := zsh.Semantics().SelectLayout, interp.SelectMenuColumns; got != want {
		t.Errorf("SelectLayout = %v, want %v", got, want)
	}
	if got, want := zsh.Diagnostics().SelectPrompt, "?# "; got != want {
		t.Errorf("SelectPrompt = %q, want %q", got, want)
	}
}

// TestPipelineStatusName: zsh's name for the same record is the lowercase one,
// and the uppercase name is nothing here.
func TestPipelineStatusName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`false | true | false; echo "[${pipestatus[@]}]"`, "[1 0 1]"},
		{`false | true | false; echo "[${PIPESTATUS[@]}]"`, "[]"},
	} {
		f, err := syntax.Parse(tc.src, zsh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		s, d := zsh.Semantics(), zsh.Diagnostics()
		r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d}
		zsh.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out.String()) != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, out.String(), tc.want)
		}
	}
}

// TestCloseBraceIsReservedEverywhere: the grammar rule that makes zsh's brace
// groups look different from the other three. It is documented in
// docs/spec/semantics.md as measured, and was measured and never built — the
// parser rejected `{ echo hi }`, which zsh runs.
func TestCloseBraceIsReservedEverywhere(t *testing.T) {
	if !zsh.Dialect().CloseBraceAlwaysReserved {
		t.Error("zsh should reserve `}` wherever a word may stand")
	}
	f, err := syntax.Parse(`{ echo hi }`, zsh.Dialect())
	if err != nil {
		t.Fatalf("zsh should accept a group with no terminator: %v", err)
	}
	var out bytes.Buffer
	s, d := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d}
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "hi" {
		t.Errorf("got %q, want hi", out.String())
	}
	// And the other half of the same rule.
	if _, err := syntax.Parse(`echo }`, zsh.Dialect()); err == nil {
		t.Error("`echo }` should be a syntax error where `}` is always reserved")
	}
}

// TestFloatFormatting: zsh shows seventeen significant digits and keeps a
// point on a whole float, so the same arithmetic reads differently from
// ksh93's.
func TestFloatFormatting(t *testing.T) {
	if !zsh.Dialect().ArithFloat {
		t.Error("zsh has floating point")
	}
	d := zsh.Diagnostics()
	if got, want := d.ArithFloatDigits, 17; got != want {
		t.Errorf("ArithFloatDigits = %d, want %d", got, want)
	}
	if !d.ArithFloatKeepsPoint {
		t.Error("zsh keeps a point on a whole float")
	}
	if got, want := d.ArithInfinity, "Inf"; got != want {
		t.Errorf("ArithInfinity = %q, want %q", got, want)
	}
}

// TestPatternGroups: which groups this shell reads, and where.
func TestPatternGroups(t *testing.T) {
	if zsh.Dialect().ExtendedPattern || zsh.Dialect().ExtendedPatternInCondition {
		t.Error("zsh has no extended patterns; a quantifier before a group is an ordinary character")
	}
	if !zsh.Dialect().PatternAlternation {
		t.Error("zsh takes a bare group")
	}
}

// TestACommandStringIsReadWhole: zsh parses all of a `-c` command before
// running any of it, where the other three run each line as they reach it. A
// script is read a line at a time in all four, so this is about the command
// string alone.
func TestACommandStringIsReadWhole(t *testing.T) {
	if !zsh.Diagnostics().CommandStringParsedWhole {
		t.Error("zsh reads a command string whole")
	}
}

// TestTheRefusalOfANonBuiltinNamesNoBuiltin: zsh names the speaking builtin in
// a diagnostic's location — `zsh:cd:1:`, `zsh:shift:1:` — and does not here.
// The message is about a name that is *not* a builtin, so there is no builtin
// speaking, and naming `builtin` there would be naming the wrong one.
func TestTheRefusalOfANonBuiltinNamesNoBuiltin(t *testing.T) {
	f, err := syntax.Parse(`builtin ls`, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	s, d := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d}
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(out.String())
	if !strings.Contains(got, "no such builtin: ls") {
		t.Errorf("got %q, want zsh's wording", got)
	}
	if strings.Contains(got, ":builtin:") {
		t.Errorf("got %q, want no builtin named in the location", got)
	}
	// The contrast: a builtin that really is speaking is named.
	out.Reset()
	f, err = syntax.Parse(`shift 99`, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	r = &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d}
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, ":shift:") {
		t.Errorf("got %q, want the speaking builtin named", got)
	}
}

// This shell says nothing at all about a signal that ended a command, so it
// has no wording for one.
func TestAKilledCommandIsNotSaidAtAll(t *testing.T) {
	if got := zsh.Diagnostics().KilledCommandNotice; got != "" {
		t.Errorf("KilledCommandNotice = %q, want nothing — this shell stays quiet", got)
	}
}

// A substitution's body is numbered from the file, whichever way it is
// written.
func TestASubstitutionsBodyIsNumberedFromTheFile(t *testing.T) {
	if zsh.Diagnostics().BackquotedSubstitutionRestartsLines {
		t.Error("backquotes are numbered from the file here, like $( )")
	}
}
