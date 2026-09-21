// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// These tests name axes and never shells, which is the rule for this package:
// a test that asserts what bash does belongs in dialect/bash. What is asserted
// here is that each axis is *reachable* and that the substrate refuses when no
// dialect answered.

// sourceRun runs src with the axes a test chooses, in a directory of its own so
// a `.` looking at the current directory cannot see the developer's.
func sourceRun(t *testing.T, dir, src string, sem Semantics, dg Diagnostics) (string, int) {
	t.Helper()
	return sourceRunWith(t, dir, src, sem, dg, nil)
}

// sourceRunWith is the same with a hand on the Runner before it starts, for
// the switches a session holds rather than the vectors a dialect does.
func sourceRunWith(t *testing.T, dir, src string, sem Semantics, dg Diagnostics, setup func(*Runner)) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
	})
	// PATH is set explicitly rather than inherited: a `.` that found something
	// on the developer's PATH would pass here and fail on a runner.
	r.Vars = map[string]string{"PATH": dir}
	if setup != nil {
		setup(r)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

// permissive is every axis this file needs answered, set to the answer that
// does the most rather than the least, so a test that cares about one axis is
// not tripped by another going unanswered.
func permissive() Semantics {
	s := PosixSemantics()
	// The two questions a *subscripted* declaration and an exported compound
	// raise, at bash's answers, which is the floor these suites assert
	// against. The tests that are *about* them set both sides themselves —
	// see exportedcompound_test.go (#1380).
	s.ExportedCompoundReachesAChildAsItsFirstValue = No
	s.SubscriptedOperandCarriesTheAttributes = Yes
	// The reach a negative subscript makes past an array's first element, at
	// the silent reading — which is what every suite here counting off the
	// end of an array on the way to something else was written against. The
	// suite that is *about* the reach answers all three itself; see
	// interp/subscriptbeforestart_test.go (#3406).
	s.SubscriptBeforeTheFirstElementRead = SubscriptBeforeStartIsNothing
	s.SubscriptBeforeTheFirstElementNeedsAnElement = No
	// What a traced assignment shows of its own elements and its subscript,
	// at bash's answers — the words as written, which is what every suite
	// here that merely traces an array on its way to something else expects.
	// The suite that is *about* them sets both sides; see
	// interp/xtracearrayvalue_test.go (#1959).
	s.TraceArrayLiteralShowsTheExpandedElements = No
	s.TraceElementSubscriptIsEvaluated = No
	// `readonly`'s kind letters, which POSIX does not give it and two of the
	// panel's shells refuse outright — see Semantics.ReadonlyOptions. A suite
	// asking what `readonly -a` *does* needs the letter to exist first, and
	// the suite that asks whether it exists sets the field itself (#2277).
	s.ReadonlyOptions = "paAf"
	// Which line a `case` subject reads, at the answer five of the six give:
	// the `case`'s own. A suite about a *failed* subject reports that
	// failure's location, so the question is live for every such row, and
	// the one suite that is about the axis sets it itself.
	s.CaseSubjectKeepsThePreviousLine = No
	// What a listing does with a name a declaration brought into being with
	// no letters and no value, at bash's answer: it writes a row. Every
	// suite here that lists a `local x` back expects one — see
	// TestBareLocalLists — and the suite that is *about* the axis runs both
	// sides itself (#2999).
	s.ValuelessDeclarationRecordsTheName = Yes
	// What an assignment prefix does to the export attribute of the name it
	// stands in front of at a *builtin*: the leave-alone reading, which is
	// four of the six columns and is the one that changes nothing about a
	// suite asking something else. The suite that is about the axis sets it
	// itself — see interp/prefixbuiltinexport_test.go (#3437).
	s.PrefixExportAtABuiltin = PrefixExportAtABuiltinUnchanged
	// And whether a declaration keeps the value its own prefix set. No,
	// which is five of the six and is the answer every suite here that
	// merely writes `x=1 readonly y` already expects.
	s.DeclarationPromotesThePrefixEntry = No
	s.BuiltinSyntaxErrorFatal = No
	s.DotMissingFileFatal = No
	s.DotWithNoOperandIsAnError = Yes
	s.DotPassesArguments = Yes
	s.DotFallsBackToCurrentDirectory = No
	// The majority answer, so a test that is not about the lone dash gets
	// the operand three of the four would pass on.
	s.LoneDashIsAnOption = No
	// Likewise for what `unset -f` says: three of the four say nothing
	// about either question, so a test that is not about those gets silence.
	s.UnsetFunctionChecksTheName = No
	s.UnsetFunctionReportsMissing = No
	// A redirection that will not open is fatal on a special builtin under
	// POSIX, and `exec` is one. A test asking what a *redirection* did needs
	// the shell still running to answer, so the permissive answer here is
	// the one that carries on; redirfatal_test.go asks the axis itself.
	s.RedirectErrorOnSpecialBuiltinFatal = No
	// The two kinds of alias one dialect has. No, because it is the answer
	// three of the four give and because it is the one that changes nothing
	// about a builtin's letters — a test not about the kinds should see the
	// plain `alias` and the plain `unalias`.
	s.GlobalAliases = No
	s.SuffixAliases = No
	// The letters that go with them, for the same reason: a test that is not
	// about `alias -L`, `-r`, `-m` or the plus forms should see the plain
	// builtins, and three of the four dialects have none of them.
	s.AliasListsAsDefinitions = No
	s.AliasRestrictsToRegularKind = No
	s.AliasOperandsCanBePatterns = No
	s.AliasPlusPrintsNamesOnly = No
	// And whether `type` speaks about an alias only while aliases expand:
	// three of the four answer from the table whatever the switch says.
	s.TypeNamesAnAliasOnlyWhenExpanded = No
	// The two an assignment prefix in front of a *function* raises, at the
	// answer six of the seven panel columns give: gone when the call
	// returns, and exported while it runs. A test that writes `v=9 f` on the
	// way to something else should see the majority reading rather than a
	// refusal, and the suite that is *about* them sets both sides — see
	// assignprefix_test.go (#2407).
	s.AssignmentPrefixPersistsAfterAFunction = No
	s.PrefixToAFunctionIsExported = Yes
	// Whether an attribute letter over a frozen name is refused before any
	// of the declaration happens, at the answer three of the four panel
	// columns give — and the quiet one, since Yes is a refusal with a
	// wording a suite about something else has not set. The suite that is
	// about it answers the axis itself; see
	// interp/frozenattribute_test.go (#2561).
	s.AttributeOverAFrozenNameIsRefused = No
	// And who can see a name a caller declared local, at the answer five of
	// the six panel columns give. A test that nests two calls on the way to
	// something else should see the dynamic reading, and the suite that is
	// about the axis sets both sides — see staticscope_test.go (#2865).
	s.CallerLocalsReachTheCallee = Yes
	return s
}

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestBorrowedTextReportsItsOwnLine pins where a parse failure inside `.` or
// `eval` says it happened.
//
// The line that matters is the one inside the borrowed text, not the line the
// builtin was called on — every shell in the panel reports the former, and
// this reported the latter for all of them. The wording is the substrate's
// here; what each dialect does with the same number is measured in dialect/.
func TestBorrowedTextReportsItsOwnLine(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "late.sh", "echo one\necho two\nif\n")

	// The `.` is on line 3 of the caller and the failure is on line 4 of the
	// file, so a test that got them the wrong way round could not pass by
	// accident.
	out, _ := sourceRun(t, dir, "echo a\necho b\n. ./late.sh\n", permissive(),
		Diagnostics{Location: LocationLineWord, Unterminated: "unfinished %[1]s"})
	if !strings.Contains(out, "line 4: ") {
		t.Errorf("got %q, want the failure's own line 4", out)
	}
	if strings.Contains(out, "line 3: ") {
		t.Errorf("got %q, want the file's line rather than the caller's", out)
	}
}

