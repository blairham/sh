// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A command substitution whose body will not parse, written in a
// **here-document body** (#3318), and the number such a failure carries where
// it is not the script's own line being read (#3319).
//
// Measured 2026-09-17 from a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C
// <shell> case.sh` with stdin from /dev/null; BusyBox v1.37.0 in the
// digest-pinned Alpine image internal/oracle reaches, under `--init`:
//
//	printf 'start\n'
//	cat <<END
//	before $(echo hi; for) after
//	END
//	printf 'after st=%s\n' "$?"
//
//	                     output                 status  carried on
//	bash 5.3.20          start, after st=1      0       yes
//	zsh 5.9.2            start                  1       no
//	ksh93u+ 2012-08-01   start, after st=3      0       yes
//	dash 0.5.12          start                  2       no
//	BusyBox ash 1.37.0   start                  2       no
//
// Nobody runs `cat`, which is the half every boundary already agreed on.
// **Whether the script continued is the assertion**, for the reason
// TestASubstitutionThatWillNotParseEndsTheScriptFromInsideASubshell gives: a
// shell that reported the failure and carried on has the same status as one
// that stopped, so the status alone cannot tell them apart.
//
// The two columns that carry on leave different numbers, and that is the
// second axis rather than a detail of this one: bash's 1 is what every fatal
// error carries there and ksh93's 3 is the refusal's own syntax status
// surviving. A plain failed redirection is 1 in both, so the numbers are told
// apart by measurement rather than by arithmetic.
func TestASubstitutionRefusedInAHereDocumentBodyCostsTheCommandAndSometimesTheScript(t *testing.T) {
	const src = "printf 'start\\n'\n" +
		"cat <<END\nbefore $(echo hi; for) after\nEND\n" +
		"printf 'after st=%s\\n' \"$?\"\n"
	for _, c := range []struct {
		preset string
		out    string
		status int
	}{
		{"bash", "start\nafter st=1\n", 0},
		{"ksh", "start\nafter st=3\n", 0},
		{"zsh", "start\n", 1},
		{"dash", "start\n", 2},
		{"ash", "start\n", 2},
		{"posix", "start\n", 2},
	} {
		t.Run(c.preset, func(t *testing.T) {
			out, errs, st := runWithPath(t, presets[c.preset], src)
			if out != c.out || st != c.status {
				t.Errorf("wrote %q at %d, want %q at %d", out, st, c.out, c.status)
			}
			if errs == "" {
				t.Errorf("nothing was reported; the failure has to be visible as well as costly")
			}
			if strings.Contains(out, "before") {
				t.Errorf("the command ran with the body anyway: %q", out)
			}
		})
	}
}

// The **redirection target** is the control, and it is a different split
// rather than the other end of the same one: the same substitution written as
// `cat < "$(echo hi; for)"` ends the script in bash 5.3.20, zsh 5.9.2, dash
// and BusyBox ash and is carried on only by ksh93u+, measured the same way.
//
// So an axis asked of redirections as a class would take one of the two rows
// and lose the other, which is why the body is asked on its own.
func TestASubstitutionRefusedInARedirectionTargetIsADifferentSplit(t *testing.T) {
	const src = "printf 'start\\n'\ncat < \"$(echo hi; for)\"\nprintf 'after st=%s\\n' \"$?\"\n"
	for _, c := range []struct {
		preset string
		status int
	}{{"bash", 2}, {"zsh", 1}, {"dash", 2}, {"ash", 2}, {"posix", 2}} {
		out, _, st := runWithPath(t, presets[c.preset], src)
		if out != "start\n" || st != c.status {
			t.Errorf("%s wrote %q at %d, want %q at %d", c.preset, out, st, "start\n", c.status)
		}
	}
	// ksh93u+ carries on from a target as well — `after st=3`, measured the
	// same way — and this dialect does not. That row is right in four columns
	// of five and is a gap of its own rather than this axis's other end: the
	// target boundary keeps the stop standing in every dialect, so moving it
	// here would need the question asked a second time. Recorded as it
	// stands, so the day it moves says so.
	out, _, st := runWithPath(t, presets["ksh"], src)
	if out != "start\n" || st != 3 {
		t.Errorf("ksh wrote %q at %d, want %q at %d — the gap is recorded rather than asserted away", out, st, "start\n", 3)
	}
}

