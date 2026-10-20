// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestATracedPatternMarksItsText is Diagnostics.TracePatternEscapesLiterals:
// a pattern in a trace, in `[[ … ]]` and in a `case` arm alike, carries a
// backslash before the characters that are text rather than pattern. Every
// row measured 2026-10-01 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`); the
// first is the shape zsh's own E02xtrace asks about (#5156).
func TestATracedPatternMarksItsText(t *testing.T) {
	rows := []struct{ src, want string }{
		{`[[ 'f o' == 'f x'* || 'b r' != 'z o' ]]`, `+zsh:1> [[ 'f o' == f\ x* || 'b r' != z\ o ]]`},
		{`[[ a == x\* ]]`, `+zsh:1> [[ a == x\* ]]`},
		{`[[ a == "x*"y* ]]`, `+zsh:1> [[ a == x\*y* ]]`},
		{`v='p q*'; [[ a == $v ]]`, "+zsh:1> v='p q*' \n" + `+zsh:1> [[ a == p\ q\* ]]`},
		{`v='p q*'; [[ a == $~v ]]`, "+zsh:1> v='p q*' \n" + `+zsh:1> [[ a == p\ q* ]]`},
		{`[[ a != *'$'* ]]`, `+zsh:1> [[ a != *\$* ]]`},
		{`[[ a == (x|'y z') ]]`, `+zsh:1> [[ a == (x|y\ z) ]]`},
		{`[[ a == $'\t'x ]]`, `+zsh:1> [[ a == $'\t'x ]]`},
		{`[[ a == "a'b" ]]`, `+zsh:1> [[ a == a'b ]]`},
		{`[[ a == 'x!y' ]]`, `+zsh:1> [[ a == x!y ]]`},
		{`[[ a == [a"-"c] ]]`, `+zsh:1> [[ a == [a-c] ]]`},
		{`[[ a == [!a-c] ]]`, `+zsh:1> [[ a == [!a-c] ]]`},
		{`[[ a == 'x#y' ]]`, `+zsh:1> [[ a == x\#y ]]`},
		{`[[ a == x#y ]]`, `+zsh:1> [[ a == x#y ]]`},
		{`[[ a == x'>' ]]`, `+zsh:1> [[ a == x\> ]]`},
		{`setopt extendedglob; [[ a == <1-2>'>' ]]`, "+zsh:1> setopt extendedglob\n" + `+zsh:1> [[ a == <1-2>\> ]]`},
		{`setopt extendedglob; [[ a == '<'1-2'>' ]]`, "+zsh:1> setopt extendedglob\n" + `+zsh:1> [[ a == \<1-2\> ]]`},
		{`v='=x'; [[ a == $v ]]`, "+zsh:1> v='=x' \n" + `+zsh:1> [[ a == \=x ]]`},
		{`v='=x'; [[ a == x$v ]]`, "+zsh:1> v='=x' \n" + `+zsh:1> [[ a == x=x ]]`},
		{`case 'a b' in 'x y'*|$'\t'|'x>'|'#') ;; a\ b) ;; esac`, "+zsh:1> case a b (x\\ y* | $'\\t' | x\\> | \\#)\n+zsh:1> case a b (a\\ b)"},
	}
	for _, row := range rows {
		out, _ := runZsh(t, t.TempDir(), "set -x\n"+row.src+"\n")
		want := strings.ReplaceAll(row.want, "+zsh:1> ", "+zsh:2> ") + "\n"
		if out != want {
			t.Errorf("%s\n got %q\nwant %q", row.src, out, want)
		}
	}
}
