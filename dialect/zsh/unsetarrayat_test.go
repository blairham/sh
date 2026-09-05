// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `unset a[@]` replaces the elements with a single empty one here, which is
// this shell's reading of `unset` on a span rather than a rule for `[@]`.
// Measured against zsh 5.9.2 (2026-09-05).
func TestUnsetOfEveryElementLeavesOneEmptyElement(t *testing.T) {
	for _, sub := range []string{"@", "*"} {
		src := `a=(p q r); unset "a[` + sub + `]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
		out, st := runZsh(t, t.TempDir(), src)
		if out != "[] n=1\n" || st != 0 {
			t.Errorf("[%s] = %q (status %d), want %q", sub, out, st, "[] n=1\n")
		}
	}
}

// The empty element is a real one, so an append goes after it — the reading a
// count alone cannot distinguish from an array that was emptied outright.
func TestUnsetOfEveryElementKeepsAPlaceForAnAppend(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`a=(p q); unset "a[@]"; a+=(z); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if out != "[][z] n=2\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "[][z] n=2\n")
	}
}

// A scalar is one such span and comes back empty, quietly and at 0 — where
// the shell that takes elements away refuses it, having none to take.
func TestUnsetOfEveryElementEmptiesAScalar(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `a=hello; unset "a[@]"; echo "st=$? [$a]"`)
	if out != "st=0 []\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "st=0 []\n")
	}
}
