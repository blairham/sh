// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The third startup file: the one a shell reads when it is *not* going to
// prompt.
//
// The name of the variable holding its path is the dialect's, which is what
// makes this do nothing at all in most of them; the front end does the
// reading, and the reading is the half a dialect cannot do for itself. Named
// here by a name no shell uses, for the reason the option record is: what is
// under test is the seam and not whose spelling it is.
const startupVar = "SCRIPT_STARTUP_FILE"

// withStartupFile is a shell that has such a file, on the answers a shell
// without one would give for everything else.
func withStartupFile() interp.Semantics {
	s := interp.PosixSemantics()
	s.NonInteractiveStartupVariable = startupVar
	// The other file is the login profile, and it must not be in the way: the
	// two are separate questions and this preset answers yes to that one.
	s.LoginProfileWhenNonInteractive = false
	return s
}

// startupShell is a front end with that dialect and buffers for streams.
func startupShell(t *testing.T, sem interp.Semantics) (Shell, *strings.Builder, *strings.Builder) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	var out, errs strings.Builder
	return Shell{
		Name:      "testsh",
		Dialect:   syntax.Core(),
		Semantics: sem,
		Stdout:    &out,
		Stderr:    &errs,
	}, &out, &errs
}

// TestTheNonInteractiveStartupFileIsReadOnEveryScriptRoute.
//
// Measured 2026-09-05 on bash 5.3.15 with a scratch HOME: the file is sourced
// for a script operand, for `-c` and for a program arriving on standard input
// alike. It is a fact about the shell rather than about the route, exactly as
// the login profile is.
func TestTheNonInteractiveStartupFileIsReadOnEveryScriptRoute(t *testing.T) {
	const probe = `echo "[${FROM_FILE-unset}]"`
	script := scriptAt(t, probe+"\n")
	for _, r := range []struct {
		name  string
		argv  []string
		stdin string
	}{
		{"a script operand", []string{"testsh", script}, ""},
		{"-c", []string{"testsh", "-c", probe}, ""},
		{"standard input", []string{"testsh"}, probe + "\n"},
	} {
		t.Run(r.name, func(t *testing.T) {
			sh, out, errs := startupShell(t, withStartupFile())
			t.Setenv(startupVar, scriptAt(t, "FROM_FILE=yes\n"))
			if r.stdin != "" {
				sh.Stdin = fileHolding(t, r.stdin)
			}
			if code := MainArgs(sh, r.argv); code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			if want := "[yes]\n"; out.String() != want {
				t.Errorf("got %q, want %q", out.String(), want)
			}
		})
	}
}

// TestADialectWithNoSuchFileReadsNothing, which is three of the four: the same
// environment entry is an ordinary variable there and names nothing.
func TestADialectWithNoSuchFileReadsNothing(t *testing.T) {
	sh, out, errs := startupShell(t, interp.PosixSemantics())
	t.Setenv(startupVar, scriptAt(t, "FROM_FILE=yes\n"))
	if code := MainArgs(sh, []string{"testsh", "-c", `echo "[${FROM_FILE-unset}]"`}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "[unset]\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

// TestTheNonInteractiveStartupFileIsNotReadAtAPrompt.
//
// The whole of what separates it from `$ENV`. Measured: the shell that has
// this reads it when it is not interactive and reads a file of its own name
// when it is, and never both — so a person's prompt settings do not reach a
// script and a script's do not reach the prompt.
func TestTheNonInteractiveStartupFileIsNotReadAtAPrompt(t *testing.T) {
	sh := shellWithSemantics(t, withStartupFile())
	t.Setenv(startupVar, scriptAt(t, "echo FROM-FILE\n"))
	var out, errs strings.Builder
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Stdin = fileHolding(t, "echo typed\n")
	if code := InteractiveArgs(sh, []string{"testsh", "-i"}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, &errs)
	}
	if strings.Contains(out.String(), "FROM-FILE") {
		t.Errorf("out = %q, want the script's startup file left alone at a prompt", out.String())
	}
	if !strings.Contains(out.String(), "typed") {
		t.Errorf("out = %q, want the typed line to have run", out.String())
	}
}

// shellWithSemantics is startupShell without the streams, for a test that
// wants to set its own.
func shellWithSemantics(t *testing.T, sem interp.Semantics) Shell {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return Shell{Name: "testsh", Dialect: syntax.Core(), Semantics: sem}
}

// TestPosixModeSuppressesTheNonInteractiveStartupFile.
//
// Measured: the shell that has this file reads nothing when it was started
// with the standard's posix option, and nothing when it was invoked under the
// standard's own name. Two spellings of one mode, which is why the front end
// asks the runner rather than carrying a second answer of its own.
func TestPosixModeSuppressesTheNonInteractiveStartupFile(t *testing.T) {
	sem := withStartupFile()
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"in posix mode", []string{"testsh", "-o", "posix", "-c", `echo "[${FROM_FILE-unset}]"`}, "[unset]\n"},
		{"otherwise", []string{"testsh", "-c", `echo "[${FROM_FILE-unset}]"`}, "[yes]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh, out, errs := startupShell(t, sem)
			// The dialect has to *have* the name for the option to be
			// spellable at all; whether it does is its answer and not the
			// mode's.
			sh.Register = func(r *interp.Runner) { r.AddSetOptions("posix") }
			t.Setenv(startupVar, scriptAt(t, "FROM_FILE=yes\n"))
			if code := MainArgs(sh, tc.argv); code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			if out.String() != tc.want {
				t.Errorf("got %q, want %q", out.String(), tc.want)
			}
		})
	}
}

