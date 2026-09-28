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

// The `function` keyword with **nothing left for it to take** is a command
// here: an anonymous function with no body, which runs nothing and leaves 0.
// Six other columns refuse it — see syntax.Dialect.BareFunctionKeyword for the
// panel.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), from script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and standard input
// on the null device.
//
// **Every row here is on a route where the keyword has nothing to take**, and
// that is the whole of what #5078 changed about this file. The keyword takes
// the next command in its list as a *body* — see
// TestTheKeywordTakesTheNextCommandAsItsBody — so a row written as
// `function; printf …` is not this construct at all: the `printf` is the body,
// and the row grades the call rather than the bare form. Six rows here were
// written that way and all six passed, because a body that prints what the
// script would have printed anyway is invisible in the output.
//
// **The status claim those rows carried was false.** This file used to say the
// status rows were "the discriminating half: `false; function` leaves 1 behind
// where `false; function { }` leaves 0". It leaves 1 in *that spelling*
// because the `printf` reading `$?` is the body and `$?` is still the `false`'s
// — both readings print 1, so the probe cannot fail. Asked on a route that
// genuinely reaches the bare form, the reference answers **0**, exactly as the
// empty brace body does. The rows below are those routes.
func TestTheKeywordWithNothingToTakeRunsNothingHere(t *testing.T) {
	if !zsh.Dialect().BareFunctionKeyword {
		t.Fatal("BareFunctionKeyword is off, so the keyword needs a name here")
	}
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// A closing brace, a `)`, a `done`, an arm terminator, an operator
		// and the end of input: six ways for the keyword to have nothing
		// under it, and 0 on every one.
		{"a closing brace is not a command", `false; { function }; printf "st=%d" $?`, "st=0"},
		{"nor is a subshell's parenthesis", `false; ( function ); printf "st=%d" $?`, "st=0"},
		{"nor is a loop's `done`", `false; for i in 1; do function; done; printf "st=%d" $?`, "st=0"},
		{"nor is an arm terminator", `false; case x in x) function ;; esac; printf "st=%d" $?`, "st=0"},
		{"an operator leaves it nothing to take", `false; function && printf "st=%d" $?`, "st=0"},
		{"and so does a pipe", `false; function | cat; printf "st=%d" $?`, "st=0"},
		{"the end of the input leaves it nothing", `printf "[%s]" "$(false; function)"`, "[]"},

		// The empty brace group answers the same 0, which is what says the
		// status is not what tells the two apart. What does is the
		// redirection — see the null-command rows below.
		{"a written empty body answers the same", `false; function { }; printf "st=%d" $?`, "st=0"},
		{"and so does the other spelling of one", `false; () { }; printf "st=%d" $?`, "st=0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZshOnPath(t, dir, tc.src); out != tc.want || st != 0 {
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
