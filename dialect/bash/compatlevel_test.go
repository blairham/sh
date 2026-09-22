// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `unset a[@]` empties an indexed array and leaves it declared, and under a
// compatibility level of 5.1 or below it removes the **variable** instead.
//
// Measured 2026-09-22 against `/opt/homebrew/bin/bash` 5.3.20 under `LC_ALL=C`
// from a script file, which is the shape #4256 recorded. The threshold is
// exactly 51: every level 31 through 51 removes, 52 and above empty, and so
// do an unset parameter and one whose value is out of range.
//
// `declare -p` is the question bash's own case asks — a name that is gone is
// reported missing at status 1, one that is merely empty prints at 0.
func TestTheCompatibilityLevelChoosesWhatAWholeArrayUnsetTakes(t *testing.T) {
	dir := t.TempDir()
	const tail = `; unset a[@]; if declare -p a >/dev/null 2>&1; then echo KEPT; else echo GONE; fi`
	for _, c := range []struct{ name, set, want string }{
		{"no level at all", ``, "KEPT"},
		{"the level the reading changed in", `BASH_COMPAT=51;`, "GONE"},
		{"one above it", `BASH_COMPAT=52;`, "KEPT"},
		{"one below it", `BASH_COMPAT=50;`, "GONE"},
		{"the oldest level", `BASH_COMPAT=31;`, "GONE"},
		// The dot is legibility and nothing more: one level, two spellings.
		{"spelled with a dot", `BASH_COMPAT=5.1;`, "GONE"},
		{"a dotted level above it", `BASH_COMPAT=5.2;`, "KEPT"},
		// Out of range takes the modern reading, which is bash's answer too:
		// it complains and carries on. The complaint itself is #4262.
		{"not a number", `BASH_COMPAT=abc;`, "KEPT"},
		{"below the floor", `BASH_COMPAT=0;`, "KEPT"},
		{"past the ceiling", `BASH_COMPAT=99;`, "KEPT"},
		{"emptied", `BASH_COMPAT=51; BASH_COMPAT=;`, "KEPT"},
		// It is an ordinary variable, so it takes effect and is taken back
		// mid-run rather than being settled at startup.
		{"unset again", `BASH_COMPAT=51; unset BASH_COMPAT;`, "KEPT"},
		{"raised mid-run", `BASH_COMPAT=51; BASH_COMPAT=53;`, "KEPT"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, dir, c.set+`a=(1 2 3)`+tail)
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("%s: got %q, want %q", c.set, got, c.want)
			}
		})
	}
}

// The older reading is the plain spelling's, not a removal of its own: at this
// level `unset a[@]` *is* `unset a`, so the two have to leave the same shell
// behind whatever the name was holding. Asserted as the pair, which is what a
// second implementation of the removal would break — and a second removal is
// exactly what would miss the references and disciplines the plain spelling
// takes with it.
//
// A scalar is deliberately not a row here: the span spelling refuses it at
// *both* readings — `unset: a: not an array variable` at status 1, measured —
// where the plain spelling removes it, so the level changes what happens to
// an array and nothing about what counts as one.
func TestAtTheOlderLevelTheSpanAndThePlainUnsetAgree(t *testing.T) {
	dir := t.TempDir()
	const tail = `; if declare -p a >/dev/null 2>&1; then echo KEPT; else echo GONE; fi`
	for _, decl := range []string{
		`a=(1 2 3)`,
		`a=()`,
		`declare -A a=([k]=1)`,
	} {
		span, ss := runBash(t, dir, `BASH_COMPAT=51; `+decl+`; unset a[@]`+tail)
		plain, ps := runBash(t, dir, `BASH_COMPAT=51; `+decl+`; unset a`+tail)
		if span != plain || ss != ps {
			t.Errorf("%s: `unset a[@]` left %q/%d and `unset a` left %q/%d",
				decl, span, ss, plain, ps)
		}
	}
}

// The rows where the older level changes nothing, because the span spelling
// refuses them at both readings. Measured 2026-09-22 on bash 5.3.20 with
// `BASH_COMPAT=51`: what the level decides is what happens to an *array*, and
// a name that is not one is refused exactly as it is at the modern reading.
func TestTheOlderLevelStillRefusesWhatIsNotAnArray(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, decl, want string }{
		// A scalar: complained about, status 1, and kept.
		{"a scalar", `a=scalar`, "unset: a: not an array variable"},
		// Through a reference, the complaint names the **target** — which is
		// the scalar — and not the reference the script wrote.
		{"a reference to a scalar", `declare -n a=target; target=1`, "unset: target: not an array variable"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, dir, `BASH_COMPAT=51; `+c.decl+`; unset a[@]; echo "st=$?"`)
			if !strings.Contains(out, c.want) {
				t.Errorf("%s: output %q, want it to contain %q", c.decl, out, c.want)
			}
			if !strings.Contains(out, "st=1") {
				t.Errorf("%s: output %q, want status 1", c.decl, out)
			}
			_ = st
		})
	}
}