// TestTheStartupFilesPathIsExpanded, since `$HOME/…` is how such a path is
// written far more often than not — the same treatment `$ENV` already gets.
func TestTheStartupFilesPathIsExpanded(t *testing.T) {
	sh, out, errs := startupShell(t, withStartupFile())
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "startup.sh"), []byte("FROM_FILE=yes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(startupVar, "$HOME/startup.sh")
	if code := MainArgs(sh, []string{"testsh", "-c", `echo "[${FROM_FILE-unset}]"`}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "[yes]\n"; out.String() != want {
		t.Errorf("got %q, want %q — the value is a path with parameters in it", out.String(), want)
	}
}

// TestTheStartupFileSeesTheInvocation. It is run *by* the shell that is about
// to run the script, so it sees that shell's `$0`, its parameters and its
// options — all measured, and the reason it is sourced after the runner is
// built and after the invocation's options are applied.
func TestTheStartupFileSeesTheInvocation(t *testing.T) {
	sh, out, errs := startupShell(t, withStartupFile())
	t.Setenv(startupVar, scriptAt(t, `echo "FILE [$0] n=$# [${1-}] x=$-"`+"\n"))
	script := scriptAt(t, `echo "SCRIPT [$0] n=$#"`+"\n")
	if code := MainArgs(sh, []string{"testsh", "-x", script, "A", "B"}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	want := "FILE [" + script + "] n=2 [A] x=x\nSCRIPT [" + script + "] n=2\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

// TestAStartupFileThatExitsEndsTheShell, measured: `exit 3` in it exits 3 and
// the script never runs. Through Finish, so an EXIT trap it set still fires —
// the same shape the login profile has.
func TestAStartupFileThatExitsEndsTheShell(t *testing.T) {
	sh, out, errs := startupShell(t, withStartupFile())
	t.Setenv(startupVar, scriptAt(t, "trap 'echo TRAP' EXIT\necho FILE\nexit 3\n"))
	code := MainArgs(sh, []string{"testsh", "-c", "echo SCRIPT"})
	if code != 3 {
		t.Errorf("status %d, want 3, stderr %q", code, errs)
	}
	if want := "FILE\nTRAP\n"; out.String() != want {
		t.Errorf("got %q, want %q — the script must not run", out.String(), want)
	}
}

// TestAMissingNonInteractiveStartupFileIsNotAFailure, and neither is an unset variable.
// Every shell starts for the first time without one.
func TestAMissingNonInteractiveStartupFileIsNotAFailure(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"a path that is not there", "/nope/not-a-file"},
		{"an empty value", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh, out, errs := startupShell(t, withStartupFile())
			t.Setenv(startupVar, tc.value)
			if code := MainArgs(sh, []string{"testsh", "-c", "echo ran"}); code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			if out.String() != "ran\n" {
				t.Errorf("got %q", out.String())
			}
		})
	}
}

// The environment's own option list is the other startup input the front end
// reads, and the front end's share of it is *when*.

