// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A write to one element of an attributed array leaves the **other** elements
// as they are. The fold belongs to the value being written; an element
// standing in the array does not go through the attribute again because
// something happened elsewhere in the same name.
//
// Every failing row below zeroed the elements the write never touched, at
// status 0 with nothing said — `typeset -i b=5` over a filled array is what a
// script writes to make a counter array arithmetic, and it silently threw the
// counters away (#3888).
//
// Measured 2026-09-20 on bash 5.3.20, script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME.
func TestAnElementWriteLeavesTheOtherElementsAlone(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"a declaration carrying a value writes one element",
			`b=(p q r); typeset -i b=5; echo "[${b[*]}]"`,
			"[5 q r]\n",
		},
		{
			// The same store reached without a declaration, which is what
			// says the fold was the write's and not the letter's.
			"an element assignment likewise",
			`b=(p q r); typeset -i b; b[0]=5; echo "[${b[*]}]"`,
			"[5 q r]\n",
		},
		{
			"and one past the end",
			`b=(p q r); typeset -i b; b[5]=9; echo "[${b[*]}]"`,
			"[p q r 9]\n",
		},
		{
			"an appending literal folds only the words it adds",
			`b=(p q r); typeset -i b; b+=(7); echo "[${b[*]}]"`,
			"[p q r 7]\n",
		},
		{
			// #3897's row, filed from the other side while #3881 was being
			// fixed: a `readonly` that no longer freezes uncovered the write
			// that used to be refused before it could say anything.
			"a write mid-array leaves both its neighbors",
			`b=(x y z); typeset -i b; b[1]=3+4; echo "[${b[*]}]"`,
			"[x 7 z]\n",
		},
		{
			// A case letter asks the same question and was answered the same
			// way, which is what says this is the store and not `-i`.
			"a case letter over an element write",
			`b=(p q r); typeset -u b; b[0]=zz; echo "[${b[*]}]"`,
			"[ZZ q r]\n",
		},
		// The controls.
		{
			// A literal replaces the name's contents, so every element in it
			// is a value the write introduces and every one folds — even
			// where the words are the ones the name was already holding.
			"the control: a literal folds every word",
			`typeset -i b; b=(p q r); echo "[${b[*]}]"`,
			"[0 0 0]\n",
		},
		{
			"the control: and folds them again when they are the same words",
			`b=(p q r); typeset -i b; b=(p q r); echo "[${b[*]}]"`,
			"[0 0 0]\n",
		},
		{
			"the control: the value written still goes through the attribute",
			`typeset -i b=(1 2 3); b[1]=4+4; echo "[${b[*]}]"`,
			"[1 8 3]\n",
		},
		{
			"the control: so does an appended word",
			`typeset -i b=(1 2); b+=(3+3); echo "[${b[*]}]"`,
			"[1 2 6]\n",
		},
		{
			// #3897's own control, and it is the reason this survived
			// anything that only exercised numeric arrays: the fold is
			// invisible where every element already evaluates to itself.
			"the control: an array of numbers cannot show the fold",
			`c=(1 2 3); typeset -i c; c[0]=9; echo "[${c[*]}]"`,
			"[9 2 3]\n",
		},
		{
			// The other store that replaces the name's contents, and the row
			// that says so: every word `read -a` puts there is a value it
			// introduces, so every one folds. Measured on bash 5.3.20.
			"the control: a read that fills the name folds every word",
			`typeset -i a; read -a a <<< "1+1 2+2"; echo "[${a[*]}]"`,
			"[2 4]\n",
		},
		{
			"the control: the valueless declaration reaches no element here",
			`b=(p q r); typeset -i b; echo "[${b[*]}]"`,
			"[p q r]\n",
		},
		{
			// The keyed spelling was right throughout, because its store
			// folds the one value it writes and never walks the table. It is
			// the shape the indexed side takes now, so it must not move.
			"the control: a table's element write was always this",
			`typeset -A m=([k]=v [j]=w); typeset -i m; m[k]=5; typeset -p m`,
			"declare -Ai m=([j]=\"w\" [k]=\"5\" )\n",
		},
		{
			"the control: the listing of the declared row",
			`b=(p q r); typeset -i b=5; typeset -p b`,
			"declare -ai b=([0]=\"5\" [1]=\"q\" [2]=\"r\")\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}
