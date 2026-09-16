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
func (s Shell) expanded(line string, hist []string, remember func(string)) (string, bool) {
	if s.Runner == nil || !s.Runner.HistoryExpansion() {
		return line, true
	}
	if strings.TrimSpace(line) == "" {
		return line, true
	}
	// The history is numbered from 1, which is where every shell in the panel
	// starts it and what `!1` means.
	res, err := s.Runner.ExpandHistory(line, hist, 1)
	if err != nil {
		s.errf("%s: %s\n", or(s.Name, "sh"), s.Runner.HistoryExpansionRefusal(err))
		return "", false
	}
	if !res.Changed {
		return line, true
	}
	s.errf("%s\n", res.Line)
	if res.Print {
		// `:p` prints the expansion and runs nothing. It is still remembered,
		// which is what makes the next line able to recall it.
		if remember != nil {
			remember(res.Line)
		}
		return "", false
	}
	return res.Line, true
}

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
