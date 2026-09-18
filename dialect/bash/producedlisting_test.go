// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"regexp"
	"testing"
)

// A produced parameter lists back with its value, and with the letters this
// shell writes for it (#2451). Measured 2026-09-12, `env -i
// PATH=/usr/bin:/bin` with a scratch HOME, over a script file, bash 5.3.15:
//
//	declare -p RANDOM         declare -i RANDOM="16735"
//	declare -p SECONDS        declare -i SECONDS="0"
//	declare -p LINENO         declare -- LINENO="1"
//	declare -p EPOCHSECONDS   declare -- EPOCHSECONDS="1789252077"
//
// `LINENO` is the row that says the letters are this shell's fact rather than
// the parameter's: bash 3.2 writes `-i` for it and ksh93 writes `-i` too,
// where 5.3 writes none. This dialect models 5.3.
func TestAProducedParameterListsBack(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{"declare -p RANDOM", `^declare -i RANDOM="[0-9]+"\n$`},
		{"declare -p SECONDS", `^declare -i SECONDS="[0-9]+"\n$`},
		{"declare -p LINENO", `^declare -- LINENO="1"\n$`},
		{"declare -p EPOCHSECONDS", `^declare -- EPOCHSECONDS="[0-9]+"\n$`},
		{"declare -p EPOCHREALTIME", `^declare -- EPOCHREALTIME="[0-9]+\.[0-9]+"\n$`},
		// The listing and the expansion agree about whether the name is
		// there, which is the split the bug was: `$RANDOM` answered a
		// number on the line above and `declare -p RANDOM` said the name
		// was not found.
		{`echo "${RANDOM+set}"; declare -p RANDOM >/dev/null; echo "st=$?"`, "^set\nst=0\n$"},
	} {
		out, st := runBash(t, dir, tc.src)
		if !regexp.MustCompile(tc.want).MatchString(out) || st != 0 {
			t.Errorf("%s = %q at %d, want a match for %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The listing is a listing and nothing else. A parameter this shell lists with
// `-i` is not a name arithmetic folds on assignment — real bash ignores what
// is assigned to `EPOCHSECONDS` and seeds the generator with what is assigned
// to `RANDOM`, both of which are the producer's business (#2451, #1158).
func TestListingAProducedParameterDoesNotChangeWhatAssigningItDoes(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, "EPOCHSECONDS=7\necho \"[${EPOCHSECONDS:0:2}]\"\n")
	if want := "[17]\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0 — the clock still answers", out, st, want)
	}
}

// And the listing with **no operands** writes the row without the reading —
// the opposite of what the same shell writes when the name is asked for by
// hand, which is why the two are separate facts. Measured 2026-09-13, bash
// 5.3.15, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over a script
// file, with nothing having read the parameters first:
//
//	declare -i RANDOM      declare -- LINENO
//	declare -- SECONDS     declare -- EPOCHSECONDS
//
// The same rows come back from the same binary invoked as `sh`, so the shape
// belongs to bash rather than to the invocation. bash writes the reading once
// something has expanded the parameter and repeats the same one afterwards;
// neither the gate nor the cache is modeled here, and both are #2722 (#2518).
func TestABareListingWritesTheProducedNamesWithNoReading(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, "declare -p")
	if st != 0 {
		t.Fatalf("declare -p answered %d, want 0: %q", st, out)
	}
	for _, want := range []string{
		`(?m)^declare -i RANDOM$`,
		`(?m)^declare -- LINENO$`,
		`(?m)^declare -- EPOCHSECONDS$`,
	} {
		if !regexp.MustCompile(want).MatchString(out) {
			t.Errorf("declare -p = %q, want a row matching %q", out, want)
		}
	}
	// The control, and the reason the rows above are anchored: a listing
	// that had simply stopped writing values would pass all three.
	//
	// It used to be `OPTIND`, which is no longer ordinary: this shell gives
	// its own parameters the integer attribute and that one carries it —
	// `declare -i OPTIND="1"`, measured 2026-09-18 (#3099).
	if want := regexp.MustCompile(`(?m)^declare -- PWD="`); !want.MatchString(out) {
		t.Errorf("declare -p = %q, want an ordinary name to keep its value", out)
	}
	// And a produced name asked for by hand still carries one, from the same
	// shell in the same run.
	named, st := runBash(t, dir, "declare -p RANDOM")
	if want := regexp.MustCompile(`^declare -i RANDOM="[0-9]+"\n$`); !want.MatchString(named) || st != 0 {
		t.Errorf("declare -p RANDOM = %q at %d, want the reading beside the name", named, st)
	}
}
