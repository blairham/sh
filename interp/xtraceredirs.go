// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"strings"

	"github.com/blairham/sh/syntax"
)

// redirTrace is the line of its own that a command's redirections are traced
// on, in the dialect that writes one. See
// Semantics.TraceRedirectionsOnALineOfTheirOwn for the measurements.
//
// The line is written as the redirections are made rather than assembled and
// written once. PS4 goes first, before the first target is expanded, so a
// substitution in a target traces in the middle of it. Each redirection's
// piece, with the space after it or the line's newline, is written once its
// target has opened and before it is installed, to standard error as it stood
// then: `2>/dev/null` alone shows whole on the terminal, and after `2>/dev/null
// 1>&2` the piece for `1>&2` and the newline do not show. A redirection that
// fails writes nothing, and the line gets no newline: the next trace line
// carries on from where this one stopped.
//
// The piece is held until the next redirection starts or the list ends, and
// is written then to the stream it was held for. That is the same moment as
// far as anything the script can see is concerned: nothing is written
// between a target opening and its install, and a failure returns before the
// next iteration, which is why a failed redirection's piece is simply
// dropped.
type redirTrace struct {
	on   bool
	held string
	to   io.Writer
}

// beginRedirTrace writes the line's prefix, where this dialect has the line.
func (r *Runner) beginRedirTrace(rs []*syntax.Redirect) redirTrace {
	if len(rs) == 0 || !r.tracing() || r.sem().TraceRedirectionsOnALineOfTheirOwn != Yes {
		return redirTrace{}
	}
	r.awaitTraceTurn()
	r.tracef("%s", r.tracePrefix())
	r.releaseTraceTurn()
	return redirTrace{on: true}
}

// hold records the piece for the redirection being made, to be written to
// standard error as it stands now. last is whether no redirection follows,
// and ends says the piece ends its own line, as a here-document's does, so
// neither the space nor the newline follows it.
func (t *redirTrace) hold(r *Runner, piece string, last, ends bool) {
	if !t.on {
		return
	}
	switch {
	case ends:
	case last:
		piece += "\n"
	default:
		piece += " "
	}
	t.held, t.to = piece, r.lockedStderr()
}

// flush writes the piece being held, once the redirection it is for has been
// made.
func (t *redirTrace) flush() {
	if !t.on || t.to == nil {
		return
	}
	_, _ = io.WriteString(t.to, t.held)
	t.held, t.to = "", nil
}

// redirTracePiece is how one redirection is spelled on the line: the
// descriptor always written out, the operator, a space, and the target as it
// expanded, unquoted. `>& 1`, `<& -` and `<& 0-` keep the space too.
//
// ksh93u+ spells a `<>` with no number as `1<>`, though it opens standard
// input. That is a fact about the trace and not about the open, so it is
// spelled here and nowhere else.
func redirTracePiece(rd *syntax.Redirect, target string) string {
	var b strings.Builder
	switch lit := rd.N.Literal(); {
	case rd.N != nil:
		b.WriteString(lit)
	case rd.Op == syntax.TokLess || rd.Op == syntax.TokLessAmp ||
		rd.Op == syntax.TokTLess || rd.Op.IsHeredoc():
		b.WriteString("0")
	default:
		b.WriteString("1")
	}
	op := rd.Op
	if op == syntax.TokDLessDash {
		op = syntax.TokDLess
	}
	b.WriteString(op.String())
	if !op.IsHeredoc() {
		b.WriteString(" ")
	}
	b.WriteString(target)
	return b.String()
}

// heredocTracePiece is a here-document's piece: the body follows the
// operator, framed by its delimiter, and the piece ends its own line. An
// empty body leaves the operator alone, measured: `cat <<E 3>/dev/null`
// with nothing before the `E` traces `+ 0<<3> /dev/null`.
func heredocTracePiece(rd *syntax.Redirect, body string) string {
	if rd.Op == syntax.TokTLess {
		return redirTracePiece(rd, body)
	}
	if body == "" {
		return redirTracePiece(rd, "")
	}
	delim := rd.Word.Literal()
	return redirTracePiece(rd, " \\"+delim+"\n"+body+delim+"\n")
}
