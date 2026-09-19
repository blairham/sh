// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/internal/histexpand"
)

// SetHistoryExpansion moves the state `set -H`, `set -o histexpand` and zsh's
// `setopt banghist` all name, and remembers that something moved it.
//
// The remembering is the whole reason this is a method rather than a field
// assignment: the front end applies the dialect's default for an interactive
// session, and it has to apply it *before* the startup files and then leave it
// alone. An rc file whose first line is `set +H` — which is how a person who
// does not want the feature turns it off — would otherwise be undone by a
// default applied after the file ran.
func (r *Runner) SetHistoryExpansion(on bool) {
	r.histExpand = on
	r.histExpandMoved = true
}

// StartInteractiveHistory applies the dialect's defaults for a session that
// has a prompt: recording on, and the expander on where this shell's is.
//
// Read rather than `ask`ed, as DefaultOptionLetters is: a session starting up
// has nothing to refuse to and nobody to refuse to yet, and a shell that could
// not decide whether `!!` expands would have to refuse every line a person
// typed rather than one construct.
//
// Called before the startup files and never after them, which is the whole
// reason SetHistoryExpansion remembers that it was called: `set +H` in an rc
// file is how a person who does not want the feature turns it off, and a
// default applied afterwards would put back the thing the file removed.
func (r *Runner) StartInteractiveHistory() {
	r.histRecord = true
	if r.histExpandMoved {
		return
	}
	s := r.sem()
	r.histExpand = s.HistoryExpansion == Yes && s.HistoryExpansionAtAPrompt == Yes
}

// HistoryExpansion reports whether `!!` and its family are rewritten before a
// line is parsed.
func (r *Runner) HistoryExpansion() bool { return r.histExpand }

// HistoryExpansionVerifies reports whether an expansion that changed the line
// is handed back to the person instead of being run.
//
// The state bash spells `shopt histverify` and zsh spells `HIST_VERIFY`, off
// in both. It does nothing to the expansion — the same call, the same
// arguments, the same answer — and everything to what is done with the
// result, which is why it is a front end's question rather than this
// package's: the result goes on the editing line, and only a front end with
// an editor has one.
//
// Measured 2026-09-19 through a pseudo-terminal against bash 5.3.20, and the
// same rows against zsh 5.9.2 under `setopt HIST_VERIFY`, with `echo AAA` in
// the list:
//
//	!!                    a fresh prompt reading `echo AAA`, cursor at its end
//	then ` BBB` and Return  runs `echo AAA BBB`
//	the list afterwards   `echo AAA BBB` — one entry, and the accepted text
//	!! then ^C            nothing runs and the list gains nothing at all
//	a line nothing changed  runs, with no echo and no second look
//	a reference nothing matched  the same complaint as with it off
//
// The fourth row is the one worth naming, because the issue asking for this
// said the expansion still joins the list the way `:p` puts it there. It does
// not: an abandoned verification leaves the list exactly as it was, and an
// accepted one is recorded by the ordinary accept as whatever was accepted.
// So this option records nothing of its own.
func (r *Runner) HistoryExpansionVerifies() bool { return r.histVerify }

// SetHistoryExpansionVerifies moves it.
func (r *Runner) SetHistoryExpansionVerifies(on bool) { r.histVerify = on }

// HistoryExpansionInAScript reports whether this dialect reads a script one
// physical line at a time, keeping the list and expanding against it. See
// Semantics.HistoryExpansionInAScript, which is bash's row alone.
//
// Read through the runner rather than off the vector, because a nil vector
// means bash's and a front end testing the field directly would give every
// embedder without one bash's answer.
func (r *Runner) HistoryExpansionInAScript() bool {
	return r.sem().HistoryExpansionInAScript == Yes
}

// SetHistoryRecording and HistoryRecording are the same pair for bash's
// `set -o history`: whether accepted lines join the list at all.
func (r *Runner) SetHistoryRecording(on bool) { r.histRecord = on }

// HistoryRecording reports it.
func (r *Runner) HistoryRecording() bool { return r.histRecord }

