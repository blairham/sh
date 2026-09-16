// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// `float`, which is the declaration builtin under a third name with the
// floating-point attribute already decided — the same shape `integer` has one
// letter along, and registered through the same seam for the same reason.
//
// zsh alone in the panel has it as a builtin. ksh93 has the *word*, as the
// preset alias `typeset -lE`, which this tree already spells that way; bash,
// dash and BusyBox ash answer `float: not found`. So which dialects have it is
// a table and not an axis.
//
// The default attribute is `E` and not `F`: measured 2026-09-16 on zsh 5.9.2
// under `-f`, `float c=1.5; typeset -p c` is `typeset -E c=1.500000000e+00`,
// which is byte for byte what `typeset -E c=1.5` lists. A written letter wins
// over the name's — `float -F 2 d=1.5` lists `typeset -F d=1.50` — which is
// why the attribute here is only added where the word carried neither.
//
// It is not a second implementation of anything: a float declaration is
// Runner.declareNames exactly as `typeset -E` is, so the attribute, the
// precision a later assignment renders with, the readonly refusal and every
// letter's meaning come from the one place.

// FloatBuiltin is the `float` builtin, for a dialect that has the word to
// register.
//
// Exported for the same reason [IntegerBuiltin] is: a dialect package cannot
// reach an unexported function, and the alternative is a second copy of the
// declaration in `dialect/zsh`.
func FloatBuiltin() Builtin { return biFloat }

func biFloat(r *Runner, _ context.Context, args []string) int {
	name := r.inBuiltin
	if name == "" {
		name = "float"
	}
	known := r.sem().FloatOptions
	args, f, code := r.parseDeclareFlags(name, args, known)
	if code != 0 {
		// The same fatality `typeset` and `integer` have, from the same
		// axis: a bad letter here is a bad letter to the declaration under
		// another word.
		if r.ask(r.sem().TypesetBadOptionFatal, "a bad `typeset` option ending the script") {
			r.status = code
			r.fatalUsageQuiet()
		}
		return code
	}
	if len(args) == 0 {
		// A listing of the float variables, which this engine does not build
		// for `integer` either — see interp/integerbuiltin.go for why it
		// refuses by name rather than falling through to the whole variable
		// table.
		r.diagf("%s: a listing is not implemented yet\n", r.builtinComplaintName(name))
		return 2
	}
	// The name asked for the attribute, and the letter the *company* rules
	// read has to be in the set for the same reason `integer` puts its `i`
	// there: the word carried it, so a pair that one dialect refuses must be
	// visible under this spelling as well as under `typeset`'s.
	//
	// Only where the word carried neither float letter. `float -F 2 d=1.5`
	// is `typeset -F d=1.50` in zsh 5.9.2, so a written `F` decides the
	// rendering and this must not put an `E` beside it.
	if !strings.ContainsRune(f.letters, 'E') && !strings.ContainsRune(f.letters, 'F') {
		f.float, f.floatExponent = true, true
		f.letters += "E"
		f.letterSigns += "-"
	}
	return r.declareNames(name, args, f)
}
