// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `builtin` reads no options here — Semantics.BuiltinReadsOptions says so, and
// `builtin -q` and `builtin -- echo hi` are each `no such builtin:` the word —
// and it still **eats a lone dash**, which is what this shell does to one
// given to any of its builtins.
//
// Two rules rather than one, and they are separately falsifiable: a shell
// could read options and not eat the dash, which is bash, or eat the dash and
// not read options, which is this. Ours looked the dash up as a builtin's name
// and answered `no such builtin: -` (#3472).
//
// Measured 2026-09-18 on zsh 5.9.2 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, one probe at a time.
func TestALoneDashGivenToBuiltinIsEaten(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"and the word behind it runs", "builtin - echo x", "x\n", 0},
		{"alone it is a silent success", "builtin -", "", 0},
		// The eating repeats while the word is a lone dash, which is what
		// parts it from a single consumed option: `builtin - -` is silent at
		// 0 there rather than `no such builtin: -`.
		{"and it repeats", "builtin - -", "", 0},
		{"nesting reaches the name", "builtin - builtin - echo y", "y\n", 0},
		// Neither a bundle nor the terminator is read here, so both are
		// names this shell has not got.
		{"a bundle is a name", "builtin -q", "zsh:1: no such builtin: -q\n", 1},
		{"and so is the terminator", "builtin -- echo hi", "zsh:1: no such builtin: --\n", 1},
		// And the builtin it does reach still speaks for itself, which is
		// what says the dash was eaten rather than the call abandoned.
		{"the builtin it reaches speaks", "builtin - shift 5", "zsh:shift:1: shift count must be <= $#\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src+"\n")
			if out != c.want || st != c.status {
				t.Errorf("%s = %q at %d, want %q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}
