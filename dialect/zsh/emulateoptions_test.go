// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"
	"testing"
)

// TestTheEmulationPartitionCoversTheTable — the three measured lists are a
// partition of the option table: every name is in exactly one of them and no
// list names anything the table does not have.
//
// This is the guard the paired option-letter tables taught: a set written down
// as one list and consulted as another drifts silently, because a name left out
// of every list reads as `emulationKeeps` — which is a real answer for nine
// names and a wrong one for the rest. Both directions are checked, since a name
// added to a list and never to the table is the same fault seen from the other
// end.
func TestTheEmulationPartitionCoversTheTable(t *testing.T) {
	seen := map[string]int{}
	for _, list := range []string{emulationAlwaysReset, emulationStrictReset, emulationNeverReset} {
		for _, name := range strings.Fields(list) {
			seen[name]++
		}
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("%s is in %d of the three lists, want exactly one", name, n)
		}
		if _, _, ok := exactOptionName(name); !ok {
			t.Errorf("%s is in a list and not in the option table", name)
		}
	}
	for _, o := range zshOptions {
		if seen[o.base] == 0 {
			t.Errorf("%s is in the option table and in none of the three lists", o.base)
		}
	}
	// And the three sizes, which is what says a name has not quietly moved
	// from one list to another. Measured on zsh 5.9.2, 2026-09-12.
	for _, tc := range []struct {
		list string
		name string
		want int
	}{
		{emulationAlwaysReset, "a bare emulation resets", 81},
		{emulationStrictReset, "only `emulate -R` resets", 95},
		{emulationNeverReset, "no emulation resets", 9},
	} {
		if got := len(strings.Fields(tc.list)); got != tc.want {
			t.Errorf("%d names %s, want %d", got, tc.name, tc.want)
		}
	}
}

// TestTheStrictFormIsTheWiderSet — `-R` resets everything a bare emulation
// resets and more. Read off the classification rather than off the lists, so
// that a reordering of the data cannot make it vacuous.
func TestTheStrictFormIsTheWiderSet(t *testing.T) {
	wider := 0
	for _, o := range zshOptions {
		bare, strict := resetByEmulation(o.base, false), resetByEmulation(o.base, true)
		if bare && !strict {
			t.Errorf("%s: a bare emulation resets it and -R does not", o.base)
		}
		if strict && !bare {
			wider++
		}
	}
	if want := 95; wider != want {
		t.Errorf("-R reaches %d names a bare emulation does not, want %d", wider, want)
	}
}

// TestTheNineNobodyResetsAreTheShellsOwnState — spelled out rather than
// counted, because the list is short and each name is there for a stated
// reason: eight describe how the shell was started and `exec` cannot be
// measured from inside a shell at all.
func TestTheNineNobodyResetsAreTheShellsOwnState(t *testing.T) {
	for _, name := range []string{
		"exec", "interactive", "login", "monitor", "privileged",
		"restricted", "shinstdin", "singlecommand", "zle",
	} {
		if resetByEmulation(name, false) || resetByEmulation(name, true) {
			t.Errorf("%s is reset by an emulation, and it is measured as one that is not", name)
		}
	}
}

// TestEveryEmulationDefaultNamesAnOptionInAResetList — the value table and
// the set table are paired, and this is the pairing checked rather than
// assumed.
//
// Two directions. A name here that the option table does not have is a
// measurement about a shell we are not implementing; a name here that **no**
// emulation resets is a value nothing can ever read, which is dead data that
// looks exactly like a working entry.
func TestEveryEmulationDefaultNamesAnOptionInAResetList(t *testing.T) {
	for name := range emulationDefaults {
		if _, _, ok := exactOptionName(name); !ok {
			t.Errorf("%s has an emulation default and is not in the option table", name)
		}
		if !resetByEmulation(name, true) {
			t.Errorf("%s has an emulation default and no emulation resets it, so nothing can read it", name)
		}
	}
	// And the sizes, measured on zsh 5.9.2 2026-09-13 by reading every name
	// under `emulate -R` in each of the four modes: 62 differ from zsh's
	// default somewhere, three of those are compat spellings that resolve to
	// a canonical entry, and 49 of the remaining 59 are in the bare-reset 81.
	if got, want := len(emulationDefaults), 59; got != want {
		t.Errorf("%d names have an emulation default, want %d", got, want)
	}
	bare := 0
	for name := range emulationDefaults {
		if resetByEmulation(name, false) {
			bare++
		}
	}
	if want := 49; bare != want {
		t.Errorf("%d of them are reset by a bare emulation, want %d", bare, want)
	}
}

