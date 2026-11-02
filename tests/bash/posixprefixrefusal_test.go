// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// POSIX mode sharpens what a refused assignment prefix costs, and it moves
// **both** answers this shell holds about one.
//
// Every expected string is a transcript, measured 2026-09-18 on bash 5.3.20
// from a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch
// HOME, `--norc --noprofile`, with `; echo pre=$?` behind the command and
// `echo b=$?` on the line after it. Without the mode every one of these prints
// the complaint, runs the command and reports 0, which is what
// Semantics.PrefixRefusalCostsTheCommand records and what this shell did in
// the mode as well (#3471).
func TestPosixModeSharpensAPrefixRefusal(t *testing.T) {
	const head = "set -o posix\nreadonly v=1\n"
	for _, c := range []struct{ name, src, want string }{
		{"a regular builtin", "v=3 true; echo pre=$?\necho b=$?\n", "b=1\n"},
		{"an external command", "v=3 /usr/bin/true; echo pre=$?\necho b=$?\n", "b=1\n"},
		{"a function", "f(){ :; }\nv=3 f; echo pre=$?\necho b=$?\n", "b=1\n"},
		// The give-up unwinds out of everything but a subshell, which is the
		// shape the core's own give-ups already had.
		{"inside a function body", "f(){ v=3 true; echo in=$?; }\nf; echo after=$?\necho b=$?\n", "b=1\n"},
		{"inside an `if`", "if true; then v=3 true; echo in=$?; fi; echo after=$?\necho b=$?\n", "b=1\n"},
		{"behind a `||`", "v=3 true || echo or=$?\necho b=$?\n", "b=1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), head+c.src)
			// The complaint is on standard error, which runBash folds in.
			if !strings.Contains(out, "v: readonly variable") {
				t.Errorf("said %q, want the refusal", out)
			}
			rest := out[strings.Index(out, "\n")+1:]
			if rest != c.want {
				t.Errorf("after the refusal %q, want %q — the rest of the command list goes with it", rest, c.want)
			}
			if st != 0 {
				t.Errorf("status %d, want 0 — the script carries on at the next line", st)
			}
		})
	}
	// A subshell contains it: the parent's next command runs and reports the
	// give-up's own status.
	out, st := runBash(t, t.TempDir(), head+"( v=3 true; echo in=$? ); echo after=$?\necho b=$?\n")
	if !strings.HasSuffix(out, "after=1\nb=0\n") || st != 0 {
		t.Errorf("a subshell gave %q status %d, want it contained", out, st)
	}
	// A special builtin is the other half, and it ends the script.
	for _, src := range []string{"v=3 :; echo pre=$?\necho after\n", "v=3 export x=1; echo pre=$?\necho after\n"} {
		out, st := runBash(t, t.TempDir(), head+src)
		if strings.Contains(out, "after") || st != 1 {
			t.Errorf("%q gave %q status %d, want the script to end at 1", src, out, st)
		}
	}
	// And leaving the mode puts the dialect's own answer back, which is what
	// says this is a mode rather than a build.
	out, st = runBash(t, t.TempDir(), head+"set +o posix\nv=3 true; echo pre=$?\necho b=$?\n")
	if !strings.HasSuffix(out, "pre=0\nb=0\n") || st != 0 {
		t.Errorf("after `set +o posix` %q status %d, want the command to run again", out, st)
	}
}

// From a command string the same refusal ends the shell rather than giving up
// a line, which is the split interp.Runner.GiveUpTheCommandAt answers.
func TestPosixPrefixRefusalFromACommandStringEndsTheShell(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{
		"bash", "-c", "set -o posix; readonly v=1; v=3 true; echo pre=$?",
	})
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if out.String() != "" {
		t.Errorf("out %q, want nothing — the rest of the line never runs", out.String())
	}
	if !strings.Contains(errs.String(), "v: readonly variable") {
		t.Errorf("err %q, want the refusal", errs.String())
	}
}

