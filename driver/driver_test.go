// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package driver_test is external for the same reason interp's tests are: this
// package is what a dialect binary imports, so the tests should reach it only
// through what such a binary can reach.
package driver_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// shell is a dialect with nothing dialectal about it: the substrate's own
// answers, which is what a zero vector means. Naming a real shell here would
// break the rule that nothing outside dialect/ names one.
func shell() driver.Shell {
	return driver.Shell{Name: "testsh", Dialect: syntax.Core()}
}

// runArgs invokes the front end the way a process would and returns both
// streams, so a test can assert on the diagnostic as well as the output.
func runArgs(t *testing.T, sh driver.Shell, argv ...string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	sh.Stdout = &o
	sh.Stderr = &e
	code = driver.MainArgs(sh, argv)
	return o.String(), e.String(), code
}

func writeScript(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "case.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestTheInvocationRouteDoesNotChangeTheAnswer is the bug this package exists
// to make impossible.
//
// Every driver was written separately, so they drifted: cmd/sh learned to run
// a script file and the dialect binaries never did, which meant the conformance
// harness graded the *driver* and reported the dialects fourteen cases worse
// than they were. One front end is the fix; this is the assertion that the
// front end treats its routes alike.
func TestTheInvocationRouteDoesNotChangeTheAnswer(t *testing.T) {
	const src = "x=1\ny=2\necho $((x + y))\n"
	want := "3\n"

	byCommand, _, code := runArgs(t, shell(), "testsh", "-c", src)
	if code != 0 || byCommand != want {
		t.Errorf("-c gave %q status %d, want %q", byCommand, code, want)
	}

	byFile, _, code := runArgs(t, shell(), "testsh", writeScript(t, src))
	if code != 0 || byFile != want {
		t.Errorf("a script file gave %q status %d, want %q", byFile, code, want)
	}

	if byCommand != byFile {
		t.Errorf("-c and a script file disagree: %q vs %q", byCommand, byFile)
	}
}

// TestAScriptIsNamedByItsPathAndACommandByTheShell is the one way the routes
// are *supposed* to differ. A shell running a script names the script in its
// diagnostics; running -c it names itself. Getting this wrong is invisible
// until something reads the text.
func TestAScriptIsNamedByItsPathAndACommandByTheShell(t *testing.T) {
	const src = "set -u\necho \"$NOPE\"\n"

	path := writeScript(t, src)
	_, errs, code := runArgs(t, shell(), "testsh", path)
	if code == 0 {
		t.Fatal("an unset variable under set -u should fail")
	}
	if !strings.Contains(errs, path) {
		t.Errorf("a script's diagnostic %q should name the script %q", errs, path)
	}

	_, errs, code = runArgs(t, shell(), "/some/where/testsh", "-c", src)
	if code == 0 {
		t.Fatal("an unset variable under set -u should fail")
	}
	if !strings.Contains(errs, "/some/where/testsh") {
		t.Errorf("a -c diagnostic %q should name the shell as invoked", errs)
	}
}

// TestTheShellIsNamedByArgvZero pins that argv[0] beats the configured name.
// Real shells report the path they were invoked by — dash says "/bin/dash: 1:
// …" — so a hardcoded name is wrong both in `$0` and at the front of every
// diagnostic.
func TestTheShellIsNamedByArgvZero(t *testing.T) {
	out, _, code := runArgs(t, shell(), "/usr/local/bin/whatever", "-c", "echo $0")
	if code != 0 {
		t.Fatalf("status %d", code)
	}
	if got := strings.TrimSpace(out); got != "/usr/local/bin/whatever" {
		t.Errorf("$0 = %q, want the path it was invoked by", got)
	}
}

func TestArgumentForms(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"-c and its argument", []string{"testsh", "-c", "echo hi"}, "hi\n"},
		{"-c joined to its argument", []string{"testsh", "-cecho hi"}, "hi\n"},
		{
			// The words after the command are positional parameters, not more
			// options. Claiming them as flags is the misparse this guards.
			"words after the command are not options",
			[]string{"testsh", "-c", "echo hi", "-x", "-y"},
			"hi\n",
		},
		{
			// `--` ends the options, so what follows is an operand even when
			// it starts with a dash.
			"-- ends the options",
			[]string{"testsh", "--", "-c"},
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, _ := runArgs(t, shell(), tc.argv...)
			if tc.want == "" {
				// The `--` case names a file that does not exist, which is a
				// failure to open rather than an unknown option.
				if !strings.Contains(errs, "-c") {
					t.Errorf("stderr = %q, want it to treat -c as a path", errs)
				}
				return
			}
			if out != tc.want {
				t.Errorf("output = %q, want %q (stderr %q)", out, tc.want, errs)
			}
		})
	}
}

