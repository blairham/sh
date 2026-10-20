// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// A function may not be named after a special builtin here, and the refusal
// is a **syntax error while the input is read**.
//
// Measured 2026-09-15 on dash 0.5.12, over `-c`, a script file and standard
// input alike: `export() { :; }` is `Syntax error: Bad function name` at
// status 2 with nothing else in the input run. The set is this shell's
// special builtins — the fifteen POSIX marks special, plus `local` — and not
// its builtins: bash 5.3, bash 3.2, zsh 5.9 and BusyBox ash all define every
// one of them at status 0.
func TestASpecialBuiltinIsNotAFunctionName(t *testing.T) {
	for _, name := range []string{
		"break", "continue", "eval", "exec", "exit", "export", "local",
		"readonly", "return", "set", "shift", "times", "trap", "unset",
	} {
		src := name + "() { :; }\n"
		if _, err := syntax.Parse(src, dash.Dialect()); err == nil {
			t.Errorf("%s parsed, where this shell answers `Bad function name`", src)
		}
	}
}

// And a *regular* builtin is an ordinary name, which is what says the set is
// the special list rather than the builtin list. Without this row a fix that
// refused every builtin would pass the row above.
func TestARegularBuiltinIsAnOrdinaryFunctionName(t *testing.T) {
	for _, name := range []string{
		"true", "false", "read", "cd", "echo", "printf", "pwd", "test",
		"command", "wait", "umask", "type", "hash", "alias", "unalias",
		"getopts", "jobs", "kill", "ulimit",
	} {
		src := name + "() { :; }\n"
		if _, err := syntax.Parse(src, dash.Dialect()); err != nil {
			t.Errorf("%s refused, where this shell defines it: %v", src, err)
		}
	}
}

// The stage, which is the half that separates this shell from the other one
// that refuses: the refusal is made while the input is *read*, so nothing in
// front of the definition runs and a definition in a branch nothing takes is
// refused all the same. The whole rendered line, because the position is as
// easy to get wrong as the sentence.
func TestTheRefusalIsMadeWhileReading(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			"printf 'a\\n'; export() { :; }; printf 'b\\n'\n",
			"s.sh: 1: Syntax error: Bad function name\n",
		},
		{
			"if false; then export() { :; }; fi\nprintf 'after\\n'\n",
			"s.sh: 1: Syntax error: Bad function name\n",
		},
		{
			"(export() { :; })\nprintf 'after\\n'\n",
			"s.sh: 1: Syntax error: Bad function name\n",
		},
		{
			"printf 'a\\n'\nset() { :; }\n",
			"s.sh: 2: Syntax error: Bad function name\n",
		},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		d := dash.Diagnostics().ForScript()
		if got := d.ParseDiagnostic("s.sh", "", err, tc.src); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
