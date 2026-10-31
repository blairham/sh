// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestARefusedExpressionStoreSetsADeclaredOnlyArray: an arithmetic store the
// freeze refuses writes nothing, and still takes an array that was only
// declared out of the declared-only set, so the listing gains its `=()` —
// where every other writer's refusal leaves the listing as it was, and so
// does a refusal of the expression for any other reason (#6847). Measured
// 2026-10-31 on bash 5.3.20, `-c`, each listed by an EXIT trap.
func TestARefusedExpressionStoreSetsADeclaredOnlyArray(t *testing.T) {
	for _, c := range []struct{ name, src, listing string }{
		{"(( )) indexed", `declare -a a; readonly a; (( a[5]=5 ))`, "declare -ar a=()"},
		{"let table", `declare -A a; readonly a; let 'a[k]=5'`, "declare -Ar a=()"},
		{"(( ))++ table", `declare -A a; readonly a; (( a[k]++ ))`, "declare -Ar a=()"},
		{"the whole name", `declare -a a; readonly a; (( a=5 ))`, "declare -ar a=()"},
		{"$(( ))", `declare -a a; readonly a; echo $(( a[1]=5 ))`, "declare -ar a=()"},
		{"for (( ))", `declare -a a; readonly a; for (( a[1]=5; 0; )); do :; done`, "declare -ar a=()"},
		{"a local", `f() { local -a a; readonly a; (( a[1]=5 )); declare -p a; }; f; a=g`, "declare -ar a=()\n" +
			`declare -- a="g"`},
		// The controls: every other writer, a refusal for another reason,
		// and an expression that never stores.
		{"the statement", `declare -a a; readonly a; a[5]=5`, "declare -ar a"},
		{"read", `declare -A a; readonly a; read 'a[k]' <<< x`, "declare -Ar a"},
		{"printf -v", `declare -a a; readonly a; printf -v 'a[5]' x`, "declare -ar a"},
		{"a default", `declare -a a; readonly a; : ${a[5]:=x}`, "declare -ar a"},
		{"a declaration's operand", `declare -a a; readonly a; declare 'a[5]=1'`, "declare -ar a"},
		{"a division by zero", `declare -a a; readonly a; (( a[1/0]=5 ))`, "declare -ar a"},
		{"a store not made", `declare -a a; readonly a; (( 0 && (a[1]=5) ))`, "declare -ar a"},
		{"a scalar", `declare a; readonly a; (( a=5 ))`, "declare -r a"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), "trap 'declare -p a' EXIT; "+c.src)
			if !strings.HasSuffix(out, c.listing+"\n") {
				t.Errorf("got %q, want it to end with %q", out, c.listing)
			}
		})
	}
}
