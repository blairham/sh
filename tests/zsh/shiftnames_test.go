// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// zsh's synopsis is `shift [ n ] [ name ... ]`: the operands after the count
// are arrays to shift, and the positional parameters are left alone when any
// name is given. Measured 2026-09-09 against zsh 5.9.2.
func TestShiftTakesArrayNamesAsOperands(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{{
		name: "one name shifts one element",
		src:  `a=(1 2 3 4 5 6); shift a; print -r -- "[${(j:,:)a}]"`,
		want: "[2,3,4,5,6]\n",
	}, {
		name: "a count in front applies to the name",
		src:  `a=(1 2 3 4 5 6); shift 2 a; print -r -- "[${(j:,:)a}]"`,
		want: "[3,4,5,6]\n",
	}, {
		name: "the one count applies to every name",
		src:  `a=(1 2 3); b=(x y z); shift 2 a b; print -r -- "[${(j:,:)a}][${(j:,:)b}]"`,
		want: "[3][z]\n",
	}, {
		name: "the same name twice shifts twice",
		src:  `a=(1 2 3 4); shift 2 a a; print -r -- "[${(j:,:)a}]"`,
		want: "[]\n",
	}, {
		name: "a count of zero is a no-op",
		src:  `a=(1 2 3); shift 0 a; print -r -- "[${(j:,:)a}]"`,
		want: "[1,2,3]\n",
	}, {
		name: "a count of the whole length empties it",
		src:  `a=(1 2 3); shift 3 a; print -r -- "st=$? [${(j:,:)a}]"`,
		want: "st=0 []\n",
	}, {
		name: "the count can arrive expanded",
		src:  `a=(1 2 3); n=2; shift $n a; print -r -- "[${(j:,:)a}]"`,
		want: "[3]\n",
	}, {
		name: "a name after -- is still a name",
		src:  `a=(1 2 3); shift -- a; print -r -- "st=$? [${(j:,:)a}]"`,
		want: "st=0 [2,3]\n",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The count in front of the names is optional, so the first word is ambiguous
// — and zsh settles it by *type*, not by whether the word looks like a number.
// A scalar `q=5` is a count; an array `q=(5)` is a name, and gets shifted like
// any other. Measured 2026-09-09.
func TestShiftTellsACountFromANameByType(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir,
		`q=5; a=(1 2 3 4 5 6); shift q a; print -r -- "[${(j:,:)a}][$q]"`)
	if want := "[6][5]\n"; out != want || st != 0 {
		t.Errorf("a scalar first word = %q (status %d), want %q at 0", out, st, want)
	}
	out, st = runZsh(t, dir,
		`q=(5); a=(1 2 3 4 5 6); shift q a; print -r -- "[${(j:,:)q}][${(j:,:)a}]"`)
	if want := "[][2,3,4,5,6]\n"; out != want || st != 0 {
		t.Errorf("an array first word = %q (status %d), want %q at 0", out, st, want)
	}
}

// Naming an array leaves the positional parameters where they are — the two
// readings of `shift` do not run together.
func TestShiftWithANameLeavesThePositionalParameters(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`set -- p1 p2 p3; a=(1 2 3); shift a; print -r -- "[$*][${(j:,:)a}]"`)
	if want := "[p1 p2 p3][2,3]\n"; out != want || st != 0 {
		t.Errorf("shift a = %q (status %d), want %q at 0", out, st, want)
	}
}

// Giving any operand at all is what protects the positional parameters — not
// the operand turning out to name an array. A count in front of a name that
// does not exist still shifts nothing. Measured 2026-09-09.
func TestShiftWithAnyOperandLeavesThePositionalParameters(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"a name that is not set", `set -- x y z; shift nosuch; print -r -- "[$*] st=$?"`},
		{"a count and a name that is not set", `set -- x y z; shift 1 nosuch; print -r -- "[$*] st=$?"`},
		{"a count past the end of the positionals", `set -- x y z; shift 5 nosuch; print -r -- "[$*] st=$?"`},
		{"a scalar behind the count", `s=hi; set -- x y z; shift 1 s; print -r -- "[$*] st=$?"`},
		{"an array", `set -- x y z; a=(1 2 3); shift a; print -r -- "[$*] st=$?"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if want := "[x y z] st=0\n"; out != want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, want)
			}
		})
	}
}

// A first word that is a *scalar* is the count, though, and it shifts the
// positional parameters like a literal one — including overrunning them.
func TestShiftReadsAScalarFirstWordAsACount(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `set -- x y z; s=1; shift s; print -r -- "[$*] st=$?"`)
	if want := "[y z] st=0\n"; out != want || st != 0 {
		t.Errorf("shift s = %q (status %d), want %q at 0", out, st, want)
	}
	out, st = runZsh(t, dir, `set -- x y z; s=9; shift s; print -r -- "[$*] st=$?"`)
	want := "zsh:shift:1: shift count must be <= $#\n[x y z] st=1\n"
	if out != want || st != 0 {
		t.Errorf("shift s past the end = %q (status %d), want %q at 0", out, st, want)
	}
}

// A name that is not an array is not an error: an unset name and a scalar
// behind the count are passed over in silence, and an association has no order
// to shift off the front of, so it goes the same way.
func TestShiftLeavesANameThatIsNotAnArrayAlone(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{{
		name: "a scalar",
		src:  `s=hello; shift s; print -r -- "st=$? [$s]"`,
		want: "st=0 [hello]\n",
	}, {
		name: "an association",
		src:  `typeset -A m=(k v); shift m; print -r -- "st=$? [${(kv)m}]"`,
		want: "st=0 [k v]\n",
	}, {
		name: "an unset name",
		src:  `shift nosuch; print -r -- "st=$?"`,
		want: "st=0\n",
	}, {
		name: "an unset name behind a real one",
		src:  `a=(1 2); shift 1 a nosuch; print -r -- "st=$? [${(j:,:)a}]"`,
		want: "st=0 [2]\n",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A count past the end of a named array is the same complaint the positional
// reading makes, and it does not stop the names behind it: the remaining names
// are still shifted, and only the status carries the failure out. Measured
// 2026-09-09 — note the message names `$#` even though the operand is an
// array, and that only one line is printed for the one name that overran.
func TestShiftReportsACountPastTheEndOfANamedArray(t *testing.T) {
	dir := t.TempDir()
	tooMany := "zsh:shift:1: shift count must be <= $#\n"
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{{
		name: "one name",
		src:  `a=(1 2 3); shift 5 a; print -r -- "st=$? [${(j:,:)a}]"`,
		want: tooMany + "st=1 [1,2,3]\n",
	}, {
		name: "the second of two",
		src:  `a=(1 2 3); b=(x y); shift 3 a b; print -r -- "st=$? [${(j:,:)a}][${(j:,:)b}]"`,
		want: tooMany + "st=1 [][x,y]\n",
	}, {
		name: "an empty array cannot give up its first element",
		src:  `a=(); shift a; print -r -- "st=$? n=${#a}"`,
		want: tooMany + "st=1 n=0\n",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
