// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// A length over `$!` is a shape this shell does not have. Measured 2026-09-12
// on zsh 5.9.2 from a script file under `env -i` with a scratch HOME and
// ZDOTDIR, `set -- p q r`:
//
//	$ zsh s.sh          # echo "[${#!}]"
//	s.sh:2: bad substitution
//	$ zsh s.sh          # echo "[$!]"
//	[0]
//
// bash 5.3, bash 3.2, bash-as-`sh`, dash, ksh93 and BusyBox ash all answer
// `[0]` to the first — the length of an empty `$!` — and `[]` to the second.
//
// The second row is what makes the first a fact about the shape: `$!` reads
// here, and is `0` rather than empty, so the refusal is not "there is nothing
// to measure" (#2415).
func TestALengthOverTheBangNameIsRefusedHere(t *testing.T) {
	if !zsh.Dialect().ParamLengthRefusesTheBangName {
		t.Error("zsh has no length over `$!`")
	}
	for _, tc := range []struct {
		src, out string
		fails    bool
	}{
		{src: `echo "[${#!}]"`, fails: true},
		// `$!` itself still reads, and is `0` here with no background job.
		{src: `echo "[$!]"`, out: "[0]\n"},
		{src: `echo "[${!}]"`, out: "[0]\n"},
		// The neighbors: a special that is not `!` is a length as ever, and
		// `$$` is five digits in this process, so the row is self-checking.
		{src: `echo "[${#?}]"`, out: "[1]\n"},
		// Deferred, not fatal: the refusal belongs to the run, so a branch
		// never taken never reaches it and the script ends normally.
		{src: `echo s; if false; then echo "[${#!}]"; fi; echo e`, out: "s\ne\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		switch {
		case tc.fails && out == "":
			t.Errorf("%q: no diagnostic, want a refusal", tc.src)
		case !tc.fails && err != nil:
			t.Errorf("%q: %v", tc.src, err)
		case !tc.fails && out != tc.out:
			t.Errorf("%q: out = %q, want %q", tc.src, out, tc.out)
		}
	}
}
