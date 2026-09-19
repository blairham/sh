// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// runZshOnPath is runZshPrelude with the machine's own command directories on
// PATH, which the null command needs: the default is `cat`, and a row that
// could not find it would report an absent null command as a working one.
func runZshOnPath(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": "/usr/bin:/bin"},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// The `function` keyword standing entirely alone is a command here: an
// anonymous function with no body, which runs nothing and leaves the status
// it found. Six other columns refuse it — see
// syntax.Dialect.BareFunctionKeyword for the panel.
//
// Measured 2026-09-19 on zsh 5.9.2, from a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null device.
//
// The status rows are what say the body is absent rather than empty, and they
// are the discriminating half: `false; function` leaves 1 behind where
// `false; function { }` and `false; () { }` both leave 0. A reading that gave
// the keyword an empty brace group would pass every parse row here and answer
// the wrong status on all three.
func TestTheKeywordAloneRunsNothingHere(t *testing.T) {
	if !zsh.Dialect().BareFunctionKeyword {
		t.Fatal("BareFunctionKeyword is off, so the keyword needs a name here")
	}
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"it runs and is silent", `function; printf "st=%d" $?`, "st=0"},
		{"and leaves the status it found", `false; function; printf "st=%d" $?`, "st=1"},
		{"a newline behind it says the same", "false\nfunction\n" + `printf "st=%d" $?`, "st=1"},
		{"before the end of the input", "false\nfunction\n" + `printf "st=%d" $?`, "st=1"},
		{"it defines nothing", `function; printf "n=%d" ${#functions}`, "n=0"},

		// An empty brace group is a different command: a call whose body ran
		// to the end, which is a success.
		{"a written empty body is a call", `false; function { }; printf "st=%d" $?`, "st=0"},
		{"and so is the other spelling of one", `false; () { }; printf "st=%d" $?`, "st=0"},

		// Where it may stand. Each of these is a place a command ends.
		{"before a semicolon", `function; printf a`, "a"},
		{"inside a brace group", `{ function }; printf a`, "a"},
		{"inside a subshell", `( function ); printf a`, "a"},
		{"as an and-or's right-hand side", `true && function; printf "st=%d" $?`, "st=0"},
		{"and its left one", `false || function; printf "st=%d" $?`, "st=1"},
		// Either side of a pipe, and what a bare keyword on the reading
		// side does with what was written to it is the same nothing: the
		// output is dropped rather than passed on.
		{"on either side of a pipe", `printf a | function; function | printf b`, "b"},
		{"in a case arm", `case x in x) function ;; esac; printf a`, "a"},
		{"in a loop body", `for i in 1; do function; done; printf a`, "a"},
		{"and inside a substitution", `printf "[%s]" "$(function)"`, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A redirection written after the bare keyword is a redirection with no
// command, null command and all — which is what says the construct runs an
// empty command rather than a function with an empty body. Measured the same
// day: `echo hi | function > f` puts `hi` in the file, because the default
// null command is `cat`.
func TestARedirectionAfterTheBareKeywordIsTheNullCommand(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"the file is written", `printf hi | function > f; printf "[%s]" "$(cat f)"`, "[hi]"},
		{"and the status is the null command's", `false; function > g; printf "st=%d" $?`, "st=0"},
		// A redirection that will not open is reported against the name a
		// nameless function is given rather than against the line it stands
		// on, which is this shell saying the frame is pushed before the
		// files are opened: `(anon): no such file or directory: nosuch`,
		// measured the same day.
		{
			"a redirection that will not open blames the nameless frame",
			`function < nosuch; printf "st=%d" $?`, "(anon): no such file or directory: nosuch\nst=1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZshOnPath(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// And where it may not stand. A background operator ends the line here in any
// of its spellings, and every reserved word but `}` is read as the name this
// form does not have — so all of these are refusals, exactly as they are in
// the columns without the construct at all.
func TestTheBareKeywordDoesNotReachTheseHere(t *testing.T) {
	for _, src := range []string{
		"function & printf a\n",
		"function &\n",
		"function &!\n",
		"if true; then function fi\n",
		"while false; do function done\n",
		"case x in x) function esac\n",
		// Nor is `}` a name: the one reserved word that ends the form is not
		// a word this form can be given.
		"function } { printf b }\n",
	} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err == nil {
			t.Errorf("%q parsed, want a refusal", src)
		}
	}
}
