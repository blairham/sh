// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// zsh's `enable` is not the other dialect's. These pin the differences that
// made it a builtin of its own rather than a flag on the shared one.

// TestEnableHasNoDashN, which is the whole reason the core's `enable` could
// not be reused: `-n` is what it means there and a bad option here.
func TestEnableHasNoDashN(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "enable -n cd\n")
	if !strings.Contains(out, "bad option: -n") {
		t.Errorf("output = %q, want the option refused", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	// And the builtin names itself in the location, which is this dialect's
	// habit and comes from being dispatched as one rather than from the
	// message saying so.
	if !strings.Contains(out, ":enable:") {
		t.Errorf("output = %q, want the builtin named in the location", out)
	}
}

// TestDisableSwitchesABuiltinOffAndEnableBringsItBack. Off is not removal:
// the same builtin comes back, which is what makes this a pair rather than
// an unregistration.
func TestDisableSwitchesABuiltinOffAndEnableBringsItBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "disable cd\ndisable\n")
	if strings.TrimSpace(out) != "cd" {
		t.Errorf("output = %q, want the disabled name listed", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}

	out, _ = runZsh(t, t.TempDir(), "disable cd\nenable cd\ndisable\n")
	if strings.TrimSpace(out) != "" {
		t.Errorf("output = %q, want nothing left disabled", out)
	}
}

// TestAListingIsBareNames, one per line, with no `enable ` in front of them —
// which is the other dialect's shape and would be wrong here.
func TestAListingIsBareNames(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "enable\n")
	if strings.Contains(out, "enable ") {
		t.Errorf("output = %q, want bare names", out)
	}
	if !strings.Contains(out, "\ncd\n") && !strings.HasPrefix(out, "cd\n") {
		t.Errorf("output = %q, want one name per line", out)
	}
}

// TestANameThatIsNotABuiltinIsAHashTableElement, and the complaint says so
// rather than calling it a command — which is what it is called in the
// dialects that have no such builtin.
func TestANameThatIsNotABuiltinIsAHashTableElement(t *testing.T) {
	for _, word := range []string{"enable", "disable"} {
		out, st := runZsh(t, t.TempDir(), word+" nosuchthing\n")
		if !strings.Contains(out, "no such hash table element: nosuchthing") {
			t.Errorf("%s: output = %q, want the hash table wording", word, out)
		}
		if st != 1 {
			t.Errorf("%s: status = %d, want 1", word, st)
		}
	}

	// Every operand is looked at, not just the first: the bad one is put
	// first on purpose, so a loop that gave up on it would never reach the
	// good one.
	out, st := runZsh(t, t.TempDir(), "disable nosuchthing cd\ndisable\n")
	if !strings.Contains(out, "cd") {
		t.Errorf("output = %q, want the good name still disabled", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want the last command's", st)
	}
}

// TestATableThisShellDoesNotKeepIsSaidOutLoud. `-a` and the rest name hash
// tables this shell has no answer for, and doing nothing quietly would leave
// the alias in place and report success.
func TestATableThisShellDoesNotKeepIsSaidOutLoud(t *testing.T) {
	for _, opt := range []string{"-a", "-f", "-m", "-r", "-s"} {
		out, st := runZsh(t, t.TempDir(), "disable "+opt+" foo\n")
		if !strings.Contains(out, "not implemented yet") {
			t.Errorf("%s: output = %q, want it said out loud", opt, out)
		}
		if st != 2 {
			t.Errorf("%s: status = %d, want 2", opt, st)
		}
	}
}
