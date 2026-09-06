// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// RecordCheck is what the committed artifacts say about each other.
//
// Three files have to agree: the corpus in case.go, the golden record, and
// the rendered measurements. Two of those comparisons need a panel and one
// does not, and keeping them apart is the point.
//
// `oracle -check` asks a fifth of a question the rest of the time: it runs
// the panel and compares behavior, which is the comparison that *cannot* be
// the same on two machines. A runner does not have the same builds of the
// same shells — measured on ubuntu-latest against a record made on macOS, 380
// of 1122 cases differ and bash 3.2 is not installed at all — so that half is
// reported and not enforced, because a check that is red for a legitimate
// reason is one people learn to ignore.
//
// What this asks instead is whether the committed files are consistent *with
// each other*, which needs no shells and gives the same answer everywhere. So
// it can block, and it is the half that catches the ways a record actually
// goes stale in practice: a case added, removed or renamed without
// regenerating, and prose edited after the regeneration that produced the
// document.
type RecordCheck struct {
	// Unrecorded are cases the record has no entry for. The corpus grew and
	// nothing re-measured it.
	Unrecorded []string
	// Orphaned are entries for a case that no longer exists — a removal or,
	// far more often, a rename, which is a removal and an addition.
	Orphaned []string
	// DocDiffers reports that measurements.md is not what rendering the
	// record with the corpus produces.
	DocDiffers bool
	// DocDetail says where the two first part company.
	DocDetail string
	// Restale are cells the current normalizer would still change — the
	// record's text was produced by an older build of it. See
	// stillNormalizable.
	Restale []string
	// RestaleTotal is how many there are, where Restale lists only the first
	// few.
	RestaleTotal int
}

// OK reports whether the three artifacts agree.
func (c *RecordCheck) OK() bool {
	return len(c.Unrecorded) == 0 && len(c.Orphaned) == 0 && !c.DocDiffers &&
		c.RestaleTotal == 0
}

// CheckRecord compares the corpus, the golden record and the rendered
// document against one another.
//
// doc is the committed measurements file. The comparison is possible at all
// because the document is a pure function of the other two — Markdown reads
// nothing else — so re-rendering and comparing every byte asks the question
// completely rather than sampling it.
//
// That is what makes an edited Why fail. `oracle -check` compares *behavior*,
// and a Why is prose, so editing one after `make oracle` has run left the
// document carrying the old sentence while case.go carried the new one and
// nothing failed. It happened twice in one day, caught by eye both times, and
// a wrong explanation attached to a correct measurement is worse than a wrong
// number: the numbers are checked and the prose was not.
func CheckRecord(golden *Run, doc string, cases []Case) *RecordCheck {
	c := &RecordCheck{}
	if golden == nil {
		return c
	}
	live := make(map[string]bool, len(cases))
	for _, cs := range cases {
		live[cs.ID] = true
		if _, ok := golden.Results[cs.ID]; !ok {
			c.Unrecorded = append(c.Unrecorded, cs.ID)
		}
	}
	for id := range golden.Results {
		if !live[id] {
			c.Orphaned = append(c.Orphaned, id)
		}
	}
	sort.Strings(c.Unrecorded)
	sort.Strings(c.Orphaned)

	want, got := portable(golden.Markdown(cases)), portable(doc)
	if want != got {
		c.DocDiffers = true
		c.DocDetail = firstDifference(want, got)
	}
	c.Restale, c.RestaleTotal = stillNormalizable(golden)
	return c
}

// signalWord matches the system's word for a signal, beside the number the
// record actually keeps.
var signalWord = regexp.MustCompile(`(killed by signal \d+) \([^)]*\)`)

// portable removes the one thing in the rendered document that is a property
// of the reader's kernel rather than of the measurement.
//
// The document is otherwise a pure function of the record and the corpus,
// which is the whole basis for comparing it byte for byte. The exception is
// the word beside a signal number: it comes from syscall.Signal.String(),
// which is the operating system's spelling, and the number in the record is
// the one the *recording* machine used. Signal 30 is SIGUSR1 on macOS and
// SIGPWR on Linux, so a name derived from a recorded number is wrong away
// from the machine that recorded it — deriving it from the constant instead
// only moves the problem, because the constant's value is what differs.
//
// This was found by the check failing on a Linux runner and passing on macOS
// for a byte-identical tree, which is the failure mode a machine-independent
// gate must not have.
//
// Nothing is lost by excluding it. The word is computed from the number, so
// it carries no fact the number does not, and it is removed from *both* sides
// rather than from the committed file — the document keeps its words for a
// reader, and the comparison stops pretending they are evidence.
func portable(doc string) string {
	return signalWord.ReplaceAllString(doc, "$1")
}

// String is the report, or empty when there is nothing to report.
func (c *RecordCheck) String() string {
	if c.OK() {
		return ""
	}
	var b strings.Builder
	if len(c.Unrecorded) > 0 {
		fmt.Fprintf(&b, "%d case(s) the golden record has no entry for:\n", len(c.Unrecorded))
		for _, id := range c.Unrecorded {
			fmt.Fprintf(&b, "  %s\n", id)
		}
	}
	if len(c.Orphaned) > 0 {
		fmt.Fprintf(&b, "%d record entr(ies) for a case that no longer exists:\n", len(c.Orphaned))
		for _, id := range c.Orphaned {
			fmt.Fprintf(&b, "  %s\n", id)
		}
	}
	if c.DocDiffers {
		b.WriteString("the generated measurements are not what the record and the corpus render:\n")
		b.WriteString(c.DocDetail)
	}
	if c.RestaleTotal > 0 {
		fmt.Fprintf(&b, "%d recorded cell(s) the normalizer would still change, so the record's\n"+
			"text was produced by an older build of it:\n", c.RestaleTotal)
		for _, s := range c.Restale {
			fmt.Fprintf(&b, "  %s\n", s)
		}
		if c.RestaleTotal > len(c.Restale) {
			fmt.Fprintf(&b, "  … and %d more\n", c.RestaleTotal-len(c.Restale))
		}
	}
	b.WriteString("\nThe committed files disagree with each other, which needs no shell to\n" +
		"see and is the same on every machine. Run `make oracle`.\n")
	return b.String()
}

