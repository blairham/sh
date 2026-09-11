// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Confirming a change before it is written down.
//
// `make oracle` records whatever the panel did on the run it happened to make,
// and on a saturated machine that can be a **resource failure recorded as a
// fact** — a record that is then wrong in a way nothing downstream can tell
// from a real measurement. It happened on 2026-09-07: a regeneration made
// while a mutation suite saturated the machine wrote
//
//	pipe/both-streams-takes-no-blank-between-its-two-bytes [ksh93]
//	  err "<script>: line 1: export COSHELL={ksh,sh,coshell}
//	       <script>: line 1: echo: unable to create namespace"  status 1
//
// which is ksh93 failing to start a coshell because the machine had no room,
// not ksh93 behaving that way. Reproduced for #1249: that cell came out the
// other way **2 runs in 40** with the machine loaded and **0 in 60** with it
// idle.
//
// The guard that caught it was somebody running `make oracle-check`
// afterwards and reading the result, and a load failure looks exactly like the
// ordinary drift a shell upgrade produces. What separates them is that **a
// flake usually does not reproduce and real drift always does**, so the cheap
// thing is to ask twice: run the cases whose cells moved a second time, and
// keep only the moves that happen again.
//
// It costs a second run of the cases that changed, which on an ordinary
// regeneration is a handful and on a quiet one is none at all.

// CellChange is one recorded cell that a run disagrees with.
type CellChange struct {
	CaseID string
	Shell  string
	Was    Result
	Now    Result
}

func (c CellChange) String() string {
	return fmt.Sprintf("%s [%s]\n    was: %s\n    now: %s",
		c.CaseID, c.Shell, describe(c.Was), describe(c.Now))
}

// CellChangesFrom lists the cells where this run disagrees with a previous record.
//
// Every cell, including the racing rows: what is wanted here is "what would
// this regeneration write down", and KeepRacingRows has already carried those
// forward by the time this is asked, so a racing row that still differs is one
// KeepRacingRows could not pin — a shell that was rebuilt, or a case the
// previous record never had.
//
// A case the record has no entry for is not a change. It is an addition, which
// NewCases already reports and which nothing can confirm against.
func (r *Run) CellChangesFrom(prev *Run) []CellChange {
	var out []CellChange
	if prev == nil {
		return nil
	}
	for id, now := range r.Results {
		was, ok := prev.Results[id]
		if !ok {
			continue
		}
		for sh, nowRes := range now {
			wasRes, ok := was[sh]
			if !ok {
				continue
			}
			if wasRes != nowRes {
				out = append(out, CellChange{CaseID: id, Shell: sh, Was: wasRes, Now: nowRes})
			}
		}
	}
	sortChanges(out)
	return out
}

func sortChanges(cs []CellChange) {
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].CaseID != cs[j].CaseID {
			return cs[i].CaseID < cs[j].CaseID
		}
		return cs[i].Shell < cs[j].Shell
	})
}

// Rerun is how Confirm asks for a second opinion. Execute has this shape.
type Rerun func(context.Context, []Case) (*Run, error)

