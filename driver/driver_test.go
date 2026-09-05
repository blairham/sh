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
		{
			// `c` in the middle of a bundle is still `-c`, and the letters
			// after it are still option letters. Reading the rest of the
			// word as the command string instead ran a command called `e`
			// with the string as `$0`; the panel is unanimous that this is
			// `-c -e` and that the string is the first operand.
			"-c in the middle of a bundle",
			[]string{"testsh", "-ce", "echo hi"},
			"hi\n",
		},
		{
			// The mirror image, which already worked and must keep working:
			// `c` last in the bundle is the same invocation as `-ce`.
			"-c at the end of a bundle",
			[]string{"testsh", "-ec", "echo hi"},
			"hi\n",
		},
		{
			// `+c` is `-c` — measured, all four shells run the command.
			// Sign-gating it made `c` a set letter to turn off and then
			// opened the command string as a script file.
			"+c runs the command too",
			[]string{"testsh", "+c", "echo hi"},
			"hi\n",
		},
		{
			// Options may come between `-c` and the command string. An agent
			// harness writes this, and all four shells read it: the option
			// loop does not stop at `c`, it stops at the first operand.
			"options after -c and before the command string",
			[]string{"testsh", "-c", "-x", "echo hi"},
			"hi\n",
		},
		{
			// `--` between them too, which is the same rule seen from the
			// other side: it ends the options, and the first operand after
			// it is the command string rather than a path.
			"-- between -c and the command string",
			[]string{"testsh", "-c", "--", "echo hi"},
			"hi\n",
		},
		{
			// `-c` beats `-s` and `-i` about *where the program comes from*:
			// all four run the command rather than reading standard input or
			// prompting.
			"-c beats -s",
			[]string{"testsh", "-sc", "echo hi"},
			"hi\n",
		},
		{
			"-c beats -i",
			[]string{"testsh", "-ic", "echo hi"},
			"hi\n",
		},
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

// TestAnAttachedCommandStringIsRefused pins that `-c<string>` written as one
// word is not an invocation, which is measured rather than reasoned: no shell
// in the panel accepts it. bash reads the tail as more option letters and
// refuses the space in `-cecho hi`; dash, ksh93 and zsh each refuse it in
// their own words. Running it was a misreading of what getopt allows — a
// shell's own option scanner is not getopt, and `c` is not an option that
// takes an attached value in any of them.
func TestAnAttachedCommandStringIsRefused(t *testing.T) {
	out, _, code := runArgs(t, shell(), "testsh", "-cecho hi")
	if code == 0 {
		t.Error("an attached command string should not be accepted")
	}
	if out != "" {
		t.Errorf("stdout = %q, want the command never to have run", out)
	}
}

// TestABundledCTakesItsCommandStringFromTheFirstOperand is the whole of the
// fix in one invocation: the letters around `c` are options, the first operand
// is the command string, the second is `$0` and the rest are parameters.
func TestABundledCTakesItsCommandStringFromTheFirstOperand(t *testing.T) {
	out, errs, code := runArgs(t, shell(), "testsh",
		"-ce", `echo "0=$0 n=$# 1=$1"; false`, "name", "a")
	// `e` was a real option: the failing command at the end ends the shell.
	if code != 1 {
		t.Errorf("status %d, want 1 — the bundled -e should have applied (stderr %q)", code, errs)
	}
	if want := "0=name n=1 1=a\n"; out != want {
		t.Errorf("output = %q, want %q (stderr %q)", out, want, errs)
	}
}

// TestCWithNoOperandIsRefused pins that `-c` alone is a usage error rather
// than a shell that reads standard input. All four refuse it; only the wording
// and the status differ, so only the refusal is asserted.
func TestCWithNoOperandIsRefused(t *testing.T) {
	for _, argv := range [][]string{
		{"testsh", "-c"},
		{"testsh", "-ec"},
		{"testsh", "-c", "-e"},
	} {
		_, errs, code := runArgs(t, shell(), argv...)
		if code == 0 {
			t.Errorf("%q: status 0, want a refusal", argv)
		}
		if !strings.Contains(errs, "-c") {
			t.Errorf("%q: stderr = %q, want it to name -c", argv, errs)
		}
	}
}

