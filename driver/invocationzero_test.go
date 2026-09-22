// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// `$0` can arrive in the environment — #4159.
//
// bash alone, through `BASH_ARGV0`: the same parameter a running shell assigns
// to rename itself, read once on the way in. It stands exactly where the
// shell's own name would, diagnostics included, and loses to either route that
// names a `$0` of its own.
func TestAnInheritedNameStandsWhereTheShellsOwnWould(t *testing.T) {
	sh := shell()
	sh.Semantics.DollarZeroFromEnvironment = "SH_ZERO"
	sh.Env = []string{"SH_ZERO=renamed"}
	out, _, _ := runArgs(t, sh, "testsh", "-c", `echo "[$0]"`)
	if out != "[renamed]\n" {
		t.Errorf("stdout = %q, want the inherited name", out)
	}

	// And the diagnostic goes with it, which is what says it stands in the
	// shell's own place rather than only in the parameter.
	sh.Diagnostics.Location = interp.LocationLineWord
	sh.Diagnostics.NotFound = "%s: command not found"
	_, errs, _ := runArgs(t, sh, "testsh", "-c", "nosuchcommand")
	if !strings.HasPrefix(errs, "renamed: line 1:") {
		t.Errorf("stderr = %q, want the inherited name in front of it", errs)
	}
}

// A route that names its own `$0` wins, which is the half that keeps this from
// being a rename of the shell: measured, `bash -c 'echo $0' operand` is
// `operand` and `bash ./s.sh` is `./s.sh`, both with the variable set.
func TestARouteThatNamesItsOwnZeroWins(t *testing.T) {
	script := writeScript(t, `echo "[$0]"`)
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"a name operand", []string{"testsh", "-c", `echo "[$0]"`, "operand"}, "[operand]\n"},
		{"a script operand", []string{"testsh", script}, "[" + script + "]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			sh.Semantics.DollarZeroFromEnvironment = "SH_ZERO"
			sh.Env = []string{"SH_ZERO=renamed"}
			if out, _, _ := runArgs(t, sh, tc.argv...); out != tc.want {
				t.Errorf("stdout = %q, want %q", out, tc.want)
			}
		})
	}
}

// And it is *consumed*: the variable is not in the environment the shell hands
// its children, measured with `env` from inside one. A rename that traveled
// would rename every shell underneath it too.
func TestAnInheritedNameDoesNotReachAChild(t *testing.T) {
	sh := shell()
	sh.Semantics.DollarZeroFromEnvironment = "SH_ZERO"
	sh.Env = []string{"SH_ZERO=renamed", "PATH=/usr/bin:/bin"}
	out, _, _ := runArgs(t, sh, "testsh", "-c",
		`/usr/bin/env | grep '^SH_ZERO=' || echo "(none)"`)
	if out != "(none)\n" {
		t.Errorf("stdout = %q, want the variable consumed", out)
	}
}

// An option word at an invocation that takes an argument and was given none is
// refused in the dialect's words. Four spellings across the panel, and this
// front end had dash's for everybody: `-c: option requires an argument` in
// bash, `string expected after -c` in zsh, `-c requires argument` in ksh93.
func TestAMissingOptionArgumentIsRefusedInTheDialectsWords(t *testing.T) {
	for _, tc := range []struct {
		name    string
		wording string
		want    string
	}{
		{"the dialect's own", "%[1]s: option requires an argument", "testsh: -c: option requires an argument\n"},
		{"one that names the option last", "string expected after %[1]s", "testsh: string expected after -c\n"},
		{"a preset that names none", "", "testsh: -c requires an argument\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			sh.Diagnostics.InvocationMissingOptionArgument = tc.wording
			out, errs, code := runArgs(t, sh, "testsh", "-c")
			if code != 2 || out != "" {
				t.Errorf("got %q status %d, want a usage refusal", out, code)
			}
			if errs != tc.want {
				t.Errorf("stderr = %q, want %q", errs, tc.want)
			}
		})
	}
}
