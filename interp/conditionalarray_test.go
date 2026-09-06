// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// All four conditional expansions come to the *parameter* when their test does
// not fire, and the parameter is the whole array — so all four keep its
// fields. `-` and `+` did; `=` and `?` fell to the scalar path and came back
// as one joined string.
//
// Core rather than an axis: `"${a[@]:=d}"` on a two-element array is two
// fields in bash 5.3.15, bash 3.2.57, that 5.3.15 build invoked as `sh`,
// ksh93u+ 2012-08-01 and zsh 5.9.2 — every panel member that has arrays.
// dash has none and refuses the literal.
//
// Measured 2026-09-06. The unquoted spelling hid the bug in three of the four
// dialects: the scalar path joins on the first character of IFS and the split
// that follows takes the fields straight back out, so `${a[@]:=d}` on
// `(one two)` was right by coincidence wherever splitting is on. An element
// holding a separator is what tells the two apart, and quoting removes the
// coincidence outright — which is why every row below counts fields.

// condArrayRun runs src on a core with nothing moved. The behavior asserted
// here is unanimous, so a dialect answer would only hide which question is
// being asked.
func condArrayRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return run(t, src, nil)
}

// The quoted spelling, which no coincidence can make right: joining gives one
// field and the shells give one per element.
func TestConditionalExpansionsKeepTheArraysFields(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// `-` and `+` already did, and are here as the pair the other two
		// have to match rather than as new coverage.
		{`a=("x y" z); set -- "${a[@]:-d}"; printf "%d" "$#"; printf "[%s]" "$@"`, `2[x y][z]`},
		{`a=("x y" z); set -- "${a[@]-d}"; printf "%d" "$#"; printf "[%s]" "$@"`, `2[x y][z]`},
		// The two that did not. One field holding `x y z` is the bug's answer
		// and it is a plausible string, so the count is the assertion.
		{`a=("x y" z); set -- "${a[@]:=d}"; printf "%d" "$#"; printf "[%s]" "$@"`, `2[x y][z]`},
		{`a=("x y" z); set -- "${a[@]=d}"; printf "%d" "$#"; printf "[%s]" "$@"`, `2[x y][z]`},
		{`a=("x y" z); set -- "${a[@]:?e}"; printf "%d" "$#"; printf "[%s]" "$@"`, `2[x y][z]`},
		{`a=("x y" z); set -- "${a[@]?e}"; printf "%d" "$#"; printf "[%s]" "$@"`, `2[x y][z]`},
		// The `[*]` spelling is the other side of the same branch and must
		// stay one joined field, so the fix cannot have been "keep the fields
		// always".
		{`a=("x y" z); set -- "${a[*]:=d}"; printf "%d" "$#"; printf "[%s]" "$@"`, `1[x y z]`},
		{`a=("x y" z); set -- "${a[*]:?e}"; printf "%d" "$#"; printf "[%s]" "$@"`, `1[x y z]`},
		// One element named by index is one field and a substring of it, not a
		// list — the guard that says the fix went to the whole-array subscript
		// and not to the operator.
		{`a=("x y" z); set -- "${a[0]:=d}"; printf "%d" "$#"; printf "[%s]" "$@"`, `1[x y]`},
	} {
		if out, st := condArrayRun(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The join used a hard space, so a changed IFS is a second way to see the same
// bug — and the way that shows the fields were never there rather than merely
// mis-separated. Under `IFS=-` the buggy answer is one field `one-two`, which
// no shell in the panel gives.
func TestConditionalExpansionsKeepTheirFieldsUnderAChangedIFS(t *testing.T) {
	for _, op := range []string{":=", ":?", "=", "?"} {
		src := `IFS=-; a=(one two); set -- "${a[@]` + op + `d}"; printf "%d" "$#"; printf "[%s]" "$@"`
		if out, st := condArrayRun(t, src); out != `2[one][two]` || st != 0 {
			t.Errorf("%s: got %q status %d, want %q at 0", op, out, st, `2[one][two]`)
		}
	}
}

// The firing side is untouched, and that is not incidental. `=` assigns and
// `?` is fatal, and both of those happen on the scalar path — answering "the
// parameter" for a fired test would take the array path and skip them, so a
// script that asked to be told its array was missing would silently receive
// the array instead.
func TestAFiredTestStillAssignsAndStillFails(t *testing.T) {
	// `?` on an unset name is fatal, and the message is the operator's word.
	out, st := condArrayRun(t, `a=(); printf "before"; printf "[%s]" "${a[@]:?e}"; printf "after"`)
	if st == 0 || !strings.Contains(out, "e") || strings.Contains(out, "after") {
		t.Errorf("fired `?`: got %q status %d, want the word and a nonzero status with nothing after it", out, st)
	}
	// `=` substitutes its word, and the word's own fields — one here.
	if out, st := condArrayRun(t, `unset a; set -- "${a[@]:=d}"; printf "%d" "$#"; printf "[%s]" "$@"`); out != `1[d]` || st != 0 {
		t.Errorf("fired `=`: got %q status %d, want %q at 0", out, st, `1[d]`)
	}
	// A `+` that fires yields nothing at all, which is zero fields rather
	// than one empty one.
	// `printf "[%s]"` runs its format once with no arguments, so the empty
	// brackets are printf's and the `0` is the field count.
	if out, st := condArrayRun(t, `a=(); set -- "${a[@]:+w}"; printf "%d" "$#"; printf "[%s]" "$@"`); out != `0[]` || st != 0 {
		t.Errorf("fired `+`: got %q status %d, want %q at 0", out, st, `0[]`)
	}
}

// The word side keeps the *word's* fields, which is the idiom's whole point,
// and it is a different question from the parameter side. `=` reaches it too.
func TestTheParameterSideAndTheWordSideAreDifferentQuestions(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`b=(p q); unset a; set -- "${a[@]:-${b[@]}}"; printf "%d" "$#"; printf "[%s]" "$@"`, `2[p][q]`},
		{`b=(p q); a=(one); set -- "${a[@]:+${b[@]}}"; printf "%d" "$#"; printf "[%s]" "$@"`, `2[p][q]`},
		// And when the parameter is what it came to, the parameter's fields.
		{`b=(p q); a=(one two); set -- "${a[@]:=${b[@]}}"; printf "%d" "$#"; printf "[%s]" "$@"`, `2[one][two]`},
	} {
		if out, st := condArrayRun(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}
