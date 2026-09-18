// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// firstSpanOf returns the first span of the first argument of the only
// command of src, which is where every probe below puts the `$`.
func firstSpanOf(t *testing.T, src string, d syntax.Dialect) syntax.Span {
	t.Helper()
	cmd, ok := onlyCommand(t, src, d).(*syntax.SimpleCmd)
	if !ok {
		t.Fatalf("%q is not a simple command", src)
	}
	if len(cmd.Args) < 2 || len(cmd.Args[1].Spans) == 0 {
		t.Fatalf("%q has no operand to read", src)
	}
	return cmd.Args[1].Spans[0]
}

// A line continuation written between a `$` and what it introduces is removed
// and the `$` introduces what stands behind it. The core reaches every form
// across the pair, in both quotings.
//
// Measured 2026-09-16 from script files, `env -i PATH=/usr/bin:/bin LC_ALL=C`:
// bash 5.3, bash 3.2 and dash read every shape below through the pair, in both
// quotings, and zsh 5.9.2 reads every one of them outside quotes. Before, the
// `$` was left as text in every dialect and the pair was removed behind it, so
// `echo [$\⏎{x}]` printed `[${x}]` where the whole panel prints `[5]` (#3457).
//
// This is the construct's *delimiter* rather than its inside, which is what
// makes it a different question from the continuation inside `${ }` or inside
// an arithmetic expression: the pair stands in front of the character that
// says which construct this is.
func TestADollarReachesTheFormBehindALineContinuation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src string
		kind      syntax.SpanKind
		value     string
	}{
		{"a braced parameter", "echo $\\\n{x}", syntax.ParamExp, "x"},
		{"a bare parameter", "echo $\\\nx", syntax.ParamExp, "x"},
		{"a special parameter", "echo $\\\n@", syntax.ParamExp, "@"},
		{"a command substitution", "echo $\\\n(echo hi)", syntax.CommandSubst, "echo hi"},
		{"arithmetic", "echo $\\\n((1+2))", syntax.ArithSubst, "1+2"},
		{"two of them", "echo $\\\n\\\n{x}", syntax.ParamExp, "x"},

		{"a braced parameter in double quotes", "echo \"$\\\n{x}\"", syntax.ParamExp, "x"},
		{"a bare parameter in double quotes", "echo \"$\\\nx\"", syntax.ParamExp, "x"},
		{"a command substitution in double quotes", "echo \"$\\\n(echo hi)\"", syntax.CommandSubst, "echo hi"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := firstSpanOf(t, tc.src, syntax.Core())
			if sp.Kind != tc.kind || sp.Value != tc.value {
				t.Errorf("%q is a %v holding %q, want a %v holding %q",
					tc.src, sp.Kind, sp.Value, tc.kind, tc.value)
			}
		})
	}
}

// The floor. A `$` reaches a *form*, so a pair with nothing a `$` introduces
// behind it leaves the `$` as text — `"$\⏎ x"` is `$ x` and `"$\⏎"` is `$` in
// bash 5.3, bash 3.2, dash, zsh 5.9.2 and ksh93u+ alike. These are the rows a
// change that simply deleted the pair and forgot about it would break.
func TestADollarWithNoFormBehindTheContinuationStaysText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"a blank behind it", "echo \"$\\\n x\"", "$ x"},
		{"the end of the word behind it", "echo \"$\\\n\"", "$"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := firstSpanOf(t, tc.src, syntax.Core())
			if sp.Kind != syntax.Literal || sp.Value != tc.want {
				t.Errorf("%q is a %v holding %q, want a literal holding %q",
					tc.src, sp.Kind, sp.Value, tc.want)
			}
		})
	}
}

