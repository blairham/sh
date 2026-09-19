// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import "github.com/blairham/sh/syntax"

// kshReserves is what [interp.Runner.SetReservedWords] is handed: the
// classification `type` and `command -v` answer with, which is this shell's
// grammar less one word.
//
// `]]` is the whole of the difference, and it is a fact about the *report*
// rather than about what parses: the construct is here, `[[ -n x ]]` runs,
// and the closing word is not a name this shell will own up to. Measured
// 2026-09-18 on ksh93u+ 2012-08-01, script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`:
//
//	word   bash 5.3  zsh 5.9.2  ksh93u+  dash    BusyBox ash
//	]]     `]]`      1          1        127     127
//	[[     `[[`      `[[`       `[[`     127     `[[`
//	in     `in`      1          `in`     `in`    `in`
//
// So `]]` is bash's alone among the shells that have the construct, where
// `[[` is claimed by every one of them — which is what says the two ends of
// one construct are two answers and not a flag either way. zsh declines both
// `]]` and `in` through the same hook and for the same kind of reason, and
// the two remaining columns have neither construct and fall through to the
// search (#2981, #2918).
func kshReserves(d syntax.Dialect) func(string) bool {
	return func(name string) bool {
		if name == "]]" {
			return false
		}
		return d.Reserves(name)
	}
}
