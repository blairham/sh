// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The two readings of `${!…}` the panel does not agree about, either side of
// the flag that carries the indirection itself.

// oneParam is the expansion a source's first command carries, which is what
// each row below asserts on.
func oneParam(t *testing.T, src string, d syntax.Dialect) *syntax.ParamExpr {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	e, ok := firstParamExpr(f)
	if !ok {
		t.Fatalf("no parameter expansion in %q", src)
	}
	return e
}

// indirectionDialect is the core with `${!x}` and nothing else, so each row
// below says what its own flag does rather than what a shell happens to have.
func indirectionDialect() syntax.Dialect {
	d := syntax.Core()
	d.ParamIndirection = true
	return d
}

// TestThePrefixListingIsItsOwnFlag: `${!name@}` and `${!name*}` are a
// spelling of their own, and a dialect may have the indirection without them.
//
// bash and ksh93 have both. zsh's `emulate ksh` has the indirection and
// refuses the prefix form — measured, `${!ZQ_@}` is `bad substitution` there
// while `${!x}` in the same script reads. See
// [syntax.Dialect.ParamIndirectionPrefixListing].
func TestThePrefixListingIsItsOwnFlag(t *testing.T) {
	t.Parallel()
	with := indirectionDialect()
	with.ParamIndirectionPrefixListing = true
	without := indirectionDialect()

	for _, src := range []string{`echo ${!ZQ_@}`, `echo ${!ZQ_*}`} {
		e := oneParam(t, src, with)
		if e.Prefix == 0 || !e.Indirect {
			t.Errorf("%s with the flag: prefix %q indirect %v, want the listing",
				src, e.Prefix, e.Indirect)
		}
		// Without it the `@` is not the listing's, and what is left is an
		// expansion this grammar cannot read — which is the reference's
		// `bad substitution` rather than a parse that ended.
		e = oneParam(t, src, without)
		if e.Prefix != 0 {
			t.Errorf("%s without the flag: prefix %q, want none", src, e.Prefix)
		}
		if !e.Bad {
			t.Errorf("%s without the flag: want the expansion marked unreadable", src)
		}
	}
	// The control, and it is what says the flag is the *spelling* and not the
	// sigil: the plain indirection reads in both dialects.
	for _, d := range []syntax.Dialect{with, without} {
		e := oneParam(t, `echo ${!ZQ_}`, d)
		if !e.Indirect || e.Prefix != 0 || e.Bad {
			t.Errorf("${!ZQ_} = %+v, want a plain indirection", e)
		}
	}
}

// TestTheIndirectionRefusalIsCarriedPastTheParse: a dialect without the
// construct either ends the parse or carries the refusal to the run, and the
// panel splits on which.
//
// BusyBox ash says `syntax error: bad substitution`, which is its wording for
// a parse failure. zsh says `bad substitution` where the words are *read* —
// measured with `set -n`, `echo "${!x}"` is refused and `{ echo "${!x}"; }` is
// silent at 0, exactly as `${9nope}` is in both positions. See
// [syntax.Dialect.IndirectionRefusedAtExpansion].
func TestTheIndirectionRefusalIsCarriedPastTheParse(t *testing.T) {
	t.Parallel()
	carried := syntax.Core()
	carried.IndirectionRefusedAtExpansion = true

	// Carried: the file parses and the expansion is marked unreadable, which
	// is what lets a `${!x}` in a branch nothing takes cost nothing.
	e := oneParam(t, `echo "${!x}"`, carried)
	if !e.Bad || e.Indirect {
		t.Errorf("carried: %+v, want an unreadable expansion and no indirection", e)
	}
	if _, err := syntax.Parse(`if false; then echo "${!x}"; fi`, carried); err != nil {
		t.Errorf("carried: a branch nothing takes still ended the parse: %v", err)
	}

	// And ended, which is where it was for every dialect before the flag: the
	// same text is a parse failure.
	if _, err := syntax.Parse(`echo "${!x}"`, syntax.Core()); err == nil {
		t.Error("ended: want a parse failure without the flag")
	}

	// The control both halves need: a dialect that *has* the construct reads
	// it either way, so neither flag is standing in for the indirection.
	for _, d := range []syntax.Dialect{indirectionDialect(), carried} {
		d.ParamIndirection = true
		if e := oneParam(t, `echo "${!x}"`, d); !e.Indirect || e.Bad {
			t.Errorf("with the indirection: %+v, want it read", e)
		}
	}
}
