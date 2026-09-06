// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// An expression that will not read does not refuse the file.
//
// Measured unanimous: `bash -n`, `ksh -n` and `zsh -n` all take `((echo hi))`,
// and dash has no arithmetic *command* but defers inside `$(( ))` too. So this
// is the core's answer and not a dialect's.
//
// The raw text stays on the node either way, because that is what a diagnostic
// quotes when the command finally runs — and the tree is nil, which is the
// state an expression holding an expansion has always been left in.
func TestAnUnreadableExpressionDoesNotRefuseTheFile(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		text string
		why  string
	}{
		{
			name: "an arithmetic command",
			src:  "((echo hi))",
			text: "echo hi",
			why:  "the construct the bug report named",
		},
		{
			name: "an arithmetic command with an operand missing",
			src:  "((1+))",
			text: "1+",
			why:  "the other way an expression fails, so this is not about one shape of text",
		},
		{
			name: "an arithmetic substitution",
			src:  `echo "$((echo hi))"`,
			text: "echo hi",
			why:  "the substitution form, which reaches it through a word's spans",
		},
		{
			name: "the older bracket spelling",
			src:  `echo $[echo hi]`,
			text: "echo hi",
			why:  "the same span kind, so one line covers both spellings",
		},
		{
			name: "a C-style for header",
			src:  "for ((echo hi;;)); do :; done",
			text: "echo hi",
			why:  "bash, ksh93 and zsh all take this under -n and all reach past one in a branch that never runs",
		},
		{
			name: "a float where the dialect has none",
			src:  "echo $((1.5))",
			text: "1.5",
			why:  "a grammar flag inside an expression gates the read the run does, not the read the file does",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := Core()
			d.DollarBracketArith = true
			f, err := Parse(c.src, d)
			if err != nil {
				t.Fatalf("%q: %v\n%s", c.src, err, c.why)
			}
			// The text is kept and the tree is not built, which together are
			// what the interpreter needs: it reads the text when it runs.
			if got := arithTextOf(t, f); got != c.text {
				t.Errorf("kept %q, want %q\n%s", got, c.text, c.why)
			}
			if err := arithErr(c.text, d); err == nil {
				t.Errorf("%q reads cleanly; the case is not about an expression that fails", c.text)
			}
		})
	}
}

// And an expression that *does* read keeps the tree the read built, so the
// deferral costs a well-formed program nothing.
func TestAReadableExpressionKeepsItsTree(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an arithmetic command", "((1 + 2))", "(1 + 2)"},
		{"an arithmetic substitution", "echo $((1 + 2))", "(1 + 2)"},
		{"a C-style for header", "for ((i=0;;)); do break; done", "(i = 0)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse(c.src, Core())
			if err != nil {
				t.Fatalf("%q: %v", c.src, err)
			}
			tree := arithTreeOf(t, f)
			if tree == nil {
				t.Fatalf("%q: no tree; a readable expression should keep the one the read built", c.src)
			}
			if got := arith(tree); got != c.want {
				t.Errorf("%q: tree is %s, want %s", c.src, got, c.want)
			}
		})
	}
}

// arithTextOf is the raw expression text the one arithmetic node in f carries.
func arithTextOf(t *testing.T, f *File) string {
	t.Helper()
	text, _ := arithNodeOf(t, f)
	return text
}

// arithTreeOf is the tree that node carries, or nil where the read failed.
func arithTreeOf(t *testing.T, f *File) ArithExpr {
	t.Helper()
	_, tree := arithNodeOf(t, f)
	return tree
}

// arithNodeOf finds the one arithmetic node in f — a command, a for header, or
// a span inside a word — and returns its text and its tree.
func arithNodeOf(t *testing.T, f *File) (string, ArithExpr) {
	t.Helper()
	for _, st := range f.Stmts {
		p, ok := st.Expr.(*Pipeline)
		if !ok {
			continue
		}
		for _, cmd := range p.Cmds {
			switch x := cmd.(type) {
			case *ArithCmdClause:
				return x.Expr, x.Parsed
			case *ForArithClause:
				return x.InitText, x.Init
			case *SimpleCmd:
				for _, w := range x.Args {
					for _, s := range w.Spans {
						if s.Kind == ArithSubst {
							return s.Value, s.Arith
						}
					}
				}
			}
		}
	}
	t.Fatalf("no arithmetic node in the tree")
	return "", nil
}
