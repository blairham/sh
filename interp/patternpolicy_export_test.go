// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "fmt"

// SetPatternPolicyRouteForTest switches the route that reads a pattern's text
// facts from its family on or off and returns what it was, so a test can run
// the same script both ways.
func SetPatternPolicyRouteForTest(on bool) (was bool) {
	was, patternPolicyRoute = patternPolicyRoute, on
	return was
}

// PatternPolicyRoutedForTest is how many matches the route set up.
func PatternPolicyRoutedForTest(r *Runner) int { return r.patternPolicyRouted }

// PatternTextFactsDisagreementForTest is every question patternText answers
// of p, asked once of facts read in one pass and once of the scan the setup
// ran before the route, with each one-setting question under both settings.
// It describes the questions whose answers differ, and is empty where none do.
func PatternTextFactsDisagreementForTest(p string) string {
	read := patternText{p: p, facts: readPatternTextFacts(p)}
	scanned := scanText(p)
	var out string
	note := func(name string, a, b bool) {
		if a != b {
			out += fmt.Sprintf("%s: facts %v, scan %v; ", name, a, b)
		}
	}
	note("opensCaret", read.opensCaret(), scanned.opensCaret())
	note("escapesClose", read.escapesClose(), scanned.escapesClose())
	note("opensClass", read.opensClass(), scanned.opensClass())
	note("leavesClassOpen", read.leavesClassOpen(), scanned.leavesClassOpen())
	for _, s := range []bool{false, true} {
		suffix := fmt.Sprintf("(%v)", s)
		note("escapesInBracket"+suffix, read.escapesInBracket(s), scanned.escapesInBracket(s))
		note("opensCollating"+suffix, read.opensCollating(s), scanned.opensCollating(s))
		note("leavesBracketOpen"+suffix, read.leavesBracketOpen(s), scanned.leavesBracketOpen(s))
		note("leavesGroupOpen"+suffix, read.leavesGroupOpen(s), scanned.leavesGroupOpen(s))
		note("leftOpenBySub"+suffix, read.leftOpenBySub(s), scanned.leftOpenBySub(s))
	}
	return out
}

// PatternTextFactsTrueForTest counts the questions the facts answer yes to,
// so a test can see that its patterns put each question at all.
func PatternTextFactsTrueForTest(p string) map[string]bool {
	t := patternText{p: p, facts: readPatternTextFacts(p)}
	yes := map[string]bool{
		"opensCaret":      t.opensCaret(),
		"escapesClose":    t.escapesClose(),
		"opensClass":      t.opensClass(),
		"leavesClassOpen": t.leavesClassOpen(),
	}
	for _, s := range []bool{false, true} {
		suffix := fmt.Sprintf("(%v)", s)
		yes["escapesInBracket"+suffix] = t.escapesInBracket(s)
		yes["opensCollating"+suffix] = t.opensCollating(s)
		yes["leavesBracketOpen"+suffix] = t.leavesBracketOpen(s)
		yes["leavesGroupOpen"+suffix] = t.leavesGroupOpen(s)
		yes["leftOpenBySub"+suffix] = t.leftOpenBySub(s)
	}
	return yes
}
