// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// TestARefusedStoreUnderAFrozenTablesKeyLeavesTheKeyDeclared: a store the
// freeze refuses under a table's key writes nothing, and leaves the key
// behind as a valueless declaration — by every writer, and only under a key
// that was not there (#6847). Measured 2026-10-31 on ksh93u+ 2012-08-01,
// `-c`, each listed by an EXIT trap after `typeset -A a; readonly a`.
func TestARefusedStoreUnderAFrozenTablesKeyLeavesTheKeyDeclared(t *testing.T) {
	const keyed = "typeset -r -A a=([k]=)"
	for _, c := range []struct{ name, src, listing string }{
		{"let", `let 'a[k]=5'`, keyed},
		{"let's +=", `let 'a[k]+=5'`, keyed},
		{"(( ))", `(( a[k]=5 ))`, keyed},
		{"a default", `: ${a[k]:=x}`, keyed},
		{"the statement", `a[k]=5`, keyed},
		{"the statement's +=", `a[k]+=x`, keyed},
		{"the statement's empty value", `a[k]=`, keyed},
		{"a computed key", `x=k; a[$x]=v`, keyed},
		{"a declaration's operand", `typeset 'a[k]=1'`, keyed},
		{"export's operand", `export 'a[k]=1'`, keyed},
		{"read", `read 'a[k]' <<< x`, keyed},
		{"read's second name", `read b 'a[k]' <<< '1 2'`, keyed},
		{"read -A", `read -A 'a[k]' <<< x`, keyed},
		{"getopts", `getopts x 'a[k]' -x`, keyed},
		{"a reference", `typeset -n r='a[k]'; r=5`, keyed},
		{"in a function", `function f { a[k]=1; }; f`, keyed},
		{"read's key is its text", `x=k; read 'a[$x]' <<< v`, "typeset -r -A a=(['$x']=)"},
		{"read's empty key", `read 'a[]' <<< v`, "typeset -r -A a=(['']=)"},
		// The controls: a literal is refused whole and unset writes nothing,
		// so neither leaves a key.
		{"a literal", `a+=([k]=5)`, "typeset -r -A a=()"},
		{"unset", `unset 'a[k]'`, "typeset -r -A a=()"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "trap 'typeset -p a' EXIT; typeset -A a; readonly a; " + c.src
			out, _ := runKsh(t, t.TempDir(), src)
			if !strings.Contains(out, "is read only") {
				t.Errorf("got %q, want the refusal", out)
			}
			if !strings.HasSuffix(out, c.listing+"\n") {
				t.Errorf("got %q, want it to end with %q", out, c.listing)
			}
		})
	}
}

// TestARefusedStoreKeepsAKeyThatWasThere: the key left behind never takes a
// value back down to nothing, and a key the refused store did not name is
// left alone (#6847). Measured 2026-10-31 on ksh93u+ 2012-08-01.
func TestARefusedStoreKeepsAKeyThatWasThere(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(),
		"trap 'typeset -p a' EXIT; typeset -A a=([j]=1); readonly a; let 'a[j]=5'; a[k]=5")
	if want := "typeset -r -A a=([j]=1 [k]=)\n"; !strings.HasSuffix(out, want) {
		t.Errorf("got %q, want it to end with %q", out, want)
	}
}

// TestARefusedStoresKeyIsDeclaredAheadOfTheRefusal: the key is worked out,
// and left, before the freeze is asked — so under `set -x` the empty key's
// warning comes ahead of the refusal, whichever writer it is (#6847).
// Measured 2026-10-31 on ksh93u+ 2012-08-01.
func TestARefusedStoresKeyIsDeclaredAheadOfTheRefusal(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"the statement", `a[""]=5`},
		{"read", `read 'a[]' <<< v`},
		{"getopts", `getopts x 'a[]' -x`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "typeset -A a; readonly a\nset -x\n" + c.src + "\n"
			out, _ := runKsh(t, t.TempDir(), src)
			warn := strings.Index(out, "adding empty subscript")
			refused := strings.Index(out, "is read only")
			if warn < 0 || refused < 0 || warn > refused {
				t.Errorf("got %q, want the empty key's warning ahead of the refusal", out)
			}
		})
	}
}

// TestARefusedStoreIntoAFrozenIndexedArrayLeavesItAsItWas: the same writers,
// refused by the same freeze, leave an indexed array exactly as it was — the
// key trace above is keyed on the table, and an indexed array is not one.
//
// This is **not** ksh93u+'s answer, and is not claimed to be. Measured
// 2026-10-31, `-c`, ksh93u+ 2012-08-01: every one of these leaves a counted
// element no subscript lists — `${#a[@]}` is 3, `typeset -p` switches to
// `([0]=1 [1]=2)` — which is the "allocated, unset" slot docs/spec/semantics.md
// records under ArithKeyReadCreatesTheElement and does not model, now reached
// by a third route (#7186). bash 5.3.20 and zsh 5.9.2 leave the array as it
// was, and so does this shell; the rows pin that, so that the table's key
// cannot quietly start being left in an indexed array as a real element.
func TestARefusedStoreIntoAFrozenIndexedArrayLeavesItAsItWas(t *testing.T) {
	const unchanged = "typeset -r -a a=(1 2)\nn=2\n"
	for _, c := range []struct{ name, src string }{
		{"read", `read 'a[5]' <<< x`},
		{"read -A", `read -A 'a[5]' <<< x`},
		{"getopts", `getopts x 'a[5]' -x`},
		{"let", `let 'a[5]=5'`},
		{"(( ))", `(( a[5]=5 ))`},
		{"a default", `: ${a[5]:=x}`},
		{"the statement", `a[5]=5`},
		{"a declaration's operand", `typeset 'a[5]=1'`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "trap 'typeset -p a; echo n=${#a[@]}' EXIT; a=(1 2); readonly a; " + c.src
			out, _ := runKsh(t, t.TempDir(), src)
			if !strings.Contains(out, "is read only") {
				t.Errorf("got %q, want the refusal", out)
			}
			if !strings.HasSuffix(out, unchanged) {
				t.Errorf("got %q, want it to end with %q", out, unchanged)
			}
		})
	}
}
