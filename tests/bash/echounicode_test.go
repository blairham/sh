// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `echo -e` reads \uHHHH and \UHHHHHHHH in 5.3.15, and a run with no digit
// after it is left as written — the other half of the pair zsh answers the
// other way. Measured 2026-09-10, bytes read back through `od`.
func TestEchoReadsTheUnicodeEscapes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`echo -e 'a\u0041Z'`, "aAZ\n"},
		{`echo -e 'a\U00000041Z'`, "aAZ\n"},
		{`echo -e 'a\u00e9Z'`, "a\u00e9Z\n"},
		{`echo -e '\ud800'`, "\xed\xa0\x80\n"},
		{`echo -e 'a\uZ'`, "a" + `\uZ` + "\n"},
		{`echo -e 'a\xZ'`, "a" + `\xZ` + "\n"},
		{`echo 'a\u0041Z'`, "a" + `\u0041Z` + "\n"},
	} {
		if out, st := runBash(t, dir, tc.src+"\n"); out != tc.want || st != 0 {
			t.Errorf("%s: said % x status %d, want % x and 0", tc.src, out, st, tc.want)
		}
	}
}
