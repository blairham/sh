// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// This dialect has no increment operators, and that is not the same fact as
// refusing the doubled sign. The two characters are read as two unary signs,
// so a name behind them comes back unchanged and nothing behind them is a
// sign that ran out of operand.
//
// Measured 2026-09-17 against dash 0.5.12, a script file, `env -i` with
// LC_ALL=C: `x=5; echo $(( --x ))` writes `5` and `echo $(( -- ))` is
// `arithmetic expression: expecting primary: " -- "` at 2. Ours wrote a
// sentence of its own — `Syntax error: -- is not available in this dialect` —
// for both, which refused an expression this shell evaluates (#3475).
func TestTheDoubledSignIsTwoSignsAndNotAnOperator(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"--x", "5"},
		{"++x", "5"},
		{"- -x", "5"},
		{"-- -x", "-5"},
	} {
		out, st := answersRun(t, `x=5; echo $(( `+tc.src+` ))`)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("$(( %s )): got %q at %d, want %q at 0", tc.src, got, st, tc.want)
		}
	}
}

// And with nothing behind it the sign runs out of operand, which is this
// shell's ordinary missing-operand sentence over the whole expression rather
// than a refusal of the spelling.
func TestADoubledSignWithNothingBehindItRanOutOfOperand(t *testing.T) {
	for _, src := range []string{"--", "++", "1 - --"} {
		out, st := answersRun(t, `echo $(( `+src+` ))`)
		want := `arithmetic expression: expecting primary: " ` + src + ` "`
		if !strings.Contains(out, want) || st != 2 {
			t.Errorf("$(( %s )): got %q at %d, want it to contain %q at 2", src, out, st, want)
		}
	}
}

// A parenthesised group that reads a value and then meets text it can use for
// neither an operator nor a close has a sentence of its own here, which no
// other leftover gets: plain leftover text is `expecting EOF`.
//
// Measured 2026-09-17 against dash 0.5.12: `echo $(( (echo a) ))` is
// `arithmetic expression: expecting ')': " (echo a) "` at 2 and
// `echo $(( (1)x ))` is `expecting EOF`. Ours wrote the parser's own prose,
// `Syntax error: expected ) in arithmetic`, and made it a parse failure
// besides (#3071).
func TestAnUnclosedGroupHasItsOwnSentence(t *testing.T) {
	out, st := answersRun(t, `echo $(( (echo a) ))`)
	want := `arithmetic expression: expecting ')': " (echo a) "`
	if !strings.Contains(out, want) || st != 2 {
		t.Errorf("got %q at %d, want it to contain %q at 2", out, st, want)
	}
	out, st = answersRun(t, `echo $(( (1)x ))`)
	want = `arithmetic expression: expecting EOF: " (1)x "`
	if !strings.Contains(out, want) || st != 2 {
		t.Errorf("got %q at %d, want it to contain %q at 2", out, st, want)
	}
}
