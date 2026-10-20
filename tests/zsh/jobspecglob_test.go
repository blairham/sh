// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **The `?` right after a word's leading `%` is not a pattern** — the word is
// the `%?string` job spec — and that one `?` is all it reaches: another
// metacharacter still makes the word a pattern with the `?` as content, a
// quoted or expanded `%` counts, and a `?` further along is live. Measured
// 2026-10-01 against zsh 5.9.2, in a directory holding `%xb1`, `%?b2`, `%?`,
// `%a` and `b%a`. See interp.Semantics.JobSpecQuestionMarkIsLiteral.
func TestAJobSpecQuestionMarkIsNotAPattern(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `: > '%xb1'; : > '%?b2'; : > '%?'; : > '%a'; : > 'b%a'
print -r -- %? %?bar
print -r -- %?*
print -r -- '%'? $'%'? ${:-%}?
print -r -- b%?
print -r -- %??`)
	if want := "%? %?bar\n%? %?b2\n%? %? %?\nb%a\nzsh:6: no matches found: %??\n"; out != want || st != 1 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// **Pattern matching is not reached** — a `case` arm and a `[[ ]]` operand
// match `%x` with `%?` here as in every column.
func TestAJobSpecQuestionMarkStillMatchesInAPattern(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `case %x in %?) echo case;; esac; [[ %x == %? ]] && echo cond`)
	if want := "case\ncond\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
