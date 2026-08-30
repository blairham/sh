// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command sh will be the shell. It is not one yet.
//
// What exists today is the lexer, so this exposes that and refuses everything
// else rather than pretending. It is a development tool that will grow into
// the real thing as the pieces land, which is also how it becomes a column in
// the oracle panel: that harness needs something it can hand a snippet to.
//
//	sh -tokens 'echo hi'    # dump the token stream
//	sh -parse 'a && b'      # dump the syntax tree
//	sh -parse -f script.sh
//	sh -c 'echo hi'         # not yet: there is no interpreter
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blairham/sh/internal/syntax"
)

const (
	exitOK      = 0
	exitFailure = 2
)

func main() {
	var (
		tokens  = flag.Bool("tokens", false, "print the token stream and exit")
		parse   = flag.Bool("parse", false, "print the syntax tree and exit")
		command = flag.String("c", "", "run the given command (not implemented)")
		file    = flag.String("f", "", "read from this file instead of an argument")
		dialect = flag.String("dialect", "core", "core, posix or bash")
	)
	flag.Parse()

	d, err := pickDialect(*dialect)
	if err != nil {
		fail(err)
	}

	src, err := source(*file, flag.Args())
	if err != nil {
		fail(err)
	}

	switch {
	case *tokens:
		if err := dumpTokens(src, d); err != nil {
			fail(err)
		}
	case *parse:
		if err := dumpTree(src, d); err != nil {
			fail(err)
		}
	case *command != "":
		// Refused rather than silently doing nothing. Anything that ran this
		// expecting a shell should find out immediately.
		fail(fmt.Errorf("-c needs an interpreter, which does not exist yet; try -parse"))
	default:
		fail(fmt.Errorf("nothing to do: pass -tokens or -parse with a script, or -f file"))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "sh:", err)
	os.Exit(exitFailure)
}

func pickDialect(name string) (syntax.Dialect, error) {
	switch name {
	case "core":
		return syntax.Core(), nil
	case "posix":
		return syntax.POSIX(), nil
	case "bash":
		return syntax.Bash(), nil
	}
	return syntax.Dialect{}, fmt.Errorf("unknown dialect %q: want core, posix or bash", name)
}

func source(file string, args []string) (string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		return string(b), err
	}
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	return "", nil
}

// dumpTokens prints one token per line: position, kind, and for a word its
// spans with the quoting made visible, because the quoting is the part that
// decides what happens to a word later and the part hardest to see by eye.
func dumpTokens(src string, d syntax.Dialect) error {
	l := syntax.NewLexer(src, d)
	for {
		t := l.Next()
		if t.Kind == syntax.TokEOF {
			break
		}
		fmt.Printf("%-8s %-12s %s\n", t.Pos, kindName(t.Kind), detail(t))
	}
	if err := l.Err(); err != nil {
		if l.Incomplete() {
			return fmt.Errorf("%w (input ends unfinished)", err)
		}
		return err
	}
	return nil
}

func kindName(k syntax.Kind) string {
	switch k {
	case syntax.TokWord:
		return "word"
	case syntax.TokIONumber:
		return "io-number"
	case syntax.TokNewline:
		return "newline"
	case syntax.TokArithCmd:
		return "arith-cmd"
	}
	return "operator"
}

func detail(t syntax.Token) string {
	if t.Kind == syntax.TokArithCmd {
		return "expr(" + t.Text + ")"
	}
	if t.Kind != syntax.TokWord {
		return t.Kind.String()
	}
	parts := make([]string, 0, len(t.Spans))
	for _, s := range t.Spans {
		parts = append(parts, spanLabel(s)+"("+s.Value+")")
	}
	return strings.Join(parts, " + ")
}

// spanLabel names a span by what it is and, where it matters, by the quoting
// it sits in. A substitution shown as "plain" would read as literal text,
// which is the opposite of what this tool is for.
func spanLabel(s syntax.Span) string {
	switch s.Kind {
	case syntax.CommandSubst:
		return quotePrefix(s) + "cmd-subst"
	case syntax.ArithSubst:
		return quotePrefix(s) + "arith"
	case syntax.ParamExp:
		return quotePrefix(s) + "param"
	}
	switch s.Quoting {
	case syntax.SingleQuoted:
		return "single"
	case syntax.DoubleQuoted:
		return "double"
	case syntax.DollarSingleQuoted:
		return "dollar-single"
	}
	return "plain"
}

