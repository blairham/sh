// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An array literal that fails partway through stops expanding there and stores
// nothing, whatever its shape: plain, subscripted, a declaration's operand,
// a table's. Measured 2026-10-15 on bash 5.3.20 from a file, this script, byte
// for byte apart from the location in front of each complaint. The elements
// after the failure ran here, and a declaration's operand reported the
// failure twice (#6606, #6608).
func TestAFailedSubscriptedLiteralStoresNothing(t *testing.T) {
	src := "row() { ( trap 'declare -p a m 2>/dev/null' EXIT; eval \"$1\" ); }\n" +
		"row 'a=(1 2); a=(5 $(echo SIDE >&2) $((1/0)) $(echo AFTER >&2))'\n" +
		"row 'a=(1 2); typeset -a a=(5 $(echo SIDE >&2) $((1/0)) $(echo AFTER >&2))'\n" +
		"row 'a=(1 2); a=([0]=x [5]=y [1]=$((1/0)) [2]=$(echo AFTER >&2))'\n" +
		"row 'a=(1 2); a=([$((1/0))]=v)'\n" +
		"row 'typeset -a a=(1 2); a=([0]=x [1]=$((1/0)))'\n" +
		"row 'a=(1 2); typeset a=([0]=x [1]=$((1/0)))'\n" +
		"row 'a=(1 2); a+=([5]=x [6]=$((1/0)))'\n" +
		"row 'a=(1 2); a=(x [3]=$((1/0)))'\n" +
		"row 'typeset -A m; m=([k]=old); m=([j]=new [k]=$((1/0)))'\n"
	want := "SIDE\n" +
		"ERR\n" +
		"declare -a a=([0]=\"1\" [1]=\"2\")\n" +
		"SIDE\n" +
		"ERR\n" +
		"declare -a a=([0]=\"1\" [1]=\"2\")\n" +
		"ERR\n" +
		"declare -a a=([0]=\"1\" [1]=\"2\")\n" +
		"ERR\n" +
		"declare -a a=([0]=\"1\" [1]=\"2\")\n" +
		"ERR\n" +
		"declare -a a=([0]=\"1\" [1]=\"2\")\n" +
		"ERR\n" +
		"declare -a a=([0]=\"1\" [1]=\"2\")\n" +
		"ERR\n" +
		"declare -a a=([0]=\"1\" [1]=\"2\")\n" +
		"ERR\n" +
		"declare -a a=([0]=\"1\" [1]=\"2\")\n" +
		"ERR\n" +
		"declare -A m=([k]=\"old\" )\n"
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if got := elementOrderNormalized(out); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
