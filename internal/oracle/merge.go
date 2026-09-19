// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"fmt"
	"sort"
	"strings"
)

// Regenerating part of the record, and why that is a mechanism rather than a
// convenience.
//
// A regeneration writes the whole record from one live run, which means a case
// cannot be recorded until every column of the panel is reachable at once. That
// has parked measured, correct cases indefinitely: #3384 holds three collating
// element rows measured against five columns, and they cannot be committed
// because committing them means regenerating 1122 cases across seven — a sweep
// nobody may start casually and which needs a BusyBox that is not on a Mac
// without a container runtime.
//
// So "this case is measured" and "this case can be committed" became two
// different questions, and the second one depended on hardware. [Run.Merge] is
// what makes them one question again: run the named cases, lay them over the
// record for everything else, and write the result.
//
// # Why this is not a second way to write the record
//
// It produces a [Run] and stops. [Run.Record] still owns the ordering, the
// racing rows, the unmeasured markers and the refusal to lose a column, and it
// is handed the whole corpus exactly as a full regeneration hands it. A second
// path that wrote the record itself is the drift this repository has paid for
// more than once; there is one writer and this runs in front of it.
//
// # The two ways a partial regeneration can lie, and what refuses each
//
// Both are the same shape — the record comes out clean and a measurement is
// quietly gone — which is the failure [Run.LostMeasurements] exists for one
// level up, and it is why these are refusals rather than warnings.
//
//  1. **A named case with a cell nothing measured.** LostMeasurements cannot
//     catch this one: it asks what prev held, and for a case prev never had it
//     held nothing, so a brand new row would be written with a column simply
//     absent and nothing would look wrong. A partial run is also the most
//     likely place for it, because reaching six columns out of seven is
//     exactly the state a developer's machine is usually in.
//
//  2. **A panel that is not the record's panel.** Every unnamed case keeps
//     prev's cells, so a run that reached fewer shells would write a record
//     whose named rows have six columns and whose other 1119 have seven — and
//     a build that moved is worse, because the row would be *measured* and the
//     record would carry two builds of one shell with nothing saying so. A
//     whole regeneration cannot produce either state, so neither may this.
//
// Both refusals are checked before anything is merged, so a refused merge
// leaves the record exactly as it was.

// Merge lays this run's cells for the named cases over prev's record for
// every other case, and returns the result.
//
// ids are case IDs, and every one of them has to be in this run: a partial
// regeneration that silently did nothing for a case somebody named is the
// "measured and then not written down" failure, which reads identically to
// success.
func (r *Run) Merge(prev *Run, ids []string) (*Run, error) {
	if prev == nil {
		return nil, fmt.Errorf("a partial regeneration needs a record to lay the named cases over, and there is none")
	}
	if err := samePanel(r, prev); err != nil {
		return nil, err
	}
	named := make(map[string]bool, len(ids))
	for _, id := range ids {
		named[id] = true
	}
	if err := r.measuredEverywhere(ids); err != nil {
		return nil, err
	}

	out := &Run{
		Shells:  append([]ShellRecord(nil), r.Shells...),
		Missing: append([]string(nil), r.Missing...),
		Absent:  r.Absent,
		Results: make(map[string]map[string]Result, len(prev.Results)+len(ids)),
	}
	for id, row := range prev.Results {
		if named[id] {
			continue
		}
		copied := make(map[string]Result, len(row))
		for sh, res := range row {
			copied[sh] = res
		}
		out.Results[id] = copied
	}
	for id := range named {
		row := r.Results[id]
		copied := make(map[string]Result, len(row))
		for sh, res := range row {
			copied[sh] = res
		}
		out.Results[id] = copied
	}
	if err := keptEveryCase(prev, out); err != nil {
		return nil, err
	}
	return out, nil
}