// quotePrefix marks a substitution that sits inside double quotes, because
// that is what decides whether its result is field-split afterwards.
func quotePrefix(s syntax.Span) string {
	if s.Quoting == syntax.DoubleQuoted {
		return "quoted-"
	}
	return ""
}

// dumpTree prints the syntax tree, indented.
func dumpTree(src string, d syntax.Dialect) error {
	p := syntax.NewParser(src, d)
	f := p.Parse()
	if err := p.Err(); err != nil {
		if p.Incomplete() {
			return fmt.Errorf("%w (input ends unfinished)", err)
		}
		return err
	}
	for _, st := range f.Stmts {
		printNode(st, 0)
	}
	return nil
}

func printNode(n syntax.Node, depth int) {
	pad := strings.Repeat("  ", depth)
	switch x := n.(type) {
	case *syntax.Stmt:
		label := "stmt"
		if x.Background {
			label = "stmt &"
		}
		fmt.Printf("%s%-8s %s\n", pad, x.Pos(), label)
		printNode(x.Expr, depth+1)
	case *syntax.BinaryExpr:
		fmt.Printf("%s%-8s %s\n", pad, x.OpPos, x.Op)
		printNode(x.X, depth+1)
		printNode(x.Y, depth+1)
	case *syntax.Pipeline:
		label := "pipeline"
		if x.Negated {
			label = "pipeline !"
		}
		fmt.Printf("%s%-8s %s\n", pad, x.Pos(), label)
		for _, c := range x.Cmds {
			printNode(c, depth+1)
		}
	case *syntax.SimpleCmd:
		fmt.Printf("%s%-8s command\n", pad, x.Pos())
		for _, a := range x.Assigns {
			fmt.Printf("%s  %-8s assign %s=%s\n", pad, a.Pos(), a.Name, a.Value.Literal())
		}
		for _, w := range x.Args {
			fmt.Printf("%s  %-8s word %s\n", pad, w.Pos(), w.Literal())
			printParams(w, pad+"    ")
		}
		printRedirs(x.Redirs, pad, depth)
	case *syntax.Subshell:
		fmt.Printf("%s%-8s subshell\n", pad, x.Pos())
		printList(x.List, depth+1)
		printRedirs(x.Redirs, pad, depth)
	case *syntax.Group:
		fmt.Printf("%s%-8s group\n", pad, x.Pos())
		printList(x.List, depth+1)
		printRedirs(x.Redirs, pad, depth)
	case *syntax.IfClause:
		fmt.Printf("%s%-8s if\n", pad, x.Pos())
		printBranch("cond", x.Cond, depth+1)
		printBranch("then", x.Then, depth+1)
		for _, e := range x.Elifs {
			printBranch("elif-cond", e.Cond, depth+1)
			printBranch("elif-then", e.Then, depth+1)
		}
		if x.HasElse {
			printBranch("else", x.Else, depth+1)
		}
		printRedirs(x.Redirs, pad, depth)
	case *syntax.LoopClause:
		kw := "while"
		if x.Until {
			kw = "until"
		}
		fmt.Printf("%s%-8s %s\n", pad, x.Pos(), kw)
		printBranch("cond", x.Cond, depth+1)
		printBranch("do", x.Body, depth+1)
		printRedirs(x.Redirs, pad, depth)
	case *syntax.ForClause:
		items := "(no word list — iterates the positional parameters)"
		if x.HasItems {
			var ws []string
			for _, w := range x.Items {
				ws = append(ws, w.Literal())
			}
			items = "in " + strings.Join(ws, " ")
		}
		fmt.Printf("%s%-8s for %s %s\n", pad, x.Pos(), x.Name, items)
		printBranch("do", x.Body, depth+1)
		printRedirs(x.Redirs, pad, depth)
	case *syntax.CaseClause:
		fmt.Printf("%s%-8s case %s\n", pad, x.Pos(), x.Word.Literal())
		for _, it := range x.Items {
			var pats []string
			for _, w := range it.Patterns {
				pats = append(pats, w.Literal())
			}
			fmt.Printf("%s  %-8s pattern %s %s\n", pad, it.Pos(),
				strings.Join(pats, "|"), it.Term)
			printList(it.Body, depth+2)
		}
		printRedirs(x.Redirs, pad, depth)
	case *syntax.ArithCmdClause:
		fmt.Printf("%s%-8s arithmetic %s\n", pad, x.Pos(), strings.TrimSpace(x.Expr))
		printRedirs(x.Redirs, pad, depth)
	case *syntax.FuncDecl:
		kw := ""
		if x.Keyword {
			kw = " (function keyword)"
		}
		fmt.Printf("%s%-8s func %s%s\n", pad, x.Pos(), x.Name, kw)
		printNode(x.Body, depth+1)
	default:
		fmt.Printf("%s%-8s %T\n", pad, n.Pos(), n)
	}
}

