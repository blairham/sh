// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The whitespace half of IFS here is every space character of the C locale
// rather than POSIX's three, so a run of vertical tabs is one separator and
// one at either end of a value is discarded. Measured 2026-09-22 against bash
// 5.3.20; dash, zsh and BusyBox ash give the other answer, and bash 3.2 gives
// it too — this is a change within bash and the dialect is 5.3's.
func TestTheWhitespaceHalfOfIFSIsEverySpaceCharacter(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`IFS=$'\v'; v=$'a\v\vb'; set -- $v; echo "n=$# 1=[$1] 2=[$2]"`, "n=2 1=[a] 2=[b]\n"},
		{`IFS=$'\r'; v=$'a\r\rb'; set -- $v; echo "n=$#"`, "n=2\n"},
		{`IFS=$'\f'; v=$'a\f\fb'; set -- $v; echo "n=$#"`, "n=2\n"},
		{`IFS=$'\v'; v=$'\va\v'; set -- $v; echo "n=$# 1=[$1]"`, "n=1 1=[a]\n"},
		{
			// The same answer through `read`, where the last name takes the
			// remainder of the line with the closing run of IFS whitespace
			// off it. IFS is every space character but the space itself, so
			// the leading spaces survive and the trailing one does too.
			`IFS=$'\t\n\v\f\r'; printf '  line\tb \t\r\f\v\n' | ` +
				`{ read -r v1 v2; echo "v1=[$v1] v2=[$v2]"; }`,
			"v1=[  line] v2=[b ]\n",
		},
	} {
		if out, st := runBash(t, dir, tc.src+"\n"); out != tc.want || st != 0 {
			t.Errorf("%s: said %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A `read` with no names hands REPLY the record as it came, where ksh93 and
// zsh trim it the way a named operand's value is trimmed. Measured 2026-09-22
// against bash 5.3.20 and 3.2.57, which agree.
func TestABareReadKeepsTheRecordWhole(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`printf '  A B  \n' | { read; echo "[$REPLY]"; }`, "[  A B  ]\n"},
		{`printf '\tA B\n' | { read; echo "[$REPLY]"; }`, "[\tA B]\n"},
		{
			// The escapes are still processed, so this is the trim alone.
			`printf 'a\\ b \n' | { read; echo "[$REPLY]"; }`, "[a b ]\n",
		},
		{
			// A name the script wrote is an operand and is trimmed, which is
			// what says the answer belongs to the default name.
			`printf '  A B  \n' | { read x; echo "[$x]"; }`, "[A B]\n",
		},
	} {
		if out, st := runBash(t, dir, tc.src+"\n"); out != tc.want || st != 0 {
			t.Errorf("%s: said %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