// TestEndOfInputWithNothingOpenHasItsOwnWording: a dialect that names the
// construct in its end-of-input sentence needs something else to say when
// there is no construct — `f()` with the parens already closed — so that is a
// field rather than the same sentence printed with an empty hole in it. A
// dialect that leaves it empty keeps the one wording it already had.
func TestEndOfInputWithNothingOpenHasItsOwnWording(t *testing.T) {
	dir := t.TempDir()
	both := Diagnostics{
		Location: LocationColonLine, Unterminated: "unfinished %[1]s",
		UnterminatedNoConstruct: "gave out with nothing open",
	}
	if out, _ := sourceRun(t, dir, `eval "f()"`, permissive(), both); !strings.Contains(out, "gave out with nothing open") {
		t.Errorf("got %q, want the construct-less wording", out)
	}
	if out, _ := sourceRun(t, dir, `eval "if"`, permissive(), both); !strings.Contains(out, "unfinished if") {
		t.Errorf("got %q, want the construct named where there is one", out)
	}
	only := Diagnostics{Location: LocationColonLine, Unterminated: "unfinished %[1]s"}
	if out, _ := sourceRun(t, dir, `eval "f()"`, permissive(), only); !strings.Contains(out, "unfinished ") {
		t.Errorf("got %q, want the one wording the dialect has", out)
	}
}

