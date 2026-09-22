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
	name := r.sem().IgnoredNamesVariable
	if name == "" {
		return nil
	}
	if !r.ignoredNamesFollowTheParameter() && !r.ignoredNamesLive {
		return nil
	}
	value, ok := r.getVar(name)
	if value == "" || !ok {
		return nil
	}
	if r.ask(r.sem().IgnoredNamesValueIsOnePattern,
		"the ignore parameter's value being one pattern rather than a list") {
		// One pattern, colons and all. Measured on ksh93u+: `FIGNORE='*.txt'`
		// takes both `.txt` files out and `FIGNORE='*.txt:*.log'` takes
		// nothing out at all, because no name holds a colon. The alternation
		// a script wants there is written `@(*.txt|*.log)`, which does work.
		return []string{value}
	}
	if r.unspecified {
		return nil
	}
	var out []string
	for _, p := range splitIgnorePatterns(value, r.readsQuantifiedGroups(false)) {
		// An empty element is not a pattern that matches an empty name: it
		// is nothing, so `:b` and `b:` ignore exactly what `b` ignores.
		// Measured, and it is what keeps a value ending in a colon from
		// deleting every word an expansion produced.
		//
		// **A mutation that deletes this guard survives**, and is recorded
		// so the next reader does not go hunting for the row that would
		// kill it: matchPattern answers false for an empty pattern against
		// every word an expansion can produce, so an empty element kept is
		// an element that never matches. The guard says the intent rather
		// than carrying it, and it is what a matcher answering `""`
		// differently would need. The rows *are* falsifiable against the
		// reading this names — a mutant making an empty element `*` is
		// killed by `an empty element ignores nothing` and by `a leading
		// colon ignores nothing either`, which is the reading a shell gets
		// wrong here and the one they exist for.
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitIgnorePatterns cuts the parameter's value at the colons that separate
// its patterns, which is not every colon in it.
//
// A colon is also an ordinary member of a bracket expression, the delimiter a
// character class is written with, and a character a group may hold — and a
// value whose patterns use any of those is cut to pieces by a plain split. A
// list of three patterns built from a class, a quantified group and a bracket
// holds eight colons of which only two separate, and a plain split makes nine
// pieces of it, none of which ignores anything.
//
// Measured 2026-09-22 on bash 5.3.20, in a directory holding `(a`, `:`, `[a`,
// `a`, `a:b`, `ab`, `b`, `b)` and `b]`, by what `echo *` then leaves:
//
//	GLOBIGNORE=      takes out           so the value is
//	[a:b]            : a b               one pattern, colon and all
//	a:b              a b                 two, as a plain split gives
//	[a:b]:ab         : a b ab            one bracket and one word
//	@(a:b)           a:b                 one group — with `extglob` on
//	@(a:b)           b)                  two, with it off
//	(a:b)            (a b)               two either way: no bare groups
//	@(a:b            nothing             an unclosed group takes the rest
//	[a:b             nothing             and so does an unclosed bracket
//	a\:b             a:b                 an escaped colon does not cut
//
// The `[` scan is the plain one — the next `]`, wherever it stands — rather
// than the matcher's, which reads a `]` written first as a member. `[]:a]`
// takes nothing out, where the matcher's reading would have made it one
// bracket holding `]`, `:` and `a` and taken both `:` and `a`.
func splitIgnorePatterns(value string, quantified bool) []string {
	var out []string
	start := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\':
			i++
		case '[':
			end, ok := ignoreBracketEnd(value, i)
			if !ok {
				i = len(value)
				continue
			}
			i = end
		case '@', '?', '*', '+', '!':
			if !quantified || i+1 >= len(value) || value[i+1] != '(' {
				continue
			}
			end, ok := ignoreGroupEnd(value, i+1)
			if !ok {
				i = len(value)
				continue
			}
			i = end
		case ':':
			out = append(out, value[start:i])
			start = i + 1
		}
	}
	return append(out, value[start:])
}

// ignoreBracketEnd is where the bracket opening at i closes, and false where
// nothing closes it — in which case it reaches the end of the value and no
// colon behind it separates anything.
func ignoreBracketEnd(value string, i int) (int, bool) {
	j := strings.IndexByte(value[i+1:], ']')
	if j < 0 {
		return 0, false
	}
	return i + 1 + j, true
}

