// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package coverage

import (
	"fmt"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// Column is one dialect's run: its grammar, its builtins, and the cases that
// parsed under it.
type Column struct {
	Name     string
	Dialect  syntax.Dialect
	Builtins []string

	// Sources is how many case texts were offered, Parsed how many this
	// dialect's grammar read. The difference is not a finding about coverage
	// — a case written for another shell is not this one's surface — but it
	// is the denominator of the denominator, and a report that hid it would
	// let a grammar break and still look well covered.
	Sources, Parsed int

	Surface  []Element
	Mentions map[Element]int
}

// Source is one piece of shell text to be read, with a label a report can
// name when it will not parse.
type Source struct {
	Label string
	Text  string
}

// Run reads every source under one dialect and returns the column.
func Run(name string, d syntax.Dialect, builtins []string, srcs []Source) (Column, error) {
	surface, err := Surface(builtins)
	if err != nil {
		return Column{}, err
	}
	if len(surface) == 0 {
		return Column{}, ErrNoSurface
	}
	set := make(map[string]bool, len(builtins))
	for _, b := range builtins {
		set[b] = true
	}
	col := Column{
		Name: name, Dialect: d, Builtins: builtins,
		Sources: len(srcs), Surface: surface, Mentions: map[Element]int{},
	}
	for _, s := range srcs {
		f, err := syntax.Parse(s.Text, d)
		if err != nil {
			// Not an error of this instrument's: the corpus records
			// rejections on purpose, and a case written for another shell's
			// grammar is not this column's surface.
			continue
		}
		col.Parsed++
		if err := Mentions(f, func(n string) bool { return set[n] }, col.Mentions); err != nil {
			return Column{}, err
		}
	}
	return col, nil
}

// Unasked is every element of the column's surface that nothing mentioned.
func (c Column) Unasked() []Element {
	var out []Element
	for _, e := range c.Surface {
		if c.Mentions[e] == 0 {
			out = append(out, e)
		}
	}
	byKindThenName(out)
	return out
}

// ByKind is the per-kind tally: how many elements, and how many were
// mentioned at least once.
func (c Column) ByKind() []KindTally {
	total := map[string]int{}
	asked := map[string]int{}
	for _, e := range c.Surface {
		total[e.Kind]++
		if c.Mentions[e] > 0 {
			asked[e.Kind]++
		}
	}
	out := make([]KindTally, 0, len(total))
	for k, n := range total {
		out = append(out, KindTally{Kind: k, Total: n, Asked: asked[k]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// KindTally is one row of the per-kind table.
type KindTally struct {
	Kind         string
	Total, Asked int
}

// Origin is one body of cases a report was computed from: a name, and how
// many of them there were.
//
// It is printed under the title because the denominator of this instrument is
// whatever it was handed, and a report that does not say what it read cannot
// be read twice and compared. It arrived the hard way: `make coverage` ran
// for a day against the corpus alone while our own suite sat in the tree
// unread, and the report named three elements as unasked that 35 committed
// files had been asking about (#2630). Nothing was wrong with the number —
// it was the honest answer to a question nobody meant to put.
//
// A Count of zero prints as a name alone, which is how a body of cases that
// could not be read says so.
type Origin struct {
	Name  string
	Count int
}

// Report renders the columns. list caps how many unasked names are printed
// per kind; 0 prints them all. read is where the cases came from, and it is
// printed under the title — see [Origin].
func Report(cols []Column, list int, read []Origin) string {
	return ReportWithLedger(cols, list, read, UnreachableByConstruction)
}

// ReportWithLedger is [Report] held against a ledger of unreachable elements
// other than the committed one. For tests.
func ReportWithLedger(cols []Column, list int, read []Origin, ledger []Unreachable) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  coverage — what the cases never mention\n")
	if len(read) > 0 {
		var parts []string
		for _, o := range read {
			if o.Count == 0 {
				parts = append(parts, o.Name)
				continue
			}
			parts = append(parts, fmt.Sprintf("%d %s", o.Count, o.Name))
		}
		fmt.Fprintf(&b, "  read %s\n", strings.Join(parts, ", "))
	}
	b.WriteString("\n")
	for _, c := range cols {
		fmt.Fprintf(&b, "  %s\n", c.Name)
		fmt.Fprintf(&b, "    %d of %d sources parsed under this grammar\n", c.Parsed, c.Sources)
		for _, t := range c.ByKind() {
			miss := t.Total - t.Asked
			fmt.Fprintf(&b, "      %-24s %4d of %4d mentioned   %4d never\n",
				t.Kind, t.Asked, t.Total, miss)
		}
		byKind := map[string][]string{}
		for _, e := range c.Unasked() {
			byKind[e.Kind] = append(byKind[e.Kind], e.Name)
		}
		kinds := make([]string, 0, len(byKind))
		for k := range byKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			names := byKind[k]
			shown := names
			if list > 0 && len(shown) > list {
				shown = shown[:list]
			}
			fmt.Fprintf(&b, "\n      never mentioned — %s (%d)\n        %s",
				k, len(names), strings.Join(shown, " "))
			if len(shown) < len(names) {
				fmt.Fprintf(&b, " … and %d more", len(names)-len(shown))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(rollup(cols, list, ledger))
	b.WriteString(caveat)
	return b.String()
}

// rollup is the strongest finding in the report: an element no dialect's
// cases mention at all.
//
// The per-dialect lists above are weaker than they look, because the operator
// vocabulary is one table shared by every grammar. `${x^^}` is bash's, so the
// case that pins it does not parse under zsh and the operator reads as never
// mentioned *there* — which is right and is not a work item. An element
// missing from every column has no such excuse.
//
// An element in the ledger (see [Unreachable]) is taken out of that count and
// printed below it with its measurement, so the headline is the number of
// *reachable* elements nothing asks about — the one that can be driven to
// zero. The subtraction is checked rather than trusted: a ledger entry some
// column mentions, or that no column's surface holds, is printed as stale.
func rollup(cols []Column, list int, ledger []Unreachable) string {
	if len(cols) == 0 {
		return ""
	}
	roll := Unmentioned(cols, ledger)
	union, never := roll.Surface, roll.Never
	unreachable, stale := roll.Unreachable, roll.Stale

	var b strings.Builder
	fmt.Fprintf(&b, "  no dialect mentions these at all — %d of %d reachable\n", len(never), union-len(unreachable))
	fmt.Fprintf(&b, "    (%d in the surface, %d unreachable by construction and listed below)\n\n", union, len(unreachable))
	byKind := map[string][]string{}
	for _, e := range never {
		byKind[e.Kind] = append(byKind[e.Kind], e.Name)
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		names := byKind[k]
		shown := names
		if list > 0 && len(shown) > list {
			shown = shown[:list]
		}
		fmt.Fprintf(&b, "    %s (%d)\n      %s", k, len(names), strings.Join(shown, " "))
		if len(shown) < len(names) {
			fmt.Fprintf(&b, " … and %d more", len(names)-len(shown))
		}
		b.WriteString("\n")
	}
	if len(kinds) > 0 {
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "  unreachable by construction — %d, each measured rather than covered\n\n", len(unreachable))
	for _, u := range unreachable {
		fmt.Fprintf(&b, "    %s (#%d)\n", u.Element, u.Issue)
		b.WriteString(wrap(u.Measured, "      ", 72))
	}
	if len(unreachable) > 0 {
		b.WriteString("\n")
	}
	if len(stale) > 0 {
		fmt.Fprintf(&b, "  STALE ledger entries — %d: listed as unreachable, and the columns say otherwise\n\n", len(stale))
		for _, u := range stale {
			why := "some case mentions it"
			if !roll.InSurface[u.Element] {
				why = "no dialect's surface holds it"
			}
			fmt.Fprintf(&b, "    %s (#%d) — %s; delete the entry\n", u.Element, u.Issue, why)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Rollup is the roll-up the report prints, as data rather than as text.
//
// One computation for the report and for the guard that gates it, because
// two would be two answers: [Report] renders this and
// internal/cmd/coverage's own test fails when [Rollup.Never] is not empty.
// See [Unmentioned].
type Rollup struct {
	// Surface is how many distinct elements the columns hold between them.
	Surface int
	// InSurface is that set, for asking about one element.
	InSurface map[Element]bool
	// Never is every reachable element no column mentions — the work-list,
	// and the number that is meant to be zero.
	Never []Element
	// Unreachable is the ledger entries the columns agree with: an element
	// nothing mentions and nothing can. Subtracted from the headline.
	Unreachable []Unreachable
	// Stale is the ledger entries the columns contradict — mentioned after
	// all, or gone from every surface. A ledger that could only grow is the
	// overstatement this instrument exists to prevent.
	Stale []Unreachable
}

// Unmentioned is the roll-up: what no column mentions, split by whether the
// ledger accounts for it.
//
// Exported because it is the one number worth gating, and a gate that
// recomputed it from the printed report would be reading its own prose. See
// [Rollup] and internal/cmd/coverage.
func Unmentioned(cols []Column, ledger []Unreachable) Rollup {
	roll := Rollup{InSurface: map[Element]bool{}}
	mentioned := map[Element]bool{}
	for _, c := range cols {
		for _, e := range c.Surface {
			roll.InSurface[e] = true
			if c.Mentions[e] > 0 {
				mentioned[e] = true
			}
		}
	}
	roll.Surface = len(roll.InSurface)
	ledgered := map[Element]Unreachable{}
	for _, u := range ledger {
		ledgered[u.Element] = u
	}
	for e := range roll.InSurface {
		if mentioned[e] {
			continue
		}
		if u, ok := ledgered[e]; ok {
			roll.Unreachable = append(roll.Unreachable, u)
			continue
		}
		roll.Never = append(roll.Never, e)
	}
	for _, u := range ledger {
		if !roll.InSurface[u.Element] || mentioned[u.Element] {
			roll.Stale = append(roll.Stale, u)
		}
	}
	byKindThenName(roll.Never)
	byLedgerOrder(roll.Unreachable)
	return roll
}

func byLedgerOrder(us []Unreachable) {
	es := make([]Element, len(us))
	byEl := make(map[Element]Unreachable, len(us))
	for i, u := range us {
		es[i] = u.Element
		byEl[u.Element] = u
	}
	byKindThenName(es)
	for i, e := range es {
		us[i] = byEl[e]
	}
}

// wrap fills text to width under indent, one line per chunk.
func wrap(text, indent string, width int) string {
	var b strings.Builder
	line := indent
	for _, w := range strings.Fields(text) {
		if len(line) > len(indent) && len(line)+1+len(w) > width {
			b.WriteString(line + "\n")
			line = indent
		}
		if len(line) > len(indent) {
			line += " "
		}
		line += w
	}
	if len(line) > len(indent) {
		b.WriteString(line + "\n")
	}
	return b.String()
}

const caveat = `  What this counts, and what it does not

    A mention is not coverage. A case naming ` + "`printf`" + ` and checking only
    that it exited 0 is scored here exactly as one that pins every
    conversion, so the "mentioned" column is an upper bound and reads
    higher than the truth. It is the *zero* that is a fact: an element
    nothing mentions is covered by nothing at all, and that half of this
    report is not a proxy. Read the never-mentioned lists as the work-list
    and the percentages as nothing at all.

    A per-dialect zero is weaker than the roll-up above it. The operator
    vocabulary is one table shared by every grammar, so an operator only
    one shell has reads as never mentioned under the other five — which
    is correct and is not a work item. An element no column mentions is
    the one with no such excuse, which is why it is reported separately.

    The Semantics axes are deliberately absent, and they are not
    unmeasured: ` + "`make axis-coverage`" + ` names every axis a dialect leaves
    unanswered and ` + "`make axis-sweep`" + ` moves each answered one and reports
    what fails to object. Both are better questions than counting the
    cases that mention an axis — which could not be counted anyway, since
    an axis leaves no mark in the text.

    The grammar readings are absent for the same reason. A type
    syntax.Dialect declares a field of — BraceQuotePolicy is the one a
    node carries — is decided by the dialect and not spelled by the
    case, and a node field holding one cannot tell unset from its zero
    (#3258). The corpus rows each Dialect field cites measure those.

    The roll-up counts reachable elements. An element no case can ask
    without stopping the harness is listed with its measurement instead,
    and a listing the columns contradict is printed as stale.

    The roll-up's zero is the one thing here that gates. Every number
    above it is report-only and is meant to be read rather than met, but
    an element nothing mentions at all is covered by nothing at all, and
    a test in internal/cmd/coverage fails while one exists — which is
    what this report did not have when the count went from 0 to 8 across
    seven merges and nothing said so (#3990).
`
