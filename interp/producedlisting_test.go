// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
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

// listedRows keeps the rows this file is about. A bare listing writes the
// runner's own presets too — IFS, OPTIND, PPID, PWD, TMPDIR — and those are
// not what any of these assert, so a whole-output comparison would fail on a
// preset being added and say nothing about a produced parameter.
func listedRows(out string, names ...string) string {
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		for _, name := range names {
			if strings.HasSuffix(line, " "+name) || strings.Contains(line, " "+name+"=") {
				kept = append(kept, line)
				break
			}
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "\n") + "\n"
}

// The listing with **no operands** is a second question, and the panel
// answers it differently from the named one in the same shell: bash writes
// `declare -i RANDOM` where its own `declare -p RANDOM` writes
// `declare -i RANDOM="16735"`. See Semantics.ProducedParameterListing.
func TestAListingWithNoOperandsNamesTheProducedParameters(t *testing.T) {
	for _, tc := range []struct {
		name    string
		listing ProducedListing
		want    string
	}{{
		name:    "the name and the letters, and no reading",
		listing: ProducedListingNameOnly,
		want:    "declare -i X\ndeclare -- v=\"1\"\n",
	}, {
		name:    "the name and what the producer gives now",
		listing: ProducedListingWithValue,
		want:    "declare -i X=\"5\"\ndeclare -- v=\"1\"\n",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			set := func(s *Semantics) { s.ProducedParameterListing = tc.listing }
			out, errs, st := declRunWith(t, "v=1; typeset -p", set, Diagnostics{}, nil,
				producing("X", "5", &ProducedDeclaration{Integer: true}))
			out = listedRows(out, "X", "v")
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("stdout %q stderr %q status %d, want %q at 0", out, errs, st, tc.want)
			}
		})
	}
}

// The ordinary name beside it is the control, and it is in both rows above
// for a reason: an implementation that answered NameOnly by dropping every
// value from the listing would pass a row holding the produced name alone.
func TestNameOnlyReachesOnlyTheProducedParameter(t *testing.T) {
	set := func(s *Semantics) { s.ProducedParameterListing = ProducedListingNameOnly }
	out, _, st := declRunWith(t, "v=1; typeset -p v", set, Diagnostics{}, nil,
		producing("X", "5", &ProducedDeclaration{Integer: true}))
	if want := "declare -- v=\"1\"\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}

// A produced parameter a dialect has not said how to list stays out of the
// bare listing exactly as it stays out of the named one — the registration is
// the opt-in, which is what keeps a produced table of a hundred entries from
// arriving in `typeset -p` because somebody registered a reader for it.
func TestAnUnregisteredProducerIsNotInTheBareListing(t *testing.T) {
	set := func(s *Semantics) { s.ProducedParameterListing = ProducedListingWithValue }
	out, _, st := declRunWith(t, "v=1; typeset -p", set, Diagnostics{}, nil,
		producing("X", "5", nil))
	out = listedRows(out, "X", "v")
	if want := "declare -- v=\"1\"\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}

// `unset` takes the name out of the bare listing too, and by the same route
// the named `-p` uses: a listing that reached the producer anyway would draw
// a reading out of a parameter that is gone.
func TestAnUnsetProducedParameterLeavesTheBareListing(t *testing.T) {
	set := func(s *Semantics) { s.ProducedParameterListing = ProducedListingWithValue }
	out, _, st := declRunWith(t, "v=1; unset X; typeset -p", set, Diagnostics{}, nil,
		producing("X", "5", &ProducedDeclaration{Integer: true}))
	out = listedRows(out, "X", "v")
	if want := "declare -- v=\"1\"\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}

// A script that has assigned to the name gets its own value back, and gets it
// through the tables the walk already collects from rather than through the
// produced merge — so the name-only reading does not reach it.
func TestAStoredValueWinsOverTheProducedRow(t *testing.T) {
	set := func(s *Semantics) {
		s.ProducedParameterListing = ProducedListingNameOnly
		s.AssignmentRestoresAnUnsetProducedParameter = No
	}
	out, _, st := declRunWith(t, "unset X; X=abc; typeset -p", set, Diagnostics{}, nil,
		producing("X", "5", &ProducedDeclaration{Integer: true}))
	out = listedRows(out, "X")
	if want := "declare -i X=\"abc\"\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}

// The axis is asked once per listing and not once per name: a dialect that
// registers six producers and answers nothing would otherwise write six
// refusals for one command.
func TestAnUnansweredProducedListingRefusesOnce(t *testing.T) {
	two := func(r *Runner) {
		producing("X", "5", &ProducedDeclaration{Integer: true})(r)
		producing("Y", "6", &ProducedDeclaration{Integer: true})(r)
	}
	out, errs, st := declRunWith(t, "typeset -p", nil, Diagnostics{}, nil, two)
	if out != "" || st != 2 || strings.Count(errs, "\n") != 1 {
		t.Errorf("stdout %q stderr %q status %d, want one refusal at 2", out, errs, st)
	}
	if !strings.Contains(errs, "produced parameter") {
		t.Errorf("stderr %q does not name the axis", errs)
	}
}

// `export -p` and `readonly -p` narrow the same walk with a filter, and no
// produced parameter carries either attribute anywhere in the panel. They are
// left out rather than admitted and rejected, which is also what keeps them
// from asking an axis they would never use.
func TestTheFilteredListingsDoNotReachTheProducedParameters(t *testing.T) {
	for _, word := range []string{"export -p", "readonly -p"} {
		t.Run(word, func(t *testing.T) {
			// No ProducedParameterListing at all: reaching the axis here
			// would refuse at 2, which is what this asserts against.
			set := func(s *Semantics) {
				s.ExportListing = DeclareListingCommandWord
				s.ReadonlyListing = DeclareListingCommandWord
			}
			out, errs, st := declRunWith(t, "export v=1; readonly v; "+word, set,
				Diagnostics{}, nil, producing("X", "5", &ProducedDeclaration{Integer: true}))
			if strings.Contains(out, "X") || errs != "" || st != 0 {
				t.Errorf("stdout %q stderr %q status %d, want no produced row and no refusal", out, errs, st)
			}
		})
	}
}