func printList(list []*syntax.Stmt, depth int) {
	for _, s := range list {
		printNode(s, depth)
	}
}

func printBranch(label string, list []*syntax.Stmt, depth int) {
	fmt.Printf("%s%s:\n", strings.Repeat("  ", depth), label)
	printList(list, depth+1)
}

func printRedirs(rs []*syntax.Redirect, pad string, depth int) {
	for _, r := range rs {
		n := ""
		if r.N != nil {
			n = r.N.Literal()
		}
		fmt.Printf("%s  %-8s redirect %s%s %s\n", pad, r.Pos(), n, r.Op, r.Word.Literal())
		if r.Heredoc != nil {
			kind := "expanded"
			if r.Heredoc.Spans[0].Quoting != syntax.Unquoted {
				kind = "literal"
			}
			for _, line := range strings.Split(strings.TrimRight(r.Heredoc.Literal(), "\n"), "\n") {
				fmt.Printf("%s    %-8s heredoc(%s) %s\n", pad, "", kind, line)
			}
		}
	}
	_ = depth
}

// printParams shows the parsed form of any ${ } inside a word. A word's
// Literal() flattens them, which is exactly what hides whether they were
// understood.
func printParams(w *syntax.Word, pad string) {
	for _, s := range w.Spans {
		if s.Kind != syntax.ParamExp || s.Param == nil {
			continue
		}
		e := s.Param
		desc := "param " + e.Name
		if e.Length {
			desc = "param length-of " + e.Name
		}
		if e.Indirect {
			desc = "param indirect " + e.Name
		}
		if e.Index != nil {
			desc += "[" + e.Index.Literal() + "]"
		}
		if e.Op != syntax.ParamNone {
			colon := ""
			if e.Colon {
				colon = ": (empty counts as unset)"
			}
			desc += fmt.Sprintf("  op %q%s", e.Op.String(), colon)
		}
		fmt.Printf("%s%s\n", pad, desc)
		if e.Arg != nil {
			fmt.Printf("%s  arg %s\n", pad, wordShape(e.Arg))
			printParams(e.Arg, pad+"    ")
		}
		if e.Arg2 != nil {
			fmt.Printf("%s  arg2 %s\n", pad, wordShape(e.Arg2))
		}
	}
}

// wordShape renders a word so a substitution in it is not mistaken for
// literal text. Literal() flattens them, which is what hides the difference
// that matters: an operand is a word and is expanded.
func wordShape(w *syntax.Word) string {
	var parts []string
	for _, s := range w.Spans {
		switch s.Kind {
		case syntax.CommandSubst:
			parts = append(parts, "$("+s.Value+")")
		case syntax.ArithSubst:
			parts = append(parts, "$(("+s.Value+"))")
		case syntax.ParamExp:
			parts = append(parts, "${"+s.Value+"}")
		default:
			parts = append(parts, s.Value)
		}
	}
	return strings.Join(parts, "")
}
