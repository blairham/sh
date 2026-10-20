// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An optional numbered argument leaves the rest specification standing at
// its own position (#6151). `_kill` is `1::signal:…` and `*:…:->processes`,
// so with the rest left out `kill <TAB>` offered signals and no process.
//
// Measured 2026-10-05 against zsh 5.9.2 from a completion widget calling the
// builtin, `-D`'s tags at `cmd <TAB>` and `cmd w1 <TAB>`:
//
//	1:a  *:r            argument-1             argument-rest
//	1::a *:r            argument-1 argument-rest   argument-rest
//	1::a 2:b            argument-1             argument-2
//	1::a 2::b *:r       argument-1 argument-rest   argument-2 argument-rest
//	1::a 2:b *:r        argument-1 argument-rest   argument-2
//	1:a  2::b *:r       argument-1             argument-2 argument-rest
//
// so it is the optional numbered argument at the position that lets the rest
// in, and never the next numbered one.
func TestAnOptionalArgumentLeavesTheRestStanding(t *testing.T) {
	for _, c := range []struct{ specs, line, want string }{
		{`'1:a:(x)' '*:r:(y)'`, "cmd ", "argument-1"},
		{`'1::a:(x)' '*:r:(y)'`, "cmd ", "argument-1 argument-rest"},
		{`'1::a:(x)' '2:b:(z)'`, "cmd ", "argument-1"},
		{`'1::a:(x)' '2::b:(z)' '*:r:(y)'`, "cmd ", "argument-1 argument-rest"},
		{`'1::a:(x)' '2:b:(z)' '*:r:(y)'`, "cmd ", "argument-1 argument-rest"},
		{`'1:a:(x)' '2::b:(z)' '*:r:(y)'`, "cmd ", "argument-1"},
		{`'1:a:(x)' '*:r:(y)'`, "cmd w1 ", "argument-rest"},
		{`'1::a:(x)' '2::b:(z)' '*:r:(y)'`, "cmd w1 ", "argument-2 argument-rest"},
		{`'1::a:(x)' '2:b:(z)' '*:r:(y)'`, "cmd w1 ", "argument-2"},
	} {
		t.Run(c.specs+"@"+c.line, func(t *testing.T) {
			body := "local -a descrs actions subcs; comparguments -i '' : " + c.specs +
				`; comparguments -D descrs actions subcs; compadd -QU -- "${subcs[*]}"`
			got := completionFor(t, widgetOf(body), c.line)
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("tags %q, want %q", got, c.want)
			}
		})
	}
}
