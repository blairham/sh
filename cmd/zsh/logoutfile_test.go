// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An interactive login zsh reads `~/.zlogout` however it ends, and no other
// zsh reads it at all (#5996).
//
// Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh) under `env -u
// FPATH`, a scratch `HOME` and `ZDOTDIR` holding the `.zlogout` below and
// standard input a pipe; every expectation is that shell's. `$?` in the file
// is the status the `exit` found, and the context is the file over the route's
// word when an `exit` ran and the file alone at the end of input. This shell
// read no `.zlogout` on any route.
func TestAZshLoginShellReadsItsLogoutFile(t *testing.T) {
	const zlogout = "print -r -- LOGOUT st=$? ctx=$zsh_eval_context\n"
	for _, c := range []struct {
		name, typed string
		argv        []string
		want        string // "" is not read
	}{
		{"typed exit 3 after false", "false; exit 3\n", []string{"zsh", "-l", "-i"}, "LOGOUT st=1 ctx=toplevel file"},
		{"typed logout", "true; logout 4\n", []string{"zsh", "-l", "-i"}, "LOGOUT st=0 ctx=toplevel file"},
		{"end of input", "false\n", []string{"zsh", "-l", "-i"}, "LOGOUT st=1 ctx=file"},
		{"-l -i -c exit", "", []string{"zsh", "-l", "-i", "-c", "false; exit"}, "LOGOUT st=1 ctx=cmdarg file"},
		{"-l -i -c running out", "", []string{"zsh", "-l", "-i", "-c", ":"}, "LOGOUT st=0 ctx=file"},
		{"-l -i -c over set -e", "", []string{"zsh", "-l", "-i", "-c", "set -e; false"}, ""},
		{"-l -c exit, not interactive", "", []string{"zsh", "-l", "-c", "exit"}, ""},
		{"-i, not a login shell", "exit\n", []string{"zsh", "-i"}, ""},
		{"-f", "exit\n", []string{"zsh", "-f", "-l", "-i"}, ""},
		{"rcs turned off first", "unsetopt rcs; exit\n", []string{"zsh", "-l", "-i"}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			writeHomeFile(t, home, ".zlogout", zlogout)
			out, errs, _ := prompt(t, c.typed, c.argv...)
			got := strings.Contains(out, "LOGOUT")
			switch {
			case c.want == "" && got:
				t.Errorf("read the logout file: %q (stderr %q)", out, errs)
			case c.want != "" && !strings.Contains(out, c.want+"\n"):
				t.Errorf("stdout %q, want a line %q (stderr %q)", out, c.want, errs)
			}
		})
	}
}

// And an `exit` in the file is the status the shell leaves with, at the end
// of input as after an `exit`: measured, a `.zlogout` of `exit 5` makes both
// leave 5, and one of `false` leaves the 3 the `exit` named.
func TestAZlogoutsExitIsTheSessionsStatus(t *testing.T) {
	for _, c := range []struct {
		zlogout, typed string
		want           int
	}{
		{"exit 5\n", "exit 3\n", 5},
		{"exit 5\n", "", 5},
		{"false\n", "exit 3\n", 3},
	} {
		home := scratchHome(t)
		writeHomeFile(t, home, ".zlogout", c.zlogout)
		if _, errs, code := prompt(t, c.typed, "zsh", "-l", "-i"); code != c.want {
			t.Errorf("%q then %q: status %d, want %d (stderr %q)", c.zlogout, c.typed, code, c.want, errs)
		}
	}
}

// `rcs` and `globalrcs` are asked before each startup file, not once at the
// start, so a `~/.zshenv` that moves them decides the files after it (#5905).
//
// Measured 2026-10-05 on zsh 5.9.2 through `-l -i -c` with `PATH` at
// `/usr/bin:/bin`, where `/etc/zprofile` adds `path_helper`'s entries and
// `/etc/zshrc` sets `SAVEHIST`; here the system directory is a scratch one
// holding files that announce themselves.
func TestTheRcsOptionsAreAskedBeforeEachStartupFile(t *testing.T) {
	for _, c := range []struct {
		name  string
		argv  []string
		files map[string]string
		want  string
	}{
		{
			"control",
			[]string{"zsh", "-l", "-i", "-c", ":"},
			nil,
			"etc-zprofile\nzprofile\netc-zshrc\nzshrc\nzlogin\n",
		},
		{
			"globalrcs off in .zshenv",
			[]string{"zsh", "-l", "-i", "-c", ":"},
			map[string]string{".zshenv": "unsetopt GLOBAL_RCS\n"},
			"zprofile\nzshrc\nzlogin\n",
		},
		{
			"off in .zshenv, on again in .zprofile",
			[]string{"zsh", "-l", "-i", "-c", ":"},
			map[string]string{".zshenv": "unsetopt GLOBAL_RCS\n", ".zprofile": "echo zprofile; setopt GLOBAL_RCS\n"},
			"zprofile\netc-zshrc\nzshrc\nzlogin\n",
		},
		{
			"-d, and globalrcs on in .zshenv",
			[]string{"zsh", "-d", "-l", "-i", "-c", ":"},
			map[string]string{".zshenv": "setopt GLOBAL_RCS\n"},
			"etc-zprofile\nzprofile\netc-zshrc\nzshrc\nzlogin\n",
		},
		{
			"-d alone",
			[]string{"zsh", "-d", "-l", "-i", "-c", ":"},
			nil,
			"zprofile\nzshrc\nzlogin\n",
		},
		{
			"rcs off in .zshenv",
			[]string{"zsh", "-l", "-i", "-c", ":"},
			map[string]string{".zshenv": "unsetopt RCS\n"},
			"",
		},
		{
			"rcs off in .zprofile",
			[]string{"zsh", "-l", "-i", "-c", ":"},
			map[string]string{".zprofile": "echo zprofile; unsetopt rcs\n"},
			"etc-zprofile\nzprofile\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			for name, body := range map[string]string{".zprofile": "echo zprofile\n", ".zshrc": "echo zshrc\n", ".zlogin": "echo zlogin\n"} {
				writeHomeFile(t, home, name, body)
			}
			for name, body := range c.files {
				writeHomeFile(t, home, name, body)
			}
			sh := scratchShell(t)
			for _, name := range []string{"zprofile", "zshrc"} {
				if err := os.WriteFile(filepath.Join(sh.SystemStartupDirectory, name), []byte("echo etc-"+name+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var o, e bytes.Buffer
			sh.Stdout, sh.Stderr = &o, &e
			null, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = null.Close() }()
			sh.Stdin = null
			driver.MainArgs(sh, c.argv)
			if o.String() != c.want {
				t.Errorf("stdout %q, want %q (stderr %q)", o.String(), c.want, e.String())
			}
		})
	}
}
