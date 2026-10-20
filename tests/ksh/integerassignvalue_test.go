// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A plain `=` inside arithmetic has the value the target's numeric attribute
// makes of it, which this shell answers the same way zsh does.
//
// The rule is shared and lives in interp/arith.go at numericAttribute; the
// rows below are this shell's own measurements of it, taken beside zsh's so
// that the one place they part is written down rather than assumed. Measured
// 2026-09-26 on ksh93u+ 2012-08-01 at `/bin/ksh`, from a script file.
//
// **The rows that discriminate are not the same ones in both shells**, and
// that is worth saying because the issue's own reduction is *not* one of them
// here: a float worth 31415 renders as `31415` in this shell either way, so
// `$(( i = f * 10000 ))` agreed before the rule existed. The rows that part
// are the ones where the float's own rendering differs from the integer's — a
// negative half, and a value the word cannot hold. See
// dialect/zsh/integerassignvalue_test.go.
func TestAnAssignmentsValueIsWhatTheAttributeMadeOfIt(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			// Truncation toward zero, which only the negative can say and
			// which this shell's float rendering makes visible.
			name: "a negative half truncates toward zero",
			src:  `typeset -i i; print -- $(( i = -1.5 ))`,
			want: "-1\n",
		},
		{"and a positive one", `typeset -i i; print -- $(( i = 1.9 ))`, "1\n"},
		{
			// **The number and not the text**: the name holds `16#6c` and
			// the assignment is worth 108.
			name: "an integer name with a base",
			src:  `typeset -i16 a; print -- "$(( a = 108 )) $a"`,
			want: "108 16#6c\n",
		},
		{
			// Its pair, holding the type fixed and moving the rendering the
			// other way.
			name: "a float name with a precision",
			src:  `typeset -F3 g; print -- "$(( g = 3.14159265358979 )) $g"`,
			want: "3.14159265358979 3.142\n",
		},
		{
			name: "an element of an integer name",
			src:  `typeset -i b; print -- "$(( b[2] = 1.5 )) ${b[2]}"`,
			want: "1 1\n",
		},
		{
			// The controls: a name with no numeric attribute is unmoved, and
			// an integer expression through the same operator is unmoved.
			name: "a name with no attribute keeps the float",
			src:  `print -- $(( u = 1.5 ))`,
			want: "1.5\n",
		},
		{
			name: "an integer into an integer name",
			src:  `typeset -i j; print -- $(( j = 7 ))`,
			want: "7\n",
		},
		{
			// And the truth of `(( ))` is the converted number here too.
			name: "the truth is the converted number",
			src:  `typeset -i d; (( d = 0.5 )); print -- "status=$?"`,
			want: "status=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// Where the two shells part: this one converts what a **compound** assignment
// computed as well, and zsh hands that value back untouched.
//
// Measured 2026-09-26 in one run of both binaries — ksh93u+ 2012-08-01 at
// `/bin/ksh` and zsh 5.9.2 (aarch64-apple-darwin25.4.0) run `-f` — with
// `typeset -i n=1` in front of each:
//
//	probe                  ksh93u+   zsh 5.9.2   n afterwards
//	$(( n += 0.5 ))        1         1.5         1 in both
//	$(( n = n + 0.5 ))     1         1           1 in both
//	$(( n *= 2.5 ))        2         2.5         2 in both
//
// The second row is the control and it is what makes this one question
// rather than two: the plain operator agrees in both columns and the store
// agrees on every row, so the only thing that moves is the compound
// operator's value. That pair also holds the attribute fixed and moves the
// operator, which is what says the rule is keyed on the operator rather than
// on "an assignment".
//
// This used to be recorded and deliberately not modeled, asserting the
// standing answer of `1.5 1`. It is
// Semantics.CompoundArithAssignmentConvertsItsValue now (#4606).
func TestACompoundAssignmentIsConvertedHere(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"added", `typeset -i n=1; print -- "$(( n += 0.5 )) $n"`, "1 1\n"},
		{"subtracted", `typeset -i n=1; print -- "$(( n -= 0.5 )) $n"`, "0 0\n"},
		{"multiplied", `typeset -i n=1; print -- "$(( n *= 2.5 )) $n"`, "2 2\n"},
		{"divided", `typeset -i n=8; print -- "$(( n /= 3.0 )) $n"`, "2 2\n"},
		{
			"read out of a larger expression",
			`typeset -i n=1; print -- "$(( (n += 0.5) + 0 )) $n"`, "1 1\n",
		},
		// The controls, each unmoved by the axis.
		{"the plain operator", `typeset -i n=1; print -- "$(( n = n + 0.5 )) $n"`, "1 1\n"},
		{"an integer value", `typeset -i n=1; print -- "$(( n += 1 )) $n"`, "2 2\n"},
		{"no attribute at all", `n=1; print -- "$(( n += 0.5 )) $n"`, "1.5 1.5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
