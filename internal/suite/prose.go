// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// A differing line is not always a line anybody here may produce.
//
// Some of what a reference shell prints is that shell's *own documentation* —
// the text of its help builtin, the usage block it answers a bad option with,
// the version and license it announces. Reproducing those means copying them,
// and CLEANROOM.md's red list covers exactly that. They are as unavailable to
// this project as its source is, and a burndown ranked by differing lines
// without separating them sends somebody to work on text they may not write.
//
// It is not a small effect. Measured on the bash column, 2026-09-13: one file
// of the suite calls the help builtin throughout, and two thirds of its
// differing lines are that builtin printing manual pages at itself.
//
// So the report carries a second number beside the differing lines: how many
// of them are the reference quoting its own documentation. Neither figure is
// the other's correction — the raw count is what the two runs did, and this
// is how much of it was ever available.
//
// # What makes this clean
//
// Nothing here is read by a person and nothing is printed. The shell is asked
// for its own documentation, the answer is held as a set of lines, and a
// program tests membership and reports a count — the same standing the
// longest common subsequence over the same bytes already has, and for the
// reason the package comment gives: a program matching bytes is not a person
// forming a derivative work.
//
// The dictionary is built by *running the reference*, never by reading
// anything. It is per-run and in memory, and no part of it reaches the
// report.

// Doc is one reference shell's own documentation, as two sets of lines.
//
// The zero value attributes nothing, which is what a column with no
// [Suite.SelfDoc] gets: an unasked question, counted as zero rather than
// guessed at.
//
// Two sets rather than one because a block of documentation is not only its
// sentences. `lines` holds the prose — what a sentence looks like is the
// length-and-a-space rule below — and is what *anchors* an attribution.
// `block` holds every line the shell wrote, the blank lines and the one-word
// headings of the same pages included, and is only ever crossed from an
// anchor. See [Doc.Attribute] for why that pair is what makes the number
// honest without letting it run away.
type Doc struct {
	lines map[string]bool
	block map[string]bool
}

// docTimeout bounds the collection. Asking a shell to print its own help is
// bounded by how much of it there is.
const docTimeout = 30 * time.Second

// SelfDocumentation asks a shell to print its own documentation and keeps the
// lines.
//
// The command is the suite's, because how a shell is asked for its help is
// the shell's own vocabulary and not something to guess at from another
// column's. A column without one gets an empty [Doc] and attributes nothing.
func SelfDocumentation(ctx context.Context, s Suite, shell string) Doc {
	if s.SelfDoc == "" {
		return Doc{}
	}
	doc := Doc{lines: map[string]bool{}}
	ctx, cancel := context.WithTimeout(ctx, docTimeout)
	defer cancel()
	for _, args := range [][]string{{"-c", s.SelfDoc}, {"--help"}, {"--version"}} {
		cmd := exec.CommandContext(ctx, shell, args...)
		// The same C locale the runs get: a dictionary collected in one
		// locale and matched against output produced in another attributes
		// nothing and looks like a finding.
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "LANG=C", "TERM=dumb"}
		cmd.Stdin = nil
		out, _ := cmd.CombinedOutput()
		doc.add(string(out))
	}
	return doc
}

func (d *Doc) add(out string) {
	if d.lines == nil {
		d.lines = map[string]bool{}
	}
	if d.block == nil {
		d.block = map[string]bool{}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		// Everything the shell wrote goes in the block set, blanks and
		// headings included: those are what a page of documentation is made
		// of between its sentences, and leaving them out is what made the
		// figure understate. Membership in this set is never enough on its
		// own — see [Doc.Attribute].
		d.block[line] = true
		// Blank and one-word lines are dropped from the anchors. A
		// dictionary that matched `done` or `fi` would attribute a file's own
		// echoes to documentation, and the number this exists to make honest
		// would start overstating in the other direction.
		if len(line) < docMinLen || !strings.Contains(line, " ") {
			continue
		}
		d.lines[line] = true
	}
}

// docMinLen is the shortest line the dictionary keeps. A shell's help text is
// prose; a short line that happens to appear in both is far more likely to be
// a file's own output than a quotation of the manual.
const docMinLen = 12

// Empty says the dictionary was never built, so its counts mean "not asked"
// rather than "none".
func (d Doc) Empty() bool { return len(d.lines) == 0 }

// Attribute counts, among the reference's lines that our run did not produce
// at all, how many are lines of the reference's own documentation.
//
// The unmatched side is a multiset difference rather than the alignment the
// longest common subsequence would give: a line we printed somewhere is not a
// line we failed to produce, wherever it landed. That makes this a *lower*
// bound on what the subsequence left unmatched, which is the direction an
// honest discount has to err in — every line counted here is one the
// reference printed and we printed nowhere.
//
// # Why a page is attributed and not only its sentences
//
// A line-at-a-time membership test undercounts, and by a lot. A page of
// documentation is sentences with blank lines between them, indented
// continuations, and one-word headings — `NAME`, `SYNOPSIS`, `SEE ALSO` —
// and every one of those fails the length-and-a-space rule that keeps a
// file's own `done` out of the dictionary. Measured on the builtins file of
// the bash column, 2026-09-22: 52 differing lines, of which 50 are the help
// builtin's output, and the per-line test attributed 28. The 22 it missed
// were the blanks and the headings *of the pages it had already attributed*,
// so the report said there were 24 lines of work in a file that has two.
//
// So the walk is over runs rather than lines. A line in the prose set is an
// **anchor**; from an anchor the run extends outward through neighboring
// unmatched lines for as long as each one is something the shell itself
// wrote, and stops at the first line that is not. Crossing is what the
// second set is for, and it is never a starting point: a blank line or a
// bare `fi` is attributed only when the page it sits in already was.
func (d Doc) Attribute(mine, theirs []string) int {
	if d.Empty() {
		return 0
	}
	have := map[string]int{}
	for _, line := range mine {
		have[strings.TrimSpace(line)]++
	}
	// unmatched is the multiset difference, kept positionally so the runs
	// below are the reference's own order.
	unmatched := make([]bool, len(theirs))
	trimmed := make([]string, len(theirs))
	attributed := make([]bool, len(theirs))
	for i, line := range theirs {
		line = strings.TrimSpace(line)
		trimmed[i] = line
		if have[line] > 0 {
			have[line]--
			continue
		}
		unmatched[i] = true
		attributed[i] = d.lines[line]
	}
	// Two passes and no more. A run only ever grows outward from an anchor,
	// so one sweep each way reaches every line either sweep could: a line the
	// forward pass marks can extend the run rightwards alone, and the
	// backward pass is the mirror of that.
	crosses := func(i int) bool { return unmatched[i] && d.block[trimmed[i]] }
	for i := 1; i < len(theirs); i++ {
		if attributed[i-1] && !attributed[i] && crosses(i) {
			attributed[i] = true
		}
	}
	for i := len(theirs) - 2; i >= 0; i-- {
		if attributed[i+1] && !attributed[i] && crosses(i) {
			attributed[i] = true
		}
	}
	n := 0
	for _, ok := range attributed {
		if ok {
			n++
		}
	}
	return n
}
