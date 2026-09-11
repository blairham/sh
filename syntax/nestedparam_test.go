// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// nestingDialect is the core plus the grammar these expansions need, named by
// the constructs rather than by a shell.
func nestingDialect() Dialect {
	d := Core()
	d.NestedParamExpansion = true
	d.ParamExpansionFlags = true
	d.ParamElementSelection = true
	return d
}

// An expansion may stand where a parameter name would, and the operator that
// follows the inner brace belongs to the outer expansion.
func TestNestedParamExpansionParses(t *testing.T) {
	for _, tc := range []struct {
		src       string
		innerName string
		op        ParamOp
		length    bool
		flags     string
	}{
		{src: `echo ${${v}}`, innerName: "v"},
		{src: `echo ${${v}#a}`, innerName: "v", op: ParamTrimPrefix},
		{src: `echo ${${v}##a}`, innerName: "v", op: ParamTrimPrefixLong},
		{src: `echo ${${v}%a}`, innerName: "v", op: ParamTrimSuffix},
		{src: `echo ${${v}/a/b}`, innerName: "v", op: ParamReplace},
		{src: `echo ${${v}:-d}`, innerName: "v", op: ParamDefault},
		{src: `echo ${${v}:1}`, innerName: "v", op: ParamSubstring},
		{src: `echo ${#${v}}`, innerName: "v", length: true},
		{src: `echo ${(U)${v}}`, innerName: "v", flags: "U"},
		{src: `echo ${(U)${v}#a}`, innerName: "v", op: ParamTrimPrefix, flags: "U"},
		// A flag group on the inner belongs to the inner, so the outer node
		// carries none.
		{src: `echo ${${(U)v}}`, innerName: "v"},
	} {
		e := firstParam(t, tc.src, nestingDialect())
		if e.Inner == nil {
			t.Errorf("%q: Inner is nil, want the nested expansion", tc.src)
			continue
		}
		if e.Name != "" {
			t.Errorf("%q: Name = %q, want it empty — the inner is the whole name position", tc.src, e.Name)
		}
		if got := innerParamName(e); got != tc.innerName {
			t.Errorf("%q: inner names %q, want %q", tc.src, got, tc.innerName)
		}
		if e.Op != tc.op {
			t.Errorf("%q: Op = %v, want %v", tc.src, e.Op, tc.op)
		}
		if e.Length != tc.length {
			t.Errorf("%q: Length = %v, want %v", tc.src, e.Length, tc.length)
		}
		if e.Flags != tc.flags {
			t.Errorf("%q: Flags = %q, want %q", tc.src, e.Flags, tc.flags)
		}
	}
}

// innerParamName is the name the inner expansion reads, for a test that wants
// to say which parameter ended up inside.
func innerParamName(e *ParamExpr) string {
	if e.Inner == nil || len(e.Inner.Spans) != 1 {
		return ""
	}
	s := e.Inner.Spans[0]
	if s.Kind != ParamExp || s.Param == nil {
		return ""
	}
	return s.Param.Name
}

// The inner need not be a parameter expansion: a command substitution and an
// arithmetic one stand in the same position, measured.
func TestANestedInnerMayBeAnySubstitution(t *testing.T) {
	for _, tc := range []struct {
		src  string
		kind SpanKind
	}{
		{`echo ${$(echo x)#a}`, CommandSubst},
		{`echo ${$((1+1))#a}`, ArithSubst},
	} {
		e := firstParam(t, tc.src, nestingDialect())
		if e.Inner == nil || len(e.Inner.Spans) != 1 {
			t.Errorf("%q: no single inner span", tc.src)
			continue
		}
		if got := e.Inner.Spans[0].Kind; got != tc.kind {
			t.Errorf("%q: inner kind %v, want %v", tc.src, got, tc.kind)
		}
		if e.Op != ParamTrimPrefix {
			t.Errorf("%q: Op = %v, want the trim after the inner", tc.src, e.Op)
		}
	}
}

// The backquoted spelling of a command substitution is *not* one of them.
// Measured: `${`+"`"+`echo x`+"`"+`#a}` is a bad substitution in the shell with the
// grammar, so the two spellings part company here even though everything else
// treats them alike.
func TestABackquotedInnerIsNotANestedExpansion(t *testing.T) {
	e := firstParam(t, "echo ${`echo x`#a}", nestingDialect())
	if e.Inner != nil {
		t.Error("Inner is set, want the shape refused")
	}
	if !e.Bad {
		t.Error("read as a shape, want it marked bad")
	}
}