func TestAnUnknownOptionIsRefusedRatherThanIgnored(t *testing.T) {
	// A shell that silently drops an option it does not understand lets a
	// script believe it asked for something.
	_, errs, code := runArgs(t, shell(), "testsh", "-Q", "-c", "echo hi")
	if code == 0 {
		t.Fatal("an unknown option should not succeed")
	}
	if !strings.Contains(errs, "-Q") {
		t.Errorf("stderr = %q, want it to name the option it refused", errs)
	}
}

func TestDashCWithNoArgumentIsRefused(t *testing.T) {
	_, errs, code := runArgs(t, shell(), "testsh", "-c")
	if code == 0 {
		t.Fatal("-c with nothing after it should not succeed")
	}
	if !strings.Contains(errs, "-c") {
		t.Errorf("stderr = %q, want it to say what was wrong", errs)
	}
}

func TestAMissingScriptIsReportedNotRunAsEmpty(t *testing.T) {
	// Reading a file that is not there must not fall through to "run nothing
	// successfully", which is the failure mode that hides a typo in a path.
	_, errs, code := runArgs(t, shell(), "testsh", filepath.Join(t.TempDir(), "absent.sh"))
	if code == 0 {
		t.Fatal("a missing script should not succeed")
	}
	if !strings.Contains(errs, "absent.sh") {
		t.Errorf("stderr = %q, want it to name the file it could not read", errs)
	}
}

// TestTheDialectWordsItsOwnSyntaxError checks the front end asks the dialect
// rather than answering itself, both for the wording and for the status. The
// status is the easier one to get wrong: it is 2 in half the panel, 3 in ksh93
// and 1 in zsh, so a hardcoded 2 looks right until it is measured.
func TestTheDialectWordsItsOwnSyntaxError(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{
		SyntaxError:       "Bespoke syntax complaint: %s",
		Unterminated:      "Bespoke unfinished %[1]s on line %[2]d",
		SyntaxUnexpected:  "Bespoke surprise at %[1]s",
		SyntaxErrorStatus: 7,
	}

	if _, _, code := runArgs(t, shell(), "testsh", "-c", "if"); code == 0 {
		t.Fatal("an unfinished `if` should not succeed")
	}

	// Two kinds, not one. A token in the wrong place is the general failure;
	// input that ran out with a construct open is its own, because the panel
	// names four different parts of that state rather than wording a shared
	// diagnosis four ways. Both are the dialect's to word, and a dialect that
	// words only one still gets the substrate's sentence for the other.
	for _, tc := range []struct{ name, src, want string }{
		// Three kinds now, and the split is the point: the panel words each
		// of them its own way, so a dialect that answers one and not the
		// others gets the substrate's sentence for the rest.
		{"a token in the wrong place", "echo )", "Bespoke surprise at )"},
		{"input that ran out", "if", "Bespoke unfinished if on line 1"},
		{"something else entirely", "echo ${", "Bespoke syntax complaint"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, code := runArgs(t, sh, "testsh", "-c", tc.src)
			if code != 7 {
				t.Errorf("status = %d, want the dialect's 7", code)
			}
			if !strings.Contains(errs, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", errs, tc.want)
			}
		})
	}
}

// TestThePreludeIsInstalledBeforeTheScript pins the layering the extension
// story describes: a prelude function is in scope for the script, because it
// was sourced on the same runner first.
func TestThePreludeIsInstalledBeforeTheScript(t *testing.T) {
	sh := shell()
	sh.Prelude = "greet() { echo hello $1; }\n"

	out, errs, code := runArgs(t, sh, "testsh", "-c", "greet world")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if got := strings.TrimSpace(out); got != "hello world" {
		t.Errorf("output = %q, want the prelude's function to have run", got)
	}
}

// TestABrokenPreludeIsTheDialectsFaultNotTheScripts keeps the two apart. A
// prelude that fails means the dialect is broken, and reporting it as though
// the script had failed would send someone to debug the wrong file.
func TestABrokenPreludeIsTheDialectsFaultNotTheScripts(t *testing.T) {
	sh := shell()
	sh.Prelude = "if\n"

	_, errs, code := runArgs(t, sh, "testsh", "-c", "echo never")
	if code == 0 {
		t.Fatal("a broken prelude should not succeed")
	}
	if !strings.Contains(errs, "prelude") {
		t.Errorf("stderr = %q, want it to say the prelude was at fault", errs)
	}
}

