// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A complaint raised while a builtin is running is not always the builtin's.
// What a prompt substitution, an arithmetic warning or a trap action says is
// the shell's own, and zsh names no builtin in its location. Measured
// 2026-10-04 on zsh 5.9.2:
//
//	setopt promptsubst; print -P '${x?boom}'    zsh:1: x: boom
//	[ -t 99999999999999999999 ]                 zsh:1: number truncated …
//	trap nosuch INT; kill -INT $$               zsh:1: command not found: nosuch
//
// The control is the builtin's own refusal, which still carries its name.
func TestTheShellSpeaksInsideABuiltin(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a prompt substitution", `setopt promptsubst; print -P '${x?boom}'`, "zsh:1: x: boom"},
		{"a math failure in one", `setopt promptsubst; print -P '$((nofunc()))'`, "zsh:1: unknown function: nofunc"},
		{"an arithmetic warning", `[ -t 99999999999999999999 ]`, "zsh:1: number truncated after 19 digits"},
		{"a trap action", `trap 'nosuchcmd-xyz' INT; kill -INT $$`, "zsh:1: command not found: nosuchcmd-xyz"},
		{"the builtin's own refusal", `print -Q x`, "zsh:print:1: bad option: -Q"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, errs := runZshSplit(t, t.TempDir(), tc.src)
			if !strings.Contains(errs, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", errs, tc.want)
			}
		})
	}
}
