// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// declarationReservedWords is every declaration this shell spells as a
// reserved word: `whence -w` names all seven `reserved` on zsh 5.9.2, before
// and after a function of the same name is defined.
var declarationReservedWords = []string{
	"typeset", "declare", "export", "local", "readonly", "integer", "float",
}

// **A function named after a declaration reserved word is defined and never
// reached by the bare word** (#7128). Measured 2026-11-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f -c` under `env -i PATH=/usr/bin:/bin`), every
// row for all seven words, with and without `setopt posixbuiltins` — so it is
// the word being reserved that decides, and not the builtin being special.
//
// The word the grammar reads as the reserved one runs the declaration: bare,
// behind `nocorrect` (a reserved word itself, gone before the command word is
// read), and inside an `eval`. Every spelling the grammar does not read that
// way is an ordinary command name and finds the function: a quoted or escaped
// word, an expansion, `noglob` in front, and the word after `disable -r`.
// `builtin` reaches the builtin, as it does past any function.
//
// `${x%%.*}` is so that `float` reads like the rest: it stores `1` as
// `1.000000000e+00`.
func TestADeclarationReservedWordOutranksAFunction(t *testing.T) {
	t.Parallel()
	const declared, called = "rc=0 x=1", "fn rc=0 x="
	for _, set := range []string{"", "setopt posixbuiltins; "} {
		for _, w := range declarationReservedWords {
			def := set + "function " + w + " { print fn; }; "
			report := `; print "rc=$? x=${x%%.*}"`
			for _, c := range []struct{ src, want string }{
				{def + w + " x=1" + report, declared},
				{def + "f() { " + w + ` x=1; print "in rc=$? x=${x%%.*}"; }; f`, "in rc=0 x=1"},
				{def + "nocorrect " + w + " x=1" + report, declared},
				{def + "eval '" + w + " x=1'" + report, declared},
				{def + "builtin " + w + " x=1" + report, declared},
				{def + `\` + w + " x=1" + report, called},
				{def + "'" + w + "' x=1" + report, called},
				{def + `"` + w + `" x=1` + report, called},
				{set + "w=" + w + "; function " + w + " { print fn; }; $w x=1" + report, called},
				{def + "noglob " + w + " x=1" + report, called},
				{def + "disable -r " + w + "; eval '" + w + " x=1'" + report, called},
			} {
				t.Run(c.src, func(t *testing.T) {
					t.Parallel()
					out, st := runZsh(t, t.TempDir(), c.src)
					if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
						t.Errorf("%s\ngot  %q at %d\nwant %q at 0", c.src, got, st, c.want)
					}
				})
			}
		}
	}
}

// **Under `posixbuiltins`, `command local a=1` declares the local** (#7127),
// which is interp.Semantics.LocalThroughCommandDeclaresNothing answered No.
// Measured 2026-11-02 on zsh 5.9.2, the same way. Without the option
// `command` finds no builtin at all, so the word is `command not found` at
// 127 and the question is never put; that row is here so the answer cannot
// leak into it. `command typeset` is the control: a declaration through
// `command` that was never in question.
func TestCommandLocalDeclaresUnderPosixBuiltins(t *testing.T) {
	t.Parallel()
	const inF = "f(){ command local a=1; print in rc=$? a=${a-unset}; }; a=out; f; print a=$a"
	for _, c := range []struct{ src, want string }{
		{
			"setopt posixbuiltins; command local a=1; print rc=$? a=${a-unset}",
			"rc=0 a=1\n",
		},
		{"setopt posixbuiltins; " + inF, "in rc=0 a=1\na=out\n"},
		{
			"setopt posixbuiltins; f(){ command typeset a=1; print in a=$a; }; a=out; f; print a=$a",
			"in a=1\na=out\n",
		},
		{inF, "f: command not found: local\nin rc=127 a=out\na=out\n"},
		{
			"command local a=1; print rc=$? a=${a-unset}",
			"zsh:1: command not found: local\nrc=127 a=unset\n",
		},
	} {
		t.Run(c.src, func(t *testing.T) {
			t.Parallel()
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%s\ngot  %q at %d\nwant %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
