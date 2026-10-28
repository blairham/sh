// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// `read -a` and `mapfile` filling a frozen array through a reference name
// the reference as written, once; `mapfile` asks the freeze before the kind
// and before any `-C` callback runs (#6887).
//
// Measured 2026-10-27 against GNU bash 5.3.20 under `-c`.
func TestAFrozenArrayFilledThroughAReferenceNamesTheReference(t *testing.T) {
	const frozen = "a=(1 2); readonly a; typeset -n r=a; "
	for _, c := range []struct{ probe, want string }{
		{frozen + `read -a r <<< x; echo st=$?`, "bash: line 1: r: readonly variable\nst=1\n"},
		{frozen + `mapfile -t r <<< x; echo st=$?`, "bash: line 1: r: readonly variable\nst=1\n"},
		{frozen + `readarray -t r <<< x; echo st=$?`, "bash: line 1: r: readonly variable\nst=1\n"},
		{frozen + `mapfile -O 1 -t r <<< x; echo st=$?`, "bash: line 1: r: readonly variable\nst=1\n"},
		{frozen + `mapfile -t -C 'echo cb' -c 1 r <<< x; echo st=$?`, "bash: line 1: r: readonly variable\nst=1\n"},
		{frozen + `typeset -n s=r; mapfile -t s <<< x; echo st=$?`, "bash: line 1: s: readonly variable\nst=1\n"},
		{frozen + `typeset -n s=r; read -a s <<< x; echo st=$?`, "bash: line 1: s: readonly variable\nst=1\n"},
		// The controls: a scalar read through the reference names the
		// target, and the array named directly is named directly.
		{frozen + `read r <<< x; echo st=$?`, "bash: line 1: a: readonly variable\nst=1\n"},
		{frozen + `read -a a <<< x; echo st=$?`, "bash: line 1: a: readonly variable\nst=1\n"},
		{frozen + `mapfile -t a <<< x; echo st=$?`, "bash: line 1: a: readonly variable\nst=1\n"},
		// A frozen table is refused by the freeze, the kind never reached.
		{`declare -A b; readonly b; mapfile -t b <<< x; echo st=$?`, "bash: line 1: b: readonly variable\nst=1\n"},
		{`declare -A b; mapfile -t b <<< x; echo st=$?`, "bash: line 1: mapfile: b: not an indexed array\nst=1\n"},
		// And an unfrozen target behind the reference is filled.
		{`a=(1 2); typeset -n r=a; mapfile -t r <<< x; read -a r <<< 'p q'; declare -p a`, "declare -a a=([0]=\"p\" [1]=\"q\")\n"},
	} {
		got, _ := posixRouteStatus(t, c.probe, interp.RouteCommandString)
		if got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.probe, got, c.want)
		}
	}
}
