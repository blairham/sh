// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The whole-array spelling is the **typed characters** and nothing else. A
// subscript that merely *expands* to `@` is an expression over an indexed
// array and a key over a table, and neither reading is the whole array.
//
// The store has said so since #3878 — `i=@; a[$i]=Z` is an arithmetic refusal
// here, which is what `Runner.assignWholeArraySubscript` reads `IndexText`
// for — and the read was still asking what the subscript came out as. Every
// row below answered `p q r` at status 0, silently, where bash refuses and
// gives up the rest of the line (#3889).
//
// Measured 2026-09-20 on bash 5.3.20, script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME.
func TestOnlyATypedWholeArraySubscriptNamesEveryElement(t *testing.T) {
	t.Parallel()
	const decl = `a=(p q r); K=@; `
	const bad = "sh: line 1: @: arithmetic syntax error: operand expected (error token is \"@\")\n"
	for _, c := range []struct{ name, src, want string }{
		{
			"an expanded subscript is an expression",
			decl + `echo "[${a[$K]}]"` + "\n" + `echo "next=$?"`,
			bad + "next=1\n",
		},
		{
			"the braced spelling likewise",
			decl + `echo "[${a[${K}]}]"`,
			bad,
		},
		{
			"and a substitution that produces the characters",
			`a=(p q r); echo "[${a[$(echo @)]}]"`,
			bad,
		},
		{
			// The blanks alone are enough, which is what says a trim cannot
			// stand in for this reading: bash answers `@ : arithmetic syntax
			// error` for the same line, naming the spacing it was given.
			"blanks around the characters take the spelling away",
			`a=(p q r); echo "[${a[ @ ]}]"`,
			"sh: line 1: @ : arithmetic syntax error: operand expected (error token is \"@ \")\n",
		},
		{
			"the star spelling asks the same question",
			`a=(p q r); K=*; echo "[${a[$K]}]"`,
			"sh: line 1: *: arithmetic syntax error: operand expected (error token is \"*\")\n",
		},
		{
			// A table reads it as a key rather than as arithmetic, and says
			// so by finding the one `m[@]=Z` stored — where this shell
			// joined every value and answered `Z v`.
			"over a table an expanded subscript is the key",
			`typeset -A m=([k]=v); m[@]=Z; K=@; echo "[${m[$K]}]"; echo "st=$?"`,
			"[Z]\nst=0\n",
		},
		// The controls. The typed spellings must not move.
		{
			"the control: the typed subscript is the array",
			`a=(p q r); echo "[${a[@]}][${a[*]}]"; printf '<%s>' "${a[@]}"; echo`,
			"[p q r][p q r]\n<p><q><r>\n",
		},
		{
			"the control: the count and the keys read it too",
			`a=(p q r); echo "${#a[@]} [${!a[@]}]"`,
			"3 [0 1 2]\n",
		},
		{
			"the control: the quoted spelling was already an expression",
			`a=(p q r); echo "[${a["@"]}]"`,
			bad,
		},
		{
			"the control: a table's typed subscript is every value",
			`typeset -A m=([k]=v); m[@]=Z; echo "[${m[@]}]"`,
			"[Z v]\n",
		},
		{
			"the control: an ordinary expanded subscript still indexes",
			`a=(p q r); K=1; echo "[${a[$K]}]"`,
			"[q]\n",
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
