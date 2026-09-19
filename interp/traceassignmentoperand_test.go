// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// How `set -x` writes a declaration utility's `name=value` operand — see
// Diagnostics.TraceAssignmentOperand.
//
// Named for the field rather than for the shells that pick each value; the
// presets' picks are asserted in dialect/. What is asserted here is that the
// *shape* carries the panel: the same bytes are one quoted word after a
// command that declares nothing and a target plus a value after one that
// does, and that the second reading is the expansion's own answer rather
// than a second look at the command word.

// tracedLine is one command's trace line with the prefix and the newline
// trimmed off.
func tracedLine(t *testing.T, src string, tweak func(*Semantics), d Diagnostics) string {
	t.Helper()
	sem := permissive()
	sem.DeclarationCommandWord = DeclarationByUnquotedLiteralWord
	sem.CommandPrefixKeepsADeclaration = No
	sem.DeclarationTakesAnAppendOperand = Yes
	if tweak != nil {
		tweak(&sem)
	}
	d.TraceQuoting = QuoteShell
	// The last *traced* line, which is not the last line written: a command
	// this dialect goes on to refuse writes its complaint behind the trace,
	// and taking the tail would assert on the refusal instead.
	last := ""
	for _, line := range strings.Split(traceOf(t, "set -x; "+src, sem, d), "\n") {
		if strings.HasPrefix(line, "+ ") {
			last = strings.TrimPrefix(line, "+ ")
		}
	}
	return last
}

// TestAnAssignmentOperandIsWrittenTwoWays is the axis: one value quotes the
// operand whole and the other quotes only what follows the first `=`.
func TestAnAssignmentOperandIsWrittenTwoWays(t *testing.T) {
	for _, c := range []struct {
		src         string
		whole, sole string
	}{
		{`typeset x="a b"`, `typeset 'x=a b'`, `typeset x='a b'`},
		{`typeset -- x="a b"`, `typeset -- 'x=a b'`, `typeset -- x='a b'`},
		{`export x="a b" y`, `export 'x=a b' y`, `export x='a b' y`},
		// The first `=` splits it, which is what leaves an append's `+` and
		// a second `=` on the sides they are written on.
		{`typeset x+="a b"`, `typeset 'x+=a b'`, `typeset x+='a b'`},
		{`typeset x="a=b c"`, `typeset 'x=a=b c'`, `typeset x='a=b c'`},
		// The reach: the rendering runs from the first operand that was
		// *written* as an assignment to the end of the line, so the same
		// quoted word is written both ways depending on what stands in front
		// of it. Measured 2026-09-19 on zsh 5.9.2, which is the column that
		// holds this value.
		{`typeset x=1 "y=a b"`, `typeset x=1 'y=a b'`, `typeset x=1 y='a b'`},
		{`typeset "y=a b" x=1`, `typeset 'y=a b' x=1`, `typeset 'y=a b' x=1`},
	} {
		if got := tracedLine(t, c.src, nil, Diagnostics{}); got != c.whole {
			t.Errorf("%s whole: got %q, want %q", c.src, got, c.whole)
		}
		got := tracedLine(t, c.src, nil, Diagnostics{
			TraceAssignmentOperand: TraceAssignmentOperandValue,
		})
		if got != c.sole {
			t.Errorf("%s value-only: got %q, want %q", c.src, got, c.sole)
		}
	}
}

// TestOnlyADeclarationsOperandIsWrittenAsAnAssignment is the control the axis
// needs: the same bytes after a command that declares nothing are one word
// under either value, so this is a question about what stands in front of the
// operand and not about the operand.
func TestOnlyADeclarationsOperandIsWrittenAsAnAssignment(t *testing.T) {
	for _, d := range []Diagnostics{
		{},
		{TraceAssignmentOperand: TraceAssignmentOperandValue},
	} {
		if got := tracedLine(t, `echo "x=a b"`, nil, d); got != `echo 'x=a b'` {
			t.Errorf("%v: got %q, want %q", d.TraceAssignmentOperand, got, `echo 'x=a b'`)
		}
	}
	// And a declaration's operand that is not an assignment at all is left
	// exactly as the other reading leaves it.
	if got := tracedLine(t, `typeset "a b"`, nil, Diagnostics{
		TraceAssignmentOperand: TraceAssignmentOperandValue,
	}); got != `typeset 'a b'` {
		t.Errorf("operand with no `=`: got %q, want %q", got, `typeset 'a b'`)
	}
}

// TestTheTracedAssignmentReadsTheExpansionsOwnAnswer is the row that says the
// line and the expansion cannot disagree about what the command meant: a
// spelling the dialect does not accept as a declaration's command word makes
// the operand an ordinary word to both.
func TestTheTracedAssignmentReadsTheExpansionsOwnAnswer(t *testing.T) {
	d := Diagnostics{TraceAssignmentOperand: TraceAssignmentOperandValue}
	for _, c := range []struct {
		name, src, want string
	}{
		{"written plainly", `typeset x="a b"`, `typeset x='a b'`},
		{"quoted spelling", `'typeset' x="a b"`, `typeset 'x=a b'`},
		{"reached by an expansion", `c=typeset; $c x="a b"`, `typeset 'x=a b'`},
		{"behind a prefix the dialect does not keep it through", `command typeset x="a b"`, `command typeset 'x=a b'`},
	} {
		if got := tracedLine(t, c.src, nil, d); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	// And under the reading that keys on the utility's *name*, the quoted
	// spelling is a declaration and is written as one.
	got := tracedLine(t, `'typeset' x="a b"`, func(s *Semantics) {
		s.DeclarationCommandWord = DeclarationByUtilityName
	}, d)
	if got != `typeset x='a b'` {
		t.Errorf("by utility name: got %q, want %q", got, `typeset x='a b'`)
	}
}

// An empty value asks the field the shell's own assignment line asks, rather
// than a second answer of its own.
func TestAnEmptyAssignmentOperandFollowsTheEmptyValueField(t *testing.T) {
	for _, c := range []struct {
		bare bool
		want string
	}{
		{false, `typeset x=''`},
		{true, `typeset x=`},
	} {
		got := tracedLine(t, `typeset x=`, nil, Diagnostics{
			TraceAssignmentOperand:          TraceAssignmentOperandValue,
			TraceEmptyAssignmentValueIsBare: c.bare,
		})
		if got != c.want {
			t.Errorf("bare=%v: got %q, want %q", c.bare, got, c.want)
		}
	}
}
