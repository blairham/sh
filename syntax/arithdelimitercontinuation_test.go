// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A line continuation between the two characters of an arithmetic expansion's
// delimiters decides whether the construct is arithmetic at all, and the two
// ends are two questions.
//
// Measured 2026-09-16 from script files, `env -i PATH=/usr/bin:/bin LC_ALL=C`:
// the core reads over the pair at both ends, which is bash 5.3, bash 3.2 and
// dash — `$(\⏎( 1 + 2 ))` and `$(( 1 + 2 )\⏎)` are both 3 there. Before,
// every dialect took the command-substitution reading at both ends and the
// three that answer 3 printed a diagnostic about a command called `1` (#3454).
//
// Asserted on the span kind rather than on whether the line parses, because
// both readings parse: the wrong one is a command substitution holding a
// subshell, which runs and prints at status 0.
func TestAContinuationInsideAnArithmeticExpansionsDelimitersIsReadOver(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src string
		kind      syntax.SpanKind
		value     string
	}{
		{"the opener", "echo $(\\\n( 1 + 2 ))", syntax.ArithSubst, " 1 + 2 "},
		{"the closer", "echo $(( 1 + 2 )\\\n)", syntax.ArithSubst, " 1 + 2 "},
		{"both ends at once", "echo $(\\\n( 1 + 2 )\\\n)", syntax.ArithSubst, " 1 + 2 "},
		{"two pairs at the opener", "echo $(\\\n\\\n( 1 + 2 ))", syntax.ArithSubst, " 1 + 2 "},

		// The control that keeps this about the *pair* and not about the
		// parentheses: a blank between them takes the question away, and
		// every column reads a command substitution.
		{"a blank in front of the opener's pair", "echo $( \\\n( 1 + 2 ))", syntax.CommandSubst, " \\\n( 1 + 2 )"},
		{"a blank in front of the closer's pair", "echo $(( 1 + 2 ) \\\n)", syntax.CommandSubst, "( 1 + 2 ) \\\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := firstDelimiterSpan(t, tc.src, syntax.Core())
			if sp.Kind != tc.kind || sp.Value != tc.value {
				t.Errorf("%q is a %v holding %q, want a %v holding %q",
					tc.src, sp.Kind, sp.Value, tc.kind, tc.value)
			}
		})
	}
}

// The two ends are two fields, and this is the test that says so: each flag
// moves its own end and leaves the other alone. A single yes/no could not be
// given a value for the dialect that joins the opener and parts the closer.
func TestEachArithmeticDelimiterFlagMovesOnlyItsOwnEnd(t *testing.T) {
	t.Parallel()
	opener := syntax.Core()
	opener.ContinuationPartsTheArithmeticOpener = true
	closer := syntax.Core()
	closer.ContinuationPartsTheArithmeticCloser = true

	for _, tc := range []struct {
		name, src string
		d         syntax.Dialect
		kind      syntax.SpanKind
	}{
		{"the opener parted", "echo $(\\\n( 1 + 2 ))", opener, syntax.CommandSubst},
		{"the closer still joined", "echo $(( 1 + 2 )\\\n)", opener, syntax.ArithSubst},
		{"the closer parted", "echo $(( 1 + 2 )\\\n)", closer, syntax.CommandSubst},
		{"the opener still joined", "echo $(\\\n( 1 + 2 ))", closer, syntax.ArithSubst},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := firstDelimiterSpan(t, tc.src, tc.d)
			if sp.Kind != tc.kind {
				t.Errorf("%q is a %v, want a %v", tc.src, sp.Kind, tc.kind)
			}
		})
	}
}

// ContinuationEndsTheArithmeticCommandOpener consumes the `((` and the pair
// and produces no command, so what follows is read on its own.
//
// The two rows are the discrimination: with the flag the text behind the pair
// is an ordinary program, and without it the whole line is one arithmetic
// command. A reading that merely *refused* the shape would pass neither.
func TestAContinuationBehindAnArithmeticCommandsOpenerCanEndIt(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.ContinuationEndsTheArithmeticCommandOpener = true

	f, err := syntax.Parse("((\\\necho hi\necho b\n", d)
	if err != nil {
		t.Fatalf("`((\\⏎echo hi⏎echo b` : %v", err)
	}
	if len(f.Stmts) != 2 {
		t.Fatalf("`((\\⏎echo hi⏎echo b` read %d statements, want 2", len(f.Stmts))
	}

	// Without the flag the same text is one arithmetic command that never
	// closes, which is what every other dialect makes of it.
	if _, err := syntax.Parse("((\\\necho hi\necho b\n", syntax.Core()); err == nil {
		t.Error("`((\\⏎echo hi⏎echo b` parsed with the flag off, want it unterminated")
	}

	// A blank in front of the backslash takes the rule away: the pair is then
	// an ordinary continuation inside the expression.
	if _, err := syntax.Parse("(( \\\nx = 5 ))\n", d); err != nil {
		t.Errorf("`(( \\⏎x = 5 ))` : %v", err)
	}
}

// firstDelimiterSpan returns the first substitution span in the operand of the
// only command of src.
func firstDelimiterSpan(t *testing.T, src string, d syntax.Dialect) syntax.Span {
	t.Helper()
	cmd, ok := onlyCommand(t, src, d).(*syntax.SimpleCmd)
	if !ok {
		t.Fatalf("%q is not a simple command", src)
	}
	for _, w := range cmd.Args {
		for _, sp := range w.Spans {
			if sp.Kind == syntax.ArithSubst || sp.Kind == syntax.CommandSubst {
				return sp
			}
		}
	}
	t.Fatalf("%q holds no substitution", src)
	return syntax.Span{}
}