func TestAnUnknownOptionIsRefusedRatherThanIgnored(t *testing.T) {
	// A shell that silently drops an option it does not understand lets a
	// script believe it asked for something. A *known* option is a `set`
	// option now, so the refusal comes from the same machinery `set` uses —
	// and it still stops the shell before anything runs.
	out, errs, code := runArgs(t, shell(), "testsh", "-Q", "-c", "echo hi")
	if code == 0 {
		t.Fatal("an unknown option should not succeed")
	}
	if !strings.Contains(errs, "-Q") {
		t.Errorf("stderr = %q, want it to name the option it refused", errs)
	}
	if out != "" {
		t.Errorf("stdout = %q, want the command never to have run", out)
	}
}

// TestSetOptionsAtInvocationReachEveryRoute is the gap the front end had: it
// knew `-i`, `-c`, `-s` and nothing else, so `sh -e script.sh` — an everyday
// invocation, and how the wild-run sweep invokes everything — exited 2 with
// `unknown option`. All four panel shells hand invocation options to the
// `set` machinery, whatever the route.
func TestSetOptionsAtInvocationReachEveryRoute(t *testing.T) {
	// Under errexit the `false` ends the script, so `alive` never prints.
	const src = "false\necho alive\n"

	out, _, code := runArgs(t, shell(), "testsh", "-e", "-c", src)
	if code == 0 || out != "" {
		t.Errorf("-c gave %q status %d, want errexit to end it silently", out, code)
	}

	out, _, code = runArgs(t, shell(), "testsh", "-e", writeScript(t, src))
	if code == 0 || out != "" {
		t.Errorf("a script gave %q status %d, want errexit to end it silently", out, code)
	}

	f, err := os.Open(writeScript(t, src))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sh := shell()
	sh.Stdin = f
	out, _, code = runArgs(t, sh, "testsh", "-e")
	if code == 0 || out != "" {
		t.Errorf("standard input gave %q status %d, want errexit to end it silently", out, code)
	}
}

// TestDollarDashReflectsInvocationOptions: `case $- in *e*)` is the standard
// errexit check, and an option set at invocation has to be visible to it —
// the letters are the same state `set` reads and writes, not a note the
// front end kept to itself.
func TestDollarDashReflectsInvocationOptions(t *testing.T) {
	out, errs, code := runArgs(t, shell(), "testsh", "-eu", "-c", `echo "$-"`)
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	for _, letter := range []string{"e", "u"} {
		if !strings.Contains(out, letter) {
			t.Errorf("$- = %q, want %q in it", strings.TrimSpace(out), letter)
		}
	}

	// And `set +e` can undo what the invocation set, because they are the
	// same option and not two.
	out, _, code = runArgs(t, shell(), "testsh", "-e", "-c", "set +e\nfalse\necho alive\n")
	if code != 0 || strings.TrimSpace(out) != "alive" {
		t.Errorf("got %q status %d, want `set +e` to undo the invocation's -e", out, code)
	}
}

// TestOptionFormsAtInvocation: the spellings the panel is unanimous on —
// single letters, bundles, the `+` sign, `-o name` and `+o name`, `o` ending
// a bundle the way `set -euo pipefail` writes it, and letters bundled with
// `-c` itself.
func TestOptionFormsAtInvocation(t *testing.T) {
	const alive = "false\necho alive\n"
	for _, tc := range []struct {
		name  string
		argv  []string
		alive bool
	}{
		{"a single letter", []string{"testsh", "-e", "-c", alive}, false},
		{"a bundle", []string{"testsh", "-ue", "-c", alive}, false},
		{"a plus turns one off", []string{"testsh", "-e", "+e", "-c", alive}, true},
		{"-o and its name", []string{"testsh", "-o", "errexit", "-c", alive}, false},
		{"+o and its name", []string{"testsh", "-e", "+o", "errexit", "-c", alive}, true},
		{"o ending a bundle", []string{"testsh", "-uo", "errexit", "-c", alive}, false},
		{"a letter bundled with -c", []string{"testsh", "-ec", alive}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := runArgs(t, shell(), tc.argv...)
			got := strings.Contains(out, "alive")
			if got != tc.alive {
				t.Errorf("output %q status %d (stderr %q), want alive=%v", out, code, errs, tc.alive)
			}
		})
	}
}

