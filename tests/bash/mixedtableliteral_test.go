// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A table literal mixing `[key]=` heads with bare words lets the **first**
// element choose the reading.
//
// Measured 2026-09-23 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C bash f.sh`, standard input on the null device, bash 5.3.20, with the
// table declared in front of each row and the commands newline-separated so an
// abandoned list does not hide what survived:
//
//	a=([zero]=5 [one]=10 four [two]=2)   `a: four: must use subscript when
//	                                     assigning associative array`, 1, and
//	                                     `a` keeps `zero` and `one`
//	b=(one 1 [two]=2 three 3)            0, and `[two]=2` is a literal key
//	k=zz; e=([a]=1 $k)                   names `$k`, the text the source wrote
//	declare f=([a]=1 $k)                 names `'zz'` — a declaration's operand
//	                                     was expanded before the builtin saw it
//
// This shell paired every shape off instead, so the first row left a key `four`
// holding the empty string at status 0 (#4241). It is the table literal's
// question and not the array's: bash mixes freely on an indexed name, which the
// last case here is the control for.
func TestATableLiteralsFirstElementChoosesTheReading(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		refuses         bool
	}{
		{
			name: "a bare element after a head is refused", refuses: true,
			// A newline and not a `;`: the refusal gives up the rest of the
			// command list — measured, and the same in bash — so a `printf`
			// behind a semicolon would never run in either shell.
			src: "declare -A a\na=([zero]=5 [one]=10 four [two]=2)\n" +
				`printf '[%s][%s][%s][%s]' "$?" "${a[zero]}" "${a[one]}" "${#a[@]}"`,
			want: "[1][5][10][2]",
		},
		{
			name: "a head after a bare element is a literal key",
			src:  `declare -A b; b=(one 1 [two]=2 three 3); printf '[%s][%s][%s]' "$?" "${b[one]}" "${b['[two]=2']}"`,
			want: "[0][1][three]",
		},
		{
			name: "an all-bare literal still pairs off",
			src:  `declare -A c; c=(p 1 q 2); printf '[%s][%s][%s]' "$?" "${c[p]}" "${c[q]}"`,
			want: "[0][1][2]",
		},
		{
			name: "an all-subscripted literal is untouched",
			src:  `declare -A d; d=([p]=1 [q]=2); printf '[%s][%s][%s]' "$?" "${d[p]}" "${d[q]}"`,
			want: "[0][1][2]",
		},
		{
			// The control that says this is the table literal's question: an
			// indexed name mixes the two shapes freely in this column.
			name: "an indexed literal mixes freely",
			src:  `e=([2]=c p q); printf '[%s][%s][%s][%s]' "$?" "${e[2]}" "${e[3]}" "${e[4]}"`,
			want: "[0][c][p][q]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) || st != 0 {
				t.Errorf("= %q status %d, want it to end %q", out, st, tc.want)
			}
			said := strings.Contains(out, "must use subscript when assigning associative array")
			if said != tc.refuses {
				t.Errorf("= %q, refusal said %v, want %v", out, said, tc.refuses)
			}
		})
	}
}

// The refusal names the element the **source** wrote, and a declaration's
// operand names the value it came to instead — single-quoted, which is the shape
// of a word the shell expanded before the builtin saw it.
func TestABareTableLiteralElementIsNamedByItsRoute(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an assignment names the text",
			`declare -A h; k=zz; h=([p]=1 $k)`,
			"h: $k: must use subscript",
		},
		{
			"a declaration's operand names the value",
			`declare -A h; k=zz; declare h=([p]=1 $k)`,
			"h: 'zz': must use subscript",
		},
		{
			"and quoting comes off it first",
			`declare -A h; declare h=([p]=1 "x y")`,
			"h: 'x y': must use subscript",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want %q in it", out, tc.want)
			}
		})
	}
}

// An empty key in a table literal is refused in both shapes, and the two cost
// different amounts.
//
// Measured in the same run:
//
//	g=(p 1 "" x q 2)         `"": bad array subscript`, status **0**, that pair
//	                         dropped, `p` and `q` kept
//	h=([p]=1 [""]=x [r]=2)   `[""]=x: bad array subscript`, status **1**, the
//	                         rest of the literal abandoned, `h` keeps `p`
//
// zsh stores the empty key in both shapes, which is what makes this an axis. The
// element route already agreed — `s[""]=v` is `s[""]: bad array subscript` — so
// it was the literal that parted (#4241).
func TestAnEmptyKeyInATableLiteralIsRefusedHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want, said string }{
		{
			"a bare pair is dropped and the literal carries on",
			`declare -A g; g=(p 1 "" x q 2); printf '[%s][%s][%s][%s]' "$?" "${g[p]}" "${g[q]}" "${#g[@]}"`,
			"[0][1][2][2]", `"": bad array subscript`,
		},
		{
			"a head gives up the rest of the literal",
			"declare -A h\nh=([p]=1 [\"\"]=x [r]=2)\n" +
				`printf '[%s][%s][%s]' "$?" "${h[p]}" "${#h[@]}"`,
			"[1][1][1]", `[""]=x: bad array subscript`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) || st != 0 {
				t.Errorf("= %q status %d, want it to end %q", out, st, tc.want)
			}
			if !strings.Contains(out, tc.said) {
				t.Errorf("= %q, want %q in it", out, tc.said)
			}
		})
	}
}

// Both policies, pinned so that no preset here drifts off the column they were
// measured from.
func TestTheTableLiteralPoliciesAreThisDialects(t *testing.T) {
	s := bash.Semantics()
	if got := s.MixedTableLiteral; got != interp.MixedTableLiteralFollowsTheFirstElement {
		t.Errorf("MixedTableLiteral = %v, want follows the first element", got)
	}
	if got := s.EmptyKeyInATableLiteral; got != interp.EmptyKeyInATableLiteralRefused {
		t.Errorf("EmptyKeyInATableLiteral = %v, want refused", got)
	}
}
