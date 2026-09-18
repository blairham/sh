// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

// What our own suite's directories can say, what they cannot, and whether an
// axis is holding the difference instead.
//
// # The question
//
// `make suite` grades a tier by asking the reference shells themselves to
// agree or to differ, and each directory is one claim about how they came
// out. `core/` says all four wrote the same bytes. A dialect tier says this
// is the answer only that shell has, which suite.OnlyHere grades as *its
// group is a singleton* — it does not ask what the other three did.
//
// Four references partition fifteen ways, and those two shapes reach twelve
// of them: the one where nobody disagrees, and the eleven in which at least
// one shell stands alone. The remaining three are the even splits — two
// shells against two — and **no directory can express one**. A file that
// lands in `core/` fails the cross-check, and a file that lands in a dialect
// tier fails only-here, so the disagreement has nowhere to be filed and
// nothing measures it.
//
// A fifth shape has the same problem for a different reason. BusyBox ash is
// reached inside a container, so it is in no cross-check at all and its tier
// has no only-here check either (both are printed by `make suite` as
// omissions). An answer ash alone gives, where the four agree, is therefore
// invisible to every directory — which is how #2605 was found, by hand.
//
// # What holds them instead
//
// A Semantics axis. Where the tiers cannot express a disagreement, the
// disagreement is supposed to be a named field whose values across the
// presets *are* that partition, with a probe reading it out of the record.
// That is leg 4 of #2291, and until #3482 nothing counted it, so nobody could
// say whether it was met.
//
// This counts it. A probed axis holds a shape when the values the record
// gives its dialects partition them exactly that way — so the claim is made
// of the same readings `graded.txt` is made of, and a probe that stops
// reading, or a cell that moves, changes the count.

// CrossReferences are the dialects whose references a tier cross-check can
// compare: the four whose reference shell is on the machine beside ours.
// ash is absent for the reason above, and is the subject of [AshAlone].
var CrossReferences = []string{"bash", "dash", "ksh", "zsh"}

// Split is one way the cross-checked references can come out: a partition of
// [CrossReferences] into the groups that wrote the same answer.
type Split struct {
	// Groups are the blocks, each sorted, and the whole sorted by first
	// member — so one partition has one spelling and two Splits compare as
	// values.
	Groups [][]string
}

// String is the shape as `make suite` already prints a cross-check failure:
// the groups joined by the sign that says they differ.
func (s Split) String() string {
	out := make([]string, len(s.Groups))
	for i, g := range s.Groups {
		out[i] = strings.Join(g, "+")
	}
	return strings.Join(out, " ≠ ")
}

// Tiers are the suite directories that can express this shape.
//
// `core/` for the one where nothing disagrees, and a dialect's own tier for
// every shape in which that dialect stands alone — which is what OnlyHere
// grades. Empty is the answer this file exists to count.
//
// `ext/` is deliberately not here. It is the same claim as `core/` over a
// different and smaller reference set, so a shape over these four is not
// something it can be asked about.
func (s Split) Tiers() []string {
	if len(s.Groups) == 1 {
		return []string{"core"}
	}
	var out []string
	for _, g := range s.Groups {
		// A singleton that is not one of the cross-checked references does
		// not get a tier from standing alone. That is ash, and it is the
		// whole of [AshAlone]: its column is reached in a container, so
		// `make suite` prints an omission for the cross-check *and* one for
		// only-here, and neither half is run.
		if len(g) == 1 && slices.Contains(CrossReferences, g[0]) {
			out = append(out, g[0])
		}
	}
	return out
}

// Splits enumerates every way the cross-checked references can come out.
//
// Every partition rather than the ones seen so far: the question is which
// shapes have a home, and a shape nobody has met yet has exactly the same
// answer as one met yesterday. Four references partition fifteen ways.
func Splits() []Split {
	return partitions(CrossReferences)
}

// AshAlone is the fifth shape: ash answering differently from four references
// that agree with each other.
//
// Not one of [Splits], because ash is not one of the cross-checked
// references — it is reached in a container, where comparing its bytes
// against shells on this machine would score an operating system as a shell.
// It is here because it has the same property and for a stronger reason:
// `make suite` prints *two* omissions for this column, so neither the
// cross-check nor only-here can see this shape at all.
func AshAlone() Split {
	return Split{Groups: [][]string{{"ash"}, slices.Clone(CrossReferences)}}
}

// SplitHold is one shape, what can express it, and what does.
type SplitHold struct {
	Split Split
	// Tiers are the suite directories that can express the shape, empty when
	// none can.
	Tiers []string
	// Fields are the probed axes whose recorded values partition the
	// dialects exactly this way.
	Fields []string
}