// The inner is a *braced* expansion or a parenthesized substitution, and
// nothing else that begins with a dollar.
//
// Measured: `${$v}` and `${$v#a}` are a bad substitution in the shell with the
// grammar even though `$v` is a perfectly good expansion on its own, so the
// braces are part of the shape. `${$'x'}` is refused for a second reason —
// the quoting carries the dollar and the span is literal text — and `${$}` is
// the parameter named `$`, which is not this construct at all.
func TestANestedInnerNeedsItsBracesOrParentheses(t *testing.T) {
	for _, src := range []string{
		`echo ${$v}`,
		`echo ${$v#a}`,
		"echo ${$'ab'#a}",
	} {
		e := firstParam(t, src, nestingDialect())
		if e.Inner != nil {
			t.Errorf("%q: Inner is set, want the shape refused", src)
		}
		if !e.Bad {
			t.Errorf("%q: read as a shape, want it marked bad", src)
		}
	}
	// And the one that only looks like it: `$` is a parameter name.
	e := firstParam(t, `echo ${$}`, nestingDialect())
	if e.Inner != nil {
		t.Error("${$} read as a nested expansion")
	}
	if e.Name != "$" {
		t.Errorf("${$} names %q, want the parameter $", e.Name)
	}
}

// Depth: the inner may itself be nested, and each level keeps its own
// operator.
func TestNestedParamExpansionsNest(t *testing.T) {
	e := firstParam(t, `echo ${${${v}#a}%c}`, nestingDialect())
	if e.Op != ParamTrimSuffix {
		t.Fatalf("outer Op = %v, want the suffix trim", e.Op)
	}
	mid := e.Inner.Spans[0].Param
	if mid == nil || mid.Op != ParamTrimPrefix {
		t.Fatalf("middle Op = %v, want the prefix trim", mid.Op)
	}
	if got := innerParamName(mid); got != "v" {
		t.Fatalf("innermost names %q, want v", got)
	}
}

// Text either side of the nesting is not a longer name. Measured: the shell
// that has the construct refuses both spellings, so this is the grammar's
// boundary rather than an implementation limit.
func TestTextBesideANestedInnerIsNotReadAsAName(t *testing.T) {
	for _, src := range []string{`echo ${x${v}}`, `echo ${${v}x}`} {
		e := firstParam(t, src, nestingDialect())
		if !e.Bad {
			t.Errorf("%q: read as a shape, want it marked bad", src)
		}
	}
}

// Without the flag the same characters are not this shape, and the dialect
// that refuses while reading says so with the character it stopped at.
func TestNestingNeedsTheGrammarFlag(t *testing.T) {
	d := Core()
	e := firstParam(t, `echo ${${v}#a}`, d)
	if e.Inner != nil {
		t.Error("Inner is set without the flag")
	}
	if !e.Bad {
		t.Error("read as a shape without the flag, want it marked bad")
	}

	strict := Core()
	strict.BadSubstitutionAtParseTime = true
	if _, err := Parse(`echo ${${v}#a}`, strict); err == nil {
		t.Error("parsed under a grammar that refuses while reading")
	} else if !strings.Contains(err.Error(), "${${v}#a}") {
		t.Errorf("error %q does not name the expansion", err)
	}
}

// A subscript after the inner brace is *read* rather than left to make the
// operator scan fail. Leaving it would call the whole expansion unreadable,
// which is a claim about the grammar and not about this implementation; the
// run says which construct is missing instead.
func TestASubscriptAfterANestedInnerIsRead(t *testing.T) {
	e := firstParam(t, `echo ${${v}[2]}`, nestingDialect())
	if e.Inner == nil {
		t.Fatal("Inner is nil")
	}
	if e.Bad {
		t.Error("marked bad, want the subscript read")
	}
	if e.Index == nil {
		t.Fatal("Index is nil, want the subscript")
	}
	if got := e.Index.Literal(); got != "2" {
		t.Errorf("Index = %q, want 2", got)
	}
}

// The inner expansion is read in the quoting the outer was written in.
//
// Whether a bare `{` opens a level inside `${…}` is BareBraceNestsInExpansion
// *and* the quoting: the flag is only consulted outside double quotes, because
// the panel agrees that a quoted `{` is an ordinary character. The nested
// spelling was reached through a lexer built with no quoting at all, so the
// quoted half was read as if the word stood bare and the inner expansion ran
// past the `}` that closes it.
//
// The word is what says so: read correctly, `"${${:-${w::=a{b}c}}+}"` is an
// expansion and then the two characters `+}`, because the brace before them
// closed the outer one. Read as if unquoted, the whole of it is one expansion
// and nothing is left over.
func TestANestedExpansionTakesTheOuterQuoting(t *testing.T) {
	d := nestingDialect()
	d.BareBraceNestsInExpansion = true

	const src = `echo "${${:-${w::=a{b}c}}+}"`
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	spans := sc.Args[1].Spans
	if len(spans) != 2 {
		t.Fatalf("%q: %d spans, want an expansion and the text after it: %+v", src, len(spans), spans)
	}
	if spans[0].Kind != ParamExp {
		t.Errorf("%q: first span is %v, want the expansion", src, spans[0].Kind)
	}
	if spans[1].Kind != Literal || spans[1].Value != "+}" {
		t.Errorf("%q: text after the expansion is %v %q, want the literal %q",
			src, spans[1].Kind, spans[1].Value, "+}")
	}
}
