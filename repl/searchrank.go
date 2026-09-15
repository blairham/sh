// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"unicode"
)

// Ranked subsequence matching for `C-r`, behind the contiguous search rather
// than instead of it.
//
// A plain substring search is what every shell in the panel does and it is
// what people name when they call a shell dated: `gco` does not find `git
// checkout origin/main`, so the line has to be remembered rather than
// described. A subsequence match finds it — and a subsequence match with no
// ranking is *worse* than a substring one, because it returns too much (#1311).
//
// # The contiguous pass still decides every query that it can
//
// The search runs in two passes and the order is the whole of the
// compatibility argument:
//
//  1. the newest entry at or before the walk's position that *contains* the
//     query, which is exactly what this editor has always done; and
//  2. only if nothing contains it, the best-ranked entry that has the query's
//     characters in order.
//
// So a query that is a substring of anything reaches the same line it reached
// before, in the same order under a repeated `C-r`, at the same cost. That is
// not a nicety: the substring case is the common one, real shells were
// measured for it, and `make smoke` drives `C-r` as one of its rows. The new
// behavior is reached only where the old behavior was a bell.
//
// It also keeps the per-keystroke cost honest. The ranking pass is O(query ×
// line) over the window, and it runs only on a query that has already failed
// to match anything contiguously — which is a query being typed toward
// something that is not there, not the ordinary one.
//
// # What the ranking is made of
//
// The properties are #1311's own list, in the order it puts them, compared
// lexicographically rather than summed into one number. A weighted sum makes
// "contiguity beats a word start" a matter of arithmetic nobody can predict
// from the weights; a lexicographic order says it, and lets each property be
// pinned by a test of its own that no other property can mask.
//
//   - **structure** — contiguity first, then characters at word starts, then a
//     match at the start of the line;
//   - **recency**, because a shell's history is a walk backwards; and
//   - **shortness**, as the last tie-break.

// Weights inside the structural score.
//
// They are only ever compared against each other within one line, so what
// matters is the order and the gaps, not the absolute values. A contiguous
// character is worth more than a word start so that `gco` prefers `git-commit`
// over `git checkout origin`; a word start is worth more than a bare match so
// that it prefers `git checkout` over `logging-checkout`.
const (
	matchBase       = 1
	matchContiguous = 8
	matchWordStart  = 6
	matchAtStart    = 4
)

// wordBreaks are the characters a word is taken to start after.
//
// The set a command line is actually made of: path separators, the two
// spellings of a compound word, the extension dot, and whitespace. It is
// deliberately not "any non-letter" — that would make every character of
// `--flag=value` a word start and flatten the ranking it exists to provide.
const wordBreaks = "/-_. \t:,"

// rankedMatch is one entry's answer to a query.
type rankedMatch struct {
	// index is where the entry is in the history list, so recency is a
	// comparison on it.
	index int
	// start is the rune offset of the first matched character, which is where
	// the cursor goes — the same meaning the contiguous pass's offset has.
	start int
	// score is the structural score: contiguity, word starts, and a match at
	// the start of the line.
	score int
	// length is the entry in runes, for the last tie-break.
	length int
}

// better reports whether m beats other, by the order #1311 lists.
func (m rankedMatch) better(other rankedMatch) bool {
	if m.score != other.score {
		return m.score > other.score
	}
	if m.index != other.index {
		// Newer wins: a shell's history is walked backwards, so among matches
		// that describe the line equally well the recent one is the one being
		// reached for.
		return m.index > other.index
	}
	return m.length < other.length
}

// rankBack is the best subsequence match at or before from, and where in it
// the match begins.
//
// The window is bounded the same way the contiguous search's is, and for the
// same measured reason: extending a query must not jump forward to a more
// recent line, and a repeated `C-r` must keep going back rather than landing
// on what it has just shown. Ranking decides *which* of the entries in the
// window is shown, never that the walk may turn around.
func (e *editor) rankBack(query []rune, from int) (int, int) {
	if len(query) == 0 {
		// An empty query is not a description of anything, and the contiguous
		// pass has already answered it: every entry contains the empty string,
		// so it shows the newest. Ranking nothing would be ranking every line.
		return -1, 0
	}
	from = min(from, len(e.history)-1)

	// Smart case, and only on this pass. The contiguous pass is the
	// compatibility surface — it is what the panel shells were measured for —
	// so its case sensitivity does not move. This pass has no shell to agree
	// with and is reached only where the other one found nothing, so the
	// convention people expect from the tools this is measured against applies
	// cleanly: an all-lowercase query ignores case, and one uppercase
	// character means the case was meant.
	fold := !hasUpper(query)
	// Folded once here rather than at every comparison. The walk compares the
	// query against every rune of every entry, so a ToLower per comparison is
	// paid a few million times for a query nobody has typed twice.
	if fold {
		folded := make([]rune, len(query))
		for i, r := range query {
			folded[i] = unicode.ToLower(r)
		}
		query = folded
	}

	// One scorer for the whole walk. A history search runs on every keystroke
	// of a query that is going nowhere, and converting each entry to runes and
	// allocating the scorer's four rows per candidate is most of what that
	// costs — 50,000 entries meant 50,000 allocations per keystroke, which is
	// the difference between this pass being usable and not.
	var sc searchScorer
	best, found := rankedMatch{}, false
	for i := from; i >= 0; i-- {
		// The cheap test first, and over the string rather than over runes of
		// it: most entries do not hold the query's characters in order at all,
		// and answering that costs one pass and no allocation.
		if !isSubsequence(query, e.history[i], fold) {
			continue
		}
		score, start, length := sc.score(query, e.history[i], fold)
		m := rankedMatch{index: i, start: start, score: score, length: length}
		if !found || m.better(best) {
			best, found = m, true
		}
	}
	if !found {
		return -1, 0
	}
	return best.index, best.start
}

