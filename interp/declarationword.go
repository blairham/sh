// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// DeclarationCommandWordReading is how a declaration utility's name has to be
// written for its `name=value` operands to be assignments — see
// [Semantics.DeclarationCommandWord].
type DeclarationCommandWordReading uint8

const (
	// DeclarationByUtilityName applies the rule whenever the command that
	// runs is a declaration utility, however its name was reached: through
	// an expansion, a quoted spelling, or a precommand modifier in front of
	// it.
	//
	// The zero value, because it is what every dialect did before the axis
	// existed, and dash really does answer this way — so it is a measurement
	// and not only a default.
	DeclarationByUtilityName DeclarationCommandWordReading = iota

	// DeclarationByUnquotedLiteralWord applies the rule only where the
	// utility's name is the command word as written — the first word of the
	// command in the source, one unquoted literal with no quoting of any
	// kind in it. The name is a reserved word there, recognized when the
	// line is read, so anything the parser cannot see as that word — `$cmd`,
	// `\typeset`, `'typeset'`, `type"set"`, `noglob typeset`, `builtin
	// typeset` — runs the builtin with ordinary, split arguments.
	//
	// An alias and the reserved `nocorrect` in front both keep the rule:
	// the one is replaced before the word is read and the other is not a
	// word of the command at all. Measured on zsh 5.9.2.
	DeclarationByUnquotedLiteralWord
)

// declarationWordWritten reports whether a command whose argv begins with a
// declaration utility was written in a way the dialect recognizes as one.
//
// The first word as written is enough to say which word produced argv[0]: an
// unquoted literal with no expansion in it produces exactly itself, so where
// it equals the utility's name it is the word that named it, and where argv[0]
// came from anywhere else — an expansion, a word behind an empty one, a word
// behind a precommand modifier — the first word is not that name.
//
// Asked only for a command that already declares, so the common path — any
// other command, and a declaration utility in the dialects that key the rule
// on its name — never reaches the word.
func (r *Runner) declarationWordWritten(c *syntax.SimpleCmd, name string) bool {
	if r.sem().DeclarationCommandWord != DeclarationByUnquotedLiteralWord {
		return true
	}
	return len(c.Args) > 0 && unquotedLiteral(c.Args[0]) == name
}

// unquotedLiteral is a word's text where every span of it is unquoted literal
// text, and "" where any is quoted or is a substitution.
func unquotedLiteral(w *syntax.Word) string {
	for _, s := range w.Spans {
		if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
			return ""
		}
	}
	return w.Literal()
}
