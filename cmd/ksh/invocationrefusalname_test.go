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

// runKshArgs runs the shell with the words given after argv[0], from a
// directory holding a script `s.sh` that would print if it ran.
func runKshArgs(t *testing.T, args ...string) (out, errs string, code int) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "s.sh"), []byte("echo ran\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var o, e strings.Builder
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &o, &e
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	code = driver.MainArgs(sh, append([]string{"ksh"}, args...))
	return o.String(), e.String(), code
}

// An option word refused at the invocation is the shell's to name, script
// operand or not: the script has not been opened yet. Measured 2026-10-04 on
// ksh93u+ 2012-08-01 (#5722).
func TestAnInvocationRefusalNamesTheShellAndNotTheScript(t *testing.T) {
	const letters = "Usage: ksh [-cilrsDEabefhkmnprtuvxBCGH] [-R file] [-o[option]] [arg ...]\n"
	for _, c := range []struct {
		args []string
		errs string
	}{
		{[]string{"-o", "posix", "s.sh"}, "ksh: posix: bad option(s)\n" + letters},
		{[]string{"-o", "nosuch", "s.sh"}, "ksh: nosuch: bad option(s)\n" + letters},
		{[]string{"--rcfile", "s.sh", "-i", "-c", "echo main"}, "ksh: rcfile: bad option(s)\nUsage: ksh [ options ] [arg ...]\n"},
	} {
		out, errs, code := runKshArgs(t, c.args...)
		if out != "" || errs != c.errs || code != 2 {
			t.Errorf("%q: %q, %q at %d; want nothing, %q at 2", c.args, out, errs, code, c.errs)
		}
	}
}

// `--posix` and every beginning of it are answered with the option string
// alone, and nothing runs. Measured 2026-10-04 on ksh93u+ 2012-08-01 (#5722).
// See interp.Diagnostics.InvocationPosixWordWritesTheOptionString.
func TestThePosixWordWritesTheOptionString(t *testing.T) {
	for _, args := range [][]string{
		{"--posix", "-c", "echo ran"},
		{"--p", "-c", "echo ran"},
		{"--posi", "-c", "echo ran"},
		{"--posix", "s.sh"},
	} {
		out, errs, code := runKshArgs(t, args...)
		if out != "" || errs != "cilrsDER:abefhkmno:prtuvxBCGH\n" || code != 2 {
			t.Errorf("%q: %q, %q at %d; want the option string at 2", args, out, errs, code)
		}
	}
	// The controls: a word that only begins with the name is an ordinary
	// unknown one.
	for _, w := range []string{"--posixx", "--posix=1"} {
		_, errs, code := runKshArgs(t, w, "-c", "echo ran")
		want := "ksh: " + w[2:] + ": bad option(s)\nUsage: ksh [ options ] [arg ...]\n"
		if errs != want || code != 2 {
			t.Errorf("%s: %q at %d; want %q at 2", w, errs, code, want)
		}
	}
}
