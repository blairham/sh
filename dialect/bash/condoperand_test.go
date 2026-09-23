// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// A token standing where a conditional operator wanted a word.
//
// bash is the one dialect that words this as a statement about the *operator*
// that was waiting rather than about the token it met, and it says which kind
// of operator — which is the whole of what varies within the sentence. Before
// this it shared one message of our own that matched nobody: `expected an
// operand after ==` (#826).
//
// The sentence is a **preamble**, not the whole refusal: bash writes it at the
// `[[`'s line and then the ordinary `near` and the echoed line under it, the
// same three-line shape every other token refused inside a condition gets.
// Measured 2026-09-22 on bash 5.3.20 under `-c`; before #4173 this shell wrote
// the first line and stopped, which cost `cond.tests` six lines.
func TestAConditionalOperandIsNamedTheOperatorsWay(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		// A bare group, which this dialect has nowhere.
		{"[[ $k == (a|b) ]]", []string{
			"line 1: unexpected argument `(' to conditional binary operator",
			"line 1: syntax error near `('",
			"line 1: `[[ $k == (a|b) ]]'",
		}},
		{"[[ $k = (a|b) ]]", []string{
			"line 1: unexpected argument `(' to conditional binary operator",
		}},
		{"[[ $k != (a|b) ]]", []string{
			"line 1: unexpected argument `(' to conditional binary operator",
		}},
		// The unary family says so.
		{"[[ -n (a|b) ]]", []string{
			"line 1: unexpected argument `(' to conditional unary operator",
		}},
		// And it is about the token rather than about groups.
		{"[[ $k == | ]]", []string{
			"line 1: unexpected argument `|' to conditional binary operator",
			"line 1: syntax error near `|'",
			"line 1: `[[ $k == | ]]'",
		}},
		// An operator with nothing behind it at all names the closer as the
		// argument it would not take.
		{"[[ -n ]]", []string{
			"line 1: unexpected argument `]]' to conditional unary operator",
			"line 1: syntax error near `]]'",
			"line 1: `[[ -n ]]'",
		}},
		{"[[ 4 > & ]]", []string{
			"line 1: unexpected argument `&' to conditional binary operator",
			"line 1: syntax error near `&'",
			"line 1: `[[ 4 > & ]]'",
		}},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Errorf("%s: parsed, want a syntax error", tc.src)
			continue
		}
		out := bash.Diagnostics().ParseDiagnostic("bash", "-c", err, tc.src)
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: said %q, want a line ending %q", tc.src, out, w)
			}
		}
	}
}

// TestAConditionTermOfOneWordExpectsABinaryOperator — a token standing behind
// a condition term of one bare word stood where a binary operator could have,
// and bash words a refusal there as a statement about the operator it was
// still waiting for. It is the same sentence it writes for a newline in the
// position, which is why it is the same field.
//
// The control is a term the operator already settled: `[[ -n x y ]]` gets the
// ordinary conditional wording, so the two sentences are chosen by the shape
// of the term rather than by the token. Measured 2026-09-22 on bash 5.3.20.
func TestAConditionTermOfOneWordExpectsABinaryOperator(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"[[ 4 & ]]", "line 1: unexpected token `&', conditional binary operator expected"},
		{"[[ -Q 7 ]]", "line 1: unexpected token `7', conditional binary operator expected"},
		{"[[ x 7 ]]", "line 1: unexpected token `7', conditional binary operator expected"},
		{"[[ x ; ]]", "line 1: unexpected token `;', conditional binary operator expected"},
		{"[[ x | ]]", "line 1: unexpected token `|', conditional binary operator expected"},
		{"[[ x && y & ]]", "line 1: unexpected token `&', conditional binary operator expected"},
		{"[[ -n x && y z ]]", "line 1: unexpected token `z', conditional binary operator expected"},
		// The controls: a term whose operator has already decided its shape,
		// and a second operator where an operand was wanted.
		{"[[ -n x y ]]", "line 1: syntax error in conditional expression: unexpected token `y'"},
		{"[[ a == b == c ]]", "line 1: syntax error in conditional expression: unexpected token `=='"},
	} {
		if got := condDiagnostic(t, tc.src); !strings.Contains(got, tc.want) {
			t.Errorf("%s: said %q, want a line ending %q", tc.src, got, tc.want)
		}
	}
}

// TestAConditionGroupNamesTheCloserItWanted — a refusal falling where a
// group's `)` was wanted names that closer in the same sentence as the token,
// and the group is then **not** also counted among the ones still open. A
// refusal inside one of the group's terms is the other reading: the sentence
// is the term's and the group is listed.
//
// Measured 2026-09-22 on bash 5.3.20 under `-c`. This shell used to answer
// every one of these with one line of its own — `expected ) in a condition` —
// which was a sentence no shell in the panel writes (#4173).
func TestAConditionGroupNamesTheCloserItWanted(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{"[[ ((1 -eq 1) ]]", []string{
			"line 1: unexpected token `]]', expected `)'",
			"line 1: syntax error near `]]'",
			"line 1: `[[ ((1 -eq 1) ]]'",
		}},
		{"[[ ( -n x ; ]]", []string{
			"line 1: unexpected token `;', expected `)'",
			"line 1: syntax error near `;'",
		}},
		// The input running out where the closer was wanted takes the same
		// sentence and then the unterminated-condition line, not the `]]`
		// preamble a `[[` with no group around it gets.
		{"[[ ( -t X", []string{
			"line 1: unexpected token `EOF', expected `)'",
			"line 2: syntax error: unexpected end of file from `[[' command on line 1",
		}},
		// One line per *enclosing* group under it, the innermost being named
		// in the sentence instead.
		{"[[ ( ( -t X", []string{
			"line 1: unexpected token `EOF', expected `)'",
			"line 1: expected `)'",
			"line 2: syntax error: unexpected end of file from `[[' command on line 1",
		}},
		// And a token refused *inside* the group keeps the term's sentence,
		// with the group listed as still open.
		{"[[ ( x & ) ]]", []string{
			"line 1: unexpected token `&', conditional binary operator expected",
			"line 1: expected `)'",
			"line 1: syntax error near `&'",
		}},
	} {
		out := condDiagnostic(t, tc.src)
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: said %q, want a line ending %q", tc.src, out, w)
			}
		}
	}
}
