// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"
)

// A function may not be named after a special builtin here either, and the
// stage is this shell's own: the definition **parses**, and the complaint
// comes when it is reached.
//
// Measured 2026-09-15 on ksh93u+ 2012-08-01, over `-c`, a script file and
// standard input alike: `export() { :; }` is `export: invalid function name`
// and the script stops at 1. bash 5.3, bash 3.2, zsh 5.9 and BusyBox ash all
// define it at status 0.
func TestASpecialBuiltinIsNotAFunctionName(t *testing.T) {
	for _, name := range []string{
		"alias", "break", "continue", "enum", "eval", "exec", "exit",
		"export", "login", "newgrp", "readonly", "return", "set", "shift",
		"trap", "typeset", "unalias", "unset",
	} {
		src := name + "() { :; }\nprintf 'after\\n'\n"
		out, st := answersRun(t, src)
		want := "sh: " + name + ": invalid function name\n"
		if out != want || st != 1 {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at 1", src, out, st, want)
		}
	}
}

// The `function` spelling reaches the same set, which is what says the check
// is about the name rather than about one production.
func TestTheKeywordSpellingRefusesTheSameNames(t *testing.T) {
	for _, name := range []string{"export", "set", "typeset", "unalias"} {
		src := "function " + name + " { :; }\necho after\n"
		out, st := answersRun(t, src)
		want := "sh: " + name + ": invalid function name\n"
		if out != want || st != 1 {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at 1", src, out, st, want)
		}
	}
}

// And a *regular* builtin is an ordinary name. Without this row a fix that
// refused every builtin would pass the two above.
//
// The names that are **aliases** here rather than builtins are not rows, and
// there are more of them than there were: `times`, `hash`, `integer`, `float`,
// `compound`, `nameref`, `functions` and `source` all reach this shell through
// the preset alias table. ksh93 takes every one of them as a function name —
// measured 2026-09-18, `nameref() { :; }` and `float() { :; }` from a script
// file under `env -i` are a silent 0 there, and so is `alias zz='typeset -n';
// zz() { :; }` — because **that shell does not expand an alias for a word a
// `(` follows**. This one does, so `nameref() { :; }` becomes
// `typeset -n() { :; }` and is a syntax error, which is a parser gap of its
// own and not this test's subject. It shows only for an alias whose value is a
// declaration word: `alias zz=echo; zz() { :; }` defines `zz` here exactly as
// it does there, because `echo() { :; }` is a legal definition either way.
func TestARegularBuiltinIsAnOrdinaryFunctionName(t *testing.T) {
	for _, name := range []string{
		"true", "false", "read", "cd", "pwd",
		"command", "wait", "umask", "getopts", "jobs", "kill", "ulimit",
		"builtin", "whence", "disown",
		"echo", "test",
	} {
		// Defined and not called: a body calling the name it shadows is a
		// recursion and not a definition test.
		src := name + "() { :; }\nprintf 'after\\n'\n"
		out, st := answersRun(t, src)
		if out != "after\n" || st != 0 {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at 0", src, out, st, "after\n")
		}
	}
}

// The stage, which is what separates this shell from the other one that
// refuses. The complaint is raised where the definition is **reached**, so a
// command in front of it runs, a definition in a branch nothing takes is
// never refused at all, and one inside a subshell ends the subshell alone.
func TestTheRefusalIsMadeWhereTheDefinitionRuns(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{
			"printf 'a\\n'; export() { :; }; printf 'b\\n'\n",
			"a\nsh: export: invalid function name\n", 1,
		},
		{
			"if false; then export() { :; }; fi; printf 'after\\n'\n",
			"after\n", 0,
		},
		{
			"(export() { :; }); printf 'after\\n'\n",
			"sh: export: invalid function name\nafter\n", 0,
		},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at %d",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}
