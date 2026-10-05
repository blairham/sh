// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"slices"
	"testing"
)

// The recorded options are kept as the keys of an association, so asking
// whether one has moved is a single lookup. This is a performance property
// written as a structural test, because it is invisible in behavior and
// expensive in fact: the question is asked by the save on the way into every
// function, by every `emulate -L zsh` and by the restore on the way out, and
// on the maintainer's real configuration (2026-10-05) one interactive start
// asked it 423,491 times. Kept as a list, each answer walked every name in the
// store; that was about a third of the cost of a function that opens with
// `emulate -L zsh`.
//
// So the row asserts the shape, and the behavior beside it: a name set and a
// name cleared read back through recordedDeviates.
func TestTheRecordedOptionsAreAnAssociation(t *testing.T) {
	r := optionStateRunner(t)
	setRecordedDeviation(r, "autocd", true)
	setRecordedDeviation(r, "banghist", true)
	setRecordedDeviation(r, "banghist", false)
	set, ok := r.GetAssoc(zshRecordedStore)
	if !ok {
		t.Fatal("the recorded store is not an association")
	}
	if _, in := set["autocd"]; !in || len(set) != 1 {
		t.Errorf("the store holds %v, want autocd alone", set)
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
