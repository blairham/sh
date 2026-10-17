// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAnArithmeticSubscriptsDollarSingleIsDecoded is the Yes side of
// interp.Semantics.ArithSubscriptDollarSingleIsDecoded: the key is the
// quotation's decoded text, where this stored `$x` (#6773). Measured
// 2026-10-16 on bash 5.3.20, `-c`.
func TestAnArithmeticSubscriptsDollarSingleIsDecoded(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`declare -A a; (( a[$'x']=1 )); declare -p a`, "declare -A a=([x]=\"1\" )\n"},
		{`declare -A a; (( a[$'x\ty']=1 )); declare -p a`, "declare -A a=([$'x\\ty']=\"1\" )\n"},
	} {
		out, _ := runBash(t, t.TempDir(), c.src)
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.src, out, c.want)
		}
	}
}
