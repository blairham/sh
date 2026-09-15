// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// TraceArrayLiteralShowsTheExpandedElements and
// TraceElementSubscriptIsEvaluated — #1959, the other half of #1937.
//
// #1937 fixed the *spelling* of a traced assignment: the subscript, the `+=`
// and the parenthesized element list all reach the trace. These two are what
// the columns that print what the assignment *came to* show instead, and they
// are separate axes because they split the panel differently — one column
// prints its expanded elements and its written subscript, and one prints both
// as values.
//
// Named for the axes and never for a shell.

// tracedByValueAxis is traceOf with the two axes moved and the spaced rendering the
// two columns that answer yes to either of them use.
func tracedByValueAxis(t *testing.T, src string, elems, subscript Answer) string {
	t.Helper()
	sem := permissive()
	sem.TraceArrayLiteralShowsTheExpandedElements = elems
	sem.TraceElementSubscriptIsEvaluated = subscript
	// One line per assignment, so the line's position against a
	// substitution's own trace is visible: a column that writes one line for
	// the whole list cannot write it until every value is known, which hides
	// the ordering half of the first axis.
	sem.TraceAssignmentsSeparately = Yes
	return traceOf(t, src, sem, Diagnostics{
		TraceArrayLiteral: TraceArraySpaced,
		TraceQuoting:      QuoteShell,
	})
}

func TestATracedArrayLiteralShowsItsElementsByAxis(t *testing.T) {
	for _, c := range []struct{ name, src, yes, no string }{
		{
			// The row the issue is named for.
			"a quoted parameter is one element either way",
			`set -x; x="p q"; a=("$x" r)`,
			"+ x='p q'\n+ a=( 'p q' r )\n",
			"+ x='p q'\n+ a=( \"$x\" r )\n",
		},
		{
			// The count is the *expansion's* and not the literal's: an
			// unquoted parameter that splits is two elements, and the trace
			// of the column that prints values says two.
			"an unquoted parameter is however many fields it split into",
			`set -x; x="p q"; a=($x)`,
			"+ x='p q'\n+ a=( p q )\n",
			"+ x='p q'\n+ a=( $x )\n",
		},
		{
			// And an expansion that produced nothing leaves an empty pair
			// rather than an empty word, which a per-element rendering would
			// have written as `''`.
			"a parameter that expands to nothing is no elements at all",
			`set -x; x=""; a=($x)`,
			"+ x=''\n+ a=( )\n",
			"+ x=''\n+ a=( $x )\n",
		},
		{
			// The append spelling carries its operator through either
			// reading, which is #1937's half and must not move.
			"an append keeps its operator",
			`set -x; x="p q"; a+=("$x")`,
			"+ x='p q'\n+ a+=( 'p q' )\n",
			"+ x='p q'\n+ a+=( \"$x\" )\n",
		},
		{
			// A literal of plain words reads the same either way, which is
			// the control: a run where only this row passed would be a
			// rendering that had stopped expanding anything.
			"a literal with nothing to expand reads alike",
			`set -x; a=(1 2)`,
			"+ a=( 1 2 )\n",
			"+ a=( 1 2 )\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := tracedByValueAxis(t, c.src, Yes, No); got != c.yes {
				t.Errorf("yes: got %q, want %q", got, c.yes)
			}
			if got := tracedByValueAxis(t, c.src, No, No); got != c.no {
				t.Errorf("no: got %q, want %q", got, c.no)
			}
		})
	}
}

// The substitution runs *before* the line is written under the yes reading and
// after it under the no reading, which is the ordering the axis carries with
// it — and the reason the elements are expanded by the assignment rather than
// by the printing.
func TestTheOrderingMovesWithTheElements(t *testing.T) {
	if got, want := tracedByValueAxis(t, `set -x; a=($(echo x))`, Yes, No),
		"+ echo x\n+ a=( x )\n"; got != want {
		t.Errorf("yes: got %q, want %q", got, want)
	}
	if got, want := tracedByValueAxis(t, `set -x; a=($(echo x))`, No, No),
		"+ a=( $(echo x) )\n+ echo x\n"; got != want {
		t.Errorf("no: got %q, want %q", got, want)
	}
}

