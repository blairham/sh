// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// How a produced parameter lists back. A listing walks tables a producer is in
// none of, so `typeset -p` reported one as missing from a name the same shell
// answers an expansion for one line earlier (#2451) — a listing and an
// expansion giving two answers to whether a name exists.
//
// These name the seam and no shell: what letters each dialect writes is
// asserted under dialect/bash, dialect/zsh and dialect/ksh.

// producing registers a produced scalar answering a fixed value, with
// whatever the dialect would have said about listing it.
func producing(name, value string, d *ProducedDeclaration) func(*Runner) {
	return func(r *Runner) {
		r.SetDynamic(name, func(*Runner) string { return value })
		r.SetDynamicWriter(name, func(*Runner, string) {})
		if d != nil {
			r.SetDynamicDeclaration(name, *d)
		}
	}
}

func TestAProducedParameterListsAsTheDialectSaysItDoes(t *testing.T) {
	// The clustered form is bash's and the export-spelled one is what ksh93
	// and zsh write; a base rides on the letter only in the second, and the
	// float letter is only written there, so each row asks the form that has
	// something to say about it.
	clustered := func(s *Semantics) { s.DeclarePrintReportsAMissingName = Yes }
	spelled := func(s *Semantics) {
		s.DeclarePrintReportsAMissingName = Yes
		s.DeclareListing = DeclareListingExportSpelled
	}
	for _, tc := range []struct {
		name      string
		set       func(*Semantics)
		d         *ProducedDeclaration
		out, errs string
		status    int
	}{{
		name:   "unregistered is not there",
		set:    clustered,
		d:      nil,
		errs:   "testsh: typeset: X: not found\n",
		status: 1,
	}, {
		name: "no letters",
		set:  clustered,
		d:    &ProducedDeclaration{},
		out:  "declare -- X=\"5\"\n",
	}, {
		name: "an integer",
		set:  clustered,
		d:    &ProducedDeclaration{Integer: true},
		out:  "declare -i X=\"5\"\n",
	}, {
		name: "an integer with a base on the letter",
		set:  spelled,
		d:    &ProducedDeclaration{Integer: true, Base: 10},
		out:  "typeset -i10 X=\"5\"\n",
	}, {
		name: "a float",
		set:  spelled,
		d:    &ProducedDeclaration{Float: true},
		out:  "typeset -F X=\"5\"\n",
	}, {
		// The third answer: the name is known and the listing writes
		// nothing for it, at 0. Neither a row nor a refusal.
		name: "silent",
		set:  clustered,
		d:    &ProducedDeclaration{Silent: true},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRunWith(t, "typeset -p X", tc.set, Diagnostics{}, nil, producing("X", "5", tc.d))
			if out != tc.out || errs != tc.errs || st != tc.status {
				t.Errorf("stdout %q stderr %q status %d, want %q / %q / %d", out, errs, st, tc.out, tc.errs, tc.status)
			}
		})
	}
}

// The value comes from the producer at the moment of the listing, which is
// what makes the row worth anything: a listing that wrote the name with no
// value would report a parameter whose value is empty, and one that wrote a
// stored value would report a value nothing here holds.
func TestAListedProducedParameterCarriesTheProducedValue(t *testing.T) {
	out, _, st := declRunWith(t, "typeset -p X", nil, Diagnostics{}, nil,
		producing("X", "drawn", &ProducedDeclaration{}))
	if want := "declare -- X=\"drawn\"\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}

// A name `unset` has taken away is not listed however it was registered: a
// read of it answers nothing, and a listing that reached the producer anyway
// would draw a value out of a parameter that is gone.
func TestAnUnsetProducedParameterIsNotListed(t *testing.T) {
	answered := func(s *Semantics) { s.DeclarePrintReportsAMissingName = Yes }
	out, errs, st := declRunWith(t, "unset X; typeset -p X", answered, Diagnostics{}, nil,
		producing("X", "5", &ProducedDeclaration{Integer: true}))
	if out != "" || errs != "testsh: typeset: X: not found\n" || st != 1 {
		t.Errorf("stdout %q stderr %q status %d, want the name reported missing", out, errs, st)
	}
}

// The declaration says how the name lists and changes nothing else. It is not
// the integer *attribute*: a script can still assign whatever it likes, and
// the producer keeps answering.
func TestAProducedDeclarationIsNotAnAttribute(t *testing.T) {
	out, _, st := declRunWith(t, `X=abc; echo "[$X]"`, nil, Diagnostics{}, nil,
		producing("X", "5", &ProducedDeclaration{Integer: true}))
	if want := "[5]\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0 — the letter is a listing's and not an arithmetic fold", out, st, want)
	}
}
