// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A word a diagnostic names is written with each character it cannot print
// in the caret notation, and no quotes around it. See
// interp.Diagnostics.DiagnosticNamesAWordVisibly.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin
// LC_ALL=en_US.UTF-8 zsh -fc "\$'…'"` (#5153).
func TestADiagnosticNamesAWordVisibly(t *testing.T) {
	for _, c := range []struct{ word, want string }{
		{`\x01x`, `command not found: ^Ax`},
		{`a\tb`, `command not found: a\tb`},
		{`a\x1b`, `command not found: a^[`},
		{`a\x7f`, `command not found: a^?`},
		{`\x80`, `command not found: \M-^@`},
		{`a\x9b`, `command not found: a\M-^[`},
		{`\xe9x`, `command not found: \M-ix`},
		{`a\xff\xa0\nb`, `command not found: a\M-^?\M- \nb`},
		{`é`, `command not found: é`},
	} {
		t.Run(c.word, func(t *testing.T) {
			_, _, errs := runZshUTF8(t, "$'"+c.word+"'")
			if !strings.Contains(errs, c.want+"\n") {
				t.Errorf("$'%s': stderr %q, want it to hold %q", c.word, errs, c.want)
			}
		})
	}
}

// Under C a byte from 0xa0 up is written as it is, and one from 0x80 to 0x9f
// still takes `\M-`; `cd` names its operand the same way.
func TestADiagnosticNamesAWordVisiblyUnderTheCLocale(t *testing.T) {
	_, _, errs := runZshUTF8(t, "LC_ALL=C; $'a\\xe9\\x9b'")
	if want := "command not found: a\xe9\\M-^[\n"; !strings.Contains(errs, want) {
		t.Errorf("stderr %q, want it to hold %q", errs, want)
	}
	_, _, errs = runZshUTF8(t, "cd $'a\\x01\\xe9'")
	if want := "no such file or directory: a^A\\M-i\n"; !strings.Contains(errs, want) {
		t.Errorf("cd: stderr %q, want it to hold %q", errs, want)
	}
}

// `print -X` counts the columns a character is drawn in: three wide letters
// take six. Measured 2026-10-02, `print -X8 -r -- $'one\tＺＳＨ\tthree'`.
func TestPrintExpandsTabsByColumns(t *testing.T) {
	out, _, errs := runZshUTF8(t, "print -X8 -r -- $'one\\tＺＳＨ\\tthree'")
	if want := "one     ＺＳＨ  three\n"; out != want || errs != "" {
		t.Errorf("got %q, %q, want %q", out, errs, want)
	}
}
