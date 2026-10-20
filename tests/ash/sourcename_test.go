// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// sourceRun runs src as a script file named case.sh, which is what puts the
// invoked name in the location this file is about.
func sourceRun(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "case.sh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// `source` is this shell's second name for `.`, and everything it says it says
// under that name — the location carries the word the script wrote and the
// message carries no verb at all.
//
// Measured 2026-09-18 in the digest-pinned alpine image
// (alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b),
// BusyBox v1.37.0, with `cmd/ash` cross-compiled for linux/arm64 and run in
// the same container so both halves come from one kernel:
//
//	source ./nope.sh   case.sh: source: line 1: can't open './nope.sh': …  2
//	. ./nope.sh        case.sh: .: line 1: can't open './nope.sh': …       2
//
// Ours wrote a `.: ` between the location and the sentence under the second
// spelling only. The templates opened with a literal `.: ` and
// Diagnostics.NamesBuiltinInLocation takes the *invoked* name back out again,
// so the verb came off for `.` and stayed for `source` (#3277).
func TestTheSecondNameForDotSaysNothingAboutTheFirst(t *testing.T) {
	for _, verb := range []string{".", "source"} {
		t.Run(verb, func(t *testing.T) {
			out, st := sourceRun(t, verb+" ./nope.sh\n")
			// The whole line, so the verb cannot be in the sentence as
			// well as in the location: the reason is the operating
			// system's and is the only part not asserted.
			want := "case.sh: " + verb + ": line 1: can't open './nope.sh': "
			if !strings.HasPrefix(out, want) || strings.Count(out, "\n") != 1 {
				t.Errorf("%s ./nope.sh said %q, want one line opening %q", verb, out, want)
			}
			if st != 2 {
				t.Errorf("%s ./nope.sh ended at %d, want 2", verb, st)
			}
		})
	}
}

// And a missing operand is an error with **no sentence for it**, which is the
// one place in this file those two questions come apart.
//
// Measured in the same run, from a script file with a line after it: `.` and
// `source` with no operand each leave 2 behind, write nothing at all on either
// stream, and the next line runs — where the same shell's `.` on a file it
// cannot open ends the script. So the silence is Diagnostics.DotNoOperandSilent
// and the survival is Semantics.DotWithNoOperandIsFatal, which is a second
// question from DotMissingFileFatal and was reading it (#3277).
func TestAMissingOperandIsSilentAndSurvivable(t *testing.T) {
	for _, verb := range []string{".", "source"} {
		t.Run(verb, func(t *testing.T) {
			out, st := sourceRun(t, verb+"\necho A=$?\necho second\n")
			if want := "A=2\nsecond\n"; out != want {
				t.Errorf("%s alone said %q, want %q — silent, 2, and the script runs on", verb, out, want)
			}
			if st != 0 {
				t.Errorf("%s alone ended the script at %d, want 0", verb, st)
			}
		})
	}
}
