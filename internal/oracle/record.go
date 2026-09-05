// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"fmt"
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
}

// OK reports whether the three artifacts agree.
func (c *RecordCheck) OK() bool {
	return len(c.Unrecorded) == 0 && len(c.Orphaned) == 0 && !c.DocDiffers
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
