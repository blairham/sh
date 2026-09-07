// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// alwaysAssigning is the core plus the always-assign operator. The flag is
// named here and the shell that sets it is not, which is what keeps this
// package free of dialect names.
func alwaysAssigning() Dialect {
	d := Core()
	d.ParamAssignAlways = true
	return d
}

// `::=` parses as its own operator, with the text after it as the word.
func TestTheAlwaysAssignOperatorParses(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		arg  string
	}{
		{"a word", `echo ${v::=new}`, "new"},
		{"an empty word, which assigns nothing at all", `echo ${v::=}`, ""},
		// The word is a *word*: whatever is written in it keeps its
		// structure, so a colon or an equals sign inside it is text.
		{"a word holding the operators", `echo ${v::=new:-x}`, "new:-x"},
		{"a word holding a space", `echo ${v::=a b}`, "a b"},
		{"a subscripted name", `echo ${Z[k]::=new}`, "new"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := firstParam(t, tc.src, alwaysAssigning())
			if e.Op != ParamAssignAlways {
				t.Fatalf("op = %v (%q), want %v (%q)",
					e.Op, e.Op, ParamAssignAlways, ParamAssignAlways)
			}
			got := ""
			if e.Arg != nil {
				got = e.Arg.Literal()
			}
			if got != tc.arg {
				t.Errorf("word = %q, want %q", got, tc.arg)
			}
		})
	}
}

// Exactly one character after the second colon makes the operator, and it is
// the `=`. This is the guard the whole change rests on: everything else after
// a colon is the substring, the conditional or the element selection it
// already was, and a grammar that widened the reading by one character would
// break shapes every shell in the panel shares.
//
// The three near-misses are measured rather than reasoned. On zsh 5.9.2 with
// `v=old`: `${v::-new}` and `${v::+new}` are substrings and answer empty, and
// `${v::?new}` fails in arithmetic on the word `new` — so the second colon is
// part of an offset in all three.
func TestOnlyTheEqualsMakesTheOperator(t *testing.T) {
	for _, tc := range []struct {
		src string
		op  ParamOp
	}{
		{`echo ${v::=new}`, ParamAssignAlways},
		{`echo ${v::-new}`, ParamSubstring},
		{`echo ${v::+new}`, ParamSubstring},
		{`echo ${v::?new}`, ParamSubstring},
		{`echo ${v::}`, ParamSubstring},
		{`echo ${v:=new}`, ParamAssign},
		{`echo ${v=new}`, ParamAssign},
		{`echo ${v:-new}`, ParamDefault},
		{`echo ${v:+new}`, ParamAlternate},
		{`echo ${v:?new}`, ParamError},
		{`echo ${v:2}`, ParamSubstring},
		{`echo ${v:2:2}`, ParamSubstring},
		{`echo ${v: -2}`, ParamSubstring},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if e := firstParam(t, tc.src, alwaysAssigning()); e.Op != tc.op {
				t.Errorf("op = %v (%q), want %v (%q)", e.Op, e.Op, tc.op, tc.op)
			}
		})
	}
}

// Without the flag the same characters are the substring they are in the four
// grammars that have no such operator — an empty offset and a length of
// `=word`, which is where their arithmetic error comes from. The reading has
// to stay that, because the error is the evidence for it.
func TestWithoutTheFlagItIsASubstring(t *testing.T) {
	e := firstParam(t, `echo ${v::=new}`, Core())
	if e.Op != ParamSubstring {
		t.Fatalf("op = %v (%q), want %v (%q)", e.Op, e.Op, ParamSubstring, ParamSubstring)
	}
	off, ln := "", ""
	if e.Arg != nil {
		off = e.Arg.Literal()
	}
	if e.Arg2 != nil {
		ln = e.Arg2.Literal()
	}
	if off != "" || ln != "=new" {
		t.Errorf("offset = %q and length = %q, want %q and %q", off, ln, "", "=new")
	}
}

// The word is a word and not a string: an expansion written in it is a span
// of its own, so `${v::=$w}` assigns what `w` came to rather than the three
// characters.
func TestTheWordIsExpanded(t *testing.T) {
	e := firstParam(t, `echo ${v::=$w}`, alwaysAssigning())
	if e.Arg == nil || len(e.Arg.Spans) != 1 || e.Arg.Spans[0].Kind != ParamExp {
		t.Fatalf("word = %#v, want one parameter-expansion span", e.Arg)
	}
	if got := e.Arg.Spans[0].Param.Name; got != "w" {
		t.Errorf("word expands %q, want %q", got, "w")
	}
}

// The operator spells itself back as written, which is what a diagnostic
// naming it has to print.
func TestTheAlwaysAssignOperatorSpellsItself(t *testing.T) {
	if got := ParamAssignAlways.String(); got != "::=" {
		t.Errorf("String() = %q, want %q", got, "::=")
	}
}
