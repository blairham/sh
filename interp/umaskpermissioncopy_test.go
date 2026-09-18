// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A permission copy written beside permission letters, which POSIX's grammar
// does not describe and the three shells that read it read three ways (#3074).
//
// From a mask of 222 — so what is allowed is 555 — `g=u` copies the owner's
// `r-x`. `g=uw` is the non-discriminating spelling, since two of the readings
// arrive at `rwx` from opposite directions; `g=wu`, with the copy **last**, is
// the one that parts them.
func TestAPermissionCopyBesideLetters(t *testing.T) {
	for _, tc := range []struct {
		name    string
		policy  UmaskPermissionCopyPolicy
		src     string
		mask    int
		refused bool
	}{
		// The copy throws the letters away — so `g=wu` leaves the group
		// exactly what the owner is allowed, which from 222 is what it
		// already had. The status is what parts this row from the refusal
		// below, and the copy-first row is what parts it from the reading
		// that joins them.
		{name: "replaces, copy last", policy: UmaskPermissionCopyReplaces, src: "umask g=wu", mask: 0o222},
		{name: "replaces, copy first", policy: UmaskPermissionCopyReplaces, src: "umask g=uw", mask: 0o202},
		// The copy joins them.
		{name: "contributes, copy last", policy: UmaskPermissionCopyContributes, src: "umask g=wu", mask: 0o202},
		{name: "contributes, copy first", policy: UmaskPermissionCopyContributes, src: "umask g=uw", mask: 0o202},
		// Neither: the mixture is refused, in either order, and the mask is
		// left where it was.
		{name: "refuses, copy last", policy: UmaskPermissionCopyRefusesTheMixture, src: "umask g=wu", mask: 0o222, refused: true},
		{name: "refuses, copy first", policy: UmaskPermissionCopyRefusesTheMixture, src: "umask g=uw", mask: 0o222, refused: true},
		// The controls. A copy on its own is the portable spelling and is
		// taken under every answer, and a clause with no copy in it never
		// reaches the question.
		{name: "a copy alone, replaces", policy: UmaskPermissionCopyReplaces, src: "umask g=u", mask: 0o222},
		{name: "a copy alone, contributes", policy: UmaskPermissionCopyContributes, src: "umask g=u", mask: 0o222},
		{name: "a copy alone, refuses the mixture", policy: UmaskPermissionCopyRefusesTheMixture, src: "umask g=u", mask: 0o222},
		{name: "letters alone, replaces", policy: UmaskPermissionCopyReplaces, src: "umask g=w", mask: 0o252},
		{name: "letters alone, refuses the mixture", policy: UmaskPermissionCopyRefusesTheMixture, src: "umask g=w", mask: 0o252},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, held := umaskRun(t, 0o222, func(s *Semantics) {
				s.SymbolicMaskTakesAPermissionCopy = Yes
				s.UmaskPermissionCopyBesideLetters = tc.policy
			}, tc.src)
			if held != tc.mask {
				t.Errorf("mask %04o afterwards, want %04o (%q)", held, tc.mask, out)
			}
			if tc.refused != (st != 0) {
				t.Errorf("status %d, refused=%v want %v (%q)", st, st != 0, tc.refused, out)
			}
		})
	}
	// An unanswered axis refuses by name, and only for the clause that holds
	// both — the portable spelling is still taken.
	out, st, held := umaskRun(t, 0o222, func(s *Semantics) {
		s.SymbolicMaskTakesAPermissionCopy = Yes
		s.UmaskPermissionCopyBesideLetters = UmaskPermissionCopyUnspecified
	}, "umask g=wu")
	if st != 2 || !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
		t.Errorf("unanswered: out %q status %d, want the refusal by name at 2", out, st)
	}
	if held != 0o222 {
		t.Errorf("unanswered left the mask %04o, want it untouched", held)
	}
	out, st, held = umaskRun(t, 0o222, func(s *Semantics) {
		s.SymbolicMaskTakesAPermissionCopy = Yes
		s.UmaskPermissionCopyBesideLetters = UmaskPermissionCopyUnspecified
	}, "umask g=u")
	if st != 0 || out != "" || held != 0o222 {
		t.Errorf("a copy alone with the axis unanswered: out %q status %d mask %04o", out, st, held)
	}
}
