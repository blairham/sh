// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A dialect written as shell has one function table where the shells it
// claims to be have a builtin table and a function table, so the names its
// prelude *presents* are builtins that happen to be spelled as functions.
// Until #1117 nothing said so: `compgen -A builtin` named none of them,
// `type` called them functions, and `builtin NAME` — the defensive spelling
// a script writes to get past a user function — simply failed.
//
// The rule is one table read by every surface, which is #1035's "one notion
// of prelude-ness, one answer" kept rather than broken. These cases name a
// rule and no shell; what a dialect's prelude presents is asserted under
// dialect/bash and dialect/zsh.

// presented fills in the axes these cases pass through, so that what they
// report is the presented-name rule and not an unanswered question.
func presented(s *Semantics) {
	s.TypePrintsFunctionBody = No
	s.CommandNotFoundStatusIsNotFound = No
	s.CommandReachesABuiltin = Yes
	s.BuiltinReadsOptions = No
	s.LoneDashIsAnOption = No
	s.EnableUnloadsABuiltin = No
	s.TypeOptions = "atfpPw"
	s.TypeEndsOptionsWithDashDash = Yes
}

// TestEverySurfaceCallsAPresentedNameABuiltin: the four reports that had two
// answers between them, now agreeing. The last row is the discriminator —
// `__helper` is a prelude declaration too, and it is nothing rather than a
// builtin, so this is the mark deciding which answer a name gets rather than
// every prelude name being promoted.
func TestEverySurfaceCallsAPresentedNameABuiltin(t *testing.T) {
	for _, tc := range []struct {
		src, out string
		status   int
	}{
		{src: "type p", out: "p is a shell builtin\n"},
		{src: "type -t q", out: "builtin\n"},
		{src: "command -V p", out: "p is a shell builtin\n"},
		{src: "compgen -A builtin q", out: "q\n"},
		{src: "compgen -A builtin __helper", status: 1},
		{src: "type __helper", status: 1},
	} {
		out, _, st := preludeListingRun(t, listingPrelude, tc.src+"\n", presented)
		if out != tc.out || st != tc.status {
			t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.out, tc.status)
		}
	}
}

// TestTheWordsThatAskForABuiltinReachAPresentedName: the execution half, and
// the reason option 2 in #1117 could not have been left alone — these are not
// a name missing from a listing. `builtin NAME` and `command NAME` both run
// the shell's own command and refuse a function, and a dialect written as
// shell has to reconstruct that boundary by hand.
func TestTheWordsThatAskForABuiltinReachAPresentedName(t *testing.T) {
	for _, src := range []string{"builtin q", "command q"} {
		out, errs, st := preludeListingRun(t, listingPrelude, src+"\n", presented)
		if out != "q\n" || errs != "" || st != 0 {
			t.Errorf("%s = %q / %q at %d, want %q and a silent 0", src, out, errs, st, "q\n")
		}
	}
}

// TestAWordAskingForABuiltinPassesOverAScriptsOwnFunction: what those two
// words are *for*. A script that wraps `q` and calls the shell's own from
// inside the wrapper must not call itself, which is the whole reason the
// spelling exists — and it is the case a fix that merely added the names to
// a listing would still get wrong.
func TestAWordAskingForABuiltinPassesOverAScriptsOwnFunction(t *testing.T) {
	src := "q() { echo wrapped; builtin q; }\nq\n"
	out, errs, st := preludeListingRun(t, listingPrelude, src, presented)
	if want := "wrapped\nq\n"; out != want || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q status %d, want %q and a silent 0", out, errs, st, want)
	}
}

// TestSwitchingAPresentedNameOffStopsTheWordFinding: a builtin that is
// switched off stops resolving, and a presented name is a builtin. The
// implementation is still in the one function table this shell has, so
// nothing but this would stop the word reaching it — which would have made
// the switch a silent no-op the moment `enable` started accepting the name.
func TestSwitchingAPresentedNameOffStopsTheWordFinding(t *testing.T) {
	out, _, st := preludeListingRun(t, listingPrelude, "enable -n q\nq\n", presented)
	if out != "" {
		t.Errorf("stdout = %q, want nothing — the name was switched off", out)
	}
	if st == 0 {
		t.Error("status = 0, want a failure: the word resolves to nothing now")
	}

	// And the report agrees, which is the same table being read: a name
	// switched off is not one this shell has.
	out, _, st = preludeListingRun(t, listingPrelude, "enable -n q\ntype q\n", presented)
	if out != "" || st != 1 {
		t.Errorf("type after the switch = %q at %d, want nothing at 1", out, st)
	}

	// Switching it back on gets the command back, which is what separates a
	// switch from a removal.
	out, _, st = preludeListingRun(t, listingPrelude, "enable -n q\nenable q\nq\n", presented)
	if out != "q\n" || st != 0 {
		t.Errorf("after switching back on = %q at %d, want %q at 0", out, st, "q\n")
	}
}

// TestSwitchingAPresentedNameIsNotRefused: the row #1117 measures. `enable`
// naming one of these answered `not a shell builtin` at 1, from the same
// missing table `type` was reading.
func TestSwitchingAPresentedNameIsNotRefused(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "enable p\n", presented)
	if out != "" || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q status %d, want a silent 0", out, errs, st)
	}

	// The listing names it, and names a builtin proper beside it so the
	// assertion is about the prelude rather than about an empty listing.
	out, _, st = preludeListingRun(t, listingPrelude, "enable\n", presented)
	for _, want := range []string{"enable p\n", "enable q\n", "enable pwd\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("listing = %q, want %q in it", out, want)
		}
	}
	if strings.Contains(out, "enable __helper\n") || st != 0 {
		t.Errorf("listing %q status %d, want the prelude's machinery left out at 0", out, st)
	}
}

// TestAScriptsFunctionShadowsAPresentedNameRatherThanReplacingIt: the line
// between the two tables, drawn where real bash draws it. Measured
// 2026-09-20 on 5.3.20: after `pushd() { echo mine; }`, `type pushd` is the
// function, `declare -f pushd` writes its body, `compgen -A builtin pushd`
// still answers `pushd`, and `type -a pushd` writes the function and then
// the builtin. A shell with one function table has to reconstruct that, and
// a rule keyed on the live table instead of on the prelude's record would
// get the last two rows wrong in a way nothing else asks about.
func TestAScriptsFunctionShadowsAPresentedNameRatherThanReplacingIt(t *testing.T) {
	for _, tc := range []struct{ src, out string }{
		{"p() { echo mine; }\ntype p\n", "p is a function\n"},
		{"p() { echo mine; }\ncompgen -A builtin p\n", "p\nprintf\npwd\n"},
		{"p() { echo mine; }\np\n", "mine\n"},
		{"p() { echo mine; }\ntype -a p\n", "p is a function\np is a shell builtin\n"},
	} {
		out, errs, st := preludeListingRun(t, listingPrelude, tc.src, presented)
		if out != tc.out || errs != "" || st != 0 {
			t.Errorf("%q = %q / %q at %d, want %q and a silent 0", tc.src, out, errs, st, tc.out)
		}
	}
}