// POSIX mode makes a refused **subscripted** prefix word cost the command the
// way a frozen name's refusal does, and a refused reference aim written as an
// assignment of its own end the shell. Each row is a transcript measured
// 2026-11-02 on GNU bash 5.3.20 under `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, `set -o posix` in front of the line: once as a `-c` string, and
// once as a script file with `echo next=$?` on the line after it. Before this,
// every subscripted row carried on to `after=0`, the aim gave up its line and
// reached `next=1`, and the frozen name's `r=2 :` ended a `-c` string at 1
// (#7343).
//
// The minimal pair is what keys it: `:`, `export` and `eval` end the shell —
// at 127 from `-c`, at 1 from a file — and `true`, `command :`, an external
// and a function give up the line instead, which from `-c` is the whole
// string at 1. The two controls are the issue's: a `for` over an aimed
// reference carries on, and the frozen name still ends the shell.
func TestPosixModeSharpensASubscriptedPrefixAndAnAim(t *testing.T) {
	t.Parallel()
	const (
		ident = "`a[1]': not a valid identifier\n"
		aim   = "`1': not a valid identifier\n"
		ro    = "r: readonly variable\n"
	)
	for _, c := range []struct {
		name, src, said string
		// What follows the complaint, and the exit status, on each route.
		cTail string
		cCode int
		fTail string
		fCode int
	}{
		{"a special builtin", `a[1]=x :; echo after=$?`, ident, "", 127, "", 1},
		{"an external", `a[1]=x /usr/bin/true; echo after=$?`, ident, "", 1, "next=1\n", 0},
		{"a function", `f(){ :; }; a[1]=x f; echo after=$?`, ident, "", 1, "next=1\n", 0},
		{"a refused aim", `typeset -n m; m=1; echo after=$?`, aim, "", 127, "", 1},
		{
			"an element through an unaimed reference", `typeset -n ref; ref[0]=foo; echo after=$?`,
			"`': not a valid identifier\n", "", 127, "", 1,
		},
		{"pair: export", `a[1]=x export y=1; echo after=$?`, ident, "", 127, "", 1},
		{"pair: eval", `a[1]=x eval :; echo after=$?`, ident, "", 127, "", 1},
		{"pair: a regular builtin", `a[1]=x true; echo after=$?`, ident, "", 1, "next=1\n", 0},
		{"pair: command in front of a special", `a[1]=x command :; echo after=$?`, ident, "", 1, "next=1\n", 0},
		{
			"pair: command in front of a special, frozen", `readonly r=1; r=2 command :; echo after=$?`,
			ro, "", 1, "next=1\n", 0,
		},
		{"the walk stops at the first", `a[1]=x c[2]=y true; echo after=$?`, ident, "", 1, "next=1\n", 0},
		{
			"control: a loop over an aimed reference", `x=0; typeset -n m=x; for m in 1; do :; done; echo after=$?`,
			aim, "after=1\n", 0, "after=1\nnext=0\n", 0,
		},
		{"control: a frozen name", `readonly r=1; r=2 :; echo after=$?`, ro, "", 127, "", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			src := "set -o posix; " + c.src
			var both bytes.Buffer
			code := driver.MainArgs(bashShell(&both, &both), []string{"bash", "-c", src})
			if want := "bash: line 1: " + c.said + c.cTail; both.String() != want || code != c.cCode {
				t.Errorf("-c: %q at %d, want %q at %d", both.String(), code, want, c.cCode)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "p.sh")
			if err := os.WriteFile(path, []byte(src+"\necho next=$?\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			both.Reset()
			code = driver.MainArgs(bashShell(&both, &both), []string{"bash", path})
			got := strings.ReplaceAll(both.String(), dir+string(filepath.Separator), "")
			if want := "p.sh: line 1: " + c.said + c.fTail; got != want || code != c.fCode {
				t.Errorf("file: %q at %d, want %q at %d", got, code, want, c.fCode)
			}
		})
	}
}
