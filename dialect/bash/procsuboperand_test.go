// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// This dialect is the one that carries a process substitution into a `${…}`
// operand and into a condition, and it is the only one.
//
// Measured 2026-09-05: `${u:-<(:)}` is a path in bash 3.2 and 5.3 and the
// five characters `<(:)` in ksh93, zsh and dash — which is not for want of
// the construct, since ksh93 and zsh have it in an ordinary word.
func TestProcessSubstitutionReachesAnOperandHere(t *testing.T) {
	t.Run("a default's word is a path", func(t *testing.T) {
		out, st := answersRun(t, `p=${nosuch:-<(:)}; case "$p" in /*) echo pathy;; *) echo "[$p]";; esac`)
		if want := "pathy"; strings.TrimSpace(out) != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
	})

	// And **not** when the expansion is written inside double quotes, which
	// is the spelling this row was first measured with and the reason it is
	// spelled without them now. Re-measured 2026-09-07 on bash 5.3.15 and
	// 3.2.57: `p=${nosuch:-<(:)}` is `/dev/fd/63` and `p="${nosuch:-<(:)}"`
	// is the five characters `<(:)`, in both builds.
	//
	// It is one fact rather than two: a `${ }` body inside double quotes is
	// double-quoted content, and a process substitution is no more written
	// there than it is anywhere else inside a pair of quotes (#1150).
	t.Run("and quoted it is the text, since a quoted body is quoted content", func(t *testing.T) {
		out, st := answersRun(t, `printf "[%s]" "${nosuch:-<(:)}"`)
		if want := "[<(:)]"; strings.TrimSpace(out) != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
	})

	t.Run("and a condition's operand is one too", func(t *testing.T) {
		// It matches nothing a script would have written down, which is the
		// whole observable effect: the pattern is a path this shell just
		// made. The row is here to say the word was *read* as a
		// substitution rather than refused.
		out, st := answersRun(t, `[[ x == <(:) ]] && echo hit || echo miss; echo after`)
		if want := "miss\nafter"; strings.TrimSpace(out) != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
	})
}
