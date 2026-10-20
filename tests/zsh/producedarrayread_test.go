// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestReadingAProducedArrayDoesNotWriteIt is the gate that stood in front of
// the whole shipped completion system, reduced to two lines.
//
// A parameter whose elements are *produced* answers with the elements the
// name holds now, and the shortest way to write such a producer is to hand
// back the storage they already live in — `argv` answered with the runner's
// own positional parameters, `$words` with the completion's own word list.
// Several steps of expansion then write into the slice they were given,
// because every other array reaching them is a copy made at the read. So
// reading the positional parameters through a modifier **replaced** them.
//
// Measured on zsh 5.9.2, 2026-09-16 against this shell:
//
//	f() { : ${(@)argv%%:*}; print -r -- "$argv" }; f a:1 b:2
//	          zsh: a:1 b:2      here, before: a b
//
// The shipped `_alternative` is the caller that found it. Its first act is
// `_tags "${(@)argv%%:*}"` — the tag names, taken off the front of each
// `tag:description:action` it was given — and after that read its own `for
// def` loop walked the *tag names*. So every definition's action was the tag,
// and `git che<TAB>` printed twelve lines of `command not found: aliases`,
// `command not found: main-porcelain-commands` and the rest.
//
// Both spellings are asserted. `${(@)argv%%:*}` is the one that was wrong and
// `${@%%:*}` is the one that was right, and a fix at the operator rather than
// at the seam would have left them disagreeing.
func TestReadingAProducedArrayDoesNotWriteIt(t *testing.T) {
	for _, c := range []struct{ name, read, want string }{
		{"a modifier through the (@) flag", `: ${(@)argv%%:*}`, "a:1 b:2"},
		{"the same read spelled with @", `: ${@%%:*}`, "a:1 b:2"},
		{"a case-changing flag", `: ${(U)argv}`, "a:1 b:2"},
		{"and the read still answers", `print -r -- "${(@)argv%%:*}"`, "a b\na:1 b:2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "f() { " + c.read + `; print -r -- "$argv" }` + "\nf a:1 b:2\n"
			out, st := runZsh(t, t.TempDir(), src)
			if got := strings.TrimRight(out, "\n"); got != c.want || st != 0 {
				t.Errorf("%s left $argv %q (status %d), want %q", c.name, got, st, c.want)
			}
		})
	}
}
