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
	return r.withRedirs(ctx, c.Redirs, func() error {
		name := anonFuncName
		if n := r.diag().AnonymousFunctionName; n != "" {
			name = n
		}
		var args []string
		for _, w := range c.Args {
			args = append(args, r.expandWord(w)...)
		}
		return r.callFunc(ctx, &syntax.FuncDecl{
			Name: name, Keyword: c.Keyword, Body: c.Body, Start: c.Start,
		}, args)
	})
}
