// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// `=(cmd)` is this shell's alone: the temp-file spelling of process
// substitution, which runs the command to completion and hands over the path
// of a regular file holding its output.
//
// The substrate's tests name the grammar flag and the mechanism; this one
// names the shell, which is where having the construct at all is decided.
// Measured 2026-09-11 on zsh 5.9.2 against the whole panel — the other five
// columns refuse the `(`.
func TestFileProcessSubstitutionIsThisShells(t *testing.T) {
	t.Run("the word is a file holding the output", func(t *testing.T) {
		out, st := answersRun(t, `cat =(echo hi); [ -f =(echo hi) ] && echo regular`)
		if want := "hi\nregular\n"; out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
	})

	t.Run("the idiom it exists for", func(t *testing.T) {
		out, st := answersRun(t, `printf 'b\na\n' > u; diff =(sort u) =(printf 'a\nb\n') && echo same`)
		if want := "same\n"; out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
	})

	// The two spellings are refused in a condition with the same sentence and
	// a different status: 2 for the pipe forms, 1 for the file form. Measured
	// 2026-09-11, and the status is the only thing that tells them apart —
	// `echo after` runs after neither.
	t.Run("a condition refuses one and abandons the input", func(t *testing.T) {
		out, st := answersRun(t, `[[ x == =(x) ]] && echo hit || echo miss; echo after`)
		if want := "process substitution =(x) cannot be used here"; !strings.Contains(out, want) {
			t.Errorf("got %q, want it to contain %q", out, want)
		}
		if strings.Contains(out, "miss") || strings.Contains(out, "after") {
			t.Errorf("got %q, want nothing after the refusal", out)
		}
		if st != 1 {
			t.Errorf("status %d, want 1 — the pipe spellings answer 2 there and this one does not", st)
		}
	})

	// The prompt theme's own line, end to end, with the construct in the
	// grammar. The `=` is the twelfth character of the word there, so the
	// `(` behind it captures a pattern rather than opening a substitution —
	// and reading it the other way is what left a real startup with no
	// prompt (#1585). Measured 2026-09-11: zsh 5.9.2 captures `PROMPT_X`.
	t.Run("the prompt theme's pattern still captures", func(t *testing.T) {
		out, st := answersRun(t, `setopt extended_glob; cfg=$'\nprompt=PROMPT_X\n'; `+
			`[[ $'\n'$cfg$'\n' == (#b)*$'\n'prompt[$' \t']#=([^$'\n']#)$'\n'* ]] && `+
			`echo "captured=$match[1]" || echo nomatch`)
		if want := "captured=PROMPT_X\n"; out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
	})

	// An array literal is still two words, which is the other thing the
	// guard in opensPatternGroup protects and the one a `=(` rule written
	// as "an equals in front of a parenthesis" would take for a
	// substitution.
	t.Run("an array literal is still an array literal", func(t *testing.T) {
		out, st := answersRun(t, `a=(x y); echo "[${a[@]}] n=$#a"`)
		if want := "[x y] n=2\n"; out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
	})

	// An unterminated one is named the way the other two spellings are.
	// Without a case of its own the opener falls through to the wording for
	// an unmatched *quote*, and `cat =(echo hi` reported `unmatched =(` —
	// a sentence about a quote, for a script with none. Exact bytes,
	// measured on zsh 5.9.2.
	t.Run("an unterminated one names what was opened", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{`cat =(echo hi`, "parse error near `=(echo hi'"},
			{`echo =(`, "parse error near `=('"},
			// The two spellings it must not drift from.
			{`cat <(echo hi`, "parse error near `<(echo hi'"},
			{`cat $(echo hi`, "parse error near `$(echo hi'"},
		} {
			_, err := syntax.Parse(tc.src, zsh.Dialect())
			if err == nil {
				t.Errorf("%s: parsed, want a refusal", tc.src)
				continue
			}
			if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
				t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
			}
		}
	})

	// Where it opens, in the shell that has it. The path is random and the
	// file is gone with the command that named it — measured, `v==(echo hi);
	// [[ -f $v ]]` is false in zsh 5.9.2 — so what is asserted is the shape
	// of what came back. Which spellings are refused is the parser's
	// question and is pinned in syntax.
	t.Run("at the front of a word and at the front of a value", func(t *testing.T) {
		out, st := answersRun(t, `a=(=(echo hi) x); echo "$#a"; b==(echo hi); case $b in /*) echo absolute;; esac`)
		if want := "2\nabsolute\n"; out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q at 0", out, st, want)
		}
	})
}
