// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// frozenOrderSem is elementOrderSem with the two freeze axes answered too.
func frozenOrderSem(subscriptFirst, beforeSubscript, literalFirst Answer) Semantics {
	s := elementOrderSem(subscriptFirst)
	s.FrozenElementIsRefusedBeforeItsSubscript = beforeSubscript
	s.FrozenLiteralIsExpandedBeforeItIsRefused = literalFirst
	s.SubscriptedArrayLiteral = SubscriptedArrayLiteralNests
	return s
}

// frozenSideEffects is what a run wrote, with the refusal reduced to a word
// and the trace and the division's complaint left out — the order the side
// effects happened in, and how the write ended.
func frozenSideEffects(out string) string {
	var keep []string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+"):
		case strings.Contains(line, "1/0"), strings.Contains(line, "division"):
			keep = append(keep, "DIV")
		case strings.Contains(line, "readonly"), strings.Contains(line, "read only"),
			strings.Contains(line, "read-only"):
			keep = append(keep, "RO")
		default:
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, " ")
}

// A frozen scalar is refused once its value has been expanded, and a value
// that will not expand is what is reported. Unanimous, so core, and the same
// traced and untraced (#6674).
func TestAFrozenScalarIsRefusedOnceItsValueIsExpanded(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{"readonly x=1\nx=$(echo VAL >&2)y\n", "VAL RO"},
		{"readonly x=1\nx+=$(echo VAL >&2)y\n", "VAL RO"},
		{"readonly x=1\nx=$((1/0))\n", "DIV"},
		{"readonly x=1\nx=y\n", "RO"},
	} {
		for _, traced := range []bool{false, true} {
			src := c.src
			if traced {
				src = "set -x\n" + src
			}
			out, _ := run(t, src, withSem(frozenOrderSem(No, No, No)))
			if got := frozenSideEffects(out); got != c.want {
				t.Errorf("%q traced=%v: said %q, want %q", c.src, traced, got, c.want)
			}
		}
	}
}

// Where a frozen element write is refused is the axis: once the value is
// expanded, or at the store, after the subscript's words — in the order the
// element-order axis gives them — and its arithmetic.
func TestTheFrozenElementOrderIsTheAxis(t *testing.T) {
	t.Parallel()
	sub := `$(echo SUB >&2; echo 1)`
	val := `$(echo VAL >&2)`
	for _, c := range []struct {
		name, src                       string
		before, storeValFirst, storeSub string
	}{
		{
			"an indexed element", "readonly a=(1 2)\na[" + sub + "]=" + val + "x\n",
			"VAL RO", "VAL SUB RO", "SUB VAL RO",
		},
		{
			"an appended element", "readonly a=(1 2)\na[" + sub + "]+=" + val + "x\n",
			"VAL RO", "VAL SUB RO", "SUB VAL RO",
		},
		{
			"a table's key", "typeset -A m\nm[k]=1\nreadonly m\nm[" + sub + "]=" + val + "x\n",
			"VAL RO", "VAL SUB RO", "SUB VAL RO",
		},
		{
			"a literal value", "readonly a=(1 2)\na[" + sub + "]=x\n",
			"RO", "SUB RO", "SUB RO",
		},
		{
			"a subscript that will not evaluate", "readonly a=(1 2)\na[$(echo SUB >&2; echo 1/0)]=" + val + "y\n",
			"VAL RO", "VAL SUB DIV", "SUB VAL DIV",
		},
		{
			"a value that will not expand", "readonly a=(1 2)\na[" + sub + "]=$((1/0))\n",
			"DIV", "DIV", "SUB DIV",
		},
	} {
		for _, traced := range []bool{false, true} {
			src := c.src
			if traced {
				src = "set -x\n" + src
			}
			for _, row := range []struct {
				sem  Semantics
				want string
			}{
				{frozenOrderSem(No, Yes, No), c.before},
				{frozenOrderSem(No, No, No), c.storeValFirst},
				{frozenOrderSem(Yes, No, No), c.storeSub},
			} {
				out, _ := run(t, src, withSem(row.sem))
				if got := frozenSideEffects(out); got != row.want {
					t.Errorf("%s, traced=%v, refused before the subscript=%v, subscript first=%v: said %q, want %q",
						c.name, traced, row.sem.FrozenElementIsRefusedBeforeItsSubscript,
						row.sem.ElementSubscriptIsExpandedBeforeTheValue, got, row.want)
				}
			}
		}
	}
}

