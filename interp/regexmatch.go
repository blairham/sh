// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// What the last `=~` captured.
//
// A successful match is worth more than its status: the whole match and every
// parenthesized group are what a script matched *for*, and reading them back
// is the idiom the operator exists to serve. Every shell that keeps them keeps
// the same values; what differs is the *name* — and the shape — so the core
// keeps the record and a dialect names it, the same seam the pipeline-status
// record uses. One shell calls this array BASH_REMATCH; the others that have
// `=~` keep their captures under names and shapes of their own and never
// touch this one, so with no name from the dialect nothing is recorded.

// SetRegexMatch exposes what `=~` captures under a name, as an ordinary
// stored array: element 0 is the whole match and the rest are the groups, in
// order. A dialect calls it from Apply, the same seam that names the
// pipeline-status record.
//
// Ordinary rather than produced, which is measured: a script can assign to
// the name and read its own value back, and `unset` removes it until the next
// `=~` fills it again — there is no producer to outlive the unset.
func (r *Runner) SetRegexMatch(name string) { r.regexMatchName = name }

// regexFold is the prefix that turns a `=~` expression case-insensitive, and
// the empty string where the option asking for that is off.
//
// A **prefix on the expression** rather than a fold applied at comparison
// time, because a regular expression is not a glob: what has to fold is the
// whole compiled thing — literals, bracket expressions, character classes,
// and the complement of a negated class — and only the engine knows which
// bytes of the text are which. Folding the subject and the pattern before
// compiling would fold `[^a]` the wrong way round and would turn `\.` into a
// pattern about the letter it is not.
//
// The prefix is exactly what the option means and no more: `(?i)` sets the
// engine's initial case flag, so an expression that turns it off again for
// part of itself still does. Measured to agree with bash 5.3.15 on every
// cell that distinguishes the readings, with `shopt -s nocasematch` on:
//
//	[[ ABC =~ ^abc$ ]]              yes — a literal folds
//	[[ abc =~ ^ABC$ ]]              yes — and in the other direction
//	[[ ABC =~ ^[a-c]+$ ]]           yes — a range folds
//	[[ ABC =~ ^[[:lower:]]+$ ]]     yes — so does a character class
//	[[ abc =~ ^[[:upper:]]+$ ]]     yes — and that one too
//	[[ A   =~ ^[^a]$ ]]             no  — the fold precedes the negation
//	[[ ABC =~ ^(a)(B)c$ ]]          yes, and captures ABC, A, B
//
// The last is what says the fold must not touch the captures: they are spans
// of the **subject** as the script wrote it, so `BASH_REMATCH` holds `A` and
// not `a`.
//
// Locale is the one cell this does not yet answer. An explicit C or POSIX
// locale narrows the fold to ASCII in the panel — measured, `LC_ALL=C` makes
// `[[ ÉTÉ =~ ^été$ ]]` fail in bash 5.3.15 and in zsh 5.9.2 where the same
// line under a UTF-8 locale matches — and `(?i)` has no locale to be told
// about, so ours folds it either way. The narrowing is the policy
// Runner.caseMapper holds for the sites that convert a whole value; this one
// hands the question to an engine that cannot take it. #2622 records the
// measurement rather than leaving it unwritten, and the glob side of `[[ ]]`
// has the same gap pointing the other way: its fold is a byte-wise ASCII one,
// so `[[ ÉTÉ == été ]]` fails here under every locale and matches in bash
// under a UTF-8 one.
func (r *Runner) regexFold() string {
	if r.MatchOption(RegexFoldsCase) {
		return "(?i)"
	}
	return ""
}

// recordRegexMatch stores what a `=~` evaluation captured.
//
// Called for every evaluation, not only matching ones: a failed match stores
// an empty array rather than leaving the previous capture, so a script that
// forgets to check the status reads nothing instead of the match before last.
// And it is the *evaluation* that records, before `!` or `&&` see the result
// — a negated match still fills the record, exactly as the pipeline-status
// record is taken before `!` inverts anything.
//
// An optional group that matched nothing is an empty element, not a gap: the
// record is dense, so the group after it keeps its number.
func (r *Runner) recordRegexMatch(m []string) {
	if r.regexMatchName == "" {
		return
	}
	r.setArray(r.regexMatchName, m)
}

// SetRegexCaptureReport makes a `=~` evaluation publish what it matched
// through the same parameters a *reporting pattern* fills — the whole match,
// the groups, and the character positions of each. A dialect calls it from
// Apply, beside SetRegexMatch.
//
// Two shapes rather than one, because the shells that keep captures disagree
// about more than the name. One keeps a single dense array whose element 0 is
// the whole match; the other splits the whole match from the groups and
// reports where each began and ended, which is the same record a pattern flag
// already writes — so the second shape needs no parameters of its own, only
// permission to use the ones patterncapture.go already knows.
//
// It is a switch and not a name for that reason: the names are the reporting
// pattern's, and a dialect that has one has the other.
func (r *Runner) SetRegexCaptureReport() { r.regexCaptureReport = true }

// publishRegexCapture writes a successful `=~` into the reporting parameters.
//
// Only a successful one, which is measured: a match that fails leaves the
// parameters holding what the match before it put there, exactly as a
// reporting *pattern* that fails does. That is the opposite of the dense
// record beside it, which is emptied on every evaluation — and both are the
// shell they belong to, which is why they are two calls rather than one.
//
// loc is the byte offsets `regexp` reports: a pair per group, the first pair
// being the whole match, and -1 for a group that did not participate.
func (r *Runner) publishRegexCapture(subject string, loc []int) {
	if !r.regexCaptureReport || loc == nil {
		return
	}
	report := matchReport{
		subject:  subject,
		whole:    capSpan{begin: loc[0], end: loc[1], set: true},
		wantsAll: true,
	}
	for i := 2; i+1 < len(loc); i += 2 {
		var span capSpan
		if loc[i] >= 0 {
			span = capSpan{begin: loc[i], end: loc[i+1], set: true}
		}
		report.groups = append(report.groups, span)
	}
	r.publishMatch(report)
}
