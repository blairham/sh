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
		// And the parameter this name is a second spelling of, where it is
		// one. A restore is the option moving, so the tie has to hear it:
		// measured on bash 5.3.20, `IGNOREEOF=0; set -o ignoreeof;
		// f(){ local -; set +o ignoreeof; }; f; echo $IGNOREEOF` is **10**
		// there — the return turns the option back on and the tie writes the
		// parameter — and it was empty here, because the option went back
		// through the table directly and the tie was only wired to `set`
		// itself. See interp/tiedoption.go and #4163.
		r.optionTieMoved(name, want)
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
	sc.shellOptionsSaved = true
	sc.onReturn = append(sc.onReturn, func() { saved.restore(r) })
}

// localDashListingRow is the row a bare `local` writes for a save this call
// already made, or "" where there is none.
//
// The save is one of the call's locals as far as the listing is concerned,
// and it is written **first**, ahead of every name — measured 2026-09-21,
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME, from a script
// file, on bash 5.3.20:
//
//	f() { local -; local; }                     local -
//	g() { local -; local x=1; local; }          local -
//	                                            declare -- x="1"
//	g() { local x=1; local -; local; }          the same two lines, same order
//	g() { local -; local -; local x=1; local; } one `local -` row, not two
//	g() { local -; h() { local; }; h; }         nothing — the save is the
//	                                            outer call's, not h's
//
// So it is the running call's own save, exactly as the names are, and the
// row's own word is `local` rather than the `declare` every name carries:
// it is not a declaration of a parameter called `-`, it is the save saying
// it happened. Written here rather than through a wording field because only
// one column reaches it — the two other shells that save the options list
// nothing at all for a bare `local`, and the shell that lists every
// parameter reads `-` as a parameter name and never makes the save (#4048).
//
// Two neighboring shapes measured in the same run and deliberately left
// where they are: a whole-table `declare -p` inside the same call writes the
// row too, ahead of the table, and `local -p -` writes it where this shell
// answers `local: -: not found`. Both are the same fact reached through the
// declaration builtin's own listing rather than through this one, and
// neither is what the bare form is.
func (r *Runner) localDashListingRow() string {
	if len(r.scopes) == 0 || !r.scopes[len(r.scopes)-1].shellOptionsSaved {
		return ""
	}
	return "local -"
}
