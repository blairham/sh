// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The both-streams operators spelled with the ampersand **last**, asserted on
// the filesystem rather than on an exit status — status 0 is what makes this
// family's failure mode invisible, since `>&! f` with the family off is a
// `>&` onto a file called `!` that reports success and leaves `f` absent.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// says *not a Go executable*), script files with standard error kept apart.
// The five spellings are their `&>` twins row for row — thirty rows over both
// streams, a numeric target, a `-` target, `set -C` over an existing file,
// `set -C` over a missing one, and an append onto seeded contents — and the
// same probe reports a difference when the pairs are deliberately mismatched,
// so the agreement is evidence rather than a probe that cannot tell anything
// apart.
//
// This is what `A04redirect.ztst` stops on under `'>&|', '>&!', '&>|', '&>!'
// redirection`, a chunk that writes ten of the twelve spellings in a row.

// TestTheReversedFamilyTakesBothStreams is the base reading: each of the five
// sends standard error to the named file along with standard output.
func TestTheReversedFamilyTakesBothStreams(t *testing.T) {
	for _, op := range []string{">>&", ">&|", ">&!", ">>&|", ">>&!"} {
		t.Run(op, func(t *testing.T) {
			out, st, read := markerRun(t, "( print O; print E >&2 ) "+op+"f\n")
			if out != "" || st != 0 {
				t.Errorf("%s: out %q status %d, want both streams in the file", op, out, st)
			}
			if got := read("f"); got != "O\nE\n" {
				t.Errorf("%s: f holds %q, want both streams", op, got)
			}
			if got := read("!f"); got != "<absent>" {
				t.Errorf("%s: wrote a file called %q — the marker was read as a word", op, "!f")
			}
		})
	}
}

// TestTheReversedAppendAppends separates `>>&` from `>&`, which is the pair
// that the base reading above cannot tell apart: both put the two streams in
// the file, and only one of them keeps what was there.
func TestTheReversedAppendAppends(t *testing.T) {
	for _, tc := range []struct{ op, want string }{
		{">>&", "seed\nO\nE\n"},
		{">>&|", "seed\nO\nE\n"},
		{">>&!", "seed\nO\nE\n"},
		// The control: the truncating spellings of the same family, which
		// must not keep it.
		{">&|", "O\nE\n"},
		{">&!", "O\nE\n"},
		{">&", "O\nE\n"},
	} {
		t.Run(tc.op, func(t *testing.T) {
			_, st, read := markerRun(t, "print seed > f\n( print O; print E >&2 ) "+tc.op+"f\n")
			if st != 0 {
				t.Errorf("%s: status %d", tc.op, st)
			}
			if got := read("f"); got != tc.want {
				t.Errorf("%s: f holds %q, want %q", tc.op, got, tc.want)
			}
		})
	}
}

// TestTheMarkerOverridesNoclobberInThisFamilyToo is the cell where the marker
// is the only thing that can produce the result, and it has two halves
// because this dialect puts noclobber on appends as well: over an existing
// file the unmarked spellings refuse, and over a *missing* one the unmarked
// append refuses.
func TestTheMarkerOverridesNoclobberInThisFamilyToo(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a truncating write over an existing file",
			"setopt noclobber\nprint seed > f\n( print O ) >&|f\n", "O\n",
		},
		{
			"the same with the bang",
			"setopt noclobber\nprint seed > f\n( print O ) >&!f\n", "O\n",
		},
		{
			"an append onto a file that is not there",
			"setopt noclobber\n( print O ) >>&|f\n", "O\n",
		},
		{
			"the same with the bang",
			"setopt noclobber\n( print O ) >>&!f\n", "O\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, read := markerRun(t, tc.src)
			if got := read("f"); got != tc.want {
				t.Errorf("f holds %q, want %q", got, tc.want)
			}
		})
	}
	// And the unmarked spellings still refuse, which is what makes the four
	// rows above about the marker rather than about noclobber being ignored.
	for _, tc := range []struct{ name, src, want string }{
		{"noclobber still refuses a truncating write", "setopt noclobber\nprint seed > f\n( print O ) >&f\n", "seed\n"},
		{"and still refuses an append to a missing file", "setopt noclobber\n( print O ) >>&f\n", "<absent>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, read := markerRun(t, tc.src)
			if got := read("f"); got != tc.want {
				t.Errorf("f holds %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTheReversedFamilyAlwaysNamesAFile is what tells the family from `>&`
// itself, and it is the row a reader would get wrong from the spelling: `>&2`
// duplicates the descriptor and `>&-` closes it, while every spelling this
// flag adds treats the same text as a **filename**.
func TestTheReversedFamilyAlwaysNamesAFile(t *testing.T) {
	for _, tc := range []struct{ src, file, want string }{
		{"print O >>&2\n", "2", "O\n"},
		{"print O >>&-\n", "-", "O\n"},
		{"print O >&|2\n", "2", "O\n"},
		{"print O >&!2\n", "2", "O\n"},
		{"print O >>&|2\n", "2", "O\n"},
		{"print O >>&!2\n", "2", "O\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st, read := markerRun(t, tc.src)
			if st != 0 {
				t.Errorf("%q: status %d, out %q", tc.src, st, out)
			}
			if got := read(tc.file); got != tc.want {
				t.Errorf("%q: file %q holds %q, want %q", tc.src, tc.file, got, tc.want)
			}
		})
	}
	// The controls, which must keep the descriptor reading they have always
	// had: `>&2` is a duplication and writes no file, and `>&-` is a close.
	for _, tc := range []struct{ src, absent, out string }{
		{"print O >&2\n", "2", "O\n"},
		{"print O >&-\n", "-", ""},
	} {
		t.Run("control "+tc.src, func(t *testing.T) {
			out, _, read := markerRun(t, tc.src)
			if out != tc.out {
				t.Errorf("%q: combined output %q, want %q", tc.src, out, tc.out)
			}
			if got := read(tc.absent); got != "<absent>" {
				t.Errorf("%q: wrote a file called %q holding %q — the target was read as a name",
					tc.src, tc.absent, got)
			}
		})
	}
}