// Held reports whether anything at all would notice this shape.
func (h SplitHold) Held() bool { return len(h.Tiers) > 0 || len(h.Fields) > 0 }

// SplitHolds classifies every shape a tier cannot express against the probes.
//
// Only those shapes. A shape with a tier is already measured by `make suite`
// on every run, and reporting it here beside the ones that are not would bury
// the number this exists to print.
func (g *GradeResult) SplitHolds() []SplitHold {
	byField := map[string]map[string]string{}
	for _, j := range g.Judgments {
		if j.Recorded == "" {
			// The record is silent for this dialect, so this axis cannot
			// partition anything — it has a hole where a group would be.
			continue
		}
		if byField[j.Field] == nil {
			byField[j.Field] = map[string]string{}
		}
		byField[j.Field][j.Dialect] = j.Recorded
	}

	var out []SplitHold
	for _, s := range append(Splits(), AshAlone()) {
		if len(s.Tiers()) > 0 {
			continue
		}
		h := SplitHold{Split: s}
		for _, field := range slices.Sorted(maps.Keys(byField)) {
			if inducedBy(byField[field], s) {
				h.Fields = append(h.Fields, field)
			}
		}
		out = append(out, h)
	}
	return out
}

// UnheldSplits are the shapes no tier expresses and no probe reads: a
// disagreement this project has no instrument for.
func (g *GradeResult) UnheldSplits() []SplitHold {
	var out []SplitHold
	for _, h := range g.SplitHolds() {
		if !h.Held() {
			out = append(out, h)
		}
	}
	return out
}

// inducedBy reports whether the recorded values partition the shape's members
// exactly the way the shape does.
//
// Every member has to be read. A dialect the record is silent about would
// otherwise let a two-group axis stand in for a three-group shape by being
// absent from it, which is the same blind spot one level up.
func inducedBy(values map[string]string, s Split) bool {
	for _, g := range s.Groups {
		for _, d := range g {
			if _, ok := values[d]; !ok {
				return false
			}
		}
	}
	members := []string{}
	for _, g := range s.Groups {
		members = append(members, g...)
	}
	by := map[string][]string{}
	for _, d := range members {
		by[values[d]] = append(by[values[d]], d)
	}
	return splitOf(by).String() == s.String()
}

// splitOf turns groups keyed by whatever they had in common into a Split.
func splitOf(by map[string][]string) Split {
	var groups [][]string
	for _, g := range by {
		sort.Strings(g)
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i][0] < groups[j][0] })
	return Split{Groups: groups}
}

// partitions enumerates every partition of a set of names.
func partitions(names []string) []Split {
	if len(names) == 0 {
		return []Split{{}}
	}
	first, rest := names[0], names[1:]
	var out []Split
	for _, p := range partitions(rest) {
		// The first name joins each existing block in turn, and then stands
		// alone — which is every partition of the whole set, once each.
		for i := range p.Groups {
			groups := make([][]string, len(p.Groups))
			for j, g := range p.Groups {
				groups[j] = slices.Clone(g)
			}
			groups[i] = append(groups[i], first)
			out = append(out, splitOf(keyed(groups)))
		}
		groups := make([][]string, 0, len(p.Groups)+1)
		for _, g := range p.Groups {
			groups = append(groups, slices.Clone(g))
		}
		out = append(out, splitOf(keyed(append(groups, []string{first}))))
	}
	return out
}

// keyed gives each group a key of its own so splitOf can normalize it.
func keyed(groups [][]string) map[string][]string {
	by := make(map[string][]string, len(groups))
	for i, g := range groups {
		by[fmt.Sprint(i)] = g
	}
	return by
}

// SplitReport is the roll-up: the shapes no directory can express, and what
// holds each of them instead.
func (g *GradeResult) SplitReport() string {
	holds := g.SplitHolds()
	unheld := g.UnheldSplits()
	var even int
	for _, s := range Splits() {
		if len(s.Tiers()) == 0 {
			even++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d of the %d ways the cross-checked references can come out have no\n"+
		"  directory that can express them, and ash against a panel that agrees has\n"+
		"  none either — %d shapes in all. %d of those are held by no probe: a\n"+
		"  disagreement of that shape would be measured by nothing at all.\n",
		even, len(Splits()), len(holds), len(unheld))
	for _, h := range holds {
		if len(h.Fields) == 0 {
			fmt.Fprintf(&b, "  %-26s NO PROBE — nothing would notice this\n", h.Split)
			continue
		}
		fmt.Fprintf(&b, "  %-26s %d axis/axes: %s\n", h.Split, len(h.Fields), strings.Join(h.Fields, ", "))
	}
	return b.String()
}
