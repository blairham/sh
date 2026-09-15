// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// `local -` is the operand that is not a name: it asks that the shell's
// option table be put back the way it was when the function returns.
//
// Measured 2026-09-15, `env -i PATH=/usr/bin:/bin`, over `-c` and a script
// file alike, with `f() { local -; set -f; }; f; echo $-`:
//
//	bash 5.3.15    the `f` is gone     the options are restored
//	dash           the `f` is gone     the same, though `local` takes no
//	                                   letters there at all
//	BusyBox ash    the `f` is gone     the same
//	bash 3.2.57    refused as a bad name, and the option survives — the
//	               form arrived between the two builds
//	zsh 5.9.2      `-` is a *parameter* there, so `local -` is a declaration
//	               of it and a different construct entirely
//	ksh93          no `local` builtin, so the form cannot be written
//
// Three of the four shells that have `local` agree, and the fourth does
// something else with the same two characters — which is a conflict rather
// than a gap, so it is an axis. See
// Semantics.LocalDashSavesTheShellOptions.
//
// What is saved is the `set` table and not `shopt`. Measured the same day on
// bash: `f() { local -; shopt -s nullglob; }; f; shopt nullglob` still reads
// `on`, while `f() { local -; set -o pipefail; }; f` leaves `pipefail off`.
// So it is exactly the names `set -o` lists, including the ones that have no
// letter, and nothing beyond them.

// shellOptions is the `set` table as it stood, ready to be put back.
//
// A map of name to state rather than a copy of the fields, so that an option
// added to the tables is saved by existing: the alternative is a struct
// listing every field, which is a list that rots silently and whose failure
// is an option quietly not restored. TestLocalDashRestoresEverySetOption is
// the guard — it moves every name this shell lists and requires the listing
// to come back — and it is a test over the tables rather than over a copy of
// them.
type shellOptions struct {
	was      map[string]bool
	pipefail bool
}

// saveShellOptions records the state of every `set` option this shell has.
//
// Read through the table's own `get` for the same reason the listing is:
// several of these are not plain fields — the editing mode is one state
// under two names, and command tracking answers from the startup letters
// until something has moved it — so a field copy would record something the
// listing does not agree with.
func (r *Runner) saveShellOptions() shellOptions {
	s := shellOptions{was: make(map[string]bool, len(commonSetOptions)+len(r.extraOptions))}
	for name := range commonSetOptions {
		s.record(r, name)
	}
	for name := range r.extraOptions {
		s.record(r, name)
	}
	// pipefail is the one real state the table cannot write: its *existence*
	// is an axis older than the table, so `set -o pipefail` never reaches an
	// `apply` and the entry is there to put it in the listings. Saved and
	// put back directly, and the guard test covers it like any other name.
	s.pipefail = r.pipefail
	return s
}

func (s shellOptions) record(r *Runner, name string) {
	o, ok := r.lookupSetOption(name)
	if !ok || o.get == nil {
		// A name with no live state behind it cannot have moved, so there
		// is nothing to put back. Those are the ones this shell does not
		// act on, whose `on` field is the whole answer.
		return
	}
	s.was[name] = o.get(r)
}

// restore puts the table back, writing only the names that moved.
//
// Only the ones that moved, because several of these `apply` functions do
// more than write a field — command tracking records that the state has been
// spoken for, and the POSIX-mode one moves a whole vector — so re-applying a
// state the shell is already in is not free and not always a no-op.
func (s shellOptions) restore(r *Runner) {
	for name, want := range s.was {
		o, ok := r.lookupSetOption(name)
		if !ok || o.get == nil || o.get(r) == want {
			continue
		}
		switch {
		case o.apply != nil:
			o.apply(r, want)
		case o.try != nil:
			// The one request a dialect can refuse — `set -m` needs a
			// terminal in two of the panel. A refusal here leaves the option
			// where it is, which is the same thing the script's own `set`
			// would have got, and the status of a refusal nobody asked for
			// is not this function's to report.
			_ = o.try(r, want, name)
			r.setOptionStatus = 0
		}
	}
	r.pipefail = s.pipefail
}

// localDashOperands takes the `-` operands out of a `local` command and says
// whether one was there.
//
// A whole operand and never a prefix: `local -x` is the export letter and
// `local -- x` is the end of the options, and neither is this. It may appear
// beside names — measured, `f() { local - x=2; }` declares `x` and saves the
// options both — so it is removed from the list rather than ending it.
func localDashOperands(args []string) ([]string, bool) {
	found := false
	kept := make([]string, 0, len(args))
	for _, a := range args {
		if a == "-" {
			found = true
			continue
		}
		kept = append(kept, a)
	}
	return kept, found
}

// saveOptionsForThisCall arranges for the option table to be put back when
// the running function returns.
//
// Registered on the scope rather than run at a fixed depth, so that it
// unwinds with everything else the call saved — a `return`, a failure and a
// `set -e` stop all go through the same place, and a nested call that asks
// for it again gets its own.
func (r *Runner) saveOptionsForThisCall() {
	sc := r.ownScope()
	if sc == nil {
		// No scope, or the innermost one belongs to an enclosing shell: a
		// subshell inside a function body shares its caller's stack, and a
		// restore hung there would put the *caller's* table back. The same
		// reading `trap` takes of the same stack — see localtraps.go — and
		// for the same reason.
		return
	}
	saved := r.saveShellOptions()
	sc.onReturn = append(sc.onReturn, func() { saved.restore(r) })
}
