// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"fmt"
	"sort"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An emulation reaches the **grammar**, and this file is where it does it.
//
// `emulate MODE` moves the option table and the semantics vector, and until
// this seam existed it moved nothing about what the shell would *parse* — so
// a construct this shell has and the emulated mode has not was a refusal in
// the reference and a defined function here. `emulate -R sh -c 'd1() { print
// hi }'` is the shortest case, and the same snippet in a **script file** and
// under an `argv[0]` of `sh` moves too, which is what says the noun is the
// mode rather than the `-c` route (#4814).
//
// # The mechanism is the one two options already use
//
// A grammar that changes under a running shell is not a new idea here:
// `set -o posix` moves `AliasesExpandReservedWords` and `rcquotes` moves
// `DoubledQuoteInSingleQuotesIsALiteralQuote`, both by **replacing**
// `Runner.Dialect` rather than writing through it. The front end's run loop
// watches that pointer and re-reads the rest of the program with whatever it
// finds, so the next line of a script is read the new way — see
// `interp/setoptions.go`, `interp/doubledquote.go` and driver's run loop.
//
// Replacing rather than writing is load-bearing twice over. A subshell is a
// cloned runner holding the *same* pointer, so a script must not change the
// grammar of the shell that spawned it; and the pointer's identity is the
// whole signal the front end has.
//
// # Why the table is empty
//
// It is, today, and that is the point rather than an omission. `syntax.Dialect`
// has some 110 fields and **the emulated grammar and this shell's agree almost
// everywhere**, so a table keyed on a guessed subset comes back green over a
// wide corpus and moves nothing — the same failure a letter set made in #4518.
// Every field here is measured against the reference a mode at a time, by the
// row of #4814 that owns its family, and `make emulate-sweep` is what grades
// each row.
//
// A seam landed with a guessed table would also have made every later row
// unattributable: some of the 160 would have moved on the commit that built
// the place to put them, and no row could then say what its own change did.
//
// # Every axis answers every mode
//
// A mode missing from an axis would leave that field wherever the *previous*
// emulation left it, so `emulate sh; emulate zsh` would not be the shell it
// started as — a restore that silently does not restore. So an axis states an
// answer for every mode `emulations` knows, and grammarTableHoles is the check
// rather than the convention.
//
// # What this table may not hold
//
// **A field an option name already owns.** `rcquotes` and `extendedglob` reach
// `syntax.Dialect` through the option table, which knows each emulation's own
// default for every name it holds (see emulationDefaults), so a field named
// here as well would have two owners writing it in one call and the answer
// would be whichever ran last.

// grammarAxis is one question `syntax.Dialect` answers that an emulation moves.
type grammarAxis struct {
	// name is the `syntax.Dialect` field, for a report and for the check that
	// every mode is answered.
	name string

	// modes is every mode this axis states an answer for.
	modes []string

	// apply writes this mode's answer into d and reports whether that moved
	// anything. The bool is what decides whether the runner's dialect is
	// replaced at all: a replacement nobody needs makes the front end re-hand
	// the parser a language identical to the one it had.
	apply func(d *syntax.Dialect, mode string) bool
}

// emulationGrammar is the per-mode grammar table.
//
// Empty. See the file comment: the seam is #4815 and every field behind it is
// measured by a row of its own.
var emulationGrammar []grammarAxis

// grammarFlag is the ordinary shape of an axis — one bool field of
// `syntax.Dialect`, with the answer each mode holds for it.
//
// The field is reached through a function returning a pointer rather than by
// name, which is the arrangement setAxis already uses for the semantics
// vector: it is the compiler that checks the field exists and the spelling
// cannot drift from it.
func grammarFlag(name string, field func(*syntax.Dialect) *bool, byMode map[string]bool) grammarAxis {
	modes := make([]string, 0, len(byMode))
	for mode := range byMode {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	return grammarAxis{
		name:  name,
		modes: modes,
		apply: func(d *syntax.Dialect, mode string) bool {
			want, stated := byMode[mode]
			if !stated {
				// A mode the axis does not answer leaves the field alone.
				// grammarTableHoles is what stops that reaching a shell: a
				// silent carry-over from the previous emulation is the one
				// shape a restore cannot undo.
				return false
			}
			p := field(d)
			if *p == want {
				return false
			}
			*p = want
			return true
		},
	}
}

// emulateDialect is the grammar this mode reads with, given the one the shell
// is reading with now, and whether that is a different grammar at all.
//
// The dialect is taken and returned by value: the caller's job is to replace
// the runner's pointer with a copy, never to write through the one it holds.
func emulateDialect(d syntax.Dialect, mode string) (syntax.Dialect, bool) {
	moved := false
	for _, a := range emulationGrammar {
		if a.apply(&d, mode) {
			moved = true
		}
	}
	return d, moved
}

// setEmulationGrammar puts this mode's grammar on the runner.
//
// Nothing happens where the mode's answers are the ones already in force,
// which is every call while the table is empty — so this seam costs an
// emulation a walk of an empty slice and changes no verdict, which is exactly
// what #4815's bar asks of it.
func setEmulationGrammar(r *interp.Runner, mode string) {
	if r.Dialect == nil {
		// A runner with no dialect of its own parses nested input with the
		// core (see interp.Runner.dialect), and an emulation has no business
		// inventing one: the fields this table moves are read off whatever the
		// shell was reading with, and there is nothing here to read off.
		return
	}
	d, moved := emulateDialect(*r.Dialect, mode)
	if !moved {
		return
	}
	r.Dialect = &d
}

// grammarTableHoles names every (axis, mode) pair the table does not answer.
//
// It is a function rather than a test body so that the test can point it at a
// table with a hole in it and watch it fire: a check over a table that is
// empty today would otherwise pass by having nothing to look at, which is the
// same line of output as a check that passed.
func grammarTableHoles(axes []grammarAxis) []string {
	modes := make([]string, 0, len(emulations))
	for mode := range emulations {
		modes = append(modes, mode)
	}
	sort.Strings(modes)

	var holes []string
	for _, a := range axes {
		stated := make(map[string]bool, len(a.modes))
		for _, m := range a.modes {
			stated[m] = true
		}
		for _, mode := range modes {
			if !stated[mode] {
				holes = append(holes, fmt.Sprintf("%s does not answer %q", a.name, mode))
			}
		}
	}
	return holes
}
