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

// Report renders the columns. list caps how many unasked names are printed
// per kind; 0 prints them all.
func Report(cols []Column, list int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  coverage — what the cases never mention\n\n")
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
	b.WriteString(rollup(cols, list))
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
func rollup(cols []Column, list int) string {
	if len(cols) == 0 {
		return ""
	}
	mentioned := map[Element]bool{}
	union := map[Element]bool{}
	for _, c := range cols {
		for _, e := range c.Surface {
			union[e] = true
			if c.Mentions[e] > 0 {
				mentioned[e] = true
			}
		}
	}
	var never []Element
	for e := range union {
		if !mentioned[e] {
			never = append(never, e)
		}
	}
	byKindThenName(never)

	var b strings.Builder
	fmt.Fprintf(&b, "  no dialect mentions these at all — %d of %d\n\n", len(never), len(union))
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
	b.WriteString("\n")
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

    Report only. Nothing here gates anything.
`
