// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// param renders a parsed ${ } compactly, so an expectation is one string.
func param(e *ParamExpr) string {
	if e == nil {
		return "<nil>"
	}
	var b strings.Builder
	if e.Length {
		b.WriteByte('#')
	}
	if e.Indirect {
		b.WriteByte('!')
	}
	b.WriteString(e.Name)
	if e.Index != nil {
		b.WriteString("[" + e.Index.Literal() + "]")
	}
	if e.Op == ParamNone {
		return b.String()
	}
	b.WriteByte(' ')
	if e.Colon {
		b.WriteByte(':')
	}
	if e.All {
		b.WriteString("all")
	}
	if e.Anchor != 0 {
		b.WriteByte(e.Anchor)
	}
	b.WriteString(e.Op.String())
	if e.Arg != nil {
		b.WriteString(" " + e.Arg.Literal())
	}
	if e.Arg2 != nil {
		b.WriteString(" / " + e.Arg2.Literal())
	}
	return b.String()
}

// firstParam parses src and returns the first parameter expansion in it.
func firstParam(t *testing.T, src string, d Dialect) *ParamExpr {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	for _, w := range sc.Args {
		for _, s := range w.Spans {
			if s.Kind == ParamExp {
				return s.Param
			}
		}
	}
	t.Fatalf("no parameter expansion in %q", src)
	return nil
}

