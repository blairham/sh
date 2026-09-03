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
	rest, opts, code := r.builtinOptions("enable", args, "anps")
	if code != 0 {
		return code
	}
	off := containsByte(opts, 'n')
	if len(rest) == 0 {
		r.listBuiltins(off)
		return 0
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
func (r *Runner) listBuiltins(disabled bool) {
	names := r.BuiltinNames()
	if disabled {
		names = nil
		for name := range r.disabledBuiltins {
			names = append(names, name)
		}
		sort.Strings(names)
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
	_, ok := r.lookupBuiltin(name)
	return ok
}

func containsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}
