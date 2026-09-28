// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// `[n,m]` is the glob qualifier that selects **by position in the match
// list**, and it is not a file attribute at all.
//
// Every other qualifier asks something of a file and is answered per file.
// This one keeps the n-th through m-th of the names the pattern produced,
// which can only be decided once they all are — and once they are in order,
// since the positions are into the *sorted* list. The diagnostic it used to
// get is the tell: `unknown file attribute: [` is the qualifier reader
// reaching the bracket looking for a letter (#4968).
//
// Measured 2026-09-28 on zsh 5.9.2 under `-f` from a script file, in a
// directory holding `a b c d e`:
//
//	*([2,4])     b c d      the n-th through the m-th
//	*([1])       a          one number is one name
//	*([-1])      e          negative counts from the end
//	*([2,-1])    b c d e
//	*([-2,-1])   d e
//	*([2,99])    b c d e    a end past the last is the last
//
// # Where it sits in the pipeline
//
// **After everything else, sort included.** Measured in the same directory
// plus two subdirectories: `*(.[1,2])` and `*([1,3].)` both answer out of the
// files alone, wherever the range is written in the list — so the file tests
// narrow first and the range is taken of what is left. And `*(On[1,2])` is
// `e d`, so the sort has run too: the range is positions into the order the
// pattern finally reports, not into the order the walk found.
//
// # A range that selects nothing is not "no matches"
//
// `*([0])`, `*([9])` and `*([4,2])` each expand to **nothing at status 0** in
// a directory where the pattern itself matched five names, where `zzz*([1])`
// in the same directory is `no matches found` and ends the script. So the
// emptiness a range makes is not the emptiness a pattern makes, and the
// range is applied past the point where a pattern with no matches is refused.
//
// # The numbers are arithmetic
//
// Which is what explains the otherwise odd `*([x])`: `x` is a name, an unset
// name is zero, zero is no position, and the answer is nothing at status 0
// rather than a complaint. The two malformed spellings are the arithmetic
// evaluator's own: `*([1,])` and `*([,2])` are `bad math expression: empty
// string`, and an unterminated `*([1)` is `invalid subscript`.
type globRange struct {
	// from and to are the expressions as written, evaluated when the pattern
	// is expanded rather than when the list is parsed — they may name
	// parameters, and a qualifier list is parsed once per word.
	from, to string
	// set says a range was written at all. `from` alone can be empty text,
	// which is a refusal rather than an absence.
	set bool
}

// readGlobRange reads a `[…]` from the front of a qualifier list, returning
// what it read, how many bytes it consumed and the diagnostic when the
// bracket never closes.
//
// The split is at the **first** comma, which is the whole of the grammar: a
// second one is part of the second expression and the evaluator refuses it
// there. Nesting is not read, because a subscript inside one of these is not
// a shape this qualifier has — the brackets a name carries are inside its own
// expression text and the scan stops at the first unnested `]`.
func readGlobRange(list string) (globRange, int, string) {
	end := strings.IndexByte(list, ']')
	if end < 0 {
		// Measured: `*([1)` is `invalid subscript`, which is the
		// evaluator's sentence for a subscript that never closes rather
		// than the qualifier reader's for a character it does not know.
		return globRange{}, 0, "invalid subscript"
	}
	inside := list[:end]
	from, to, cut := strings.Cut(inside, ",")
	if !cut {
		// One number is one position: `*([1])` is the first name.
		to = from
	}
	return globRange{from: from, to: to, set: true}, end + 1, ""
}

// selectGlobRange keeps the n-th through m-th of a match list, counting from
// one and counting a negative from the end.
//
// Out of range is empty rather than clamped at the near end, and that is two
// separate measured rows: `*([9])` in a five-name directory is nothing, and
// so is `*([4,2])` where the end is in front of the start. What *is* clamped
// is an end past the last name — `*([2,99])` is `b c d e` — so the two ends
// are not symmetrical and a single clamp would have got one of them wrong.
func selectGlobRange(out []string, from, to int) []string {
	n := len(out)
	if from < 0 {
		from = n + from + 1
	}
	if to < 0 {
		to = n + to + 1
	}
	if from < 1 {
		from = 1
	}
	if to > n {
		to = n
	}
	if from > to || from > n {
		return nil
	}
	return out[from-1 : to]
}

// globRangeBounds evaluates the two expressions, which happens at expansion
// time because they may name parameters.
//
// The refusals are the arithmetic evaluator's own and are fatal the way every
// arithmetic refusal in a word is: measured, `*([1,])` is `bad math
// expression: empty string` and nothing after it runs.
func (r *Runner) globRangeBounds(g globRange) (from, to int, ok bool) {
	from, ok = r.globRangeNumber(g.from)
	if !ok {
		return 0, 0, false
	}
	to, ok = r.globRangeNumber(g.to)
	if !ok {
		return 0, 0, false
	}
	return from, to, true
}

func (r *Runner) globRangeNumber(text string) (int, bool) {
	if err := r.emptySubscriptText(text); err != nil {
		// Nothing at all between the brackets or between the comma and the
		// bracket, which this dialect will not read as an expression:
		// measured, `*([1,])` and `*([,2])` are both `bad math expression:
		// empty string` and the script ends. Through emptySubscriptText so
		// that the sentence is the one `${a[1,]}` already gives rather than
		// a second copy of it — the two are the same complaint about the
		// same shape, and a wording written twice is how they would come
		// apart.
		r.fatal("%s\n", err)
		return 0, false
	}
	tree, err := r.arithTreeRead(text)
	if err != nil {
		r.fatal("%s\n", err)
		return 0, false
	}
	n, err := r.evalArith(tree)
	if err != nil {
		r.fatal("%s\n", err)
		return 0, false
	}
	return n, true
}
