// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// The dialect has the grammar, so the real lines that need it run here.
//
// The substrate's tests name the flag; this one names the shell, which is the
// only place that is allowed — and it is worth having, because a flag nothing
// turns on is a construct nobody can write.
func TestTheElementSelectionOperatorsAreThisDialects(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			// The line this change was made for, whole: a startup file taking
			// its own hook out of the hook list and assigning the rest back.
			// It was `bad math expression: operand expected at ` + "`#fig_precmd'",
			"a startup file removing its own hook from the hook list",
			`precmd_functions=(a fig_precmd b); ` +
				`precmd_functions=(${(@)precmd_functions:#fig_precmd}); ` +
				`printf "[%s]" "${precmd_functions[@]}"`,
			"[a][b]",
		},
		{
			// The plugin loader's idiom for "is this path absolute": the value
			// survives only when the pattern does *not* match it. Written
			// through a variable rather than as the one nested expansion the
			// loader writes — `${${v:#/*}:-fallback}` — because nesting a
			// `${ }` inside another is a construct this shell does not have
			// yet, measured and independent of this operator: `${${v}}` is a
			// bad substitution here and `abc` in the shell.
			"an absolute path leaves nothing behind",
			`v=/abs/p; keep=${v:#/*}; printf "[%s]" "${keep:-WASABSOLUTE}"`,
			"[WASABSOLUTE]",
		},
		{
			"and a relative one survives",
			`v=rel/p; keep=${v:#/*}; printf "[%s]" "${keep:-WASABSOLUTE}"`,
			"[rel/p]",
		},
		{
			"the set operators, on this dialect's arrays",
			`a=(x y z); b=(y w); printf "[%s]" "${(@)a:|b}"; printf "[%s]" "${(@)a:*b}"`,
			"[x][z][y]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The offset is untouched in the dialect that gained the operators, which is
// where a widened disambiguation would actually be felt.
func TestTheOffsetStillWorksInThisDialect(t *testing.T) {
	const src = `v=abcdef; printf "[%s][%s][%s][%s]" "${v:2}" "${v:2:2}" "${v: -2}" "${v:-alt}"`
	if out, _ := runZsh(t, t.TempDir(), src); out != "[cdef][cd][ef][abcdef]" {
		t.Errorf("got %q, want %q", out, "[cdef][cd][ef][abcdef]")
	}
}
