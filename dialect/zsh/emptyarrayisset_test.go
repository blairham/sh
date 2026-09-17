// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An array that exists and holds no elements **is** a set parameter here,
// which is the answer the other three columns with arrays do not give.
// Measured 2026-09-16 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, from a script file with stdin closed (#2298).
//
// It is the far side of Semantics.EmptyArrayIsSet, and it is worth a suite
// of its own because this shell's answer was the one every dialect was
// giving.

func runZshEmptyArray(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

func TestAnArrayWithNoElementsIsSet(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the plus test", `e=(); echo "[${e[@]+S}]"`, "[S]\n"},
		{"the minus test", `e=(); echo "[${e[@]-D}]"`, "[]\n"},
		{"the star spelling", `e=(); echo "[${e[*]+S}]"`, "[S]\n"},
		// The controls the panel agrees on.
		{"with elements", `e=(x); echo "[${e[@]-D}]"`, "[x]\n"},
		{"the colon form", `e=(); echo "[${e[@]:-D}]"`, "[D]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshEmptyArray(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
