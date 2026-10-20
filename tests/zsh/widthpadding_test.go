// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheWidthFlagPadsInColumns pins `(m)` beside the padding pair: the field
// is measured in columns, the right field keeps every character that starts
// inside it and the left only what fits, a fill does the same, and `(mm)`
// gives every character one column (#5151, a chunk of D04parameter.ztst).
// Measured 2026-10-03 on zsh 5.9.2 under `-f` in a UTF-8 locale.
func TestTheWidthFlagPadsInColumns(t *testing.T) {
	const setup = "LC_ALL=en_US.UTF-8; w=日本語\n"
	for _, tc := range []struct{ src, want string }{
		{`print -r -- "[${(mr:1:)w}]" "[${(mr:3:)w}]" "[${(mr:5:)w}]" "[${(mr:7:)w}]"`, "[日] [日本] [日本語] [日本語 ]\n"},
		{`print -r -- "[${(ml:1:)w}]" "[${(ml:3:)w}]" "[${(ml:5:)w}]" "[${(ml:7:)w}]"`, "[] [語] [本語] [ 日本語]\n"},
		{`v=日本; print -r -- "[${(mr:7::日:)v}]" "[${(ml:8::日:)v}]" "[${(ml:7::日:)v}]"`, "[日本日日] [日日日本] []\n"},
		{`v=ab; print -r -- "[${(mr:5::-::日:)v}]" "[${(ml:5::-::日:)v}]"`, "[ab日-] [-日ab]\n"},
		{`v=日本; print -r -- "[${(ml:3:r:3:)v}]" "[${(mmr:6:)v}]" "[${(mmr:2:)w}]"`, "[ 日本 ] [日本    ] [日本]\n"},
		// The control: without the letter a character is one unit.
		{`v=日本; print -r -- "[${(r:6:)v}]"`, "[日本    ]\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
