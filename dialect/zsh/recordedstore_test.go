// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// The recorded options are bits on the runner, so asking whether one has
// moved is a mask and saving them is a copy. This is a performance property
// written as a structural test, because it is invisible in behavior and
// expensive in fact: the question is asked by the save on the way into every
// function, by every `emulate -L zsh` and by the restore on the way out, and
// on the maintainer's real configuration (2026-10-05) one interactive start
// asked it 423,491 times. Kept in the variable store, each answer found a
// variable and then a key, and each save copied the table (#6019, #6100).
//
// So the row asserts the shape — the state is in interp.Runner.DialectOptions
// and not in any variable — and the behavior beside it: a name set and a name
// cleared read back through recordedDeviates.
func TestTheRecordedOptionsAreBitsOnTheRunner(t *testing.T) {
	r := optionStateRunner(t)
	setRecordedDeviation(r, "autocd", true)
	setRecordedDeviation(r, "banghist", true)
	setRecordedDeviation(r, "banghist", false)
	if !r.DialectOptions.Has(zshOptionIndex["autocd"]) || r.DialectOptions.Has(zshOptionIndex["banghist"]) {
		t.Errorf("the runner's bits: autocd %v, banghist %v; want true, false",
			r.DialectOptions.Has(zshOptionIndex["autocd"]), r.DialectOptions.Has(zshOptionIndex["banghist"]))
	}
	if _, ok := r.GetAssoc(".zsh.setopt"); ok {
		t.Error("the recorded options are in the variable store again")
	}
	if !recordedDeviates(r, "autocd") || recordedDeviates(r, "banghist") {
		t.Errorf("recordedDeviates: autocd %v, banghist %v; want true, false",
			recordedDeviates(r, "autocd"), recordedDeviates(r, "banghist"))
	}
	// And the wholesale write an emulation makes, which reads back sorted.
	setRecordedOptions(r, []string{"nomatch", "autocd"})
	if got := recordedNames(r); !slices.Equal(got, []string{"autocd", "nomatch"}) {
		t.Errorf("recordedNames = %q, want [autocd nomatch]", got)
	}
}

// The bits are numbered by the table, so the table must fit in them, and the
// listing order recordedNames promises is the table's own order.
func TestTheOptionTableFitsTheRunnersBits(t *testing.T) {
	if len(zshOptions) > interp.DialectOptionCapacity {
		t.Fatalf("%d options, room for %d", len(zshOptions), interp.DialectOptionCapacity)
	}
	if !slices.IsSortedFunc(zshOptions, func(a, b zshOption) int { return strings.Compare(a.base, b.base) }) {
		t.Error("zshOptions is not sorted by base, so recordedNames is not sorted either")
	}
}

// A function's own `setopt` under `localoptions` comes back off on return,
// and a recorded name set before the call is still set after it: the save and
// the restore carry the store across the call as a copy.
func TestARecordedOptionSurvivesALocalOptionsCall(t *testing.T) {
	r := optionStateRunner(t)
	setRecordedDeviation(r, "autocd", true)
	saved := saveOptionState(r)
	applyEmulation(r, "zsh", false)
	setLocalOptions(r, true)
	setRecordedDeviation(r, "cdablevars", true)
	if recordedDeviates(r, "autocd") {
		t.Error("emulate zsh left autocd standing")
	}
	saved.restoreIfLocal(r)
	if !recordedDeviates(r, "autocd") || recordedDeviates(r, "cdablevars") {
		t.Errorf("after the return: autocd %v, cdablevars %v; want true, false",
			recordedDeviates(r, "autocd"), recordedDeviates(r, "cdablevars"))
	}
}