// Whether a frozen name's whole literal is expanded before it is refused is
// the other axis, and the trace does not expand it where the refusal would
// not — even in the trace that prints what the elements came to.
func TestTheFrozenLiteralOrderIsTheAxis(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, first, refused string }{
		{"readonly a=(1 2)\na=($(echo VAL >&2))\n", "VAL RO", "RO"},
		{"readonly a=(1 2)\na+=($(echo VAL >&2))\n", "VAL RO", "RO"},
		{"readonly a=(1 2)\na=(p $((1/0)))\n", "DIV", "RO"},
		{"typeset -A m\nm[k]=1\nreadonly m\nm=([$(echo SUB >&2; echo k)]=$(echo VAL >&2))\n", "SUB VAL RO", "RO"},
	} {
		for _, traced := range []bool{false, true} {
			for _, first := range []Answer{Yes, No} {
				src := c.src
				sem := frozenOrderSem(No, No, first)
				if traced {
					src = "set -x\n" + src
					sem.TraceArrayLiteralShowsTheExpandedElements = Yes
				}
				want := c.refused
				if first == Yes {
					want = c.first
				}
				out, _ := run(t, src, withSem(sem))
				if got := frozenSideEffects(out); got != want {
					t.Errorf("%q, traced=%v, expanded first=%v: said %q, want %q", c.src, traced, first, got, want)
				}
			}
		}
	}
}

// An element literal over a frozen name reaches its store in every reading
// before the freeze refuses it, both sides expanded in the element order's
// sequence — and the reading that refuses the construct does so first,
// without running the subscript to quote it.
func TestAFrozenElementLiteralIsRefusedAtItsStore(t *testing.T) {
	t.Parallel()
	src := "readonly a=(1 2)\na[$(echo SUB >&2; echo 1)]=(p $(echo VAL >&2))\n"
	for _, c := range []struct {
		policy SubscriptedArrayLiteralPolicy
		first  Answer
		want   string
	}{
		{SubscriptedArrayLiteralNests, Yes, "SUB VAL RO"},
		{SubscriptedArrayLiteralNests, No, "VAL SUB RO"},
		{SubscriptedArrayLiteralSplices, Yes, "SUB VAL RO"},
		{SubscriptedArrayLiteralSplices, No, "VAL SUB RO"},
	} {
		sem := frozenOrderSem(c.first, No, No)
		sem.SubscriptedArrayLiteral = c.policy
		out, _ := run(t, src, withSem(sem))
		if got := frozenSideEffects(out); got != c.want {
			t.Errorf("policy %v, subscript first=%v: said %q, want %q", c.policy, c.first, got, c.want)
		}
	}
	sem := frozenOrderSem(No, No, No)
	sem.SubscriptedArrayLiteral = SubscriptedArrayLiteralRefused
	out, _ := run(t, src, withSem(sem))
	if strings.HasPrefix(out, "SUB\n") || strings.Contains(out, "\nSUB\n") || strings.Contains(out, "\nVAL\n") ||
		strings.Contains(out, "readonly") ||
		!strings.Contains(out, "a[$(echo SUB >&2; echo 1)]: cannot assign list to array member") {
		t.Errorf("refused construct: said %q, want the list refused by its written subscript and nothing run", out)
	}
}