// TestBorrowedTextIsNamedWhereTheDialectNamesIt covers the three shapes, and
// the reason there are two fields rather than one: `eval` and a sourced file
// are two questions, and a dialect may answer them differently.
func TestBorrowedTextIsNamedWhereTheDialectNamesIt(t *testing.T) {
	dir := t.TempDir()
	// The name in the diagnostic is the operand as the shell constructed it —
	// `./p.sh` as written here — never the absolute path it resolved to.
	// Measured: `. ./bad.sh` from a script is reported as `./bad.sh` however
	// deep the directory it really lives in.
	write(t, dir, "p.sh", "if\n")
	const path = "./p.sh"
	const src = ". " + path

	for _, tc := range []struct {
		name string
		dg   Diagnostics
		want string
	}{
		{
			"after the location", Diagnostics{
				Location: LocationColonLine, Unterminated: "unfinished",
				SourceFileNaming: SourceAfterLocation,
			}, "testsh: 2: " + path + ": unfinished\n",
		},
		{
			"before it", Diagnostics{
				Location: LocationLineWord, Unterminated: "unfinished",
				SourceFileNaming: SourceBeforeLocation,
			}, "testsh: " + path + ": line 2: unfinished\n",
		},
		{
			"instead of the shell", Diagnostics{
				Location: LocationTightLine, Unterminated: "unfinished",
				SourceFileNaming: SourceReplacesShell,
			}, path + ":2: unfinished\n",
		},
		{
			// The builtin that read the file rather than the file itself.
			"as the builtin", Diagnostics{
				Location: LocationLineWord, Unterminated: "unfinished",
				SourceFileNaming: SourceBeforeLocation, SourceFileIsTheBuiltin: true,
			}, "testsh: .: line 2: unfinished\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := sourceRun(t, dir, src, permissive(), tc.dg); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}

	// eval is asked separately, and can be named something else entirely.
	dg := Diagnostics{
		Location: LocationTightLine, Unterminated: "unfinished",
		EvalNaming: SourceReplacesShell, EvalSourceName: "(eval)",
		SourceFileNaming: SourceAfterLocation,
	}
	if out, _ := sourceRun(t, dir, `eval "if"`, permissive(), dg); out != "(eval):1: unfinished\n" {
		t.Errorf("eval: got %q, want the name eval was given", out)
	}
}

// TestEvalRunsInTheCallingShell is the whole point of eval being a builtin
// rather than a command: a child process could not do this.
func TestEvalRunsInTheCallingShell(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an assignment survives", `x=1; eval "x=2"; echo $x`, "2\n"},
		{"a function survives", `eval "f() { echo in-f; }"; f`, "in-f\n"},
		{
			// The reason eval exists. The text is expanded once as a word and
			// again as a script, so `$$a` reaches the second pass as `$b`.
			"the text is expanded twice", `a=b; b=hi; eval "echo \$$a"`, "hi\n",
		},
		{"arguments are rejoined with a space", `eval echo a b c`, "a b c\n"},
		{"eval nests", `eval eval "echo deep"`, "deep\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := sourceRun(t, t.TempDir(), tc.src, permissive(), Diagnostics{})
			if st != 0 {
				t.Fatalf("status %d, output %q", st, out)
			}
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
		})
	}
}

// TestEvalWithNothingToRunReportsSuccess is a case that reads like it should
// leave the status alone and does not: an eval with no text clears a failure.
func TestEvalWithNothingToRunReportsSuccess(t *testing.T) {
	for _, src := range []string{
		`false; eval; echo st=$?`,
		`false; eval ""; echo st=$?`,
		`false; eval "# only a comment"; echo st=$?`,
	} {
		out, _ := sourceRun(t, t.TempDir(), src, permissive(), Diagnostics{})
		if strings.TrimSpace(out) != "st=0" {
			t.Errorf("%s gave %q, want st=0", src, strings.TrimSpace(out))
		}
	}
}

// TestEvalIsTransparentToControlFlow pins the half of eval that is easy to get
// backwards. eval is not a scope: `return` inside it returns from the function
// around it and `break` breaks the loop around it.
func TestEvalIsTransparentToControlFlow(t *testing.T) {
	out, st := sourceRun(t, t.TempDir(),
		`f() { eval return 3; echo NOT-REACHED; }; f; echo st=$?`,
		permissive(), Diagnostics{})
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("eval return should leave the function: %q", out)
	}
	if strings.TrimSpace(out) != "st=3" {
		t.Errorf("output = %q, want st=3 (status %d)", strings.TrimSpace(out), st)
	}

	out, _ = sourceRun(t, t.TempDir(),
		`for i in 1 2 3; do eval break; echo NOT-REACHED; done; echo done`,
		permissive(), Diagnostics{})
	if strings.TrimSpace(out) != "done" {
		t.Errorf("eval break should leave the loop: %q", out)
	}
}

