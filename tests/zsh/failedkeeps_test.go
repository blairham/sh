// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A table literal that fails partway through keeps nothing, appended or
// assigned, subscripted or plain. Measured 2026-10-15 on zsh 5.9.2 from a
// file, this script, byte for byte (#6704).
func TestAFailedTableAppendKeepsNothing(t *testing.T) {
	checkFailedKeep(t, failedTableScript,
		"k=old \n"+
			"k=old \n"+
			"k=old \n"+
			"k=old \n"+
			"k=old \n"+
			"k=old \n"+
			"k=old \n")
}

// A declaration whose literal fails leaves the name as it was, whatever
// container letter it carries. Measured 2026-10-15 on zsh 5.9.2 from a
// file, this script, byte for byte (#6705).
func TestAFailedOperandKeepsTheNameWithAnyLetter(t *testing.T) {
	checkFailedKeep(t, failedOperandScript,
		"typeset -g -a v=( 1 2 )\n"+
			"typeset -A v=( [k]=old )\n"+
			"typeset -g -a v=( 1 2 )\n"+
			"typeset -A v=( [k]=old )\n"+
			"export v=x\n"+
			"typeset -i v=3\n"+
			"typeset -g -a v=( 1 2 )\n"+
			"typeset -g -a v=( 1 2 )\n")
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