// An element literal does the whole of one side before the other, the
// subscript's arithmetic included, and which side is first is the element
// order's answer — under both stores (#6675).
func TestAnElementLiteralFollowsTheElementOrder(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src            string
		valueFirst, subFirst string
	}{
		{
			"words", "a=(1 2)\na[$(echo SUB >&2; echo 1)]=(p $(echo VAL >&2))\n",
			"VAL SUB", "SUB VAL",
		},
		{
			"a subscript that will not evaluate", "a=(1 2)\na[$(echo SUB >&2; echo 1/0)]=(p $(echo VAL >&2))\n",
			"VAL SUB DIV", "SUB DIV",
		},
		{
			"a list that will not expand", "a=(1 2)\na[$(echo SUB >&2; echo 1)]=(p $((1/0)))\n",
			"DIV", "SUB DIV",
		},
		{
			"the arithmetic", "a=(1 2)\ni=1\na[i++]=($((i*10)))\necho \"${a[1]} $i\"\n",
			"", "",
		},
	} {
		for _, policy := range []SubscriptedArrayLiteralPolicy{SubscriptedArrayLiteralNests, SubscriptedArrayLiteralSplices} {
			for _, first := range []Answer{No, Yes} {
				sem := frozenOrderSem(first, No, No)
				sem.SubscriptedArrayLiteral = policy
				out, _ := run(t, c.src, withSem(sem))
				got := frozenSideEffects(out)
				want := c.valueFirst
				if first == Yes {
					want = c.subFirst
				}
				if c.name == "the arithmetic" {
					// The value reads `i` before the step in one order and
					// after it in the other.
					want = map[bool]string{false: "10", true: "20"}[first == Yes]
					if !strings.Contains(got, want) || strings.Contains(got, map[bool]string{false: "20", true: "10"}[first == Yes]) {
						t.Errorf("%s, policy %v, subscript first=%v: said %q, want the value to read %s",
							c.name, policy, first, got, want)
					}
					continue
				}
				if !strings.HasPrefix(got, want) {
					t.Errorf("%s, policy %v, subscript first=%v: said %q, want it to start %q",
						c.name, policy, first, got, want)
				}
			}
		}
	}
}

// With every freeze axis unanswered, a frozen write whose order nobody could
// see asks nothing and is refused, and one whose order shows is the question.
func TestAnOrdinaryFrozenWriteAsksNothing(t *testing.T) {
	t.Parallel()
	sem := frozenOrderSem(No, Unspecified, Unspecified)
	for _, src := range []string{
		"readonly a=(1 2)\na[1]=x\n",
		"readonly a=(1 2)\ni=1\na[$i]=$i\n",
		"readonly a=(1 2)\na=(p q)\n",
		"typeset -A m\nm[k]=1\nreadonly m\nm=([z]=9)\n",
		"readonly a=(1 2)\na+=([3]=x)\n",
		"readonly x=1\nx=$(echo v)\n",
	} {
		out, _ := run(t, src, withSem(sem))
		if strings.Contains(out, "disagree") || !strings.Contains(out, "readonly") {
			t.Errorf("%q: said %q, want the refusal and no question", src, out)
		}
	}
	for _, c := range []struct{ src, question string }{
		{"readonly a=(1 2)\na[$(echo 1)]=x\n", "refused before its subscript"},
		{"readonly a=(1 2)\na[1]=$(echo x)\n", "refused before its subscript"},
		{"readonly a=(1 2)\na=($(echo x))\n", "expanded before it is refused"},
		{"readonly a=(1 2)\na=([i++]=x)\n", "expanded before it is refused"},
		// The whole-array spelling's own refusal is a different sentence
		// from the freeze's, so its order shows with nothing to run (#6761).
		{"readonly a=(1 2)\na[@]=x\n", "refused before its subscript"},
		{"typeset -A m\nm[k]=1\nreadonly m\nm[@]=x\n", "refused before its subscript"},
	} {
		out, _ := run(t, c.src, withSem(sem))
		if !strings.Contains(out, c.question) || !strings.Contains(out, "disagree") {
			t.Errorf("%q: said %q, want the question", c.src, out)
		}
	}
}

