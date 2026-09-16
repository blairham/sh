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
	// an expansion, a quoted spelling, or a word in front of it that
	// expanded to nothing.
	//
	// The zero value, and dash's and BusyBox ash's answer.
	DeclarationByUtilityName DeclarationCommandWordReading = iota

	// DeclarationByUnquotedLiteralWord applies the rule only where the
	// utility's name is the command word as written — the first word of the
	// command in the source, one unquoted literal with no quoting of any
	// kind in it. Anything the parser cannot see as that word — `$cmd`,
	// `\typeset`, `'typeset'`, `type"set"`, `noglob typeset`, `builtin
	// typeset` — runs the builtin with ordinary, split arguments.
	//
	// An alias and zsh's reserved `nocorrect` in front both keep the rule:
	// the one is replaced before the word is read and the other is not a
	// word of the command at all. zsh's and bash's answer.
	DeclarationByUnquotedLiteralWord

	// DeclarationByWrittenWord applies the rule where the utility's name is
	// the command word as written, quoted or not — `\typeset`, `'typeset'`
	// and `type"set"` all keep it — but not where any part of the word is an
	// expansion, and not where the name arrives from a later word behind an
	// empty expansion. ksh93's answer.
	DeclarationByWrittenWord
)

// declarationCommand reports whether a command whose expanded words so far
// are argv is a declaration utility the dialect recognizes as one, so that a
// `name=value` word after them is an assignment.
//
// The common path is a command that is no declaration utility at all, and it
// reads nothing but argv[0].
func (r *Runner) declarationCommand(c *syntax.SimpleCmd, argv []string) bool {
	k := 0
	if !r.declares(argv[0]) {
		if argv[0] != "command" {
			return false
		}
		k = r.commandPrefixLength(argv)
		if k == len(argv) || !r.declares(argv[k]) {
			return false
		}
		if !r.ask(r.sem().CommandPrefixKeepsADeclaration,
			"a declaration utility run through `command`") {
			return false
		}
	}
	reading := r.sem().DeclarationCommandWord
	if reading == DeclarationByUtilityName {
		return true
	}
	// Every word up to and including the utility's name has to be written as
	// itself. Checking them in order is also what makes each comparison
	// meaningful: a word with no expansion in it produces exactly one field,
	// itself, so when every word before this one matched, this word is the
	// one that produced argv[j].
	if len(c.Args) <= k {
		return false
	}
	for j := 0; j <= k; j++ {
		if writtenAs(c.Args[j], reading) != argv[j] {
			return false
		}
	}
	return true
}

// commandPrefixLength is how many words at the front of argv are the
// `command` prefix, under the dialect's reading of how it must be written.
//
// Under the reading keyed on the utility's name, `-p` is part of the prefix —
// it changes where the utility is looked for and not which utility runs, and
// dash and BusyBox ash keep the rule through it. Under the readings keyed on
// the word as written it is not, and ksh93 splits `command -p typeset a=$b`.
func (r *Runner) commandPrefixLength(argv []string) int {
	byName := r.sem().DeclarationCommandWord == DeclarationByUtilityName
	k := 0
	for k < len(argv) {
		switch {
		case argv[k] == "command":
			k++
		case byName && k > 0 && argv[k] == "-p" && argv[k-1] == "command":
			k++
		default:
			return k
		}
	}
	return k
}

// writtenAs is a word's text under a reading of how a declaration's words
// must be written, and "" where the word does not qualify: a substitution
// anywhere in it disqualifies it under both readings, and any quoting at all
// under the unquoted one.
func writtenAs(w *syntax.Word, reading DeclarationCommandWordReading) string {
	for _, s := range w.Spans {
		if s.Kind != syntax.Literal {
			return ""
		}
		if reading == DeclarationByUnquotedLiteralWord && s.Quoting != syntax.Unquoted {
			return ""
		}
	}
	return w.Literal()
}
