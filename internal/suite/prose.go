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

// Doc is one reference shell's own documentation, as a set of lines.
//
// The zero value attributes nothing, which is what a column with no
// [Suite.SelfDoc] gets: an unasked question, counted as zero rather than
// guessed at.
type Doc struct {
	lines map[string]bool
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

func (d Doc) add(out string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		// Blank and one-word lines are dropped. A dictionary that matched
		// `done` or `fi` would attribute a file's own echoes to
		// documentation, and the number this exists to make honest would
		// start overstating in the other direction.
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
func (d Doc) Attribute(mine, theirs []string) int {
	if d.Empty() {
		return 0
	}
	have := map[string]int{}
	for _, line := range mine {
		have[strings.TrimSpace(line)]++
	}
	n := 0
	for _, line := range theirs {
		line = strings.TrimSpace(line)
		if have[line] > 0 {
			have[line]--
			continue
		}
		if d.lines[line] {
			n++
		}
	}
	return n
}
