// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// The synopsis and the usage line are one string, so the derivation has to
// hold for every builtin that has both. Measured 2026-09-05 on bash 5.3.15:
// the first line of `help NAME` is NAME's usage line with the `usage: ` taken
// out, without exception. A hand-written entry that broke the relation would
// be a builtin whose `--help` and whose bad option describe it differently.
func TestEveryUsageLineIsItsBuiltinsSynopsis(t *testing.T) {
	d := bash.Diagnostics()
	if len(d.BuiltinUsage) == 0 {
		t.Fatal("no usage lines at all")
	}
	for name, usage := range d.BuiltinUsage {
		want := strings.Replace(usage, "usage: ", "", 1)
		if want == usage {
			t.Errorf("%s: usage line %q has no `usage: ` in it", name, usage)
			continue
		}
		if got := d.BuiltinHelp[name]; got != want {
			t.Errorf("%s: help %q, want %q", name, got, want)
		}
	}
}

// The builtins with no options to give a bad one to have no usage line and
// still answer `--help`, so they are written out rather than derived. This
// pins that they did not go missing, and that the status is bash's.
func TestTheBuiltinsWithNoUsageLineStillAnswerHelp(t *testing.T) {
	d := bash.Diagnostics()
	if d.BuiltinHelpStatus != 2 {
		t.Errorf("BuiltinHelpStatus = %d, want 2", d.BuiltinHelpStatus)
	}
	for name, want := range map[string]string{
		"bg":     "bg: bg [job_spec ...]",
		"fg":     "fg: fg [job_spec]",
		"exit":   "exit: exit [n]",
		"shift":  "shift: shift [n]",
		"local":  "local: local [option] name[=value] ...",
		"umask":  "umask: umask [-p] [-S] [mode]",
		"printf": "printf: printf [-v var] format [arguments]",
	} {
		if got := d.BuiltinHelp[name]; got != want {
			t.Errorf("%s: help %q, want %q", name, got, want)
		}
	}
	// And the six that answer nothing, measured: they read the word as an
	// ordinary operand rather than as an option.
	for _, name := range []string{":", "true", "false", "test", "[", "echo"} {
		if got := d.BuiltinHelp[name]; got != "" {
			t.Errorf("%s: help %q, want none", name, got)
		}
	}
}

// Two Runners must not share a map a third could edit.
func TestTheAnswersAreFreshEachTime(t *testing.T) {
	first := bash.Diagnostics()
	first.BuiltinHelp["alias"] = "tampered"
	first.BuiltinUsage["alias"] = "tampered"
	second := bash.Diagnostics()
	if second.BuiltinHelp["alias"] == "tampered" || second.BuiltinUsage["alias"] == "tampered" {
		t.Error("one caller's edit reached the next caller's answers")
	}
}

// `/usr/bin/alias --help` is a builtin call on macOS, which is what makes
// this the most-run one there is. End to end, through the dialect.
func TestABuiltinAnswersHelpOnStandardOutput(t *testing.T) {
	out, st := answersRun(t, `builtin alias --help`)
	if !strings.HasPrefix(out, "alias: alias [-p]") {
		t.Errorf("said %q, want the synopsis", out)
	}
	if st != 2 {
		t.Errorf("status %d, want 2", st)
	}
}

// The usage line under a bad option, for the six that had the complaint and
// nothing under it (#825).
func TestABadOptionIsFollowedByTheUsageLine(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`builtin alias -Q`, "alias: usage: alias [-p]"},
		{`builtin unalias -Q`, "unalias: usage: unalias [-a]"},
		{`builtin cd -Q`, "cd: usage: cd [-L|[-P [-e]]]"},
		{`builtin fc -Q`, "fc: usage: fc [-e ename]"},
		{`builtin hash -Q`, "hash: usage: hash [-lr]"},
		{`builtin ulimit -Q`, "ulimit: usage: ulimit [-SH"},
		// And the one that named the word itself rather than letting the
		// dialect's rule name it.
		{`umask --version`, "umask: --: invalid option"},
	} {
		out, _ := answersRun(t, c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: said %q, want %q in it", c.src, out, c.want)
		}
	}
}
