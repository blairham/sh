// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strconv"
	"strings"
	"testing"
)

// An array literal that runs out **inside another construct** is still the
// literal's refusal, not the enclosing construct's.
//
// A literal that ran out gives up its line — Parser.giveUpOnTheArray — which
// is what makes it a refused line rather than the file's error, and is where
// bash's status of **1** comes from. Inside a subshell there is then nothing
// left to read, so the `(` fails with its own *unterminated* error, and a
// front end checks the parse error before it checks the line's refusal: the
// inner refusal was discarded and the outer complaint stood in its place, at
// the wrong line, with the wrong sentence and the wrong status.
//
// Measured 2026-09-22 against bash 5.3.20 through `-c`, with the status taken
// from the shell itself rather than from a pipeline — which is a trap worth
// naming, because `$?` after `sh -c … | sed` is sed's and reports every row
// as agreeing:
//
//	( a=(       line 1: unexpected EOF while looking for matching `)'   status 1
//	( a=( ;     line 1: syntax error near unexpected token `;'          status 1
//
// where this shell answered `line 2: syntax error: unexpected end of file
// from `(' command on line 1` at status 2 for both. #4131.
//
// # Two branches, one rule
//
// The second row is the tell that this is not about which *sentence* the
// literal takes: there the refusal is a **token**, not an end of input, and
// it is reported over just the same. What the two share is that the recovery
// had nowhere left to go — the `;` branch reads on looking for the `)` and
// arrives at the end of the input anyway — so the rule is "a refusal given up
// with the input already spent is final", and it is applied in both places.
//
// The enclosing construct is not only `(`: `{`, `if`, `while`, `for` and
// `case` all reported over it, and nesting two deep did too.
func TestAnArrayLiteralsRefusalSurvivesTheConstructAroundIt(t *testing.T) {
	const matching = "unexpected EOF while looking for matching `)'"
	for _, c := range []struct {
		name, src, want string
		line, status    int
	}{
		{"inside a subshell", "( a=(", matching, 1, 1},
		{"inside a brace group", "{ a=(", matching, 1, 1},
		{"inside an if", "if a=(", matching, 1, 1},
		{"inside a while", "while a=(", matching, 1, 1},
		{"inside a for body", "for i in 1; do a=(", matching, 1, 1},
		{"two subshells deep", "( ( a=(", matching, 1, 1},
		{"with elements read", "( a=(1 2", matching, 1, 1},
		// The token branch: a `;` stopped the literal and the recovery then
		// ran out looking for the `)`.
		{
			"a token refusal inside a subshell", "( a=( ;",
			"syntax error near unexpected token `;'", 1, 1,
		},
		// Controls. At the top level both shapes were always right, and a
		// construct that runs out with no literal in it keeps its own
		// grammatical sentence.
		{"the literal alone", "a=(", matching, 1, 1},
		{
			"a subshell alone", "(",
			"syntax error: unexpected end of file from `(' command on line 1", 2, 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, status := runScript(t, "-c", c.src)
			if out != "" {
				t.Errorf("ran %q: wrote %q, want nothing", c.src, out)
			}
			if status != c.status {
				t.Errorf("ran %q: status %d, want %d (said %q)", c.src, status, c.status, errs)
			}
			if !strings.Contains(errs, c.want) {
				t.Errorf("ran %q: said %q, want it to carry %q", c.src, errs, c.want)
			}
			if at := "line " + strconv.Itoa(c.line) + ":"; !strings.Contains(errs, at) {
				t.Errorf("ran %q: said %q, want it blamed on %q", c.src, errs, at)
			}
		})
	}
}
