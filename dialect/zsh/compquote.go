// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"

	"github.com/blairham/sh/interp"
)

// `compquote`: quote what is already in a parameter, for a completion
// function that adds its matches with `compadd -Q` and does the quoting
// itself.
//
// This is the one of the eight the manual calls useful outside the shipped
// tree: "Instead of interpreting the first character of the all_quotes key of
// the compstate special association and using the q flag for parameter
// expansions, one can use this builtin command. The arguments are the names
// of scalar or array parameters and the values of these parameters are quoted
// as needed for the innermost quoting level. If the -p option is given,
// quoting is done as if there is some prefix before the values of the
// parameters, so that a leading equal sign will not be quoted."
//
// Which is exactly repl's `Completion.Escape` with the word's own quotation
// taken into account — the same call `compadd` writes a match out through —
// minus the opening quote, because these values are going back into a
// parameter and not onto the line.
//
// `-p` costs nothing here: this shell does not escape a leading `=` in the
// first place, since `=` is not special to its grammar (see
// docs/spec/completion.md, where the character is measured against both
// shells), so the letter is accepted and changes nothing rather than being
// refused for a difference nobody can see.
//
// The status is the documented one: "non-zero in case of an error and zero
// otherwise", and a name that holds nothing is not an error.

func compquoteBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	// This is the one of the ten whose count is of *operands* rather than of
	// words: `-p` is parsed off first and does not count toward the one it
	// needs. Measured on zsh 5.9.2, 2026-09-16 — `compquote -p` and
	// `compquote -p -p` are both `not enough arguments`, and `compquote -p x`
	// and `compquote -p -p x` both reach the refusal about the place. Every
	// other one of the ten counts the words as given, which is why they call
	// compArity directly and this strips first.
	//
	// Ahead of the context check for compArity's reason: zsh answers the
	// count in the dispatcher, before the builtin body runs.
	for len(args) > 0 && args[0] == "-p" {
		args = args[1:]
	}
	if !compArity(r, args, 1, -1) {
		return 1
	}
	cs, _, ok := computilFrom(r, ctx)
	if !ok {
		return 1
	}
	for _, name := range args {
		if values, isArray := r.GetArray(name); isArray {
			quoted := make([]string, 0, len(values))
			for _, value := range values {
				quoted = append(quoted, cs.quoteForLine(value))
			}
			r.SetArray(name, quoted)
			continue
		}
		if value, set := r.GetVar(name); set {
			r.SetVar(name, cs.quoteForLine(value))
		}
	}
	return 0
}

// quoteForLine is one value written so that the parser reads it back as
// itself, in whatever quotation the word being completed is already inside.
//
// The opening quote `Escape` writes is taken off again: the caller is putting
// this back in a parameter it will hand to `compadd -Q`, which writes the
// quotation itself — see compadd.go's offer, where the same opening quote is
// added exactly once.
func (cs *completionState) quoteForLine(value string) string {
	quoted := cs.c.Escape(value)
	if cs.qiprefix != "" && len(quoted) > 0 && quoted[:1] == cs.qiprefix {
		return quoted[1:]
	}
	return quoted
}
