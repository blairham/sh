// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// arithmeticOnlyBody is a substitution body that is nothing but one `(( … ))`
// command, and the expression it holds.
//
// The whole body, which is what makes it a *shape* rather than a special case
// for the last command: one statement, no terminator of its own beyond the
// one that ends it, no redirection and not backgrounded. `$( (( 0 )); echo hi )`
// is an ordinary substitution and so is `$( echo hi; (( 0 )) )`.
func arithmeticOnlyBody(f *syntax.File) (*syntax.ArithCmdClause, bool) {
	if f == nil || len(f.Stmts) != 1 {
		return nil, false
	}
	st := f.Stmts[0]
	if st.Background {
		return nil, false
	}
	pipe, ok := st.Expr.(*syntax.Pipeline)
	if !ok || pipe.Negated || len(pipe.Cmds) != 1 {
		return nil, false
	}
	c, ok := pipe.Cmds[0].(*syntax.ArithCmdClause)
	if !ok || len(c.Redirs) > 0 {
		return nil, false
	}
	return c, true
}

// arithmeticBodySubstitution is a substitution whose body is that one command,
// expanded as the *arithmetic expansion* one dialect reads it as, and reports
// whether it answered.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null:
//
//	echo "[$( (( 1+1 )) )]"            [2]
//	n=0; echo "[$( (( n+=5 )) )]"; $n  [5], and n is 5 afterwards
//	echo "[${ (( 1+1 )); }]"           [2]
//	echo "[$( (( 1+1 )); echo hi )]"   [hi]
//	echo "[$( echo hi; (( 1+1 )) )]"   [hi]
//	echo "[$( (( 1+1 )) >/dev/null )]" refused: `>' unexpected
//	echo "[$( (( 1+1 )) & )]"          []
//	echo "[`(( 1+1 ))`]"               []
//
// Three of those rows are the discriminators. The value is the expression's,
// which no command substitution could produce — `(( … ))` writes nothing. The
// *assignment escapes*, which no subshell could allow. And a redirection
// written after it is a syntax error, which is what a command substitution
// would have taken without complaint. So the body is not being run at all: it
// is being read as `$(( … ))`, and the backquoted spelling, a body with a
// second command in it and a backgrounded one are all ordinary substitutions.
//
// #3364 filed the visible half — `set -e; x=$( (( 0 )) )` not stopping — as
// #3348's rule about a zero `(( … ))` reaching through a substitution's own
// copy of the shell. It is not that: the expansion is not a command, so there
// is no status for `set -e` to judge, which is also why `x=$( ( (( 0 )) ) )`
// and `x=$(f() { (( 0 )); }; f)` *do* stop there — both are substitutions with
// a command in them.
func (r *Runner) arithmeticBodySubstitution(f *syntax.File, span syntax.Span) (string, bool) {
	if span.Backquoted {
		// The older spelling is an ordinary substitution, measured above,
		// which is what keeps this a property of the newer one rather than
		// of arithmetic.
		return "", false
	}
	c, ok := arithmeticOnlyBody(f)
	if !ok {
		return "", false
	}
	if !r.ask(r.sem().ArithmeticOnlyBodyIsAnArithmeticExpansion,
		"a substitution whose whole body is `(( … ))` being an arithmetic expansion") {
		return "", false
	}
	// The same span an expansion written `$(( … ))` would have carried, so
	// the reading, the refusals, the output format and the hold are the one
	// implementation. A second evaluator here is how this family grows a
	// helper that omits the first one's fix.
	v, got := r.arithSpanValue(syntax.Span{Kind: syntax.ArithSubst, Arith: c.Parsed, Value: c.Expr, Pos: span.Pos})
	if !got {
		return "", true
	}
	return v, true
}
