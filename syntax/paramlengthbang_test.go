// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A length over the special name `!`. The core reads it — `${#!}` is a
// `ParamExpr{Name: "!", Length: true}` — and one grammar has no such shape,
// where the same characters are a bad substitution deferred to the run.
//
// The two boundary rows are what make this a rule about the one name rather
// than about lengths or about specials: `${#$}` and `${#?}` are lengths under
// both values of the flag, and `$!` at the front of an expansion is untouched
// by it. Without them a mutant that refused every special behind a `${#`, or
// one that refused `$!` anywhere, would pass (#2415).
func TestALengthOverTheBangName(t *testing.T) {
	d := Core()
	e := firstParam(t, `echo ${#!}`, d)
	if e.Bad || !e.Length || e.Name != "!" {
		t.Errorf("core: read as %+v, want a length over `!`", e)
	}

	d.ParamLengthRefusesTheBangName = true
	if e := firstParam(t, `echo ${#!}`, d); !e.Bad {
		t.Errorf("refusing the bang name: read as %+v, want a bad substitution", e)
	}
	// The neighbours the flag must leave alone.
	for _, src := range []string{`echo ${#$}`, `echo ${#?}`, `echo ${#-}`} {
		if e := firstParam(t, src, d); e.Bad || !e.Length {
			t.Errorf("%s: read as %+v, want a length", src, e)
		}
	}
	// And a bare `$!` is still the parameter, with no length in front of it.
	if e := firstParam(t, `echo ${!}`, d); e.Bad || e.Length || e.Name != "!" {
		t.Errorf("${!}: read as %+v, want the parameter `!`", e)
	}
}
