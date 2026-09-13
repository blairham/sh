// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/internal/dialecttest"
)

// The regression guard for #2272, and the shape it guards against is worth
// stating because no other test in this tree has it.
//
// A `Semantics` axis added after a dialect is written is *unanswered* there,
// and an unanswered axis does not fail a test — it refuses at run time, in
// the shipped binary, at status 2, with `go test ./...` green. That is how
// `read -t 1 x` came to be rejected by `cmd/ash`: ReadTimeoutBoundsReadability
// landed within an hour of this package, from another session, and neither
// change was wrong on its own.
//
// Nothing structural catches it — the conformance harness has no ash column
// (#2263) and the axis sweep has no ash target, both for the same reason,
// that the shell it grades against is not on this machine's PATH. So this
// runs the snippets instead: each is the corpus case that reaches one axis
// this dialect answers, and the assertion is the refusal's own words. A
// future axis that this file has no value for lands here rather than in
// somebody's terminal.
//
// The sweep that produced the list ran every `oracle.Corpus` snippet through
// the binary and collected each refusal: nineteen axes, not the one that was
// reported, and a twentieth uncovered by answering the first — a refusal early
// on a path hides the next question along it, so it was re-run until it
// stopped moving. Three remain unanswered on purpose and are named in
// ash.go and in docs/spec/ash.md: #2276, #2277 and #2278.
// runIn is run() with a working directory of its own. Two of the snippets
// below write a file — a `read -t 0` has to have a stream that is always
// ready, which is what a file is and a pipe is not — and run() leaves the
// runner's Dir empty, which is the package directory. A `dialect/ash/f`
// committed by this very test is how that was found.
func runIn(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "ash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		return out + "unsupported: " + err.Error(), -1
	}
	return out, st
}

func TestNoAnsweredAxisRefusesAtRunTime(t *testing.T) {
	for _, tc := range []struct{ name, src, why string }{
		{
			"read -t bounding the whole read",
			`read -t 1 x </dev/null; echo rc=$?`,
			"ReadTimeoutBoundsReadability — the row #2272 was reported on",
		},
		{
			"an expired read -t leaving the name alone",
			`v=old; printf '' | { read -t 1 v; echo "st=$? [$v]"; }`,
			"ReadTimeoutKeepsWhatArrived — reachable only once the axis above was answered",
		},
		{
			"read -t 0 polling",
			`printf 'a\nb\n' >f; exec <f; read -t 0 v; echo "st=$? v=[$v]"; read w; echo "w=[$w]"`,
			"ReadZeroTimeout",
		},
		{
			"a count and the names after the first",
			`printf 'XYZW\n' | { read -n 3 a 1bad b; echo st=$?; }`,
			"ReadCountJudgesTheNamesAfterTheFirst",
		},
		{
			"which of echo -e -E decides",
			`echo -e -E 'm\tn'`,
			"EchoLastEscapeFlagWins",
		},
		{
			"errexit and a failure only pipefail saw",
			"set -eo pipefail\nfalse | true\necho reached\n",
			"ErrexitSeesPipefailFailure",
		},
		{
			"the status pipefail substitutes for a signal",
			"set -o pipefail\nv=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done\n{ echo \"$v\"; } | true; echo st=$?\n",
			"PipefailSubstitutesTheBareSignal",
		},
		{
			"an assignment through an expansion onto a positional",
			"set --\nprintf '<%s>' ${1:=abc}\necho after\n",
			"AssignThroughExpansionMayNameAPositional",
		},
		{
			"a quote in a quoted replacement operand",
			`s=xay; v=VAL; printf '[%s]' "${s/a/'$v'}"; echo`,
			"ReplacementOperandTakesTheEnclosingQuoting",
		},
		{
			"an empty pattern in a span replacement",
			`v=abc; printf '[%s]' "${v///X}"; echo`,
			"EmptyReplacementPattern",
		},
		{
			"an escape no escape claims",
			`printf '[%s]' $'\q\8'; echo`,
			"DollarSingleUnknownEscape",
		},
		{
			"the backslash-c of a dollar-single",
			`printf '[%s]' $'\cA'; echo`,
			"DollarSingleBackslashC",
		},
		{
			"the caret and meta escapes",
			`printf '[%s]' $'\C-A' $'\M-x'; echo`,
			"DollarSingleCaretMeta",
		},
		{
			"a negative exponent",
			`printf '[%s]' "$((2**-1))"`,
			"ArithNegativeExponentIsError",
		},
		{
			"a declaration over a readonly",
			"readonly x=1\nf() { local x=2; }\nf\necho end\n",
			"DeclarationMayShadowAReadonly",
		},
		{
			"a valueless declaration of a name its scope holds",
			`f() { local v=1; local v; echo "[$v]"; }; f`,
			"ValuelessDeclarationOfAHeldNameListsIt",
		},
		{
			"a pid listing and a job that has finished",
			`sleep 0.05 & sleep 0.4; jobs -p >/dev/null; jobs; jobs`,
			"PidListingFinishesWithAJob — a bare `jobs -p` never reaches it, " +
				"which is why the issue's second report did not reproduce",
		},
	} {
		if strings.Contains(tc.src, "sleep") && testing.Short() {
			continue
		}
		out, _ := runIn(t, tc.src)
		if strings.Contains(out, "no dialect was chosen") {
			t.Errorf("%s: %s\n\nrefused: %s\n\nthis dialect has no value for the axis "+
				"the snippet reaches. Measure it against BusyBox — docs/spec/ash.md "+
				"says how — and write the value into ash.go. Do not copy dash's.",
				tc.name, tc.why, strings.TrimSpace(out))
		}
	}
}

