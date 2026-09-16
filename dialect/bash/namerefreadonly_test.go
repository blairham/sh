// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A **frozen reference**: `declare -rn r=v` freezes the reference and not
// what it points at, and the two halves of that are separate answers.
//
// Measured 2026-09-16 against bash 5.3.20, `env -i` with a scratch HOME and
// no startup files. bash is the only column that can make one — ksh93u+
// answers `typeset -rn` with its usage block at 2, and no other shell on the
// panel spells a reference at all — so this is the core's answer and no
// dialect is asked.
//
// The whole of it was missing: the `r` letter is marked at the bottom of the
// declaration loop and the reference branch leaves before it, so the letter
// went nowhere. That is the shape #3106 found for the export attribute a call
// writes, under a second letter.
func TestAFrozenReferenceIsFrozenAndItsTargetIsNot(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		st              int
	}{
		{
			name: "the letter is carried",
			src:  `v=1; declare -rn r=v; declare -p r`,
			want: "declare -nr r=\"v\"\n",
		},
		{
			name: "and the target is left alone",
			src:  `v=1; declare -rn r=v; declare -p v`,
			want: "declare -- v=\"1\"\n",
		},
		// A write through a frozen reference is a write to what it points
		// at, and that name is not frozen. This shell asked the *written*
		// name, so every write through one was refused.
		{
			name: "a write goes through at 0",
			src:  `v=1; declare -rn r=v; r=5; echo "[$v]"`,
			want: "[5]\n",
		},
		// And the freeze still speaks when the *target* is the frozen one,
		// naming the target and giving up the rest of the line — measured,
		// the `echo` after it never runs. That row is what says this change
		// moved which name is consulted and nothing else.
		{
			name: "a frozen target still refuses",
			src:  `v=1; readonly v; declare -n r=v; r=5; echo "[$v]"`,
			want: "bash: line 1: v: readonly variable\n",
			st:   1,
		},
		// Re-aiming is a declaration over a frozen name and takes the
		// ordinary refusal. The bare form too: `declare -n r` over a frozen
		// reference is refused rather than being the no-op it is otherwise.
		{
			name: "a re-aim is refused",
			src:  `v=1; w=2; declare -rn r=v; declare -n r=w; echo "st=$?"; declare -p r`,
			want: "bash: line 1: declare: r: readonly variable\nst=1\ndeclare -nr r=\"v\"\n",
		},
		{
			name: "and so is the bare letter",
			src:  `v=1; declare -rn r=v; declare -n r; echo "st=$?"`,
			want: "bash: line 1: declare: r: readonly variable\nst=1\n",
		},
		{
			name: "and a second frozen declaration",
			src:  `v=1; w=2; declare -rn q=v; declare -rn q=w; echo "st=$?"`,
			want: "bash: line 1: declare: q: readonly variable\nst=1\n",
		},
		// `unset -n` is the one route that takes a reference apart, and it
		// is refused in the same words the plain `unset` uses — so a script
		// cannot use the letter to quietly undo what it may not unset.
		{
			name: "unset -n is refused",
			src:  `v=1; declare -rn r=v; unset -n r; echo "st=$?"; declare -p r`,
			want: "bash: line 1: unset: r: cannot unset: readonly variable\nst=1\ndeclare -nr r=\"v\"\n",
		},
		// An unfrozen reference is untouched by all of it, which is what
		// says these are the freeze's doing and not the branch's.
		{
			name: "an unfrozen reference still unsets",
			src:  `v=1; declare -n r=v; unset -n r; echo "st=$? ${v-GONE}"`,
			want: "st=0 1\n",
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

// The `n` letter **selects** in a listing, exactly as every other attribute
// letter does.
//
// It was the one letter the filter did not read, because this engine keeps
// the record in its own reference table rather than in an attribute one: a
// bare `declare -n` was not a listing at all and wrote nothing at 0, and
// `declare -p -n` fell through to the unfiltered walk and wrote the whole
// variable table — well over a hundred rows where bash writes the references.
//
// Measured 2026-09-16 against bash 5.3.20 over a table holding one reference,
// one integer and one export.
func TestTheReferenceLetterSelectsInAListing(t *testing.T) {
	dir := t.TempDir()
	const table = `v=1; declare -n r=v; declare -i k=5; declare -x e=3; `
	for _, tc := range []struct{ name, src, want string }{
		{"the bare letter lists the references", table + `declare -n`, "declare -n r=\"v\"\n"},
		{"and so does the other spelling", table + `typeset -n`, "declare -n r=\"v\"\n"},
		{"`-p` beside it is the same set", table + `declare -p -n`, "declare -n r=\"v\"\n"},
		{"a named operand is not a filter", table + `declare -pn k`, "declare -i k=\"5\"\n"},
		// Two letters join, which is the reading the other letters already
		// take: `declare -ni` writes the integers and the references alike.
		{
			"two letters join",
			table + `n=$(declare -ni); case $n in *'declare -n r'*) echo has-ref;; esac; case $n in *'declare -i k'*) echo has-int;; esac; case $n in *' e='*) echo has-export;; esac`,
			"has-ref\nhas-int\n",
		},
		// Nothing carrying the letter is an empty listing at 0 rather than
		// the whole table, which is the failure this closes.
		{"no reference lists nothing", `k=1; declare -n; echo "st=$?"`, "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