// HistoryChars is the three characters history expansion is spelled with,
// read out of `histchars` where the dialect has the parameter.
//
// The parameter's own rules are the dialect's — see docs/spec/history.md,
// where `histchars` was measured being one parameter under two names and
// truncated to three characters — and this only reads what is there. An empty
// value turns the expander off, which is measured in both shells that have it.
func (r *Runner) HistoryChars() histexpand.Chars {
	c := histexpand.Default
	c.DoubleQuotesProtect = r.posixMode && r.sem().HistoryExpansionSparesDoubleQuotesInPosixMode == Yes
	c.QuoteInPlace = r.sem().HistoryQuoteModifierInPlace == Yes
	c.CommentStops = r.sem().HistoryCommentStopsExpansion == Yes
	c.QuoteIsText = r.sem().HistoryQuoteEndsAnEventReference == Yes
	c.EventCharClosesAnEventName = r.sem().HistoryEventCharClosesAnEventName == Yes
	c.BracedEvent = r.sem().HistoryBracedEventReference == Yes
	c.LastWordEndsTheDesignator = r.sem().HistoryLastWordEndsTheDesignator == Yes
	c.FirstWordEndsARange = r.sem().HistoryFirstWordEndsARange == Yes
	c.WordwiseSubstitution = r.sem().HistoryWordwiseSubstitutionModifier == Yes
	switch r.sem().HistoryWords {
	case HistoryWordsShell:
		c.Words = histexpand.WordsShell
	case HistoryWordsShellBraces:
		c.Words = histexpand.WordsShellBraces
	default:
		c.Words = histexpand.WordsQuotes
	}
	v, ok := r.GetVar(historyCharsParameter)
	if !ok {
		return c
	}
	chars := []rune(v)
	// A short value leaves the rest at their defaults and an empty one turns
	// the feature off, which is the reading both shells with the parameter
	// give it.
	if len(chars) == 0 {
		return histexpand.Chars{}
	}
	c.Event = chars[0]
	if len(chars) > 1 {
		c.Quick = chars[1]
	}
	if len(chars) > 2 {
		c.Comment = chars[2]
	}
	return c
}

// historyCharsParameter is the name both shells that have it use. bash reads
// `histchars` and zsh reads the same name under a second spelling; the second
// spelling is the zsh dialect's business and this reads the one they share.
const historyCharsParameter = "histchars"

// ExpandHistory rewrites one line against the history list, and is what a
// front end holding that list calls.
//
// The list is the caller's because this package has none: a Runner embedded in
// another program has no prompt, no editor and nothing a person typed
// yesterday. Oldest entry first, and first is the history number of the first
// entry — see histexpand.List, where the numbering is explained.
//
// A shell with the state off hands the line straight back, so the caller can
// call unconditionally and the one place that decides is here.
func (r *Runner) ExpandHistory(line string, lines []string, first int) (histexpand.Result, error) {
	return r.ExpandHistoryIn(line, histexpand.Unquoted, lines, first)
}

// ExpandHistoryAlways expands whether or not `set -H` is on, which is what
// `history -p` wants: the letter decides whether the shell expands the lines
// it *reads*, and an operand handed to that builtin was handed to it on
// purpose. Measured — `set -o history; echo one two three; history -p "!!"`
// writes `echo one two three` with the letter never written.
func (r *Runner) ExpandHistoryAlways(line string, lines []string, first int) (histexpand.Result, error) {
	return histexpand.Expand(line, histexpand.List{Lines: lines, First: first, Memory: r.historyMemory()}, r.HistoryChars())
}

// ExpandHistoryIn is the same for a front end handing over one **physical**
// line of a script, which may begin inside a quote an earlier line opened.
//
// See histexpand.Quote: the state seeds the scanner, and the scanner still
// closes the quote where the line closes it.
func (r *Runner) ExpandHistoryIn(line string, in histexpand.Quote, lines []string, first int) (histexpand.Result, error) {
	if !r.histExpand {
		return histexpand.Result{Line: line}, nil
	}
	return histexpand.ExpandIn(line, in, histexpand.List{Lines: lines, First: first, Memory: r.historyMemory()}, r.HistoryChars())
}

// historyMemory is the shell's histexpand.Memory, made the first time a line
// needs one.
func (r *Runner) historyMemory() *histexpand.Memory {
	if r.histMemory == nil {
		r.histMemory = &histexpand.Memory{}
	}
	return r.histMemory
}

// SetHistoryStore hands the Runner the list `history` keeps, so that the
// front end reading a script can add to it and the expander can index it.
//
// A pair of functions rather than a slice on the Runner, and they take a
// *Runner rather than closing over one, because the list is a **dialect's**
// and lives where that dialect keeps it — bash's is a shell array under a
// name no script can reach, which is what gives `(history -s x)` a copy of
// its parent's list rather than a handle on it. Closures capturing the runner
// they were registered on would have written a subshell's entry into the
// parent's array, which is the one thing that storage shape exists to
// prevent.
//
// A dialect with no list leaves these nil, and a front end then records
// nothing and expands against an empty list, which is what a shell without
// the feature does anyway.
func (r *Runner) SetHistoryStore(entries func(*Runner) []string, add func(*Runner, string)) {
	r.histEntries, r.histAdd = entries, add
}

// SetHistoryListFilledByTheReader records that the front end reading this
// program is putting its commands into the list, which is the state bash's
// `remember_on_history` names.
//
// Two builtins turn on it. `history -s` is documented to remove the last
// entry before adding its own — the last entry being the `history -s` line
// itself — and measured, `history -p` drops its own line too. Both are
// conditional on the line being there in the first place: measured, `history
// -s a` followed by `history -s b` in a shell with no list leaves *both*,
// because neither line was ever recorded.
//
// Asked of the front end rather than derived from HistoryRecording, because
// the two are not the same thing here: an interactive session records into
// the editor's own history and not into this list, so a prompt would drop a
// planted entry that nothing had pushed the line in front of. Joining those
// two lists is a real question and not this one — see dialect/bash's
// history.go, which has said so since the builtin landed.
func (r *Runner) SetHistoryListFilledByTheReader(on bool) { r.histFromReader = on }

