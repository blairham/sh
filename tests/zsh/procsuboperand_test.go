// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A `${…}` operand carries no process substitution here, and a condition's
// operand may not be one at all.
//
// The substrate's tests name the grammar flag and the axis; this one names
// the shell, which is where the answers are chosen. Measured 2026-09-05 on
// zsh 5.9.2: `${u:-<(:)}` is the five characters and `[[ x == <(x) ]]` is
// `process substitution <(x) cannot be used here` at status 2, with the rest
// of the command string never run.
func TestProcessSubstitutionIsNotAnOperandHere(t *testing.T) {
	t.Run("a default's word is the text it was written as", func(t *testing.T) {
		out, st := answersRun(t, `printf "[%s]" "${nosuch:-<(:)}"`)
		if want := "[<(:)]"; out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
	})

	t.Run("and a pattern operand is pattern text, groups and all", func(t *testing.T) {
		// `<(x)` is a `<` and the group `(x)`, which this shell's bare-group
		// pattern grammar matches against `<x`. The reading that took the
		// substitution's inner text matched a bare `x` instead.
		out, st := answersRun(t, `v='<x'; printf "[%s]" "${v#<(x)}"; v=x; printf "[%s]" "${v#<(x)}"`)
		if want := "[][x]"; out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
	})

	t.Run("a condition refuses one and abandons the input", func(t *testing.T) {
		out, st := answersRun(t, `[[ x == <(x) ]] && echo hit || echo miss; echo after`)
		if want := "process substitution <(x) cannot be used here"; !strings.Contains(out, want) {
			t.Errorf("got %q, want it to contain %q", out, want)
		}
		if strings.Contains(out, "miss") || strings.Contains(out, "after") {
			t.Errorf("got %q, want nothing after the refusal", out)
		}
		if st != 2 {
			t.Errorf("status %d, want 2", st)
		}
	})
}
