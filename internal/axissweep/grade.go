// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/interp"
)

// This file grades a **preset against the golden record**, which is the third
// leg of a triangle whose other two were already here (#2441).
//
//	axis coverage (coverage.go)   does every dialect *answer* every axis?
//	the oracle panel              does the *record* match the real shells?
//	this                          does a *preset* match the record?
//
// Completeness cannot see a wrong answer and the panel cannot see a preset:
// `make check` grades the record against the shells and never grades a vector
// against the record, so a preset that contradicts a measured row it has no
// corpus case for is invisible. Nothing fails, nothing drifts, and the wrong
// value ships.
//
// That is not hypothetical either. dialect/ash answered
// QuitIgnoredWhenNotInteractive with dash's value for the whole life of the
// ash column while the record had BusyBox's answer right the entire time, and
// the first run of this instrument found a second one: ash inherited
// ShiftPastEndFatal from PosixSemantics and BusyBox ash prints `survived`.
//
// # Why a probe, and why not the flip sweep
//
// The flip sweep can only speak about a value the corpus *exercises*, and it
// grades from the passing set: a preset that is already wrong fails its rows
// at the baseline, where a conformance failure is indistinguishable from an
// unimplemented feature. It also costs thousands of processes, so it cannot
// be a gate.
//
// A probe goes the other way round and costs nothing. The record already
// holds what each panel shell did with a given snippet; a probe says how to
// *read an axis's value off* one of those cells, and the check compares that
// reading with what the dialect's vector holds. No shell is run, so this sits
// in `make check` beside the coverage gate rather than needing colima.
//
// # Why the blind spot is printed rather than implied
//
// A probe has to be written, so most axes have none, and a pair nothing
// compares is exactly the state this issue is about. So "not graded" is a
// reported outcome with a count beside it, never an absence: the ledger in
// testdata/graded.txt lists every probed pair including the ones the record
// cannot answer, and the report prints how many of the dialect/axis pairs
// have no probe at all. That number is the honest measure of what is still
// invisible, and it is meant to be read as such rather than mistaken for
// agreement — the inert-reads-as-pass shape this tree has been bitten by more
// than once.

// Probe is how one axis's value is read off the golden record.
//
// It names the corpus rows whose recorded cells show the axis, and carries
// the reading itself as code. Read is handed one panel column's cells — the
// results that column recorded for each of Cases — and answers with the
// constant name the axis must hold for a shell that behaved that way, or with
// a sentence saying why those cells cannot say.
//
// The reading is deliberately per *axis* and not per dialect. Every dialect
// with a panel column is graded by every probe, so a dialect added tomorrow
// is graded by all of them on the commit that adds it — there is no list of
// dialects here to forget to extend. Presets is that list, and it is asserted
// against the packages on disk by TestPresetsAreEveryDialectPackage.
type Probe struct {
	// Field is the axis, as a path into interp.Semantics.
	Field string
	// Cases are the corpus rows the reading looks at. Usually one; a second
	// appears where a cell only means something given what another row says,
	// which is how a row that asks nothing of a shell is told apart from one
	// that answers.
	Cases []string
	// Reading is what the cells are taken to show, in one sentence, printed
	// beside the verdict so a reader can disagree with it.
	Reading string
	// Read answers with a constant name of the axis's type, or with a reason
	// the record is silent. Exactly one of the two is returned; both empty,
	// or both set, is an instrument fault and is reported as one.
	Read func(cells map[string]oracle.Result) (value, silent string)
}

// Verdict is what grading one dialect/axis pair came to. The flip sweep's
// Outcome is a different question asked by a different instrument, so the two
// enumerations stay apart.
type Verdict string

// The four outcomes. Only Disagrees is a failure; the other three are
// reported, and two of them are the blind spot being counted rather than
// hidden.
const (
	// Agrees means the preset holds what the record shows.
	Agrees Verdict = "agrees"
	// Disagrees means it holds something else. This is the bug #2441 is
	// about, and the only outcome that fails.
	Disagrees Verdict = "disagrees"
	// NoEvidence means the record has no reading for this shell: the
	// construct does not exist there, or the row asks it nothing.
	NoEvidence Verdict = "no evidence"
	// Unanswered means the dialect leaves the axis unspecified, so there is
	// nothing to compare. What the record shows is printed anyway, because a
	// record that answers an axis a dialect declines to is worth a look —
	// see coverage.go for the ledger that governs those.
	Unanswered Verdict = "unanswered"
)

