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
