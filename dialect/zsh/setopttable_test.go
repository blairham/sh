// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// And neither is recorded any more, which is a claim about the table rather
// than about behavior — so it is read off the table.
//
// The honest split #856 established is that `recorded` means remembered and
// not acted on. An option something reads must leave that set, or the count
// in docs/spec/semantics.md is a promise the code no longer keeps.
func TestTheHistoryOptionsAreNoLongerRecordedOnly(t *testing.T) {
	for _, base := range []string{"histignorespace", "histignoredups"} {
		o, _, ok := resolveOptionName(base)
		if !ok {
			t.Fatalf("%s is not in the table at all", base)
		}
		if o.recorded {
			t.Errorf("%s is still marked recorded, but a session reads it", base)
		}
		if o.set == nil {
			t.Errorf("%s cannot be moved, so `setopt %s` would refuse", base, base)
		}
	}
	// The count the spec publishes, read from the table rather than from the
	// prose. A name moving in or out without the document following is the
	// failure this catches.
	recordedCount := 0
	for _, o := range zshOptions {
		if o.recorded {
			recordedCount++
		}
	}
	if want := 149; recordedCount != want {
		t.Errorf("%d recorded names, want %d — docs/spec/semantics.md publishes the count", recordedCount, want)
	}
}