// TestDotIsAScopeForReturnAndEvalIsNot is the difference between the two, and
// it is the only one in how they treat control flow.
func TestDotIsAScopeForReturnAndEvalIsNot(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.sh", "echo one\nreturn 5\necho NOT-REACHED\n")

	out, _ := sourceRun(t, dir, `. `+filepath.Join(dir, "r.sh")+`; echo st=$?`,
		permissive(), Diagnostics{})
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("return should end the sourced file: %q", out)
	}
	if got := strings.TrimSpace(out); got != "one\nst=5" {
		t.Errorf("output = %q, want \"one\\nst=5\"", got)
	}
}

func TestDotRunsInTheCallingShell(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "v.sh", "x=7\n")
	out, st := sourceRun(t, dir, `. `+filepath.Join(dir, "v.sh")+`; echo $x`,
		permissive(), Diagnostics{})
	if st != 0 || strings.TrimSpace(out) != "7" {
		t.Errorf("output = %q status %d, want 7", out, st)
	}
}

// TestSourcingAnEmptyFileClearsTheStatus is the same surprise as an empty
// eval, and worth its own case because "the status of the last command" reads
// like it should preserve one when there is no last command.
func TestSourcingAnEmptyFileClearsTheStatus(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "empty.sh", "")
	out, _ := sourceRun(t, dir, `false; . `+filepath.Join(dir, "empty.sh")+`; echo st=$?`,
		permissive(), Diagnostics{})
	if strings.TrimSpace(out) != "st=0" {
		t.Errorf("output = %q, want st=0", strings.TrimSpace(out))
	}
}

// TestDotSearchesPathBeforeTheCurrentDirectory covers the rule that surprises
// people: a `.` operand with no slash is a PATH lookup, and PATH wins over a
// file of the same name next to you.
func TestDotSearchesPathBeforeTheCurrentDirectory(t *testing.T) {
	pathDir, cwd := t.TempDir(), t.TempDir()
	write(t, pathDir, "amb.sh", "echo from-path\n")
	write(t, cwd, "amb.sh", "echo from-cwd\n")

	sem := permissive()
	sem.DotFallsBackToCurrentDirectory = Yes
	f, err := syntax.Parse(`. amb.sh`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: cwd, Name: "testsh", Vars: map[string]string{"PATH": pathDir},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "from-path" {
		t.Errorf("output = %q, want from-path: PATH must win over the cwd", got)
	}
}

// TestDotFallsBackToTheCurrentDirectoryOnlyWhenAsked pins the axis in both
// directions, which is what makes it an axis rather than a behavior.
func TestDotFallsBackToTheCurrentDirectoryOnlyWhenAsked(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fall  Answer
		found bool
	}{
		{"with the fallback the file is found", Yes, true},
		{"without it the same file is not", No, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cwd, empty := t.TempDir(), t.TempDir()
			write(t, cwd, "here.sh", "echo found-in-cwd\n")

			sem := permissive()
			sem.DotFallsBackToCurrentDirectory = tc.fall
			f, err := syntax.Parse(`. here.sh`, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			dg := Diagnostics{}
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
				Dir: cwd, Name: "testsh",
				// PATH deliberately points somewhere without the file, so the
				// fallback is the only thing that could find it.
				Vars: map[string]string{"PATH": empty},
			})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(buf.String(), "found-in-cwd"); got != tc.found {
				t.Errorf("found = %v, want %v (output %q)", got, tc.found, buf.String())
			}
		})
	}
}

// TestDotPassesArgumentsOnlyWhenAsked covers the axis and the restore. The
// caller's parameters come back either way, which is unanimous in the panel.
func TestDotPassesArgumentsOnlyWhenAsked(t *testing.T) {
	for _, tc := range []struct {
		name string
		pass Answer
		want string
	}{
		{"passed", Yes, "got=INNER\nafter=OUTER"},
		{"not passed, so the caller's are still visible", No, "got=OUTER\nafter=OUTER"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			p := write(t, dir, "a.sh", "echo got=$1\n")
			sem := permissive()
			sem.DotPassesArguments = tc.pass
			out, _ := sourceRun(t, dir,
				`set -- OUTER; . `+p+` INNER; echo after=$1`, sem, Diagnostics{})
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("output = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestUnparseableTextIsFatalOnlyWhenTheDialectSaysSo covers both halves of the
// axis. POSIX makes a special builtin's failure fatal; most shells stopped
// doing it, so the substrate cannot pick one.
func TestUnparseableTextIsFatalOnlyWhenTheDialectSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fatal Answer
		want  string
	}{
		{"fatal: the script stops", Yes, ""},
		{"not fatal: the script carries on", No, "REACHED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.BuiltinSyntaxErrorFatal = tc.fatal
			out, _ := sourceRun(t, t.TempDir(), `eval "if"; echo REACHED`, sem, Diagnostics{})
			if got := strings.Contains(out, "REACHED"); got != (tc.want != "") {
				t.Errorf("reached = %v, want %v (output %q)", got, tc.want != "", out)
			}
		})
	}
}

// TestASourcedFileCanCarryItsOwnSyntaxStatus is the field that exists because
// one dialect answers "a syntax error" differently depending on where it read
// the text. Zero means "the same as the ordinary one".
func TestASourcedFileCanCarryItsOwnSyntaxStatus(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "bad.sh", "if\n")
	sem := permissive()

	_, st := sourceRun(t, dir, `. `+p, sem, Diagnostics{SyntaxErrorStatus: 3})
	if st != 3 {
		t.Errorf("status = %d, want the ordinary syntax status 3 when no sourced one is set", st)
	}

	_, st = sourceRun(t, dir, `. `+p, sem,
		Diagnostics{SyntaxErrorStatus: 3, SourcedSyntaxErrorStatus: 126})
	if st != 126 {
		t.Errorf("status = %d, want the sourced syntax status 126", st)
	}

	// And the ordinary one is untouched by it, which is the whole reason they
	// are two fields.
	_, st = sourceRun(t, dir, `eval "if"`, sem,
		Diagnostics{SyntaxErrorStatus: 3, SourcedSyntaxErrorStatus: 126})
	if st != 3 {
		t.Errorf("eval status = %d, want 3: a sourced status must not leak into eval", st)
	}
}

