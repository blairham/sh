// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"testing"
)

// nextJobNumberByMap is the numbering as it was written before it stopped
// building a map per command (#5873). The reference the scan has to agree
// with.
func (r *Runner) nextJobNumberByMap() int {
	high, taken := 0, make(map[int]bool, len(r.jobs)+1)
	for _, j := range r.jobs {
		taken[j.num] = true
		if j.num > high {
			high = j.num
		}
	}
	for _, n := range append([]int{r.commandSlot}, r.outerSlots...) {
		if n != 0 {
			taken[n] = true
			high = max(high, n)
		}
	}
	floor := 1
	if r.ownJobsStartAtTwo {
		floor = 2
	}
	lowest := floor
	for taken[lowest] {
		lowest++
	}
	if high < floor-1 {
		high = floor - 1
	}
	if lowest == high+1 {
		return lowest
	}
	if r.ask(r.sem().NextJobNumberRefillsAHole, "the number a job takes when the table has a hole in it") {
		return lowest
	}
	return high + 1
}

// TestNextJobNumberAgreesWithTheMap walks tables with and without holes,
// with the running commands' slots inside, outside and on top of them, from
// both floors, under both answers to the hole question.
func TestNextJobNumberAgreesWithTheMap(t *testing.T) {
	tables := [][]int{{}, {1}, {2}, {1, 2}, {1, 3}, {3, 1}, {2, 3, 5}, {1, 2, 3, 4}, {4}}
	slots := []int{0, 1, 2, 4, 6}
	outers := [][]int{nil, {1}, {2}, {3, 1}, {5}}
	for _, refills := range []Answer{Yes, No} {
		for _, two := range []bool{false, true} {
			for _, table := range tables {
				for _, slot := range slots {
					for _, outer := range outers {
						sem := PosixSemantics()
						sem.NextJobNumberRefillsAHole = refills
						r := newTestRunner(t, &Runner{Semantics: &sem})
						for _, n := range table {
							r.jobs = append(r.jobs, &Job{num: n})
						}
						r.commandSlot, r.outerSlots, r.ownJobsStartAtTwo = slot, outer, two
						got, want := r.nextJobNumber(), r.nextJobNumberByMap()
						if got != want {
							t.Errorf("%s", fmt.Sprintf("refills %v, floor two %v, jobs %v, slot %d, outer %v: scan %d, map %d",
								refills, two, table, slot, outer, got, want))
						}
					}
				}
			}
		}
	}
}

// TestNumberingAJobAllocatesNothing pins the cost, which is paid per command
// in a shell that has a job in its table.
func TestNumberingAJobAllocatesNothing(t *testing.T) {
	sem := PosixSemantics()
	sem.NextJobNumberRefillsAHole = Yes
	r := newTestRunner(t, &Runner{Semantics: &sem})
	r.jobs = []*Job{{num: 1}, {num: 3}}
	r.commandSlot, r.outerSlots = 4, []int{2}
	if n := testing.AllocsPerRun(100, func() { r.nextJobNumber() }); n > 0 {
		t.Errorf("numbering a job allocated %v times, want none", n)
	}
}
