// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What this shell will and will not take as an alias **name**, measured on
// ksh93u+ 2012-08-01 on 2026-09-12 by defining `alias '<name>'=echo` for
// every printable ASCII character in turn (#2413).
//
// It refuses everything bash refuses and five more — `* ? [ { }` — which is
// why the set is a value on the vector rather than one axis shared by the two
// shells that check. `]` is in neither set, so the extra five are the pattern
// characters and not a bracket rule.

func runKshAlias(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

func TestAnAliasNameHoldingARefusedCharacter(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		// The complaint names the whole *operand* — value and all — and
		// carries no shell or line in front of it, which is what this shell
		// does for `alias` and almost nowhere else.
		{"a dollar", `alias 'a$b'=echo`, "alias: a$b=echo: invalid alias name\n", 1},
		{"a space", `alias 'a b'=echo`, "alias: a b=echo: invalid alias name\n", 1},
		// The five bash takes.
		{"a star", `alias 'a*b'=echo`, "alias: a*b=echo: invalid alias name\n", 1},
		{"a question mark", `alias 'a?b'=echo`, "alias: a?b=echo: invalid alias name\n", 1},
		{"an open bracket", `alias 'a[b'=echo`, "alias: a[b=echo: invalid alias name\n", 1},
		{"an open brace", `alias 'a{b'=echo`, "alias: a{b=echo: invalid alias name\n", 1},
		{"a close brace", `alias 'a}b'=echo`, "alias: a}b=echo: invalid alias name\n", 1},
		// And `]` is not one of them.
		{"a close bracket is taken", `alias 'a]b'=echo; alias 'a]b'`, "a]b=echo\n", 0},
		// A bare lookup is checked too, which bash does not do: there the
		// same word is answered `not found`.
		{"a lookup is checked", `alias 'a$b'`, "alias: a$b: invalid alias name\n", 1},
		// And the script ends on it. The builtin's own status is 1 in the
		// other checking shell as well, so only the unrun line parts them.
		{"the script ends", `echo one; alias 'a$b'=echo; echo two`, "one\nalias: a$b=echo: invalid alias name\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKshAlias(t, c.src)
			if out != c.want || st != c.status {
				t.Errorf("ran %q: got %q status %d, want %q status %d",
					c.src, out, st, c.want, c.status)
			}
		})
	}
}
