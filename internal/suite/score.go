// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"regexp"
	"strings"
)

// Three numbers, and the reason there are three is that each of them lies on
// its own.
//
// **Strict** — the whole file byte-identical, status included — reads as
// catastrophe. A suite file is hundreds of assertions and one disagreement
// forfeits every one of them, so a column at 4% may be wrong about 4% of what
// it does.
//
// **Line agreement** reads as triumph for the mirror-image reason: most lines
// of most files are text the shell echoed back, and two shells agree on an
// echo. It is still worth having, and it has to be a longest common
// subsequence rather than a positional comparison — a file that emits one
// extra line near the top agrees on everything afterwards, and lining the two
// outputs up by index would score that zero and call it a total failure.
//
// **Parsed** is the one that explains the other two. Our parser reads a file
// whole before running any of it, so a single refused construct forfeits a
// file that might have agreed line for line. The distance between parsed and
// strict is the size of *that* effect; the distance between parsed and 100%
// is the gap list.

// tempPattern matches the per-run directory, whose name differs every time
// and would otherwise be a difference in itself.
var tempPattern = regexp.MustCompile(`(/private)?/(tmp|var/folders)/[^\s:"']*`)

// normalize removes what is true of the run rather than of the shell.
//
// The shell's own path appears in its diagnostics and the two shells have
// different ones, so comparing those would report a difference on every file
// that fails — which is most of the interesting ones. The full path only,
// never the base name: replacing a base name turns unrelated words into
// `<shell>` and reports a difference between two identical outputs.
func normalize(out, shell, dir string) string {
	out = strings.ReplaceAll(out, shell, "<shell>")
	if dir != "" {
		out = strings.ReplaceAll(out, dir, "<dir>")
	}
	return tempPattern.ReplaceAllString(out, "<tmp>")
}

// lines splits output for comparison, dropping a single trailing empty line
// so that a file ending in a newline and one that does not are not reported
// as differing by a line neither shell wrote.
func lines(out string) []string {
	if out == "" {
		return nil
	}
	ls := strings.Split(out, "\n")
	if len(ls) > 0 && ls[len(ls)-1] == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

// maxLines bounds the comparison. The DP below is O(n·m) and a suite file can
// print tens of thousands of lines; past this the two are compared on their
// first maxLines, which is reported rather than hidden.
const maxLines = 8000

// agreement is the longest common subsequence of two outputs' lines, as a
// fraction of the longer one, and whether the comparison had to be cut short.
//
// The denominator is the longer side rather than the sum, because the
// question is "how much of this run is right" and an answer of 0.99 for one
// extra line is the reading that matches what a person means by it.
func agreement(a, b []string) (common, longest int, truncated bool) {
	if len(a) > maxLines {
		a, truncated = a[:maxLines], true
	}
	if len(b) > maxLines {
		b, truncated = b[:maxLines], true
	}
	// Identical heads and tails are the overwhelming majority of two nearly
	// agreeing outputs, and trimming them turns the quadratic part into the
	// size of the disagreement rather than the size of the file.
	head := 0
	for head < len(a) && head < len(b) && a[head] == b[head] {
		head++
	}
	a, b = a[head:], b[head:]
	tail := 0
	for tail < len(a) && tail < len(b) && a[len(a)-1-tail] == b[len(b)-1-tail] {
		tail++
	}
	a, b = a[:len(a)-tail], b[:len(b)-tail]

	longest = head + tail + max(len(a), len(b))
	return head + tail + lcs(a, b), longest, truncated
}

// lcs is the length of the longest common subsequence, two rows at a time.
//
// Only the length is wanted, so the table is never built: a full table over
// two eight-thousand-line outputs is sixty-four million cells, and two rows
// is sixteen thousand.
func lcs(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
				continue
			}
			curr[j] = max(prev[j], curr[j-1])
		}
		prev, curr = curr, prev
		clear(curr)
	}
	return prev[len(b)]
}
