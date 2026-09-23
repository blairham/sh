// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"

	"github.com/blairham/sh/internal/histexpand"
)

// expanded rewrites one typed line the way the shell's history expander does,
// reporting whether the loop should go on to run it.
//
// The order is the measured one and every part of it is visible to a person at
// the prompt:
//
//   - nothing happens at all when the state is off, so a dialect without an
//     expander and a session that typed `set +H` both reach the parser with
//     what was typed;
//   - a reference the list does not hold is a complaint and the line does not
//     run — `bash: !nosuch: event not found`, and measured, `$?` is left where
//     the command before it put it;
//   - a line the expansion *changed* is echoed before it runs, to standard
//     error. That echo is part of the feature rather than noise: the whole
//     bargain of `!!` is that a person sees what they are about to run;
//   - what goes into the history is the **expanded** text. Measured on bash
//     5.3.20 with `echo AAA`, `!!`, `!!`: the list holds `echo AAA`, `echo
//     echo AAA`, `echo echo echo AAA`, so each reference is resolved against
//     what the one before it produced and not against its own text.
//
// A `:p` modifier is the one route that remembers a line and runs nothing,
// which is what makes it the safe way to look at what a reference resolves to.
func (s Shell) expanded(line string, hist []string, remember func(string)) (string, lineOutcome) {
	if s.Runner == nil || !s.Runner.HistoryExpansion() {
		return line, runLine
	}
	if strings.TrimSpace(line) == "" {
		return line, runLine
	}
	// The history is numbered from 1, which is where every shell in the panel
	// starts it and what `!1` means.
	res, err := s.Runner.ExpandHistory(line, hist, 1)
	if err != nil {
		// The complaint is written in both states — measured, so this option
		// does not replace the diagnostic the way `histverify` replaces the
		// echo one branch down.
		s.errf("%s: %s\n", or(s.Name, "sh"), s.Runner.HistoryExpansionRefusal(err))
		if s.Runner.HistoryExpansionReedits() {
			// The line **as typed** goes back on the editing line. There is no
			// partial expansion to hand back: what failed is the reference.
			// See interp.Runner.HistoryExpansionReedits for the rows, and
			// repl.lineStart for where the text lands — the same road
			// verifyLine takes, which is why this is not a fourth outcome.
			return line, verifyLine
		}
		return "", dropLine
	}
	if !res.Changed {
		return line, runLine
	}
	if s.Runner.HistoryExpansionVerifies() && !res.Print {
		// The expansion goes back where it came from instead of being run,
		// and the echo above is what it replaces rather than something it is
		// added to: measured, the person sees the text once, on the line they
		// are about to press Return on. See interp.Runner.
		// HistoryExpansionVerifies for the rows and repl.lineStart for where
		// the text lands.
		return res.Line, verifyLine
	}
	s.errf("%s\n", res.Line)
	if res.Print {
		// `:p` prints the expansion and runs nothing. It is still remembered,
		// which is what makes the next line able to recall it.
		//
		// Unmoved by `histverify`, and measured rather than assumed: with the
		// option on, `!!:p` still prints and still joins the list, so the two
		// routes that run nothing do different things with the same text.
		if remember != nil {
			remember(res.Line)
		}
		return "", dropLine
	}
	return res.Line, runLine
}

// lineOutcome is what the expander leaves the session to do with a line.
//
// A boolean until there were three answers rather than two, and the third is
// not a shade of either: dropLine abandons the construct in hand the way ^C
// abandons it, and verifyLine keeps it — measured, a `!!` verified on the
// second line of a `for` loop is redrawn at the **continuation** prompt and
// the loop goes on being typed.
type lineOutcome int

const (
	// runLine is the ordinary answer: the text is what the parser is given.
	runLine lineOutcome = iota
	// dropLine runs nothing and abandons whatever was half-typed — a
	// reference nothing matched, or a `:p` that asked only to be shown.
	dropLine
	// verifyLine runs nothing and puts the text back on the editing line,
	// with the construct in hand left alone.
	verifyLine
)

// A compile-time reminder that the engine's errors are the only ones this
// renders: a new one added there without a wording here would print its Go
// error text at somebody's prompt.
var _ = []error{
	(*histexpand.NotFound)(nil),
	(*histexpand.SubstFailed)(nil),
	(*histexpand.BadModifier)(nil),
	(*histexpand.BadWordSpecifier)(nil),
	(*histexpand.NoPreviousSubstitution)(nil),
}