// HistoryListFilledByTheReader reports it.
func (r *Runner) HistoryListFilledByTheReader() bool { return r.histFromReader }

// HistoryEntries is the list, oldest first.
func (r *Runner) HistoryEntries() []string {
	if r.histEntries == nil {
		return nil
	}
	return r.histEntries(r)
}

// SetHistoryNumbering hands the Runner the history number of the list's
// oldest entry, for a dialect whose list drops entries off the front and
// keeps numbering from where it was. Nil numbers from one.
func (r *Runner) SetHistoryNumbering(first func(*Runner) int) { r.histFirst = first }

// HistoryFirst is the history number of the oldest entry HistoryEntries holds.
func (r *Runner) HistoryFirst() int {
	if r.histFirst == nil {
		return 1
	}
	return r.histFirst(r)
}

// RecordHistoryEntry appends one command to the list.
//
// The **expanded** text, which is what every reference after it resolves
// against: measured on bash 5.3.20 with `echo one two three`, `!!`, `!!`, the
// list holds `echo one two three`, `echo echo one two three`, `echo echo echo
// one two three`.
//
// Empty text is refused here and nowhere else, which is the one guard for the
// one rule: a blank line is not a command and does not join the list. A line
// of *blanks* is — measured, a script whose second line is three spaces has
// those three spaces as its first entry — so the test is emptiness and not
// blankness, and a caller trimming before it called would lose that.
func (r *Runner) RecordHistoryEntry(line string) {
	if r.histAdd == nil || line == "" {
		return
	}
	r.histAdd(r, line)
}

// HistoryExpansionRefusal renders what this dialect says about a reference the
// list does not hold, ready to write.
//
// The panel words it three ways and two of them are not even the same shape,
// measured 2026-09-15 through a pty: bash says `bash: !nosuch: event not
// found`, ksh93 says `ksh: !nosuch: event not found`, and zsh says `zsh: event
// not found: nosuch` — the reference last, and without the character that
// introduced it. So the wording takes both spellings and each dialect uses the
// one it says.
func (r *Runner) HistoryExpansionRefusal(err error) string {
	d := r.diag()
	switch e := err.(type) {
	case *histexpand.NotFound:
		bare := strings.TrimLeft(e.Ref, string(r.HistoryChars().Event))
		return Wording(d.HistoryEventNotFound, "%[1]s: event not found", e.Ref, bare)
	case *histexpand.SubstFailed:
		return Wording(d.HistorySubstitutionFailed, "%[1]s: substitution failed", e.Ref, e.Bare)
	case *histexpand.NoPreviousSubstitution:
		return Wording(d.HistoryNoPreviousSubstitution, "%[1]s: no previous substitution", e.Ref)
	case *histexpand.BadWordSpecifier:
		return Wording(d.HistoryBadWordSpecifier, "%[1]s: bad word specifier", e.Ref)
	case *histexpand.BadModifier:
		return Wording(d.HistoryBadModifier, "%[1]s: unrecognized history modifier", e.Mod, e.Mod)
	}
	// Nothing else reaches here — the engine raises those three and no
	// others, and the compile-time reminder in repl/historyexpand.go says so
	// — but a fourth added later must not print its Go error text at
	// somebody's prompt without at least being legible.
	return err.Error()
}

// SetHistoryFile hands the Runner the two moments a **script's** history
// list meets a file: start runs the first time the list is turned on, and
// finish runs as the shell ends with the list still on.
//
// Measured 2026-09-16 on bash 5.3.20, from a script file with no terminal
// anywhere: `HISTFILE=f; set -o history; history` lists f's lines, and the
// same script leaves f with its own two lines appended. Neither happens in a
// subshell's ending, a shell killed by a signal, a shell `exec` replaced, or
// a shell that turned the list off again before it ended — each measured.
//
// The file is the dialect's business, as the list is, and a front end with an
// interactive session keeps its own: repl reads and writes the file around a
// prompt, and a Runner that is Interactive never runs either of these.
func (r *Runner) SetHistoryFile(start, finish func(*Runner)) {
	r.histStart, r.histFinish = start, finish
}

// setHistoryRecording is `set -o history` and `set +o history`.
//
// The start runs **once**, at the first time the state goes from off to on:
// measured, `set -o history` a second time, or after a `set +o history`,
// neither reads the file again nor puts back a HISTSIZE the script unset. A
// first `set -o history` with no HISTFILE to read still counts as the first,
// so naming the file afterwards and toggling reads nothing.
func (r *Runner) setHistoryRecording(on bool) {
	first := on && !r.histRecord && !r.histStarted
	r.histRecord = on
	if !first || r.Interactive || r.histStart == nil {
		return
	}
	r.histStarted = true
	r.histStart(r)
}

// finishHistoryFile runs the dialect's finish, where the shell that is ending
// is one the file is written for.
func (r *Runner) finishHistoryFile() {
	if r.histFinish == nil || !r.histStarted || !r.histRecord || r.inSubshell || r.Interactive || r.killedBy != "" {
		return
	}
	r.histFinish(r)
}
