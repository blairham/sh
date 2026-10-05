// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// expand-absolute-path, driven through a session on a pseudo-terminal with
// the shipped file on $fpath (follow-up to #5894). Every expected line was
// measured 2026-10-05 through a pseudo-terminal against /opt/homebrew/bin/zsh
// (zsh 5.9.2) with its own copy, from `~/sub` in a home holding `f`,
// `real/r`, `lnk -> real`, `sp ace`, `sub/dd` and `sub/ace`, with `nd` a named
// directory for `~/other`. The line is read back as $BUFFER|$CURSOR by a
// widget on ^G, which writes it $'…'-quoted — so a backslash in the line is
// two in the expected text. docs/spec/functions.md has the table.
func TestExpandAbsolutePathWritesTheWordUnderTheCursor(t *testing.T) {
	const rc = `HOME=${HOME:A}
mkdir -p ~/sub/dd ~/real ~/other
: > ~/f; : > ~/real/r; : > ~/'sp ace'; : > ~/sub/ace
ln -s real ~/lnk
hash -d nd=~/other
cd ~/sub
autoload -Uz expand-absolute-path; zle -N expand-absolute-path; bindkey '^X^A' expand-absolute-path
`
	const left = "\x02"
	for _, tc := range []struct{ name, keys, want string }{
		{"up a level", "../f", "~/f|3"},
		{"through a link", "../lnk/r", "~/real/r|8"},
		{"a link and then up", "../lnk/../f", "~/f|3"},
		{"the directory itself", ".", "~/sub|5"},
		{"a directory below", "dd", "~/sub/dd|8"},
		{"a named directory", "~nd", "~nd|3"},
		{"the home", "~/f", "~/f|3"},
		{"a doubled slash", "..//f", "~/f|3"},
		{"a pattern, first match, quoted", "../s*", `~/sp\\ ace|9`},
		{"twice is once", "../f\x18\x01", "~/f|3"},
		{"a name that is not there", "nosuch", "nosuch|6"},
		{"a quoted word", `"../f"`, `"../f"|6`},
		{"a backslash", `../sp\ ace`, `../sp\\ ace|10`},
		{"a parameter", "$HOME/f", "$HOME/f|7"},
		{"a brace", "../{f,real}", "../{f,real}|11"},
		{"in the middle of the line", "x ../f extra" + strings.Repeat(left, 7), "x ~/f extra|4"},
		{"at the start of the word", "x ../f" + strings.Repeat(left, 4), "x ~/f|2"},
		{"on the blank before the word", "x ../f" + strings.Repeat(left, 5), "x ../f|1"},
		{"after a trailing blank", "x ../f ", "x ~/f |5"},
		{"on an operator", "a ../f;b" + strings.Repeat(left, 2), "a ../f;b|6"},
		{"before an operator", "a ../f;b" + strings.Repeat(left, 3), "a ~/f;b|4"},
		{"a pipe is a word of its own", "echo ../f|cat" + strings.Repeat(left, 5), "echo ~/f|cat|7"},
		{"an empty line", "", "|0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen, home := contribSession(t, rc)
			contribSend(t, control, tc.keys+"\x18\x01")
			got := contribRecorded(t, control, screen, home, 1)
			got = strings.TrimPrefix(got, "$'")
			if i := strings.LastIndex(got, "'|"); i >= 0 {
				got = got[:i] + got[i+1:]
			}
			if got != tc.want {
				t.Errorf("%q: got %q, want %q", tc.keys, got, tc.want)
			}
		})
	}
}
