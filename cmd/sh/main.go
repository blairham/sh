// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command sh drives the substrate.
//
// It is not the product. This repository is the core — a parser and an
// interpreter other programs embed — and the shell people run, with its own
// choice of dialect and its own interactive surface, is a separate thing
// built on top. What is here exists to exercise the library, to make its
// behavior inspectable, and to be a column in the conformance harness.
//
// It runs scripts, and the -dialect flag chooses which shell it is being:
// core refuses anything the panel disagrees about, and the others answer the
// way that shell does. That is what makes it a column in the conformance
// harness, which needs something it can hand a snippet to.
//
//	sh -c 'echo hi'              # run a command
//	sh script.sh                 # run a script
//	sh -dialect bash -c '…'      # be bash where the shells differ
//	sh -tokens 'echo hi'         # dump the token stream
//	sh -parse 'a && b'           # dump the syntax tree
//
// It is still not the product. A shell people run needs an interactive
// surface — line editing, history, job control — and none of that is here.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

const (
	exitOK      = 0
	exitFailure = 2
)

func main() {
	var (
		tokens  = flag.Bool("tokens", false, "print the token stream and exit")
		parse   = flag.Bool("parse", false, "print the syntax tree and exit")
		command = flag.String("c", "", "run the given command")
		file    = flag.String("f", "", "read from this file instead of an argument")
		dialect = flag.String("dialect", "core", "core, posix, bash, zsh, ksh or dash")
	)
	flag.Parse()

	sh, err := pickDialect(*dialect)
	if err != nil {
		fail(err)
	}
	sh.Name = shellName()

	src, err := source(*file, flag.Args())
	if err != nil {
		fail(err)
	}

	switch {
	case *tokens:
		if err := dumpTokens(src, sh.Dialect); err != nil {
			fail(err)
		}
	case *parse:
		if err := dumpTree(src, sh.Dialect); err != nil {
			fail(err)
		}
	case *command != "":
		os.Exit(driver.Run(sh, *command, sh.Name))
	case len(flag.Args()) > 0:
		// A bare argument is a script to run, which is how a shell is
		// normally invoked and how the corpus runs the cases that depend on
		// being read from a file rather than from -c.
		os.Exit(runPath(sh, flag.Args()[0]))
	case *file != "":
		// -f names a file, and its help text says so, but until the run path
		// moved into driver only -tokens and -parse ever read it: `sh -f x.sh`
		// fell through to "nothing to do". It reads it as a script, which is
		// what it always claimed to do.
		os.Exit(runPath(sh, *file))
	default:
		fail(fmt.Errorf("nothing to do: pass a script, -c, -tokens or -parse"))
	}
}

// runPath runs a file, which is not the same as running its contents: a shell
// names the *script* in `$0` and in every diagnostic, not itself, and ksh93
// also changes how it names the line.
func runPath(sh driver.Shell, path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	return driver.RunScript(sh, string(b), path)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "sh:", err)
	os.Exit(exitFailure)
}

// shellName is what `$0` reports and what a diagnostic names itself with.
//
// Real shells use the path they were invoked by — dash says "/bin/dash: 1: …"
// — so a hardcoded "sh" was wrong twice: in `$0` inside a function, and at the
// start of every diagnostic. driver does the same for a binary that hands it
// the argument vector; this one parses its own flags, so it answers here.
func shellName() string {
	if len(os.Args) > 0 && os.Args[0] != "" {
		return os.Args[0]
	}
	return "sh"
}

