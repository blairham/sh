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

// The same rule at a span of one: `unset a[3]` on a three-element array
// blanks the element where it stands rather than taking the subscript away,
// so the array still has three. Invisible in the middle — a removed subscript
// reads back empty anyway — and plain at the end, which is where this was
// found. Measured against zsh 5.9.2 (2026-09-05).
func TestUnsetOfTheLastElementBlanksItInPlace(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`a=(x y z); unset "a[3]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if out != "[x][y][] n=3\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "[x][y][] n=3\n")
	}
}

// So the next append goes after the blank rather than into its place, which
// is the half a count cannot show and the half a script feels.
func TestUnsetOfTheLastElementKeepsThePlaceAnAppendGoesAfter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`a=(p q r); unset "a[3]"; a+=(z); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if out != "[p][q][][z] n=4\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "[p][q][][z] n=4\n")
	}
}

// Of the end-relative subscripts only `-1` acts here. `unset a[-2]` leaves
// every element where it was, where the shells that take a subscript away
// remove the middle one. Measured, not reasoned.
func TestUnsetFromTheEndReachesOnlyTheLastElement(t *testing.T) {
	for _, c := range []struct{ sub, want string }{
		{"-1", "[x][y][] n=3\n"},
		{"-2", "[x][y][z] n=3\n"},
	} {
		src := `a=(x y z); unset "a[` + c.sub + `]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
		out, st := runZsh(t, t.TempDir(), src)
		if out != c.want || st != 0 {
			t.Errorf("[%s] = %q (status %d), want %q", c.sub, out, st, c.want)
		}
	}
}

// The keyed attribute is the boundary the blanking must not cross: a key is
// removed from the table, and reading it back finds nothing rather than an
// empty element left standing.
func TestUnsetOfOneKeyRemovesIt(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`typeset -A m; m[k]=v; m[j]=w; unset "m[k]"; echo "n=${#m[@]} [${m[k]}]"`)
	if out != "n=1 []\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "n=1 []\n")
	}
}
