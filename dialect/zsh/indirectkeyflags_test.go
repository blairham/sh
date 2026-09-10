// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `${(kP)name}` — the indirection carrying the group's `k` and `v` to the
// lookup it moves to. The substrate names the flags; this names the shell,
// which is the only place that is allowed, and it is worth having because
// only the preset turns on all four of the things the real line needs at
// once: the flag group, `typeset -A`, the set test and the array literal.
//
// The letters were accepted, dropped on the way through the flag, and the
// values came back — a list of the right length at status 0, which is the
// shape nothing complains about. See #1608.
func TestTheIndirectionFlagAnswersWithTheKeysThisDialectAsksFor(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"k names the keys of the table the value named",
			`typeset -A tab=(k1 v1 k2 v2); n=tab; printf "[%s]" "${(kP)n}"`,
			"[k1 k2]",
		},
		{
			"kv names its pairs",
			`typeset -A tab=(k1 v1 k2 v2); n=tab; printf "[%s]" "${(kvP)n}"`,
			"[k1 v1 k2 v2]",
		},
		{
			"and the set test still answers about the resolved name",
			`typeset -A tab=(k1 v1); n=tab; nx=nosuchparam; printf "[%s]" "${(kP)+n}" "${(kP)+nx}"`,
			"[1][0]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The line a prompt theme actually writes, whole.
//
// `_p9k_print_params` reproduces every parameter it is given as a `typeset`
// line, and the association branch is this: the pairs are read through the
// indirection, quoted, and printed back as an array literal. With the `k`
// dropped the values landed in the key positions and half the table went
// missing — a declaration that parses, assigns, and is wrong, which is worse
// than one that fails.
//
// Asserted as the whole line rather than as the expansion's text, because
// that is the artifact the caller keeps: a reader diffing two of these sees
// the words, and an implementation that answered the right *number* of fields
// in the wrong order would satisfy anything counting them.
func TestTheParameterPrinterIdiomReproducesAnAssociation(t *testing.T) {
	const src = `typeset -A tab=(k1 v1 k2 'has space')
name=tab
local kv=("${(@kvP)name}")
print -r -- "$name=(" "${(@q)kv}" ")"`
	out, st := runZsh(t, t.TempDir(), src)
	const want = "tab=( k1 v1 k2 has\\ space )\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