// pickDialect resolves a name to a shell.
//
// The three vectors are chosen together because they answer different
// questions about the same shell: which constructs it accepts, what it means
// by them, and what it says when they fail. A driver.Shell is those three plus
// the two extension points, which is the whole of "which shell am I" — the
// same value each cmd/<shell> binary is built from, so this cannot answer
// differently from them.
//
// `core` is built the same way on both sides. The grammar refuses constructs
// not every shell has; the semantics refuses *behaviors* not every shell
// shares. A script that runs under it depends on nothing the panel disagrees
// about, which is a useful thing to be able to check and a poor way to run a
// shell — the same split docs/spec/core.md drew for strict POSIX.
func pickDialect(name string) (driver.Shell, error) {
	switch name {
	case "core":
		// Strict: what every shell agrees on is done, and anything they
		// disagree about is refused rather than silently given one shell's
		// answer. A portability check rather than a runtime.
		return driver.Shell{
			Dialect: syntax.Core(), Semantics: interp.CoreSemantics(),
			Diagnostics: interp.CoreDiagnostics(),
		}, nil
	case "posix":
		return driver.Shell{
			Dialect: syntax.POSIX(), Semantics: interp.PosixSemantics(),
			Diagnostics: interp.PosixDiagnostics(),
		}, nil
	case "bash":
		return driver.Shell{
			Dialect: bash.Dialect(), Semantics: bash.Semantics(),
			Diagnostics: bash.Diagnostics(), Register: bash.Apply,
		}, nil
	case "zsh":
		return driver.Shell{
			Dialect: zsh.Dialect(), Semantics: zsh.Semantics(),
			Diagnostics: zsh.Diagnostics(), Register: zsh.Apply,
		}, nil
	case "ksh":
		return driver.Shell{
			Dialect: ksh.Dialect(), Semantics: ksh.Semantics(),
			Diagnostics: ksh.Diagnostics(), Register: ksh.Apply,
		}, nil
	case "dash":
		return driver.Shell{
			Dialect: dash.Dialect(), Semantics: dash.Semantics(),
			Diagnostics: dash.Diagnostics(), Register: dash.Apply,
		}, nil
	}
	return driver.Shell{},
		fmt.Errorf("unknown dialect %q: want core, posix, bash, zsh, ksh or dash", name)
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
			switch {
			case a.IsArray:
				var els []string
				for _, e := range a.Elems {
					els = append(els, e.Literal())
				}
				fmt.Printf("%s  %-8s assign %s=(%s)\n", pad, a.Pos(), a.Name, strings.Join(els, " "))
			case a.Index != nil:
				fmt.Printf("%s  %-8s assign %s[%s]=%s\n", pad, a.Pos(), a.Name,
					a.Index.Literal(), a.Value.Literal())
			default:
				fmt.Printf("%s  %-8s assign %s=%s\n", pad, a.Pos(), a.Name, a.Value.Literal())
			}
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
	case *syntax.TestClause:
		fmt.Printf("%s%-8s test %s\n", pad, x.Pos(), condString(x.Expr))
		printRedirs(x.Redirs, pad, depth)
	case *syntax.ArithCmdClause:
		fmt.Printf("%s%-8s arithmetic %s\n", pad, x.Pos(), arithString(x.Parsed))
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
		if s.Kind == syntax.ArithSubst && s.Arith != nil {
			fmt.Printf("%sarith %s\n", pad, arithString(s.Arith))
			continue
		}
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

// arithString renders an expression fully parenthesised, so precedence is
// visible rather than implied. That is the point of parsing it at all: the
// source text was already in Expr.
func arithString(e syntax.ArithExpr) string {
	switch x := e.(type) {
	case nil:
		return "(unparsed)"
	case *syntax.ArithNum:
		return x.Text
	case *syntax.ArithVar:
		return x.Name
	case *syntax.ArithUnary:
		if x.Postfix {
			return "(" + arithString(x.X) + x.Op + ")"
		}
		return "(" + x.Op + arithString(x.X) + ")"
	case *syntax.ArithBinary:
		return "(" + arithString(x.X) + " " + x.Op + " " + arithString(x.Y) + ")"
	case *syntax.ArithCond:
		return "(" + arithString(x.Cond) + " ? " + arithString(x.Then) + " : " + arithString(x.Else) + ")"
	case *syntax.ArithAssign:
		return "(" + x.Name + " " + x.Op + " " + arithString(x.Value) + ")"
	}
	return "?"
}

// condString renders a condition fully parenthesised, so the precedence is
// visible — and inside [[ ]] it is not the precedence the same operators have
// outside, which is exactly the thing worth being able to see.
func condString(c syntax.CondExpr) string {
	switch x := c.(type) {
	case nil:
		return "(empty)"
	case *syntax.CondUnary:
		return "(" + x.Op + " " + x.X.Literal() + ")"
	case *syntax.CondBinary:
		rhs := x.Y.Literal()
		if !x.Y.IsQuoted() && (x.Op == "==" || x.Op == "=" || x.Op == "!=") {
			rhs += " [pattern]"
		}
		if !x.Y.IsQuoted() && x.Op == "=~" {
			rhs += " [regex]"
		}
		return "(" + x.X.Literal() + " " + x.Op + " " + rhs + ")"
	case *syntax.CondLogic:
		return "(" + condString(x.X) + " " + x.Op + " " + condString(x.Y) + ")"
	case *syntax.CondNot:
		return "(! " + condString(x.X) + ")"
	case *syntax.CondGroup:
		return "[" + condString(x.X) + "]"
	}
	return "?"
}
