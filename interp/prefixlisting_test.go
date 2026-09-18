// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a listing with **no operand** makes of the running command's own
// assignment prefix — Semantics.PrefixInAWholeTableListing. Three answers,
// and the three names below are what need all three: one exported before the
// command, one set and not exported, one the prefix creates. Tests name the
// axis and never a shell; see interp/prefixlisting.go for the panel.

func wholeTableSem(p PrefixInAWholeTableListing) Semantics {
	s := testSemantics()
	s.PrefixInAWholeTableListing = p
	s.PrefixExportAtABuiltin = PrefixExportAtABuiltinOff
	s.DeclarationPromotesThePrefixEntry = No
	// The prefix has to be taken back for there to be two tables to tell
	// apart: where it persists, the name really is the shell's and every
	// listing agrees about it.
	s.AssignmentPrefixPersistsOnSpecialBuiltin = No
	return s
}

// The shell's own table: the value, the attributes and the existence the name
// had before the prefix. So the exported name lists its *old* value, the
// unexported one is in no export listing, and the one the prefix created is in
// no listing at all.
func TestAWholeTableListingCanAnswerFromBeforeThePrefix(t *testing.T) {
	t.Parallel()
	sem := wholeTableSem(PrefixInAWholeTableListingIsNotThere)
	out, errs := builtinPrefixRun(t, "export k=1\nk=9 export -p", sem)
	if !strings.Contains(out, `export k="1"`) || strings.Contains(out, `k="9"`) {
		t.Errorf("export -p = %q (err %q), want the value from before the prefix", out, errs)
	}
	out, _ = builtinPrefixRun(t, "m=1\nm=9 export -p", sem)
	if strings.Contains(out, "m=") {
		t.Errorf("export -p = %q, want an unexported name left out", out)
	}
	out, _ = builtinPrefixRun(t, "z=9 export -p", sem)
	if strings.Contains(out, "z=") {
		t.Errorf("export -p = %q, want a name the prefix created left out", out)
	}
	// And the *named* listing on the same vector still sees the prefix,
	// which is the row that says this is about the operand-less shape.
	out, _ = builtinPrefixRun(t, "export k=1\nk=9 export -p k", sem)
	if !strings.Contains(out, `k="9"`) {
		t.Errorf("export -p k = %q, want the prefix's value", out)
	}
}

// An ordinary entry: the prefix's value is written and the attribute tables
// say the rest, so the export listing follows what this vector's
// PrefixExportAtABuiltin just did.
func TestAWholeTableListingCanSeeThePrefixAsAnOrdinaryEntry(t *testing.T) {
	t.Parallel()
	sem := wholeTableSem(PrefixInAWholeTableListingIsAnOrdinaryEntry)
	sem.PrefixExportAtABuiltin = PrefixExportAtABuiltinUnchanged
	out, _ := builtinPrefixRun(t, "export k=1\nk=9 export -p", sem)
	if !strings.Contains(out, `export k="9"`) {
		t.Errorf("export -p = %q, want the prefix's value", out)
	}
	out, _ = builtinPrefixRun(t, "m=1\nm=9 export -p", sem)
	if strings.Contains(out, "m=") {
		t.Errorf("export -p = %q, want the attribute table to keep an unexported name out", out)
	}
}

// The command's environment: the prefix's value is written and the entry
// counts as exported however the tables answer — including for a name nothing
// ever exported, and with the attribute this vector has just taken off.
func TestAWholeTableListingCanSeeThePrefixAsTheCommandsEnvironment(t *testing.T) {
	t.Parallel()
	sem := wholeTableSem(PrefixInAWholeTableListingIsTheCommandsEnvironment)
	for _, row := range []struct{ src, want string }{
		{"export k=1\nk=9 export -p", `export k="9"`},
		{"m=1\nm=9 export -p", `export m="9"`},
		{"z=9 export -p", `export z="9"`},
	} {
		out, errs := builtinPrefixRun(t, row.src, sem)
		if !strings.Contains(out, row.want) {
			t.Errorf("%q = %q (err %q), want %q", row.src, out, errs, row.want)
		}
	}
	// And it is the **export** filter that admits it, not every filter: the
	// same prefix in front of a listing narrowed some other way writes
	// nothing for the name.
	out, _ := builtinPrefixRun(t, "m=1\nm=9 readonly -p", sem)
	if strings.Contains(out, "m=") {
		t.Errorf("readonly -p = %q, want the frozen names alone", out)
	}
}

// And a dialect with such a listing that has not chosen refuses by name.
func TestAWholeTableListingIsRefusedWhereTheDialectHasNotChosen(t *testing.T) {
	t.Parallel()
	sem := wholeTableSem(PrefixInAWholeTableListingUnspecified)
	_, errs := builtinPrefixRun(t, "export k=1\nk=9 export -p", sem)
	if !strings.Contains(errs, "prefix in a whole-table listing") {
		t.Errorf("stderr = %q, want the axis named", errs)
	}
}