func TestDotReportsAFileItCannotRead(t *testing.T) {
	dir := t.TempDir()
	sem := permissive()

	out, st := sourceRun(t, dir, `. `+filepath.Join(dir, "absent.sh")+`; echo st=$?`,
		sem, Diagnostics{DotCannotOpenStatus: 127})
	if !strings.Contains(out, "absent.sh") {
		t.Errorf("output %q should name the file it could not read", out)
	}
	if !strings.Contains(out, "st=127") {
		t.Errorf("output %q should carry the dialect's status (status %d)", out, st)
	}
}

// TestTheReasonIsCapitalizedLikeStrerror pins a one-letter difference that is
// nonetheless visible in every diagnostic.
//
// Shells print the C strerror text — "No such file or directory" — and Go's
// syscall.Errno lowercases it. Three dialects differed from the real shell by
// that capital until the substrate fixed it, and nothing but the conformance
// harness noticed: removing the capitalization broke no unit test at all,
// which is what this is for.
func TestTheReasonIsCapitalizedLikeStrerror(t *testing.T) {
	dir := t.TempDir()
	out, _ := sourceRun(t, dir, `. `+filepath.Join(dir, "absent.sh"), permissive(),
		Diagnostics{DotCannotOpen: "%[2]s"})
	got := strings.TrimSpace(out)
	if !strings.Contains(got, "No such file") {
		t.Errorf("output = %q, want the capitalized strerror text", got)
	}
	if strings.Contains(got, "no such file") {
		t.Errorf("output = %q, want a capital N: Go lowercases where strerror does not", got)
	}
}

// TestDotDistinguishesNotFoundFromCannotOpen exists because one dialect uses
// two different messages for what the others call one failure.
func TestDotDistinguishesNotFoundFromCannotOpen(t *testing.T) {
	dir := t.TempDir()
	dg := Diagnostics{
		DotCannotOpen: "CANNOT-OPEN %[1]s",
		DotNotFound:   "NOT-FOUND %[1]s",
	}

	// A bare name PATH did not have.
	out, _ := sourceRun(t, dir, `. nosuchname.sh`, permissive(), dg)
	if !strings.Contains(out, "NOT-FOUND") {
		t.Errorf("a bare name off PATH should use DotNotFound, got %q", out)
	}

	// A path that will not open.
	out, _ = sourceRun(t, dir, `. `+filepath.Join(dir, "absent.sh"), permissive(), dg)
	if !strings.Contains(out, "CANNOT-OPEN") {
		t.Errorf("a path that will not open should use DotCannotOpen, got %q", out)
	}

	// With no DotNotFound the one message covers both, which is the other
	// three dialects.
	out, _ = sourceRun(t, dir, `. nosuchname.sh`, permissive(),
		Diagnostics{DotCannotOpen: "CANNOT-OPEN %[1]s"})
	if !strings.Contains(out, "CANNOT-OPEN") {
		t.Errorf("an empty DotNotFound should fall back to DotCannotOpen, got %q", out)
	}
}

