// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A bare append onto a table that fails partway through keeps the pairs it
// had added, plain pairs included, and drops a key whose value never came;
// an assignment, a declaration's operand and a function's append are the
// same split. Measured 2026-10-15 on bash 5.3.20 from a file, this script,
// byte for byte. We kept nothing on any row (#6704).
func TestAFailedTableAppendKeepsThePairsItAdded(t *testing.T) {
	checkFailedKeep(t, failedTableScript,
		"k=new \n"+
			"j=new k=old \n"+
			"j=new k=old \n"+
			"j=new k=old i=q \n"+
			"j=new k=old i= \n"+
			"k=old \n"+
			"k=old \n"+
			"j=new k=old \n")
}

// A declaration whose literal fails leaves the name as it was, whatever
// container letter it carries. Measured 2026-10-15 on bash 5.3.20 from a
// file, this script, byte for byte (#6705).
func TestAFailedOperandKeepsTheNameWithAnyLetter(t *testing.T) {
	checkFailedKeep(t, failedOperandScript,
		"declare -a v=([0]=\"1\" [1]=\"2\")\n"+
			"declare -A v=([k]=\"old\" )\n"+
			"declare -a v=([0]=\"1\" [1]=\"2\")\n"+
			"declare -A v=([k]=\"old\" )\n"+
			"declare -x v=\"x\"\n"+
			"declare -i v=\"3\"\n"+
			"declare -a v=([0]=\"1\" [1]=\"2\")\n"+
			"declare -a v=([0]=\"1\" [1]=\"2\")\n")
}

// failedTableScript appends to and assigns a table whose literal fails, each
// row in a subshell of its own, and reads the keys back in a fixed order so
// that no row depends on the order a table lists its keys in.
const failedTableScript = "show() { for k in j k i l q; do [ \"${m[$k]+set}\" ] && printf '%s=%s ' \"$k\" \"${m[$k]}\"; done; echo; }\n" +
	"row() { ( typeset -A m; m=([k]=old); trap show EXIT; eval \"$1\" ) 2>/dev/null; }\n" +
	"row 'm+=([k]=new [i]=$((1/0)) [l]=z)'\n" +
	"row 'm+=([j]=$(echo new) [i]=${u?boom})'\n" +
	"row 'm+=(j new i $((1/0)))'\n" +
	"row 'm+=(j new $(echo i) $(echo q) l $((1/0)))'\n" +
	"row 'm+=(j new i $(echo SIDE >&2) $((1/0)))'\n" +
	"row 'typeset -A m+=([j]=new [i]=$((1/0)))'\n" +
	"row 'm=([j]=new [k]=$((1/0)))'\n" +
	"row 'f() { m+=(j new i $((1/0))); }; f'\n"

// failedOperandScript declares a name whose literal fails, each row in a
// subshell of its own, and lists what is left.
const failedOperandScript = "row() { ( trap 'typeset -p v' EXIT; eval \"$1\" ) 2>/dev/null; }\n" +
	"row 'v=(1 2); typeset -a v=(5 $((1/0)))'\n" +
	"row 'typeset -A v=([k]=old); typeset -A v=(j new i $((1/0)))'\n" +
	"row 'v=(1 2); typeset -A v=(5 $((1/0)))'\n" +
	"row 'typeset -A v=([k]=old); typeset -a v=(5 $((1/0)))'\n" +
	"row 'export v=x; typeset -a v=(5 $((1/0)))'\n" +
	"row 'typeset -i v=3; typeset -a v=(5 $((1/0)))'\n" +
	"row 'unset v; typeset -a v=(5 $((1/0)))'\n" +
	"row 'v=(1 2); typeset v=(5 $((1/0)))'\n" +
	"row 'v=(1 2); typeset -a v+=(5 $((1/0)))'\n"

func checkFailedKeep(t *testing.T, src, want string) {
	t.Helper()
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}
