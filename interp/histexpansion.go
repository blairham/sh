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
	return histexpand.Expand(line, histexpand.List{Lines: lines, First: first}, r.HistoryChars())
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
	return histexpand.ExpandIn(line, in, histexpand.List{Lines: lines, First: first}, r.HistoryChars())
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

// HistoryEntries is the list, oldest first.
func (r *Runner) HistoryEntries() []string {
	if r.histEntries == nil {
		return nil
	}
	return r.histEntries(r)
}

// RecordHistoryEntry appends one command to the list.
//
// The **expanded** text, which is what every reference after it resolves
// against: measured on bash 5.3.20 with `echo one two three`, `!!`, `!!`, the
// list holds `echo one two three`, `echo echo one two three`, `echo echo echo
// one two three`.
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
	case *histexpand.BadModifier:
		return Wording(d.HistoryBadModifier, "%[1]s: unrecognized history modifier", e.Mod, e.Mod)
	}
	// Nothing else reaches here — the engine raises those three and no
	// others, and the compile-time reminder in repl/historyexpand.go says so
	// — but a fourth added later must not print its Go error text at
	// somebody's prompt without at least being legible.
	return err.Error()
}