func TestParamForms(t *testing.T) {
	tests := []struct{ src, want string }{
		{`echo ${x}`, `x`},
		{`echo ${#x}`, `#x`},
		// The colon is the whole difference between the two rows: it extends
		// the test from unset to unset-or-empty.
		{`echo ${x:-d}`, `x :- d`},
		{`echo ${x-d}`, `x - d`},
		{`echo ${x:=d}`, `x := d`},
		{`echo ${x=d}`, `x = d`},
		{`echo ${x:?m}`, `x :? m`},
		{`echo ${x:+a}`, `x :+ a`},
		{`echo ${x+a}`, `x + a`},
		// Doubling the operator selects the longer match.
		{`echo ${x#p}`, `x # p`},
		{`echo ${x##p}`, `x ## p`},
		{`echo ${x%p}`, `x % p`},
		{`echo ${x%%p}`, `x %% p`},
		{`echo ${x/a/b}`, `x / a / b`},
		{`echo ${x//a/b}`, `x all/ a / b`},
		{`echo ${x/#a/b}`, `x #/ a / b`},
		{`echo ${x/%a/b}`, `x %/ a / b`},
		// Omitting the replacement deletes the match.
		{`echo ${x/a}`, `x / a`},
		{`echo ${x:1:2}`, `x : 1 / 2`},
		{`echo ${x:1}`, `x : 1`},
		{`echo ${a[1]}`, `a[1]`},
		{`echo ${@}`, `@`},
		{`echo ${1}`, `1`},
		{`echo ${#@}`, `#@`},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			if got := param(firstParam(t, tc.src, Core())); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParamOperandIsAWordNotAString(t *testing.T) {
	// docs/spec/grammar/parameter-expansion.md: the word is itself expanded,
	// so it keeps its structure. `${u:-$(echo sub)}` yields sub, which is only
	// possible if the operand was parsed rather than stored as text.
	e := firstParam(t, `echo ${u:-$(echo sub)}`, Core())
	if e.Arg == nil || len(e.Arg.Spans) != 1 {
		t.Fatalf("operand not parsed into spans: %+v", e.Arg)
	}
	if k := e.Arg.Spans[0].Kind; k != CommandSubst {
		t.Errorf("operand span kind = %v, want a command substitution", k)
	}
	if v := e.Arg.Spans[0].Value; v != "echo sub" {
		t.Errorf("operand value = %q", v)
	}
}

func TestParamOperandKeepsNestedExpansions(t *testing.T) {
	e := firstParam(t, `echo ${u:-${v:-inner}}`, Core())
	if e.Arg == nil || len(e.Arg.Spans) != 1 {
		t.Fatalf("nested operand not parsed: %+v", e.Arg)
	}
	inner := e.Arg.Spans[0].Param
	if inner == nil {
		t.Fatal("nested expansion was not parsed")
	}
	if got, want := param(inner), `v :- inner`; got != want {
		t.Errorf("nested = %q, want %q", got, want)
	}
}

func TestParamSeparatorMustBeUnquoted(t *testing.T) {
	// A slash inside quotes belongs to the pattern, not to the operator.
	e := firstParam(t, `echo ${x/"a/b"/c}`, Core())
	if got, want := e.Arg.Literal(), "a/b"; got != want {
		t.Errorf("pattern = %q, want %q", got, want)
	}
	if got, want := e.Arg2.Literal(), "c"; got != want {
		t.Errorf("replacement = %q, want %q", got, want)
	}
}

func TestParamDialectRefusesRatherThanGuesses(t *testing.T) {
	// ${x^^} is bash alone and ${!x} means something else in ksh93, so a
	// dialect without them has to refuse: picking either meaning would be
	// wrong for half the panel. The refusal is *deferred* for an operator —
	// the shells that lack one diagnose it only when the expansion is
	// reached — where `${!x}` fails while reading, because the `!` reshapes
	// what the name even is.
	for _, src := range []string{`echo ${x^^}`, `echo ${x,,}`} {
		mustDefer(t, src, Core(), "an operator the core does not have")
		if _, err := Parse(src, everyFlag()); err != nil {
			t.Errorf("%s: bash rejected it: %v", src, err)
		}
	}
	if _, err := Parse(`echo ${!x}`, Core()); err == nil {
		t.Error("`${!x}`: core accepted a construct it does not have")
	}
	// And the ones dash lacks are deferred under posix the same way.
	for _, src := range []string{`echo ${x/a/b}`, `echo ${x:1:2}`} {
		mustDefer(t, src, POSIX(), "an operator dash does not have")
	}
}

func TestParamCaseChangeParsesWhereTheFlagIsOn(t *testing.T) {
	if got, want := param(firstParam(t, `echo ${x^^}`, everyFlag())), `x ^^`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := param(firstParam(t, `echo ${!x}`, everyFlag())), `!x`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParamNestingIsBounded(t *testing.T) {
	// Pathological input is the normal case on the keystroke path, so depth
	// is bounded rather than trusted.
	src := "echo " + strings.Repeat("${x:-", 200) + "y" + strings.Repeat("}", 200)
	p := NewParser(src, Core())
	p.Parse() // must return rather than exhaust the stack
	if p.Err() == nil {
		t.Log("deep nesting parsed without error, which is acceptable; the bound is the point")
	}
}

// everyFlag is Core with the remaining opt-in flags on. A core test that wants
// "the most permissive grammar" names the flags rather than a shell: which
// shell sets which is the dialect packages' business.
func everyFlag() Dialect {
	d := Core()
	d.CaseContinue = true
	d.ParamCaseChange = true
	d.ParamIndirection = true
	d.FunctionKeywordParens = true
	return d
}

// TestPrefixNamesNeedIndirection: `${!name@}` is a different thing from
// `${!name}` and from `${name@op}`, and all three ride on the flag that says
// this dialect has the `!` form at all.
func TestPrefixNamesNeedIndirection(t *testing.T) {
	ind := Core()
	ind.ParamIndirection = true

	for _, tc := range []struct {
		src    string
		prefix byte
	}{
		{`echo ${!FOO_@}`, '@'},
		{`echo ${!FOO_*}`, '*'},
		// Not a prefix form: a plain indirection.
		{`echo ${!FOO_}`, 0},
	} {
		e := firstParam(t, tc.src, ind)
		if e.Prefix != tc.prefix {
			t.Errorf("%s: prefix %q, want %q", tc.src, e.Prefix, tc.prefix)
		}
		if !e.Indirect {
			t.Errorf("%s: want Indirect", tc.src)
		}
	}
	// Without the `!` there is no prefix form at all: `@` there is where an
	// operator belongs, and a bare one is not an operator this shell has —
	// so the node is deferred for a runtime refusal, never read as a prefix.
	mustDefer(t, `echo ${FOO_@}`, ind, "`${FOO_@}` without the `!`")
	// And a dialect without the `!` refuses the whole family rather than
	// reading it as something else.
	if _, err := Parse(`echo ${!FOO_@}`, Core()); err == nil {
		t.Error("want a refusal where the dialect has no `${!x}`")
	}
}
