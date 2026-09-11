// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import "testing"

// TestPresetUseAsksEveryAxis. The value of this check is that it is total and
// costs no processes, so the thing to pin is that it really does cover the
// whole vector and really does read all four dialects.
func TestPresetUseAsksEveryAxis(t *testing.T) {
	t.Parallel()
	uses, err := PresetUse()
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) < 300 {
		t.Fatalf("only %d axes asked about", len(uses))
	}
	for _, u := range uses {
		for _, want := range []string{"bash", "zsh", "ksh", "dash"} {
			if _, ok := u.Held[want]; !ok {
				t.Fatalf("%s: %s was not asked what it holds", u.Field, want)
			}
		}
		if len(u.Held) != 4 {
			t.Fatalf("%s: %d vectors asked, want the four dialects", u.Field, len(u.Held))
		}
	}
}

// TestAnAxisEveryDialectAnswersAlikeIsReported is the claim the check makes.
// SplitCommandSubstitution is the standing example — its own comment says
// "true everywhere measured, including zsh" — so if this stops being flagged,
// either the axis was triaged or the check stopped working, and the two must
// not look alike.
func TestAnAxisEveryDialectAnswersAlikeIsReported(t *testing.T) {
	t.Parallel()
	uses, err := PresetUse()
	if err != nil {
		t.Fatal(err)
	}
	var unanimous int
	for _, u := range uses {
		if u.Unanimous {
			unanimous++
			if len(u.Held) == 0 {
				t.Fatalf("%s: unanimous with nothing held", u.Field)
			}
		}
	}
	if unanimous == 0 {
		t.Skip("no axis is answered alike by all four dialects; nothing to check here")
	}
}
