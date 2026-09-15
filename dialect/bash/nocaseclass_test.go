// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `nocasematch` and `nocaseglob` fold a literal and a range inside a bracket
// expression and stop at a POSIX character class.
//
// This shell folded the class too, which reads as harmless and is not: a
// script testing `[[ $x == [[:lower:]]* ]]` to find out whether a name is
// already lower case was answered yes for `Foo` the moment anything in the
// session had turned `nocasematch` on (#2716).
//
// Measured 2026-09-14 under `LC_ALL=C` on bash 5.3.15, which is the column
// this dialect grades. bash 3.2.57 folds `A` into `[[:lower:]]` and does not
// fold `a` into `[[:upper:]]`, so it disagrees with itself as well as with
// 5.3 and no single rule about the option covers it; no preset here is bash
// 3.2, so it is recorded rather than given an axis.
//
// The `=~` rows are the neighbor that does fold a class, because there the
// fold belongs to the compiled regular expression rather than to this
// matcher — see TestNocasematchFoldsTheRegexOperatorAndTheWholeExpression.
func TestNocasematchStopsAtAPosixClassInABracket(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a class stays exact",
			`shopt -s nocasematch; [[ A == [[:lower:]] ]] && echo fold || echo exact`, "exact",
		},
		{
			"and stays exact the other way round",
			`shopt -s nocasematch; [[ a == [[:upper:]] ]] && echo fold || echo exact`, "exact",
		},
		{
			"a range beside it folds",
			`shopt -s nocasematch; [[ A == [a-z] ]] && echo fold || echo exact`, "fold",
		},
		{
			"an enumerated member folds",
			`shopt -s nocasematch; [[ A == [ab] ]] && echo fold || echo exact`, "fold",
		},
		{
			"a case pattern reads the class the same way",
			`shopt -s nocasematch; case A in [[:lower:]]) echo fold;; *) echo exact;; esac`, "exact",
		},
		{
			"a substitution reads the class the same way",
			`shopt -s nocasematch; v=ABC; echo ${v//[[:lower:]]/X}`, "ABC",
		},
		{
			"and folds a range in one",
			`shopt -s nocasematch; v=ABC; echo ${v//[a-z]/X}`, "XXX",
		},
		{
			"and folds a literal in one",
			`shopt -s nocasematch; v=ABC; echo ${v//abc/X}`, "X",
		},
		// The regular-expression operator is the other mechanism and folds
		// the class, which is what makes the rows above a statement about
		// the glob bracket rather than about character classes.
		{
			"the regex operator still folds a class",
			`shopt -s nocasematch; [[ A =~ ^[[:lower:]]$ ]] && echo fold || echo exact`, "fold",
		},
		// And with the option off the exact rows are unchanged, so nothing
		// above is evidence that the class simply never matched.
		{
			"the class matches its own case with the option off",
			`[[ a == [[:lower:]] ]] && echo yes || echo no`, "yes",
		},
		{
			"and with it on",
			`shopt -s nocasematch; [[ a == [[:lower:]] ]] && echo yes || echo no`, "yes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if got := strings.TrimRight(out, "\n"); got != tc.want {
				t.Errorf("%s = %q status %d, want %q", tc.src, got, st, tc.want)
			}
		})
	}
}

// The same seam on the other option, which is pathname expansion's own: a
// class in a bracket stays exact and the range beside it folds.
//
// Two options rather than one, because they are two fields here and a fix
// applied to only the matcher's option call site would leave this one folding.
func TestNocaseglobStopsAtAPosixClassInABracket(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"A", "b"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a class stays exact", `shopt -s nocaseglob; echo [[:lower:]]`, "b"},
		{"a range folds", `shopt -s nocaseglob; echo [a-z]`, "A b"},
		{"a literal folds", `shopt -s nocaseglob; echo a`, "a"},
		{"and the option is what decides the range", `echo [a-z]`, "b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if got := strings.TrimRight(out, "\n"); got != tc.want {
				t.Errorf("%s = %q status %d, want %q", tc.src, got, st, tc.want)
			}
		})
	}
}
