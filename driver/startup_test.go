// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A login shell reads ~/.profile and an interactive one reads $ENV, and the
// two answer different questions: what a person wants set for everything
// started from the session, and what only makes sense at a prompt.
func TestStartupFiles(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".profile"), "FROM_PROFILE=yes\n")
	write(t, filepath.Join(home, "rc.sh"), "FROM_ENV=yes\n")

	for _, c := range []struct {
		name          string
		login         bool
		env           string
		profile, rcSh bool
	}{
		{"interactive", false, filepath.Join(home, "rc.sh"), false, true},
		{"login", true, filepath.Join(home, "rc.sh"), true, true},
		{"login with no ENV", true, "", true, false},
		{"interactive with no ENV", false, "", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			sh, r := newTestShell(t, map[string]string{"HOME": home, "ENV": c.env})
			if code := sh.startup(r, source{login: c.login, interactive: true}); code != 0 {
				t.Fatalf("startup reported %d", code)
			}
			if _, ok := r.GetVar("FROM_PROFILE"); ok != c.profile {
				t.Errorf(".profile read = %v, want %v", ok, c.profile)
			}
			if _, ok := r.GetVar("FROM_ENV"); ok != c.rcSh {
				t.Errorf("$ENV read = %v, want %v", ok, c.rcSh)
			}
		})
	}
}

// ENV is a path with parameters in it more often than not, so it is expanded
// before it is opened — `$HOME/.shrc` is the usual spelling.
func TestEnvIsExpanded(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".shrc"), "FROM_ENV=yes\n")
	sh, r := newTestShell(t, map[string]string{"HOME": home, "ENV": "$HOME/.shrc"})
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	if _, ok := r.GetVar("FROM_ENV"); !ok {
		t.Error("the expanded ENV was not read")
	}
}

// A file that is not there is not a failure. Every shell starts for the first
// time with none of these, and refusing to start would be unusable on a fresh
// machine.
func TestAMissingStartupFileIsNotAFailure(t *testing.T) {
	home := t.TempDir()
	sh, r := newTestShell(t, map[string]string{
		"HOME": home, "ENV": filepath.Join(home, "nothing-here"),
	})
	if code := sh.startup(r, source{login: true, interactive: true}); code != 0 {
		t.Errorf("a missing file reported %d, want it ignored", code)
	}
	// Nor is a directory where a file was named.
	sh, r = newTestShell(t, map[string]string{"HOME": home, "ENV": home})
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Errorf("a directory reported %d, want it ignored", code)
	}
}

// A startup file that does not parse is reported rather than ignored: the
// person wrote it, and silence would leave them wondering why their settings
// were not there.
func TestAStartupFileThatDoesNotParse(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "rc.sh")
	write(t, path, "if\n")
	var errs strings.Builder
	sh, r := newTestShell(t, map[string]string{"HOME": home, "ENV": path})
	sh.Stderr = &errs
	if code := sh.startup(r, source{interactive: true}); code == 0 {
		t.Error("a broken startup file reported 0, want a failure")
	}
	if !strings.Contains(errs.String(), path) {
		t.Errorf("said %q, want the file named", errs.String())
	}
}

// With no HOME there is no ~/.profile to name, and nothing to complain about.
func TestNoHome(t *testing.T) {
	sh, r := newTestShell(t, nil)
	if code := sh.startup(r, source{login: true, interactive: true}); code != 0 {
		t.Errorf("reported %d with no HOME, want it skipped", code)
	}
}

// A login shell is one whose argv[0] begins with a dash, which is the
// convention `login` and every terminal emulator follows — there is nowhere
// else to put it, since the shell is exec'd with no arguments of its own.
func TestLoginShell(t *testing.T) {
	for _, c := range []struct {
		argv []string
		want bool
	}{
		{[]string{"-sh"}, true},
		{[]string{"-bash"}, true},
		{[]string{"sh"}, false},
		{[]string{"/bin/sh"}, false},
		{[]string{""}, false},
		{nil, false},
	} {
		if got := LoginShell(c.argv); got != c.want {
			t.Errorf("%q gave %v, want %v", c.argv, got, c.want)
		}
	}
}

