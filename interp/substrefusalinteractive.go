// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// substRefusalShape is how the dialect whose interactive shell speaks as at
// its prompt words a substitution body it refused. See
// Runner.interactiveSubstRefusal.
type substRefusalShape int

const (
	// substRefusalAsEver is every other shell's, and the interactive one's
	// where nothing below applies: the runner's own location.
	substRefusalAsEver substRefusalShape = iota
	// substRefusalBare is the shell's name and the sentence, and nothing
	// quoted back.
	substRefusalBare
	// substRefusalBareAtThePrompt is that, and then `syntax error` alone.
	substRefusalBareAtThePrompt
	// substRefusalInAFile is the file and its line, as a script that is not
	// interactive writes it, with the shell's name in front of each line.
	substRefusalInAFile
	// substRefusalInEvalInAFile is `eval` and its line, as a script that is
	// not interactive writes it, the shell's name already being its first
	// word.
	substRefusalInEvalInAFile
	// substRefusalBackquoted is the non-interactive wording, named by the
	// shell's last component.
	substRefusalBackquoted
)

// interactiveSubstRefusal chooses how a refused substitution body is worded,
// and returns what undoes the choice once the refusal has been written.
//
// A body is refused when the line holding it is read, so it is worded as a
// parse failure of the text it is in — the rule
// reportBorrowedParseFailure already follows for the interactive dialect —
// and not as a line typed at the prompt is. Measured 2026-10-06 on bash
// 5.3.20 (/opt/homebrew/bin/bash), `--norc`, with `echo $(fi)`:
//
//	typed at the prompt        bash: syntax error near unexpected token `fi'
//	                           while looking for matching `)'
//	                           bash: syntax error
//	under -i -c                the first line alone
//	eval'd at the prompt or    the first line alone
//	under -i -c
//	line 1 of an -i script     bash: m.sh: line 1: syntax error near …
//	                           bash: m.sh: line 1: `echo $(fi)'
//	line 2 of a file . read,   bash: ./g2: line 2: syntax error near …
//	at a prompt or -i -c       bash: ./g2: line 2: `echo $(fi)'
//	eval'd in an -i script     bash: eval: line 1: syntax error near …
//	                           bash: eval: line 1: `echo $(fi)'
//	`echo \`fi\`` anywhere      bash: command substitution: line 1: syntax
//	                           error near unexpected token `fi'
//	                           bash: command substitution: line 1: `fi'
//
// where this wrote the prompt's shape for all of them, with `-c` in front
// under -i -c and the line quoted back (#6279).
//
// lead is what goes in front of each line written: the shell's name where
// the location after it names a file, and nothing otherwise. It is taken
// before the prompt's wording is turned off, which is what changes the name.
func (r *Runner) interactiveSubstRefusal(span syntax.Span) (shape substRefusalShape, lead string, restore func()) {
	if !r.speaksAsAtAPrompt() {
		return substRefusalAsEver, "", func() {}
	}
	name := r.name()
	asWritten := func(shape substRefusalShape) (substRefusalShape, string, func()) {
		lead := ""
		if shape == substRefusalInAFile {
			lead = name + ": "
		}
		was, wasNamed, wasInstead := r.locatedAsWritten, r.namedAsWritten, r.locationNamesInstead
		r.locatedAsWritten, r.namedAsWritten = true, shape == substRefusalInAFile
		if shape != substRefusalInAFile && r.locationNamesInstead == "" {
			// Named as everything else this shell says is, where no file's
			// name takes the place.
			r.locationNamesInstead = name
		}
		return shape, lead, func() {
			r.locatedAsWritten, r.namedAsWritten, r.locationNamesInstead = was, wasNamed, wasInstead
		}
	}
	inFile := r.borrowedFiles > 0 || r.Route == RouteScriptFile
	switch {
	case span.Backquoted:
		return asWritten(substRefusalBackquoted)
	case r.locationIsInsideEvalText() && inFile:
		return asWritten(substRefusalInEvalInAFile)
	case r.locationIsInsideEvalText():
		return substRefusalBare, "", func() {}
	case inFile:
		return asWritten(substRefusalInAFile)
	case r.AtPrompt:
		return substRefusalBareAtThePrompt, "", func() {}
	}
	return substRefusalBare, "", func() {}
}
