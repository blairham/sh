// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// A function with no name, defined and run where it stands.
//
// It is a *function* and not a group, and the difference is observable: a
// `local` inside one is local, `$#` counts the words written after the body,
// and a `return` leaves it with a status. Only the name is missing, so the
// call needs one to report — the shell that has this invents `(anon)`, which
// is what a frame and `$0` show.

// anonFuncName is what a nameless function's frame reports. A dialect that
// spells it differently overrides it through Diagnostics.AnonymousFunctionName.
const anonFuncName = "(anon)"

func (r *Runner) anonFunc(ctx context.Context, c *syntax.AnonFunc) error {
	if c.Bare {
		// The keyword with nothing after it for it to take: no name, no
		// parameter list, no brackets, and no command in front of it in the
		// list. It runs nothing, and it leaves **0** behind for having run
		// nothing.
		//
		// The status used to be the one it found, and the measurement
		// written here for that was `false; function; echo $?` printing 1
		// against `false; function { }; echo $?` printing 0. **That probe
		// cannot fail.** The keyword takes the next command as its body —
		// see syntax.Parser.impliedAnonBody — so `echo $?` was the body, and
		// `$?` inside it is still the 1 the `false` left. Both readings
		// print 1, and it was quoted as the reason for the reading it cannot
		// distinguish (#5078).
		//
		// Measured 2026-09-28 on zsh 5.9.2 over routes that genuinely reach
		// this branch, because the keyword has nothing to take on any of
		// them:
		//
		//	false; { function ⏎ }; print $?          	0
		//	false; { function { } ⏎ }; print $?      	0
		//	for i in 1; do false; function; done; $? 	0
		//	`false` then `function` as the last line 	exit 0
		//
		// So it answers 0, exactly as the empty brace body does, and this
		// left 1 on all four. The status is the only thing the two spellings
		// agree on: a redirection written after the bare keyword is a
		// redirection with no command, null command and all — `print hi |
		// function > f` puts `hi` in the file, where `function { } > f` runs
		// an empty body and drops it.
		//
		// The empty command is only reached where there is something to
		// redirect: with no redirection at all there is nothing for it to
		// do, and the null command would then be run by a line that named no
		// file.
		// Traced before the give-up, because the call is what is traced and
		// the keyword standing alone is still a call: measured 2026-09-28
		// on zsh 5.9.2, `setopt xtrace; function` writes `+F:2> '(anon)'`
		// with nothing after it. Outside the redirection rather than inside
		// it, which is the difference between this branch and the other:
		// what carries the redirection here is the *null command*, and a
		// simple command's line is written before its files are opened.
		// Measured the same day, `function 2>f1` leaves `f1` empty and puts
		// both lines on the terminal, where `() { print A } 2>err.txt` puts
		// both in the file.
		r.traceAnonymousCall(r.anonymousFunctionName(), nil)
		if len(c.Redirs) == 0 {
			r.status = 0
			return nil
		}
		empty := &syntax.SimpleCmd{Start: c.Pos(), Stop: c.End()}
		empty.Redirs = c.Redirs
		// Through the call rather than beside it, so that a redirection
		// that will not open is reported against the name a nameless
		// function is given: `function <nosuchfile` says `(anon)` and not
		// the line it stands on, which is the shell telling us the frame is
		// pushed before the files are opened.
		return r.callFunc(ctx, &syntax.FuncDecl{
			Name: r.anonymousFunctionName(), Keyword: c.Keyword,
			Body: empty, Start: c.Start,
		}, nil)
	}
	return r.withRedirsOfABracketedCommand(ctx, c.Redirs, func() error {
		name := r.anonymousFunctionName()
		var args []string
		for _, w := range c.Args {
			args = append(args, r.expandWord(w)...)
		}
		if r.failedHeading() {
			// The words written after the body are a heading: they are
			// expanded before the construct decides what to run, and one
			// that failed means it does not run. Measured 2026-09-28 on zsh
			// 5.9.2, `() { print A } $((1/0))` reports the division and ends
			// the shell, where this reported it and then ran the body and
			// the line after it.
			//
			// This is also what keeps the trace off a call that never
			// happened: `() { print A } *nosuchthing*` under `xtrace` is
			// `no matches found` alone, with no line for the call.
			return nil
		}
		r.traceAnonymousCall(name, args)
		return r.callFunc(ctx, &syntax.FuncDecl{
			Name: name, Keyword: c.Keyword, Body: c.Body, Start: c.Start,
		}, args)
	})
}

// anonymousFunctionName is what a nameless function's frame reports, the
// dialect's own spelling of it where it has one.
func (r *Runner) anonymousFunctionName() string {
	if n := r.diag().AnonymousFunctionName; n != "" {
		return n
	}
	return anonFuncName
}

// traceAnonymousCall writes the `xtrace` line for a nameless function's call:
// the name it reports standing where a named function's own word would, with
// the words written after the body behind it.
//
// A named function needs nothing here — its call *is* a simple command, and
// the ordinary command line covers it. A nameless one is a compound command
// with no word of its own, so the line has to be written where the call is
// made. Measured 2026-09-28 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin`,
// a script file with standard input on the null device:
//
//	() { print A }         	+F:2> '(anon)'  then  +(anon):0> print A
//	() { print A } a b     	+F:2> '(anon)' a b
//	() { print A } "a b"   	+F:2> '(anon)' 'a b'
//	() { () { print i } }  	+F:2> '(anon)'  then  +(anon):0> '(anon)'
//
// The prefix is the *caller's*, which comes free: this runs before the frame
// is pushed, so `%N` is the file or the enclosing function and `%i` is the
// line the call stands on. The nested row is the proof — the inner call's
// line carries the outer function's name.
//
// The words are quoted one at a time rather than through
// Runner.traceCommandWords, which is the quoting a *simple command's* words
// get. That reader consults the operand state a declaration utility's
// expansion leaves behind — Runner.declarationOperands and the array
// operands — and none of it can apply here: the first word is a name this
// shell invented and the rest are a call's arguments, so a stale operand
// from whatever ran before would be the only thing it could ever find.
func (r *Runner) traceAnonymousCall(name string, args []string) {
	if !r.tracing() {
		return
	}
	d := r.diag()
	words := make([]string, 0, 1+len(args))
	words = append(words, name)
	words = append(words, args...)
	for i, w := range words {
		words[i] = r.traceQuote(w, d.TraceQuoting, d.TraceMetacharacters)
	}
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	r.tracef("%s%s\n", r.tracePrefix(), strings.Join(words, " "))
}
