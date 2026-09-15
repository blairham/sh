// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `declare -n` — a **name reference**, which is a language feature rather
// than an option letter and is why #2553 was split out of #2412.
//
// Measured 2026-09-15 against bash 5.3.15, `env -i` with a scratch HOME and
// no startup files. Every row here was run against the real shell and this
// one side by side; interp/nameref.go holds the mechanism and
// dialect/ksh/nameref_test.go the other spelling.
func TestANameReferenceReadsAndWritesThroughTheName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a read goes through", `v=1; declare -n r=v; echo "[$r]"`, "[1]\n"},
		{"a write goes through", `v=1; declare -n r=v; r=2; echo "[$v]"`, "[2]\n"},
		{"an append goes through", `v=1; declare -n r=v; r+=x; echo "[$v]"`, "[1x]\n"},
		// The target is resolved when the name is *used*, never when the
		// reference is made: the declaration here happens before `v` exists.
		{"the target may not exist yet", `declare -n r=v; v=5; echo "[$r]"`, "[5]\n"},
		{"and may never exist", `declare -n r=nope; echo "[$r]"; echo "st=$?"`, "[]\nst=0\n"},
		// Every operator reads through, which is what resolving at the store
		// rather than at the expansion buys: none of these knows about
		// references.
		{
			"every operator reads through",
			`v=abc; declare -n r=v; echo "${r:1:2} ${#r} ${r/a/Z} ${r^^} ${r:-d}"`,
			"bc 3 Zbc ABC abc\n",
		},
		{"set-ness reads through", `v=1; declare -n r=v; echo "${r+SET}"`, "SET\n"},
		{"and an unset target is unset", `declare -n r=v; echo "${r+SET}x"`, "x\n"},
		{"[[ -v ]] reads through", `v=1; declare -n r=v; [[ -v r ]] && echo yes`, "yes\n"},
		{"arithmetic reads through", `v=1; declare -n r=v; echo $(( r + 1 ))`, "2\n"},
		{"arithmetic writes through", `declare -n r=v; v=5; (( r++ )); echo "[$v]"`, "[6]\n"},
		{"read writes through", `v=1; declare -n r=v; read r <<< "zz"; echo "[$v]"`, "[zz]\n"},
		{"printf -v writes through", `v=1; declare -n r=v; printf -v r %s zz; echo "[$v]"`, "[zz]\n"},
		// Containers, in both directions and by element.
		{
			"an array is read through",
			`v=(a b c); declare -n r=v; echo "${r[1]} ${r[*]} ${#r[@]} ${!r[*]}"`,
			"b a b c 3 0 1 2\n",
		},
		{"an element is written through", `v=(a b c); declare -n r=v; r[1]=Z; echo "${v[*]}"`, "a Z c\n"},
		{"a table is read through", `declare -A m=([k]=1); declare -n r=m; echo "${r[k]}"`, "1\n"},
		// A reference to a reference resolves all the way.
		{"a chain resolves", `v=1; declare -n r=v; declare -n s=r; echo "[$s]"`, "[1]\n"},
		{"and `${!r}` is the end of it", `v=1; declare -n r=v; declare -n s=r; echo "${!s}"`, "v\n"},
		// `${!r}` is the target's *name* and not the double read the same
		// spelling means for an ordinary parameter — a double read would
		// have gone looking for a parameter called `1`.
		{"${!r} is the target name", `v=1; declare -n r=v; echo "${!r}"`, "v\n"},
		{"and the name itself is listed", `v=1; declare -n r=v; echo "${!r@}"`, "r\n"},
		// A reference may be aimed at an element.
		{"a reference to an element reads", `declare -n r="a[2]"; a=(x y z); echo "[$r]"`, "[z]\n"},
		{"and writes", `declare -n r="a[2]"; a=(x y z); r=Q; echo "${a[*]}"`, "x y Q\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// What is **not** a read through the reference: three places where the
// reference itself is the subject.
func TestTheReferenceItselfIsTheSubject(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// The listing writes where the reference points and never what it
		// reaches, which is why declarationOf asks the reference table
		// before any of the stores.
		{"a listing writes the reference", `v=1; declare -n r=v; declare -p r`, `declare -n r="v"` + "\n"},
		{"and the target lists as itself", `v=1; declare -n r=v; declare -p v`, `declare -- v="1"` + "\n"},
		{"a reference with no target lists bare", `declare -n r; declare -p r`, "declare -n r\n"},
		// An assignment to a reference that points nowhere **aims** it.
		{"the first value aims it", `v=1; declare -n r; r=v; echo "[$r]"`, "[1]\n"},
		{"and the listing says so", `v=1; declare -n r; r=v; declare -p r`, `declare -n r="v"` + "\n"},
		// A `for` loop re-points the reference rather than writing through
		// it, which is the same rule from the other side: the loop's
		// assignment is an assignment.
		{
			"a loop re-points it",
			`v=1; declare -n r=v; for r in x y; do :; done; declare -p r; echo "[$v]"`,
			"declare -n r=\"y\"\n[1]\n",
		},
		// `unset` takes the *target* away and `unset -n` takes the
		// reference away.
		{"unset removes the target", `v=1; declare -n r=v; unset r; echo "${v-UNSET}"`, "UNSET\n"},
		{"unset -n removes the reference", `v=1; declare -n r=v; unset -n r; echo "${v-UNSET}"`, "1\n"},
		{
			"and leaves nothing under the name",
			`v=1; declare -n r=v; unset -n r; declare -p r; echo "st=$?"`,
			"bash: line 1: declare: r: not found\nst=1\n",
		},
		// A plain declaration over a reference writes *through* it, which
		// says the letter is what makes the operand a target rather than a
		// value.
		{"a plain declaration writes through", `declare -n r=v; declare r=x; echo "$v"`, "x\n"},
		{"and re-aiming needs the letter", `v=1; declare -n r=v; declare -n r=w; w=2; echo "[$r]"`, "[2]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// `local -n` is what the letter exists for: a function takes the name of a
// variable to fill in. The fresh binding is the whole of what `local` adds —
// the reference belongs to the call, and the caller's own name of the same
// spelling gets itself back on return.
func TestALocalNameReferenceBelongsToTheCall(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a function fills in its caller's variable", `f(){ local -n o=$1; o=filled; }; v=; f v; echo "[$v]"`, "[filled]\n"},
		{"declare -n does the same inside one", `f(){ declare -n o=$1; o=filled; }; v=; f v; echo "[$v]"`, "[filled]\n"},
		{"a target that does not exist is created", `f(){ local -n o=$1; o=filled; }; f nosuch; echo "[$nosuch]"`, "[filled]\n"},
		{"it reaches a caller's local", `f(){ local -n o=$1; o=Z; }; g(){ local m=q; f m; echo "[$m]"; }; g`, "[Z]\n"},
		{"and the call's own local", `f(){ local x=in; local -n o=x; o=set; echo "$x"; }; x=out; f; echo "$x"`, "set\nout\n"},
		// The reference goes away with the call, which is the row that says
		// it is an attribute of the *binding* and not of the name.
		{
			"the reference does not outlive the call",
			`f(){ local -n r=v; }; v=1; r=plain; f; declare -p r`,
			`declare -- r="plain"` + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The two refusals and the one warning, which are where this shell and ksh93
// part company — see Semantics.NamerefCycleIsRefused.
func TestWhatBashSaysAboutABadReference(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			name: "a target that is not a name", src: `declare -n r=1bad; echo "st=$?"`,
			want: "bash: line 1: declare: `1bad': invalid variable name for name reference\nst=1\n",
		},
		{
			name: "a reference straight to itself", src: `declare -n r=r; echo "st=$?"`,
			want: "bash: line 1: declare: r: nameref variable self references not allowed\nst=1\n",
		},
		// A cycle through *another* reference is made here and complained
		// about at the read, which is the axis. The refusal above is the
		// core's answer and is the same in both shells.
		{
			name: "a cycle is made, not refused", src: `declare -n a=b; declare -n b=a; echo "st=$?"`,
			want: "st=0\n",
		},
		{
			name: "and the read is where it is said", src: `declare -n a=b; declare -n b=a; echo "[$a]"`,
			want: "bash: line 1: warning: a: circular name reference\n[]\n",
		},
		// The refusal is not fatal here, which is the other half of the same
		// difference: `typeset` is a special builtin in ksh93 and not in
		// this shell.
		{
			name: "a refusal does not end the script", src: `declare -n r=r; echo after`,
			want: "bash: line 1: declare: r: nameref variable self references not allowed\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}
