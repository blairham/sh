// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestPrintfReadsItsOwnWay: three printf answers BusyBox ash 1.37.0 does not
// share with dash, measured 2026-10-03 in the pinned image. A `%b` with a
// field is no conversion at all; an integer conversion reads a quoted
// character behind blanks; and a conversion it refuses is named by the rest
// of the format as written.
func TestPrintfReadsItsOwnWay(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf '[%5b][%s]\n' ab cd; echo "st=$?"`, "%5b][%s]\\n: invalid format\n[st=1\n"},
		{`printf '[%b]\n' ab; echo "st=$?"`, "[ab]\nst=0\n"},
		{`printf '%d %x\n' " 'A" "	'A"; echo "st=$?"`, "65 41\nst=0\n"},
		{`printf 'a%kb\n'; echo "st=$?"`, "%kb\\n: invalid format\nast=1\n"},
	} {
		out, _ := run(t, tc.src)
		if !strings.HasSuffix(out, tc.want) {
			t.Errorf("%s\n got %q\nwant it to end %q", tc.src, out, tc.want)
		}
	}
}