// Confirm re-runs the cases that changed and reports which changes held.
//
// A change that comes out the same way twice is a measurement and is left in
// the run. One that does not is a coin landing, or a machine that had no room
// for a moment, and **the previous value is put back** — the run is edited so
// that what gets written down is the record that was already there rather than
// the sample that could not be repeated.
//
// Putting the old value back rather than refusing to write at all is the
// choice that matters. A regeneration exists to record the *other* cases, and
// a command that gave up over one unrepeatable cell would make the whole thing
// hostage to the busiest moment of the run.
//
// Both are returned, because "nothing changed" and "one thing changed and did
// not hold" are different states and a caller has to be able to say which.
func (r *Run) Confirm(ctx context.Context, prev *Run, cases []Case, again Rerun) (held, dropped []CellChange, err error) {
	changed := r.CellChangesFrom(prev)
	if len(changed) == 0 {
		return nil, nil, nil
	}

	affected := map[string]bool{}
	for _, c := range changed {
		affected[c.CaseID] = true
	}
	var subset []Case
	for _, c := range cases {
		if affected[c.ID] {
			subset = append(subset, c)
		}
	}
	second, err := again(ctx, subset)
	if err != nil {
		return nil, nil, fmt.Errorf("re-running the %d case(s) that changed: %w", len(subset), err)
	}

	for _, c := range changed {
		row, ok := second.Results[c.CaseID]
		if !ok {
			// The second run did not reach it — a shell that went away
			// between the two runs, or a case the rerun declined. Not
			// confirmed, so the record keeps what it had.
			dropped = append(dropped, c)
			r.Results[c.CaseID][c.Shell] = c.Was
			continue
		}
		if res, ok := row[c.Shell]; ok && res == c.Now {
			held = append(held, c)
			continue
		}
		dropped = append(dropped, c)
		r.Results[c.CaseID][c.Shell] = c.Was
	}
	sortChanges(held)
	sortChanges(dropped)
	return held, dropped, nil
}

// ChangeReport is what `make oracle` prints about the cells it rewrote.
//
// **Saying how many is the whole of it.** A regeneration that silently
// rewrites one unrelated cell is indistinguishable from one that rewrites
// none, and every agent in this repository runs this command and has had no
// way to tell a clean regeneration from a drifting one. It is the argument
// corpus-guard makes for the case *set*, applied to the case *values*.
func CellChangeReport(held, dropped []CellChange) string {
	var b strings.Builder
	switch len(held) {
	case 0:
		b.WriteString("no cell changed: the panel behaves as the committed record says.\n")
	case 1:
		b.WriteString("1 cell changed since the committed record, and held on a second run:\n\n")
	default:
		fmt.Fprintf(&b, "%d cells changed since the committed record, and held on a second run:\n\n", len(held))
	}
	for _, c := range held {
		b.WriteString("  " + c.String() + "\n")
	}
	if len(dropped) > 0 {
		fmt.Fprintf(&b, "\n%d cell(s) changed on the first run and not on the second, so the\n"+
			"committed value was kept. A measurement that will not repeat is the\n"+
			"machine and not the shell — see internal/oracle/confirm.go:\n\n", len(dropped))
		for _, c := range dropped {
			b.WriteString("  " + c.String() + "\n")
		}
	}
	return b.String()
}

// ConfirmDrift is the same question asked of `-check` rather than of a
// regeneration: run the cases that drifted again and report which drifted
// twice.
//
// The check has the same hazard as the record and one more consequence. A
// load-induced flip read as drift sends somebody to look at a shell that never
// moved, and on a machine several agents share that is most of the drift
// anybody sees — which is why agents have taken to re-running `make oracle`
// until the diff is additions only, without knowing which rows can flip under
// them.
//
// Nothing is edited here. A check writes nothing down, so all this does is
// separate what reproduces from what does not, and report both.
func ConfirmDrift(ctx context.Context, drifts []Drift, cases []Case, again Rerun) (held, dropped []Drift, err error) {
	if len(drifts) == 0 {
		return nil, nil, nil
	}
	affected := map[string]bool{}
	for _, d := range drifts {
		affected[d.CaseID] = true
	}
	var subset []Case
	for _, c := range cases {
		if affected[c.ID] {
			subset = append(subset, c)
		}
	}
	second, err := again(ctx, subset)
	if err != nil {
		return nil, nil, fmt.Errorf("re-running the %d case(s) that drifted: %w", len(subset), err)
	}
	for _, d := range drifts {
		if res, ok := second.Results[d.CaseID][d.Shell]; ok && res == d.Now {
			held = append(held, d)
			continue
		}
		dropped = append(dropped, d)
	}
	return held, dropped, nil
}
