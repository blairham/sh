// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"sync"
	"sync/atomic"
)

// Body is the definition's body: the command the function runs, and what a
// listing of it prints.
//
// It is the one way to read a body, because a body need not exist yet. A
// declaration the parse cache read back holds its body still encoded and
// decodes it here, the first time anything asks — a call, `functions`,
// `whence -c`, `$functions[name]` — since most of the functions a start
// defines are never called during it (#6483). Every caller sees the same
// body, decoded once, whichever goroutine asks first.
//
// Nil is a definition the parser refused, as it always was.
func (c *FuncDecl) Body() Command {
	if d := c.deferred; d != nil {
		d.once.Do(d.decode)
		return d.cmd
	}
	return c.BodyCommand
}

// DeferBody makes c's body one that load makes the first time [FuncDecl.Body]
// is asked for it, and end the position [FuncDecl.End] answers until then and
// after. It is for a reader of encoded trees and nothing else: BodyCommand is
// left as it is, and is never consulted again.
//
// load runs at most once. It must return the body a parse would have made,
// and has nowhere to report a failure, because by the time a body is asked
// for the definition has long since run: a reader that cannot promise to
// decode what it encoded must not defer.
func (c *FuncDecl) DeferBody(end Pos, load func() Command) {
	c.deferred = &deferredBody{end: end, load: load}
}

// Deferred reports whether c's body is still encoded, without decoding it:
// one that has been asked for is decoded and no longer deferred. For tests of
// the reader and of the callers that must not decode.
func (c *FuncDecl) Deferred() bool {
	d := c.deferred
	return d != nil && !d.done.Load()
}

type deferredBody struct {
	end  Pos
	once sync.Once
	load func() Command
	cmd  Command
	done atomic.Bool
}

func (d *deferredBody) decode() {
	d.cmd = d.load()
	d.load = nil
	d.done.Store(true)
}

// HasBody reports whether the definition has a body, without decoding one
// that is still encoded: only a definition the parser refused has none.
func (c *FuncDecl) HasBody() bool {
	return c.deferred != nil || c.BodyCommand != nil
}
