// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Inside double quotes a key's `\"` is spent by the quotes, so the key read is
// `a"b`; outside them it is `a\"b`, and a search keeps the backslash either
// way (#5270). Measured 2026-10-01 on zsh 5.9.2 (`-f`, a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`) with both keys planted; each row is
// what that shell wrote.
func TestADoubleQuotedSubscriptSpendsTheEscapedQuote(t *testing.T) {
	dir := t.TempDir()
	const plant = "typeset -A h; h=('a\"b' bare 'a\\\"b' kept 'a$b' D)\na=('a\"b' 'a\\\"b')\n"
	for _, c := range []struct{ name, src, want string }{
		{"in double quotes", `print -rn -- "${h[a\"b]}"`, "bare"},
		{"in an assignment's double quotes", `x="${h[a\"b]}"; print -rn -- "$x"`, "bare"},
		{"in a command substitution's", `print -rn -- "$(print -rn -- "${h[a\"b]}")"`, "bare"},
		// The controls: unquoted keeps it, a search keeps it in quotes too,
		// and the escapes every reading drops still drop.
		{"unquoted", `print -rn -- ${h[a\"b]}`, "kept"},
		{"a search in double quotes", `print -rn -- "${a[(r)a\"b]}"`, `a\"b`},
		{"a dollar in double quotes", `print -rn -- "${h[a\$b]}"`, "D"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, plant+c.src+"\n"); out != c.want || st != 0 {
				t.Errorf("got %q at %d, want %q", out, st, c.want)
			}
		})
	}
}
