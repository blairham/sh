// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `%N<string<` and `%N>string>` truncate what follows them (#5150). Measured
// 2026-10-01 on zsh 5.9.2 under `-c`; each row is what `print -P` wrote.
func TestPromptTruncation(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ arg, want string }{
		{"start %10<...<truncated at 10%<< Not truncated%3< ...<Not shown", "start ...d at 10 Not truncated ..."},
		{"start %10>...>truncated at 10%>> Not truncated%3> ...>Not shown", "start truncat... Not truncated ..."},
		{"[%5<..<abcdefgh]", "[..gh]"},
		{"[%5>..>abcdefgh]", "[abc.."},
		{"[%5<..<abc]", "[abc]"},
		{"[%3<..<abcdefgh]", "[..]"},
		{"[%2<..<abcdefgh]", "[.."},
		{"[%0<..<abcdefgh]", "[abcdefgh]"},
		{"[%5<<abcdefgh]", "[efgh]"},
		{"[%5<..<ab%10<**<cdefghijklmnop]", "[ab**jklmnop]"},
		{"[%5<..<ab%(?.cdefgh.x)ij]", "[..ij]"},
		{"[%(?.%5<..<abcdefgh.x)ij]", "[..fghij]"},
		{"[%5<..<ab%F{red}cdefgh%f]", "[..\x1b[31mgh\x1b[39m]"},
		{"[%5>..>ab%F{red}cdefgh%f]", "[ab\x1b[31mc..\x1b[39m"},
		{"[%5<..<a%{XYZ%}bcdefgh]", "[..XYZgh]"},
		{"[%5<.%v.<abcdefgh]", "[.%v.]"},
		{"[%5<..<abc]%<<rest", "[abc]rest"},
	} {
		src := "print -rP -- " + shquote(c.arg) + "\n"
		if out, st := runZsh(t, dir, src); out != c.want+"\n" || st != 0 {
			t.Errorf("%q: got %q at %d, want %q", c.arg, out, st, c.want)
		}
	}
}
