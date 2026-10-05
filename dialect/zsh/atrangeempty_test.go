// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAQuotedRangeOfThePositionalsThatNamesNothingIsNoWord is #5955: a quoted
// range on `@` that selects nothing is no word, the same as the `(@)argv`
// spelling — so `man ${1:t} "${@[2,-1]}"` with one argument passes no stray
// empty one. Every row measured on zsh 5.9.2 (/opt/homebrew/bin/zsh, -f),
// 2026-10-05; `show` prints the count and then each word.
func TestAQuotedRangeOfThePositionalsThatNamesNothingIsNoWord(t *testing.T) {
	const pre = `show() { printf '%s:' $#; for x in "$@"; do printf '<%s>' "$x"; done; print; }
`
	rows := []struct{ src, want string }{
		{`f() { show "${@[2,-1]}"; }; f a`, "0:"},
		{`f() { show "${@[2,-1]}"; }; f`, "0:"},
		{`f() { show "${@[2,2]}"; }; f a`, "0:"},
		{`f() { show "${@[3,-1]}"; }; f a b`, "0:"},
		{`f() { show "${@[2,1]}"; }; f a b`, "0:"},
		{`f() { show x "${@[2,-1]}" y; }; f a`, "2:<x><y>"},
		{`f() { print -r -- "n=${#${@[2,-1]}}"; }; f a`, "n=0"},
		{`f() { local -a r; r=( "${@[2,-1]}" ); print -r -- "r=$#r"; }; f a`, "r=0"},
		// The cells where the same shell does make one empty word: a range
		// past the end whose far side is further still, and one starting
		// before the first.
		{`f() { show "${@[2,3]}"; }; f a`, "1:<>"},
		{`f() { show "${@[-9,-1]}"; }; f a`, "1:<>"},
		// Controls: a range that selects something, the `(@)argv` spelling,
		// one element past the end, and the joining spelling.
		{`f() { show "${@[2,-1]}"; }; f a b c`, "2:<b><c>"},
		{`f() { show "${(@)argv[2,-1]}"; }; f a`, "0:"},
		{`f() { show "${@[2]}"; }; f a`, "1:<>"},
		{`f() { show "${*[2,-1]}"; }; f a`, "1:<>"},
		{`f() { show "x${@[2,-1]}y"; }; f a`, "1:<xy>"},
	}
	for _, row := range rows {
		out, _ := runZsh(t, t.TempDir(), pre+row.src+"\n")
		if out != row.want+"\n" {
			t.Errorf("%s\n got %q\nwant %q", row.src, out, row.want+"\n")
		}
	}
}
