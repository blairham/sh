// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// `${a[1][2]}` reads the nested compound `a[1][2]=v` builds.
//
// The write half landed in #2491 and the value it built was reachable only
// through `typeset -p`: the read was a parse error in the very dialect whose
// assignment had made it. Measured 2026-09-15 on ksh93u+ 2012-08-01, `env -i
// PATH=/usr/bin:/bin HOME=<scratch>`, from a script file (#2830).
//
// Each row carries a companion that a wrong reading would pass. The character
// reading the other shell with this grammar has answers `${a[1][1]}` on a
// nested element with a letter rather than an element, and answers a string
// element with a letter where this shell answers nothing — so a table of
// single values would look reasonable under either.
func TestAChainedSubscriptReadsTheNestedCompound(t *testing.T) {
	preset := dialecttest.Preset{
		Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
		Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
	}
	for _, tc := range []struct{ name, src, want string }{
		{
			"the nested array is indexed",
			`a[1]=(p q); printf "[%s]" "${a[1][0]}" "${a[1][1]}" "${a[1][9]}"`,
			"[p][q][]",
		},
		{
			"and a missing one is unset rather than empty",
			`a[1]=(p q); printf "[%s]" "${a[1][1]-none}" "${a[1][9]-none}" "${a[9][0]-none}"`,
			"[q][none][none]",
		},
		{
			"the whole-array spelling names the nested array",
			`a[1]=(p q); printf "[%s]" "${a[1][@]}"; printf "|%s|" "${a[1][*]}"; printf "n=%s" "${#a[1][@]}"`,
			"[p][q]|p q|n=2",
		},
		{
			"a string element answers at the base and nowhere else",
			`a[1]=(p q); a[2]=plain; printf "[%s]" "${a[2][0]}" "${a[2][1]-none}"; printf "n=%s" "${#a[2][@]}"`,
			"[plain][none]n=0",
		},
		{
			"the walk is any depth, and the value the write built comes back",
			`c[1][2][3]=v; printf "[%s]" "${c[1][2][3]}" "${c[1][2]}" "${c[1]}"`,
			"[v][][]",
		},
		{
			"only the first subscript is a key",
			`typeset -A m; m[k]=(x y); printf "[%s]" "${m[k][1]}"; printf "n=%s" "${#m[k][@]}"`,
			"[y]n=2",
		},
		{
			"an operator runs on what the chain found",
			`a[1]=(p q); printf "[%s]" "${a[1][1]#q}" "${#a[1][1]}"`,
			"[][1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{}, tc.src)
			if err != nil || st != 0 {
				t.Fatalf("status %d, err %v: %s", st, err, out)
			}
			if out != tc.want {
				t.Errorf("answered %q, want %q", out, tc.want)
			}
		})
	}
}

// And the other shell with the same grammar keeps its own reading, which is
// the whole reason the reading is an axis.
//
// Measured the same day on zsh 5.9.2: `a=(one two three); ${a[1][2]}` is `n`,
// the second character of the first element, where ksh93 answers empty —
// element 1 there is `two`, a second subscript on a string reaches a nested
// array that is not there, and there is nothing to find. Turning the grammar
// on for ksh93 without a second value would have given it a plausible wrong
// character at status 0.
//
// The half that moves when the axis does is zsh's: this text is one where the
// two readings happen to agree in the ksh column, because a subscript on a
// string there is not read as characters either way. The ksh row is the
// contrast that makes the zsh one mean something, and the reading itself is
// pinned by the table above.
func TestTheOtherChainedReadingCountsCharacters(t *testing.T) {
	for _, tc := range []struct {
		name   string
		preset dialecttest.Preset
		want   string
	}{
		{
			name: "zsh counts through what the link before it named",
			preset: dialecttest.Preset{
				Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
				Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
			},
			want: "[n][]",
		},
		{
			name: "ksh walks into a compound that is not there",
			preset: dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			want: "[][two]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := tc.preset.Combined(t, dialecttest.Base{},
				`a=(one two three); printf "[%s]" "${a[1][2]}" "${a[1][0]}"`)
			if err != nil || st != 0 {
				t.Fatalf("status %d, err %v: %s", st, err, out)
			}
			if out != tc.want {
				t.Errorf("answered %q, want %q", out, tc.want)
			}
		})
	}
}
