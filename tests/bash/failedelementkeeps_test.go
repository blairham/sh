// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An element write whose value would not expand stores nothing, over an
// indexed array and over a table, traced or not. Measured 2026-10-15 on bash
// 5.3.20 from a file, one statement a line, with and without `set -x`: each
// row reports the division once and the state line after it is `[1 2] 2
// [unset] [k] [old]` every time, `m[@]=…` included, which is a key like any
// other over a table. Every row here stored the empty string — a new
// element, a new key and an unset name created among them (#6579).
func TestAFailedElementWriteKeepsWhatTheTableAndArrayHeld(t *testing.T) {
	const body = "a=(1 2); declare -A m=([k]=old)\n" +
		"s() { echo \"[${a[*]}] ${#a[@]} [${b-unset}] [${!m[*]}] [${m[*]}]\"; }\n" +
		"a[0]=$((1/0))\ns\n" +
		"a[5]=$((1/0))\ns\n" +
		"a[0]+=$((1/0))\ns\n" +
		"a[1]=$(echo hi)$((1/0))\ns\n" +
		"unset b; b[3]=$((1/0))\ns\n" +
		"m[k]=$((1/0))\ns\n" +
		"m[n]=$((1/0))\ns\n" +
		"m[k]+=$((1/0))\ns\n" +
		"m[@]=$((1/0))\ns\n" +
		"m[@]+=$((1/0))\ns\n"
	for _, traced := range []bool{false, true} {
		src := body
		if traced {
			src = "set -x\n" + src
		}
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
		if err != nil {
			t.Fatal(err)
		}
		var states []string
		complaints := 0
		for _, line := range strings.Split(out, "\n") {
			switch {
			case strings.HasPrefix(line, "["):
				states = append(states, line)
			case strings.HasSuffix(line, `1/0: division by 0 (error token is "0")`):
				complaints++
			}
		}
		want := strings.Repeat("[1 2] 2 [unset] [k] [old]\n", 10)
		if got := strings.Join(states, "\n") + "\n"; got != want || complaints != 10 {
			t.Errorf("traced=%v: %d complaints and states\n%s\nwant 10 and every state the first one; whole output:\n%s",
				traced, complaints, got, out)
		}
	}
}