// TestTheSubstrateRefusesWhenNoDialectAnswered is the property that makes the
// vector a specification rather than a set of defaults: an unanswered axis is
// refused, not guessed.
func TestTheSubstrateRefusesWhenNoDialectAnswered(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "a.sh", "echo got=$1\n")

	for _, tc := range []struct {
		name string
		src  string
		sem  func(Semantics) Semantics
	}{
		{
			"whether unparseable text is fatal", `eval "if"`,
			func(s Semantics) Semantics { s.BuiltinSyntaxErrorFatal = Unspecified; return s },
		},
		{
			"whether a sourced file gets its own parameters", `. ` + p + ` INNER`,
			func(s Semantics) Semantics { s.DotPassesArguments = Unspecified; return s },
		},
		{
			"whether a missing file is fatal", `. ` + filepath.Join(dir, "absent.sh"),
			func(s Semantics) Semantics { s.DotMissingFileFatal = Unspecified; return s },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := sourceRun(t, dir, tc.src, tc.sem(permissive()), Diagnostics{})
			if !strings.Contains(out, "no dialect was chosen") {
				t.Errorf("output %q should say the axis went unanswered", out)
			}
			if st == 0 {
				t.Error("an unanswered axis must not report success")
			}
		})
	}
}

// TestSourceIsNotASubstrateBuiltin pins the decision that `source` belongs to
// the dialects. One shell in the panel does not have it, so the core offering
// it would give that dialect no way to say no.
func TestSourceIsNotASubstrateBuiltin(t *testing.T) {
	if _, ok := newTestRunner(t, &Runner{}).Builtin("source"); ok {
		t.Error("`source` should come from a dialect's Apply, not the core")
	}
	if _, ok := newTestRunner(t, &Runner{}).Builtin("."); !ok {
		t.Error("`.` is POSIX and belongs to the core")
	}
}

// TestBuiltinLookupIsWhatMakesASynonymTheSameFunction covers the accessor the
// dialects use to register `source`. A synonym has to be the same function, or
// the two names drift.
func TestBuiltinLookupIsWhatMakesASynonymTheSameFunction(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	dot, ok := r.Builtin(".")
	if !ok {
		t.Fatal("no `.` builtin")
	}
	r.Register("source", dot)
	again, ok := r.Builtin("source")
	if !ok {
		t.Fatal("`source` did not register")
	}
	// Compared by behavior rather than by pointer: Go does not define equality
	// on funcs, and what matters is that the second name does the first name's
	// job.
	dir := t.TempDir()
	p := write(t, dir, "s.sh", "echo via-synonym\n")
	var buf bytes.Buffer
	sem := permissive()
	dg := Diagnostics{}
	r2 := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	if got := again(r2, context.Background(), []string{p}); got != 0 {
		t.Fatalf("status %d, output %q", got, buf.String())
	}
	if strings.TrimSpace(buf.String()) != "via-synonym" {
		t.Errorf("output = %q, want via-synonym", buf.String())
	}

	// And a dialect that replaced `.` gets its own back rather than the
	// core's, which is what "follows lookupBuiltin's precedence" means.
	r3 := newTestRunner(t, &Runner{})
	r3.Register(".", func(*Runner, context.Context, []string) int { return 42 })
	mine, ok := r3.Builtin(".")
	if !ok {
		t.Fatal("no `.` after replacing it")
	}
	if got := mine(r3, context.Background(), nil); got != 42 {
		t.Errorf("got the core's `.` back, not the replacement")
	}
}