// Judgment is one dialect, one axis, and what came of comparing them.
type Judgment struct {
	Dialect string  `json:"dialect"`
	Against string  `json:"against"`
	Field   string  `json:"field"`
	Outcome Verdict `json:"outcome"`
	// Held is the constant the preset holds, and Recorded the one the record
	// shows. Recorded is empty when the record is silent.
	Held     string `json:"held"`
	Recorded string `json:"recorded,omitempty"`
	// Why is the reason the record is silent, or the reading that produced
	// Recorded.
	Why string `json:"why,omitempty"`
	// By are the corpus rows the reading was taken from.
	By []string `json:"by,omitempty"`
}

// GradeResult is the whole comparison.
type GradeResult struct {
	Judgments []Judgment `json:"judgments"`
	// Axes is every field of the vector and Probed the ones a probe reads,
	// so the two numbers together are the size of the blind spot.
	Axes   int `json:"axes"`
	Probed int `json:"probed"`
	// Pairs is dialects times axes, and Unprobed the pairs no probe reaches.
	Pairs    int `json:"pairs"`
	Unprobed int `json:"unprobed"`
	// Faults are the instrument's own breakages: a probe naming an axis that
	// does not exist, a row that is not in the corpus or not in the record, a
	// reading that answers with a value the axis's type does not declare.
	//
	// They are failures rather than skips. A probe that quietly stopped
	// reading anything is the shape that makes a green check meaningless,
	// and this tree has shipped that shape before.
	Faults []string `json:"faults,omitempty"`
}

// goldenFile is the committed record the grading is done against.
func goldenFile() string {
	return filepath.Join(moduleRoot, "internal", "oracle", "testdata", "golden.json")
}

// GradeCommitted grades every preset against the committed corpus and record.
//
// One entry point for the test and for the command, so the gate and the
// report cannot come to answer slightly different questions — which is the
// same reason oracle.Verdict is exported rather than reimplemented.
func GradeCommitted() (*GradeResult, error) {
	golden, err := oracle.Load(goldenFile())
	if err != nil {
		return nil, fmt.Errorf("reading the golden record: %w", err)
	}
	return Grade(oracle.Corpus, golden)
}

// Grade compares every preset against the golden record, through the probes.
func Grade(cases []oracle.Case, golden *oracle.Run) (*GradeResult, error) {
	fields, err := Fields(reflect.TypeOf(interp.Semantics{}))
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]Field, len(fields))
	for _, f := range fields {
		byPath[f.Path] = f
	}
	inCorpus := make(map[string]bool, len(cases))
	for _, c := range cases {
		inCorpus[c.ID] = true
	}
	consts, err := typeConstants()
	if err != nil {
		return nil, err
	}

	presets := Presets()
	out := &GradeResult{Axes: len(fields), Pairs: len(fields) * len(presets)}
	probed := map[string]bool{}

	for _, p := range Probes() {
		f, ok := byPath[p.Field]
		if !ok {
			out.Faults = append(out.Faults, fmt.Sprintf(
				"%s: no such axis in interp.Semantics — a probe for a renamed or deleted axis reads nothing", p.Field))
			continue
		}
		if probed[p.Field] {
			out.Faults = append(out.Faults, fmt.Sprintf(
				"%s: two probes read the same axis; one of them is unread", p.Field))
			continue
		}
		probed[p.Field] = true
		bad := false
		for _, id := range p.Cases {
			if !inCorpus[id] {
				out.Faults = append(out.Faults, fmt.Sprintf(
					"%s: no corpus row named %q — the probe reads nothing", p.Field, id))
				bad = true
			} else if _, ok := golden.Results[id]; !ok {
				out.Faults = append(out.Faults, fmt.Sprintf(
					"%s: the golden record has no entry for %q; re-measure with `make oracle`", p.Field, id))
				bad = true
			}
		}
		if bad {
			continue
		}
		for _, pr := range presets {
			j, fault := judge(p, f, pr, golden, consts)
			if fault != "" {
				out.Faults = append(out.Faults, fault)
				continue
			}
			out.Judgments = append(out.Judgments, j)
		}
	}
	out.Probed = len(probed)
	out.Unprobed = out.Pairs - len(out.Judgments) - len(out.Faults)
	sort.Slice(out.Judgments, func(i, j int) bool {
		if out.Judgments[i].Dialect != out.Judgments[j].Dialect {
			return out.Judgments[i].Dialect < out.Judgments[j].Dialect
		}
		return out.Judgments[i].Field < out.Judgments[j].Field
	})
	sort.Strings(out.Faults)
	return out, nil
}

