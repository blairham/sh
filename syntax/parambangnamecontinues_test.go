// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// Where an indirection begins. The character standing immediately after `${!`
// decides whether the `!` is the *parameter* — with whatever follows it as an
// operator — or the first character of a name the expansion indirects through,
// and that decision is the dialect's.
//
// The set used to be written into the parser as `:-+=?`, which is one column's
// partition standing in for the panel's three. See
// [Dialect.ParamBangNameContinues] for the seventeen characters measured one
// at a time (#3966).
func TestWhereABangNameCarriesOn(t *testing.T) {
	t.Parallel()

	// The empty set: nothing but a name character carries on, so every
	// operator character leaves the `!` as the parameter.
	d := Core()
	d.ParamIndirection = true
	for _, tc := range []struct{ src, name string }{
		{`echo ${!#}`, "!"},
		{`echo ${!?}`, "!"},
		{`echo ${!%x}`, "!"},
		{`echo ${!:-w}`, "!"},
		{`echo ${!v}`, "v"},
		{`echo ${!_v}`, "_v"},
	} {
		e := firstParam(t, tc.src, d)
		if e.Bad || e.Name != tc.name {
			t.Errorf("empty set, %s: read as %+v, want the name %q", tc.src, e, tc.name)
		}
	}
	// A digit is on the far side of the same split, and with nothing carrying
	// on it is the parameter `!` with a stray character behind it.
	if e := firstParam(t, `echo ${!1}`, d); !e.Bad {
		t.Errorf("empty set, ${!1}: read as %+v, want a bad substitution", e)
	}

	// Widening it moves exactly those rows and nothing else.
	d.ParamBangNameContinues = "0123456789#?@*["
	for _, tc := range []struct{ src, name string }{
		{`echo ${!#}`, "#"},
		{`echo ${!?}`, "?"},
		{`echo ${!1}`, "1"},
		{`echo ${!@}`, "@"},
	} {
		e := firstParam(t, tc.src, d)
		if e.Bad || !e.Indirect || e.Name != tc.name {
			t.Errorf("wide set, %s: read as %+v, want an indirection through %q", tc.src, e, tc.name)
		}
	}
	// The characters the wide set does not name stay where they were: a
	// mutant that widened the test to every non-name character would pass
	// without these.
	for _, tc := range []struct{ src, name string }{
		{`echo ${!%x}`, "!"},
		{`echo ${!:-w}`, "!"},
		{`echo ${!-w}`, "!"},
	} {
		e := firstParam(t, tc.src, d)
		if e.Bad || e.Indirect || e.Name != tc.name {
			t.Errorf("wide set, %s: read as %+v, want the parameter %q", tc.src, e, tc.name)
		}
	}

	// And a bare `${!}` is the parameter under both, since there is no second
	// character for the set to be consulted about.
	for _, wide := range []string{"", "0123456789#?@*["} {
		d.ParamBangNameContinues = wide
		if e := firstParam(t, `echo ${!}`, d); e.Bad || e.Indirect || e.Name != "!" {
			t.Errorf("%q: ${!} read as %+v, want the parameter `!`", wide, e)
		}
	}
}

// A dotted name carries on after `${!` wherever [Dialect.DottedName] is on,
// without being named in the set: the two flags meet here, and `${!.}` is a
// name in the one grammar that has dotted names and a bad substitution
// everywhere else.
func TestADottedNameCarriesOnAfterABang(t *testing.T) {
	t.Parallel()
	d := Core()
	d.ParamIndirection = true
	if e := firstParam(t, `echo ${!.}`, d); !e.Bad {
		t.Errorf("without dotted names: read as %+v, want a bad substitution", e)
	}
	d.DottedName = true
	if e := firstParam(t, `echo ${!.}`, d); e.Bad || !e.Indirect || e.Name != "." {
		t.Errorf("with dotted names: read as %+v, want an indirection through `.`", e)
	}
}
