// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A name a running script may not move, asked for by the words the shell was
// started with — Semantics.ImmovableOptionsSetAtInvocation, reached through a
// long *name* rather than through the `t` letter that already had it (#3221).
//
// The split is one shell's and it is measured: ksh93u+ 2012-08-01 answers
// `set: interactive: bad option(s)` at 2 to a script in both directions, and
// `ksh -o interactive` on its own command line prompts, runs the line and
// prompts again at status 0.
//
// The axis governs the refusal and not the applying, which is the second half
// here: a name the table records without acting on keeps the refusal it had,
// because granting it would trade the shell's own `bad option(s)` for a
// wording no column ever writes.
func invocationRunner(t *testing.T, atInvocation Answer) (*Runner, *strings.Builder) {
	t.Helper()
	s := PosixSemantics()
	s.ImmovableOptionsSetAtInvocation = atInvocation
	s.BadSetOptionNameFatal = No
	buf := new(strings.Builder)
	r := newTestRunner(t, &Runner{Stdout: buf, Stderr: buf, Semantics: &s, Name: "sh"})
	// `interactive` is the substrate's own row and has an apply; `recorded`
	// is declared and acted on by nothing, which is the shape `rc` and
	// `login_shell` have in the shell this is for.
	r.AddSetOptions("interactive", "recorded")
	r.AddImmovableSetOptions("interactive", "recorded")
	return r, buf
}

func TestAnImmovableNameIsTakenAtAnInvocation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		atInvocation Answer
		option       string
		on           bool
		want         int
		wantMoved    bool
		wording      string
	}{
		{"the name on", Yes, "interactive", true, 0, true, ""},
		{"the name off", Yes, "interactive", false, 0, false, ""},
		// The same request from a shell that does not split by route.
		{"no split declared", Unspecified, "interactive", true, 2, false, "invalid option name"},
		{"the split declined", No, "interactive", true, 2, false, "invalid option name"},
		// And a name with nothing to apply, which is refused either way and
		// in the *same words* — this is the row that says the grant is not
		// a blanket one. Without the wording the two refusals are one
		// status apiece and a shell that promoted every immovable name
		// would pass by writing `not implemented` instead.
		{"nothing to move", Yes, "recorded", true, 2, false, "invalid option name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, buf := invocationRunner(t, tc.atInvocation)
			r.Interactive = false
			if got := r.SetNamedOption(tc.option, tc.on); got != tc.want {
				t.Errorf("status %d, want %d", got, tc.want)
			}
			if r.Interactive != tc.wantMoved {
				t.Errorf("Interactive = %v, want %v", r.Interactive, tc.wantMoved)
			}
			if tc.wording == "" {
				if buf.String() != "" {
					t.Errorf("said %q, want nothing", buf.String())
				}
				return
			}
			if !strings.Contains(buf.String(), tc.wording) {
				t.Errorf("said %q, want %q in it", buf.String(), tc.wording)
			}
		})
	}
}

// And the same name from inside a script, which is the half the grant above
// could quietly take away. ApplyNamedOption is the seam a running shell uses.
func TestAnImmovableNameIsStillRefusedToAScript(t *testing.T) {
	for _, on := range []bool{true, false} {
		r, _ := invocationRunner(t, Yes)
		r.Interactive = false
		if got := r.ApplyNamedOption("interactive", on); got == 0 {
			t.Errorf("on=%v: status 0, want a refusal", on)
		}
		if r.Interactive {
			t.Errorf("on=%v: the script moved the name", on)
		}
	}
}

// The negative spelling of the interactivity name, which the shell this is
// for takes at an invocation and does not list. Measured 2026-09-16 on
// ksh93u+: `ksh -o nointeractive` draws no prompt and `ksh +o nointeractive`
// draws one, both at status 0, and its `set -o` listing names only
// `interactive`.
func TestTheNegativeInteractiveNameIsTheNameUpsideDown(t *testing.T) {
	for _, tc := range []struct {
		name      string
		on        bool
		wantMoved bool
	}{
		{"under a minus", true, false},
		{"under a plus", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := invocationRunner(t, Yes)
			r.Semantics.InteractiveOptionName = "interactive"
			r.Semantics.NonInteractiveOptionName = "nointeractive"
			r.Interactive = !tc.wantMoved
			if got := r.SetNamedOption("nointeractive", tc.on); got != 0 {
				t.Fatalf("status %d, want 0", got)
			}
			if r.Interactive != tc.wantMoved {
				t.Errorf("Interactive = %v, want %v", r.Interactive, tc.wantMoved)
			}
		})
	}
}

// It is refused to a script exactly as the name it negates is, which is one
// fact with two spellings rather than a gap in the refusal.
func TestTheNegativeInteractiveNameIsRefusedToAScript(t *testing.T) {
	for _, on := range []bool{true, false} {
		r, _ := invocationRunner(t, Yes)
		r.Semantics.InteractiveOptionName = "interactive"
		r.Semantics.NonInteractiveOptionName = "nointeractive"
		r.Interactive = false
		if got := r.ApplyNamedOption("nointeractive", on); got == 0 {
			t.Errorf("on=%v: status 0, want a refusal", on)
		}
	}
}

// And a preset that declares no negative spelling has no such name at all:
// without this the resolution above would be a rule about the characters `no`
// rather than a dialect's declaration.
func TestTheNegativeNameNeedsDeclaring(t *testing.T) {
	r, _ := invocationRunner(t, Yes)
	r.Semantics.InteractiveOptionName = "interactive"
	if got := r.SetNamedOption("nointeractive", true); got == 0 {
		t.Errorf("status 0, want the undeclared spelling refused")
	}
}
