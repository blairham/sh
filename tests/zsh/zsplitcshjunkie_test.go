// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheShellWordSplitReadsOnAfterASecondUnclosedQuote pins that under
// `cshjunkiequotes` the `(z)` split reads the words after a second quote that
// never closes as words, as it does after the first (#5151, a chunk of
// D04parameter.ztst). Measured 2026-10-03 on zsh 5.9.2 under `-f`.
func TestTheShellWordSplitReadsOnAfterASecondUnclosedQuote(t *testing.T) {
	const f = "setopt cshjunkiequotes\n"
	for _, tc := range []struct{ src, want string }{
		{`b=$'\' s\n" d\nword'; printf '<%s>' "${(@Z+n+)b}"; echo`, "<' s><\" d><word>\n"},
		{"b=$'\\' s\\n\" d\\n` b\\nword w2'; printf '<%s>' \"${(@Z+n+)b}\"; echo", "<' s><\" d><` b><word><w2>\n"},
		{`b=$'a \' s\nb " d\nc'; printf '<%s>' "${(@z)b}"; echo`, "<a><' s><;><b><\" d><;><c>\n"},
		// The control: one unclosed quote was already read on from.
		{"b=$'` b\\nword'; printf '<%s>' \"${(@Z+n+)b}\"; echo", "<` b><word>\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), f+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
