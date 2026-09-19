// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A successful **pattern** match writing the record a `=~` writes.
//
// Two dialects name that record — see Runner.SetRegexMatch — and only one of
// them fills it from anything but a regular expression. The record was
// `=~`-only here, so every glob comparison and every pattern operator left it
// holding whatever the last `=~` put there (#2916).
//
// Measured 2026-09-19 on ksh93u+ 2012-08-01 (`/bin/ksh`), a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin on `/dev/null`, each row
// preceded by a write that seeds the record with something else so that
// "unchanged" is legible:
//
//	written                     	`${#.sh.match[@]}` and `${.sh.match[*]}`
//	`[[ abcd == a*d ]]`         	1  `abcd`
//	`[[ abcd == a?(b)cd ]]`     	2  `abcd b`
//	`[[ abcd == a?(b)?(c)d ]]`  	3  `abcd b c`
//	`[[ abcd == a?(z)bcd ]]`    	2  `abcd ` — a group that matched nothing
//	`[[ abcd != a*d ]]`         	1  `abcd` — the operator's sense is nothing to it
//	`v=hello; ${v/@(l)(l)/X}`   	3  `ll l l`
//	`v=hello; ${v//[lo]/X}`     	1  `l` — the **first** match and not the last
//	`v=hello; ${v#he}`          	1  `he` — a literal pattern, and it writes
//	`v=hello; ${v%%[lo]*}`      	1  `llo`
//	`[[ abc == abc ]]`          	unchanged — a literal comparison writes nothing
//	`[[ 'a*c' == a\*c ]]`       	unchanged — nor does an escaped one that matches
//	`[[ $z == "a*" ]]`          	unchanged
//	`case abc in a*) ;; esac`   	unchanged
//	`y=(a*)`, `printf '%s\n' a*`	unchanged — pathname expansion writes nothing
//	`[[ -n $u ]]`, `[[ $u < b ]]`	unchanged — only the pattern operators
//	`${v:1:2}`                  	unchanged
//	`w=abc; ${w/zz/Y}`          	unchanged — a failed match leaves it
//	`[[ abc == a* && def == d* ]]`	1  `def` — each comparison writes as it runs
//
// So the set is: **every `[[ ]]` pattern comparison whose right operand is a
// pattern, and every pattern operator of parameter expansion.** A `case` arm
// is not in it, pathname expansion is not in it, and neither is a comparison
// whose right operand holds no unescaped operator — which is the one place the
// two halves differ, since a *literal* `${v#he}` writes and a literal
// `[[ abc == abc ]]` does not.
//
// Three things this deliberately does not become:
//
//   - It is not Runner.publishMatch. That writes the parameters a **reporting
//     pattern** asked for — `$match`, `$MATCH` and the positions — and its
//     silence where nothing asked is measured and load-bearing in the shell
//     that has those. This record is written with nothing having asked, so the
//     two are separate publishers over one report.
//   - It does not make a replacement report. Whether a replacement is expanded
//     again per match is decided by reportsAMatch, and that must stay a
//     question about the pattern's *flags*: measured on the same binary,
//     `x=aaa; i=0; ${x//a/$((++i))}` is `111` and leaves `i` at 1 there, so the
//     record is filled and the replacement is still read once. capturePlan.reports
//     answers no for a recording plan for that reason.
//   - It does not reach pathname expansion, which is the hot path. The plan is
//     built only where a surface asked to record, so a glob walk carries the
//     empty plan it has always carried.

// recordsAPatternMatch reports whether a successful pattern match writes the
// record, having asked the dialect.
//
// Asked only where a dialect named the record — Runner.SetRegexMatch — so the
// shells that keep their captures under another shape, or keep none, are never
// questioned. That is the same guard Runner.recordRegexMatch uses, and it is
// what keeps this axis off every surface of a dialect with no such parameter.
func (r *Runner) recordsAPatternMatch() bool {
	if r.regexMatchName == "" {
		return false
	}
	return r.ask(r.sem().PatternMatchWritesTheMatchRecord,
		"a successful pattern match writing the record a `=~` writes")
}

