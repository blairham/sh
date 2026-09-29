// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A pipeline element's own redirection **joins** the pipe rather than
// replacing it, which is this shell's MULTIOS reading applied to the one
// target it had not been applied to.
//
// `print one >foo | cat` writes `one` to `foo` *and* down the pipe, and `cat
// a | cat <b` reads the pipe and then `b`. The shell already joined two files
// — `>foo >bar` and `<a <b` both worked — and the pipe was simply not in the
// set, so the first redirection replaced it. That is what `A04redirect.ztst`
// stops on under `read file+pipe multio` and `read multio input with pipe`.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), script files under `env -i
// PATH=/usr/bin:/bin` with standard input on the null device.
func TestARedirectionOnAPipelineElementJoinsThePipe(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the write side", "print one >foo | cat\nprint -r -- \"foo=[$(<foo)]\"\n",
			"one\nfoo=[one]\n",
		},
		{
			"two files and the pipe", "print one >foo >baz | cat\nprint -r -- \"foo=[$(<foo)] baz=[$(<baz)]\"\n",
			"one\nfoo=[one] baz=[one]\n",
		},
		{
			"appending joins too", "print seed >foo\nprint one >>foo | cat\nprint -r -- \"foo=[$(<foo)]\"\n",
			"one\nfoo=[seed\none]\n",
		},
		{
			"a middle element", "print one | sed s/one/two/ >foo | cat\nprint -r -- \"foo=[$(<foo)]\"\n",
			"two\nfoo=[two]\n",
		},
		{
			"the read side, and the pipe comes first",
			"print AAA >a\nprint BBB >b\ncat a | cat <b\n", "AAA\nBBB\n",
		},
		{
			"a group's own list joins", "{ print one } >foo | cat\nprint -r -- \"foo=[$(<foo)]\"\n",
			"one\nfoo=[one]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := pipeMultioRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}

// **Only the element's own list joins**, which is the half that keeps this
// from being "a redirection under a pipe always tees".
//
// A redirection on a command *inside* a brace group, a subshell or a function
// replaces the pipe as it always did: `{ print one >foo } | cat` writes `foo`
// and nothing reaches `cat`. Measured the same day. That boundary is the
// command dispatcher rather than the redirection list — a group with no
// redirections of its own runs no list, so a rule written on the list let the
// first command inside the group take the pipe.
func TestOnlyTheElementsOwnListJoins(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"inside a brace group", "{ print one >foo } | cat\nprint -r -- \"foo=[$(<foo)]\"\n", "foo=[one]\n"},
		{"inside a subshell", "( print one >foo ) | cat\nprint -r -- \"foo=[$(<foo)]\"\n", "foo=[one]\n"},
		{"inside a function", "f(){ print one >foo }\nf | cat\nprint -r -- \"foo=[$(<foo)]\"\n", "foo=[one]\n"},
		{
			"the second command in a group",
			"{ print zero; print one >foo } | cat\nprint -r -- \"foo=[$(<foo)]\"\n", "zero\nfoo=[one]\n",
		},
		{"the read side, in a group", "print AAA >a\nprint BBB >b\ncat a | { cat <b }\n", "BBB\n"},
		{"the read side, in a function", "print AAA >a\nprint BBB >b\nf(){ cat <b }\ncat a | f\n", "BBB\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := pipeMultioRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}

// And `unsetopt multios` puts the old reading back on both sides, which is
// what makes this the existing axis rather than a new rule: the pipe goes
// into the same maps `>foo >bar` and `<a <b` already use, and
// interp.Semantics.RedirectsUseEveryTarget still decides.
//
// The option is read rather than asked here, which matters for a vector that
// has not answered it: `print one >foo | cat` is not an ambiguous thing to
// have written — every column does something with it — so a shell with no
// dialect chosen must go on writing the file rather than refusing by name.
func TestWithoutMultiosThePipeIsReplacedAgain(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the write side", "unsetopt multios\nprint one >foo | cat\nprint -r -- \"foo=[$(<foo)]\"\n", "foo=[one]\n"},
		{"the read side", "print AAA >a\nprint BBB >b\nunsetopt multios\ncat a | cat <b\n", "BBB\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := pipeMultioRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}

// pipeMultioRun runs one snippet with a PATH, because every row here needs a
// real `cat` or `sed` on the other side of the pipe: the whole question is
// what the element writes into a pipe somebody else is reading.
func pipeMultioRun(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "sh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}