// TestPWDIsSetBeforeTheFirstCommand covers a bug this work surfaced rather
// than caused.
//
// POSIX requires a shell to set `$PWD` at startup, and `cd` was the only thing
// that ever did — so a runner handed an environment with no PWD in it expanded
// `$PWD` to nothing. A corpus case building `PATH=$PWD/d:$PATH` therefore
// searched `/d`, missed, and fell through to the current directory, which
// looked like a `.` bug and was not one.
//
// The empty Vars map is the whole test: it stands in for the environment the
// conformance harness passes, which carries PATH, HOME, LC_ALL and TERM and
// nothing else.
func TestPWDIsSetBeforeTheFirstCommand(t *testing.T) {
	dir := t.TempDir()
	f, err := syntax.Parse(`echo "[$PWD]"`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := permissive()
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Vars: map[string]string{},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "["+dir+"]" {
		t.Errorf("$PWD = %s, want [%s]", got, dir)
	}
}

// TestPWDAlreadySetIsLeftAlone is the other half: the startup value must not
// clobber one the caller or a `cd` already chose.
func TestPWDAlreadySetIsLeftAlone(t *testing.T) {
	f, err := syntax.Parse(`echo "[$PWD]"`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := permissive()
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh",
		Vars: map[string]string{"PWD": "/chosen/by/the/caller"},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[/chosen/by/the/caller]" {
		t.Errorf("$PWD = %s, want the caller's value", got)
	}
}

// TestEvalAndDotAreSpecialBuiltins closes the gap this change was for: the
// table said they were special long before either existed.
func TestEvalAndDotAreSpecialBuiltins(t *testing.T) {
	for _, name := range []string{"eval", "."} {
		if !IsSpecialBuiltin(name) {
			t.Errorf("%s should be a special builtin", name)
		}
		if _, ok := newTestRunner(t, &Runner{}).Builtin(name); !ok {
			t.Errorf("%s is called special and does not exist", name)
		}
	}
}

// `. -p list file` — bash 5.3's way of saying "look along this list instead
// of $PATH, for this call only" — and the switch that turns the ordinary
// search off. Neither half existed until #3058: `-p` was taken as the name
// of the file, so the complaint named `-p`.
func TestDotSearchPathOptionAndSwitch(t *testing.T) {
	home := t.TempDir()
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, "f.sh"), []byte("echo sourced\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sem := func(tweak func(*Semantics)) Semantics {
		s := permissive()
		s.DotReadsOptions = Yes
		s.DotTakesTheSearchPathOption = Yes
		s.DotFallsBackToCurrentDirectory = Yes
		if tweak != nil {
			tweak(&s)
		}
		return s
	}

	// The whole point: a directory the shell is not in and PATH does not
	// hold. `sourceRun` puts only `dir` on PATH, so nothing else could find
	// this file.
	out, st := sourceRun(t, home, `. -p `+elsewhere+` f.sh`, sem(nil), Diagnostics{})
	if st != 0 || !strings.Contains(out, "sourced") {
		t.Errorf(". -p dir f.sh = %q status %d, want it read", out, st)
	}
	// Several directories, like a PATH.
	out, st = sourceRun(t, home, `. -p /nonexistent-zz:`+elsewhere+` f.sh`, sem(nil), Diagnostics{})
	if st != 0 || !strings.Contains(out, "sourced") {
		t.Errorf("a list = %q status %d, want it read", out, st)
	}
	// An empty argument is a list of one empty element, which is the current
	// directory — the same rule an empty element inside a longer list
	// follows. Measured in bash.
	if err := os.WriteFile(filepath.Join(home, "viapath.sh"), []byte("echo reached\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st = sourceRun(t, home, `. -p "" viapath.sh`, sem(nil), Diagnostics{})
	if st != 0 || !strings.Contains(out, "reached") {
		t.Errorf(`. -p "" = %q status %d, want the current directory`, out, st)
	}
	// The list *replaces* the search: a miss is a miss even when the file is
	// right there, where a `.` without the option would have found it.
	out, st = sourceRun(t, home, `. -p /nonexistent-zz viapath.sh`, sem(nil), Diagnostics{})
	if st == 0 || !strings.Contains(out, "file not found") {
		t.Errorf("a missed list = %q status %d, want the search's own complaint", out, st)
	}
	// An operand with a slash in it is a path and is not searched for.
	out, st = sourceRun(t, home, `. -p `+elsewhere+` ./f.sh`, sem(nil), Diagnostics{})
	if st == 0 || strings.Contains(out, "sourced") {
		t.Errorf("a slashed operand = %q status %d, want the path taken as written", out, st)
	}
	// `-p` with nothing after it, and every option with no operand at all.
	out, st = sourceRun(t, home, `. -p`, sem(nil), Diagnostics{})
	if st == 0 || !strings.Contains(out, "requires an argument") {
		t.Errorf(". -p alone = %q status %d", out, st)
	}
	out, st = sourceRun(t, home, `. -p `+elsewhere, sem(nil), Diagnostics{})
	if st == 0 || !strings.Contains(out, "filename") {
		t.Errorf(". -p dir with no file = %q status %d", out, st)
	}

	// The switch. With it off the operand is a path relative to the shell's
	// own directory and nothing else — so a file only PATH holds is gone,
	// and one in the directory is still read.
	// The shell runs in `elsewhere` and only `home` is on PATH, so a bare
	// `here.sh` is reachable by the search and by nothing else.
	onPath := func(r *Runner) { r.Vars["PATH"] = home }
	off := func(r *Runner) { r.Vars["PATH"] = home; r.SetSearchesPathForSource(false) }
	out, st = sourceRunWith(t, elsewhere, `. viapath.sh`, sem(nil), Diagnostics{}, onPath)
	if st != 0 || !strings.Contains(out, "reached") {
		t.Errorf("with the search on, PATH holds it: %q status %d", out, st)
	}
	out, st = sourceRunWith(t, elsewhere, `. viapath.sh`, sem(nil), Diagnostics{}, off)
	if st == 0 || strings.Contains(out, "reached") {
		t.Errorf("with the search off, PATH must not be consulted: %q status %d", out, st)
	}
	// And `-p` wins over the switch, which is measured rather than assumed.
	out, st = sourceRunWith(t, home, `. -p `+elsewhere+` f.sh`, sem(nil), Diagnostics{}, off)
	if st != 0 || !strings.Contains(out, "sourced") {
		t.Errorf("-p with the switch off = %q status %d, want the list still searched", out, st)
	}

	// Where the dialect reads no options at all, the dash-word is the file.
	out, st = sourceRun(t, home, `. -p `+elsewhere+` f.sh`,
		sem(func(s *Semantics) { s.DotReadsOptions = No }), Diagnostics{})
	if st == 0 || !strings.Contains(out, "-p") {
		t.Errorf("without option reading = %q status %d, want a complaint naming the file -p", out, st)
	}
	// And where it reads options and has not got this one, it is an option
	// it does not know rather than a file.
	out, st = sourceRun(t, home, `. -p `+elsewhere+` f.sh`,
		sem(func(s *Semantics) { s.DotTakesTheSearchPathOption = No }),
		Diagnostics{BuiltinBadOption: "%[1]s: %[2]s: unknown option"})
	if st == 0 || !strings.Contains(out, "unknown option") {
		t.Errorf("without the option = %q status %d, want the shared bad-option wording", out, st)
	}
}

// TestDotSetCancelsTheRestoreOnlyWhenAsked covers both answers of the axis
// and the three shapes that are not it.
//
// The rows that do not move are the point: a `shift`, a `set` inside a
// function the file calls and a `set` inside a subshell it opens all leave
// the caller's parameters coming back whichever way the axis is set, which is
// what says the axis is about a replacement of the *sourced file's own* list
// and not about the `set` word appearing anywhere under a `.`.
func TestDotSetCancelsTheRestoreOnlyWhenAsked(t *testing.T) {
	for _, tc := range []struct {
		name          string
		body          string
		cancels, kept string
	}{
		{
			// The discriminator.
			"a replacement at the file's own level",
			"set -- m n o p\n",
			"m n o p", "a b c",
		},
		{
			// The control the panel was measured against: every column
			// that passes the words restores over a `shift`.
			"a shift",
			"shift\n",
			"a b c", "a b c",
		},
		{
			// A call has a list of its own, so the `set` replaces that one
			// and the file's is untouched.
			"a replacement inside a function the file calls",
			"f() { set -- z; }\nf\n",
			"a b c", "a b c",
		},
		{
			// And a subshell has a copy, so nothing crosses back.
			"a replacement inside a subshell the file opens",
			"( set -- z )\n",
			"a b c", "a b c",
		},
		{
			// An empty replacement is still a replacement, which is the
			// row that says this is about the `--` and not about the file
			// leaving something behind: measured on both bash builds,
			// `set --` in the sourced file leaves the caller with no
			// positional parameters at all. An option letter on its own is
			// the other side of it and is pinned where a dialect has
			// answered the letter — see dialect/bash.
			"an empty replacement",
			"set --\n",
			"", "a b c",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, ans := range []struct {
				cancel Answer
				want   string
			}{{Yes, tc.cancels}, {No, tc.kept}} {
				dir := t.TempDir()
				p := write(t, dir, "g.sh", tc.body)
				sem := permissive()
				sem.DotPassesArguments = Yes
				sem.DotSetCancelsTheRestore = ans.cancel
				out, _ := sourceRun(t, dir,
					`set -- a b c; . `+p+` p q; echo "after=[$@]"`, sem, Diagnostics{})
				want := "after=[" + ans.want + "]"
				if got := strings.TrimSpace(out); got != want {
					t.Errorf("DotSetCancelsTheRestore=%v: output = %q, want %q", ans.cancel, got, want)
				}
			}
		})
	}
}

// TestDotSetIsNotAskedWithoutAPassedList pins where the axis is *not*
// consulted, which is the half that keeps the column ignoring the words from
// being asked a question it has no second list for.
//
// An unanswered axis refuses by name wherever it is reached, so leaving it
// unanswered is the sharpest probe there is: a shell that asks anyway writes
// the refusal and leaves 2 behind, and one that does not runs the script.
func TestDotSetIsNotAskedWithoutAPassedList(t *testing.T) {
	for _, tc := range []struct {
		name string
		pass Answer
		call string
		want string
	}{
		{
			// The words are ignored, so the file's `set` changes the
			// caller's own list and there is no restore to cancel.
			"the dialect ignores the words",
			No, `. %s p q`, "after=[m n o p]",
		},
		{
			// And with no words at all the parameters were never swapped,
			// in any dialect.
			"no words after the filename",
			Yes, `. %s`, "after=[m n o p]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			p := write(t, dir, "g.sh", "set -- m n o p\n")
			sem := permissive()
			sem.DotPassesArguments = tc.pass
			sem.DotSetCancelsTheRestore = Unspecified
			src := `set -- a b c; ` + strings.Replace(tc.call, "%s", p, 1) + `; echo "after=[$@]"`
			out, st := sourceRun(t, dir, src, sem, Diagnostics{})
			if strings.Contains(out, "unanswered") || strings.Contains(out, "no dialect") {
				t.Errorf("the axis was consulted where there is no restore: %q", out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("output = %q, want %q (status %d)", got, tc.want, st)
			}
		})
	}
}
