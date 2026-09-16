// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// An attribute letter over an existing **name reference** belongs to what the
// reference points at, and ksh93 keeps the same rule bash does — which is
// what makes it the core's answer and not an axis, and what makes it worth
// pinning in both dialects (#3136).
//
// Measured against AT&T ksh93u+ 2012-08-01 (`/bin/ksh` here) on 2026-09-16,
// in `env -i` with a script file. Two rows part from bash's and are ksh93's
// own rather than this change's: a case letter folds the value that is
// already there, and an `-i` over an array folds every element to a number.
func TestAnAttributeLetterLandsOnWhatTheReferencePointsAt(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		st              int
	}{
		{
			name: "the integer letter",
			src:  `u=1; typeset -n s=u; typeset -i s; typeset -p s u`,
			want: "typeset -n s=u\ntypeset -i u=1\n",
		},
		{
			name: "and the target evaluates",
			src:  `u=1; typeset -n s=u; typeset -i s; s=3+4; print "[$u]"`,
			want: "[7]\n",
		},
		{
			name: "the readonly letter",
			src:  `u=1; typeset -n s=u; typeset -r s; typeset -p s u`,
			want: "typeset -n s=u\ntypeset -r u=1\n",
		},
		// The row #3136 is about, in the words this dialect uses: the freeze
		// refuses a write to the target by its own name and through the
		// reference alike.
		{
			name: "the readonly word freezes the target",
			src:  `u=1; typeset -n s=u; readonly s; u=9; print "never"`,
			want: "sh: u: is read only\n",
			st:   1,
		},
		{
			name: "and through the reference",
			src:  `u=1; typeset -n s=u; readonly s; s=9; print "never"`,
			want: "sh: u: is read only\n",
			st:   1,
		},
		{
			name: "the export word",
			src:  `u=1; typeset -n s=u; export s; typeset -p s u`,
			want: "typeset -n s=u\ntypeset -x u=1\n",
		},
		// ksh93's case letter folds what is already stored, where bash's
		// folds only what is assigned afterwards. The letter reaching the
		// target is the half this change is about; which value it then shows
		// is that dialect's own.
		{
			name: "the case letter folds the target",
			src:  `u=AB; typeset -n s=u; typeset -l s; typeset -p u`,
			want: "typeset -l u=ab\n",
		},
		{
			name: "two letters and a value",
			src:  `u=1; typeset -n s=u; typeset -ix s=7; typeset -p s u`,
			want: "typeset -n s=u\ntypeset -x -i u=7\n",
		},
		{
			name: "a chain of references",
			src:  `u=1; typeset -n s=u; typeset -n s2=s; typeset -i s2; typeset -p u`,
			want: "typeset -i u=1\n",
		},
		{
			name: "a target that does not exist yet",
			src:  `typeset -n s=nope; typeset -i s; typeset -p nope`,
			want: "typeset -i nope\n",
		},
		// The letter lands on the array and not on the one element, so every
		// element meets it — which in this dialect means every one of them is
		// read as a number.
		{
			name: "a reference aimed at an element",
			src:  `typeset -a a=(x y); typeset -n s=a[1]; typeset -i s; typeset -p a`,
			want: "typeset -a -i a=(0 0)\n",
		},
		{
			name: "the table letter",
			src:  `u=1; typeset -n s=u; typeset -A s; typeset -p u`,
			want: "typeset -A u=([0]=1)\n",
		},
		// `nameref` is this dialect's own word for the same declaration, so
		// a reference made with it is followed the same way.
		{
			name: "a reference made with the nameref word",
			src:  `u=1; nameref s=u; typeset -i s; typeset -p u`,
			want: "typeset -i u=1\n",
		},
		// A POSIX-form function has no scope here, so a `typeset` inside one
		// is the same declaration as at the top level and reaches the
		// reference the caller made.
		{
			name: "inside a function with no scope of its own",
			src:  `u=1; typeset -n s=u; f() { typeset -i s; }; f; typeset -p u`,
			want: "typeset -i u=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != tc.st {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.st)
			}
		})
	}
}
