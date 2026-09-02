// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package repl is the interactive surface: a prompt, a line editor, and the
// loop between them.
//
// It is the part that makes this a shell someone types into rather than a
// library that runs scripts. Everything it needs from the substrate was
// already there — the parser reports whether input ended part-way through a
// construct, which is what a continuation prompt is, and the interpreter runs
// one statement at a time — so this package is the terminal and the loop, and
// borrows the rest.
package repl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Shell is one interactive session.
type Shell struct {
	// Runner is the interpreter the lines are run in. It carries the state a
	// session accumulates — variables, functions, the working directory — so
	// it is the caller's rather than made here.
	Runner *interp.Runner

	// Dialect is the grammar lines are parsed with.
	Dialect syntax.Dialect

	// In, Out and Err are the terminal. In has to be one for the editor to
	// work; Run says so rather than guessing otherwise.
	In       *os.File
	Out, Err io.Writer

	// Report renders a parse failure the way this dialect does. Nil prints
	// the error plainly, which is what a caller without a dialect gets.
	Report func(err error) string
}

// Run reads, evaluates and prints until the input ends.
//
// It returns the status of the last command, which is what the shell exits
// with — the same thing a script's last command decides.
func (s Shell) Run(ctx context.Context) (int, error) {
	if !isTerminal(s.In) {
		return 0, errNotTerminal
	}
	state, err := makeRaw(s.In)
	if err != nil {
		return 0, err
	}
	// Deferred rather than restored at each return: a panic here would
	// otherwise leave the terminal with echo off, which is a broken terminal
	// and not merely a crash.
	defer func() { _ = state.restore() }()

	ed := &editor{in: s.In, out: s.Out}
	var pending strings.Builder
	for {
		prompt := s.prompt("PS1", "$ ")
		if pending.Len() > 0 {
			prompt = s.prompt("PS2", "> ")
		}
		line, err := ed.readLine(prompt)
		switch {
		case errors.Is(err, ErrInterrupted):
			// ^C abandons whatever was half-typed, including the earlier
			// lines of an unfinished construct — which is the whole point of
			// it at a continuation prompt.
			pending.Reset()
			continue
		case errors.Is(err, io.EOF):
			return s.status(), nil
		case err != nil:
			return s.status(), err
		}
		pending.WriteString(line)
		pending.WriteString("\n")
		text := pending.String()

		p := syntax.NewParser(text, s.Dialect)
		stmts, perr := collect(p)
		if (perr != nil && p.Incomplete()) || endsWithContinuation(text) {
			// Not an error: the construct has not finished. This is the one
			// thing an interactive shell needs from a parser that a script
			// runner does not, and it was already there.
			ed.remember(line)
			continue
		}
		pending.Reset()
		ed.remember(strings.TrimSuffix(text, "\n"))
		if perr != nil {
			s.errf("%s", s.report(perr))
			continue
		}
		if s.run(ctx, state, stmts) {
			return s.status(), nil
		}
	}
}

// run executes the statements of one accepted line, reporting whether the
// shell should stop.
//
// The terminal goes back to its own line discipline first. A command is not
// the editor: it may want echo, it may want ^C to interrupt it, and it will
// print lines that need the terminal translating them.
func (s Shell) run(ctx context.Context, state *terminalState, stmts []*syntax.File) bool {
	if err := state.restore(); err != nil {
		s.errf("%v\n", err)
	}
	defer func() {
		if _, err := makeRaw(s.In); err != nil {
			s.errf("%v\n", err)
		}
	}()
	for _, st := range stmts {
		if err := s.Runner.RunPart(ctx, st); err != nil {
			// Refused rather than silently skipped, the same way the script
			// driver does it.
			s.errf("%v\n", err)
			return false
		}
		if s.Runner.Exited() {
			return true
		}
	}
	return false
}

// endsWithContinuation reports whether the text ends with a backslash joining
// it to a line that has not been typed yet.
//
// The parser cannot answer this, and is right not to. `echo one \` at the end
// of a *file* is a finished command — the continuation joins it to nothing and
// the shells all print `one`. At a terminal the same text is a promise, and
// every shell in the panel gives a continuation prompt rather than running it.
// The difference is not in the text, so it belongs here, where the difference
// lives.
//
// Counted rather than looked for: `echo \\` ends with a backslash that is
// itself escaped, and is a finished command that prints one.
func endsWithContinuation(text string) bool {
	text = strings.TrimSuffix(text, "\n")
	n := 0
	for i := len(text) - 1; i >= 0 && text[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// collect reads every statement the parser can make of the text.
//
// The error is returned rather than reported, because the caller has to ask
// whether the input was merely unfinished before deciding it was wrong.
func collect(p *syntax.Parser) ([]*syntax.File, error) {
	var stmts []*syntax.File
	for {
		line, ok := p.NextLine()
		if !ok {
			return stmts, p.Err()
		}
		if err := p.Err(); err != nil {
			return nil, err
		}
		stmts = append(stmts, line)
	}
}

// prompt reads one of the prompt parameters, falling back to the usual text.
func (s Shell) prompt(name, fallback string) string {
	if v, ok := s.Runner.Vars[name]; ok && v != "" {
		return v
	}
	return fallback
}

func (s Shell) status() int { return s.Runner.ExitStatus() }

func (s Shell) report(err error) string {
	if s.Report != nil {
		return s.Report(err)
	}
	return err.Error() + "\n"
}

func (s Shell) errf(format string, a ...any) {
	if s.Err == nil {
		return
	}
	_, _ = fmt.Fprintf(s.Err, format, a...)
}