// keptEveryCase is a post-condition rather than a check on an input, and it is
// here because **nothing downstream can see a row that went missing.**
//
// Measured by breaking this function's caller so that it dropped one unnamed
// case: the regeneration ran, `LostMeasurements` reported nothing, the change
// report said "no cell changed: the panel behaves as the committed record
// says", and the record came out 51 lines shorter. Both of those guards walk
// *this* run's results, so a case that is not in them is not a cell they
// compare — they are built to catch a column that stopped answering, and a
// missing row answers nothing at all.
//
// Cheap, and the only thing standing between a future edit here and a record
// that loses a case silently. `corpus-guard` catches the corpus losing a case;
// this is the same hazard one file over.
func keptEveryCase(prev, out *Run) error {
	var gone []string
	for id := range prev.Results {
		if _, kept := out.Results[id]; !kept {
			gone = append(gone, id)
		}
	}
	if len(gone) == 0 {
		return nil
	}
	sort.Strings(gone)
	return fmt.Errorf("the merge lost %d case(s) the record holds, which nothing downstream would notice: %s",
		len(gone), strings.Join(gone, ", "))
}

// measuredEverywhere refuses a named case the run did not fully measure.
//
// "Fully" is against the panel this run had, which samePanel has already
// established is the record's panel — so a cell missing here is a cell that
// would be written absent, and a cell marked Unmeasured is the harness's own
// complaint standing where a shell's answer belongs.
func (r *Run) measuredEverywhere(ids []string) error {
	var missing []string
	for _, id := range ids {
		row, ran := r.Results[id]
		if !ran {
			missing = append(missing, id+": this run has no result for it at all")
			continue
		}
		for _, s := range r.Shells {
			res, ok := row[s.Name]
			switch {
			case !ok:
				missing = append(missing, id+" ["+s.Name+"]: no cell")
			case res.Unmeasured:
				missing = append(missing, id+" ["+s.Name+"]: "+
					strings.TrimPrefix(res.Stderr, "harness error: "))
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("a partial regeneration may not write a named case with a column missing from it, "+
		"because the record would come out clean with the measurement simply gone:\n  %s",
		strings.Join(missing, "\n  "))
}

// samePanel refuses a merge between runs that did not see the same shells, or
// saw different builds of one.
//
// The record holds one build string per shell for the whole file, so a merge
// across two panels writes a record whose rows were measured under conditions
// the panel table cannot express. A whole regeneration has no such state to
// get into — it writes every row from one panel — and the point of this
// mechanism is that a partial record is indistinguishable from a whole one.
func samePanel(now, prev *Run) error {
	was := make(map[string]string, len(prev.Shells))
	for _, s := range prev.Shells {
		was[s.Name] = s.Version
	}
	is := make(map[string]string, len(now.Shells))
	for _, s := range now.Shells {
		is[s.Name] = s.Version
	}
	var why []string
	for _, s := range prev.Shells {
		v, here := is[s.Name]
		switch {
		case !here:
			why = append(why, fmt.Sprintf("%s is in the record and this run did not reach it", s.Name))
		case v != s.Version:
			why = append(why, fmt.Sprintf("%s is %q in the record and %q here", s.Name, s.Version, v))
		}
	}
	for _, s := range now.Shells {
		if _, inRecord := was[s.Name]; !inRecord {
			why = append(why, fmt.Sprintf("%s is here and not in the record", s.Name))
		}
	}
	if len(why) == 0 {
		return nil
	}
	sort.Strings(why)
	return fmt.Errorf("a partial regeneration has to run the record's own panel, and this one did not:\n  %s\n"+
		"regenerate the whole record instead, since a record whose rows were measured "+
		"under two panels cannot say so in its panel table",
		strings.Join(why, "\n  "))
}

// Select returns the cases whose IDs were named, and refuses a name the
// corpus does not have.
//
// The refusal is the useful half. A typo that narrowed the run to nothing
// would regenerate a record identical to the one on disk, exit 0, and read as
// "there was nothing to change" — which is the same silence this whole
// mechanism is arranged to make impossible.
func Select(cases []Case, ids []string) ([]Case, error) {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var out []Case
	for _, c := range cases {
		if want[c.ID] {
			out = append(out, c)
			delete(want, c.ID)
		}
	}
	if len(want) > 0 {
		unknown := make([]string, 0, len(want))
		for id := range want {
			unknown = append(unknown, id)
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf("no case in the corpus is named %s", strings.Join(unknown, ", "))
	}
	return out, nil
}

// IDs is the case IDs, in corpus order.
func IDs(cases []Case) []string {
	out := make([]string, len(cases))
	for i, c := range cases {
		out[i] = c.ID
	}
	return out
}
