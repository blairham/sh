// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// Whether a compound command replaces the pipeline-status record is decided by
// what its body *holds*, not by what ran.
//
// Measured 2026-09-11, zsh 5.9.2, each line written after `false | true` so a
// replaced record shows as one element:
//
//	if [[ a = b ]]; then :; fi              0
//	if [[ a = b ]]; then [[ b = b ]]; fi    1 0
//
// Neither body runs. The only difference is the text inside `then`, and an
// unexecuted `:` is enough to make the compound count as a command. We wrote
// the record for every compound before this (#1931), which is the plausible
// one-element answer `pipestatus/reading-it-twice-in-one-chain` exists to
// protect against.
func TestACompoundWritesTheRecordOnlyForWhatItsBodyHolds(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`if [[ a = b ]]; then :; fi`, "0"},
		{`if [[ a = b ]]; then [[ b = b ]]; fi`, "1 0"},
		{`if :; then [[ b = b ]]; fi`, "0"},
		{`{ :; }`, "0"},
		{`{ [[ a = a ]]; }`, "1 0"},
		{`{ x=1; }`, "1 0"},
		{`{ (( 1 )); }`, "1 0"},
		// The condition of a loop is body too, which is what the pair says.
		{`while false; do [[ a = a ]]; done`, "0"},
		{`while [[ a = b ]]; do [[ a = a ]]; done`, "1 0"},
		{`for i in 1; do [[ a = a ]]; done`, "1 0"},
		{`for i in 1; do :; done`, "0"},
		{`case a in b) [[ a = a ]];; esac`, "1 0"},
		{`case a in b) :;; esac`, "0"},
		{`case a in b) ;; esac`, "1 0"},
		{`repeat 0; do [[ a = a ]]; done`, "1 0"},
		{`repeat 0; do :; done`, "0"},
		// Nesting, both ways round.
		{`{ { [[ a = a ]]; } }`, "1 0"},
		{`{ { :; } }`, "0"},
		// A subshell is a job whatever it holds, and so is a background job,
		// a real pipeline and a redirected construct.
		{`( [[ a = a ]] )`, "0"},
		{`{ ( [[ a = a ]] ); }`, "0"},
		{`{ [[ a = a ]] & }`, "0"},
		{`{ [[ a = a ]] | [[ b = b ]]; }`, "0"},
		{`{ [[ a = a ]]; } >/dev/null`, "0"},
		// A definition runs nothing and is not read into.
		{`{ f() { :; }; }`, "1 0"},
	} {
		out, st := answersRun(t, "false | true; "+tc.src+`; print -r -- ${pipestatus}`)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("false | true; %s\n got %q (status %d)\nwant %q at 0", tc.src, got, st, tc.want)
		}
	}
}
