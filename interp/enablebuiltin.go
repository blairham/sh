// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"sort"
)

// `enable` turns a shell's own commands off and on.
//
// Two of the panel have it and two do not, so it is registered here and taken
// away by the dialects without it, the way `let` already is.
//
// What it is *for* in bash is loading builtins from a shared object, which
// this shell cannot do and refuses rather than pretends. What scripts
// actually use it for is the other half: making sure a name runs the shell's
// own command rather than a function or something on PATH. Homebrew's `brew`
// opens with `builtin enable compgen unset` for exactly that reason.
//
// Turning one off is real here: the name stops resolving to a builtin and is
// looked up on PATH like any other word. It is kept in a set of its own
// rather than by unregistering, so that enabling it again gets back whatever
// was there — a dialect may have replaced a builtin with its own, and
// forgetting that would quietly restore the core's.
func init() {
	builtins["enable"] = biEnable
}

func biEnable(r *Runner, _ context.Context, args []string) int {
	known := "anps"
	if r.ask(r.sem().EnableUnloadsABuiltin, "`enable -d` naming the operand rather than the letter") {
		known = "adnps"
	}
	rest, opts, code := r.builtinOptions("enable", args, known)
	if code != 0 {
		return code
	}
	off := containsByte(opts, 'n')
	if len(rest) == 0 {
		// `-d` with no name adds nothing to the listing: measured
		// 2026-09-18, the shell with the letter writes byte for byte what a
		// bare `enable` writes. So it is read and then falls through here
		// like any other letter that named nothing.
		r.listBuiltins(off, containsByte(opts, 's'))
		return 0
	}
	if containsByte(opts, 'd') {
		if r.restricted {
			// Taking a builtin away is how a restricted shell would be
			// handed the disk back: with `cd` unloaded the name falls to a
			// PATH search, and the mode's refusal of it goes with it.
			// Measured, and it is the builtin that is named rather than the
			// letter or the operand — `enable: restricted` at 1.
			//
			// The other spellings are not refused, which is measured too:
			// `enable name` and `enable -n name` are silent at 0 in a
			// restricted shell, because neither adds or removes anything.
			return r.restrictedRefusal("enable")
		}
		return r.unloadBuiltins(rest)
	}
	status := 0
	for _, name := range rest {
		if !r.isBuiltin(name) {
			r.diagf("enable: %s: not a shell builtin\n", name)
			status = 1
			continue
		}
		if off {
			if r.disabledBuiltins == nil {
				r.disabledBuiltins = map[string]bool{}
			}
			r.disabledBuiltins[name] = true
			continue
		}
		delete(r.disabledBuiltins, name)
	}
	return status
}

// listBuiltins writes the names, one to a line and in order.
//
// `-s` narrows it to the ones POSIX marks special, and it narrows whichever
// listing was asked for: measured on bash 5.3.15, a name switched off with
// `-n` leaves the plain listing and appears in the `-n` one, and `-n -s`
// prints the disabled names that are special and nothing else. So this is an
// intersection rather than a third listing.
func (r *Runner) listBuiltins(disabled, special bool) {
	names := r.BuiltinNames()
	if disabled {
		names = nil
		for name := range r.disabledBuiltins {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	if special {
		kept := names[:0:0]
		for _, name := range names {
			if specialBuiltins[name] {
				kept = append(kept, name)
			}
		}
		names = kept
	}
	// The command that would put it back the way it is, which is the form
	// the one shell with this builtin prints — so a listing can be fed
	// straight back in.
	form := "enable %s\n"
	if disabled {
		form = "enable -n %s\n"
	}
	for _, name := range names {
		r.printf(form, name)
	}
}

// isBuiltin reports whether a name is one of this shell's own commands,
// whether or not it is switched on. A name that was turned off is still a
// builtin, which is what lets `enable` turn it back on.
func (r *Runner) isBuiltin(name string) bool {
	if r.disabledBuiltins[name] {
		return true
	}
	if _, ok := r.lookupBuiltin(name); ok {
		return true
	}
	// A name the dialect's prelude presents is one of the shell's own
	// commands: `enable pushd` is silent at 0 in real bash and was
	// `enable: pushd: not a shell builtin` at 1 here, which is the same
	// wrong answer `type` gave from the same missing table (#1117).
	return r.presentedPreludeName(name)
}

func containsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// unloadBuiltins is `enable -d`: take away a builtin that was loaded from a
// file, of which this shell has none and can have none.
//
// The letter is read rather than refused, and that is the whole of what this
// answers. Nothing here is loadable — there is no `enable -f`, and a shared
// object is not a thing a Go program opens — so every name reaches one of two
// refusals and neither is ever wrong for the shell it is asked of:
//
//	a name that is not a builtin at all   `enable: NAME: not a shell builtin`
//	a builtin that came from nowhere      `enable: NAME: not dynamically loaded`
//
// Both at status 1, which is the builtin's own and not the 2 a refused option
// carries. Measured 2026-09-18, and the discriminator between the two rows is
// that they exist at all: a shell that read the letter and then answered
// `not a shell builtin` for `echo` would be saying something false about its
// own command.
//
// The sentences are written here rather than in Diagnostics because the
// letter is one dialect's and so is the wording — see
// Semantics.EnableUnloadsABuiltin, which is what a shell without the letter
// answers, and `enable: NAME: not a shell builtin` above, which is spelled
// the same way for the same reason.
func (r *Runner) unloadBuiltins(names []string) int {
	status := 0
	for _, name := range names {
		if !r.isBuiltin(name) {
			r.diagf("enable: %s: not a shell builtin\n", name)
		} else {
			r.diagf("enable: %s: not dynamically loaded\n", name)
		}
		status = 1
	}
	return status
}
