// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// rangeDiags is a vector that says enough about a refused range for its text
// to be readable: the arithmetic sentence, and the range's own frame around it.
func rangeDiags() Diagnostics {
	return Diagnostics{
		ArithError:            "%[1]s: %[2]s",
		ArithOperandExpected:  "arithmetic syntax error",
		ArithExpressionRanOut: "arithmetic syntax error",
		ArithOperatorExpected: "arithmetic syntax error",
		SubstringRangeError:   "%[2]s",
	}
}

func rangeSem() Semantics {
	s := PosixSemantics()
	s.SubstringRangeReadsModifiers = No
	s.SubstringRangeQuotesPatternCharacters = No
	s.SubstringNegativeLengthIsEmpty = No
	s.SubstringRangeThirdColonIsABadSubstitution = No
	return s
}

func runBlamedRange(t *testing.T, src string, set func(*Runner)) string {
	t.Helper()
	out, _ := run(t, src, set)
	return out
}

// What a refused range is quoted back *with*. Not the text the reader saw:
// bash 5.3 and ksh93u+ both name `'&&'` with its quotes still on, so the
// complaint shows what the script wrote, before quote and backslash removal
// (#2818).
//
// The `$i` row is what says this is not simply the source text. A parameter
// the range read from is expanded first and its value carries no quoting of
// the script's, so that row is blamed as `&&`.
func TestASubstringRangeIsBlamedAsTheScriptWroteIt(t *testing.T) {
	set := func(r *Runner) {
		sem := rangeSem()
		diag := rangeDiags()
		r.Semantics, r.Diagnostics = &sem, &diag
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a quoted offset keeps its quotes", `x=abc; echo "[${x:'&&'}]"`, "'&&'"},
		{"a backslash offset keeps its backslashes", `x=abc; echo "[${x:\&\&}]"`, `\&\&`},
		{"a quoted length too", `x=abc; echo "[${x:1:'&&'}]"`, "'&&'"},
		{"and a value read from a parameter is its value", `x=abc; i='&&'; echo "[${x:$i}]"`, "&&"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := runBlamedRange(t, tc.src, set)
			if want := "sh: " + tc.want + ": arithmetic syntax error"; !strings.Contains(out, want) {
				t.Errorf("got %q, want it to name %q", out, want)
			}
		})
	}
}

// One complaint per range. A range whose *offset* was refused evaluates no
// length at all, which is unanimous: `${x:&&:&&}` is one line in bash 5.3 and
// one in ksh93u+, and we wrote the offset's and then the length's.
func TestARefusedOffsetEvaluatesNoLength(t *testing.T) {
	set := func(r *Runner) {
		sem := rangeSem()
		diag := rangeDiags()
		r.Semantics, r.Diagnostics = &sem, &diag
	}
	out := runBlamedRange(t, `x=abc; echo "[${x:&&:&&}]"`, set)
	if n := strings.Count(out, "arithmetic syntax error"); n != 1 {
		t.Errorf("got %q — %d complaints, want one", out, n)
	}
	// And the flag is about *this* range's offset rather than about the
	// command: a range whose offset was fine still reports its length.
	out = runBlamedRange(t, `x=abc; echo "[${x:1:&&}]"`, set)
	if n := strings.Count(out, "arithmetic syntax error"); n != 1 {
		t.Errorf("got %q — %d complaints, want the length's one", out, n)
	}
}

// Where the range is cut. One dialect does not read a colon written straight
// after the parameter's own as a separator: it belongs to the offset's
// expression, so `${v::2}` is the expression `:2` and a refusal.
func TestASubstringOffsetCanTakeALeadingColon(t *testing.T) {
	for _, tc := range []struct {
		name    string
		takes   bool
		src     string
		want    string
		refused bool
	}{
		{"an empty offset and a length", false, `v=oldoldold; echo "[${v::2}]"`, "[ol]", false},
		{"or one expression beginning with a colon", true, `v=oldoldold; echo "[${v::2}]"`, ":2", true},
		{"an empty range", false, `v=oldoldold; echo "[${v::}]"`, "[]", false},
		{"or the expression `:`", true, `v=oldoldold; echo "[${v::}]"`, ":", true},
		// Three segments, where the two readings are two different
		// *expressions* rather than a refusal against a slice: an empty
		// offset leaves `1:2` as the length, and the other reading makes
		// the whole of `:1:2` the offset.
		{"three segments blame the length", false, `v=oldoldold; echo "[${v::1:2}]"`, "1:2", true},
		{"or the whole of it", true, `v=oldoldold; echo "[${v::1:2}]"`, ":1:2", true},
		{"a written offset is unaffected", true, `v=oldoldold; echo "[${v:1:2}]"`, "[ld]", false},
		{"and so is the default-value operator", true, `unset v; echo "[${v:-D}]"`, "[D]", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runGrammar(t, tc.src, func(d *syntax.Dialect) {
				d.ParamSubstring = true
				d.ParamSubstringOffsetTakesALeadingColon = tc.takes
			}, func(r *Runner) {
				sem := rangeSem()
				diag := rangeDiags()
				diag.SubstringErrorNamesTheWholeRange = true
				r.Semantics, r.Diagnostics = &sem, &diag
			})
			want := tc.want
			if tc.refused {
				want = "sh: " + tc.want + ": arithmetic syntax error"
			}
			if !strings.Contains(out, want) {
				t.Errorf("got %q, want %q in it", out, want)
			}
		})
	}
}

// A range with a *third* segment, where the modifier reading has declined it:
// one column reads the rest as an expression and complains about the
// arithmetic, and one refuses the substitution outright.
func TestASubstringRangeWithAThirdColonIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name string
		bad  Answer
		want string
	}{
		{"refused as a substitution", Yes, "bad substitution"},
		{"or read as an expression", No, "arithmetic syntax error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `x=abcdefgh; echo "[${x:1:5:9}]"`, func(r *Runner) {
				sem := rangeSem()
				sem.SubstringRangeThirdColonIsABadSubstitution = tc.bad
				diag := rangeDiags()
				diag.BadSubstitution = "%[1]s: bad substitution"
				r.Semantics, r.Diagnostics = &sem, &diag
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
		})
	}

	// And the axis is asked only where a third segment is written, so an
	// ordinary range under an unanswered vector runs as it always did.
	t.Run("a two-segment range asks nothing", func(t *testing.T) {
		out, _ := run(t, `x=abcdefgh; echo "[${x:1:2}]"`, func(r *Runner) {
			sem := rangeSem()
			sem.SubstringRangeThirdColonIsABadSubstitution = Unspecified
			diag := rangeDiags()
			r.Semantics, r.Diagnostics = &sem, &diag
		})
		if out != "[bc]\n" {
			t.Errorf("got %q, want the slice", out)
		}
	})
}
