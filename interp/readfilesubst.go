// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/blairham/sh/syntax"
)

// readFileSubstitution reports whether a command substitution's body is the
// special form `<file` and nothing else, and returns the redirection it is.
//
// The test is narrow on purpose, and every clause of it was measured rather
// than assumed. What disqualifies a body is exactly what the shells that have
// the form say disqualifies it:
//
//   - a command word — `$(<f echo hi)` runs `echo` with the file on its
//     input, unanimously, and prints `hi`;
//   - an assignment prefix — `$(x=1 <f)` is empty in all five shells that
//     have the form;
//   - anything else in the list — `$(<f; :)` and `$(:; <f)` are empty in
//     bash and ksh93. zsh prints the file for both, but by its
//     `READNULLCMD` route rather than by this one, which is a separate
//     mechanism and a separate question;
//   - a second redirection — `$(<f <g)` is empty in bash, so the form is one
//     redirection rather than the first of several;
//   - a descriptor other than standard input — `$(3<f)` is empty in zsh
//     5.9.2, bash 5.3 and ksh93. An explicit `0` is standard input written
//     out and is still the form;
//   - a here-document or here-string body — the operator has to be a plain
//     `<`, since `$(<<<hi)` is not this form even in the shells that give it
//     a value;
//   - a `&` on the end, or a `!` in front of the pipeline, because then the
//     body is a job or a negation rather than a redirection.
//
// Measured 2026-09-10 across zsh 5.9.2, bash 5.3, bash 3.2, bash as `sh`,
// ksh93 and dash.
func readFileSubstitution(f *syntax.File) (*syntax.Redirect, bool) {
	if len(f.Stmts) != 1 {
		return nil, false
	}
	st := f.Stmts[0]
	if st.Background || st.Disown || st.Coprocess {
		return nil, false
	}
	p, ok := st.Expr.(*syntax.Pipeline)
	if !ok || p.Negated || len(p.Cmds) != 1 {
		return nil, false
	}
	c, ok := p.Cmds[0].(*syntax.SimpleCmd)
	if !ok {
		return nil, false
	}
	if len(c.Args) != 0 || len(c.Assigns) != 0 || len(c.Precommands) != 0 || len(c.Redirs) != 1 {
		return nil, false
	}
	rd := c.Redirs[0]
	if rd.Op != syntax.TokLess || rd.Heredoc != nil || rd.Word == nil || rd.PipeBoth {
		return nil, false
	}
	// Standard input, written out or left out. Anything else — a number, a
	// `{name}` the shell would pick — is a descriptor the form does not
	// cover.
	if rd.N != nil && rd.N.Literal() != "0" {
		return nil, false
	}
	return rd, true
}

// readFileSubst is `$(<file)`: the file's contents, with no command run.
//
// The read goes through applyRedirs rather than opening the file here, and
// that is the whole design of this function. Everything a redirection already
// knows how to do is what this form needs: the operand is expanded and
// tilde-resolved the way a target is, a name that will not open is reported
// in the dialect's own words at the dialect's own status, the sandbox gate
// sees the open and can refuse it, and the audit stream records it. Opening
// the file here would have been four lines and a second place for every one
// of those rules to be forgotten — the shape a second helper always takes.
//
// The bytes are then copied straight from the stream the redirection aimed at
// standard input. Nothing is executed, which is what the form means: no
// process, no `cat`, no PATH lookup, and no null-command hook.
func (r *Runner) readFileSubst(ctx context.Context, rd *syntax.Redirect, span syntax.Span) string {
	var out bytes.Buffer
	sub := r.clone()
	sub.inheritJobs(jobBoundarySubstitution)
	sub.inCommandSubst = true
	sub.lineBase = r.lineBase + span.Pos.Line - 1
	if span.Backquoted && r.diag().BackquotedSubstitutionRestartsLines {
		sub.lineBase = 0
	}
	// A read that works reports success, whatever the status before it was:
	// measured, `false; v=$(<f); echo $?` is 0 in every shell that has the
	// form. The clone starts with this runner's status, so leaving it alone
	// would have let the previous command's failure travel through a
	// substitution that succeeded.
	sub.status = 0
	closers, err := sub.applyRedirs(ctx, []*syntax.Redirect{rd}, false, false)
	if err == nil && !sub.redirErr && !sub.unspecified {
		// Before the closers run: the first of them puts the saved streams
		// back and the rest close the file this is reading.
		//
		// A read that fails part way — the file is a directory, which is the
		// case that reaches here — keeps what it got and says nothing, which
		// is what bash 5.3, ksh93 and dash do. zsh has a sentence for it and
		// that is a disagreement of its own rather than part of this form.
		_, _ = io.Copy(&out, sub.Stdin)
	}
	for _, cl := range closers {
		_ = cl.Close()
	}
	if err != nil {
		r.diagf("%v\n", err)
		return ""
	}
	r.status = sub.status
	// The same trailing-newline rule every command substitution follows, and
	// measured to be the same one: a file holding `A\n\n\n` substitutes as
	// `A` in all five shells that have the form.
	return strings.TrimRight(out.String(), "\n")
}
