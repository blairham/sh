// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// `${.sh.level}` before the first call is **unset** here, and `0` ever after.
//
// The `0` is a starting value rather than an off-by-one, which is what the
// second row of the measurement says: once a call has returned, ksh93u+ says
// `0` too, so the parameter is written on the way out of the first call and
// was never written before it. Measured 2026-09-18 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input from /dev/null
// (#3310).
//
// `${.sh.level-word}` and `${.sh.level+word}` are the discriminating pair: a
// bare read is empty either way, and only those two separate "no value" from
// "no parameter".
func TestTheCallLevelIsUnsetUntilTheFirstCall(t *testing.T) {
	dir := t.TempDir()
	src := strings.Join([]string{
		`printf 'top [%s] [%s] [%s]\n' "${.sh.level}" "${.sh.level-UNSET}" "${.sh.level+SET}"`,
		`function g { printf 'in g [%s]\n' "${.sh.level}"; }`,
		`g`,
		`printf 'after [%s] [%s] [%s]\n' "${.sh.level}" "${.sh.level-UNSET}" "${.sh.level+SET}"`,
	}, "\n")
	out, st := runKsh(t, dir, src)
	if st != 0 {
		t.Fatalf("status = %d, want 0 — output %q", st, out)
	}
	for _, want := range []string{
		"top [] [UNSET] []\n",
		"in g [1]\n",
		"after [0] [0] [SET]\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q line:\n%s", want, out)
		}
	}
}

// And `set -u` sees the absence, which is the half a script relies on.
func TestNounsetStopsForTheCallLevelBeforeTheFirstCall(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "set -u\nprintf '[%s]' \"${.sh.level}\"\nprintf NOT-REACHED\n")
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("the read should have stopped the script:\n%s", out)
	}
	if !strings.Contains(out, ".sh.level") {
		t.Errorf("the refusal should name the parameter:\n%s", out)
	}
}

// A function this shell has been *inside* is what makes the parameter appear,
// and not merely one it has defined — which is the control that says the
// predicate reads the run rather than the table.
func TestDefiningAFunctionDoesNotBringTheCallLevelIntoBeing(t *testing.T) {
	dir := t.TempDir()
	out, st := runKsh(t, dir, "function g { :; }\nprintf '[%s]' \"${.sh.level-UNSET}\"\n")
	if st != 0 || !strings.Contains(out, "[UNSET]") {
		t.Errorf("got %q/%d, want a definition alone to leave it unset", out, st)
	}
}