// firstDifference says where two renderings part company, in a way a reader
// can act on.
//
// A byte count is not actionable and a whole diff is unreadable at this size,
// so this names the line and shows both sides of it. The common cause is an
// edited Why, and a Why is one line of a table cell — visible immediately once
// the line is in front of you.
func firstDifference(want, got string) string {
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := range min(len(wl), len(gl)) {
		if wl[i] != gl[i] {
			return fmt.Sprintf("  line %d\n    committed: %s\n    rendered:  %s\n",
				i+1, truncate(gl[i]), truncate(wl[i]))
		}
	}
	return fmt.Sprintf("  the committed file has %d lines and the rendering has %d\n", len(gl), len(wl))
}

// truncate keeps a long table row readable in a terminal.
func truncate(s string) string {
	const width = 160
	if len(s) <= width {
		return s
	}
	return s[:width] + "…"
}

// stillNormalizable finds cells the current normalizer would still change.
//
// This is the gap #660 actually was, and #777 recorded so it would not be
// found a third time. #660 added a normalization rule — the shell's name
// inside a `Usage:` block — and committed a record generated *before* it, so
// five cells kept a name the rule would have stripped. Nothing in the tree
// disagreed with anything else: the record was internally consistent, the
// document rendered from it exactly, and TestTheCommittedRecordAndDocument-
// Agree was green. Only a live panel run knew, and that comparison cannot be
// a gate — 380 of 1122 cases differ between a macOS-made record and an
// ubuntu-latest runner, so "the record is stale" and "this is a different
// machine" are the same red on a runner.
//
// The question this asks instead needs no shells and gives the same answer
// everywhere: **is every recorded cell already a fixed point of the
// normalizer we have now?** A cell is what normalize produced, so re-running
// normalize over it must change nothing. A cell that *does* change was made
// by an older build, and the text it still carries is exactly the text the
// new rule was written to remove.
//
// The record keeps a cell with its newlines flattened to `~`, which is
// normalize's own last step, so they are put back before the rules run and
// taken out again after: every rule that anchors to the start of a line —
// which is most of them, because that is where a shell names itself — would
// otherwise have nothing to anchor to.
//
// The shell's *path* is not in the record and does not need to be. What the
// name-stripping rules read is its basename, and the panel entry lists the
// paths it is ever found at, so every basename that machine could have used
// is known here. All of them are tried: a record made where ksh93 was
// installed as `ksh93` and one made where it was `ksh` must both come back
// clean, and trying both is also what makes this stricter than the machine
// that recorded it.
//
// What it does not catch, said plainly rather than left to be discovered: a
// rule that got *looser*. Those leave old cells over-normalized — carrying
// less than a fresh run would — and the text that would show it was thrown
// away at record time. Answering that needs the raw output kept beside every
// cell, which is a much larger record for a rarer mistake.
func stillNormalizable(golden *Run) (first []string, total int) {
	const listed = 10
	byName := make(map[string]Shell, len(Panel))
	for _, s := range Panel {
		byName[s.Name] = s
	}
	ids := make([]string, 0, len(golden.Results))
	for id := range golden.Results {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		row := golden.Results[id]
		shells := make([]string, 0, len(row))
		for name := range row {
			shells = append(shells, name)
		}
		sort.Strings(shells)
		for _, name := range shells {
			s, known := byName[name]
			if !known {
				// A column from a panel this build no longer has. The
				// orphan check is about cases; this one would be about
				// shells, and there is no rule to re-run without one.
				continue
			}
			res := row[name]
			for _, cell := range []struct{ stream, text string }{
				{"stdout", res.Stdout}, {"stderr", res.Stderr},
			} {
				if cell.text == "" {
					continue
				}
				for _, lookup := range s.Lookup {
					sh := Found{Shell: s, Path: filepath.Join(unusedRoot, filepath.Base(lookup))}
					again := normalize(strings.ReplaceAll(cell.text, "~", "\n"), sh, unusedTmp)
					if again == cell.text {
						continue
					}
					total++
					if len(first) < listed {
						first = append(first, fmt.Sprintf("%s [%s %s] as %s\n      recorded:   %s\n      normalizes: %s",
							id, name, cell.stream, filepath.Base(lookup),
							truncate(cell.text), truncate(again)))
					}
					break
				}
			}
		}
	}
	return first, total
}

// unusedRoot and unusedTmp stand where a live run had a real shell path and a
// real scratch directory.
//
// normalize replaces both by literal text, and neither can appear in a cell:
// a cell that named either would have had it replaced when it was recorded,
// which is the whole point of those two rules. So a path that exists nowhere
// leaves the literal replacements inert and lets the rules that read the
// shell's *name* — the ones a stale cell fails — do the talking.
const (
	unusedRoot = "/nonexistent/oracle/renormalize"
	unusedTmp  = "/nonexistent/oracle/renormalize/tmp"
)