func newTestShell(t *testing.T, vars map[string]string) (Shell, *interp.Runner) {
	t.Helper()
	sem := interp.PosixSemantics()
	// The same vector on both, which is what a binary does: the shell's copy
	// is what names the startup files and the runner's is what runs them, and
	// a test giving only one of them the preset was asking a shell with no
	// profile to read one.
	sh := Shell{Semantics: sem, Stdout: &strings.Builder{}, Stderr: &strings.Builder{}}
	r := &interp.Runner{Semantics: &sem, Vars: vars, Stdout: sh.Stdout, Stderr: sh.Stderr}
	return sh, r
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// With no home there is nothing to name, and in particular not `/.profile` —
// which is a real path on a real machine and belongs to root. Checked here
// rather than through startup, where the file simply would not exist on the
// machine running the test and the mistake would go unseen.
func TestHomeFileNeedsAHome(t *testing.T) {
	sem := interp.PosixSemantics()
	sh := Shell{Semantics: sem}
	r := &interp.Runner{Semantics: &sem, Vars: map[string]string{"HOME": "/home/someone"}}
	if got := sh.startupPath(r, ".profile"); got != "/home/someone/.profile" {
		t.Errorf("got %q", got)
	}
	// An empty environment as well as empty Vars: a Runner with no Vars
	// still sees the process's own HOME, which is the right answer for a
	// shell and the wrong one for this test.
	r = &interp.Runner{Semantics: &sem, Env: []string{}}
	if got := sh.startupPath(r, ".profile"); got != "" {
		t.Errorf("with no HOME got %q, want nothing — not root's", got)
	}
	// And a name of nothing is nothing, however good the directory: the
	// dialects that have no file for a slot leave the name empty, and a
	// path built from one would be the home directory itself.
	r = &interp.Runner{Semantics: &sem, Vars: map[string]string{"HOME": "/home/someone"}}
	if got := sh.startupPath(r, ""); got != "" {
		t.Errorf("an unnamed file gave %q, want nothing", got)
	}
}

// The startup directory is a variable the dialect names, and it redirects
// every file rather than one of them. zsh's ZDOTDIR is the only one.
func TestTheStartupDirectoryVariableRedirects(t *testing.T) {
	sem := interp.PosixSemantics()
	sem.StartupDirectoryVariable = "ZDOTDIR"
	sh := Shell{Semantics: sem}
	vars := map[string]string{"HOME": "/home/someone", "ZDOTDIR": "/elsewhere"}
	r := &interp.Runner{Semantics: &sem, Vars: vars}
	if got := sh.startupPath(r, ".rc"); got != "/elsewhere/.rc" {
		t.Errorf("got %q, want the named directory", got)
	}
	// Set to nothing is not the same as unset, which is measured: a shell
	// whose directory variable is the empty string reads none of its files
	// rather than falling back to the home directory.
	vars["ZDOTDIR"] = ""
	if got := sh.startupPath(r, ".rc"); got != "" {
		t.Errorf("an empty directory gave %q, want nothing", got)
	}
	// Unset, and the home directory answers again.
	delete(vars, "ZDOTDIR")
	if got := sh.startupPath(r, ".rc"); got != "/home/someone/.rc" {
		t.Errorf("got %q, want the home directory", got)
	}
	// A dialect that names no variable never looks for one, however set it
	// happens to be — this is the three shells that have no ZDOTDIR.
	vars["ZDOTDIR"] = "/elsewhere"
	plain := Shell{Semantics: interp.PosixSemantics()}
	if got := plain.startupPath(r, ".rc"); got != "/home/someone/.rc" {
		t.Errorf("got %q, want the home directory for a dialect with no such variable", got)
	}
}

// TestAnErrorInAStartupFileCostsThatFileAndNotTheSession: a startup file is a
// file of its own, and every shell that reads one without a person on the
// other end agrees. Measured: a `$BASH_ENV` or a `$ZDOTDIR/.zshenv` whose
// third line is `echo X${NOPE}` under `set -u` stops there, the startup files
// after it are still read, and the script the shell was started for still
// runs. Before this the same error ended the session — one bad line in one
// file cost a person the whole rest of their invocation, silently.
//
// Two files, because the second is the half that says the *session* survived
// rather than only that startup returned zero.
func TestAnErrorInAStartupFileCostsThatFileAndNotTheSession(t *testing.T) {
	home := t.TempDir()
	first := filepath.Join(home, "rc.sh")
	write(t, first, "FIRST=yes\nset -u\necho X${NOPE_UNSET_VAR}\nAFTER_ERROR=yes\n")
	write(t, filepath.Join(home, ".profile"), "PROFILE=yes\n")

	var errs strings.Builder
	sh, r := newTestShell(t, map[string]string{"HOME": home, "ENV": first})
	sh.Stderr = &errs
	r.Stderr = &errs
	if code := sh.startup(r, source{login: true, interactive: true}); code != 0 {
		t.Fatalf("startup reported %d, want the error to cost the file only", code)
	}
	if r.Exited() {
		t.Error("Exited() = true; an error in a startup file must not end the session")
	}
	if _, ok := r.GetVar("FIRST"); !ok {
		t.Error("the failing file did not run at all")
	}
	if _, ok := r.GetVar("AFTER_ERROR"); ok {
		t.Error("the failing file kept going past its error")
	}
	if _, ok := r.GetVar("PROFILE"); !ok {
		t.Error("the startup file after the failing one was not read")
	}
	if !strings.Contains(errs.String(), "NOPE_UNSET_VAR") {
		t.Errorf("said %q, want the failure reported", errs.String())
	}
}

// TestAnExitInAStartupFileStillEndsTheSession is the neighboring rule, and
// the one the catch above must not repair: `exit 3` in a startup file exits 3
// and the files after it are not read, measured in every shell that reads more
// than one.
func TestAnExitInAStartupFileStillEndsTheSession(t *testing.T) {
	home := t.TempDir()
	first := filepath.Join(home, "rc.sh")
	write(t, first, "FIRST=yes\nexit 3\nAFTER_EXIT=yes\n")
	write(t, filepath.Join(home, ".profile"), "PROFILE=yes\n")

	sh, r := newTestShell(t, map[string]string{"HOME": home, "ENV": first})
	// The run-commands file is read before the profile only for a shell that
	// is interactive and not a login shell, so the order here is the one that
	// puts the exiting file first.
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	if !r.Exited() {
		t.Error("Exited() = false after `exit 3` in a startup file")
	}
	if _, ok := r.GetVar("AFTER_EXIT"); ok {
		t.Error("the file kept going past its exit")
	}
	if r.ExitStatus() != 3 {
		t.Errorf("status = %d, want 3", r.ExitStatus())
	}
}

// A startup file is a sourced script, which is what gives a `return` in one
// something to return from.
//
// The probe is deliberately **unconditional**. The natural one —
// `[ -z "$PS1" ] && return` — cannot tell the hypotheses apart: where PS1 is
// set the guard never fires and the `return` never runs, so the file reads as
// accepted whether or not the shell would have accepted it (#1422).
//
// Measured through a pty with `echo BEFORE; return 3; echo AFTER` as the whole
// file: bash 5.3.15, bash 3.2.57, bash under argv[0] `sh`, dash, ksh93 and zsh
// 5.9.2 all print BEFORE, stop there, and diagnose nothing.
func TestAReturnInAStartupFileIsObeyed(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "rc.sh"), "BEFORE=yes\nreturn 3\nAFTER=yes\n")
	sem := interp.PosixSemantics()
	// The answer that refuses a `return` with nothing to return from, on
	// purpose: a startup file has to be a place one is obeyed *whatever* a
	// script's own top level does, and a test that answered No here would
	// pass for a shell that had never been given a frame.
	sem.ReturnOutsideAFunctionIsRefused = interp.Yes
	sem.StartupFileReturnCarriesItsArgument = interp.Yes
	out := &strings.Builder{}
	sh := Shell{Semantics: sem, Stdout: out, Stderr: out}
	r := &interp.Runner{
		Semantics: &sem, Stdout: out, Stderr: out,
		Vars: map[string]string{"HOME": home, "ENV": filepath.Join(home, "rc.sh")},
	}
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	if _, ok := r.GetVar("BEFORE"); !ok {
		t.Error("the lines before the return did not run")
	}
	if _, ok := r.GetVar("AFTER"); ok {
		t.Error("the file did not stop at the return")
	}
	if said := out.String(); said != "" {
		t.Errorf("said %q, want nothing — no shell in the panel diagnoses this", said)
	}
	if r.ExitStatus() != 3 {
		t.Errorf("status = %d, want the returned 3", r.ExitStatus())
	}
}
