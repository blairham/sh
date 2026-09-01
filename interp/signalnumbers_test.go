// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"testing"
)

// Every signal this shell knows has both spellings. The map used to be seven
// entries written out by hand, which is a claim about which signals scripts
// bother to name — and /usr/bin/bzless, which traps 0 2 3 5 10 13 15, is the
// counterexample sitting on the machine.
//
// This is in-package on purpose: the point is that the two tables cannot drift
// apart, and that is a fact about the tables rather than about anything the
// shell prints.
func TestEverySignalHasBothSpellings(t *testing.T) {
	for _, k := range knownSignals {
		n := strconv.Itoa(int(k.Sig))
		got, ok := signalNumbers[n]
		if !ok {
			t.Errorf("%s is %s, and %s names nothing", k.Name, n, n)
			continue
		}
		// A number two constants share resolves to whichever the list names
		// first, so the round trip is checked by number rather than by name.
		if sig, found := trappableSignals[got]; found && int(sig) != int(k.Sig) {
			t.Errorf("%s resolves to %s, which is %d and not %d", n, got, sig, k.Sig)
		}
	}
}

// And nothing else does. A number the host has no signal for is not a signal,
// which is what keeps `trap ” 99` a refusal rather than a silent acceptance.
func TestNoNumberNamesASignalTheHostDoesNotHave(t *testing.T) {
	known := map[string]bool{}
	for _, k := range knownSignals {
		known[strconv.Itoa(int(k.Sig))] = true
	}
	for n := range signalNumbers {
		if !known[n] {
			t.Errorf("%s names %s, which is not one of the host's signals", n, signalNumbers[n])
		}
	}
	for _, n := range []string{"0", "99", "-1", "1000", ""} {
		if name, ok := signalNumbers[n]; ok {
			t.Errorf("%q names %s, want nothing — 0 is EXIT and is handled before the table", n, name)
		}
	}
}