// The status a substitution refusal ends the script with, where the failing
// text was **borrowed** — a file `.` read, or an `eval` argument (#3319).
//
// Measured 2026-09-17, the sourced file and the `eval` argument each holding
// `printf 'inner-start\n'` and then `v=$(echo hi; for)`:
//
//	case                               bash 5.3.20  dash 0.5.12
//	the substitution at the top level   2            2
//	inside a file `.` read              1            2
//	inside an `eval` argument           1            2
//
// The top-level row is the control and is the test below this one: it says
// the two shells agree about the status of the failure itself, so bash's 1 is
// what the borrowed route does to it. zsh and ksh93 are not rows here because
// both **catch** the failure at the borrowed text and carry the script on,
// which Semantics.FatalErrorEndsBorrowedTextOnly already answers — their rows
// assert that, so a change that made them stop would be caught.
func TestTheStatusABorrowedSubstitutionRefusalEndsTheScriptWith(t *testing.T) {
	for _, route := range []struct{ name, src string }{
		{"an eval argument", "printf 'start\\n'\neval 'printf \"inner-start\\n\"\nv=$(echo hi; for)'\nprintf 'never\\n'\n"},
		{"a file the shell read", "printf 'start\\n'\n. ./inner.sh\nprintf 'never\\n'\n"},
	} {
		t.Run(route.name, func(t *testing.T) {
			for _, c := range []struct {
				preset    string
				status    int
				carriesOn bool
			}{
				{preset: "bash", status: 1},
				{preset: "dash", status: 2},
				{preset: "ash", status: 2},
				{preset: "posix", status: 2},
				// The two that catch it where it was borrowed.
				{preset: "zsh", status: 0, carriesOn: true},
				{preset: "ksh", status: 0, carriesOn: true},
			} {
				out, _, st := runWithPath(t, presets[c.preset], route.src)
				if st != c.status {
					t.Errorf("%s ended at %d, want %d", c.preset, st, c.status)
				}
				if strings.Contains(out, "never") != c.carriesOn {
					t.Errorf("%s wrote %q, carrying on = %v, want %v", c.preset, out, !c.carriesOn, c.carriesOn)
				}
			}
		})
	}
}

// And the row the status axis must not move: the same substitution on the
// script's own line, where every column reports its syntax status.
func TestASubstitutionRefusalOnTheScriptsOwnLineIsTheSyntaxStatusEverywhere(t *testing.T) {
	const src = "printf 'start\\n'\nv=$(echo hi; for)\nprintf 'never\\n'\n"
	for _, c := range []struct {
		preset string
		status int
	}{{"bash", 2}, {"zsh", 1}, {"ksh", 3}, {"dash", 2}, {"ash", 2}, {"posix", 2}} {
		out, _, st := runWithPath(t, presets[c.preset], src)
		if out != "start\n" || st != c.status {
			t.Errorf("%s wrote %q at %d, want %q at %d", c.preset, out, st, "start\n", c.status)
		}
	}
}

// runWithPath is splitRun with a directory the snippets can source from and a
// PATH the external command in them resolves on.
//
// A here-document body is expanded in the process the redirection is for, and
// the boundary these rows are about is reached only for a command this shell
// does not run itself — so `cat` has to be a program rather than a builtin,
// and the give-up happens before it is started either way.
func runWithPath(t *testing.T, p dialecttest.Preset, src string) (out, errs string, status int) {
	t.Helper()
	dir := t.TempDir()
	const inner = "printf 'inner-start\\n'\nv=$(echo hi; for)\n"
	if err := os.WriteFile(filepath.Join(dir, "inner.sh"), []byte(inner), 0o600); err != nil {
		t.Fatalf("write inner.sh: %v", err)
	}
	f := p.Parse(t, src)
	var o, e strings.Builder
	r := p.Runner(dialecttest.Base{
		Stdout: &o, Stderr: &e, Dir: dir,
		Env: []string{"PATH=/usr/bin:/bin"},
	})
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return o.String(), e.String(), st
}
