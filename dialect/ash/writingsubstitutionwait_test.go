// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// ash answers WritingSubstitutionIsWaitedForAtTheCommand, and answers it with
// bash: the command that named a `>(cmd)` finishes, and the body's bytes land
// after it.
//
// The axis was left unanswered here on a premise that was never true of this
// shell — that BusyBox has no process substitution, so `echo >(:)` is the two
// characters as written. It has it. Measured 2026-09-21 in the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0: `echo >(:)` is `/dev/fd/64` at 0, the three duration rows
// (`printf x | tee >(sleep 3) >/dev/null`, `echo >(sleep 3)`,
// `echo hi > >(sleep 3)`) are each 0s where zsh is 3s, and the ordering row
// below is `AFTER[PIPE]` where zsh is `[PIPE]AFTER` (#4002, after #3986).
//
// The assertion is the *ordering* and the presence together, which is what
// the issue's own probe could not do. Its body sleeps before writing and its
// script ended first, so it read `AFTER` alone — no `[PIPE]` anywhere — which
// is a third answer the axis does not have, and it was filed as one. Asserting
// only that the output starts with `AFTER` would pass that broken reading too,
// so `[PIPE]` is required separately.
func TestAWritingSubstitutionIsNotWaitedForAtTheCommand(t *testing.T) {
	const src = `printf "PIPE\n" | tee >(read -r v; sleep 0.2; printf "[%s]" "$v") >/dev/null; printf "AFTER"`

	out, st := run(t, src)
	if out != "AFTER[PIPE]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "AFTER[PIPE]")
	}
	if !strings.Contains(out, "[PIPE]") {
		t.Error("the body's bytes were lost — the reading #4002 was filed on, which no answer to this axis has")
	}
}

// And the axis is answered rather than merely reading as No by default. An
// unanswered axis is Unspecified, which this one is read as `!= Yes` and so
// behaves identically — which is exactly why the ledger entry could stay wrong
// without any test going red.
func TestTheWritingSubstitutionWaitAxisIsAnswered(t *testing.T) {
	if got := ash.Semantics().WritingSubstitutionIsWaitedForAtTheCommand; got != interp.No {
		t.Errorf("got %v, want %v — measured as bash's ordering in the pinned image (#4002)", got, interp.No)
	}
}
