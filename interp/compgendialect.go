// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// SetCompgenAction installs one `compgen -A` action a *dialect* can generate
// and this package cannot, because what it lists is the dialect's own.
//
// bash's `shopt` is the case this was added for: the names belong to a builtin
// registered in dialect/bash, and interp has no business knowing any of them.
// Before this the action was refused as `compgen: shopt: not implemented`,
// which is what `shopt1.sub` in bash's own suite reads back.
//
// follows names the **core** action this one is generated after, so the
// dialect states its own place in the order rather than this package guessing
// at it. Measured 2026-09-23 on bash 5.3.15 with a prefix matching one of
// each, a function `exfunc`, a file `extfile` and a variable `extvar` present:
//
//	compgen -b -A shopt ex             exec, exit, export, then the shopt names
//	compgen -A function -A shopt ex    exfunc, then the shopt names
//	compgen -v -A shopt ex             the shopt names, then the variables
//	compgen -f -A shopt ex             the shopt names, then the file
//
// So bash generates `shopt` after `function` and before `variable`, and
// passing "function" here is that measurement written down rather than a
// position chosen to look tidy. An unknown or empty follows puts the action
// last, which is the only other answer that cannot reorder what is already
// there.
//
// A name this package does not recognize as an action at all is still refused
// as the script's typo — installing a generator does not make up a new `-A`
// name, it fills one bash already has.
func (r *Runner) SetCompgenAction(name, follows string, generate func(r *Runner, word string) []string) {
	if r.dialectCompgen == nil {
		r.dialectCompgen = map[string]func(*Runner, string) []string{}
	}
	r.dialectCompgen[name] = generate
	r.dialectCompgenAfter = append(r.dialectCompgenAfter, [2]string{name, follows})
}

// compgenGenerator is the function that generates one action's words, or nil
// where neither this package nor the dialect has one.
//
// The dialect is asked first, so a dialect that has a better answer than the
// core's for a shared action can give one. None does today; the order is the
// one every other extension point here uses, and the alternative would make an
// installed generator silently unreachable.
func (r *Runner) compgenGenerator(name string) func(*Runner, string) []string {
	if generate, ok := r.dialectCompgen[name]; ok {
		return generate
	}
	if generate, ok := compgenActions[name]; ok {
		return generate
	}
	return nil
}

// compgenOrder is the order the actions asked for are generated in, with the
// dialect's own inserted where it said it belonged.
//
// Built per call rather than cached: the list is six or seven names long, a
// completion is not a hot path, and a cache would have to be invalidated by
// every SetCompgenAction — which is the kind of bookkeeping that goes wrong
// long before it pays for itself.
func (r *Runner) compgenOrder() []string {
	if len(r.dialectCompgenAfter) == 0 {
		return compgenActionOrder
	}
	order := slices.Clone(compgenActionOrder)
	for _, pair := range r.dialectCompgenAfter {
		name, follows := pair[0], pair[1]
		if slices.Contains(order, name) {
			continue
		}
		if at := slices.Index(order, follows); at >= 0 {
			order = slices.Insert(order, at+1, name)
			continue
		}
		order = append(order, name)
	}
	return order
}
