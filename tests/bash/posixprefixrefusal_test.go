// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
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