// ignoreGroupEnd is where the group opening at the `(` at i closes. A bracket
// inside one is stepped over whole, because a `)` in a bracket is a member;
// an unclosed bracket leaves the group unclosed too, which is the same
// reading closingParen takes.
func ignoreGroupEnd(value string, i int) (int, bool) {
	depth := 0
	for j := i; j < len(value); j++ {
		switch value[j] {
		case '\\':
			j++
		case '[':
			end, ok := ignoreBracketEnd(value, j)
			if !ok {
				return 0, false
			}
			j = end
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return j, true
			}
		}
	}
	return 0, false
}

// ignoredNamesFilterTheListing answers *where* the patterns are applied: to
// the names a directory listing gave, as each component of the walk produces
// them, or to the whole word the expansion produced at the end.
//
// See Semantics.IgnoredNamesMatchTheLastComponent, which holds the panel.
func (r *Runner) ignoredNamesFilterTheListing() bool {
	return r.ask(r.sem().IgnoredNamesMatchTheLastComponent,
		"an ignore pattern being matched against the name in the directory rather than the word")
}

// ignoredListedName reports whether one name a directory listing gave is
// taken back out by one of the patterns.
//
// One component against one component: the subject is the entry, with no
// `./` in front of it and no `/` behind it, so a pattern holding a separator
// matches nothing here — which is the measurement, `FIGNORE='sub/x.txt'`
// taking nothing out of `sub/*` where `FIGNORE='*.txt'` takes `x.txt`.
func (r *Runner) ignoredListedName(name string, patterns []string) bool {
	for _, p := range patterns {
		if r.ignoredNameMatches([]string{name}, p) {
			return true
		}
	}
	return false
}

// ignoredName reports whether one word a pathname expansion produced is taken
// back out by one of the patterns.
//
// This is the whole-word reading, which is bash's: the subject is the word as
// the pattern spelled it, `GLOBIGNORE='./a.txt'` taking `./a.txt` out of `./*`
// where `GLOBIGNORE='a.txt'` does not. The other reading never reaches here —
// it is applied to each listing as the walk produces it, in Runner.matchIn.
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

// ignoredNamesFollowTheParameter is the axis that says the facility is the
// parameter's current state rather than a switch an assignment latched.
//
// Asked only in a dialect that has the parameter at all, which is what keeps
// the four with none of it from being refused for an answer they cannot have.
func (r *Runner) ignoredNamesFollowTheParameter() bool {
	if r.sem().IgnoredNamesVariable == "" {
		return false
	}
	return r.ask(r.sem().IgnoredNamesFollowTheParameter,
		"the ignore facility following the parameter rather than an assignment to it")
}

// ignoredNamesRevealHidden reports whether the ignore facility is what makes
// a `*` see the names beginning with a period, for a dialect that reads the
// parameter's state rather than latching it.
//
// The bash half of this is a hook on the assignment — see
// ignoredNamesAssigned — because there the switch is `dotglob` itself and a
// script may write it back. ksh93 has no such option to write, and answers
// about the parameter as it stands: measured on ksh93u+, `FIGNORE=”` shows
// the hidden names where bash's null value shows none, and an inherited
// value shows them where bash's does nothing at all.
func (r *Runner) ignoredNamesRevealHidden() bool {
	if !r.ignoredNamesFollowTheParameter() || !r.sem().IgnoredNamesRevealHiddenNames {
		return false
	}
	_, ok := r.getVar(r.sem().IgnoredNamesVariable)
	return ok
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
		// measured rather than assumed: on bash 5.3.15, with `nocaseglob`
		// on, an ignore pattern of `*.TXT` takes `a.txt` out, and with it
		// off it does not. Pinned both ways in
		// TestIgnoredNamesFoldCaseWithTheExpansion — one row alone would be
		// answered by a filter that always folded or by one that never did.
		//
		// bash 3.2.57 folds neither way here, so this is 5.x's answer and
		// not bash's. No preset is bash 3.2, so it is recorded rather than
		// given an axis.
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
	if r.ignoredNamesFollowTheParameter() {
		// Nothing to latch: the expansion reads the parameter where it
		// stands, so an assignment is the ordinary one it already was.
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
	if r.ignoredNamesFollowTheParameter() {
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
	if r.ignoredNamesFollowTheParameter() {
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
