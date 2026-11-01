// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestAnArrayPrefixToAFixedScalarSlotIsRefused pins that an array value
// written as a command's **assignment prefix** to a slot of a fixed scalar
// kind is refused exactly as a prefix to a frozen name is: fatal in front of
// a builtin or a function, and in front of an external or `command` the
// command is not run, the status is 1 and the script carries on. The
// controls — an ordinary name, a container, a tie's array half and a private
// array — take the literal, which is what keeps a shell refusing every array
// prefix from passing.
//
// Every row measured 2026-10-15 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`
// from a script file under `env -i PATH=/usr/bin:/bin`), output compared byte
// for byte (#6485).
func TestAnArrayPrefixToAFixedScalarSlotIsRefused(t *testing.T) {
	const home = "zsh:1: HOME: attempt to assign array value to non-array\n"
	const tie = "typeset -T FOO=a:b foo; "
	const private = "zmodload zsh/param/private; "
	for _, tc := range []struct{ name, src, want string }{
		// Fatal: a builtin, a function, `builtin`, `exec`.
		{"a builtin", "HOME=(x) true; echo rc=$?", home},
		{"after a failure", "false; HOME=(x) true; echo rc=$?", home},
		{"a function", "f() { echo in; }; HOME=(x) f; echo rc=$?", home},
		{"builtin", "HOME=(x) builtin echo ran; echo rc=$?", home},
		{"exec", "HOME=(x) exec /bin/echo ran; echo rc=$?", home},
		{"an empty literal", "HOME=() true; echo rc=$?", home},
		{"an appended literal", "HOME+=(x) true; echo rc=$?", home},
		{"IFS", "IFS=(x) true; echo rc=$?", "zsh:1: IFS: attempt to assign array value to non-array\n"},
		{"SECONDS", "SECONDS=(x) true; echo rc=$?", "zsh:1: SECONDS: attempt to assign array value to non-array\n"},
		{"in a function", "f() { HOME=(x) true; echo in; }; f; echo rc=$?", "f: HOME: attempt to assign array value to non-array\n"},
		{"a script tie's scalar half", tie + "FOO=(x) true; echo rc=$? $FOO", "zsh:1: FOO: attempt to assign array value to non-array\n"},
		{"a script tie before eval", tie + "FOO=(x) eval 'echo in'; echo after", "zsh:1: FOO: attempt to assign array value to non-array\n"},
		{"a private", private + "f() { private v; v=(x) true; echo rc=$?; }; f; echo after", "f: v: attempt to assign array value to non-array\n"},

		// Not fatal: the command is not run and the status is 1.
		{"an external", "HOME=(x) /bin/echo ran; echo rc=$?", home + "rc=1\n"},
		{"command", "HOME=(x) command /bin/echo ran; echo rc=$?", home + "rc=1\n"},
		{"a name nothing runs", "HOME=(x) nosuchcmd; echo rc=$?", home + "rc=1\n"},
		{"a tie before an external", tie + "FOO=(x) /bin/echo ran; echo rc=$? $FOO", "zsh:1: FOO: attempt to assign array value to non-array\nrc=1 a:b\n"},
		{"a private before an external", private + "f() { private v; v=(x) /bin/echo ran; echo rc=$?; }; f; echo after", "f: v: attempt to assign array value to non-array\nrc=1\nafter\n"},
		{"twice", "HOME=(x) /bin/echo ran; echo rc=$?; HOME=(y) /bin/echo again", home + "rc=1\n" + home},

		// The first refused entry in written order decides the sentence.
		{"array then frozen", "readonly r=1; HOME=(x) r=2 true", home},
		{"frozen then array", "readonly r=1; r=2 HOME=(x) true", "zsh:1: read-only variable: r\n"},
		{"two arrays", "HOME=(x) IFS=(y) /bin/echo ran; echo rc=$?", home + "rc=1\n"},
		{"a plain prefix beside it", "A=1 HOME=(x) /bin/echo ran; echo rc=$?", home + "rc=1\n"},
		// The walk stops at the refused entry: a value behind it is never
		// expanded, and one in front of it is.
		{"a value behind it", "HOME=(x) B=$(echo s >&2) true", home},
		{"a value in front of it", "B=$(echo s >&2) HOME=(x) true", "s\n" + home},
		{"a value behind it, before a function", "f() { :; }; HOME=(x) B=$(echo s >&2) f", home},

		// The controls.
		{"an ordinary name", "x=s; x=(a) true; echo rc=$? $x", "rc=0 s\n"},
		{"a container", "path=(a b) true; echo rc=$?", "rc=0\n"},
		{"a tie's array half", tie + "foo=(x) /bin/echo ran; echo rc=$? $FOO", "ran\nrc=0 a:b\n"},
		{"a private array", private + "f() { private -a v; v=(x) true; echo rc=$? ${(t)v}; }; f", "rc=0 array-local-hide-special\n"},
		{"a plain value", "HOME=/h /bin/echo ran; echo rc=$?", "ran\nrc=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runZshOnPath(t, t.TempDir(), tc.src)
			if got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestARefusedWholeArrayStoreLeavesZeroFromACommandString pins that a
// refused whole-array store through `set -A`, `set +A` or `read -A` ends the
// shell at 0 from `-c` and at 1 from a script file, where the same refusal
// through `read`'s scalar store or a bare literal ends at 1 by both routes.
//
// Measured 2026-10-15 against zsh 5.9.2, `env -i PATH=/usr/bin:/bin`, `-f`
// (#6486). The sentence is the same on both routes and the `echo` after it
// never runs, so the status is the only thing that moves.
func TestARefusedWholeArrayStoreLeavesZeroFromACommandString(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src     string
		command int
	}{
		{"typeset -r v=1; set -A v x", 0},
		{"typeset -r v=1; set +A v x", 0},
		{"typeset -r v=1; set -A v", 0},
		{"typeset -ra a=(1); set -A a x", 0},
		{"typeset -r v=1; read -A v <<< x", 0},
		{"typeset -ra a=(1); read -A a <<< x", 0},
		{"set -A HOME x", 0},
		{"false; set -A HOME x", 0},
		{"set +A HOME x", 0},
		{"read -A HOME <<< x", 0},
		{"typeset -T FOO=a:b foo; set -A FOO x", 0},
		{"typeset -T FOO=a:b foo; read -A FOO <<< x", 0},
		{"f() { set -A HOME x; }; f", 0},
		// The raisers, measured over this refusal: `||` leaves the 0 and
		// `&&` and a loop raise it.
		{"set -A HOME x || echo no", 0},
		{"set -A HOME x && echo yes", 1},
		{"for i in 1; do set -A HOME x; done", 1},
		// The controls, 1 by both routes.
		{"typeset -r v=1; read v <<< x", 1},
		{"typeset -r v=1; v=(x)", 1},
		{"HOME=(x)", 1},
		{": ${(A)HOME::=x}", 1},
		{"HOME=(x) true", 1},
	} {
		for _, route := range []struct {
			route interp.Route
			want  int
		}{
			{interp.RouteCommandString, tc.command},
			{interp.RouteScriptFile, 1},
		} {
			out, st := runZshRoute(t, t.TempDir(), tc.src+"; echo after", route.route)
			if st != route.want {
				t.Errorf("%s from %v = status %d, want %d", tc.src, route.route, st, route.want)
			}
			if out == "" || out[len(out)-6:] == "after\n" {
				t.Errorf("%s from %v = %q: want a refusal and nothing after it", tc.src, route.route, out)
			}
		}
	}
}

// TestARefusedWholeArrayStoreInASubshellLeavesZero is the raiser that is not
// the route: a subshell's own status is 0 from a script file as well, so the
// file's 1 above is the top level's and not the refusal's.
func TestARefusedWholeArrayStoreInASubshellLeavesZero(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"( set -A HOME x; echo x ); echo next=$?",
		"( read -A HOME <<< x; echo x ); echo next=$?",
		"( typeset -r v=1; set -A v x ); echo next=$?",
	} {
		got, _ := runZshOnPath(t, t.TempDir(), src)
		if want := "next=0\n"; len(got) < len(want) || got[len(got)-len(want):] != want {
			t.Errorf("%s = %q, want it to end %q", src, got, want)
		}
	}
}
