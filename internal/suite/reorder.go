// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// A differing line is not always a line anybody here may produce, and
// [Doc] is only one of the two ways that happens. The other is an **order**.
//
// An associative array has no order a script can ask for, and a reference
// shell lists its keys in the order its own hash table holds them. Measured
// 2026-09-16 and again 2026-09-18, each probe from a script file with
// standard input on the null device:
//
//	inserted                 reference             here
//	zebra apple mango 0 !    0 ! apple mango zebra ! 0 apple mango zebra
//	one two three            two three one         one three two
//	c b a                    c b a                 a b c
//
// The second row is the discriminator: inserted `one two three`, the
// reference lists `two three one`, so this is not insertion order either.
// Reproducing the sequence means reproducing that shell's hash function, its
// table size and its growth policy, none of which is in any vendor manual and
// all of which is in the source `CLEANROOM.md`'s red list covers. So it is a
// **floor** rather than a defect, and #3304 is where the cost is recorded.
//
// # Two uses, and they are not the same claim
//
// [canonicalOrder] is a per-side function: it rewrites one run's lines into a
// form that does not depend on the order. Whether that is *sound* depends
// entirely on which two runs are being compared.
//
// Against the reference's **own second run** — [repeats] — it is sound with
// nothing to argue about. One shell disagreeing with itself about the order
// of the same keys is not a fact about either shell, and until this the file
// was reported *unstable* and scored nothing at all: `assoc.tests` will not
// reproduce its own run, because one two-key table printed three times at the
// end comes out in a different order each time.
//
// Against **our** run it is not sound, and this deliberately subtracts
// nothing. A per-side sort would hide an order that is genuinely ours to get
// right, and nothing downstream would ever say so. What it does instead is
// report a second figure beside [Report.Prose] — how many of the differing
// lines go away when order is ignored — which is an **upper bound** on the
// floor and corrects neither of the other numbers.
//
// # What it does not reach
//
// A rotation **across** lines. `assoc4.sub` is five groups of five, each a
// joined listing and its four elements, ours rotated by one against the
// reference; a window of lines that is a permutation is a different detection
// and is deliberately left to be decided separately rather than folded in
// here under a name that would make it look covered.

// pairPattern is one `[key]="value"` of a listed array, as the reference's
// own `declare -p` writes one. The key is taken to the first `]` and the
// value to the closing quote, so a bracket inside the value cannot end the
// pair early.
var pairPattern = regexp.MustCompile(`\[[^\]]*\]="(?:[^"\\]|\\.)*"`)

// echoedWord is one word of the suite's own `recho` helper, which writes each
// word of an expansion on a line of its own.
var echoedWord = regexp.MustCompile(`^argv\[(\d+)\] = <(.*)>$`)

// canonicalOrder rewrites lines into a form that does not depend on an order
// the reference's hash table chose.
//
// Two shapes and nothing else, which is the whole of the care in it: a rule
// that canonicalised more than it can name would quietly reach a line whose
// order is ours to get right.
//
//   - a line holding two or more `[key]="value"` pairs, which is a listed
//     array, has its pairs sorted and everything around them left alone;
//   - a run of consecutive `argv[N] = <word>` lines, which is one expansion
//     written a word to a line, is sorted by word and renumbered from where
//     the run started.
//
// A pure function of one side. It never compares anything, so it cannot be
// told what the other run did and cannot be tuned to make two runs agree.
func canonicalOrder(ls []string) []string {
	out := make([]string, 0, len(ls))
	for i := 0; i < len(ls); i++ {
		if n := echoedRun(ls[i:]); n > 1 {
			out = append(out, sortedEchoedRun(ls[i:i+n])...)
			i += n - 1
			continue
		}
		out = append(out, sortedPairs(ls[i]))
	}
	return out
}

// sortedPairs sorts the `[key]="value"` pairs of one line in place, leaving
// every other byte of the line where it was.
//
// Two or more, because one pair has no order to it and rewriting a line that
// held one would only widen what this function touches.
func sortedPairs(line string) string {
	found := pairPattern.FindAllStringIndex(line, -1)
	if len(found) < 2 {
		return line
	}
	pairs := make([]string, len(found))
	for i, at := range found {
		pairs[i] = line[at[0]:at[1]]
	}
	sort.Strings(pairs)
	var b strings.Builder
	prev := 0
	for i, at := range found {
		b.WriteString(line[prev:at[0]])
		b.WriteString(pairs[i])
		prev = at[1]
	}
	b.WriteString(line[prev:])
	return b.String()
}

// echoedRun is how many lines from the front of ls are consecutive
// `argv[N] = <word>` rows **numbered upward from 1**, which is one call of
// the helper.
//
// The numbering is what bounds the run rather than the shape alone: two calls
// in a row would otherwise be read as one, and sorting across a boundary
// would move a word from one expansion into another.
func echoedRun(ls []string) int {
	n := 0
	for _, line := range ls {
		m := echoedWord.FindStringSubmatch(line)
		if m == nil || m[1] != fmt.Sprint(n+1) {
			break
		}
		n++
	}
	return n
}

// sortedEchoedRun sorts one call's words and renumbers them from 1.
func sortedEchoedRun(run []string) []string {
	words := make([]string, len(run))
	for i, line := range run {
		words[i] = echoedWord.FindStringSubmatch(line)[2]
	}
	sort.Strings(words)
	out := make([]string, len(run))
	for i, w := range words {
		out[i] = fmt.Sprintf("argv[%d] = <%s>", i+1, w)
	}
	return out
}

// sameButForOrder reports whether two runs differ only in an order neither
// shell was asked for.
//
// The comparison [repeats] makes when the plain one has already failed. It
// is the reference against its own second run, which is what makes a per-side
// canonicalisation sound here and unsound one caller over.
func sameButForOrder(a, b string) bool {
	if a == b {
		return true
	}
	la, lb := canonicalOrder(lines(a)), canonicalOrder(lines(b))
	if len(la) != len(lb) {
		return false
	}
	for i := range la {
		if la[i] != lb[i] {
			return false
		}
	}
	return true
}

// reordered is how many of two runs' differing lines go away when order is
// ignored.
//
// An **upper bound** on the floor and not a correction: nothing subtracts it,
// for the reason the file comment gives. It is clamped into the differing
// lines it is reported beside, so a canonicalisation that happened to align
// more lines than the raw comparison did cannot report more differing lines
// than there were.
func reordered(mine, theirs []string, differing int) int {
	if differing <= 0 {
		return 0
	}
	common, longest, _ := agreement(canonicalOrder(mine), canonicalOrder(theirs))
	return min(max(differing-(longest-common), 0), differing)
}
