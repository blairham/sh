// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestADenyCoversTheOtherSpellingsOfTheSameFile is #2044, written as the
// policy that leaked rather than as a unit on the matcher.
//
// The `allow` beside the `deny` is the whole bug and not scenery: under a
// bare `default deny` every one of these spellings is refused anyway, and a
// test written that way passes against a matcher that folds nothing. It is
// the carve-out shape — allow the project, deny the secret — that lets a
// respelling miss the deny and land on the allow.
func TestADenyCoversTheOtherSpellingsOfTheSameFile(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /proj/**\ndeny read /proj/.env\n")
	want(t, p, interp.Allow, open("/proj/src/main.go", false))
	want(t, p, interp.Deny,
		open("/proj/.env", false),
		open("/proj/.ENV", false),
		open("/proj/.Env", false),
		open("/proj/.eNv", false),
	)
}

// TestTheFoldReachesEveryComponent, because a rule is about a path and not
// about its last name. `/PROJ/.env` is the same file as `/proj/.env` on the
// same volume, and a fold that stopped at the leaf would be a fold that
// closed the demonstration and not the hole.
func TestTheFoldReachesEveryComponent(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /proj/**\ndeny read /proj/secret/**\n")
	want(t, p, interp.Deny,
		open("/proj/secret/k", false),
		open("/proj/SECRET/k", false),
		open("/PROJ/secret/k", false),
		open("/PrOj/SeCrEt/k", false),
	)
}

// TestAFoldedDenyCoversWritingToo, since the measured escape overwrote the
// file it could not read — a fold on the read selector alone would have left
// the more damaging half open.
func TestAFoldedDenyCoversWritingToo(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow write /proj/**\ndeny write /proj/.env\n")
	want(t, p, interp.Allow, open("/proj/build.log", true))
	want(t, p, interp.Deny, open("/proj/.ENV", true))
}

// TestAnAllowIsNotFolded is the other half of the argument, and the one that
// would be a hole rather than an inconvenience if it were wrong.
//
// Widening a deny only ever refuses more. Widening an allow would hand a
// policy naming `/srv/data` a read of `/srv/DATA`, which on a case-sensitive
// volume is a different file nobody granted. So the allow side stays exact,
// and this test is what stops a later "make it symmetrical" tidy-up.
func TestAnAllowIsNotFolded(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/data/**\n")
	want(t, p, interp.Allow, open("/srv/data/x", false))
	want(t, p, interp.Deny, open("/srv/DATA/x", false), open("/SRV/data/x", false))
}

// TestTheFoldDoesNotDisturbAnExactPolicy — the fold is a fallback tried only
// after an exact match has failed, so a policy with no respelling in it has
// to decide exactly as it did before.
func TestTheFoldDoesNotDisturbAnExactPolicy(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/**\ndeny read /srv/secret/**\n")
	want(t, p, interp.Allow, open("/srv/x", false), open("/srv/secretive/x", false))
	want(t, p, interp.Deny, open("/srv/secret/k", false))
}

// TestTheFoldKeepsComponentBoundaries. Lowercasing must not turn a rule into
// a prefix: `/srv/etc` still has nothing to say about `/srv/etcetera`, in
// either case, or the widening would be a different bug wearing this one's
// clothes.
func TestTheFoldKeepsComponentBoundaries(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/**\ndeny read /srv/etc/**\n")
	want(t, p, interp.Allow, open("/srv/etcetera/x", false), open("/srv/ETCETERA/x", false))
	want(t, p, interp.Deny, open("/srv/ETC/x", false))
}

// TestTheFoldIsNotADefaultAllow guards the one way a widening could turn into
// a leak: it must add refusals, never permissions. A policy that denies
// everything it was asked about still refuses what it never mentioned.
func TestTheFoldIsNotADefaultAllow(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndeny read /srv/secret/**\n")
	want(t, p, interp.Deny, open("/srv/secret/k", false), open("/srv/SECRET/k", false),
		open("/elsewhere/x", false))
}
