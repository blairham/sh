// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// `repeat N` runs its body a fixed number of times.
//
// The count is a *word*, expanded and then read as an arithmetic expression,
// and it is read once before the first iteration rather than every time round
// — which is what makes it a count rather than a condition. Measured: a count
// that is not a number, and a negative one, run the body no times and report
// success, so this is not an error path.

func (r *Runner) repeatClause(ctx context.Context, c *syntax.RepeatClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		r.status = 0
		n, ok := r.repeatCount(c.Count)
		if !ok {
			return nil
		}
		// A loop as far as `break` is concerned, which is what the count
		// this raises is asked for: zsh names `repeat` among the loops a
		// `break` may be in, and without this a `break` inside one would be
		// reported as having none (#1236).
		defer r.enteringLoop()()
		for i := int64(0); i < n; i++ {
			r.traceForIteration(c.Header, "", "")
			if err := r.runList(ctx, c.Body); err != nil {
				return err
			}
			if stop := r.loopControl(); stop {
				return nil
			}
		}
		return nil
	})
}

// repeatCount reads the count, reporting whether the loop runs at all.
//
// Not an error when it will not: `repeat x { … }` and `repeat -1 { … }` print
// nothing, report 0 and carry on, which is the same answer as `repeat 0`.
func (r *Runner) repeatCount(w *syntax.Word) (int64, bool) {
	if w == nil {
		return 0, false
	}
	r.beginHeading()
	fields := r.expandOneWord(w)
	if r.failedHeading() {
		// The count is this loop's heading, so a failure in it costs the
		// loop. Before the emptiness test below, which would otherwise read
		// a failed expansion as a count of zero and exit 0 (#1215).
		return 0, false
	}
	if len(fields) == 0 || fields[0] == "" {
		return 0, false
	}
	tree, perr := r.arithTree(nil, fields[0])
	if perr != nil {
		// A count that is not a *number* is not an error — `repeat x` is a
		// variable that is unset, which is zero — but a count that is not an
		// expression at all is one, and it is the arithmetic failure the
		// dialect already words: `repeat '1+'` says the same thing `$(( 1+ ))`
		// says.
		r.diagf("%s\n", r.diag().ParseFailure(perr))
		r.status = 1
		return 0, false
	}
	n, err := r.evalArith(tree)
	if err != nil {
		return 0, false
	}
	// A count of zero or less needs no test of its own: the loop counts up
	// to it and runs no times, which is the answer measured for `repeat 0`
	// and `repeat -1` alike. A guard for it was a line no test could
	// distinguish.
	return int64(n), true
}
