// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestAFrozenArrayKeepsItsElementsThroughEveryRoute: an element write a freeze
// refuses leaves the element as it was, whichever route made it. `(( ))` and
// `let` printed the refusal and then wrote the element anyway into an indexed
// array, and wrote into a table with no refusal at all; `getopts` refused a
// subscripted name as not a valid identifier and stored it regardless (#6826).
// Measured 2026-10-16 on bash 5.3.20, `-c`.
func TestAFrozenArrayKeepsItsElementsThroughEveryRoute(t *testing.T) {
	for _, c := range []struct{ name, src, refusal, listing string }{
		{"(( )) indexed", `a=(1 2); readonly a; (( a[1]=5 )); declare -p a`, "a: readonly variable", `declare -ar a=([0]="1" [1]="2")`},
		{"let indexed", `a=(1 2); readonly a; let 'a[1]=5'; declare -p a`, "a: readonly variable", `declare -ar a=([0]="1" [1]="2")`},
		{"(( )) table", `declare -A a=([j]=1); readonly a; (( a[k]=5 )); declare -p a`, "a: readonly variable", `declare -Ar a=([j]="1" )`},
		{"(( ))++ table", `declare -A a=([j]=1); readonly a; (( a[k]++ )); declare -p a`, "a: readonly variable", `declare -Ar a=([j]="1" )`},
		{"let table", `declare -A a=([j]=1); readonly a; let 'a[k]=5'; declare -p a`, "a: readonly variable", `declare -Ar a=([j]="1" )`},
		{"a default table", `declare -A a=([j]=1); readonly a; : ${a[k]:=x}; echo after`, "a: readonly variable", ""},
		{"getopts element", `getopts x 'b[1]' -x; echo "st=$? [${b[1]-unset}]"`, "`b[1]': not a valid identifier", "st=1 [unset]"},
		{"getopts key", `declare -A m; getopts x 'm[k]' -x; declare -p m`, "`m[k]': not a valid identifier", "declare -A m"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), c.src)
			if !strings.Contains(out, c.refusal) {
				t.Errorf("got %q, want the refusal %q", out, c.refusal)
			}
			if c.listing == "" {
				if strings.Contains(out, "after") {
					t.Errorf("got %q, want the refusal to end the script", out)
				}
			} else if !strings.HasSuffix(out, c.listing+"\n") {
				t.Errorf("got %q, want it to end with %q", out, c.listing)
			}
		})
	}
}