// Expanded once. The elements the trace prints are the ones the store used, so
// a substitution in a literal runs a single time — the double run #1915 fixed
// for a scalar's value, at the other construct.
func TestTheElementsAreExpandedOnceUnderEitherReading(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		// The substitution's own trace line is the counter: it is written
		// once per run, so two of them is the double expansion. A variable
		// cannot be the counter here — the body runs in a subshell, so what
		// it steps does not come back.
		src := `set -x; a=($(echo v))`
		got := tracedByValueAxis(t, src, a, No)
		if n := lineCount(got, "+ echo v"); n != 1 {
			t.Errorf("answered %v: the substitution ran %d times, want once (%q)", a, n, got)
		}
	}
}

func TestATracedSubscriptIsWhatItResolvedToByAxis(t *testing.T) {
	for _, c := range []struct{ name, src, yes, no string }{
		{
			"a parameter in the subscript",
			`set -x; i=2; a[$i]=v`,
			"+ i=2\n+ a[2]=v\n",
			"+ i=2\n+ a[$i]=v\n",
		},
		{
			// A bare name in a subscript is arithmetic, and the resolved
			// reading prints what the arithmetic came to — which a rule
			// about *expansion* alone would have left as `a[i]`.
			"a bare name in the subscript",
			`set -x; i=2; a[i]=v`,
			"+ i=2\n+ a[2]=v\n",
			"+ i=2\n+ a[i]=v\n",
		},
		{
			"an expression in the subscript",
			`set -x; i=2; a[$i+1]=v`,
			"+ i=2\n+ a[3]=v\n",
			"+ i=2\n+ a[$i+1]=v\n",
		},
		{
			// The append spelling keeps its operator under both readings.
			"an element append",
			`set -x; i=1; a[$i]+=v`,
			"+ i=1\n+ a[1]+=v\n",
			"+ i=1\n+ a[$i]+=v\n",
		},
		{
			// The control: a subscript with nothing to resolve reads alike,
			// so a run where only this passed would be a reading that had
			// stopped resolving.
			"a literal subscript reads alike",
			`set -x; a[1]=v`,
			"+ a[1]=v\n",
			"+ a[1]=v\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := tracedByValueAxis(t, c.src, No, Yes); got != c.yes {
				t.Errorf("yes: got %q, want %q", got, c.yes)
			}
			if got := tracedByValueAxis(t, c.src, No, No); got != c.no {
				t.Errorf("no: got %q, want %q", got, c.no)
			}
		})
	}
}

// Resolved once under either reading, which is what makes the resolved
// spelling the *store's* rather than the printing's: `a[$((i++))]=v` steps `i`
// exactly once, and the number the trace writes is the one the element went to.
func TestTheSubscriptIsResolvedOnceUnderEitherReading(t *testing.T) {
	if got, want := tracedByValueAxis(t, `set -x; i=0; a[$((i++))]=v; echo "$i [${a[0]}]"`, No, Yes),
		"+ i=0\n+ a[0]=v\n+ echo '1 [v]'\n"; got != want {
		t.Errorf("yes: got %q, want %q", got, want)
	}
	if got, want := tracedByValueAxis(t, `set -x; i=0; a[$((i++))]=v; echo "$i [${a[0]}]"`, No, No),
		"+ i=0\n+ a[$((i++))]=v\n+ echo '1 [v]'\n"; got != want {
		t.Errorf("no: got %q, want %q", got, want)
	}
}

// An assignment whose subscript will not resolve leaves no line at all under
// the resolved reading, and that is not a nicety: the line has nothing to
// write in the brackets. Under the written reading the line is written first
// and the complaint follows it.
func TestARefusedSubscriptWritesNoResolvedLine(t *testing.T) {
	yes := tracedByValueAxis(t, `set -x; a[1/0]=v`, No, Yes)
	if containsLine(yes, "+ a[") {
		t.Errorf("yes: got %q, want no assignment line", yes)
	}
	no := tracedByValueAxis(t, `set -x; a[1/0]=v`, No, No)
	if !containsLine(no, "+ a[1/0]=v") {
		t.Errorf("no: got %q, want the line as written", no)
	}
}

// lineCount is how many lines of s are exactly this text.
func lineCount(s, want string) int {
	n := 0
	for line := range splitLines(s) {
		if line == want {
			n++
		}
	}
	return n
}

func containsLine(s, prefix string) bool {
	for line := range splitLines(s) {
		if len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func splitLines(s string) func(func(string) bool) {
	return func(yield func(string) bool) {
		start := 0
		for i := range len(s) {
			if s[i] == '\n' {
				if !yield(s[start:i]) {
					return
				}
				start = i + 1
			}
		}
		if start < len(s) {
			yield(s[start:])
		}
	}
}
