// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// elementOrderSem is the core vector with the order answered, and with the
// trace written a line per assignment after its store, so that a traced row
// shows the store's own order and not the trace loop's.
func elementOrderSem(first Answer) Semantics {
	s := PosixSemantics()
	s.ElementSubscriptIsExpandedBeforeTheValue = first
	s.FailedExpansionAbandonsTheLine = Yes
	s.FatalErrorStatusIsOne = Yes
	s.TraceArrayLiteralShowsTheExpandedElements = No
	s.TraceAssignmentsSeparately = Yes
	s.TraceLineFollowsTheStore = Yes
	s.TraceElementSubscriptIsEvaluated = No
	return s
}

// The axis decides which of an element write's two words is expanded first,
// and the subscript's arithmetic comes after the value under either answer.
// Measured 2026-10-15: two columns expand the value first and one the
// subscript's words first, traced or not, and all three evaluate `i++` after
// the value has read `i` (#6604).
func TestTheElementWriteOrderIsTheAxis(t *testing.T) {
	t.Parallel()
	sub := `$(echo SUB >&2; echo 1)`
	val := `$(echo VAL >&2)`
	for _, c := range []struct {
		name, src       string
		valFirst, after string
	}{
		{
			"an indexed element", "a=(1 2)\na[" + sub + "]=" + val + "x\necho \"${a[*]}\"\n",
			"VAL\nSUB\n1 x\n", "SUB\nVAL\n1 x\n",
		},
		{
			"an appended element", "a=(1 2)\na[" + sub + "]+=" + val + "x\necho \"${a[*]}\"\n",
			"VAL\nSUB\n1 2x\n", "SUB\nVAL\n1 2x\n",
		},
		{
			"a table's key", "typeset -A m\nm[" + sub + "]=" + val + "x\necho \"${m[1]}/${m[j]}\"\n",
			"VAL\nSUB\nx/\n", "SUB\nVAL\nx/\n",
		},
		{
			"a value that will not expand", "a=(1 2)\na[" + sub + "]=" + val + "$((1/0))\necho \"[${a[*]}]\"\n",
			"VAL\n", "SUB\nVAL\n",
		},
		{
			"a subscript that will not evaluate", "a=(1 2)\na[$(echo SUB >&2; echo 1/0)]=" + val + "y\n",
			"VAL\nSUB\n", "SUB\nVAL\n",
		},
		{
			"the arithmetic after the value", "i=1 a=(1 2)\na[i++]=$((i*10))\necho \"${a[*]} $i\"\n",
			"1 10 2\n", "1 10 2\n",
		},
		{
			"an expanded subscript", "i=1 a=(1 2)\na[$((i++))]=$((i*10))\necho \"${a[*]} $i\"\n",
			"1 10 2\n", "1 20 2\n",
		},
	} {
		for _, traced := range []bool{false, true} {
			for _, first := range []Answer{No, Yes} {
				src := c.src
				if traced {
					src = "set -x\n" + src
				}
				out, _ := run(t, src, withSem(elementOrderSem(first)))
				want := c.valFirst
				if first == Yes {
					want = c.after
				}
				if got := untraced(out); !strings.HasPrefix(got, want) {
					t.Errorf("%s, traced=%v, subscript first=%v: said %q, want it to start %q",
						c.name, traced, first, got, want)
				}
			}
		}
	}
}

// untraced drops the trace lines and the arithmetic complaint, leaving the
// side effects in the order they happened.
func untraced(out string) string {
	var keep []string
	for _, line := range strings.SplitAfter(out, "\n") {
		if strings.HasPrefix(line, "+") || strings.Contains(line, "1/0") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "")
}

// A traced element write in the dialect that expands the subscript first
// waits for its store: the subscript's substitution, the value's, then the
// line — and the value's substitution runs once, not once for the line and
// again for the store.
func TestATracedElementWriteExpandsItsValueOnce(t *testing.T) {
	for _, first := range []Answer{No, Yes} {
		out, _ := run(t, "set -x\na[$(echo SUB >&2; echo 1)]=$(echo VAL >&2)x\n",
			withSem(elementOrderSem(first)))
		if n := strings.Count("\n"+out, "\nVAL\n"); n != 1 {
			t.Errorf("subscript first=%v: VAL written %d times in %q", first, n, out)
		}
		if n := strings.Count("\n"+out, "\nSUB\n"); n != 1 {
			t.Errorf("subscript first=%v: SUB written %d times in %q", first, n, out)
		}
	}
}

// unorderedSem answers everything an element write reaches except the order.
func unorderedSem() Semantics {
	s := elementOrderSem(No)
	s.ElementSubscriptIsExpandedBeforeTheValue = Unspecified
	return s
}

// With the order unanswered, a write whose two words could only ever come out
// the same asks nothing, and one whose order shows is the question.
func TestAnOrdinaryElementWriteAsksNothing(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"a=(0 0 0); i=1 x=v; a[$i]=$x; echo \"${a[*]}\"",
		"a=(0 0 0); i=1; a[i+1]=$(echo v); echo \"${a[*]}\"",
		"a=(0 0 0); i=1; a[$((i+1))]=${i}v; echo \"${a[*]}\"",
		"a=(0 0 0); a[1]=$(echo v); echo \"${a[*]}\"",
		"a=(0 0 0); a[$(echo 1)]=v; echo \"${a[*]}\"",
	} {
		out, _ := run(t, src, withSem(unorderedSem()))
		if strings.Contains(out, "disagree") || !strings.Contains(out, "v") {
			t.Errorf("%s: said %q, want the value and no question", src, out)
		}
	}
	for _, src := range []string{
		"a[$(echo SUB >&2; echo 1)]=$(echo VAL >&2)x",
		"i=0; a[$((i++))]=$i",
		"a[${k:=1}]=$k",
	} {
		out, _ := run(t, src, withSem(unorderedSem()))
		if !strings.Contains(out, "subscript expanded before its value: the shells disagree") {
			t.Errorf("%s: said %q, want the question", src, out)
		}
	}
}

// A table's key is expanded once on a write, as it is on a read: the two
// readings of the key — quotes removed and quotes kept — used to expand the
// word one each, so a substitution in it ran twice and an arithmetic one
// stepped twice (#6605). A key that was never stored reads empty, which is
// what says the write reached a table and not an indexed array.
func TestATableKeyIsExpandedOnceOnAWrite(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{"typeset -A m\nm[$(echo SUB >&2; echo k)]=v\necho \"${m[k]}/${m[j]}\"\n", "SUB\nv/\n"},
		{"typeset -A m\nm[$(echo SUB >&2; echo k)]+=v\necho \"${m[k]}/${m[j]}\"\n", "SUB\nv/\n"},
		{"typeset -A m\ni=0\nm[$((i++))]=v\necho \"${m[0]}/${m[j]} $i\"\n", "v/ 1\n"},
	} {
		for _, first := range []Answer{No, Yes} {
			if out, _ := run(t, c.src, withSem(elementOrderSem(first))); out != c.want {
				t.Errorf("%q, subscript first=%v: said %q, want %q", c.src, first, out, c.want)
			}
		}
	}
}
