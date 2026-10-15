// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// nameFilter answers "was this name ever registered?" with no for nearly every
// name that was not, and never with no for one that was.
//
// The shell registers its own names in seven tables a store has to consult —
// a kind fixed for the slot, a guard and an action on assignment, a writer
// for a produced scalar and one for a produced array, a tie, and a parameter
// waiting to arrive. In zsh every one of them holds names, so asking each
// about an ordinary `x` was seven hashed lookups that all answered no, and in
// the plain store (interp/plainstore.go) they were most of what deciding a
// store cost (#6423).
//
// So every registration also sets two bits picked by the name's hash, and a
// name with either bit clear was never registered anywhere. It is a filter and
// not a set: a bit is never cleared, so a name that was registered once and
// taken away still reads "maybe", and the tables are asked as before. A false
// "maybe" costs the lookups it saves; a false "no" is impossible, because the
// only writes into those tables are the registrations that set the bits — and
// the two that put a writer back (putBack) restore a name that was registered
// before. A value rather than a map, so a clone has its own copy by being a
// copy of the runner.
type nameFilter [16]uint64

// nameFilterBits is the two bit positions name sets, from one FNV-1a hash.
func nameFilterBits(name string) (uint32, uint32) {
	h := uint32(2166136261)
	for i := 0; i < len(name); i++ {
		h ^= uint32(name[i])
		h *= 16777619
	}
	return h & 1023, (h >> 16) & 1023
}

// add records name.
func (f *nameFilter) add(name string) {
	a, b := nameFilterBits(name)
	f[a/64] |= 1 << (a % 64)
	f[b/64] |= 1 << (b % 64)
}

// mayHold reports whether name may have been recorded. False is certain.
func (f *nameFilter) mayHold(name string) bool {
	a, b := nameFilterBits(name)
	return f[a/64]&(1<<(a%64)) != 0 && f[b/64]&(1<<(b%64)) != 0
}

// registeredArrayWriter, registeredGuard and registeredAction are the store's
// three lookups in the tables the filter answers for, each asking the filter
// first: an ordinary name is in none of them, and a declaration or a full-road
// assignment of one paid three hashed lookups to learn it (#6373).
func (r *Runner) registeredArrayWriter(name string) (func(*Runner, []string), bool) {
	if !r.specialNames.mayHold(name) {
		return nil, false
	}
	w, ok := r.dynamicArrayWriters[name]
	return w, ok
}

func (r *Runner) registeredGuard(name string) (func(*Runner, string) (string, bool), bool) {
	if !r.specialNames.mayHold(name) {
		return nil, false
	}
	g, ok := r.assignmentGuards[name]
	return g, ok
}

func (r *Runner) registeredAction(name string) (func(*Runner, string), bool) {
	if !r.specialNames.mayHold(name) {
		return nil, false
	}
	a, ok := r.assignmentActions[name]
	return a, ok
}