// TestOptionsStopAtTheFirstOperand: a word after the script's path belongs
// to the script however it is spelled. Unanimous — `sh script.sh -e` hands
// the script `-e` and sets nothing.
func TestOptionsStopAtTheFirstOperand(t *testing.T) {
	path := writeScript(t, "echo \"1=[$1]\"\nfalse\necho alive\n")
	out, errs, code := runArgs(t, shell(), "testsh", path, "-e")
	if errs != "" || code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "1=[-e]\nalive\n"; out != want {
		t.Errorf("got %q, want %q — the -e is a parameter, not an option", out, want)
	}
}

// TestOptionsAreStillReadAfterDashS: `-s` is an option, not a terminator.
// Measured, all four: `sh -s -e arg` sets errexit and makes `arg` the first
// parameter; only an operand ends the options.
func TestOptionsAreStillReadAfterDashS(t *testing.T) {
	f, err := os.Open(writeScript(t, "echo \"1=[$1]\"\nfalse\necho alive\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sh := shell()
	sh.Stdin = f
	out, errs, code := runArgs(t, sh, "testsh", "-s", "-e", "arg")
	if code == 0 {
		t.Fatalf("status 0, want errexit to have ended the script (stderr %q)", errs)
	}
	if want := "1=[arg]\n"; out != want {
		t.Errorf("got %q, want %q — the word after the options is a parameter", out, want)
	}
}

// TestVerboseEchoesFromTheFirstLine: `-v` is the option whose behavior lives
// in the front end — the raw text is echoed as it is read — so it is the one
// that would stay silent if the invocation's options were applied anywhere
// but before the script.
func TestVerboseEchoesFromTheFirstLine(t *testing.T) {
	_, errs, code := runArgs(t, shell(), "testsh", "-v", "-c", "echo hi")
	if code != 0 {
		t.Fatalf("status %d", code)
	}
	if !strings.Contains(errs, "echo hi") {
		t.Errorf("stderr = %q, want the source echoed back", errs)
	}
}

// TestDashOWantsAName pins the refusals around `-o`, and both are decisions:
//
//   - `-o` with nothing after it: three of the panel print the option table
//     and read on, one refuses with "string expected after -o". A split
//     panel, and a listing at invocation is not worth the machinery until
//     something needs it, so the front end refuses.
//   - `-oNAME` in one word: two of the panel read NAME as the option's name,
//     two read it as more single-letter options. The core takes neither side
//     of a disagreement, so the word is refused whole.
func TestDashOWantsAName(t *testing.T) {
	_, errs, code := runArgs(t, shell(), "testsh", "-o")
	if code == 0 {
		t.Fatal("-o with nothing after it should not succeed")
	}
	if !strings.Contains(errs, "-o") {
		t.Errorf("stderr = %q, want it to say what was wrong", errs)
	}

	out, errs, code := runArgs(t, shell(), "testsh", "-oerrexit", "-c", "echo hi")
	if code == 0 || out != "" {
		t.Errorf("-oNAME gave %q status %d, want it refused whole", out, code)
	}
	if !strings.Contains(errs, "-oerrexit") {
		t.Errorf("stderr = %q, want the word it refused", errs)
	}
}

// TestABadOptionNameIsRefusedBeforeAnythingRuns: the name after `-o` is the
// dialect's to judge, exactly as it is for `set -o`, and a refused one stops
// the shell with the refusal on stderr rather than running the script
// anyway.
func TestABadOptionNameIsRefusedBeforeAnythingRuns(t *testing.T) {
	out, errs, code := runArgs(t, shell(), "testsh", "-o", "nosuchoption", "-c", "echo never")
	if code == 0 || out != "" {
		t.Errorf("got %q status %d, want nothing run", out, code)
	}
	if !strings.Contains(errs, "nosuchoption") {
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
	// And with the substrate's own number, which is a missing command's
	// rather than a usage error's: reaching no program at all is not the same
	// failure as being invoked wrongly, and every shell in the panel keeps
	// them apart. The pair above pinned only "not zero", which a flat 2 for
	// everything satisfied.
	if code != 127 {
		t.Errorf("status = %d, want 127", code)
	}
}

// TestTheDialectWordsAndNumbersAScriptItCannotRead checks the front end asks
// the dialect for both halves of this, and asks it twice: a path that is not
// there and a path that is there and will not open are two failures, and three
// of the four shells in the panel number them differently.
//
// Bespoke wordings and statuses rather than any shell's, for the reason every
// test in this package uses them: the front end's job is to ask, and a test
// that asserted a real dialect's numbers would pass just as well if the front
// end had them written into it.
func TestTheDialectWordsAndNumbersAScriptItCannotRead(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{
		ScriptNotFound:          "bespoke nothing at %[1]s, being %[2]s",
		ScriptNotFoundStatus:    41,
		ScriptNotReadable:       "bespoke will not open %[1]s, being %[2]s",
		ScriptNotReadableStatus: 42,
	}

	missing := filepath.Join(t.TempDir(), "absent.sh")
	_, errs, code := runArgs(t, sh, "testsh", missing)
	if code != 41 {
		t.Errorf("missing script status = %d, want the dialect's 41", code)
	}
	if !strings.Contains(errs, "bespoke nothing at "+missing) {
		t.Errorf("stderr = %q, want the dialect's wording naming the operand", errs)
	}

	// A directory is a path that is there and will not be read, and it is the
	// portable way to say so: a mode-000 file is still readable to a test that
	// happens to run as root, which is how a container runs one.
	_, errs, code = runArgs(t, sh, "testsh", t.TempDir())
	if code != 42 {
		t.Errorf("unreadable script status = %d, want the dialect's 42", code)
	}
	if !strings.Contains(errs, "bespoke will not open ") {
		t.Errorf("stderr = %q, want the other of the dialect's two wordings", errs)
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
		// `${` and the plain quotes have kinds of their own now, so the
		// catch-all is reached through a construct nobody has worded yet.
		{"something else entirely", "echo $'abc", "Bespoke syntax complaint"},
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

// TestALoneDashEndsTheOptions: `-` is an end-of-options marker like `--`, not
// a request to read standard input. Every shell in the panel runs `sh - a b`
// as the script `a`; only a `-` with nothing after it reaches standard input,
// and it does that by falling through to the no-operands case rather than by
// meaning anything itself.
func TestALoneDashEndsTheOptions(t *testing.T) {
	path := writeScript(t, `echo "0=[$0] n=$# 1=[$1]"`+"\n")
	out, errs, _ := runArgs(t, shell(), "testsh", "-", path, "x")
	if errs != "" {
		t.Fatalf("stderr: %s", errs)
	}
	if want := "0=[" + path + "] n=1 1=[x]"; strings.TrimSpace(out) != want {
		t.Errorf("got  %s\nwant %s", strings.TrimSpace(out), want)
	}
}

// TestLinesRunAsTheyAreRead is the difference between a shell and a compiler.
//
// A script that ends badly still does what its good lines said, because the
// shell runs what it has read rather than reading everything first. All four
// shells do this for a script file.
func TestLinesRunAsTheyAreRead(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{Location: interp.LocationLineWord, SyntaxUnexpected: `unexpected %[1]s`}

	path := writeScript(t, "echo one\necho two\n{ fi; }\necho four\n")
	out, errs, code := runArgs(t, sh, "testsh", path)
	if want := "one\ntwo\n"; out != want {
		t.Errorf("output %q, want %q — the good lines run first", out, want)
	}
	if !strings.Contains(errs, "unexpected") {
		t.Errorf("stderr %q, want the failure reported", errs)
	}
	if code == 0 {
		t.Error("status 0, want a failure")
	}
}

// TestALineIsTheUnit: the whole line is parsed before any of it runs, so a
// statement that precedes the failure on the same line never happens. A
// statement-at-a-time reader would have run it.
func TestALineIsTheUnit(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{Location: interp.LocationLineWord, SyntaxUnexpected: `unexpected %[1]s`}

	path := writeScript(t, "echo one\necho two; { fi; }\n")
	out, _, _ := runArgs(t, sh, "testsh", path)
	if want := "one\n"; out != want {
		t.Errorf("output %q, want %q — `echo two` shares its line with the failure", out, want)
	}
}

// TestTheExitTrapFiresAfterAParseFailure: the failure is an ending rather than
// an abort, so a trap set by a line that ran still fires. Unanimous.
func TestTheExitTrapFiresAfterAParseFailure(t *testing.T) {
	sh := shell()
	// The trap body is read the way a script is, which is the substrate's
	// own answer and the one three of the four give.
	sh.Semantics.TrapBodyRunsWhatParsed = interp.Yes
	sh.Diagnostics = interp.Diagnostics{Location: interp.LocationLineWord, SyntaxUnexpected: `unexpected %[1]s`}

	path := writeScript(t, "trap 'echo bye' EXIT\necho one\n{ fi; }\n")
	out, _, _ := runArgs(t, sh, "testsh", path)
	if want := "one\nbye\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
}

// TestAConstructHoldsTheLineOpen: the unit stretches past a newline while a
// construct is open, or a multi-line loop could never run at all.
func TestAConstructHoldsTheLineOpen(t *testing.T) {
	path := writeScript(t, "for i in 1 2\ndo\n  echo $i\ndone\necho after\n")
	out, errs, _ := runArgs(t, shell(), "testsh", path)
	if errs != "" {
		t.Fatalf("stderr: %s", errs)
	}
	if want := "1\n2\nafter\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
}

// TestACommandStringMayBeReadWhole is the axis: one dialect parses all of a
// `-c` command before running any of it, so a failure anywhere in it means
// nothing runs. Every dialect reads a *script* a line at a time.
func TestACommandStringMayBeReadWhole(t *testing.T) {
	const src = "echo one\n{ fi; }\n"
	for _, tc := range []struct {
		whole bool
		want  string
	}{
		{false, "one\n"},
		{true, ""},
	} {
		sh := shell()
		sh.Diagnostics = interp.Diagnostics{
			Location:                 interp.LocationLineWord,
			SyntaxUnexpected:         `unexpected %[1]s`,
			CommandStringParsedWhole: tc.whole,
		}
		out, _, _ := runArgs(t, sh, "testsh", "-c", src)
		if out != tc.want {
			t.Errorf("whole=%v: output %q, want %q", tc.whole, out, tc.want)
		}
		// A script is read a line at a time whatever the answer, because the
		// axis is about the command string alone.
		path := writeScript(t, src)
		if out, _, _ := runArgs(t, sh, "testsh", path); out != "one\n" {
			t.Errorf("whole=%v: a script gave %q, want the first line to run", tc.whole, out)
		}
	}
}

// TestExitStopsTheReading: `exit` ends the script, so what follows is never
// read — not even far enough to find that it would not parse. Without that,
// a script that exits cleanly before a broken line would report the breakage
// it was never going to reach.
func TestExitStopsTheReading(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{Location: interp.LocationLineWord, SyntaxUnexpected: `unexpected %[1]s`}

	path := writeScript(t, "echo one\nexit 3\n{ fi; }\n")
	out, errs, code := runArgs(t, sh, "testsh", path)
	if out != "one\n" {
		t.Errorf("output %q, want %q", out, "one\n")
	}
	if errs != "" {
		t.Errorf("stderr %q, want nothing — the broken line is never reached", errs)
	}
	if code != 3 {
		t.Errorf("status %d, want 3 from the exit", code)
	}
}

// The front end tells the interpreter where the program came from, which one
// dialect answers a failed expansion by.
//
// Here rather than in interp, because it is the *invocation* that is being
// asserted: the interpreter's half is tested by setting the flag directly,
// and nothing there can see whether the front end sets it right.
func TestTheFrontEndSaysWhenTheProgramWasAnArgument(t *testing.T) {
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	sh.Semantics.FatalErrorStatusIsOne = interp.Yes
	// A status this dialect gives only when the program came from `-c`.
	sh.Diagnostics.ExpansionFailureStatusFromCommandString = 127

	const src = "set -u\necho \"$NOPE\"\n"
	if _, _, code := runArgs(t, sh, "testsh", "-c", src); code != 127 {
		t.Errorf("-c gave %d, want 127", code)
	}
	if _, _, code := runArgs(t, sh, "testsh", writeScript(t, src)); code != 1 {
		t.Errorf("a script file gave %d, want the ordinary fatal status", code)
	}
}

// TestAHereDocumentWarningIsSaidOnce covers the front end's half: the parser
// produces a remark as it reads, and the loop asks after every line, so
// without a count of what has been shown the first remark would be repeated
// for every line after it.
func TestAHereDocumentWarningIsSaidOnce(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{
		Location:          interp.LocationLineWord,
		HereDocumentAtEOF: "warning: here-document at line %[1]d wanted `%[2]s'",
	}
	path := writeScript(t, "echo one\ncat <<X\nbody\n")
	_, errs, _ := runArgs(t, sh, "testsh", path)
	if n := strings.Count(errs, "warning:"); n != 1 {
		t.Errorf("said it %d times, want once: %q", n, errs)
	}
	if !strings.Contains(errs, "line 3: warning: here-document at line 2") {
		t.Errorf("stderr = %q, want it located where the input ran out and to name the other line", errs)
	}
}

// TestADialectThatSaysNothingSaysNothing, which is how three of the four are
// expressed: an empty wording rather than the front end knowing which shells
// are quiet.
func TestADialectThatSaysNothingSaysNothing(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{Location: interp.LocationLineWord}
	path := writeScript(t, "cat <<X\nbody\n")
	_, errs, _ := runArgs(t, sh, "testsh", path)
	if errs != "" {
		t.Errorf("stderr = %q, want silence", errs)
	}
}

// TestTheWarningIsSaidBeforeAFatalError, which is measured: a here-document
// with neither its delimiter nor its enclosing brace produces both, warning
// first, and a front end that rendered remarks only on success would drop it.
func TestTheWarningIsSaidBeforeAFatalError(t *testing.T) {
	sh := shell()
	sh.Diagnostics = interp.Diagnostics{
		Location:          interp.LocationLineWord,
		SyntaxError:       "syntax error: %[1]s",
		HereDocumentAtEOF: "warning: here-document at line %[1]d wanted `%[2]s'",
	}
	path := writeScript(t, "f() { cat <<X\ny\n")
	_, errs, code := runArgs(t, sh, "testsh", path)
	warn := strings.Index(errs, "warning:")
	fail := strings.Index(errs, "syntax error")
	if warn < 0 || fail < 0 {
		t.Fatalf("stderr = %q, want both the warning and the failure", errs)
	}
	if warn > fail {
		t.Errorf("stderr = %q, want the warning first", errs)
	}
	if code == 0 {
		t.Error("status = 0, want the parse failure to stand")
	}
}

// TestWhoNamesTheOperandsWithBothRoutesIsAnAxis: `-c` and `-s` together have
// two rules for naming operands and the panel splits over which applies, so
// the front end asks rather than picking one.
//
// Yes is the standard-input rule — no operand becomes `$0`, so the shell
// keeps its own name and every operand is a parameter. No is the command
// string's — the first operand is `$0` and only the rest are parameters.
// Neither is refused, because a shell that guessed would silently hand a
// script the wrong `$1`.
func TestWhoNamesTheOperandsWithBothRoutesIsAnAxis(t *testing.T) {
	const snippet = `echo "0=$0 n=$# args=$*"`
	for _, tc := range []struct {
		name   string
		answer interp.Answer
		want   string
	}{
		{"the standard-input rule", interp.Yes, "0=testsh n=2 args=name a\n"},
		{"the command string's rule", interp.No, "0=name n=1 args=a\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			sh.Semantics.StdinOptionNamesTheOperands = tc.answer
			out, errs, code := runArgs(t, sh, "testsh", "-sc", snippet, "name", "a")
			if code != 0 {
				t.Errorf("status %d, want 0 (stderr %q)", code, errs)
			}
			if out != tc.want {
				t.Errorf("output = %q, want %q (stderr %q)", out, tc.want, errs)
			}
		})
	}
	// Unanswered, the invocation is refused rather than given one side's
	// answer — and refused as a usage error, before anything runs.
	out, errs, code := runArgs(t, shell(), "testsh", "-sc", snippet, "name", "a")
	if code != 2 || out != "" {
		t.Errorf("status %d output %q, want the invocation refused (stderr %q)", code, out, errs)
	}
	if !strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("stderr = %q, want the unanswered axis named", errs)
	}
	// With no operand there is nothing for the two rules to disagree about,
	// so the question is not asked and the command runs whatever the vector
	// says. Measured: all four keep their own name and no parameters.
	out, errs, code = runArgs(t, shell(), "testsh", "-sc", snippet)
	if want := "0=testsh n=0 args=\n"; code != 0 || out != want {
		t.Errorf("status %d output %q, want %q (stderr %q)", code, out, want, errs)
	}
	// The letters are read as options wherever they appear, so the two
	// spellings and both orders reach the same answer.
	for _, argv := range [][]string{
		{"testsh", "-s", "-c", snippet, "name", "a"},
		{"testsh", "-c", "-s", snippet, "name", "a"},
		{"testsh", "-cs", snippet, "name", "a"},
	} {
		sh := shell()
		sh.Semantics.StdinOptionNamesTheOperands = interp.Yes
		out, errs, code := runArgs(t, sh, argv...)
		if want := "0=testsh n=2 args=name a\n"; code != 0 || out != want {
			t.Errorf("%v: status %d output %q, want %q (stderr %q)", argv[1:], code, out, want, errs)
		}
	}
}

// TestAPlusSignedCommandStringNamingItselfIsAnAxis: both signs of `c` select
// the command string, which is unanimous, and one shell then leaves `$0` as
// the string rather than taking it from the first operand.
//
// A bool rather than a three-state answer, because the common denominator
// exists — three of the four read `+c` as `-c` — and refusing an invocation
// every shell runs would be worse than either answer.
func TestAPlusSignedCommandStringNamingItselfIsAnAxis(t *testing.T) {
	const snippet = `echo "0=$0 n=$# args=$*"`
	sh := shell()
	sh.Semantics.PlusSignedCommandStringIsDollarZero = true
	out, errs, code := runArgs(t, sh, "testsh", "+c", snippet, "name", "a")
	if want := "0=" + snippet + " n=2 args=name a\n"; code != 0 || out != want {
		t.Errorf("status %d output %q, want %q (stderr %q)", code, out, want, errs)
	}
	// The sign belongs to the word, so a bundle answers the same way — and
	// an option word of its own after it does not change what the `c` was
	// written with.
	out, errs, code = runArgs(t, sh, "testsh", "+ce", snippet, "name", "a")
	if want := "0=" + snippet + " n=2 args=name a\n"; code != 0 || out != want {
		t.Errorf("bundle: status %d output %q, want %q (stderr %q)", code, out, want, errs)
	}
	// With no operand the two rules still differ: the string keeps `$0`
	// where the other answer leaves the shell's own name there.
	out, _, _ = runArgs(t, sh, "testsh", "+c", snippet)
	if want := "0=" + snippet + " n=0 args=\n"; out != want {
		t.Errorf("no operands: output = %q, want %q", out, want)
	}
	// The minus spelling is unaffected by the answer, which is what makes
	// this the *sign's* question and not the option's.
	out, _, _ = runArgs(t, sh, "testsh", "-c", snippet, "name", "a")
	if want := "0=name n=1 args=a\n"; out != want {
		t.Errorf("minus sign: output = %q, want %q", out, want)
	}
	// And the other answer reads `+c` as `-c` throughout, which is the
	// substrate's own and what three of the panel do.
	for _, argv := range [][]string{
		{"testsh", "+c", snippet, "name", "a"},
		{"testsh", "+ce", snippet, "name", "a"},
	} {
		out, _, code := runArgs(t, shell(), argv...)
		if want := "0=name n=1 args=a\n"; code != 0 || out != want {
			t.Errorf("%v: status %d output %q, want %q", argv[1:], code, out, want)
		}
	}
}
