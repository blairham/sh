// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// patternPolicyRoute switches the route below on. It is a variable so a test
// can run the same script with the setup's scans read from the pattern's
// family and read afresh, and require the same answers.
var patternPolicyRoute = true

// patternTextFacts are the answers of every scan of a pattern's **text** that
// matchPatternChecked's setup asks before it matches, read in one pass and
// kept with the pattern's family.
//
// The setup asks up to nine questions of the text — is there a `[^`, a `\]`,
// a `[:`, a `[:` nothing closes, a backslash inside a bracket, a `[.` or `[=`
// inside one, a bracket nothing closes, a group nothing closes, a bracket a
// sub-expression left open — and each was a walk of the pattern, at every
// match. F-Sy-H's highlighter matches about sixty patterns a call that reach
// the setup, most of them the same few patterns again (#6423), so the walks
// were a large share of what a match cost before it compared a byte.
//
// Every answer here is a function of the text and, for five of them, of one
// setting the scan is handed — which is why each of those takes two bits,
// one per setting, and why the setting is still read from the runner at every
// match. The policies the answers gate are still asked at every match too:
// what is kept is only what the text says, never what the dialect answers.
type patternTextFacts uint32

const (
	// textFactsRead says the facts were read, so that a zero value is "not
	// read" and never "the text holds none of these".
	textFactsRead patternTextFacts = 1 << iota
	textOpensCaret
	textEscapesClose
	textOpensClass
	textLeavesClassOpen
	// The rest come in pairs: the scan with its setting false, then true.
	textEscapesInBracket
	_
	textOpensCollating
	_
	textLeavesBracketOpen
	_
	textLeavesGroupOpen
	_
	textLeftOpenBySub
	_
)

// readPatternTextFacts asks every scan of p, each pair under both settings.
func readPatternTextFacts(p string) patternTextFacts {
	f := textFactsRead
	set := func(bit patternTextFacts, on bool) {
		if on {
			f |= bit
		}
	}
	set(textOpensCaret, strings.Contains(p, "[^"))
	set(textEscapesClose, strings.Contains(p, `\]`))
	set(textOpensClass, strings.Contains(p, "[:"))
	set(textLeavesClassOpen, patternHasAnUnterminatedClass(p))
	for i, setting := range [2]bool{false, true} {
		set(textEscapesInBracket<<i, hasBracketEscape(p, setting))
		set(textOpensCollating<<i, hasCollatingDelimiter(p, setting))
		set(textLeavesBracketOpen<<i, hasUnterminatedBracket(p, setting))
		set(textLeavesGroupOpen<<i, hasUnterminatedPatternGroup(p, setting))
		set(textLeftOpenBySub<<i, bracketLeftOpenBySubExpression(p, setting))
	}
	return f
}

// patternText is a pattern and, where the route read them, its text facts.
//
// Each question below answers from the facts where they were read and from
// the scan the setup has always run where they were not, so the setup is
// written once and the route decides only where its answers come from.
type patternText struct {
	p     string
	facts patternTextFacts
	// fam is the family the facts were read from, so that the setup's
	// other question of the cache — the extended scan's plan — does not
	// look the pattern up again.
	fam *programFamily
}

// scanText is a pattern whose questions are answered by scanning it.
func scanText(p string) patternText { return patternText{p: p} }

// policyText is the pattern matchPatternChecked's setup asks about, with its
// facts read from the pattern's family where the route is on.
func (r *Runner) policyText(p string) patternText {
	if !patternPolicyRoute {
		return scanText(p)
	}
	fam := programs.family(p)
	if fam == nil {
		return scanText(p)
	}
	f := patternTextFacts(fam.text.Load())
	if f == 0 {
		f = readPatternTextFacts(p)
		fam.text.Store(uint32(f))
	}
	r.patternPolicyRouted++
	return patternText{p: p, facts: f, fam: fam}
}

// fact answers one fact from the facts that were read, or from scan where
// none were.
func (t patternText) fact(bit patternTextFacts, scan func(string) bool) bool {
	if t.facts == 0 {
		return scan(t.p)
	}
	return t.facts&bit != 0
}

// pair answers one fact of a pair under setting.
func (t patternText) pair(bit patternTextFacts, setting bool, scan func(string, bool) bool) bool {
	if t.facts == 0 {
		return scan(t.p, setting)
	}
	if setting {
		bit <<= 1
	}
	return t.facts&bit != 0
}

func (t patternText) opensCaret() bool {
	return t.fact(textOpensCaret, func(p string) bool { return strings.Contains(p, "[^") })
}

func (t patternText) escapesClose() bool {
	return t.fact(textEscapesClose, func(p string) bool { return strings.Contains(p, `\]`) })
}

func (t patternText) opensClass() bool {
	return t.fact(textOpensClass, func(p string) bool { return strings.Contains(p, "[:") })
}

func (t patternText) leavesClassOpen() bool {
	return t.fact(textLeavesClassOpen, patternHasAnUnterminatedClass)
}

func (t patternText) escapesInBracket(emptyCompiles bool) bool {
	return t.pair(textEscapesInBracket, emptyCompiles, hasBracketEscape)
}

func (t patternText) opensCollating(emptyCompiles bool) bool {
	return t.pair(textOpensCollating, emptyCompiles, hasCollatingDelimiter)
}

// The three below are the scans scannedOnce already keeps for a long
// pattern, and scanning reaches them through it as before.

func (t patternText) leavesBracketOpen(emptyCompiles bool) bool {
	return t.pair(textLeavesBracketOpen, emptyCompiles, func(p string, s bool) bool {
		return scannedOnce(p, scanOpenBracket, s, hasUnterminatedBracket)
	})
}

func (t patternText) leavesGroupOpen(emptyCompiles bool) bool {
	return t.pair(textLeavesGroupOpen, emptyCompiles, func(p string, s bool) bool {
		return scannedOnce(p, scanOpenGroup, s, hasUnterminatedPatternGroup)
	})
}

func (t patternText) leftOpenBySub(collating bool) bool {
	return t.pair(textLeftOpenBySub, collating, func(p string, s bool) bool {
		return scannedOnce(p, scanLeftOpenBySub, s, bracketLeftOpenBySubExpression)
	})
}
