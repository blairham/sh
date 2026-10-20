// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An element read sees what its own subscript wrote: the subscript is
// evaluated first and the array read after it.
//
// Measured 2026-10-05 with zsh 5.9.2. The array used to be read whole before
// the subscript ran, so the first and third rows read the element as it had
// been (`x`, `y`). The second row writes past the end and reads the first
// element, so it has nothing to see and is the control (#6099).
func TestAnElementReadSeesWhatItsSubscriptWrote(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y); print -r -- "${a[(a[1]=7)>0]}"`, "7\n"},
		{`a=(x y); print -r -- "${a[(a[2]=7)>0]}"`, "x\n"},
		{`a=(x); a[6]=y; print -r -- "${a[(a[6]=4)+2]}"`, "4\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// A range is taken after its ends have been evaluated, so an end that writes
// the array is seen by the span it bounds. Measured 2026-10-05 with zsh 5.9.2:
// the first two rows read `x y` before #6120, the span having been read before
// its end assigned. The third is the control: a search in an end needs the
// array before the end, so it is read first as it always was.
func TestARangeIsReadAfterItsEnds(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y); print -r -- "${a[1,a[2]=9]}"`, "x 9\n"},
		{`a=(x y); print -r -- $a[1,a[2]=9]`, "x 9\n"},
		{`a=(p q r); print -r -- "${a[(r)q,3]}"`, "q r\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}
