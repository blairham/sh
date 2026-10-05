// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `comptags -R` and `-A` before the first `-N` ask about the first set
// (#6172), and `-A` answering 1 assigns nothing. Measured on zsh 5.9.2,
// 2026-10-05, from inside a `zle -C` widget; comptags.go has the table.

func TestComptagsBeforeTheFirstStepAsksTheFirstSet(t *testing.T) {
	requested := `local out= x; for x in a b c; do comptags -R $x; out+="$x=$? "; done`
	for _, c := range []struct{ name, body, want string }{
		{
			// The shape `_tags processes; _requested processes` makes, with
			// no `_arguments` and so an empty context.
			"offered and asked straight away", `comptags -i '' processes; comptry processes; comptags -T
			 comptags -R processes; say "R=$?"`, "R=0",
		},
		{
			"before any -N", `comptags -i '' a b c; comptry a; comptry b c; ` + requested + `; say "$out"`,
			"a=0 b=1 c=1 ",
		},
		{
			"the first -N lands on the same set",
			`comptags -i '' a b c; comptry a; comptry b c; comptags -N; ` + requested + `; say "$out"`,
			"a=0 b=1 c=1 ",
		},
		{
			"the second steps on", `comptags -i '' a b c; comptry a; comptry b c; comptags -N; comptags -N; ` +
				requested + `; say "$out"`,
			"a=1 b=0 c=0 ",
		},
		{
			"a named context the same", `comptags -i ctx a b c; comptry b; ` + requested + `; say "$out"`,
			"a=1 b=0 c=1 ",
		},
		{
			// `-A` reads the same set, and a 1 leaves both names alone.
			"-A before any -N, then once more, then another tag",
			`comptags -i '' a b c; comptry a b; comptry c
			 local ct=C0 sp=S0 out=; comptags -A a ct sp; out+="$? $ct $sp|"
			 ct=C1 sp=S1; comptags -A a ct sp; out+="$? $ct $sp|"
			 ct=C2 sp=S2; comptags -A c ct sp; out+="$? $ct $sp"
			 say "$out"`,
			"0 a a|1 C1 S1|1 C2 S2",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := reported(t, c.body, "x "); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
