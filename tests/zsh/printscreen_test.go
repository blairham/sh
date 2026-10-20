// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestPrintInAsManyColumnsAsTheScreenHolds is #5957: `print -c` fills down as
// many columns as `$COLUMNS` has room for, each the longest word plus two
// wide, the last column needing one space less. Every row measured on zsh
// 5.9.2 (/opt/homebrew/bin/zsh, -f), 2026-10-05.
func TestPrintInAsManyColumnsAsTheScreenHolds(t *testing.T) {
	const words = `w=(alias bg cd disown echo exec fc if typeset unset while zle zmodload zstyle)
`
	four := "alias     echo      typeset   zmodload\n" +
		"bg        exec      unset     zstyle\n" +
		"cd        fc        while\n" +
		"disown    if        zle\n"
	three := "alias     exec      while\n" +
		"bg        fc        zle\n" +
		"cd        if        zmodload\n" +
		"disown    typeset   zstyle\n" +
		"echo      unset\n"
	two := "alias     if\nbg        typeset\ncd        unset\ndisown    while\n" +
		"echo      zle\nexec      zmodload\nfc        zstyle\n"
	one := "alias\nbg\ncd\ndisown\necho\nexec\nfc\nif\ntypeset\nunset\nwhile\nzle\nzmodload\nzstyle\n"
	for _, tc := range []struct{ src, want string }{
		{`COLUMNS=40; print -rc -- $w`, four},
		{`COLUMNS=39; print -rc -- $w`, four},
		{`COLUMNS=38; print -rc -- $w`, three},
		{`COLUMNS=19; print -rc -- $w`, two},
		{`COLUMNS=18; print -rc -- $w`, one},
		{`COLUMNS=0; print -rc -- $w`, one},
		{`COLUMNS=80; print -rc -- $w`, "alias     cd        echo      fc        typeset   while     zmodload\n" +
			"bg        disown    exec      if        unset     zle       zstyle\n"},
		// The width counts the last column too, where `-C`'s does not, and
		// `-C` beside it decides alone.
		{`COLUMNS=40; print -c aaa b cc dddd`, "aaa   b     cc    dddd\n"},
		{`COLUMNS=40; print -cC2 aaa b cc dddd`, "aaa  cc\nb    dddd\n"},
		// The other letters reach it as they reach `-C`.
		{`COLUMNS=40; print -oc zz aa mm`, "aa  mm  zz\n"},
		{`COLUMNS=40; print -lc a b`, "a  b\n"},
		{`COLUMNS=40; print -nc a b`, "a  b\n"},
		{`COLUMNS=40; print -c 'a\tb' cc dd ee`, "a\tb  cc  dd  ee\n"},
		{`COLUMNS=40; print -v x -c a b; print -rn -- "[$x]"`, "[a  b\n]"},
		{`print -c; echo st=$?`, "st=0\n"},
	} {
		out, st := runZsh(t, t.TempDir(), words+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}
