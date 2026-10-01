// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
	"testing"
)

// `%D{…}` is written in the `strftime` builtin's language, extras included,
// and `print -P -f` prompt-expands its arguments and not its format (#5150).
// Measured 2026-10-01 on zsh 5.9.2 under `-c`.
func TestPromptClockAndPrintFormatted(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `print -rP -- '[%D{%9.}][%D{%N}][%D{%3.}][%D{%.}][%D{%6.}]'`+"\n")
	if !regexp.MustCompile(`^\[\d{9}\]\[\d{9}\]\[\d{3}\]\[\d{3}\]\[\d{6}\]\n$`).MatchString(out) || st != 0 {
		t.Errorf("the fraction conversions = %q at %d, want nine, nine, three, three and six digits", out, st)
	}
	for _, c := range []struct{ src, want string }{
		{`print -P -f '[%s]\n' '%%' x`, "[%]\n[x]\n"},
		{`print -P -f '%%n[%s]\n' y`, "%n[y]\n"},
		{`print -P -f '%d\n' '%?'`, "0\n"},
		{`LC_ALL=C; print -P -f '%s\n' '%D{%y}' | read y; print -r -- ${#y}`, "2\n"},
	} {
		if out, st := runZsh(t, dir, c.src+"\n"); out != c.want || st != 0 {
			t.Errorf("%s: got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
