// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// The `@` transformation family: `@` plus exactly one letter from a fixed
// set, riding on the ParamTransformations flag. Everything the shell that has
// the family refuses — no letter, two letters, a letter outside the set —
// stays a bad substitution where the flag is on, so the grammar accepts
// exactly what the runtime can answer.

func transformDialect() Dialect {
	d := Core()
	d.ParamTransformations = true
	return d
}

func TestTransformParsesEachLetter(t *testing.T) {
	for _, letter := range []byte("QEPAaKkLUu") {
		src := "echo ${x@" + string(letter) + "}"
		f, err := Parse(src, transformDialect())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		var found *ParamExpr
		walkParams(f, func(e *ParamExpr) { found = e })
		if found == nil {
			t.Fatalf("%q: no ParamExpr in the tree", src)
		}
		if found.Bad {
			t.Fatalf("%q: marked bad, want ParamTransform", src)
		}
		if found.Op != ParamTransform || found.Transform != letter {
			t.Errorf("%q: op %v transform %q, want ParamTransform %q",
				src, found.Op, found.Transform, letter)
		}
	}
}

func TestTransformNeedsItsFlag(t *testing.T) {
	// Off, the operator is unrecognized and defers like any other — the
	// dialects without the family call it a bad substitution when reached.
	f, err := Parse(`echo "${x@Q}"`, Core())
	if err != nil {
		t.Fatalf("the core grammar should defer, got %v", err)
	}
	var found *ParamExpr
	walkParams(f, func(e *ParamExpr) { found = e })
	if found == nil || !found.Bad {
		t.Fatal("want a Bad node in the core grammar")
	}
}

// The one grammar that refuses an unrecognized operator at parse time still
// defers the `@` family: measured, `${x@Q}` in a branch never taken is silent
// there and `${x^^}` in the same branch is a parse-time syntax error.
func TestTransformIsDeferredEvenByTheParseTimeRefuser(t *testing.T) {
	d := Core()
	d.BadSubstitutionAtParseTime = true

	f, err := Parse(`echo "${x@Q}"`, d)
	if err != nil {
		t.Fatalf("the @ family should defer to the run, got %v", err)
	}
	var found *ParamExpr
	walkParams(f, func(e *ParamExpr) { found = e })
	if found == nil || !found.Bad || found.Src != "x@Q" {
		t.Fatalf("want a Bad node keeping the raw text, got %+v", found)
	}

	// The contrast that shows it is the `@` and not the flag: another
	// unrecognized operator under the same grammar still fails while reading.
	if _, err := Parse(`echo "${x ~}"`, d); err == nil {
		t.Fatal("a non-@ bad operator should still be a parse error here")
	}
}

func TestTransformRefusesWhatTheShellRefuses(t *testing.T) {
	// Measured: `${x@}`, `${x@Z}`, `${x@QQ}` and `${x@ Q}` are all bad
	// substitutions in the shell that has the family, so the flag being on
	// must not start accepting them.
	for _, src := range []string{
		`echo "${x@}"`,
		`echo "${x@Z}"`,
		`echo "${x@QQ}"`,
		`echo "${x@ Q}"`,
	} {
		f, err := Parse(src, transformDialect())
		if err != nil {
			t.Fatalf("parse %q: %v — want a deferred Bad node", src, err)
		}
		var found *ParamExpr
		walkParams(f, func(e *ParamExpr) { found = e })
		if found == nil || !found.Bad {
			t.Errorf("%q: want a Bad node, got %+v", src, found)
		}
	}
}

// `${!name@}` is the prefix listing and `${!x@Q}` an operator on an
// indirection; the family arriving must not disturb the first or claim the
// second is anything else.
func TestTransformLeavesThePrefixListingAlone(t *testing.T) {
	d := transformDialect()
	d.ParamIndirection = true

	f, err := Parse(`echo "${!FOO_@}"`, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found *ParamExpr
	walkParams(f, func(e *ParamExpr) { found = e })
	if found == nil || found.Prefix != '@' || found.Op != ParamNone {
		t.Errorf("${!FOO_@} = %+v, want the prefix listing", found)
	}

	f, err = Parse(`echo "${!x@Q}"`, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	found = nil
	walkParams(f, func(e *ParamExpr) { found = e })
	if found == nil || !found.Indirect || found.Op != ParamTransform || found.Transform != 'Q' {
		t.Errorf("${!x@Q} = %+v, want a transform of an indirection", found)
	}
}

func TestTransformPrintsBackAsWritten(t *testing.T) {
	src := `echo "${x@Q}"`
	f, err := Parse(src, transformDialect())
	if err != nil {
		t.Fatal(err)
	}
	if printed := Print(f); !strings.Contains(printed, "${x@Q}") {
		t.Errorf("printed = %q, want the construct kept as written", printed)
	}
}
