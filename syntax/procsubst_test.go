// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// `<(...)` and `>(...)` are a word, not an operator — which is the whole of
// the grammar question. The parser has no rule for them; the lexer has to
// decide, before it reaches the operator table, that a `<` followed by `(` is
// the start of a word rather than a redirection.
func TestProcessSubstitutionScansAsAWord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src string
		kind      syntax.SpanKind
		inner     string
	}{
		{"read from", `cat <(echo hi)`, syntax.ProcSubstIn, "echo hi"},
		{"write to", `echo x > >(tr a-z A-Z)`, syntax.ProcSubstOut, "tr a-z A-Z"},
		{"parens inside", `cat <(echo a; (echo b))`, syntax.ProcSubstIn, "echo a; (echo b)"},
		{"a substitution inside", `cat <(echo $(date))`, syntax.ProcSubstIn, "echo $(date)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			span, ok := findSpan(f, tc.kind)
			if !ok {
				t.Fatalf("no %v span in %q", tc.kind, tc.src)
			}
			// Unparsed, exactly as a command substitution is: what is inside
			// is a script, and it is the interpreter that decides which
			// dialect to read it in.
			if span.Value != tc.inner {
				t.Errorf("inner = %q, want %q", span.Value, tc.inner)
			}
		})
	}
}

// The dialect that does not have it reads the same text as a redirection whose
// target is another redirection, and says so. Not a special case anywhere —
// the lexer simply never takes the word branch.
func TestProcessSubstitutionIsADialectFeature(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.ProcessSubstitution = false
	for _, src := range []string{`cat <(echo hi)`, `echo x > >(cat)`} {
		if _, err := syntax.Parse(src, d); err == nil {
			t.Errorf("parse %q succeeded, want a refusal where the dialect has no such construct", src)
		}
	}
	if _, err := syntax.Parse(`cat <(echo hi)`, syntax.Core()); err != nil {
		t.Errorf("parse with the feature on: %v", err)
	}
}

// findSpan looks in the two places a word can be: an argument, and the target
// of a redirection.
func findSpan(f *syntax.File, kind syntax.SpanKind) (syntax.Span, bool) {
	sc := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	words := append([]*syntax.Word(nil), sc.Args...)
	for _, rd := range sc.Redirs {
		words = append(words, rd.Word)
	}
	for _, w := range words {
		if w == nil {
			continue
		}
		for _, s := range w.Spans {
			if s.Kind == kind {
				return s, true
			}
		}
	}
	return syntax.Span{}, false
}

// ProcessSubstitutionAgainstTheRedirectionIsRefused: a redirection target
// that begins with a substitution running against the operator is refused by
// its opener, and one running with it is read (#5514).
func TestASubstitutionAgainstItsRedirectionIsRefused(t *testing.T) {
	d := syntax.Core()
	d.ProcessSubstitutionAgainstTheRedirectionIsRefused = true
	for _, c := range []struct{ src, opener string }{
		{": <> >(cat)", ">("},
		{": < >(cat)", ">("},
		{": 3<> >(cat)", ">("},
		{": > <(echo)", "<("},
		{": >> <(echo)", "<("},
		{": >| <(echo)", "<("},
		{": <& <(echo)", "<("},
		{": >& >(cat)", ">("},
	} {
		_, err := syntax.Parse(c.src, d)
		if err == nil || !strings.Contains(err.Error(), "`"+c.opener+"' unexpected") {
			t.Errorf("%s: got %v, want %s refused", c.src, err, c.opener)
		}
		if _, err := syntax.Parse(c.src, syntax.Core()); err != nil {
			t.Errorf("%s without the flag: %v", c.src, err)
		}
	}
	for _, src := range []string{": < <(echo)", ": <> <(echo)", ": > >(cat)", ": >> >(cat)", "echo >(cat) <(echo)"} {
		if _, err := syntax.Parse(src, d); err != nil {
			t.Errorf("%s: %v, want it read", src, err)
		}
	}
}

// ProcessSubstitutionAgainstAPlainRedirectionIsRefused: a target that begins
// with a substitution running against a one-direction operator is refused by
// the whole word as written, and the append, the duplication, the read-write
// operator and a word the substitution does not begin are read (#6812).
func TestASubstitutionAgainstAPlainRedirectionIsRefusedByItsWord(t *testing.T) {
	d := syntax.Core()
	d.ProcessSubstitutionAgainstAPlainRedirectionIsRefused = true
	d.FdVariableRedirections = true
	for _, c := range []struct{ src, word string }{
		{": > <(cat)", "<(cat)"},
		{": >| <(cat)", "<(cat)"},
		{": 3> <(cat)", "<(cat)"},
		{": {v}> <(cat)", "<(cat)"},
		{": < >(cat)", ">(cat)"},
		{": 3< >(cat)", ">(cat)"},
		{": > <(cat)x", "<(cat)x"},
		{`: > <(cat)""`, `<(cat)""`},
	} {
		_, err := syntax.Parse(c.src, d)
		if err == nil || !strings.Contains(err.Error(), "`"+c.word+"' unexpected") {
			t.Errorf("%s: got %v, want %s refused", c.src, err, c.word)
		}
		if _, err := syntax.Parse(c.src, syntax.Core()); err != nil {
			t.Errorf("%s without the flag: %v", c.src, err)
		}
	}
	for _, src := range []string{
		": >> <(cat)", ": <> >(cat)", ": <> <(cat)", ": >& <(cat)", ": &> <(cat)",
		": > x<(cat)", ": > >(cat)", ": < <(cat)", ": > >(cat)x",
	} {
		if _, err := syntax.Parse(src, d); err != nil {
			t.Errorf("%s: %v, want it read", src, err)
		}
	}
}

// ProcessSubstitutionIsAWordOfItsOwn: the word before a substitution ends at
// its opener and the substitution's own word ends at its closer, and one
// touching a redirection's target is refused naming the `)` (#6813).
func TestASubstitutionIsAWordOfItsOwn(t *testing.T) {
	d := syntax.Core()
	d.ProcessSubstitution = true
	d.ProcessSubstitutionIsAWordOfItsOwn = true
	for _, c := range []struct {
		src   string
		words int
	}{
		{"echo <(true)x", 3},
		{"echo a<(true)", 3},
		{"echo <(true)<(true)", 3},
		{`echo "a"<(true)"b"`, 4},
		{"echo a<(true)b>(true)c", 6},
		{"cat < <(echo a)x", 2},
		{`echo "<(true)x"`, 2},
	} {
		f, err := syntax.Parse(c.src, d)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		call, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
		if !ok || len(call.Args) != c.words {
			t.Errorf("%s: %d words, want %d", c.src, len(call.Args), c.words)
		}
		plain := syntax.Core()
		plain.ProcessSubstitution = true
		if f, err := syntax.Parse(c.src, plain); err == nil && c.words > 2 {
			if n := len(f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd).Args); n == c.words {
				t.Errorf("%s without the flag: also %d words, so the row tells nothing", c.src, n)
			}
		}
	}
	for _, src := range []string{
		"cat < x<(echo a)", "cat <x<(echo a)", "cat < <(echo a)<(echo b)",
		"echo hi > x>(cat)", `cat < "x"<(echo a)`, "cat 3< x<(echo a)",
	} {
		_, err := syntax.Parse(src, d)
		if err == nil || !strings.Contains(err.Error(), "`)' unexpected") {
			t.Errorf("%s: got %v, want `)' unexpected", src, err)
		}
	}
}
