// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"os"

	"github.com/blairham/sh/internal/opened"
)

// The gate half of "a name is not an object".
//
// What the kernel says a descriptor holds, when two spellings are one place,
// and the truncation held back until a decision can be made are all in
// internal/opened, which states the design and the reason it is asked *after*
// the open rather than before. What is here is the part that belongs to a
// Runner: which action the second consultation carries, what the audit stream
// is told, and what the script is told — three things the front end's own
// enforcer answers differently, which is why they did not move.

// verifyOpened reports whether the object a just-opened descriptor holds is
// one the gate permits, having been asked about the name that reached it.
//
// The action it consults with carries the same ID as the one the caller
// already asked about, because it is the same access: a consumer joining the
// two records sees one open whose name resolved elsewhere, not two opens.
//
// A refusal is recorded here, with the resolved path on it, and reported by
// the caller, without it. That split is deliberate. The audit stream belongs
// to whoever wrote the policy and has to say what was actually reached, or
// the record cannot be acted on; the diagnostic goes to the script, which is
// the party the policy is about, and a refusal naming where a link pointed
// would hand it the one fact the rule exists to withhold. So every caller
// below reports the name as written, in the wording an ordinary refusal
// already uses — a script cannot tell the two apart, which is the same
// property a denied stat has for the same reason.
func (r *Runner) verifyOpened(ctx context.Context, a *Action, reached opened.Reached) bool {
	if r.Gate == nil {
		return true
	}
	actual, elsewhere := opened.Elsewhere(reached, a.Path)
	if !elsewhere {
		// The object has no name a rule could be speaking about, or the name
		// the script wrote is the path the open went through, or
		// the two are the operating system's own two names for one place.
		// In each case the decision already made is the decision about this
		// object.
		return true
	}
	// The action gains where it went, and keeps the name it was raised about.
	// The caller's record is the same value, so an *allowed* open now says
	// both — which it did not, and which was the whole of #943.
	a.Resolved = actual
	// The gate is asked about the *object*, and about nothing else: Path is
	// the kernel's name here, so a rule matches what was reached even if the
	// Gate has never heard of Resolved. See the field's own comment for why
	// the name as written is deliberately not handed over.
	asked := *a
	asked.Path = actual
	if r.Gate.Allow(ctx, asked) == Deny {
		r.emit(ctx, Event{Kind: EventDenied, Action: *a})
		return false
	}
	return true
}

// reportRefusal tells the script an action was refused, naming the path it
// wrote.
//
// Shared by allowed() and by the verification above so that the two produce
// the same sentence — the point being that they must, since a script that
// could tell "the name is denied" from "the name reached a denied object" has
// been told where the name went.
func (r *Runner) reportRefusal(a Action) {
	r.diagf("%s: refused: %s\n", a.Kind, a.Path)
}

// openGated opens a file the way a redirect needs it opened, and confirms
// what it reached before letting a byte through.
//
// A run with no gate takes the first line and nothing else, so its opens are
// the calls they always were, flags included.
func (r *Runner) openGated(ctx context.Context, a *Action, path string, flags int) (*os.File, error) {
	if r.Gate == nil {
		return os.OpenFile(path, flags, 0o666)
	}
	return opened.Verified(path, flags, 0o666, func(reached opened.Reached) error {
		if !r.verifyOpened(ctx, a, reached) {
			return errRefused
		}
		return nil
	})
}

// readFileGated is os.ReadFile with the same verification, for `.`.
//
// Split from openGated rather than layered on it because the two answer to
// different callers: a redirect hands the descriptor to a command and needs
// the flags, and `.` wants the bytes and closes the file itself.
func (r *Runner) readFileGated(ctx context.Context, a *Action, path string) ([]byte, error) {
	if r.Gate == nil {
		return os.ReadFile(path)
	}
	// Through Verified rather than os.Open, so that this open is resolved the
	// same way a redirect's is. It was a plain os.Open while the verification
	// read the answer back off the descriptor, because then the open itself
	// did not have to be the checked one. Now it does: the walk is what makes
	// the name honest, and a second door that opened files any other way would
	// be the #942 defect again in a smaller room.
	f, err := opened.Verified(path, os.O_RDONLY, 0, func(reached opened.Reached) error {
		if !r.verifyOpened(ctx, a, reached) {
			return errRefused
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}