// searchScorer holds the rows the dynamic program walks, so that scoring a
// second candidate reuses the first one's memory.
type searchScorer struct {
	line             []rune
	bonus            []int
	best, next       []int
	start, nextStart []int
}

// score lays the query over one entry as well as it can be laid, and reports
// the score, where the match begins, and the entry's length in runes.
//
// The query arrives already folded when it asked to be, so the entry is folded
// here too and the inner loop compares runes with `==`. And what a *position*
// is worth — the start of the line, the start of a word — depends only on the
// entry, so it is computed once per entry rather than once per cell of the
// table. Both were most of what this cost: the walk touches query × line cells
// per entry, and a case fold and a scan of the word-break set inside each of
// them is several million operations for one keystroke.
func (sc *searchScorer) score(query []rune, line string, fold bool) (int, int, int) {
	sc.line = sc.line[:0]
	for _, r := range line {
		if fold {
			r = unicode.ToLower(r)
		}
		sc.line = append(sc.line, r)
	}
	n := len(sc.line)
	sc.bonus = grow(sc.bonus, n)
	for j := range sc.line {
		switch {
		case j == 0:
			// The start of the line, which also makes it the start of a word.
			sc.bonus[j] = matchAtStart + matchWordStart
		case strings.ContainsRune(wordBreaks, sc.line[j-1]):
			sc.bonus[j] = matchWordStart
		default:
			sc.bonus[j] = 0
		}
	}
	sc.best = grow(sc.best, n)
	sc.next = grow(sc.next, n)
	sc.start = grow(sc.start, n)
	sc.nextStart = grow(sc.nextStart, n)
	score, at := scoreMatch(query, sc.line, sc.bonus, sc.best, sc.next, sc.start, sc.nextStart)
	return score, at, n
}

// grow returns a slice of exactly n ints, reusing the one given where it can.
func grow(s []int, n int) []int {
	if cap(s) >= n {
		return s[:n]
	}
	return make([]int, n)
}

// hasUpper reports whether a query says the case was meant.
func hasUpper(query []rune) bool {
	for _, r := range query {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

// isSubsequence reports whether every rune of query appears in line, in order.
//
// Greedy, which is correct for the question asked: taking the earliest
// available occurrence of each rune can never make a later rune impossible to
// place. It says nothing about *how good* the match is, which is what
// scoreMatch is for.
//
// Over the string rather than a []rune of it, because this is the test every
// entry in the history pays on every keystroke and ranging a string decodes
// without allocating.
func isSubsequence(query []rune, line string, fold bool) bool {
	q := 0
	for _, r := range line {
		if fold {
			r = unicode.ToLower(r)
		}
		if query[q] == r {
			if q++; q == len(query) {
				return true
			}
		}
	}
	return false
}

// scoreMatch is the best-scoring way to lay the query over the line, and where
// that laying begins.
//
// The greedy placement is not the answer here, and that is the reason this is
// a dynamic program rather than one more pass: `gco` over `git checkout` taken
// greedily matches the `g` of `git`, the `c` of `checkout` and then has to
// reach the `o` four characters later, while the run `co` in `checkout` is what
// a person means. Choosing between the two needs the score of every placement,
// so every placement is scored.
//
// best[j] is the score of the best way to match the query up to the current
// rune with that rune landing exactly on line[j], and from[j] is where such a
// match started. Each step needs two predecessors and not all of them — the
// best anywhere to the left, and specifically the one at j-1, because that is
// the only one that can be contiguous — which is what keeps this O(query ×
// line) rather than O(query × line²).
func scoreMatch(query, line []rune, bonus, best, next, start, nextStart []int) (int, int) {
	const none = -1 << 30
	for j := range line {
		best[j], start[j] = none, 0
		if query[0] == line[j] {
			best[j], start[j] = matchBase+bonus[j], j
		}
	}
	for q := 1; q < len(query); q++ {
		// left is the best score for the previous query rune anywhere strictly
		// left of j, and leftStart is where that match began.
		left, leftStart := none, 0
		for j := range line {
			next[j], nextStart[j] = none, 0
			if j > 0 && best[j-1] > left {
				left, leftStart = best[j-1], start[j-1]
			}
			if query[q] != line[j] {
				continue
			}
			if left > none {
				next[j], nextStart[j] = left+matchBase+bonus[j], leftStart
			}
			// The contiguous reading of the same position, which is the one
			// this whole function exists to be able to prefer.
			if j > 0 && best[j-1] > none {
				if s := best[j-1] + matchBase + matchContiguous + bonus[j]; s > next[j] {
					next[j], nextStart[j] = s, start[j-1]
				}
			}
		}
		best, next = next, best
		start, nextStart = nextStart, start
	}

	top, at := none, 0
	for j := range line {
		if best[j] > top {
			top, at = best[j], start[j]
		}
	}
	return top, at
}
