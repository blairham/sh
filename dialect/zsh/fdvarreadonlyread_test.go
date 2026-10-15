// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"runtime"
	"testing"
)

// TestAFrozenNameRefusingAReadOpenAddsALine — when the redirection a frozen
// `{name}` refuses opened a file for reading, a second line follows the
// refusal: the platform's sentence for errno zero, then the target (#6523).
//
// Measured 2026-10-15 with `-fc` on zsh 5.9.2, darwin and the pinned Linux
// image; the darwin sentence is `undefined error: 0` and the Linux one is
// `success`. `>`, `>>`, a duplication and a here-string write the first line
// alone — those are the controls, and they are the rows that keep this from
// being "a second line for every refusal".
func TestAFrozenNameRefusingAReadOpenAddsALine(t *testing.T) {
	var zero string
	switch runtime.GOOS {
	case "darwin":
		zero = "undefined error: 0"
	case "linux":
		zero = "success"
	default:
		t.Skip("errno zero's sentence is measured on darwin and Linux only")
	}
	const refusal = "zsh:2: can't allocate file descriptor to readonly parameter m\n"
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, wantErr string }{
		{"reading", "true {m}</dev/null\n", refusal + "zsh:2: " + zero + ": /dev/null\n"},
		{"reading and writing", "true {m}<>/dev/null\n", refusal + "zsh:2: " + zero + ": /dev/null\n"},
		{"on exec", "exec {m}</dev/null\n", refusal + "zsh:2: " + zero + ": /dev/null\n"},
		{"the target as it expanded", "f=/dev/null; true {m}<$f\n", refusal + "zsh:2: " + zero + ": /dev/null\n"},
		{"writing", "true {m}>/dev/null\n", refusal},
		{"appending", "true {m}>>/dev/null\n", refusal},
		{"duplicating", "true {m}<&0\n", refusal},
		{"a here-string", "true {m}<<<x\n", refusal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, "typeset -r m\n"+tc.src+"print -r -- \"st $?\"\n")
			if out != "st 1\n" || errs != tc.wantErr || st != 0 {
				t.Errorf("out %q err %q status %d, want %q / %q", out, errs, st, "st 1\n", tc.wantErr)
			}
		})
	}
}
