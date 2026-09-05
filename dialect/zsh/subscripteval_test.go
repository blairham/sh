// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// What a subscript *means* here, in the spellings the grammar reads.
//
// Measured 2026-09-05 on zsh 5.9.2 against bash 5.3.15, bash 3.2.57, bash as
// `sh`, ksh93 and dash. Three readings this dialect has and the rest of the
// panel does not: a comma is the separator of a range rather than the
// arithmetic comma operator, a plain string is subscripted by character, and
// the positional parameters are a list a subscript reaches into.
//
// The braced and the bare spellings are one node since #846, so each of these
// answers both; `TestASubscriptWithoutBracesMeansTheSame` holds the claim.
func TestASubscriptPairIsARange(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a range of elements", `a=(w x y z); echo "[${a[1,3]}]"`, "[w x y]\n"},
		{"from the first", `a=(w x y z); echo "[${a[0,2]}]"`, "[w x]\n"},
		{"to the last", `a=(w x y z); echo "[${a[2,-1]}]"`, "[x y z]\n"},
		{"both ends from the end", `a=(w x y z); echo "[${a[-3,-2]}]"`, "[x y]\n"},
		{"one element", `a=(w x y z); echo "[${a[2,2]}]"`, "[x]\n"},
		{"backwards is empty", `a=(w x y z); echo "[${a[3,2]}]"`, "[]\n"},
		{"past the end is the end", `a=(w x y z); echo "[${a[2,99]}]"`, "[x y z]\n"},
		{"wholly past the end", `a=(w x y z); echo "[${a[9,10]}]"`, "[]\n"},
		{"a start past the start is nothing", `a=(w x y z); echo "[${a[-5,2]}]"`, "[]\n"},
		{"the last start there is", `a=(w x y z); echo "[${a[-4,2]}]"`, "[w x]\n"},
		{"an end before the first", `a=(w x y z); echo "[${a[1,0]}]"`, "[]\n"},
		{"an end counted back past the first", `a=(w x y z); echo "[${a[1,-5]}]"`, "[]\n"},
		{"an empty array", `a=(); echo "[${a[1,2]}]"`, "[]\n"},
		{"a name never set", `echo "[${nothing[1,2]}]"`, "[]\n"},
		{"endpoints are expressions", `a=(w x y z); i=2; echo "[${a[i,1+2]}]"`, "[x y]\n"},
		{"spaces around the comma", `a=(w x y z); echo "[${a[ 1 , 3 ]}]"`, "[w x y]\n"},
		{"without braces", `a=(w x y z); echo "[$a[1,3]]"`, "[w x y]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := condRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A range has two ends, and a third is not a wider range — it is a refusal.
//
// The other reading of the same characters answers it happily: the arithmetic
// comma operator takes the last operand, so `${a[1,2,3]}` would be the third
// element. Answering that here would be the other dialect's reading wearing
// this one's name, which is the silent kind of wrong.
func TestASubscriptWithThreePartsIsRefused(t *testing.T) {
	out, st := condRun(t, `a=(w x y z); echo "[${a[1,2,3]}]"; echo after`)
	if want := "zsh:1: bad substitution\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
}

// A subscript on a plain string is one of its characters.
//
// Both readings answer and neither reports, so a script cannot tell which
// shell it is on except by the value — an empty string is a plausible element
// as well as a plausible miss. That is what makes it a conflict for the
// semantics vector rather than a construct one grammar has.
func TestASubscriptOnAStringIsACharacter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the first character", `s=hello; echo "[${s[1]}]"`, "[h]\n"},
		{"the second", `s=hello; echo "[${s[2]}]"`, "[e]\n"},
		{"below the first", `s=hello; echo "[${s[0]}]"`, "[]\n"},
		{"the last", `s=hello; echo "[${s[-1]}]"`, "[o]\n"},
		{"past the end", `s=hello; echo "[${s[9]}]"`, "[]\n"},
		{"past the start", `s=hello; echo "[${s[-9]}]"`, "[]\n"},
		{"a range of characters", `s=hello; echo "[${s[2,4]}]"`, "[ell]\n"},
		{"a range clamped at the start", `s=hello; echo "[${s[-6,2]}]"`, "[he]\n"},
		{"a range to the last", `s=hello; echo "[${s[3,99]}]"`, "[llo]\n"},
		{"a character is not a byte", `s=héllo; echo "[${s[2]}][${s[2,3]}]"`, "[é][él]\n"},
		{"an operator reaches it", `s=hello; echo "[${s[2]#h}]"`, "[e]\n"},
		{"without braces", `s=hello; echo "[$s[2]]"`, "[e]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := condRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// `@` and `*` are a list a subscript reaches into, and every other special
// parameter is a value a subscript reaches into by character.
//
// The rest of the panel refuses all of it: bash calls the whole expansion a
// bad substitution when it is reached, ksh93 refuses the bracket while
// reading, dash says `Bad substitution`. That is the grammar flag
// `syntax.SpecialParamSubscript`, and this is the evaluation behind it.
func TestASubscriptOnTheSpecialParameters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the first parameter", `set -- a b c; echo "[${@[1]}]"`, "[a]\n"},
		{"through the star", `set -- a b c; echo "[${*[2]}]"`, "[b]\n"},
		{"the last", `set -- a b c; echo "[${@[-1]}]"`, "[c]\n"},
		{"past the end", `set -- a b c; echo "[${@[9]}]"`, "[]\n"},
		{"with no parameters", `set --; echo "[${@[1]}]"`, "[]\n"},
		{"a range of them", `set -- a b c; echo "[${@[1,2]}]"`, "[a b]\n"},
		{"a range through the star", `set -- a b c; echo "[${*[2,3]}]"`, "[b c]\n"},
		{"the whole list either way", `set -- a b c; echo "[${@[@]}][${*[*]}]"`, "[a b c][a b c]\n"},
		{"a positional is a string", `set -- abcd; echo "[${1[2]}]"`, "[b]\n"},
		{"a range of one parameter", `set -- abcd; echo "[${1[1,2]}]"`, "[ab]\n"},
		{"the status", `true; echo "[${?[1]}]"`, "[0]\n"},
		{"without braces", `set -- a b c; echo "[$@[1]][$*[2]]"`, "[a][b]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := condRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// How many fields a quoted subscript makes, which the *name* decides for a
// range and the subscript decides for `@` and `*`.
//
// Measured: `"${a[1,2]}"` is one field holding the elements joined, exactly as
// `"$a"` is; `"${@[1,2]}"` is one field per parameter, because `@` keeps its
// fields however it is subscripted; and either spelling of the list is enough
// — `"${@[*]}"` and `"${*[@]}"` both keep theirs.
func TestAQuotedSubscriptMakesTheFieldsItsNameMakes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a range joins", `a=(x y z); set -- "${a[1,2]}"; echo "n=$#"`, "n=1\n"},
		{"joined with the first of IFS", `IFS=-; a=(x y z); echo "[${a[1,2]}]"`, "[x-y]\n"},
		{"a range of parameters keeps its fields", `set -- a b c; set -- "${@[1,3]}"; echo "n=$#"`, "n=3\n"},
		{"a range through the star joins", `set -- a b c; set -- "${*[1,3]}"; echo "n=$#"`, "n=1\n"},
		{"the star subscript on the list keeps them", `set -- a b c; set -- "${@[*]}"; echo "n=$#"`, "n=3\n"},
		{"the at subscript through the star keeps them", `set -- a b c; set -- "${*[@]}"; echo "n=$#"`, "n=3\n"},
		{"a named array joins under the star", `a=(x y z); set -- "${a[*]}"; echo "n=$#"`, "n=1\n"},
		{"and keeps its fields under the at", `a=(x y z); set -- "${a[@]}"; echo "n=$#"`, "n=3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := condRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// `${#…}` is a count where the subscript named several elements and a width
// where it named one value — and a range is on both sides of that line, since
// a range of an array is elements and a range of a string is a substring.
func TestALengthOverARangeCountsWhatTheRangeNamed(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"elements are counted", `a=(aa bb cc); echo "[${#a[1,2]}]"`, "[2]\n"},
		{"characters are measured", `s=hello; echo "[${#s[2,4]}]"`, "[3]\n"},
		{"one character is one", `s=hello; echo "[${#s[2]}]"`, "[1]\n"},
		{"parameters are counted", `set -- a b c; echo "[${#@[1,2]}]"`, "[2]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := condRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
