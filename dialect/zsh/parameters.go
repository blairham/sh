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
//	local (in a call) scalar-local                  local before everything
//	typeset -ir "     integer-local-readonly        but the type
//	PATH              scalar-tied-export-special
//	options           association-hide-hideval-special
//	typeset -h        scalar-hide                   the two hiding letters
//	typeset -H        scalar-hideval                are two attributes
//	typeset -hH       scalar-hide-hideval           hide first
//	typeset -hU (a)   array-unique-hide             and both after unique
//
// The container wins over the numeric attribute, which is why the kind is one
// value rather than a set: `typeset -ia ia; ia=(1 2)` describes as `array`
// and says nothing about integers, and the float spelling does the same.
//
// One of zsh's attributes is deliberately never written here, because this
// shell does not record it and a word claiming one would be a measurement
// nobody made. There are none left: the `L`/`R`/`Z` width letters were the
// last of them, and they were absent here because nothing carried them across
// the seam rather than because the runner did not know — `interp` has held
// them in `fieldWidth` since the letters were implemented, so the word was
// missing from a fact the shell already had (#4504).
//
// The three letters are two words apart from the other attributes, and that
// is measured rather than a rendering choice: `-L` is `left`, `-R` is
// `right_blanks` and `-Z` is `right_zeros`, so the side and the *fill* are
// spelled as one word and a name carries one of the three. Written between
// `tied` and `lower`, which is where zsh puts them — measured 2026-09-26 on
// zsh 5.9.2 under `-f`, one row per neighbor: `scalar-local-left`,
// `scalar-left-unique`, `scalar-left-upper`, `scalar-left-readonly`,
// `scalar-left-export`, `scalar-left-hideval` and `integer-left`.
//
// `hideval` used to be in that list too, on the ground that it "always
// accompanies `hide` in zsh's own module parameters, so writing `hide` alone
// is the honest half rather than a guess at the pair". The evidence behind
// that was the `options` parameter, which carries both letters at once. It
// was wrong, and the case that separates them is a parameter given **only**
// `-H`: it describes as `hideval` and not as `hide`, so the pair is not
// always a pair and `typeset -H` was recorded under the wrong one of the two
// (#2042). They are two letters, two attributes and two words now, written in
// the order `options` shows them.
//
// `special` is written for a parameter this
// shell provides — one that produces its value, or one registered as absent —
// which is narrower than zsh's, where `HOME` and `IFS` are special as well.
// Under-reporting an attribute is the failure this can afford; naming a type
// wrongly is not, because `array*` is what a caller switches on.
func zshParametersView(r *interp.Runner) interp.AssocArray {
	names := r.ParameterNames()
	out := make(interp.AssocArray, len(names))
	for _, name := range names {
		if a, ok := r.ParameterAttributes(name); ok {
			out[name] = interp.Scalar(describeParameter(a))
		}
	}
	return out
}

// zshParameterValue is `${parameters[PATH]}`: the one key, without naming
// and sorting every parameter in the shell to reach it.
//
// Both halves of the view's condition, in the same order: a name it yields,
// and attributes for it. The first is not redundant — [interp.Runner.ParameterAttributes]
// answers for an *absent* parameter, one registered to refuse by name, and
// the listing has no key for one. Dropping it would make `${parameters[x]}`
// report a parameter that `${(k)parameters}` says is not there.
func zshParameterValue(r *interp.Runner, name string) (string, bool) {
	if !r.ParameterIsNamed(name) {
		return "", false
	}
	a, ok := r.ParameterAttributes(name)
	if !ok {
		return "", false
	}
	return describeParameter(a), true
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
		{a.Local, "local"},
		{a.Justified == interp.LeftJustified, "left"},
		{a.Justified == interp.RightJustified && !a.ZeroFilled, "right_blanks"},
		// Not `RightJustified &&`, which is the pair that says the fill is a
		// letter of its own here rather than a flavor of the right-hand side:
		// a name carrying `-L` and `-Z` both is `scalar-left-right_zeros` on
		// zsh 5.9.2, measured 2026-09-27 (#4798).
		{a.ZeroFilled, "right_zeros"},
		{a.Lower, "lower"},
		{a.Upper, "upper"},
		{a.Readonly, "readonly"},
		// **Between the freeze and the tie**, which is measured rather than
		// slotted in beside the letter it comes from. Measured 2026-09-27 on
		// zsh 5.9.2 from a script file under `env -i PATH=/usr/bin:/bin`
		// with a scratch `HOME`, one run per cell, `typeset -t` paired with
		// every other letter that reaches a word:
		//
		//	typeset -t -L5 v   scalar-left-tag
		//	typeset -t -Z5 v   scalar-right_zeros-tag
		//	typeset -t -l  v   scalar-lower-tag
		//	typeset -t -u  v   scalar-upper-tag
		//	typeset -t -r  v   scalar-readonly-tag
		//	typeset -t     PATH  scalar-tag-tied-export-special
		//	typeset -t     path  array-tag-tied-special
		//	typeset -t -x  v   scalar-tag-export
		//	typeset -t -U -a v scalar… array-tag-unique
		//	typeset -t -h  v   scalar-tag-hide
		//	typeset -t -H  v   scalar-tag-hideval
		//
		// and the whole of it in one name, which is what says the position
		// is a single place in the sequence rather than a rule per pair:
		// `typeset -t -x -r -u -H -h -L5 v` is
		// `scalar-left-upper-readonly-tag-export-hide-hideval`.
		{a.Traced, "tag"},
		// **After the freeze and after the width and case words, and before
		// everything below it** — not straight after `local`, which is where
		// it used to stand (#4856). The position is a measurement and not a
		// reading of the letter table: measured 2026-09-27 on zsh 5.9.2 from
		// a script file under `env -i PATH=/usr/bin:/bin`, over a script tie
		// — `typeset -T TT tt` — so that `special` is out of the way, and
		// again over `path` and `PATH` with it in:
		//
		//	typeset -l tt    array-lower-tied          was array-tied-lower
		//	typeset -u tt    array-upper-tied          was array-tied-upper
		//	typeset -r tt    array-readonly-tied       was array-tied-readonly
		//	typeset -L5 tt   array-left-tied           was array-tied-left
		//	typeset -R5 tt   array-right_blanks-tied   was array-tied-right_…
		//	typeset -Z5 tt   array-right_zeros-tied    was array-tied-right_…
		//	typeset -x tt    array-tied-export         unchanged
		//	typeset -U tt    array-tied-unique         unchanged
		//	typeset -h tt    array-tied-hide           unchanged
		//	typeset -H tt    array-tied-hideval        unchanged
		//	typeset -ar path array-readonly-tied-special
		//	typeset -Z5 PATH scalar-right_zeros-tied-export-special
		//
		// The four unchanged rows are what makes this one word moved rather
		// than a different list: with nothing that outranks it present the
		// two shells already agreed, so the words below `tied` were in the
		// right place all along. `local` stays in front of it — measured,
		// `f() { typeset -T AA aa; typeset -r aa; ${(t)aa} }` is
		// `array-local-readonly-tied`.
		{a.Tied, "tied"},
		{a.Exported, "export"},
		{a.Unique, "unique"},
		{a.Hidden, "hide"},
		{a.HideValue, "hideval"},
		{a.Provided, "special"},
	} {
		if attr.on {
			b.WriteByte('-')
			b.WriteString(attr.word)
		}
	}
	return b.String()
}
