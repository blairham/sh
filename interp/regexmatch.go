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
