// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A subscript's source text is kept, because a diagnostic quotes back what was
// typed and an expanded word no longer remembers its spelling.
//
// The same argument Redirect.Text is kept on, and the same shape: `i=-9;
// a[$i]=q` is `a[$i]: bad array subscript` in the column that names it, not
// `a[-9]` (#1373).
func TestAnAssignmentKeepsItsSubscriptAsWritten(t *testing.T) {
	d := syntax.Core()
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.AppendAssign = true
	d.ArraySubscriptFlags = true

	for _, tc := range []struct{ src, want string }{
		{`a[$i]=q`, `$i`},
		{`a[$i+0]=q`, `$i+0`},
		{`a[x-2]=q`, `x-2`},
		{`a[${i}]=q`, `${i}`},
		{`a[$((i))]=q`, `$((i))`},
		{`a[$(echo k)]=q`, `$(echo k)`},
		{`a["k"]=q`, `"k"`},
		{`a[i]+=q`, `i`},
		{`a[(r)$y]=Q`, `(r)$y`},
		{`a[1,2]=q`, `1,2`},
		// No subscript at all leaves it empty, which is what a reader falls
		// back from.
		{`a=q`, ``},
	} {
		f := syntax.NewParser(tc.src, d).Parse()
		got, ok := firstAssign(f)
		if !ok {
			t.Errorf("%s: no assignment parsed", tc.src)
			continue
		}
		if got.IndexText != tc.want {
			t.Errorf("%s: IndexText = %q, want %q", tc.src, got.IndexText, tc.want)
		}
	}
}

// And a parameter expansion's, which is the same field on the read side: the
// `${a[$i]:=v}` route writes an element and earns the same refusal.
func TestAParameterExpansionKeepsItsSubscriptAsWritten(t *testing.T) {
	d := syntax.Core()
	d.ArraySubscript = true
	d.ArraySubscriptFlags = true

	for _, tc := range []struct{ src, want string }{
		{`${a[$i]}`, `$i`},
		{`${a[x-2]}`, `x-2`},
		{`${a[@]}`, `@`},
		{`${a[(r)y]}`, `(r)y`},
		{`${a}`, ``},
	} {
		f := syntax.NewParser("echo "+tc.src, d).Parse()
		e, ok := firstParamExpr(f)
		if !ok {
			t.Errorf("%s: no parameter expansion parsed", tc.src)
			continue
		}
		if e.IndexText != tc.want {
			t.Errorf("%s: IndexText = %q, want %q", tc.src, e.IndexText, tc.want)
		}
	}
}

// The text is the source and not the position, so a subscript that stands on a
// later line or after another assignment is still its own.
func TestASubscriptAsWrittenIsCutFromItsOwnPlace(t *testing.T) {
	d := syntax.Core()
	d.ArraySubscript = true

	f := syntax.NewParser("a[$i]=q b[$j+1]=w\nc[k]=z", d).Parse()
	var got []string
	for _, st := range f.Stmts {
		p, ok := st.Expr.(*syntax.Pipeline)
		if !ok {
			continue
		}
		for _, c := range p.Cmds {
			sc, ok := c.(*syntax.SimpleCmd)
			if !ok {
				continue
			}
			for _, a := range sc.Assigns {
				got = append(got, a.IndexText)
			}
		}
	}
	want := []string{`$i`, `$j+1`, `k`}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("subscript %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func firstAssign(f *syntax.File) (*syntax.Assign, bool) {
	for _, st := range f.Stmts {
		p, ok := st.Expr.(*syntax.Pipeline)
		if !ok {
			continue
		}
		for _, c := range p.Cmds {
			sc, ok := c.(*syntax.SimpleCmd)
			if !ok {
				continue
			}
			if len(sc.Assigns) > 0 {
				return sc.Assigns[0], true
			}
		}
	}
	return nil, false
}

func firstParamExpr(f *syntax.File) (*syntax.ParamExpr, bool) {
	for _, st := range f.Stmts {
		p, ok := st.Expr.(*syntax.Pipeline)
		if !ok {
			continue
		}
		for _, c := range p.Cmds {
			sc, ok := c.(*syntax.SimpleCmd)
			if !ok {
				continue
			}
			for _, w := range sc.Args {
				for _, s := range w.Spans {
					if s.Param != nil {
						return s.Param, true
					}
				}
			}
		}
	}
	return nil, false
}