// TestNoRecordedOverNameHasAnEmulationDefault — the recorded store holds
// deviations from the *table's* default, and three names in the table hold a
// base state that is not it: `hashdirs`, `login` and `rcs`. A deviation
// computed against `o.def` would be the wrong bit for one of those, so
// emulationDeviates refuses to answer for a name it has no entry for and this
// is what keeps the two sets apart.
//
// Checked behaviorally rather than by looking for the constructor, because
// the constructor is not visible on the value: a `recorded` name reads back
// its own default in a fresh runner and a `recordedOver` one does not.
func TestNoRecordedOverNameHasAnEmulationDefault(t *testing.T) {
	r := optionStateRunner(t)
	for _, o := range zshOptions {
		if _, ok := emulationDefaults[o.base]; !ok {
			continue
		}
		if got := o.get(r); got != o.def {
			t.Errorf("%s has an emulation default and reads %v in a fresh shell where its table default is %v; "+
				"a name whose base state is not the table's cannot have its deviation computed against it",
				o.base, got, o.def)
		}
	}
}

// TestAnEmulationLeavesItsOwnDefaults — the behavior, with the controls #2515
// established: a name whose `sh` default differs from zsh's, a name whose
// `ksh` default differs from `sh`'s, and a name all four agree about.
//
// Measured on zsh 5.9.2, 2026-09-13:
//
//	option        zsh   sh    ksh   csh
//	multios       on    off   off   off
//	posixbuiltins off   on    on    off
//	kshglob       off   off   on    off
//	cshnullcmd    off   off   off   on
//	extendedglob  off   off   off   off
func TestAnEmulationLeavesItsOwnDefaults(t *testing.T) {
	for _, tc := range []struct {
		name              string
		zsh, sh, ksh, csh bool
	}{
		// Behavior behind it in all four columns, which is what makes a
		// wrong value a wrong shell rather than a wrong report.
		{"multios", true, false, false, false},
		{"posixbuiltins", false, true, true, false},
		{"globsubst", false, true, true, true},
		{"unset", true, true, true, false},
		// The `ksh` control: a name where sh and ksh part company, so an
		// implementation that folded the two sh-family modes into one would
		// fail here and nowhere else.
		{"kshglob", false, false, true, false},
		{"localoptions", false, false, true, false},
		{"bsdecho", false, true, false, false},
		// The `csh` control, which is also the mode that used to skip the
		// axis swap outright.
		{"cshnullcmd", false, false, false, true},
		{"globassign", false, false, false, true},
		// And a name all four agree about, so the table is doing nothing
		// here and the reset is still measured.
		{"extendedglob", false, false, false, false},
		{"nullglob", false, false, false, false},
	} {
		for _, mode := range []struct {
			name string
			want bool
		}{
			{"zsh", tc.zsh}, {"sh", tc.sh}, {"ksh", tc.ksh}, {"csh", tc.csh},
		} {
			o, _, ok := exactOptionName(tc.name)
			if !ok {
				t.Fatalf("%s is not in the option table", tc.name)
			}
			if got := emulationDefault(o, mode.name); got != mode.want {
				t.Errorf("emulate %s leaves %s at %v, want %v", mode.name, tc.name, got, mode.want)
			}
		}
	}
}
