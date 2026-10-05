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

// Input that runs out inside a quote, a backquote, a `${` or a `$((` leaves
// the failing status the last command left, and 2 only over a success; a
// command substitution's `(` and every failure the grammar finds are 2 over
// anything (#5882).
//
// Measured 2026-10-05 on bash 5.3.20 (/opt/homebrew/bin/bash) with standard
// input on the null device, the first line `sh -c "exit 7"` — a status no
// syntax status in the panel could be mistaken for — and the second the one
// in the table. Every route that reads a program a line at a time agrees;
// `.` says 2 whatever came before. This shell said 2 on every row.
func TestAnUnclosedQuoteAtTheEndKeepsAFailingStatus(t *testing.T) {
	const pre = "sh -c 'exit 7'\n"
	for _, c := range []struct {
		line string
		want int
	}{
		{`echo "abc`, 7},
		{`echo 'abc`, 7},
		{`echo $'abc`, 7},
		{"echo `x", 7},
		{`echo ${x`, 7},
		{`echo $((1+`, 7},
		{`echo $[1+`, 7},
		{`echo $(echo "x`, 7},
		{`echo $(`, 2},
		{`echo <(x`, 2},
		{`echo "$(echo`, 2},
		{`echo )`, 2},
		{`if true; then`, 2},
	} {
		src := pre + c.line + "\n"
		home := scratchHome(t)
		script := filepath.Join(home, "s.sh")
		writeHomeFile(t, home, "s.sh", src)
		for _, route := range []struct {
			name  string
			argv  []string
			stdin string
		}{
			{"script", []string{"bash", script}, ""},
			{"-c", []string{"bash", "-c", src}, ""},
			{"stdin", []string{"bash"}, script},
		} {
			sh := scratchShell(t)
			var o, e bytes.Buffer
			sh.Stdout, sh.Stderr = &o, &e
			in := os.DevNull
			if route.stdin != "" {
				in = route.stdin
			}
			f, err := os.Open(in)
			if err != nil {
				t.Fatal(err)
			}
			sh.Stdin = f
			got := driver.MainArgs(sh, route.argv)
			_ = f.Close()
			if got != c.want {
				t.Errorf("%s, %q: status %d, want %d (stderr %q)", route.name, c.line, got, c.want, e.String())
			}
		}
	}
}

// The failing command has to have run: one on the failing line itself never
// did, so `false; echo "abc` is 2, and `false` with `true; echo "abc` after
// it is 1. And `.` of either is 2, whatever came before. Measured on the same
// bash.
func TestOnlyACommandThatRanKeepsItsStatusOverAnUnclosedQuote(t *testing.T) {
	for _, c := range []struct {
		src  string
		want int
	}{
		{"false; echo \"abc\n", 2},
		{"false\ntrue; echo \"abc\n", 1},
	} {
		home := scratchHome(t)
		writeHomeFile(t, home, "s.sh", c.src)
		var o, e bytes.Buffer
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &o, &e
		if code := driver.MainArgs(sh, []string{"bash", "-c", c.src}); code != c.want {
			t.Errorf("-c %q: status %d, want %d", c.src, code, c.want)
		}
		o.Reset()
		sh = scratchShell(t)
		sh.Stdout, sh.Stderr = &o, &e
		driver.MainArgs(sh, []string{"bash", "-c", `false; . "$1"; echo st=$?`, "bash", filepath.Join(home, "s.sh")})
		if !strings.Contains(o.String(), "st=2") {
			t.Errorf("`.` of %q: said %q, want st=2", c.src, o.String())
		}
	}
}

// And the startup route, which goes through the same answer: a `$BASH_ENV` of
// `false` and then `echo ${x` leaves 1 for the command, where `true` in front
// leaves 2. Measured on the same bash.
func TestAnUnclosedBraceInBashEnvKeepsAFailingStatus(t *testing.T) {
	for _, c := range []struct{ pre, want string }{
		{"false", "st=1\n"},
		{"true", "st=2\n"},
	} {
		home := scratchHome(t)
		writeHomeFile(t, home, "rc.sh", c.pre+"\necho ${x\n")
		t.Setenv("BASH_ENV", filepath.Join(home, "rc.sh"))
		var o, e bytes.Buffer
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &o, &e
		null, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		sh.Stdin = null
		driver.MainArgs(sh, []string{"bash", "-c", "echo st=$?"})
		_ = null.Close()
		if o.String() != c.want {
			t.Errorf("%s: stdout %q, want %q", c.pre, o.String(), c.want)
		}
	}
}