// TestTheAxesLeftUnansweredStillRefuse is the other half, and it is not
// redundant: what this dialect cannot answer is recorded as an open question,
// and a value quietly appearing for one of them — copied from a neighboring
// dialect to make the message go away — is exactly what docs/spec/ash.md
// forbids. When one is closed properly, its issue moves the row from here to
// the table above.
func TestTheAxesLeftUnansweredStillRefuse(t *testing.T) {
	for _, tc := range []struct{ name, src, issue string }{
		{"a NUL inside $'…'", `x=$'a\0b'; echo ${#x}`, "#2276"},
	} {
		out, _ := runIn(t, tc.src)
		if !strings.Contains(out, "no dialect was chosen") {
			t.Errorf("%s (%s): %q answered rather than refusing.\n\n"+
				"If it was measured and closed, move the row into "+
				"TestNoAnsweredAxisRefusesAtRunTime and close the issue. If the "+
				"value was copied from another dialect to quiet the message, it is "+
				"a guess in a dialect nothing grades, which reads exactly like a "+
				"measurement — revert it.", tc.name, tc.issue, strings.TrimSpace(out))
		}
	}
	// `readonly -a` used to be a third row here and is not one any more, and
	// the way it left is the point rather than the arithmetic: the axis was
	// never unanswered, it was **unreachable**. This shell has no `-a` on the
	// word — `readonly: line 0: illegal option -a`, measured 2026-09-12 —
	// so it cannot be asked whether the letter records a kind. The fix was to
	// take the letters from the vector, where every other builtin's already
	// come from, rather than to invent a value for the axis (#2277).
	if out, _ := runIn(t, `f() { readonly -a a; }; f`); !strings.Contains(out, "-a") ||
		strings.Contains(out, "no dialect was chosen") {
		t.Errorf("`readonly -a` wrote %q, want the option refused: an axis a "+
			"shell cannot be asked must be closed at the option and not by "+
			"choosing one of the answers for it (#2277)", strings.TrimSpace(out))
	}
	// The last one is asked of the vector rather than of a run, because the
	// test harness gives the runner no resource limits at all and `ulimit
	// -a` stops on *that* first — a refusal that would pass this test while
	// saying nothing about the table.
	if rows := ash.Diagnostics().UlimitListing; len(rows) != 0 {
		t.Errorf("UlimitListing has %d rows (#2278). Measured in full and left "+
			"empty on purpose: five of BusyBox's fifteen rows name resources no "+
			"interp.Resource constant does, and they were measured on Linux, "+
			"where those limits exist.", len(rows))
	}
}
