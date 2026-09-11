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

// reporting runs with the second shape: what `=~` matched published through
// the parameters a reporting *pattern* already fills, rather than through a
// dense array of its own.
func reporting(r *Runner) { r.SetRegexCaptureReport() }

// The second shape, in full. `$MATCH` is the whole match and `$match` the
// groups **alone**, so the first element of the array is the first group and
// not the whole match — which is the difference from the record above, and
// the reason the two are two rather than one under an option.
//
// The indices below count from the base this vector has, which is zero, and
// the positions are counted with it: publishMatch reads Runner.arrayBase for
// exactly that reason, so a vector that counts from one moves the elements
// and the numbers together. No shell is named here; the base is the axis.
func TestARegexMatchCanReportThroughTheReportingParameters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"whole match and positions", `[[ abcd =~ b(c) ]]; echo "[$MATCH] $MBEGIN $MEND"`, "[bc] 1 2"},
		{"groups alone, with their positions", `[[ abcd =~ b(c) ]]
echo "[${match[0]}] ${mbegin[0]} ${mend[0]} n=${#match[@]}"`, "[c] 2 2 n=1"},
		// Characters rather than bytes, which is what the reporting pattern
		// already answers — the same arithmetic, so the same code.
		{"characters, not bytes", `[[ aébc =~ b ]]; echo "$MBEGIN $MEND"`, "2 2"},
		// A group that did not participate is an empty element at -1, so the
		// group after it keeps its number.
		{"an unmatched group keeps its place", `[[ abcd =~ b(x)?(c) ]]
echo "n=${#match[@]} [${match[0]}|${match[1]}] ${mbegin[0]} ${mend[0]}"`, "n=2 [|c] -1 -1"},
		// A pattern with no groups leaves the array alone rather than
		// emptying it, which is the reporting pattern's silence rule: a
		// surface nothing asked for is not written.
		{"no groups leaves the array alone", `[[ ab =~ (a) ]]; [[ cd =~ c ]]
echo "[$MATCH] [${match[0]}]"`, "[c] [a]"},
		// And a **failed** match leaves all of them holding what the match
		// before it put there. That is the opposite of the dense record
		// above, and it is the half one implementation for both shapes would
		// have got wrong.
		{"failure changes nothing", `[[ ab =~ (a) ]]; [[ ab =~ zz ]]
echo "st=$? [$MATCH] [${match[0]}] $MBEGIN"`, "st=1 [a] [a] 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, reporting)
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// Unasked, nothing is written — so a vector without the reporting parameters
// keeps a `$MATCH` a script put there for its own reasons.
func TestWithoutTheReportARegexMatchWritesNothing(t *testing.T) {
	out, _ := run(t, `MATCH=mine; match=(mine)
[[ abcd =~ b(c) ]]
echo "st=$? [$MATCH] [${match[0]}] [${MBEGIN-UNSET}]"`, nil)
	if strings.TrimSpace(out) != "st=0 [mine] [mine] [UNSET]" {
		t.Errorf("got %q, want the script's own values untouched", strings.TrimSpace(out))
	}
}
