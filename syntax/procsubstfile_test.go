// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// fileSubstDialect is the core plus the one flag, so that nothing here
// depends on a shell's preset.
func fileSubstDialect() syntax.Dialect {
	d := syntax.Core()
	d.ProcessSubstitutionToFile = true
	return d
}

// `=(cmd)` is a word the way `<(cmd)` is, and the `=` is not an operator —
// which is why this is decided inside the word scanner rather than in front
// of the operator table. Measured 2026-09-11 on zsh 5.9.2.
func TestFileProcessSubstitutionScansAsAWord(t *testing.T) {
	for _, tc := range []struct {
		name, src, inner string
	}{
		{"an argument", `cat =(echo hi)`, "echo hi"},
		{"empty", `cat =()`, ""},
		{"parens inside", `cat =(echo a; (echo b))`, "echo a; (echo b)"},
		{"a substitution inside", `cat =(echo $(date))`, "echo $(date)"},
		{"a redirection target", `cat <=(echo hi)`, "echo hi"},
		{"an assignment value", `f==(echo hi)`, "echo hi"},
		{"a subscripted assignment value", `f[1]==(echo hi)`, "echo hi"},
		{"an appending assignment value", `f+==(echo hi)`, "echo hi"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, fileSubstDialect())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			span, ok := findFileSubst(f)
			if !ok {
				t.Fatalf("no %v span in %q", syntax.ProcSubstFile, tc.src)
			}
			if span.Value != tc.inner {
				t.Errorf("inner = %q, want %q", span.Value, tc.inner)
			}
		})
	}
}

// Where it does *not* open is the other half of the grammar, and the half a
// naive "an `=` before a `(`" rule gets wrong. Every row is measured on zsh
// 5.9.2: the accepting ones by what they print, the refusing ones by
// `missing end of string`.
func TestFileProcessSubstitutionOpensOnlyAtAWordOrAValue(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      bool
	}{
		{"at the front of a word", `cat =(echo hi)`, true},
		{"with text behind it", `echo =(echo hi)x`, true},
		{"at the front of an array element", `a=(=(echo hi))`, true},
		{"at the front of an assignment value", `a==(echo hi)`, true},

		{"mid-word", `echo x=(echo hi)`, false},
		{"after a second equals", `echo a=b=(echo hi)`, false},
		{"backslashed", `echo \=(echo hi)`, false},
		{"double quoted", `echo "=(echo hi)"`, false},
		{"single quoted", `echo '=(echo hi)'`, false},
		{"a second one in the same word", `echo =(echo hi)=(echo ho)`, false},
		{"an assignment-shaped argument", `echo a==(echo hi)`, false},
		// A redirection written *before* any word, because that is the
		// only redirection read outside argument position — `cat >
		// a==(echo hi)` is already covered by the argument rule and would
		// have left this pinning nothing. Found by mutation.
		{"a redirection target's value", `> a==(echo hi) cat`, false},
		// Two `=` and not one: `case a=(b) in` is the array-literal
		// question rather than this one, and asking it here pinned nothing.
		// Measured, `case a==(echo hi) in *)` matches in zsh 5.9.2 with no
		// command run and no file made, so the subject is text.
		{"a case subject", `case a==(echo hi) in *) echo hit;; esac`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, fileSubstDialect())
			got := false
			if err == nil {
				_, got = findFileSubst(f)
			}
			if got != tc.want {
				t.Errorf("%q opened a substitution = %v, want %v (err = %v)", tc.src, got, tc.want, err)
			}
		})
	}
}

// The construct belongs to one dialect, so the core reads the same text as a
// word ending at its parenthesis and refuses what follows.
func TestFileProcessSubstitutionIsADialectFeature(t *testing.T) {
	if _, err := syntax.Parse(`cat =(echo hi)`, syntax.Core()); err == nil {
		t.Error("parse succeeded, want a refusal where the dialect has no such construct")
	}
	if _, err := syntax.Parse(`cat =(echo hi)`, fileSubstDialect()); err != nil {
		t.Errorf("parse with the flag on: %v", err)
	}
}

// It prints back as it was written. A span the printer has no case for comes
// out as its literal text, which reads as an ordinary word and would make the
// formatter silently rewrite the construct away.
func TestFileProcessSubstitutionPrintsBack(t *testing.T) {
	const src = "cat =(echo hi)"
	f, err := syntax.Parse(src, fileSubstDialect())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := syntax.Print(f); got != src {
		t.Errorf("print = %q, want %q", got, src)
	}
}

// An unfinished one leaves the input incomplete and names the two characters
// it was opened with, so a continuation prompt can say what it is waiting
// for. Without a case of its own the kind describes itself in prose instead —
// "process substitution", which is not something anybody typed.
func TestFileProcessSubstitutionRunsOutNamingItsOpener(t *testing.T) {
	p := syntax.NewParser("cat =(echo hi", fileSubstDialect())
	p.Parse()
	if !p.Incomplete() {
		t.Fatal("the input was complete, want it still inside the substitution")
	}
	open := p.Open()
	if len(open) == 0 {
		t.Fatal("nothing recorded as open, want the substitution")
	}
	if got := open[len(open)-1].Word; got != "=(" {
		t.Errorf("open = %q, want %q", got, "=(")
	}
}

// findFileSubst looks in every place one of these tests puts a word.
//
// The `case` subject is in the list because leaving it out made a row
// vacuous rather than wrong: it asserts that a subject is *not* a
// substitution, and a search that never looked at subjects answered "not
// found" whatever the lexer had done. Found by mutation — the row stayed
// green with the position gate removed.
func findFileSubst(f *syntax.File) (syntax.Span, bool) {
	var words []*syntax.Word
	walk := func(c syntax.Command) {
		switch c := c.(type) {
		case *syntax.SimpleCmd:
			words = append(words, c.Args...)
			for _, rd := range c.Redirs {
				words = append(words, rd.Word)
			}
			for _, a := range c.Assigns {
				words = append(words, a.Value)
				words = append(words, a.Elems...)
			}
		case *syntax.CaseClause:
			words = append(words, c.Word)
		}
	}
	for _, st := range f.Stmts {
		p, ok := st.Expr.(*syntax.Pipeline)
		if !ok {
			continue
		}
		for _, c := range p.Cmds {
			walk(c)
		}
	}
	for _, w := range words {
		if w == nil {
			continue
		}
		for _, s := range w.Spans {
			if s.Kind == syntax.ProcSubstFile {
				return s, true
			}
		}
	}
	return syntax.Span{}, false
}
