// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An element write whose value would not expand stores nothing either: the
// element keeps what it held, an element that was not there is not created,
// and a name that was unset stays unset. The scalar store's rule (#6544), for
// a store that names a subscript.
//
// Measured 2026-10-15, one statement a line: `a=(1 2)` then `a[0]=$((1/0))`
// leaves `[1 2] 2` in bash 5.3.20 under `-c`, a file and `-i`, and in ksh93u+
// 2012-08-01 and zsh 5.9.2 run interactively, the one route where those two
// reach the next line. So do `a[5]=…`, `a[0]+=…` and `a[1]=$(echo
// hi)$((1/0))`, and `unset b; b[3]=…` leaves `b` unset. Every one of them
// stored the empty string (#6579). The table's rows, the traced rows and the
// shapes only one grammar has are in the dialect packages.
func TestAFailedElementWriteKeepsWhatTheElementHeld(t *testing.T) {
	t.Parallel()
	const show = "echo \"st=$? [${a[*]}] ${#a[@]} [${b-unset}]\"\n"
	const want = "st=1 [1 2] 2 [unset]\n"
	for _, c := range []struct{ name, src string }{
		{"an element is kept", "a[0]=$((1/0))\n"},
		{"no element is added", "a[5]=$((1/0))\n"},
		{"an element is not joined", "a[0]+=$((1/0))\n"},
		{"a partial value is not stored", "a[1]=$(echo hi)$((1/0))\n"},
		{"an unset name is not created", "unset b\nb[3]=$((1/0))\n"},
	} {
		out, _ := assignFailRun(t, "a=(1 2)\n"+c.src+show, Yes, Yes)
		if !strings.HasSuffix(out, want) {
			t.Errorf("%s: said %q, want it to end %q", c.name, out, want)
		}
	}
}