// recordingPatternOpts turns on the record for one surface's pattern, and
// builds the plan that numbers its groups.
//
// The plan is what a `(#b)` flag builds for the dialect that has one, seeded
// as though the flag stood in front of the pattern: the groups are numbered
// and the whole match is asked for, with nothing in the pattern having asked.
// An extended-pattern dialect has already built a plan of its own by the time
// this is reached and keeps it, which costs nothing to say and is never
// exercised — the axis is one dialect's and that dialect has no `(#…)`.
func (r *Runner) recordingPatternOpts(o patternOpts, pattern string) patternOpts {
	if o.where != nil || !r.recordsAPatternMatch() {
		return o
	}
	o.record = true
	o.where = &matchWhere{plan: planRecordedCaptures(pattern, o)}
	return o
}

// planRecordedCaptures numbers a pattern's groups for a surface that records
// what it matched.
//
// `capturing` and `whole` are seeded true, which is the whole difference from
// planCaptures: there they start false and a `(#b)` or `(#m)` turns them on.
// Here the record wants every group and the whole match with nothing having
// asked, and the group numbering is the same walk — which is what makes an
// extended-pattern operator a capture group, measured above: `a?(b)?(c)d`
// reports the match and then `b` and `c`.
func planRecordedCaptures(pattern string, o patternOpts) capturePlan {
	pl := capturePlan{index: map[int]int{}, recording: true}
	pl.whole = planWalk(pattern, 0, true, true, o, &pl)
	return pl
}

// recordPatternMatch writes a successful pattern match into the record.
//
// Element 0 is the whole match and the rest are the groups, which is the shape
// `=~` already fills — so the two axes that decide what the record keeps are
// asked once, in Runner.recordRegexMatch, and a pattern's record cannot drift
// from a regular expression's. A group that matched the empty string took
// part and is kept as an empty element; one the match never reached did not,
// which is the distinction the texts cannot carry.
func (r *Runner) recordPatternMatch(m matchReport) {
	if !m.recording {
		return
	}
	texts := make([]string, 0, len(m.groups)+1)
	took := make([]bool, 0, len(m.groups)+1)
	texts = append(texts, m.text(m.whole))
	took = append(took, true)
	for _, sp := range m.groups {
		texts = append(texts, m.text(sp))
		// Every group of a pattern is in the record, and one that contributed
		// nothing is an empty element rather than a gap — which is the
		// opposite of what the same shell does with a regular expression's
		// groups. Measured 2026-09-19: `[[ abcd == a?(z)bcd ]]` answers 2 and
		// `abcd ` with a trailing empty, where `[[ abcd =~ b(z)?c ]]` answers
		// `bc` alone. So this hands recordRegexMatch a took of all-true and
		// lets RegexMatchOmitsGroupsThatDidNotMatch keep its regular
		// expression reading intact.
		took = append(took, true)
	}
	r.recordRegexMatch(texts, took)
}

// patternHasAnOperator reports whether a pattern holds anything a matcher has
// to do rather than a string to compare.
//
// It is the one condition the `[[ ]]` half of the record carries and the
// parameter operators do not: measured 2026-09-19, `[[ abc == abc ]]`,
// `[[ 'a*c' == a\*c ]]` and `[[ $z == "a*" ]]` all leave the record alone
// where `v=hello; ${v#he}` — a literal pattern in a parameter operator —
// writes `he`. So the test is about the *characters*, and an escaped or quoted
// operator is not one: the escape is what a quoted right operand arrives as.
//
// The operators are the three POSIX ones plus whatever this dialect's own
// grammar adds in front of a parenthesis, so a dialect without extended
// patterns never counts an `@(` as anything.
func patternHasAnOperator(pattern string, o patternOpts) bool {
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case c == '\\' && i+1 < len(pattern) && patternEscapeReaches(o, pattern[i+1]):
			// The escape takes the next byte with it, which is exactly how a
			// quoted operand reaches a matcher — and only where this dialect's
			// escape reaches that character, which is what
			// Semantics.PatternEscapeReaches says.
			i++
		case c == '*' || c == '?' || c == '[':
			return true
		case c == '(' && o.group:
			return true
		case o.quantified && (c == '@' || c == '!' || c == '+') &&
			i+1 < len(pattern) && pattern[i+1] == '(':
			return true
		}
	}
	return false
}

// patternEscapeReaches reports whether a backslash escapes this character in
// this dialect's patterns.
//
// The empty set means **every** character, which is what
// Semantics.PatternEscapeReaches says and what the dialect this record belongs
// to holds — reading the field as a plain membership test made an escaped `*`
// count as an operator, and `[[ 'a*c' == a\*c ]]` wrote a record the shell
// leaves alone.
func patternEscapeReaches(o patternOpts, c byte) bool {
	return o.escapes == "" || strings.IndexByte(o.escapes, c) >= 0
}
