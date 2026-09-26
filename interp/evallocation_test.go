// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether text handed to `eval` is a place of its own — see
// Runner.EvalTextHasALocationOfItsOwn. A session switch and not an axis, so
// this names the switch and no shell; the axis beside it,
// Semantics.EvalTextContinuesTheCallersLines, decides how the text's own
// numbering is anchored and is pinned in evallines_test.go.
//
// The whole `eval` is on one physical line with a newline carried in through
// a parameter, for evalOnOneLine's reason: spread over several lines, "the
// caller's line, kept" and "the physical line the text sits on" are the same
// number and a green test would prove nothing. Here the `eval` is on line 3
// and the read is on the text's second line, so keeping the caller's line is
// 3 and every other reading is not.
func TestEvalTextHasALocationOfItsOwnPinsTheLine(t *testing.T) {
	for _, tc := range []struct {
		name string
		own  bool
		want string
	}{
		{"its own", true, "L=2"},
		{"the caller's", false, "L=3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := evalOnOneLine + `eval "echo x${nl}echo L=\$LINENO"` + "\n"
			out, _ := run(t, src, func(r *Runner) {
				r.SetEvalTextHasALocationOfItsOwn(tc.own)
			})
			if want := "x\n" + tc.want; strings.TrimSpace(out) != want {
				t.Errorf("own=%v: got %q, want %q", tc.own, strings.TrimSpace(out), want)
			}
		})
	}
}

// And it ends with the text: the line after the `eval` is the file's own
// under either state, so the pin is not a shell-wide freeze.
func TestTheEvalLocationPinEndsWithTheText(t *testing.T) {
	for _, own := range []bool{true, false} {
		src := evalOnOneLine + `eval "echo x"` + "\necho after=$LINENO\n"
		out, _ := run(t, src, func(r *Runner) {
			r.SetEvalTextHasALocationOfItsOwn(own)
		})
		if want := "x\nafter=4"; strings.TrimSpace(out) != want {
			t.Errorf("own=%v: got %q, want %q", own, strings.TrimSpace(out), want)
		}
	}
}

// A nested `eval` is the same question asked again rather than a second
// offset: with the location kept, every read inside either text is the
// outermost `eval`'s own line.
func TestTheEvalLocationPinReachesANestedEval(t *testing.T) {
	src := evalOnOneLine + `eval "echo outer=\$LINENO${nl}eval 'echo inner=\$LINENO'"` + "\n"
	out, _ := run(t, src, func(r *Runner) {
		r.SetEvalTextHasALocationOfItsOwn(false)
	})
	if want := "outer=3\ninner=3"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
	}
}

// Sourced files are not this question. A `.` is a file and its lines are its
// own in every shell in the panel, so the switch must not reach one.
func TestTheEvalLocationPinDoesNotReachASourcedFile(t *testing.T) {
	for _, own := range []bool{true, false} {
		dir := t.TempDir()
		write(t, dir, "inc.sh", "echo \"inc L=$LINENO\"\n")
		out, _ := sourceRunWith(t, dir, "echo pad\n. ./inc.sh\n",
			permissive(), Diagnostics{}, func(r *Runner) {
				r.SetEvalTextHasALocationOfItsOwn(own)
			})
		if want := "pad\ninc L=1"; strings.TrimSpace(out) != want {
			t.Errorf("own=%v: got %q, want %q", own, strings.TrimSpace(out), want)
		}
	}
}
