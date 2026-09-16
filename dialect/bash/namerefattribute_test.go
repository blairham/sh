// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// An attribute letter over an existing **name reference** belongs to what the
// reference points at. `declare -n r=v` makes `r` a reference, and a later
// declaration that does not write `-n` again operates on the target —
// attribute letters included, which is the rule bash and ksh93 share and this
// shell had wrong in both dialects (#3136).
//
// Measured 2026-09-16 against bash 5.3.20, `env -i` with a scratch HOME and
// no startup files. The `readonly` row is the one that costs a script
// something: a helper handed the name of a caller's variable and told to
// freeze it froze the reference, and the caller's variable stayed writable
// for the rest of the run at status 0.
func TestAnAttributeLetterLandsOnWhatTheReferencePointsAt(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		st              int
	}{
		{
			name: "the integer letter",
			src:  `u=1; declare -n s=u; declare -i s; declare -p s u`,
			want: "declare -n s=\"u\"\ndeclare -i u=\"1\"\n",
		},
		// And the letter *works* where it landed, which is what says it was
		// recorded and not merely printed.
		{
			name: "and the target evaluates",
			src:  `u=1; declare -n s=u; declare -i s; s=3+4; echo "[$u]"`,
			want: "[7]\n",
		},
		{
			name: "the readonly letter",
			src:  `u=1; declare -n s=u; declare -r s; declare -p s u`,
			want: "declare -n s=\"u\"\ndeclare -r u=\"1\"\n",
		},
		// The row #3136 is about: the freeze has to refuse a write to the
		// target, through the reference and by its own name alike.
		{
			name: "the readonly word freezes the target",
			src:  `u=1; declare -n s=u; readonly s; u=9; echo "never"`,
			want: "bash: line 1: u: readonly variable\n",
			st:   1,
		},
		{
			name: "and through the reference",
			src:  `u=1; declare -n s=u; readonly s; s=9; echo "never"`,
			want: "bash: line 1: u: readonly variable\n",
			st:   1,
		},
		{
			name: "the export word",
			src:  `u=1; declare -n s=u; export s; declare -p s u`,
			want: "declare -n s=\"u\"\ndeclare -x u=\"1\"\n",
		},
		{
			name: "the case letters",
			src:  `u=AB; declare -n s=u; declare -l s; declare -p u`,
			want: "declare -l u=\"AB\"\n",
		},
		{
			name: "the array letter",
			src:  `u=1; declare -n s=u; declare -a s; declare -p u`,
			want: "declare -a u=([0]=\"1\")\n",
		},
		// Two letters at once, and the value on the same operand: both halves
		// of the declaration are about the target.
		{
			name: "two letters and a value",
			src:  `u=1; declare -n s=u; declare -ix s=7; declare -p s u`,
			want: "declare -n s=\"u\"\ndeclare -ix u=\"7\"\n",
		},
		// Taking a letter off follows too — the same declaration reaching the
		// same cell.
		{
			name: "and a letter comes off again",
			src:  `u=1; declare -i u; declare -n s=u; declare +i s; declare -p u`,
			want: "declare -- u=\"1\"\n",
		},
		// The chain is walked to the end rather than one step.
		{
			name: "a chain of references",
			src:  `u=1; declare -n s=u; declare -n s2=s; declare -i s2; declare -p u`,
			want: "declare -i u=\"1\"\n",
		},
		// A reference aimed at a name nobody set brings that name into being
		// with the letter.
		{
			name: "a target that does not exist yet",
			src:  `declare -n s=nope; declare -i s; declare -p nope`,
			want: "declare -i nope\n",
		},
		// A reference aimed at an **element** carries the letter to the
		// array: an attribute is a property of the name and an array has one
		// name.
		{
			name: "a reference aimed at an element",
			src:  `declare -a a=(x y); declare -n s=a[1]; declare -i s; declare -p a`,
			want: "declare -ai a=([0]=\"x\" [1]=\"y\")\n",
		},
		// `-n` written on the same line is the exception and keeps the
		// letters on the reference, which is what
		// namerefreadonly_test.go pins from the other side.
		{
			name: "n on the same line keeps the letters",
			src:  `u=1; declare -rn s=u; declare -p s u`,
			want: "declare -nr s=\"u\"\ndeclare -- u=\"1\"\n",
		},
		// And a frozen *reference* does not stop a later letter reaching the
		// target: the freeze is the reference's and the letter is the
		// target's.
		{
			name: "a frozen reference still carries a letter through",
			src:  `v=1; declare -rn r=v; declare -i r; echo "st=$?"; declare -p r v`,
			want: "st=0\ndeclare -nr r=\"v\"\ndeclare -i v=\"1\"\n",
		},
		// A declaration that makes a **fresh** binding is a declaration of
		// that binding. `local -i s` inside a function, over a reference the
		// caller made, gives the function its own `s` and leaves the target
		// plain.
		{
			name: "a fresh local is not the reference",
			src:  `u=1; declare -n s=u; f() { local -i s; declare -p s u; }; f`,
			want: "declare -i s\ndeclare -- u=\"1\"\n",
		},
		// Where the binding is already the function's own, the letter follows
		// again — which is what says the rule is about the binding and not
		// about being inside a function.
		{
			name: "a second declaration in the same scope follows",
			src:  `f() { local u=1; local -n s=u; local -i s; declare -p s u; }; f`,
			want: "declare -n s=\"u\"\ndeclare -i u=\"1\"\n",
		},
		// `-g` takes no shadow, so it reaches the reference the caller made.
		{
			name: "the global letter follows too",
			src:  `u=1; declare -n s=u; f() { declare -g -r s; }; f; declare -p s u`,
			want: "declare -n s=\"u\"\ndeclare -r u=\"1\"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != tc.st {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.st)
			}
		})
	}
}
