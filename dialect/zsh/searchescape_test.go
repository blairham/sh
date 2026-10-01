// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A subscript search drops the backslash before `$` and a backquote, and
// keeps every other one for the matcher (#5269). Measured 2026-10-01 on zsh
// 5.9.2 (`-f`, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`) with
// both spellings planted; each row is what that shell wrote.
func TestASearchDropsTheEscapeOnlyBeforeADollarOrABackquote(t *testing.T) {
	dir := t.TempDir()
	const plant = "a=('a$b' 'a\\$b' 'a`b' 'a\\`b' 'a{b' 'a\\{b' 'a*b' 'a\\*b')\nh=('a$b' 1 'a\\$b' 2)\n"
	for _, c := range []struct{ name, src, want string }{
		{"(r) before a dollar", `print -rn -- "${a[(r)a\$b]}"`, "a$b"},
		{"(R) before a backquote", "print -rn -- \"${a[(R)a\\`b]}\"", "a`b"},
		{"(i) before a dollar", `print -rn -- "${a[(i)a\$b]}"`, "1"},
		{"(re), exact", `print -rn -- "${a[(re)a\$b]}"`, "a$b"},
		{"(i) on an association", `print -rn -- "${h[(i)a\$b]}"`, "1"},
		{"a doubled backslash before a dollar", `print -rn -- "${a[(r)a\\\$b]}"`, `a\$b`},
		// The controls, which kept their backslash before this and keep it
		// still: a brace a key would have dropped, and a star the matcher
		// reads as an escape.
		{"a brace keeps it", `print -rn -- "${a[(r)a\{b]}"`, `a\{b`},
		{"a star is escaped", `print -rn -- "${a[(r)a\*b]}"`, "a*b"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, plant+c.src+"\n"); out != c.want || st != 0 {
				t.Errorf("got %q at %d, want %q", out, st, c.want)
			}
		})
	}
}
