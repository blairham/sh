// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// Re-selecting a module parameter over a name the script has since assigned,
// measured 2026-09-11 on zsh 5.9.2 (#1981).
//
// The values already agreed before this: what did not was that the shell says
// nothing and reports 0 where zsh writes two sentences, reports 2, and leaves
// the feature deselected.

const restoreRefused = "zmodload zsh/parameter\n" +
	"zmodload -F zsh/parameter -p:parameters\n" +
	"parameters=(a b)\n"

// Two sentences and not one, and the second names the *module* — a location
// this builtin writes nowhere else.
func TestReSelectingAParameterOverAnAssignedNameIsRefused(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		restoreRefused+"zmodload -F zsh/parameter +p:parameters\necho st=$?\n")
	for _, want := range []string{
		"Can't add module parameter `parameters': parameter already exists",
		":zsh/parameter:", "error when adding parameter `parameters'",
		"st=2\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q (status %d), want %q in it", out, st, want)
		}
	}
	// The first sentence is the shell's and carries no name in the location,
	// where the second carries the module's. Told apart by what stands
	// between the two colons of the location.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Can't add module parameter") && strings.Contains(line, "zsh/parameter:") {
			t.Errorf("first line = %q, want the shell speaking with no name in the location", line)
		}
	}
}

// The refusal is a refusal: the selection does not happen, and the listing
// afterwards still writes the feature off.
func TestARefusedReSelectionLeavesTheFeatureOff(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		restoreRefused+"zmodload -F zsh/parameter +p:parameters\nzmodload -lF zsh/parameter\n")
	if !strings.Contains(out, "-p:parameters\n") {
		t.Errorf("output = %q, want the feature still deselected", out)
	}
	if strings.Contains(out, "+p:parameters\n") {
		t.Errorf("output = %q, want no selection to have gone through", out)
	}
}

// It is per feature, and the rest of the line still happens.
func TestTheRestOfTheSelectionStillHappens(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zmodload zsh/parameter\n"+
			"zmodload -F zsh/parameter -p:parameters -p:functions\n"+
			"parameters=(a b)\n"+
			"zmodload -F zsh/parameter +p:parameters +p:functions\n"+
			"echo st=$?\n"+
			"zmodload -lF zsh/parameter\n")
	if !strings.Contains(out, "st=2\n") {
		t.Errorf("output = %q, want 2", out)
	}
	if !strings.Contains(out, "+p:functions\n") || !strings.Contains(out, "-p:parameters\n") {
		t.Errorf("output = %q, want functions back and parameters still off", out)
	}
}

// A plain load widens a narrowed module, so it is the same restore and meets
// the same refusal — which is why the guard is not in the `-F` path alone.
func TestAPlainLoadMeetsTheSameRefusal(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		restoreRefused+"zmodload zsh/parameter\necho st=$?\nzmodload -lF zsh/parameter\n")
	if !strings.Contains(out, "error when adding parameter `parameters'") ||
		!strings.Contains(out, "st=2\n") {
		t.Errorf("output = %q, want the same two sentences and 2", out)
	}
	if !strings.Contains(out, "-p:parameters\n") {
		t.Errorf("output = %q, want the module still narrowed by that one feature", out)
	}
}

// The question is whether the name holds a value *now*, and `unset` frees it:
// the producer goes back in silence at 0 and answers again.
func TestAnUnsetNameIsFreeAgain(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		restoreRefused+"unset parameters\n"+
			"zmodload -F zsh/parameter +p:parameters\necho st=$?\n"+
			"print $(( ${#parameters} > 2 ))\n")
	if strings.Contains(out, "error when adding parameter") || !strings.Contains(out, "st=0\n") {
		t.Errorf("output = %q (status %d), want a silent restore at 0", out, st)
	}
	if !strings.Contains(out, "1\n") {
		t.Errorf("output = %q, want the producer answering again", out)
	}
}

// And a feature that was never deselected is not a restore at all: selecting
// it again over a name that holds a value says nothing.
func TestSelectingAFeatureThatIsAlreadyOnIsNotARestore(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/parameter\nzmodload -F zsh/parameter +p:parameters\necho st=$?\n")
	if out != "st=0\n" || st != 0 {
		t.Errorf("output = %q (status %d), want nothing and 0", out, st)
	}
}
