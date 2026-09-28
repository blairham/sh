// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A declaration of **one element** over a local the same call already made
// (#5106).
//
// The refusal `can't create local array elements` answers the question of
// whether a subscripted operand may *create* the local. It was being asked
// where there is nothing to create: the call already has the name, and the
// element goes into the local that is standing, exactly as a bare
// `array[1]=a` statement would put it there.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
func TestAnElementDeclaredOverALocalTheCallAlreadyMade(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"the issue's own line",
			"f(){ local -a array; typeset array[1]=a array[2]=b; print $array }\nf\n", "a b\n",
		},
		{"one element", "f(){ local -a array; typeset array[1]=a; print $array }\nf\n", "a\n"},
		{"under declare", "f(){ local -a array; declare array[1]=a; print $array }\nf\n", "a\n"},
		{
			"the name the local was declared with under the other word",
			"f(){ typeset -a array; typeset array[1]=a array[2]=b; print $array }\nf\n", "a b\n",
		},
		{
			"an element of a local that already holds one",
			"f(){ local -a array=(x y z); typeset array[2]=B; print $array }\nf\n", "x B z\n",
		},
		{"an associative local", "f(){ local -A h; typeset h[k]=v h[j]=w; print \"$h[k] $h[j]\" }\nf\n", "v w\n"},
		{"a scalar local", "f(){ local s; typeset s[1]=a; print \"[$s]\" }\nf\n", "[a]\n"},
		{
			"the element keeps the local's scope",
			"f(){ local -a array; typeset array[1]=a; print -r -- \"${(t)array}\" }\nf\n", "array-local\n",
		},
		{
			"and the listing says so",
			"f(){ local -a array; typeset array[1]=a; typeset -p array }\nf\n", "typeset -a array=( a )\n",
		},
		{
			"the element is gone when the call returns",
			"f(){ local -a array; typeset array[1]=a }\nf\n" + `print -r -- "[$array] ${(t)array}"` + "\n", "[] \n",
		},
		// A brace group is not a scope, so it does not change the answer.
		{
			"inside a brace group",
			"f(){ local -a array; { typeset array[1]=a }; print \"[$array]\" }\nf\n", "[a]\n",
		},
		// Two operands and two statements reach the same local.
		{
			"a second declaration of another element",
			"f(){ local -a array; typeset array[1]=a; typeset array[2]=b; print \"[$array]\" }\nf\n", "[a b]\n",
		},
		{
			"a subscript that is a parameter",
			"f(){ local -a array; local i=1; typeset array[i]=a; print \"[$array]\" }\nf\n", "[a]\n",
		},
		{
			"a quoted operand",
			"f(){ local -a array; typeset 'array[1]'=a; print \"[$array]\" }\nf\n", "[a]\n",
		},
		{
			"an element past the end",
			"f(){ local -a array; typeset array[5]=e; print \"${#array}\" }\nf\n", "5\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// **The refusal is right everywhere it was right before**, which is what says
// this is about the local and not about the element. Each of these was already
// agreeing and has to go on agreeing: the question the axis asks is whether a
// subscripted operand may *create* the local, and in every row below there is
// one to create.
func TestTheLocalElementRefusalStillFiresWhereThereIsALocalToMake(t *testing.T) {
	dir := t.TempDir()
	const refused = "f:typeset: array[1]: can't create local array elements\n"
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"nothing has made the name local", "f(){ typeset array[1]=a }\nf\n", refused, 1},
		{
			"a global is not a local of this call",
			"array=(1 2)\nf(){ typeset array[1]=X }\nf\n", refused, 1,
		},
		{
			"and the local belongs to the caller",
			"g(){ typeset array[1]=a }\nf(){ local -a array; g }\nf\n",
			"g:typeset: array[1]: can't create local array elements\n", 1,
		},
		// The two controls from the other side: no scope at all, and the bare
		// statement in the same place, which were taken before and after.
		{"at the top level there is no scope", "local -a array\ntypeset array[1]=a\nprint $array\n", "a\n", 0},
		{
			"a bare statement reaches the local",
			"f(){ local -a array; array[1]=a; print $array }\nf\n", "a\n", 0,
		},
		// `-g` says outright that no local is being made, and was right
		// already.
		{
			"the global letter asks for no local",
			"f(){ local -a array; typeset -g array[1]=a; print $array }\nf\n", "a\n", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != tc.status {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// The other three refusals a declared element runs into are unmoved, and the
// frozen name now reaches the one it should: with nothing to create, the write
// goes on to the store and the freeze answers it.
func TestTheOtherElementRefusalsAreUnmoved(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"the container letter",
			"f(){ local -a array; typeset -a array[1]=a }\nf\n",
			"f:typeset: array[1]: inconsistent type for assignment\n",
		},
		{
			"the readonly letter",
			"f(){ local -a array; typeset -r array[1]=a }\nf\n",
			"f:typeset: array[1]: can't create readonly array elements\n",
		},
		{
			"the integer letter",
			"f(){ local -a array; typeset -i array[1]=3+4 }\nf\n",
			"f:typeset: array[1]: inconsistent array element or slice assignment\n",
		},
		{
			"and a frozen local answers with its own sentence",
			"f(){ local -ar array=(1); typeset array[1]=X }\nf\n",
			"f: read-only variable: array\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 1 {
				t.Errorf("out %q status %d, want %q at 1", out, st, tc.want)
			}
		})
	}
}
