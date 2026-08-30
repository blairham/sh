// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

func TestArrays(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"assignment and subscript", `a=(1 2 3); printf "%s" "${a[0]}${a[2]}"`, "13"},
		{"count", `a=(1 2 3); printf "%s" "${#a[@]}"`, "3"},
		{"empty array is not a bare assignment", `a=(); printf "%s" "${#a[@]}"`, "0"},
		{"element assignment", `a=(1 2); a[1]=9; printf "%s" "${a[1]}"`, "9"},
		{"assigning past the end grows it", `a=(1); a[3]=x; printf "%s" "${#a[@]}"`, "4"},
		// A plain reference is the first element, which is what keeps `$a`
		// working on an array.
		{"scalar view", `a=(p q); printf "%s" "$a"`, "p"},
		// A scalar assignment replaces the array rather than leaving both.
		{"scalar replaces", `a=(p q); a=z; printf "%s" "${#a[@]}"`, "1"},
		// Elements are words, so an array can be built from an expansion.
		{"elements are words", `x="m n"; a=(l $x); printf "%s" "${#a[@]}"`, "3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQuotedArrayKeepsOneFieldPerElement(t *testing.T) {
	// The reason a subscript can produce several fields: joining them would
	// lose an element containing a space, exactly as it would for `"$@"`.
	if got, _ := run(t, `a=("x y" z); printf "[%s]" "${a[@]}"`, nil); got != "[x y][z]" {
		t.Errorf("got %q, want [x y][z]", got)
	}
	// Unquoted, each element is then split like any other expansion.
	if got, _ := run(t, `a=("x y" z); printf "[%s]" ${a[@]}`, nil); got != "[x][y][z]" {
		t.Errorf("got %q, want [x][y][z]", got)
	}
}

func TestArrayBaseIsAnAxis(t *testing.T) {
	// bash and ksh93 count from 0, zsh from 1 — measured, and the reason a
	// subscript cannot be used as a slice offset without asking.
	src := `a=(p q r); printf "%s" "${a[1]}"`
	bash := BashSemantics()
	if got, _ := run(t, src, func(r *Runner) { r.Semantics = &bash }); got != "q" {
		t.Errorf("zero-based gave %q, want q", got)
	}
	zsh := ZshSemantics()
	if got, _ := run(t, src, func(r *Runner) { r.Semantics = &zsh }); got != "p" {
		t.Errorf("one-based gave %q, want p", got)
	}
}

func TestEmptyArrayIsNotAFunctionDefinition(t *testing.T) {
	// `a=()` and `f()` both put a parenthesis pair after a word. A function
	// name is a name and cannot contain `=`, which is what tells them apart —
	// without it the empty array was read as defining a function called `a=`.
	if _, st := run(t, `a=(); printf "%s" "${#a[@]}"`, nil); st != 0 {
		t.Errorf("empty array assignment failed with status %d", st)
	}
	if got, _ := run(t, `f() { printf fn; }; f`, nil); got != "fn" {
		t.Errorf("function definitions still work: got %q", got)
	}
}
