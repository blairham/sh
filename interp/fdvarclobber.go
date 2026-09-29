// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// fdVarWouldBeClobbered reports whether `set -C` protects the name a `{var}`
// redirection is about to write a descriptor number into.
//
// One shell reads the name first and refuses when the number already there is
// one it has open: the redirection does not happen, the file is not opened,
// the name keeps its value, and the command reports 1. Measured 2026-09-29 on
// zsh 5.9.2, script files with standard input on the null device:
//
//	setopt noclobber; exec {m}>f1; exec {m}>f2
//	    can't clobber parameter m containing file descriptor 11, st 1, m=11
//
// **The name is what is protected, not the file.** Hold the file fixed and
// vary what the name holds and the answer moves with the name every time:
//
//	m holds 11, open by this shell   refused, naming 11
//	m=0 or m=2                       refused, naming 0 or 2 — a named stream
//	                                 is open too, and the message says so
//	m= (empty)                       refused, naming 0: an empty value reads
//	                                 as descriptor zero
//	m=hello                          allowed — not a number, so not a
//	                                 descriptor, and the name is overwritten
//	m=-1                             allowed, for the same reason
//	m=77, nothing open there         allowed
//	m unset                          allowed
//	the descriptor closed first      allowed
//
// And the file is not what decides it: the same file twice is refused, two
// different files are refused, and neither is created.
//
// **It is not the clobber rule with a different subject.** `>|`, the override
// that exists to defeat `set -C` on a file, does not defeat this one —
// measured, `exec {m}>|f2` is refused in the same words. Nor is it the
// operator: `>`, `>>` and a redirection on an ordinary command rather than
// `exec` are all refused alike.
//
// zsh alone. bash 5.3.20 and ksh93u+ allocate a second descriptor and
// overwrite the name in silence; dash and BusyBox ash have no `{name}` form at
// all. See Semantics.NoclobberProtectsAnFdVariable.
//
// **The readonly refusal wins where both apply**, and it has to be said here
// rather than left to fall out. A frozen name holding an open descriptor is
// `can't allocate file descriptor to readonly parameter m` in the reference,
// not this sentence — and the readonly check runs *later*, at the store, so
// putting this one in front of the open reversed the order and took that row
// with it. It was pinned, and it moved the wrong way the moment this went in.
// Hence the test below: a frozen name is not this question's to answer.
// **This predicate does not consult the axis**, and that is what keeps the
// question at the disagreement. The shells really do differ here, so the
// axis is asked rather than read — but only once the shape that divides them
// holds. Deciding from the axis instead would have asked nothing; asking
// before the shape holds would refuse on every `{var}` redirection, which is
// the "ask only at the disagreement" rule broken in the other direction.
func (r *Runner) fdVarWouldBeClobbered(ref string) (fd int, clobbered bool) {
	if ref == "" || !r.noclobber || r.readonly[ref] {
		return 0, false
	}
	v, set := r.fdVarValue(ref)
	if !set {
		return 0, false
	}
	// Empty reads as zero, which is measured rather than assumed: `m=;
	// setopt noclobber; exec {m}>f` is refused naming descriptor 0, where
	// `m=hello` is allowed. So an empty value is a number and a word is not.
	n := 0
	if v != "" {
		var ok bool
		if n, ok = atoi(v); !ok {
			return 0, false
		}
	}
	if n < 0 {
		return 0, false
	}
	// The three named streams are open without being in the table — the
	// table holds what the shell opened for itself — which is why `m=2` is
	// refused there as surely as a number this shell allocated.
	if n <= 2 {
		return n, true
	}
	if _, open := r.fds[n]; open {
		return n, true
	}
	return 0, false
}