// A frozen name refused at the store keeps every element it had: the
// refusal stands in front of the write and not behind it. Read back after a
// refusal that is not fatal, through each store a held refusal reaches — an
// output-only check cannot tell a refused write from one that was made and
// then complained about.
func TestAFrozenNameRefusedAtItsStoreKeepsItsElements(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		policy SubscriptedArrayLiteralPolicy
		src    string
	}{
		{"an indexed element", SubscriptedArrayLiteralNests, "a[$(echo 1)]=$(echo x)y"},
		{"an appended element", SubscriptedArrayLiteralNests, "a[$(echo 1)]+=$(echo x)y"},
		{"a nested literal", SubscriptedArrayLiteralNests, "a[$(echo 1)]=(p $(echo x))"},
		{"a spliced literal", SubscriptedArrayLiteralSplices, "a[$(echo 1)]=(p $(echo x))"},
	} {
		sem := frozenOrderSem(No, No, No)
		sem.ReadonlyReassignmentFatal = No
		sem.SubscriptedArrayLiteral = c.policy
		out, _ := run(t, "readonly a=(1 2)\n"+c.src+"\necho \"[${a[*]}] ${#a[@]}\"\n", withSem(sem))
		if !strings.Contains(out, "readonly") || !strings.HasSuffix(out, "[1 2] 2\n") {
			t.Errorf("%s: said %q, want the refusal and the array as it was", c.name, out)
		}
	}
}

// The whole-array spelling over a frozen name waits for its store where the
// element order is "at the store": a store that names every element or
// takes `@` for a key is refused there and keeps what the name had, and a
// spelling refused for its subscript says that and not the freeze (#6761).
func TestAFrozenWholeArraySubscriptWaitsForItsStore(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		array, table    WholeArraySubscriptAssignPolicy
		readBack        string
	}{
		{
			"every element", "readonly a=(1 2)\na[@]=$(echo VAL >&2)x\n", "VAL RO",
			WholeArraySubscriptNamesEveryElement, WholeArraySubscriptIsASliceOfATable,
			`[${a[*]}] ${#a[@]}`,
		},
		{
			"every element, appended", "readonly a=(1 2)\na[@]+=$(echo VAL >&2)x\n", "VAL RO",
			WholeArraySubscriptNamesEveryElement, WholeArraySubscriptIsASliceOfATable,
			`[${a[*]}] ${#a[@]}`,
		},
		{
			"a key", "typeset -A a\na[k]=1\nreadonly a\na[@]=$(echo VAL >&2)x\n", "VAL RO",
			WholeArraySubscriptIsABadSubscript, WholeArraySubscriptIsAnOrdinaryKey,
			`[${a[k]}] ${#a[@]}`,
		},
		{
			"a key, appended", "typeset -A a\na[k]=1\nreadonly a\na[@]+=$(echo VAL >&2)x\n", "VAL RO",
			WholeArraySubscriptIsABadSubscript, WholeArraySubscriptIsAnOrdinaryKey,
			`[${a[k]}] ${#a[@]}`,
		},
		{
			"a bad subscript", "readonly a=(1 2)\na[@]=x\n", "a[@]: bad array subscript",
			WholeArraySubscriptIsABadSubscript, WholeArraySubscriptIsAnOrdinaryKey,
			`[${a[*]}] ${#a[@]}`,
		},
		{
			"an invalid subscript", "readonly a=(1 2)\na[*]=x\n", "*: invalid subscript in assignment",
			WholeArraySubscriptIsInvalidInAnAssignment, WholeArraySubscriptIsInvalidInAnAssignment,
			`[${a[*]}] ${#a[@]}`,
		},
	} {
		sem := frozenOrderSem(No, No, No)
		sem.ReadonlyReassignmentFatal = No
		sem.WholeArraySubscriptAssigningAnArray = c.array
		sem.WholeArraySubscriptAssigningATable = c.table
		out, _ := run(t, c.src+"echo \""+c.readBack+"\"\n", withSem(sem))
		got := frozenSideEffects(out)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: said %q, want %q", c.name, got, c.want)
		}
		if c.want != "VAL RO" && strings.Contains(got, "RO") {
			t.Errorf("%s: said %q, want the subscript's refusal and not the freeze's", c.name, got)
		}
		// The invalid subscript ends the script, so there is nothing to read.
		if c.array != WholeArraySubscriptIsInvalidInAnAssignment &&
			!strings.HasSuffix(got, "[1 2] 2") && !strings.HasSuffix(got, "[1] 1") {
			t.Errorf("%s: said %q, want the name as it was", c.name, got)
		}
	}
}
