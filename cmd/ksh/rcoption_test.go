// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// #3255: `-o rc` and `-o login_shell` were refused at an invocation this shell
// grants them at, and the issue's premise about both was wrong. Neither is an
// inert row: `rc` **is** the `-E` letter — read the run-commands file though
// this shell is not interactive — and `login_shell` **is** `-l`, which runs
// the profile and puts `l` in `$-`.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01 under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, with `$HOME` an empty directory holding a `.profile` that
// announces itself and `$ENV` pointing at a file that does the same. Every row
// below is that shell's exact answer, and every one of them is a state built
// rather than a refusal lifted.
//
// Against the *binary's* shell value, because what these decide is which files
// an invocation reads and that is the front end's.
func TestTheRunCommandsAndLoginOptionsAtAnInvocation(t *testing.T) {
	for _, c := range []struct {
		name string
		argv []string
		want string
	}{
		{
			// The default, for the comparison every row below rests on:
			// neither file is read and neither letter is in `$-`.
			name: "a plain command string reads neither file",
			argv: []string{"ksh", "-c", `echo "[$-]"`},
			want: "[chsB]\n",
		},
		{
			// `-E` reads the run-commands file in a shell that is not
			// interactive at all, and the letter is in `$-`.
			name: "the letter reads the run-commands file",
			argv: []string{"ksh", "-E", "-c", `echo "[$-]"`},
			want: "ENV-RAN\n[chsBE]\n",
		},
		{
			name: "and the name is the same invocation",
			argv: []string{"ksh", "-o", "rc", "-c", `echo "[$-]"`},
			want: "ENV-RAN\n[chsBE]\n",
		},
		{
			// Both directions: the plus is granted and reads nothing.
			name: "the plus reads nothing",
			argv: []string{"ksh", "+E", "-c", `echo "[$-]"`},
			want: "[chsB]\n",
		},
		{
			name: "and so does the negated name",
			argv: []string{"ksh", "-o", "norc", "-c", `echo "[$-]"`},
			want: "[chsB]\n",
		},
		{
			// The listing and the option test agree about this row.
			name: "the listing follows the option",
			argv: []string{"ksh", "-E", "-c", `set -o | grep "^rc "`},
			want: "ENV-RAN\nrc                       on\n",
		},
		{
			name: "the login name runs the profile and shows the letter",
			argv: []string{"ksh", "-o", "login_shell", "-c", `echo "[$-]"`},
			want: "PROFILE-RAN\n[chsBl]\n",
		},
		{
			name: "which is what the letter already did",
			argv: []string{"ksh", "-l", "-c", `echo "[$-]"`},
			want: "PROFILE-RAN\n[chsBl]\n",
		},
		{
			name: "and the negated name is granted and does nothing",
			argv: []string{"ksh", "-o", "nologin_shell", "-c", `echo "[$-]"`},
			want: "[chsB]\n",
		},
		{
			// The two compose, which is how they are told apart from one
			// switch spelled twice.
			name: "the two compose",
			argv: []string{"ksh", "-o", "login_shell", "-o", "rc", "-c", `echo "[$-]"`},
			want: "PROFILE-RAN\nENV-RAN\n[chsBEl]\n",
		},
		{
			name: "and so do the letters",
			argv: []string{"ksh", "-l", "-E", "-c", `echo "[$-]"`},
			want: "PROFILE-RAN\nENV-RAN\n[chsBEl]\n",
		},
		{
			// The login row's two readings disagree in this shell, which a
			// single reading of the state could not have said: the listing
			// is `off` on **every** route and the option test is true on the
			// login ones.
			name: "the listing says off where the option test says yes",
			argv: []string{"ksh", "-l", "-c", `set -o | grep "^login_shell "; [[ -o login_shell ]] && echo yes || echo no`},
			want: "PROFILE-RAN\nlogin_shell              off\nyes\n",
		},
		{
			name: "and off and no where the shell is neither",
			argv: []string{"ksh", "-c", `set -o | grep "^login_shell "; [[ -o login_shell ]] && echo yes || echo no`},
			want: "login_shell              off\nno\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runKshHome(t, c.argv)
			if code != 0 {
				t.Fatalf("status %d, want 0 (err %q)", code, errs)
			}
			if out != c.want {
				t.Errorf("stdout %q, want %q (err %q)", out, c.want, errs)
			}
		})
	}
}

// And the route split stays: a script may move neither name, and the *letter*
// is refused with the letter's own sentence.
//
// Measured in the same run: `set -E` is `set: -E: unknown option` and `set -o
// rc` is `set: rc: bad option(s)` — two wordings, which is this shell telling
// a letter it has never heard of from a name it lists and will not take. A
// letter that resolved to the name and borrowed its sentence would have been a
// plausible-looking wrong answer.
func TestTheRunCommandsOptionIsStillRefusedToAScript(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`set -E`, "set: -E: unknown option"},
		{`set +E`, "set: +E: unknown option"},
		{`set -o rc`, "set: rc: bad option(s)"},
		{`set +o rc`, "set: rc: bad option(s)"},
		{`set -o login_shell`, "set: login_shell: bad option(s)"},
	} {
		t.Run(c.src, func(t *testing.T) {
			_, errs, code := runKshHome(t, []string{"ksh", "-c", c.src})
			if code != 2 {
				t.Errorf("status %d, want 2 (err %q)", code, errs)
			}
			if !strings.Contains(errs, c.want) {
				t.Errorf("stderr %q, want it to carry %q", errs, c.want)
			}
		})
	}
}

// runKshHome runs the binary's own shell with a scratch home holding a
// `.profile` and an `$ENV` file, each of which announces itself when read.
//
// A marker per file rather than one, because the two options this suite is
// about read *different* files and a single marker could not tell a row that
// read the wrong one from a row that read the right one.
func runKshHome(t *testing.T, argv []string) (out, errs string, code int) {
	t.Helper()
	home := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(home, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write(".profile", "echo PROFILE-RAN\n")
	env := write("envfile", "echo ENV-RAN\n")
	t.Setenv("HOME", home)
	t.Setenv("ENV", env)
	var o, e strings.Builder
	sh := shell()
	// The machine's own `/etc` is replaced, for the reason cmd/zsh's
	// scratchShell replaces it: a test that read it would be measuring the
	// runner it happened to be on.
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &o, &e
	code = driver.MainArgs(sh, argv)
	return o.String(), e.String(), code
}
