// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// Every `set` option this shell has comes back, and the roster is read off
// the option tables rather than written down here.
//
// That is the whole of the test. What `local -` saves is a map built from
// commonSetOptions and whatever the dialect declared, so an option that grows
// a new field is saved by existing — but only while the *save* reads the
// tables too. A field-by-field copy would rot silently, and what it produces
// is an option quietly not restored, which nothing else here would notice.
//
// It is an in-package test because the roster is unexported and because the
// point is the roster: driving this through a script would measure the names
// a script happened to mention, which is the list this is meant not to have.
//
// pipefail is the case that proves it. Its *existence* is an axis older than
// the option table, so `set -o pipefail` never reaches an `apply` and the
// table alone cannot put it back — it is saved by name, and the sweep below
// covers it like everything else.
func TestLocalDashRestoresEverySetOption(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	// The dialect-declared names beside the substrate's own, so the sweep is
	// over more than one table.
	r.AddSetOptions("pipefail", "errtrace", "functrace", "braceexpand", "hashall", "posix", "onecmd", "debug")

	roster := optionRoster(r)
	if len(roster) < len(commonSetOptions) {
		t.Fatalf("roster is %d names, fewer than the %d common ones", len(roster), len(commonSetOptions))
	}

	// Started somewhere other than the zero value, and that is not tidiness:
	// with every option off to begin with, a save that forgot a name still
	// "restored" it, because the field it failed to write already held false.
	// pipefail is the name that was silently passing on exactly that.
	primeSomeOptions(r, roster)

	before := optionStates(r, roster)
	saved := r.saveShellOptions()

	moved := moveEveryOption(r, roster)
	if moved == 0 {
		t.Fatal("nothing moved, so a restore that agrees proves nothing")
	}

	saved.restore(r)

	after := optionStates(r, roster)
	for _, name := range roster {
		if before[name] != after[name] {
			t.Errorf("%s came back %v, was %v", name, after[name], before[name])
		}
	}
}

// primeSomeOptions turns a spread of the roster on before anything is saved,
// so that a name the save drops is caught in both directions rather than
// agreeing with a zero value by luck.
func primeSomeOptions(r *Runner, roster []string) {
	r.pipefail = true
	for i, name := range roster {
		if i%2 == 1 || name == "noexec" || name == "monitor" {
			continue
		}
		if o, ok := r.lookupSetOption(name); ok && o.apply != nil {
			o.apply(r, true)
		}
	}
}

// optionRoster is every name this runner has a state for.
func optionRoster(r *Runner) []string {
	var names []string
	for name := range commonSetOptions {
		names = append(names, name)
	}
	for name := range r.extraOptions {
		names = append(names, name)
	}
	return names
}

// optionStates reads the roster, pipefail included — which the table does not
// answer for, and which is therefore exactly the name a table-only check
// would miss in both directions at once.
func optionStates(r *Runner, roster []string) map[string]bool {
	states := map[string]bool{"pipefail": r.pipefail}
	for _, name := range roster {
		if o, ok := r.lookupSetOption(name); ok && o.get != nil {
			states[name] = o.get(r)
		}
	}
	return states
}

// moveEveryOption flips what it can and reports how many it moved, so a
// restore is only ever graded against a table that really changed.
//
// noexec is held out because it is one-way in every shell of the panel — the
// table refuses to turn it off, so putting it back is not a promise this or
// any shell here keeps — and monitor because it wants a terminal.
func moveEveryOption(r *Runner, roster []string) int {
	moved := 0
	for _, name := range roster {
		if name == "noexec" || name == "monitor" {
			continue
		}
		o, ok := r.lookupSetOption(name)
		if !ok || o.get == nil || o.apply == nil {
			continue
		}
		want := !o.get(r)
		o.apply(r, want)
		if o.get(r) == want {
			moved++
		}
	}
	// And the one the table cannot write, moved the same way the pipeline
	// code reads it.
	r.pipefail = !r.pipefail
	moved++
	return moved
}
