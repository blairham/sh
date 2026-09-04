// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// rematch runs with the `=~` capture record exposed under a name, which is
// all a dialect supplies.
func rematch(name string) func(*Runner) {
	return func(r *Runner) { r.SetRegexMatch(name) }
}

// What the record holds: element 0 is the whole match and the rest are the
// groups, in order — reading them back is the idiom `=~` exists for.
func TestTheRegexMatchRecordsCaptures(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"whole match and groups", `[[ abcd =~ (b)(c) ]]; echo "[${M[0]}|${M[1]}|${M[2]}]"`, "[bc|b|c]"},
		{"no groups is one element", `[[ abcd =~ b.d ]]; echo "n=${#M[@]} [${M[0]}]"`, "n=1 [bcd]"},
		// The record is dense: a group that matched nothing is an empty
		// element, so the group after it keeps its number.
		{"unmatched group is empty", `[[ abcd =~ b(x)?(c) ]]; echo "n=${#M[@]} [${M[1]}|${M[2]}]"`, "n=3 [|c]"},
		// A failed match empties the record rather than leaving the capture
		// before last for an unchecked status to misread.
		{"failure empties it", `[[ ab =~ a ]]; [[ ab =~ q ]]; echo "n=${#M[@]} [${M[0]-UNSET}]"`, "n=0 [UNSET]"},
		{"failure with no history", `[[ ab =~ q ]]; echo "n=${#M[@]}"`, "n=0"},
		// The evaluation records, before `!` sees the result — the same rule
		// the pipeline-status record follows.
		{"negation does not reach it", `[[ ! ab =~ a ]]; echo "st=$? [${M[0]}]"`, "st=1 [a]"},
		{"a later match replaces it", `[[ ab =~ a ]]; [[ cd =~ (c) ]]; echo "[${M[0]}|${M[1]}]"`, "[c|c]"},
		// An ordinary stored array rather than a produced one: unset removes
		// it, and the next `=~` fills it again.
		{"unset removes until the next match", `[[ ab =~ a ]]; unset M; echo "[${M[0]-UNSET}]"; [[ cd =~ c ]]; echo "[${M[0]}]"`, "[UNSET]\n[c]"},
		{"a script can assign over it", `[[ ab =~ a ]]; M=(x y); echo "[${M[1]}]"`, "[y]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, rematch("M"))
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// Without a dialect to name it, there is no record at all — nothing could
// read one, so nothing is kept.
func TestNoNameMeansNoRegexRecord(t *testing.T) {
	out, _ := run(t, `[[ abcd =~ (b)(c) ]]; echo "st=$? [${M[@]}]"`, nil)
	if strings.TrimSpace(out) != "st=0 []" {
		t.Errorf("got %q, want an ordinary absent variable", out)
	}
}
