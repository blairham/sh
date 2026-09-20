// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// rematch runs with the `=~` capture record exposed under a name, and with
// the two questions a named record raises answered the dense way — which is
// the shape the rows below are about. Moving them is
// TestWhatARegexMatchLeavesInTheRecord's business.
func rematch(name string) func(*Runner) {
	return func(r *Runner) {
		sem := permissive()
		sem.RegexMatchSurvivesAFailedMatch = No
		sem.RegexMatchOmitsGroupsThatDidNotMatch = No
		r.Semantics = &sem
		r.SetRegexMatch(name)
	}
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

// A newline in the subject is ordinary ground: `.` passes over one, and a
// negated bracket does too.
//
// That is what a POSIX ERE is — the engine underneath is asked for it with a
// flag, because Go's own default is the other reading. The two rows answering
// `N` are the control and not decoration: they say the flag is the one that
// moves `.` alone, since the flag that moves the anchors to line boundaries
// instead would turn both of them into matches. Measured 2026-09-20 across
// every column of the panel, which agrees with itself on all six. #3892.
func TestARegexReadsANewlineAsOrdinaryGround(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a dot spans a newline", `[[ $'a\nb' =~ a.b ]] && echo Y || echo N`, "Y"},
		{"and a dot between anchors spans one", `[[ $'a\nb' =~ ^a.b$ ]] && echo Y || echo N`, "Y"},
		{"so does a negated bracket", `[[ $'a\nb' =~ a[^x]+b ]] && echo Y || echo N`, "Y"},
		// The controls. An anchor is about the ends of the *subject*, so
		// neither of these reaches the second line.
		{"^ is the start of the subject", `[[ $'a\nb' =~ ^b ]] && echo Y || echo N`, "N"},
		{"$ is the end of the subject", `[[ $'a\nb' =~ a$ ]] && echo Y || echo N`, "N"},
		// The shape this is really about: a pattern over multi-line output,
		// which is what most scripts write `=~` for. The failure it used to
		// have was silent and status 0, so the script took the other branch
		// and carried on.
		{"a pattern over multi-line output", `out=$(printf 'version 1.2\nbuild 99\n')
[[ $out =~ version.*build ]] && echo Y || echo N`, "Y"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, nil)
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// The captures come with it. A match that never happened has no groups to
// record, so the span over a newline is the same defect read through the
// record rather than a second one.
func TestARegexCaptureSpansANewline(t *testing.T) {
	out, _ := run(t, `[[ $'a\nb' =~ (a.b) ]]; printf '[%s]\n' "${M[1]}"`, rematch("M"))
	if got := strings.TrimSpace(out); got != "[a\nb]" {
		t.Errorf("got %q, want %q", got, "[a\nb]")
	}
}

// The fold composes with it rather than replacing it, which is what says the
// two flags are written as one prefix and not one overwriting the other.
// Both halves are asserted: a folded match still spans the newline, and a
// fold that reached no further than before would fail the first row here.
func TestAFoldedRegexStillSpansANewline(t *testing.T) {
	foldingRegex := func(r *Runner) { r.SetMatchOption(RegexFoldsCase, true) }
	for _, tc := range []struct{ name, src, want string }{
		{"the fold survives the flag", `[[ ABC =~ ^abc$ ]] && echo Y || echo N`, "Y"},
		{"and the newline is still ordinary", `[[ $'A\nB' =~ a.b ]] && echo Y || echo N`, "Y"},
		// The fold precedes the negation, which is the row that says the
		// prefix is still being read as a prefix.
		{"the fold still precedes a negation", `[[ A =~ ^[^a]$ ]] && echo Y || echo N`, "N"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, foldingRegex)
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}
