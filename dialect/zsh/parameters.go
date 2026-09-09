// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"

	"github.com/blairham/sh/interp"
)

// zshParametersView is `$parameters`: every parameter the shell has, to a
// hyphen-joined description of what it is.
//
// The type word comes first and is one of five. Then the attributes, in the
// order the combinations pin down — measured against zsh 5.9.2, each probe
// under `-c`:
//
//	typeset -x        scalar-export
//	typeset -r        scalar-readonly
//	typeset -xr       scalar-readonly-export        readonly before export
//	typeset -xrU (a)  array-readonly-export-unique  unique last
//	typeset -xrl      scalar-lower-readonly-export  case before readonly
//	typeset -xlZ      scalar-right_zeros-lower-export
//	typeset -T        scalar-tied / array-tied      tied first after the type
//	PATH              scalar-tied-export-special
//	options           association-hide-hideval-special
//
// The container wins over the numeric attribute, which is why the kind is one
// value rather than a set: `typeset -ia ia; ia=(1 2)` describes as `array`
// and says nothing about integers, and the float spelling does the same.
//
// Three of zsh's attributes are deliberately never written here, because this
// shell does not record them and a word claiming one would be a measurement
// nobody made: the `L`/`R`/`Z` padding attributes, `local`, and `hideval`.
// The first two are simply not tracked; `hideval` always accompanies `hide`
// in zsh's own module parameters, so writing `hide` alone is the honest half
// rather than a guess at the pair. `special` is written for a parameter this
// shell provides — one that produces its value, or one registered as absent —
// which is narrower than zsh's, where `HOME` and `IFS` are special as well.
// Under-reporting an attribute is the failure this can afford; naming a type
// wrongly is not, because `array*` is what a caller switches on.
func zshParametersView(r *interp.Runner) interp.AssocArray {
	names := r.ParameterNames()
	out := make(interp.AssocArray, len(names))
	for _, name := range names {
		if a, ok := r.ParameterAttributes(name); ok {
			out[name] = describeParameter(a)
		}
	}
	return out
}

// describeParameter is one parameter's word, and it is the whole of what a
// caller reads this table for.
func describeParameter(a interp.ParameterAttributes) string {
	var b strings.Builder
	switch a.Kind {
	case interp.AssocParameter:
		b.WriteString("association")
	case interp.ArrayParameter:
		b.WriteString("array")
	case interp.IntegerParameter:
		b.WriteString("integer")
	case interp.FloatParameter:
		b.WriteString("float")
	default:
		b.WriteString("scalar")
	}
	for _, attr := range []struct {
		on   bool
		word string
	}{
		{a.Tied, "tied"},
		{a.Lower, "lower"},
		{a.Upper, "upper"},
		{a.Readonly, "readonly"},
		{a.Exported, "export"},
		{a.Hidden, "hide"},
		{a.Unique, "unique"},
		{a.Provided, "special"},
	} {
		if attr.on {
			b.WriteByte('-')
			b.WriteString(attr.word)
		}
	}
	return b.String()
}