// TestRegisterCanReplaceABuiltin covers the third extension point: Go for what
// shell cannot express. Replacing one is the observable half — if the
// registered function did not win, the core's own answer would show through.
func TestRegisterCanReplaceABuiltin(t *testing.T) {
	sh := shell()
	sh.Register = func(r *interp.Runner) {
		r.Register("echo", func(r *interp.Runner, _ context.Context, _ []string) int {
			_, _ = io.WriteString(r.Stdout, "replaced\n")
			return 0
		})
	}

	out, errs, code := runArgs(t, sh, "testsh", "-c", "echo original")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if got := strings.TrimSpace(out); got != "replaced" {
		t.Errorf("output = %q, want the registered builtin to have won", got)
	}
}

// TestTheFrontEndAndEvalAgreeOnWhereAParseErrorIs is the same class of bug as
// TestTheInvocationRouteDoesNotChangeTheAnswer, one layer down.
//
// One dialect puts an unterminated construct on the line *after* the input's
// last when the text does not end in one. `eval` and `.` asked the dialect for
// that line and the front end read the error's own position instead, so the
// two reported the same failure on different lines — the front end being
// shared is no help if it does not use the shared answer.
func TestTheFrontEndAndEvalAgreeOnWhereAParseErrorIs(t *testing.T) {
	sh := shell()
	d := interp.Diagnostics{
		Location:                   interp.LocationLineWord,
		UnterminatedEndsOnNextLine: true,
		SyntaxError:                "syntax error on line %[3]d",
		Unterminated:               "syntax error on line %[3]d",
	}
	sh.Diagnostics = d

	// The text has no trailing newline, so the end of it is line 2.
	_, direct, _ := runArgs(t, sh, "testsh", "-c", "{ echo a")
	_, viaEval, _ := runArgs(t, sh, "testsh", "-c", `eval "{ echo a"`)
	if !strings.Contains(direct, "line 2") {
		t.Errorf("the front end reported %q, want the line after the last", direct)
	}
	if !strings.Contains(viaEval, "line 2") {
		t.Errorf("eval reported %q, want the line after the last", viaEval)
	}
}

// TestWhereTheScriptCameFromIsNamedOnlyForACommand: one dialect puts the
// origin between its name and the line, and only for `-c` — a script names
// itself and standard input names neither.
func TestWhereTheScriptCameFromIsNamedOnlyForACommand(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{
		Location:                interp.LocationLineWord,
		NamesTheInputInLocation: true,
		SyntaxUnexpected:        `unexpected %[1]s`,
	}

	_, viaCommand, _ := runArgs(t, sh, "testsh", "-c", "{ fi; }")
	if !strings.HasPrefix(viaCommand, "testsh: -c: line 1: ") {
		t.Errorf("-c gave %q, want the origin named", viaCommand)
	}
	path := writeScript(t, "{ fi; }\n")
	_, viaScript, _ := runArgs(t, sh, "testsh", path)
	if strings.Contains(viaScript, "-c") {
		t.Errorf("a script gave %q, want no origin named", viaScript)
	}
	// And a dialect that does not do this never gets it, whatever the route.
	plain := shell()
	plain.Diagnostics = interp.Diagnostics{Location: interp.LocationLineWord, SyntaxUnexpected: `unexpected %[1]s`}
	if _, errs, _ := runArgs(t, plain, "testsh", "-c", "{ fi; }"); strings.Contains(errs, "-c") {
		t.Errorf("got %q, want no origin named", errs)
	}
}

// TestTheOffendingLineIsEchoedOnlyForAToken: the dialect that repeats the
// source line does it for a word the grammar did not want and not for input
// that simply ran out — there is no offending line to point at then.
func TestTheOffendingLineIsEchoedOnlyForAToken(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{
		Location:               interp.LocationLineWord,
		EchoesTheOffendingLine: true,
		SyntaxUnexpected:       `unexpected %[1]s`,
		Unterminated:           `ran out`,
	}
	_, token, _ := runArgs(t, sh, "testsh", "-c", "echo one\n{ fi; }")
	if want := "testsh: line 2: `{ fi; }'\n"; !strings.HasSuffix(token, want) {
		t.Errorf("got %q, want it to end with %q — the line the failure was on", token, want)
	}
	_, ranOut, _ := runArgs(t, sh, "testsh", "-c", "{ echo a")
	if strings.Count(ranOut, "\n") != 1 {
		t.Errorf("got %q, want one line and no echo", ranOut)
	}
}

