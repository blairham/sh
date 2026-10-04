// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A conversion this shell does not have is named by the rest of the format
// from its `%`, as written, with the length modifiers of that one directive
// left out and nothing else touched.
//
// Measured 2026-10-04 in the digest-pinned alpine image, BusyBox v1.37.0,
// each row from a script file (#5723).
func TestAnInvalidFormatNamesTheRestOfTheFormat(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf "[%q]\n" x`, `%q]\n: invalid format`},
		{`printf "[%5q]" a`, `%5q]: invalid format`},
		{`printf "%zq-x" a`, `%q-x: invalid format`},
		{`printf "a%5.2lzb\n" x`, `%5.2b\n: invalid format`},
		{`printf "%ld%jd-y" 1 2`, `%jd-y: invalid format`},
		{`printf "[%zX][%jd][%lld][%hhd]\n" 255 42 42 42`, `%jd][%lld][%hhd]\n: invalid format`},
		{`printf "%5.2lz" 1`, `%5.2: invalid format`},
		{`printf`, `usage: printf FORMAT [ARGUMENT...]`},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := run(t, tc.src+"\n")
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s said %q, want %q in it", tc.src, out, tc.want)
			}
		})
	}
}
