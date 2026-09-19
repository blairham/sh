// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

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
		// The keyword standing alone is not a call with an empty body: it
		// runs nothing at all. The status is therefore the one it found —
		// `false; function; echo $?` prints 1 where `false; function { };
		// echo $?` prints 0, measured 2026-09-19 — and a redirection written
		// after it is a redirection with no command, null command and all:
		// `echo hi | function > f` puts `hi` in the file. Both fall out of
		// running the empty simple command rather than of a rule of its own,
		// which is what says the two are one construct.
		//
		// The empty command is only reached where there is something to
		// redirect: with no redirection at all there is nothing for it to
		// do, and the null command would then be run by a line that named no
		// file.
		if len(c.Redirs) == 0 {
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
	return r.withRedirs(ctx, c.Redirs, func() error {
		name := r.anonymousFunctionName()
		var args []string
		for _, w := range c.Args {
			args = append(args, r.expandWord(w)...)
		}
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
