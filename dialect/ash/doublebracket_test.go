// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// `[[ ]]` in BusyBox ash is `test` with a closing word (#3409).
//
// Measured 2026-09-16 on BusyBox v1.37.0 in the digest-pinned alpine image
// internal/oracle reaches, under `env -i PATH=/usr/bin:/bin LC_ALL=C`, each
// row in a subshell. Every status below is BusyBox's; `cmd/ash` gave the
// keyword reading — 1, 0, 0, 0, a parse error — on the rows marked with it.
func TestDoubleBracketIsTest(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		// The rows the keyword reading answered differently.
		{"a quoted pattern is still a pattern", `[[ abc == "a*" ]]`, 0},
		{"a single-quoted one too", `[[ abc == 'a*' ]]`, 0},
		{"an operand is split", `s="two words"; [[ $s == "two words" ]]`, 2},
		{"an unquoted empty is no word", `e=; [[ $e == "" ]]`, 2},
		{"< is a redirection", `[[ a < nosuchfile ]]`, 1},
		{"no arithmetic in an operand", `[[ 1+2 -eq 3 ]]`, 2},
		{"a leading zero is decimal", `[[ 010 -eq 8 ]]`, 1},
		{"no expression is false", `[[ ]]`, 1},
		{"test's connective is not one", `[[ a = a -a b = b ]]`, 2},
		{"only the last word closes", `[[ a ]] ]]`, 2},
		{"a quoted closer closes", `[[ a "]]"`, 0},
		{"an unclosed one", `[[ a = a`, 2},
		// The controls, which agreed before and must still.
		{"&& inside", `[[ -n x && -z "" ]]`, 0},
		{"|| inside", `[[ -z x || -z y ]]`, 1},
		{"a negation", `[[ ! -z x ]]`, 0},
		{"a number", `[[ 3 -lt 10 ]]`, 0},
		{"a file", `[[ -e / ]]`, 0},
		{"an escaped star", `x='*'; [[ $x == \* ]]`, 0},
		{"a regex", `[[ abc =~ a.c ]]`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIn(t, "("+tc.src+") 2>/dev/null")
			if st != tc.want {
				t.Errorf("%s: status %d, want %d; output %q", tc.src, st, tc.want, out)
			}
		})
	}
}

func TestDoubleBracketIsABuiltin(t *testing.T) {
	src := `type '[['; v='[['; $v -n x ]]; echo "run=$?"; echo [[ a && b ]] && echo after`
	out, _ := runIn(t, src)
	for _, want := range []string{"[[ is a shell builtin\n", "run=0\n", "[[ a && b ]]\nafter\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}
	// And a `(` after a word is the syntax error BusyBox reports: `unexpected
	// word (expecting ")")`, at 2, before anything in the file runs.
	if parses(t, `[[ ( -n x ) ]]`) {
		t.Error(`[[ ( -n x ) ]] parsed, want a refusal`)
	}
}
