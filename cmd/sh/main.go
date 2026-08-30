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
//	sh -tokens -f script.sh
//	sh -c 'echo hi'         # not yet: there is no parser or interpreter
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
	case *command != "":
		// Refused rather than silently doing nothing. Anything that ran this
		// expecting a shell should find out immediately.
		fail(fmt.Errorf("-c needs a parser and an interpreter, and neither exists yet; try -tokens"))
	default:
		fail(fmt.Errorf("nothing to do: pass -tokens with a script, or -tokens -f file"))
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
		if t.Kind == syntax.EOF {
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
	case syntax.Word:
		return "word"
	case syntax.IONumber:
		return "io-number"
	case syntax.Newline:
		return "newline"
	case syntax.ArithCmd:
		return "arith-cmd"
	}
	return "operator"
}

func detail(t syntax.Token) string {
	if t.Kind == syntax.ArithCmd {
		return "expr(" + t.Text + ")"
	}
	if t.Kind != syntax.Word {
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
