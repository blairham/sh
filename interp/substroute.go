// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"

	"github.com/blairham/sh/syntax"
)

// What a refused substitution body's location is called, between the shell's
// name and the line.
//
// One dialect names the **route** the failing text arrived by, and it names it
// for a parse failure alone: measured 2026-09-17 on bash 5.3.20, `env -i
// PATH=/usr/bin:/bin LC_ALL=C` over a script file with stdin from /dev/null,
// each row holding `v=$(echo hi; for)` somewhere,
//
//	written                                       location
//	on the script's own line                      s.sh: line 2:
//	inside `eval '…'`                             s.sh: eval: line 2:
//	inside a trap body, by condition              s.sh: exit trap: line 1:
//	                                              s.sh: error trap: line 2:
//	                                              s.sh: debug trap: line 2:
//	                                              s.sh: return trap: line 2:
//	                                              s.sh: interrupt trap: line 1:
//	                                              s.sh: trap: line 1:   (every other signal)
//	under `-c`                                    bash: -c: line 2:
//	as `$( … )` inside a backquoted body          s.sh: command substitution: line 1:
//	in a here-document body                       s.sh: command substitution: line 2:
//	inside a file `.` read                        ./inner.sh: line 2:
//	as `$( … )` inside `$( … )`                   s.sh: line 1:
//
// The last two are the controls and are why this is a question about the
// route rather than about depth: a sourced file replaces the shell's name and
// names no route beside it, and a substitution inside a substitution names
// none either, because that shell reads both bodies while it is reading the
// script's line. The two that *are* named `command substitution` are the two
// it reads at expansion time instead — the older spelling's body, and a
// here-document body.
//
// A run-time diagnostic from the same places is located plainly in all of
// them: `eval 'nosuchcmd'` is `s.sh: line 1: nosuchcmd: command not found`
// there, with no `eval:`. So this is a property of the refusal and not of the
// frame, which is why it is a value handed to one message rather than a field
// the location reads for every message inside the route.

// substFailureRoute is that name, and the empty string for every dialect that
// writes none and every route that has none.
//
// The order is innermost first, because that is what the panel writes: a
// substitution in a here-document body inside an `eval` is the here-document's
// `command substitution`, the nearest thing the text was read out of.
func (r *Runner) substFailureRoute(span syntax.Span) string {
	d := r.diag()
	if d.SubstitutionParseFailureNamesTheConstruct &&
		(span.Backquoted || r.inBodyReadAtExpansion) {
		// The two bodies that dialect reads at expansion time rather than
		// with the script's line. The older spelling is the span's own and
		// was all this answered before; the here-document body is the text
		// *around* the substitution, so it is a fact about where the shell
		// is rather than about the word.
		return "command substitution"
	}
	if !d.NamesTheInputInLocation {
		// The same flag a front end's parse failure reads for `-c`, and the
		// same dialect: this is that sentence's location, reached from a
		// body the front end never saw.
		return ""
	}
	if r.inTrapBody {
		return trapLocationName(r.trapBodyCond)
	}
	if b, ok := r.borrowedAtLocation(); ok && b.eval {
		// `eval` names itself and a sourced file does not, which is the
		// same split borrowedName draws: the file has already replaced the
		// shell's name by the time this is asked, and text that came from
		// no file has only the builtin to be called after.
		return b.label
	}
	if r.Route == RouteCommandString {
		return r.InputName
	}
	return ""
}

// substBodyExpecting is the clause one dialect appends to a token refused
// inside a `$( … )` body, naming the parenthesis it was still looking for.
//
// See Diagnostics.SubstitutionBodyExpecting for the six measured rows and for
// the two shapes that get nothing: the closer itself, which is what the shell
// was looking for, and the older spelling, whose body is read as text.
func (r *Runner) substBodyExpecting(span syntax.Span, err error) string {
	d := r.diag()
	if d.SubstitutionBodyExpecting == "" || span.Backquoted {
		return ""
	}
	// The construct's own closer, which is the `}` of the current-shell
	// spelling and the `)` of the parenthesised one. Measured 2026-09-17 on
	// bash 5.3.20: `v=${ echo hi; ;}` is `` `;' while looking for matching
	// `}' ``.
	closer := ")"
	if span.CurrentShell {
		closer = "}"
	}
	var se *syntax.Error
	if !errors.As(err, &se) || se.Kind != syntax.ErrUnexpected || se.Token == closer {
		return ""
	}
	return Wording(d.SubstitutionBodyExpecting, "", closer)
}
