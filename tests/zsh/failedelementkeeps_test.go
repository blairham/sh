// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Every store that names a subscript, and the positional parameters, keeps
// what it held when the value would not expand. Each write sits in a
// `{ … } always { … }` so that the failure ends the block and the state after
// it can still be read.
//
// Measured 2026-10-15 on zsh 5.9.2 from a file: every row reports the
// division once and the state after it is `[1,2,3] 3 [abc] [p q] [k old]
// [same]` throughout — an element, a span of elements, a flag group, the
// whole-array spelling, a character and a span of characters of a string, a
// table's key, `1=`, `1=(…)`, `0=`, and an element under `globassign`. Each of
// them stored the empty string or the words that had expanded, and `a[0]=…`
// went on to refuse the subscript after the division had been reported
// (#6579).
func TestAFailedElementWriteKeepsWhatItHeld(t *testing.T) {
	t.Parallel()
	src := "z0=$0\n" +
		"k() { local n=renamed; [[ $1 == $z0 ]] && n=same; shift; " +
		"print -r -- \"[${(j:,:)a}] ${#a} [$v] [$*] [${(kv)m}] [$n]\"; TRY_BLOCK_ERROR=0 }\n" +
		"a=(1 2 3); v=abc; set -- p q; typeset -A m; m[k]=old\n"
	rows := []string{
		"a[1]=$((1/0))", "a[0]=$((1/0))", "a[5]=$((1/0))", "a[1]+=$((1/0))",
		"a[1,2]=$((1/0))", "a[(r)2]=$((1/0))", "a[(r)2,(r)3]=$((1/0))",
		"a[@]=$((1/0))", "v[2]=$((1/0))", "v[2,3]=$((1/0))", "v[(f)1]=$((1/0))",
		"m[k]=$((1/0))", "m[n]=$((1/0))", "m[k]+=$((1/0))",
		"1=$((1/0))", "1=(x $((1/0)))", "0=$((1/0))",
		"setopt globassign; a[1]=$((1/0))",
	}
	var want strings.Builder
	for _, w := range rows {
		src += "{ " + w + " } always { k \"$0\" \"$@\" }\n"
		want.WriteString("division by zero\n[1,2,3] 3 [abc] [p q] [k old] [same]\n")
	}
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	// The location in front of each complaint is the script's name and line,
	// which is not what this is about.
	var got strings.Builder
	for _, line := range strings.SplitAfter(out, "\n") {
		if i := strings.Index(line, ": division by zero"); i >= 0 {
			line = line[i+2:]
		}
		got.WriteString(line)
	}
	if got.String() != want.String() {
		t.Errorf("got\n%s\nwant\n%s", got.String(), want.String())
	}
}

// A span's value is expanded before the elements it replaces are read, so a
// write the value makes is among them. Measured 2026-10-15 on zsh 5.9.2:
// `a=(1 2 3); a[1,2]=$((a[3]=9))` leaves `9 9`. The elements were read
// first, so the write to the third was spliced back out — `9 3` — until the
// value moved ahead of the read to be checked before the store (#6579).
func TestASpanValueIsExpandedBeforeTheElementsAreRead(t *testing.T) {
	t.Parallel()
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		"a=(1 2 3); a[1,2]=$((a[3]=9)); print -r -- \"$a\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if out != "9 9\n" {
		t.Errorf("got %q, want %q", out, "9 9\n")
	}
}
