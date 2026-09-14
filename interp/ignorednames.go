// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// The parameter whose patterns take names back out of a pathname expansion —
// bash's `GLOBIGNORE`, and ksh93's `FIGNORE` once that dialect has it. Which
// parameter it is belongs to the dialect; what it does is here, because
// pathname expansion is here.
//
// It is a *state* and not simply a lookup, which is the one surprising thing
// about it and is measured: a value inherited from the environment does
// nothing at all until something assigns the parameter. See
// Semantics.IgnoredNamesRevealHiddenNames for the run of measurements, and
// ignoredNamesAssigned for the two hooks that carry the state.

// ignoredNamePatterns is the list a pathname expansion filters its words
// through: the parameter's value split on colons, with the empty elements
// dropped.
//
// Empty where the dialect has no such parameter, where nothing has assigned
// it, and where what was assigned holds no pattern — three different reasons
// for the same answer, and none of them reaches the filter.
func (r *Runner) ignoredNamePatterns() []string {
	if !r.ignoredNamesLive {
		return nil
	}
	name := r.sem().IgnoredNamesVariable
	if name == "" {
		return nil
	}
	value, _ := r.getVar(name)
	if value == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(value, ":") {
		// An empty element is not a pattern that matches an empty name: it
		// is nothing, so `:b` and `b:` ignore exactly what `b` ignores.
		// Measured, and it is what keeps a value ending in a colon from
		// deleting every word an expansion produced.
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ignoredName reports whether one word a pathname expansion produced is taken
// back out by one of the patterns.
//
// The word is matched **component by component**, which is the half that
// separates this from the `~` exclusions one component along in glob.go: a
// `*` here stops at a `/` exactly as one in the expansion's own pattern does,
// so `*x.txt` does not reach `sub/x.txt` where `sub/?.txt` does. There is no
// leading-period rule to go with it — `sub/*` does take `sub/.y` out — so the
// two halves of the walk's own rule are split here, and only one of them
// survives.
//
// Splitting both sides is the whole of it. A pattern with fewer components
// than the word cannot match, and one with more cannot either, so the
// separators line up before any matching happens.
func (r *Runner) ignoredName(word string, patterns []string) bool {
	parts := strings.Split(word, "/")
	for _, p := range patterns {
		if r.ignoredNameMatches(parts, p) {
			return true
		}
	}
	return false
}

func (r *Runner) ignoredNameMatches(parts []string, pattern string) bool {
	pieces := strings.Split(pattern, "/")
	if len(pieces) != len(parts) {
		return false
	}
	fold := r.MatchOption(GlobFoldsCase)
	for i, piece := range pieces {
		o := r.patternOpts(piece, parts[i])
		// The same fold pathname expansion itself is under, which is
		// measured rather than assumed: with `nocaseglob` on, an ignore
		// pattern of `*.TXT` takes `a.txt` out, and with it off it does not.
		o.fold = fold
		o.foldWide = o.fold && r.caseFoldReachesBeyondASCII(piece, parts[i])
		if !matchPattern(piece, parts[i], o) {
			return false
		}
	}
	return true
}

// ignoredNamesAssigned is the hook an assignment to the parameter runs, and
// with it the hidden-name switch the assignment writes.
//
// Two things happen and they are not the same thing. The parameter becomes
// live, which is what an inherited value never does; and a **non-null** value
// turns hidden names on, where a null one leaves that switch exactly as it
// was. Both measured — see Semantics.IgnoredNamesRevealHiddenNames.
func (r *Runner) ignoredNamesAssigned(name, value string) {
	if name == "" || name != r.sem().IgnoredNamesVariable {
		return
	}
	r.ignoredNamesLive = true
	if value != "" && r.sem().IgnoredNamesRevealHiddenNames {
		r.SetMatchOption(PatternsMatchHidden, true)
	}
}

// ignoredNamesUnset is the other hook, and it is not the first one's mirror
// image: the switch goes off whoever turned it on, so `shopt -s dotglob;
// unset GLOBIGNORE` leaves hidden names hidden. Measured.
func (r *Runner) ignoredNamesUnset(name string) {
	if name == "" || name != r.sem().IgnoredNamesVariable {
		return
	}
	r.ignoredNamesLive = false
	if r.sem().IgnoredNamesRevealHiddenNames {
		r.SetMatchOption(PatternsMatchHidden, false)
	}
}

// ignoredNamesRestored is the hook a *binding* change runs, as against an
// assignment: a `local` going out of scope puts the caller's value back, and
// the facility follows the value it finds there.
//
// Measured on bash 5.3.15, 2026-09-13, with `GLOBIGNORE='*.log'` outside a
// function: `f(){ local GLOBIGNORE; echo *; }; f; echo *` sees no hidden name
// inside f and sees them again afterwards, with `*.log` filtered again. So
// the switch is not simply left where the body put it — the restore writes it
// as an assignment of the restored value would.
func (r *Runner) ignoredNamesRestored(name string) {
	if name == "" || name != r.sem().IgnoredNamesVariable {
		return
	}
	value, ok := r.getVar(name)
	if !ok || value == "" {
		// Nothing to go back to. The unset half rather than the null one,
		// because a scope that restores no value restores no parameter, and
		// a null one this shell has never been told about is the state an
		// inherited value is in: not read.
		r.ignoredNamesUnset(name)
		return
	}
	r.ignoredNamesAssigned(name, value)
}
