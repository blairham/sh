// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestANumberedTildeReadsTheDirectoryStack pins that `~N`, `~+N` and `~-N`
// index the directory stack with the current directory as slot zero, follow
// `pushdminus` for the signed forms, and are refused past the end unless
// `nonomatch` is on (#5656). Through the binary, which has `pushd`. Measured
// 2026-10-03 on zsh 5.9.2 under -f.
func TestANumberedTildeReadsTheDirectoryStack(t *testing.T) {
	const refused = "zsh:1: not enough directory stack entries.\n"
	for _, tc := range []struct {
		src, out, errs string
		status         int
	}{
		{
			`pushd -q a; pushd -q ../b; x=(~0 ~1 ~2 ~+0 ~+1 ~-0 ~-1 ~-2 ~01); print ${x:t}`,
			"b a top b a top a b a\n", "", 0,
		},
		{`pushd -q a; pushd -q ../b; v=~1 w=x:~2; print ${v:t} ${w:t}`, "a top\n", "", 0},
		{
			`pushd -q a; pushd -q ../b; pushd -q ../c; setopt pushdminus; x=(~0 ~1 ~+1 ~-1 ~-0); print ${x:t}`,
			"c b a b c\n", "", 0,
		},
		{`x=(~0); print ${x:t}`, "top\n", "", 0},
		{`pushd -q a; print ~3; print after`, "", refused, 1},
		{`print ~-1; print after`, "", refused, 1},
		{`setopt nonomatch; print ~1 ~-5`, "~1 ~-5\n", "", 0},
	} {
		top := t.TempDir() + "/top"
		src := "mkdir -p " + top + "/a " + top + "/b " + top + "/c; cd " + top + "; " + tc.src
		out, errs, status := runZsh(t, "-fc", src)
		if out != tc.out || errs != tc.errs || status != tc.status {
			t.Errorf("%s\n got %q %q %d\nwant %q %q %d", tc.src, out, errs, status, tc.out, tc.errs, tc.status)
		}
	}
}
