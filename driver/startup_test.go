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
			if code := sh.startup(r, c.login); code != 0 {
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
	if code := sh.startup(r, false); code != 0 {
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
	if code := sh.startup(r, true); code != 0 {
		t.Errorf("a missing file reported %d, want it ignored", code)
	}
	// Nor is a directory where a file was named.
	sh, r = newTestShell(t, map[string]string{"HOME": home, "ENV": home})
	if code := sh.startup(r, false); code != 0 {
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
	if code := sh.startup(r, false); code == 0 {
		t.Error("a broken startup file reported 0, want a failure")
	}
	if !strings.Contains(errs.String(), path) {
		t.Errorf("said %q, want the file named", errs.String())
	}
}

// With no HOME there is no ~/.profile to name, and nothing to complain about.
func TestNoHome(t *testing.T) {
	sh, r := newTestShell(t, nil)
	if code := sh.startup(r, true); code != 0 {
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
	sh := Shell{Stdout: &strings.Builder{}, Stderr: &strings.Builder{}}
	sem := interp.PosixSemantics()
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
	r := &interp.Runner{Semantics: &sem, Vars: map[string]string{"HOME": "/home/someone"}}
	if got := homeFile(r, ".profile"); got != "/home/someone/.profile" {
		t.Errorf("got %q", got)
	}
	// An empty environment as well as empty Vars: a Runner with no Vars
	// still sees the process's own HOME, which is the right answer for a
	// shell and the wrong one for this test.
	r = &interp.Runner{Semantics: &sem, Env: []string{}}
	if got := homeFile(r, ".profile"); got != "" {
		t.Errorf("with no HOME got %q, want nothing — not root's", got)
	}
}
