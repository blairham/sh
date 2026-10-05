// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// An emulation's effect on the option table, compiled.
//
// What `emulate MODE` does to the table depends on three things only: the
// mode, whether it is the `-R` form, and — for four names — a state the
// invocation decided. The first two are known before any script runs, so
// the walk applyEmulation used to take over all 185 names on every call,
// asking emulationClasses and emulationDefaults about each by name, is
// taken once per (mode, form) here instead, and a call applies the result.
// `emulate -L zsh` opens nearly every function in a zsh plugin, and on the
// maintainer's real configuration (2026-10-05) a start ran it 6,204 times
// (#6100).
//
// The plan is the same answer the walk gave, in three parts:
//
//   - **The recorded names** are a mask and a value over
//     interp.DialectOptions. The mask is every name the form resets —
//     recorded or not, since a name such as `aliases` keeps its state in the
//     same bits and an emulation dropped it there too before setting it
//     below — and the value is the names whose default in this mode is a
//     deviation from the table's. Merging the two is "drop every reset name,
//     then write back the ones this mode's default moves off the table's",
//     which is what the walk did with a list.
//   - **The recordedOver names** are asked at the call. Their base is not
//     the table's default but whatever the invocation decided, so whether
//     the mode's default is a deviation can only be known then. Measured on
//     zsh 5.9.2, 2026-09-25, `zsh +Z -f -c`: the four recordedOver names
//     read `rcs` off, `hashdirs` off, `login` off and `zle` off in that
//     shell, and after `emulate -R zsh` — or `-R sh`, `-R ksh`, `-R csh`,
//     which agree — `rcs` and `hashdirs` read **on** while `login` and `zle`
//     stay off. The last two are the control: both are in
//     emulationNeverReset, so neither is in any plan, and a change that put
//     every recordedOver name back would be wrong about them. A bare
//     `emulate sh` leaves all four, since the two that move are in
//     emulationStrictReset rather than in the 81.
//   - **Every other name the form resets** is set to the mode's default
//     through its own setter, in table order, exactly as the walk set it:
//     these are the names something reads, and their setters move axes,
//     switches and the grammar rather than a bit.
type emulationPlan struct {
	mask, value interp.DialectOptions
	// over is the recordedOver names this form resets, by index, with the
	// mode's default for each.
	over []planned
	// set is every other name this form resets, by index, with the mode's
	// default for each, in table order.
	set []planned
}

// planned is one name and the state a plan gives it.
type planned struct {
	i  int
	on bool
}

// emulationPlans is every plan for the four modes this shell knows, both
// forms, filled in by init below — which runs after the table is complete.
var emulationPlans map[string]*[2]emulationPlan

func init() {
	emulationPlans = make(map[string]*[2]emulationPlan, len(emulations))
	for mode := range emulations {
		emulationPlans[mode] = &[2]emulationPlan{compilePlan(mode, false), compilePlan(mode, true)}
	}
}

// planFor is the plan for one mode and form. A mode outside the four is
// compiled on the spot, so the answer for it is still the walk's — the
// table's own default for every name, which is what emulationDefault gives a
// mode it has no column for.
func planFor(mode string, strict bool) *emulationPlan {
	form := 0
	if strict {
		form = 1
	}
	if p, ok := emulationPlans[mode]; ok {
		return &p[form]
	}
	p := compilePlan(mode, strict)
	return &p
}

// compilePlan takes the walk once.
func compilePlan(mode string, strict bool) emulationPlan {
	var p emulationPlan
	for i := range zshOptions {
		o := zshOptions[i]
		if !resetByEmulation(o.base, strict) {
			continue
		}
		p.mask.Set(i, true)
		switch {
		case o.recorded && o.over != nil:
			p.over = append(p.over, planned{i, emulationDefault(o, mode)})
		case o.recorded:
			if dev, known := emulationDeviates(o, mode); known && dev {
				p.value.Set(i, true)
			}
		case o.set != nil:
			p.set = append(p.set, planned{i, emulationDefault(o, mode)})
		}
	}
	return p
}

// apply puts the plan on the runner.
func (p *emulationPlan) apply(r *interp.Runner) {
	opts := r.DialectOptions
	opts.Merge(&p.value, &p.mask)
	for _, o := range p.over {
		// A deviation from this shell's base, which is the bit's meaning for
		// a recordedOver name.
		opts.Set(o.i, o.on != zshOptions[o.i].over(r))
	}
	r.DialectOptions = opts
	for _, o := range p.set {
		_ = zshOptions[o.i].set(r, o.on)
	}
}