// judge grades one preset against one probe.
func judge(p Probe, f Field, pr Preset, golden *oracle.Run, consts map[string][]Value) (Judgment, string) {
	cur, err := At(reflect.ValueOf(pr.Semantics), f.Path)
	if err != nil {
		return Judgment{}, fmt.Sprintf("%s: %v", f.Path, err)
	}
	j := Judgment{
		Dialect: pr.Name, Against: pr.Against, Field: f.Path,
		Held: heldName(f, cur), By: p.Cases,
	}

	// A dialect with no panel column has nothing to be graded against, and
	// saying so is the point: it is not agreement.
	if pr.Against == "" {
		j.Outcome, j.Why = NoEvidence, "this dialect has no oracle panel column: "+pr.Ungraded
		return j, ""
	}
	cells := map[string]oracle.Result{}
	for _, id := range p.Cases {
		cell, ok := golden.Results[id][pr.Against]
		if !ok {
			j.Outcome = NoEvidence
			j.Why = fmt.Sprintf("the record has no %s column for %s — it was not reachable when the record was made", pr.Against, id)
			return j, ""
		}
		cells[id] = cell
	}

	value, silent := p.Read(cells)
	switch {
	case value != "" && silent != "":
		return Judgment{}, fmt.Sprintf("%s: the %s reading answers both %q and %q; a probe says one or the other",
			f.Path, pr.Name, value, silent)
	case value == "" && silent == "":
		return Judgment{}, fmt.Sprintf("%s: the %s reading answers nothing at all; a probe says a value or why it cannot",
			f.Path, pr.Name)
	case silent != "":
		j.Outcome, j.Why = NoEvidence, silent
		return j, ""
	}
	if !declares(consts, f, value) {
		return Judgment{}, fmt.Sprintf("%s: the %s reading answers %q, which %s does not declare",
			f.Path, pr.Name, value, f.Type)
	}
	j.Recorded, j.Why = value, p.Reading
	switch {
	case unspecifiedNow(f, cur):
		j.Outcome = Unanswered
	case j.Held == value:
		j.Outcome = Agrees
	default:
		j.Outcome = Disagrees
	}
	return j, ""
}

// declares reports whether the axis's type has a constant of that name, so a
// reading cannot answer with a word no shell could hold.
func declares(consts map[string][]Value, f Field, name string) bool {
	for _, v := range consts[f.Type] {
		if v.Name == name {
			return true
		}
	}
	// A bool axis has no constants of its own; the two names it can hold are
	// what the source writes.
	return f.Kind == reflect.Bool && (name == "true" || name == "false")
}

// Count is how many judgments came to one outcome.
func (g *GradeResult) Count(o Verdict) int {
	n := 0
	for _, j := range g.Judgments {
		if j.Outcome == o {
			n++
		}
	}
	return n
}

// Disagreements are the pairs where a preset contradicts the record. This is
// what fails.
func (g *GradeResult) Disagreements() []Judgment {
	var out []Judgment
	for _, j := range g.Judgments {
		if j.Outcome == Disagrees {
			out = append(out, j)
		}
	}
	return out
}

// Ungraded reports which dialects no probe could grade at all. A dialect
// nothing grades is the #2272 shape one level up: it reads as covered because
// the instrument exists, and nothing it holds has been compared with anything.
func (g *GradeResult) Ungraded() []string {
	graded := map[string]bool{}
	for _, j := range g.Judgments {
		if j.Outcome == Agrees || j.Outcome == Disagrees {
			graded[j.Dialect] = true
		}
	}
	var out []string
	for _, p := range Presets() {
		if !graded[p.Name] {
			out = append(out, p.Name)
		}
	}
	return out
}

