// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// One shell opens `${ cmd;}` on a `(` as well as on a blank, so `${(echo hi)}`
// is a command list whose first command is a subshell (#2615).
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01 against bash 5.3.15, bash 3.2.57,
// bash as `sh`, zsh 5.9.2, dash and BusyBox ash: `echo ${(echo hi)}` prints
// `hi` in ksh93 and is a bad substitution or a flag error in all six others.
// bash *has* the construct and still refuses the paren, which is why this is
// an opener of one column's rather than part of CurrentShellSubstitution.
//
// `syntax.isBraceCommandStart` used to say a blank, a tab and a newline were
// the only openers there could be, "a parameter name may not begin with any
// of them". The premise is sound and the conclusion was not: a `(` cannot
// begin a name either.

func braceParenGrammar() syntax.Dialect {
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	d.CurrentShellSubstitutionTakesAParen = true
	return d
}

// braceSpan returns the kind of the last expansion in a one-command program,
// and whether it was read as running in the current shell.
func braceSpan(t *testing.T, src string, d syntax.Dialect) (syntax.SpanKind, bool) {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var kind syntax.SpanKind
	var current bool
	for _, st := range f.Stmts {
		for _, cmd := range st.Expr.(*syntax.Pipeline).Cmds {
			for _, w := range cmd.(*syntax.SimpleCmd).Args {
				for _, sp := range w.Spans {
					if sp.Kind == syntax.CommandSubst || sp.Kind == syntax.ParamExp {
						kind, current = sp.Kind, sp.CurrentShell
					}
				}
			}
		}
	}
	return kind, current
}

func TestAParenOpensACurrentShellBody(t *testing.T) {
	d := braceParenGrammar()
	for _, c := range []struct {
		name, src string
		command   bool
	}{
		{"the bare spelling", "echo ${(echo hi)}", true},
		{"blanks inside the parens change nothing", "echo ${( echo hi )}", true},
		{"and it may stand inside a word", `echo "A${(echo hi)}B"`, true},
		{"a list of more than one command", "echo ${(a; b)}", true},
		// The blank openers are the construct's own and are unaffected.
		{"a blank still opens one", "echo ${ echo hi;}", true},
		// And a name is still a parameter: the flag adds one character to
		// the opener and does not make `${` guess.
		{"a name is still a parameter", "echo ${x}", false},
		{"and so is an operator", "echo ${x:-d}", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			kind, current := braceSpan(t, c.src, d)
			if got := kind == syntax.CommandSubst && current; got != c.command {
				t.Errorf("read as a current-shell command = %v, want %v (kind %v)",
					got, c.command, kind)
			}
		})
	}
}

// Without the paren flag the same text is a parameter expansion, which is what
// every other column in the panel makes of it — a bad substitution deferred to
// the run rather than a refusal while reading. The row is here so the flag
// cannot be deleted and leave the tests green.
func TestWithoutTheParenFlagItIsAParameter(t *testing.T) {
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	kind, current := braceSpan(t, "echo ${(echo hi)}", d)
	if kind != syntax.ParamExp {
		t.Errorf("kind = %v, want a parameter expansion", kind)
	}
	if current {
		t.Error("read as a current-shell command, want a parameter")
	}
}

// The paren is an *opener* and not a whole construct: with neither a blank nor
// a paren after the brace, ksh93 refuses the line rather than falling back to
// reading a body. Measured — `echo ${echo hi;}` is a syntax error there.
func TestOnlyABlankOrAParenOpensABody(t *testing.T) {
	kind, current := braceSpan(t, "echo ${echo hi;}", braceParenGrammar())
	if kind == syntax.CommandSubst && current {
		t.Error("read as a current-shell command, want a parameter that will not resolve")
	}
}