// TestAFailureTheDialectRefusesAtRuntimeIsNotDecorated: `for 1x` is found
// while parsing here and reported when it runs in the dialect this models, so
// it carries that dialect's runtime status and none of a parse failure's
// decoration — neither the named origin nor the echoed line.
func TestAFailureTheDialectRefusesAtRuntimeIsNotDecorated(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{
		Location:                interp.LocationLineWord,
		NamesTheInputInLocation: true,
		EchoesTheOffendingLine:  true,
		ForNameStatus:           1,
		ForName:                 `%[1]s is not a name`,
		SyntaxUnexpected:        `unexpected %[1]s`,
	}
	_, errs, code := runArgs(t, sh, "testsh", "-c", "for 1x in a; do :; done")
	if strings.Contains(errs, "-c") {
		t.Errorf("got %q, want no origin named", errs)
	}
	if strings.Count(errs, "\n") != 1 {
		t.Errorf("got %q, want no echoed line", errs)
	}
	if code != 1 {
		t.Errorf("status %d, want the dialect's runtime status", code)
	}
}

// TestPositionalParametersReachTheScript is the gap that made most real
// scripts useless: the interpreter had positional parameters all along and the
// front end never gave it any, so `$#` was 0 however the shell was invoked.
//
// Every route names them differently and the panel is unanimous about each,
// which is why they are all here rather than one standing for the rest.
func TestPositionalParametersReachTheScript(t *testing.T) {
	const show = `echo "0=[$0] n=$# 1=[$1] 2=[$2] at=[$@]"`
	path := writeScript(t, show+"\n")

	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		// A script's path is `$0` and the operands after it are the
		// parameters.
		{"a script", []string{"testsh", path, "a", "b"}, "0=[" + path + "] n=2 1=[a] 2=[b] at=[a b]"},
		{"a script with none", []string{"testsh", path}, "0=[" + path + "] n=0 1=[] 2=[] at=[]"},
		{"after --", []string{"testsh", "--", path, "a"}, "0=[" + path + "] n=1 1=[a] 2=[] at=[a]"},
		// `-c` is the odd one: the *first* operand becomes `$0`, so the
		// parameters start at the second.
		{"a command with a name", []string{"testsh", "-c", show, "name", "a", "b"}, "0=[name] n=2 1=[a] 2=[b] at=[a b]"},
		{"a command with only a name", []string{"testsh", "-c", show, "name"}, "0=[name] n=0 1=[] 2=[] at=[]"},
		// With no operands at all the shell keeps its own name.
		{"a bare command", []string{"testsh", "-c", show}, "0=[testsh] n=0 1=[] 2=[] at=[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, _ := runArgs(t, shell(), tc.argv...)
			if errs != "" {
				t.Fatalf("stderr: %s", errs)
			}
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got  %s\nwant %s", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// TestPositionalParametersSurviveTheShell: what the invocation supplies is the
// same list `set` and `shift` work on, rather than a second one beside it.
func TestPositionalParametersSurviveTheShell(t *testing.T) {
	path := writeScript(t, "shift; echo \"n=$# 1=[$1]\"\nset -- x\necho \"n=$# 1=[$1]\"\n")
	out, _, _ := runArgs(t, shell(), "testsh", path, "a", "b", "c")
	if want := "n=2 1=[b]\nn=1 1=[x]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestStandardInputKeepsTheShellsName: reading the script from standard input
// is the third naming rule — the shell stays `$0` and *every* operand is a
// parameter, since none of them was the script.
//
// The front end reads the process's own standard input, so the test replaces
// it. That is the only way to reach this route, and leaving it untested is
// what let the operands be dropped without anything noticing.
func TestStandardInputKeepsTheShellsName(t *testing.T) {
	path := writeScript(t, `echo "0=[$0] n=$# 1=[$1] at=[$@]"`+"\n")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	saved := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = saved }()

	out, errs, _ := runArgs(t, shell(), "testsh", "-s", "a", "b")
	if errs != "" {
		t.Fatalf("stderr: %s", errs)
	}
	if want := "0=[testsh] n=2 1=[a] at=[a b]"; strings.TrimSpace(out) != want {
		t.Errorf("got  %s\nwant %s", strings.TrimSpace(out), want)
	}
}