// Report renders the comparison, blind spot and all.
func (g *GradeResult) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "presets graded against the golden record: %d agree, %d disagree\n",
		g.Count(Agrees), g.Count(Disagrees))
	fmt.Fprintf(&b, "  %d of the %d axes have a probe, so %d of the %d dialect/axis pairs are\n"+
		"  compared with nothing at all. That number is the blind spot, not a pass:\n"+
		"  an axis no probe reads can hold any value and no instrument here objects.\n",
		g.Probed, g.Axes, g.Unprobed, g.Pairs)
	fmt.Fprintf(&b, "  of the %d pairs a probe does reach, %d have no evidence in the record\n"+
		"  and %d sit on an axis the dialect does not answer.\n\n",
		len(g.Judgments), g.Count(NoEvidence), g.Count(Unanswered))

	if len(g.Faults) > 0 {
		fmt.Fprintf(&b, "the instrument itself is broken in %d place(s):\n", len(g.Faults))
		for _, f := range g.Faults {
			fmt.Fprintf(&b, "  %s\n", f)
		}
		b.WriteString("\n")
	}
	if d := g.Disagreements(); len(d) > 0 {
		fmt.Fprintf(&b, "%d preset(s) contradict the record:\n", len(d))
		for _, j := range d {
			fmt.Fprintf(&b, "  %s.%s holds %s; %s recorded %s in %s\n",
				j.Dialect, j.Field, j.Held, j.Against, j.Recorded, strings.Join(j.By, ", "))
			fmt.Fprintf(&b, "      the reading: %s\n", j.Why)
		}
		b.WriteString("\n")
	}
	if u := g.Ungraded(); len(u) > 0 {
		fmt.Fprintf(&b, "%d dialect(s) no probe could grade at all: %s\n\n", len(u), strings.Join(u, ", "))
	}
	b.WriteString("every probed pair:\n")
	for _, j := range g.Judgments {
		switch j.Outcome {
		case Agrees, Disagrees:
			fmt.Fprintf(&b, "  %-6s %-44s %-11s %s\n", j.Dialect, j.Field, j.Outcome, j.Recorded)
		case Unanswered:
			fmt.Fprintf(&b, "  %-6s %-44s %-11s the record reads %s\n", j.Dialect, j.Field, j.Outcome, j.Recorded)
		default:
			fmt.Fprintf(&b, "  %-6s %-44s %-11s %s\n", j.Dialect, j.Field, j.Outcome, j.Why)
		}
	}
	return b.String()
}

// gradedLedgerHeader is written into the committed ledger, so that whoever
// opens it next reads what it is before deciding to regenerate it.
const gradedLedgerHeader = `# What the golden record says each dialect's preset should hold.
#
# One line per probed dialect/axis pair: the dialect, the axis, and the value
# read off the recorded cells of the oracle panel column that dialect claims
# to be. ` + "`-` is a pair the record cannot answer — the construct does not" + `
# exist in that shell, or the row asks it nothing — and those lines are here
# on purpose. A pair nothing grades has to be visible as *not graded*, because
# the failure this whole file exists to catch is a blind spot that reads like
# agreement (#2441).
#
# Regenerating this cannot bury a bug. Every value here comes from the record
# and never from a preset, so a preset that contradicts one still fails
# TestNoPresetContradictsTheRecord however often this file is rewritten. What
# regenerating *does* record is a probe added or removed and a measured cell
# that moved, which is why the file is committed: a pair that silently stopped
# being compared would otherwise look exactly like one that agrees.
#
# Generated by ` + "`make axis-grade ARGS=-write`" + `. Sorted; do not hand-order.

`

// gradedLedgerFile is the committed reading of the record, per probed pair.
func gradedLedgerFile() string {
	return filepath.Join(moduleRoot, "internal", "axissweep", "testdata", "graded.txt")
}

// Ledger renders the committed record of what each probed pair reads.
func (g *GradeResult) Ledger() string {
	var b strings.Builder
	b.WriteString(gradedLedgerHeader)
	for _, j := range g.Judgments {
		value := j.Recorded
		if value == "" {
			value = "-"
		}
		fmt.Fprintf(&b, "%-6s %-52s %s\n", j.Dialect, j.Field, value)
	}
	return b.String()
}

// WriteLedger rewrites the committed reading.
func (g *GradeResult) WriteLedger() (string, error) {
	path := gradedLedgerFile()
	return path, os.WriteFile(path, []byte(g.Ledger()), 0o644)
}

// ReadGradedLedger reads the committed reading back.
func ReadGradedLedger() (string, error) {
	blob, err := os.ReadFile(gradedLedgerFile())
	if err != nil {
		return "", fmt.Errorf("reading the graded ledger: %w", err)
	}
	return string(blob), nil
}