// ContinuationStopsADollarAt leaves the `$` as text at the forms it names, and
// leaves every other form alone. Asked per quoting, because the two fields are
// what a dialect that splits by quoting needs and a test that moved them
// together could not tell one from the other.
func TestContinuationStopsADollarAtTheFormsItNames(t *testing.T) {
	t.Parallel()
	bare := syntax.Core()
	bare.ContinuationStopsADollarAt = syntax.DollarBareParameter
	quoted := syntax.Core()
	quoted.ContinuationStopsADollarAtInDoubleQuotes = syntax.DollarBraces

	for _, tc := range []struct {
		name, src string
		d         syntax.Dialect
		kind      syntax.SpanKind
		value     string
	}{
		// Stopped at a bare parameter outside quotes: the `$` is text and
		// the pair behind it is removed by whatever removes one, so the word
		// reads `$x`.
		{"stopped at a bare parameter", "echo $\\\nx", bare, syntax.Literal, "$x"},
		// The forms the set does not name are untouched by it.
		{"a brace the set does not name", "echo $\\\n{x}", bare, syntax.ParamExp, "x"},
		// And the set is read per quoting: the same stop is not in force
		// inside double quotes here.
		{"the same shape in double quotes", "echo \"$\\\nx\"", bare, syntax.ParamExp, "x"},

		{"stopped at a brace in double quotes", "echo \"$\\\n{x}\"", quoted, syntax.Literal, "${x}"},
		{"the same shape outside them", "echo $\\\n{x}", quoted, syntax.ParamExp, "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := firstSpanOf(t, tc.src, tc.d)
			if sp.Kind != tc.kind || sp.Value != tc.value {
				t.Errorf("%q is a %v holding %q, want a %v holding %q",
					tc.src, sp.Kind, sp.Value, tc.kind, tc.value)
			}
		})
	}
}

// DollarGoesWhenAContinuationStopsItAtABrace drops the `$` instead of leaving
// it as text, and only where the stop is at a brace.
//
// The two rows are the discrimination the flag exists for: one dialect answers
// `{x}` for `"$\⏎{x}"` and another answers `${x}`, from the same stop, so the
// flag cannot be folded into the set that made it.
func TestTheDollarGoesWhereTheContinuationStoppedItAtABrace(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.ContinuationStopsADollarAtInDoubleQuotes = syntax.EveryDollarForm
	d.DollarGoesWhenAContinuationStopsItAtABrace = true

	for _, tc := range []struct{ name, src, want string }{
		{"the brace it drops the dollar at", "echo \"$\\\n{x}\"", "{x}"},
		// Stopped at a parenthesis by the same set, and the `$` stays: the
		// flag is asked at the brace and nowhere else.
		{"a parenthesis it keeps the dollar at", "echo \"$\\\n(echo hi)\"", "$(echo hi)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := firstSpanOf(t, tc.src, d)
			if sp.Kind != syntax.Literal || sp.Value != tc.want {
				t.Errorf("%q is a %v holding %q, want a literal holding %q",
					tc.src, sp.Kind, sp.Value, tc.want)
			}
		})
	}
}

// An unquoted here-document body stops nothing, whatever the dialect holds.
// Measured 2026-09-16: a body delimited by an unquoted `E` holding
// `[$\⏎x][$\⏎{x}]` is `[5][5]` in bash 5.3, zsh 5.9.2, ksh93u+ and dash, which
// is the same exemption the refusal inside `${ }` has there.
func TestAnUnquotedHeredocBodyStopsNoDollarAtAContinuation(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.ContinuationStopsADollarAt = syntax.EveryDollarForm
	d.ContinuationStopsADollarAtInDoubleQuotes = syntax.EveryDollarForm

	spans, err := syntax.HeredocSpans("[$\\\nx][$\\\n{x}]\n", d)
	if err != nil {
		t.Fatalf("heredoc body: %v", err)
	}
	var params int
	for _, sp := range spans {
		if sp.Kind == syntax.ParamExp && sp.Value == "x" {
			params++
		}
	}
	if params != 2 {
		t.Errorf("the body holds %d expansions of x, want 2: %v", params, spans)
	}
}
