// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A keyed literal's bare elements are each **one** value here: the word is
// expanded as an assignment's value, so it is not split, not matched against
// the directory, and not dropped when it comes to nothing.
//
// Measured 2026-09-22 against bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `k='1 2'` and `v='3 4 5'`:
//
//	declare -A m=($k $v)      declare -A m=(["1 2"]="3 4 5" )
//	declare -A m=("$k" $v)    the same
//	declare -A m=(a *)        declare -A m=([a]="*" )
//	e=; declare -A m=(p $e q) declare -A m=([q]="" [p]="" )
//
// The star row and the empty row are the two that no amount of not-splitting
// reaches: an ordinary word globs and an unquoted empty one is removed. See
// interp.Semantics.BareElementsInATableLiteralAreEachOneValue.
//
// The indexed literal beside it is the control and it is not this question —
// `declare -a a=($k)` is two elements here as it is everywhere.
func TestAKeyedLiteralsBareElementIsOneValue(t *testing.T) {
	const keys = `; printf "[%s]" "${#m[@]}" "${m["1 2"]}"`
	for _, tc := range []struct{ name, src, want string }{
		{
			"neither half is split",
			`k='1 2'; v='3 4 5'; declare -A m=($k $v)` + keys,
			`[1][3 4 5]`,
		},
		{
			"nor is a value beside a quoted key",
			`k='1 2'; v='3 4 5'; declare -A m=("$k" $v)` + keys,
			`[1][3 4 5]`,
		},
		{
			"the literal written as a plain assignment answers the same",
			`k='1 2'; v='3 4 5'; declare -A m; m=($k $v)` + keys,
			`[1][3 4 5]`,
		},
		{
			"and so does the appending spelling",
			`k='1 2'; v='3 4 5'; declare -A m; m+=($k $v)` + keys,
			`[1][3 4 5]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := answersRun(t, tc.src); out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The star is a star and the empty word is a value, which are the two rows a
// dialect that simply does not split still gets wrong.
func TestAKeyedLiteralsBareElementIsNeitherGlobbedNorDropped(t *testing.T) {
	out, st := answersRun(t, `declare -A m=(a '*'); n=(a *); printf "[%s][%s]" "${#m[@]}" "${m[a]}"`)
	if want := "[1][*]"; out != want || st != 0 {
		t.Errorf("the star = %q status %d, want %q at 0", out, st, want)
	}
	out, st = answersRun(t, `e=; declare -A m=(p $e q); printf "[%s][%s][%s]" "${#m[@]}" "${m[p]}" "${m[q]}"`)
	if want := "[2][][]"; out != want || st != 0 {
		t.Errorf("the empty word = %q status %d, want %q at 0", out, st, want)
	}
}

// And an indexed literal is untouched by it, which is what keeps this about
// the keyed literal's pairing rather than about assignments in general.
func TestAnIndexedLiteralsElementsAreStillAWordList(t *testing.T) {
	out, st := answersRun(t, `k='1 2'; a=($k); printf "[%s][%s][%s]" "${#a[@]}" "${a[0]}" "${a[1]}"`)
	if want := "[2][1][2]"; out != want || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, want)
	}
}
