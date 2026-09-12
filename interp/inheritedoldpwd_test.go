// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What an inherited OLDPWD becomes, before any `cd` has run.
//
// #1490 read this as a branch inside `cd -`, because that is where it shows:
// one shell says `OLDPWD not set` for a value that is plainly set, and
// another says nothing at all. It is neither. The value is judged once, at
// startup, and `cd -` then reads whatever survived — which is why a fix
// inside `cd` would have made an inherited value and an assigned one disagree
// in the shell that answers TakenIfADirectory, and matched neither.
//
// The probe is the variable rather than the `cd`, deliberately: reading it
// through `cd -` cannot tell "the value was dropped" from "`cd` refused it",
// which is the confusion the issue was written out of.
func TestAnInheritedOldpwdIsJudgedAtStartup(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "d")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "gone")

	for _, c := range []struct {
		name    string
		policy  InheritedOldpwdPolicy
		env     string
		want    string
		wantDir bool
	}{
		{"taken: a directory is kept", InheritedOldpwdTaken, sub, sub, false},
		{"taken: a path that is not there is kept too", InheritedOldpwdTaken, missing, missing, false},
		{"taken: nothing inherited stays nothing", InheritedOldpwdTaken, "", "UNSET", false},

		{"if a directory: a directory is kept", InheritedOldpwdTakenIfADirectory, sub, sub, false},
		{"if a directory: a path that is not there goes", InheritedOldpwdTakenIfADirectory, missing, "UNSET", false},
		{"if a directory: a plain file goes", InheritedOldpwdTakenIfADirectory, file, "UNSET", false},
		{"if a directory: the empty value goes", InheritedOldpwdTakenIfADirectory, "=", "UNSET", false},
		{"if a directory: nothing inherited stays nothing", InheritedOldpwdTakenIfADirectory, "", "UNSET", false},

		{"ignored: a usable value is still not read", InheritedOldpwdIgnored, sub, "", true},
		{"ignored: an unusable one is not read either", InheritedOldpwdIgnored, missing, "", true},
		{"ignored: nothing inherited is still the startup directory", InheritedOldpwdIgnored, "", "", true},

		{"unanswered: read as any other name is", InheritedOldpwdUnspecified, missing, missing, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := &strings.Builder{}
			sem := PosixSemantics()
			sem.InheritedOldpwd = c.policy
			var env []string
			switch c.env {
			case "":
			case "=":
				env = []string{"OLDPWD="}
			default:
				env = []string{"OLDPWD=" + c.env}
			}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Env: env, Stdout: out, Stderr: &strings.Builder{},
			})
			runCd(t, r, `echo "[${OLDPWD-UNSET}]"`)
			want := c.want
			if c.wantDir {
				want = dir
			}
			if got := strings.TrimSpace(out.String()); got != "["+want+"]" {
				t.Errorf("OLDPWD read back as %s, want [%s]", got, want)
			}
		})
	}
}

// The judgement happens once, and a `cd` of the shell's own outlives it.
//
// A Runner runs more than one chunk — a dialect's prelude and then the
// script, and a line at a time for a person typing — so a startup fixup that
// re-ran per chunk would overwrite the OLDPWD a `cd` had just recorded. That
// is the whole reason this is a flag rather than "is the name set".
func TestTheStartupJudgementDoesNotOutliveTheFirstCd(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "d")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, policy := range []InheritedOldpwdPolicy{
		InheritedOldpwdTaken, InheritedOldpwdTakenIfADirectory, InheritedOldpwdIgnored,
	} {
		t.Run(policy.String(), func(t *testing.T) {
			out := &strings.Builder{}
			sem := PosixSemantics()
			sem.InheritedOldpwd = policy
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Env:    []string{"OLDPWD=" + filepath.Join(dir, "gone")},
				Stdout: out, Stderr: &strings.Builder{},
			})
			runCd(t, r, "cd "+sub)
			// A second chunk, which is the shape that would have re-run it.
			runCd(t, r, `echo "[${OLDPWD-UNSET}]"`)
			if got := strings.TrimSpace(out.String()); got != "["+dir+"]" {
				t.Errorf("OLDPWD read back as %s, want the directory the `cd` left, [%s]", got, dir)
			}
		})
	}
}
