// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// An array that exists and holds no elements is **unset** to the `-`/`+`
// test here. Measured 2026-09-16 on bash 5.3.20 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a script file with stdin
// closed, and on bash 3.2.57 beside it, which answers the same (#2298).
//
// zsh is the column that reads it the other way, and this shell was giving
// that answer — see Semantics.EmptyArrayIsSet.

func runEmptyArray(t *testing.T, src string) (string, string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", src})
	return out.String(), errs.String(), code
}

func TestAnArrayWithNoElementsIsUnset(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the plus test", `e=(); echo "[${e[@]+S}]"`, "[]\n"},
		{"the minus test", `e=(); echo "[${e[@]-D}]"`, "[D]\n"},
		{"the star spelling", `e=(); echo "[${e[*]+S}]"`, "[]\n"},
		{"declared and never assigned", `declare -a e; echo "[${e[@]+S}]"`, "[]\n"},
		{"a keyed table", `declare -A m=(); echo "[${m[@]+S}]"`, "[]\n"},
		// The controls, where the panel agrees: one empty element is set,
		// an array with elements is set, and the colon form fires anyway.
		{"one empty element", `e=(""); echo "[${e[@]+S}]"`, "[S]\n"},
		{"with elements", `e=(x); echo "[${e[@]-D}]"`, "[x]\n"},
		{"the colon form", `e=(); echo "[${e[@]:-D}]"`, "[D]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := runEmptyArray(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.src, out, errs, st, tc.want)
			}
		})
	}
}
