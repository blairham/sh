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
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	r := &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
	}
	// PATH is set explicitly rather than inherited: a `.` that found something
	// on the developer's PATH would pass here and fail on a runner.
	r.Vars = map[string]string{"PATH": dir}
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
	s.BuiltinSyntaxErrorFatal = No
	s.DotMissingFileFatal = No
	s.DotWithNoOperandIsAnError = Yes
	s.DotPassesArguments = Yes
	s.DotFallsBackToCurrentDirectory = No
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
	r := &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: cwd, Name: "testsh", Vars: map[string]string{"PATH": pathDir},
	}
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
			r := &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
				Dir: cwd, Name: "testsh",
				// PATH deliberately points somewhere without the file, so the
				// fallback is the only thing that could find it.
				Vars: map[string]string{"PATH": empty},
			}
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
	if _, ok := (&Runner{}).Builtin("source"); ok {
		t.Error("`source` should come from a dialect's Apply, not the core")
	}
	if _, ok := (&Runner{}).Builtin("."); !ok {
		t.Error("`.` is POSIX and belongs to the core")
	}
}

// TestBuiltinLookupIsWhatMakesASynonymTheSameFunction covers the accessor the
// dialects use to register `source`. A synonym has to be the same function, or
// the two names drift.
func TestBuiltinLookupIsWhatMakesASynonymTheSameFunction(t *testing.T) {
	r := &Runner{}
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
	r2 := &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	}
	if got := again(r2, context.Background(), []string{p}); got != 0 {
		t.Fatalf("status %d, output %q", got, buf.String())
	}
	if strings.TrimSpace(buf.String()) != "via-synonym" {
		t.Errorf("output = %q, want via-synonym", buf.String())
	}

	// And a dialect that replaced `.` gets its own back rather than the
	// core's, which is what "follows lookupBuiltin's precedence" means.
	r3 := &Runner{}
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
	r := &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Vars: map[string]string{},
	}
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
	r := &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh",
		Vars: map[string]string{"PWD": "/chosen/by/the/caller"},
	}
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
		if _, ok := (&Runner{}).Builtin(name); !ok {
			t.Errorf("%s is called special and does not exist", name)
		}
	}
}
