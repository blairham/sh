// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"testing"
)

// selecting is the core plus the element-selection operators. The flag is
// named here and the shell that sets it is not, which is what keeps this
// package free of dialect names.
func selecting() Dialect {
	d := Core()
	d.ParamElementSelection = true
	return d
}

// The three operators parse, and each is its own node rather than a substring
// whose offset happens to start with a punctuation mark.
func TestTheElementSelectionOperatorsParse(t *testing.T) {
	for _, tc := range []struct {
		src  string
		op   ParamOp
		arg  string
		name string
	}{
		{`echo ${a:#two}`, ParamExclude, "two", "exclusion by pattern"},
		{`echo ${a:#}`, ParamExclude, "", "exclusion with an empty pattern"},
		{`echo ${a:#t*}`, ParamExclude, "t*", "a pattern with a metacharacter"},
		{`echo ${a:|b}`, ParamSetDifference, "b", "set difference"},
		{`echo ${a:*b}`, ParamSetIntersection, "b", "set intersection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := firstParam(t, tc.src, selecting())
			if e.Op != tc.op {
				t.Fatalf("op = %v (%q), want %v (%q)", e.Op, e.Op, tc.op, tc.op)
			}
			got := ""
			if e.Arg != nil {
				got = e.Arg.Literal()
			}
			if got != tc.arg {
				t.Errorf("operand = %q, want %q", got, tc.arg)
			}
		})
	}
}

// Exactly three characters after the colon make an operator. Everything else
// is the substring, the default, or the alternate it always was.
//
// This is the guard the whole change rests on, and it is asserted from the
// operator's own side: the disambiguation is a single character, so a grammar
// that widened it by one would break shapes every shell in the panel shares
// rather than the ones only one of them has.
func TestAColonBeforeAnythingElseIsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		src string
		op  ParamOp
	}{
		{`echo ${v:2}`, ParamSubstring},
		{`echo ${v:2:2}`, ParamSubstring},
		{`echo ${v: -2}`, ParamSubstring},
		{`echo ${v:+set}`, ParamAlternate},
		{`echo ${v:-alt}`, ParamDefault},
		{`echo ${v:=set}`, ParamAssign},
		{`echo ${v:?msg}`, ParamError},
		// A colon with nothing at all after it: too short to be an operator,
		// so it stays the substring whose offset is empty.
		{`echo ${v:}`, ParamSubstring},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if e := firstParam(t, tc.src, selecting()); e.Op != tc.op {
				t.Errorf("op = %v (%q), want %v (%q)", e.Op, e.Op, tc.op, tc.op)
			}
		})
	}
}

// Without the flag the same characters are the substring they are in the rest
// of the panel, which is what makes this additive: a grammar that has not
// asked for the operators cannot accidentally acquire them.
//
// Both readings from the same source, because asserting only the flagged side
// asserts that the flag does *something* rather than that it is the flag doing
// it.
func TestWithoutTheFlagTheColonStartsAnOffset(t *testing.T) {
	for _, src := range []string{`echo ${a:#two}`, `echo ${a:|b}`, `echo ${a:*b}`} {
		on := firstParam(t, src, selecting())
		off := firstParam(t, src, Core())
		if on.Op == off.Op {
			t.Errorf("%s: both grammars read %v — the flag decided nothing", src, on.Op)
		}
		if off.Op != ParamSubstring {
			t.Errorf("%s: without the flag op = %v, want the substring", src, off.Op)
		}
	}
}

// The operand is a word, so a pattern may be built out of expansions and
// quoting, and the spans survive to say which parts were quoted.
//
// Load-bearing rather than incidental: whether a metacharacter out of a
// parameter is a metacharacter is a semantics question, and it can only be
// asked if the parse kept the difference.
func TestTheExclusionOperandIsAWord(t *testing.T) {
	e := firstParam(t, `echo ${a:#$p}`, selecting())
	if e.Op != ParamExclude {
		t.Fatalf("op = %v, want the exclusion", e.Op)
	}
	if e.Arg == nil || len(e.Arg.Spans) != 1 {
		t.Fatalf("operand = %#v, want one span", e.Arg)
	}
	if got := e.Arg.Spans[0].Kind; got != ParamExp {
		t.Errorf("operand span kind = %v, want a parameter expansion", got)
	}
}