// TestTheEnvironmentsOptionsOutrankTheInvocations.
//
// Measured 2026-09-05 on bash 5.3.15: an inherited `xtrace` beats the
// invocation's own `+x`, while an inherited name and an explicit `-x` of
// course agree. So the environment is read after the argument vector, which is
// the opposite of the order every other startup input takes.
func TestTheEnvironmentsOptionsOutrankTheInvocations(t *testing.T) {
	const probe = `case $- in *u*) echo has-u ;; *) echo no-u ;; esac`
	for _, tc := range []struct{ name, opt, want string }{
		{"the invocation turns it off", "+u", "has-u\n"},
		{"the invocation turns it on", "-u", "has-u\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh, out, errs := startupShell(t, withStartupFile())
			sh.Register = func(r *interp.Runner) { r.SetShellOptions(optionRecordVar) }
			t.Setenv(optionRecordVar, "nounset")
			if code := MainArgs(sh, []string{"testsh", tc.opt, "-c", probe}); code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			if out.String() != tc.want {
				t.Errorf("got %q, want %q", out.String(), tc.want)
			}
		})
	}
}

// TestTheEnvironmentsOptionsAreOnBeforeTheStartupFiles, also measured: the
// file this shell reads before a script is itself traced by an inherited
// `xtrace`. So the environment is read after the argument vector and before
// the files, which is a position of its own rather than either end.
func TestTheEnvironmentsOptionsAreOnBeforeTheStartupFiles(t *testing.T) {
	sh, out, errs := startupShell(t, withStartupFile())
	sh.Register = func(r *interp.Runner) { r.SetShellOptions(optionRecordVar) }
	t.Setenv(optionRecordVar, "nounset")
	t.Setenv(startupVar, scriptAt(t, `case $- in *u*) echo file-sees-u ;; *) echo file-does-not ;; esac`+"\n"))
	if code := MainArgs(sh, []string{"testsh", "-c", "true"}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "file-sees-u\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

// optionRecordVar is the name this package's tests give the option record,
// which is a name no shell uses for the reason the file's variable above is:
// the seam is what is under test.
const optionRecordVar = "OPTIONRECORD"

// A diagnostic raised at the top level of a startup file names the startup
// file, not the script the shell was started for.
//
// Driven at full depth on purpose: the startup file is read while the shell is
// running a *script*, which is the only arrangement in which the two names
// differ. A one-line probe has nothing to confuse the file with, so it would
// have passed against the shell that got this wrong — the location fell back
// to the shell's own name, and on the script route that name is the script's
// path (#1123).
func TestADiagnosticInTheStartupFileNamesTheStartupFile(t *testing.T) {
	sem := withStartupFile()
	sh, out, errs := startupShell(t, sem)
	sh.Diagnostics = interp.Diagnostics{
		Location:                    interp.LocationTightLine,
		LocationNamesTheCurrentFile: true,
	}
	rc := scriptAt(t, "echo RC-BEFORE\n# a line to count past\nnosuchcmd_zz\necho RC-AFTER\n")
	script := scriptAt(t, "echo MAIN-RAN\n")
	t.Setenv(startupVar, rc)

	if code := MainArgs(sh, []string{"testsh", script}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := rc + ":3: "; !strings.Contains(errs.String(), want) {
		t.Errorf("said %q, want it to name %q", errs.String(), want)
	}
	if strings.Contains(errs.String(), script) {
		t.Errorf("said %q, want the script not named — it is the file that is fine", errs.String())
	}
	// And the file is read to the end and the script still runs, which is what
	// says the frame changed the name and nothing else.
	if want := "RC-BEFORE\nRC-AFTER\nMAIN-RAN\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

// And the frame is given back, so a failure in the script names the script.
// Without that, reading any startup file at all would name the rc for the rest
// of the run.
func TestAfterTheStartupFileTheScriptIsNamedAgain(t *testing.T) {
	sh, _, errs := startupShell(t, withStartupFile())
	sh.Diagnostics = interp.Diagnostics{
		Location:                    interp.LocationTightLine,
		LocationNamesTheCurrentFile: true,
	}
	rc := scriptAt(t, "true\n")
	script := scriptAt(t, "true\nnosuchcmd_zz\n")
	t.Setenv(startupVar, rc)

	MainArgs(sh, []string{"testsh", script})
	if want := script + ":2: "; !strings.Contains(errs.String(), want) {
		t.Errorf("said %q, want it to name %q", errs.String(), want)
	}
}
