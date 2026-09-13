// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"sort"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A bare declaration that writes the names carrying an attribute, and nothing
// about the ones that do not — BareLocalListsAttributedNames. Tests name the
// axis and never a shell; interp/attributephrase.go holds the measurement.

// withAttributePhrases is declRun's setter for a dialect whose bare
// declaration writes this listing. The letters come with it because a listing
// of attributes cannot be asked anything without the letters that make one.
func withAttributePhrases(s *Semantics) {
	s.DeclareOptions = "aAfilprux"
	s.BareTypesetListing = BareLocalListsAttributedNames
	s.DeclaredNameWithoutValueIsEmpty = Yes
	// The three neighbors a listing of numeric and case attributes cannot be
	// asked anything without. Each is its own axis with its own suite; here
	// they are set to the answer that leaves the attribute *on* the name, so
	// that a blank line in this listing means the renderer said nothing
	// rather than the letter having been dropped upstream of it.
	s.IntegerAttributeTakesABase = Yes
	s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = No
	s.TypesetLocalNeedsKeywordFunction = No
}

// phraseRun is declRun with the one row the framework itself puts in this
// listing taken back off.
//
// This suite is the only one that has to care, because it is the only one
// whose output is over **every name the shell holds**: the test framework
// hands each Runner a `TMPDIR` of its own so that nothing reaches the real
// one, and an exported name is exactly what this listing writes. So `export
// TMPDIR` stood in front of every table below. It is dropped here rather than
// written into each expectation, and dropping it is checked — a framework
// that stopped exporting the name would turn this red rather than quietly
// making every table one row shorter.
func phraseRun(t *testing.T, src string, set func(*Semantics)) (string, string, int) {
	t.Helper()
	out, errs, st := declRunEnv(t, src, set, Diagnostics{}, nil)
	const framework = "export TMPDIR\n"
	if !strings.Contains(out, framework) && !strings.Contains(out, "TMPDIR") {
		t.Fatalf("the framework no longer exports TMPDIR, so this suite is stripping nothing: %q", out)
	}
	return strings.Replace(out, framework, "", 1), errs, st
}

// The headline, and the control is in the same run: `plain` is assigned and
// never declared, so it is held and reachable and this listing says nothing
// about it. A form that wrote every parameter would put `plain` in the
// output; one that wrote the scope's own would leave `ex` out.
func TestAnAttributeListingWritesOnlyTheNamesThatCarryOne(t *testing.T) {
	src := strings.Join([]string{
		"plain=1",
		"typeset -x ex=2",
		"typeset -i n=3",
		"typeset -u up=q",
		"typeset -r ro=4",
		"typeset -A m",
		"typeset -a arr=(1 2)",
		"typeset -i16 h=255",
		"typeset -l lo=X",
		"typeset -xr both=1",
		"typeset -li long=7",
		"typeset -ui uns=7",
		"typeset",
	}, "\n")
	out, errs, st := phraseRun(t, src, withAttributePhrases)
	want := strings.Join([]string{
		"indexed array arr",
		"export readonly both",
		"export ex",
		"integer base 16 h",
		"tolower lo",
		"long integer long",
		"associative array m",
		"integer n",
		"readonly ro",
		"unsigned integer uns",
		"toupper up",
		"",
	}, "\n")
	if out != want || errs != "" || st != 0 {
		t.Errorf("a bare declaration = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The value is not there, and that is the form rather than an omission: the
// same name written through a listing that carries values says `n=3`.
func TestAnAttributeListingWritesNoValue(t *testing.T) {
	out, _, _ := phraseRun(t, "typeset -i n=3\ntypeset", withAttributePhrases)
	if strings.Contains(out, "3") {
		t.Errorf("the value reached the listing: %q", out)
	}
	out, _, _ = phraseRun(t, "typeset -i n=3\ntypeset", func(s *Semantics) {
		withAttributePhrases(s)
		s.BareTypesetListing = BareLocalListsEveryParameter
	})
	if !strings.Contains(out, "3") {
		t.Errorf("the control wrote no value either, so the row above proves nothing: %q", out)
	}
}

// A name the running function made local lists too, if it carries an
// attribute — so this is not the scope's table read the other way round. The
// undeclared local is the control in the same body.
func TestAnAttributedLocalListsAndAnUndeclaredOneDoesNot(t *testing.T) {
	src := "f() { typeset -i loc=9; typeset plainloc=1; typeset; }\nf\n"
	out, errs, st := phraseRun(t, src, withAttributePhrases)
	if want := "integer loc\n"; out != want || errs != "" || st != 0 {
		t.Errorf("inside a function = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// Neither case letter is a case letter beside the numeric one. Written as a
// table because the three rows are one rule and a renderer that special-cased
// only the one it was shown would pass a single row.
func TestTheCaseLettersBesideTheNumericOneAreWidthAndSign(t *testing.T) {
	for _, tc := range []struct{ letters, want string }{
		{"-i", "integer v"},
		{"-li", "long integer v"},
		{"-ui", "unsigned integer v"},
		{"-l", "tolower v"},
		{"-u", "toupper v"},
		{"-li16", "long integer base 16 v"},
	} {
		out, errs, st := phraseRun(t, "typeset "+tc.letters+" v=7\ntypeset", withAttributePhrases)
		if want := tc.want + "\n"; out != want || errs != "" || st != 0 {
			t.Errorf("typeset %s v = %q (stderr %q, status %d), want %q",
				tc.letters, out, errs, st, want)
		}
	}
}

// A base of ten is the default and earns no word, which is what says the
// `base` words follow the *named* base rather than the one in force.
func TestOnlyABaseThatIsNotTheDefaultIsAWord(t *testing.T) {
	for _, tc := range []struct{ letters, want string }{
		{"-i", "integer v"},
		{"-i10", "integer v"},
		{"-i2", "integer base 2 v"},
	} {
		out, _, _ := phraseRun(t, "typeset "+tc.letters+" v=7\ntypeset", withAttributePhrases)
		if want := tc.want + "\n"; out != want {
			t.Errorf("typeset %s v = %q, want %q", tc.letters, out, want)
		}
	}
}

// The guard the two vocabularies need, and the reason this suite exists at
// all: an attribute a declaration can carry has to be spelled by **both**
// heads, because a second renderer omitting what the first carries is this
// tree's commonest defect. Each row below is one letter, and it is asserted
// against both listings in the same run — a letter added to one form and not
// the other turns this red rather than going out as a silent blank.
func TestBothAttributeVocabulariesSpellEveryAttribute(t *testing.T) {
	// Every letter the attribute-word head has something to say about, which
	// is the set this test is a guard over. `-f` is not an attribute on a
	// name and `-p` is not an attribute at all.
	letters := []string{"-x", "-r", "-i", "-i16", "-u", "-l", "-a", "-A"}
	sort.Strings(letters)
	for _, letters := range letters {
		src := "typeset " + letters + " v\ntypeset"
		phrase, _, _ := phraseRun(t, src, withAttributePhrases)
		words, _, _ := declRun(t, src, func(s *Semantics) {
			withAttributePhrases(s)
			s.BareTypesetListing = BareLocalListsEveryParameter
		}, Diagnostics{})
		if strings.TrimSpace(phrase) == "v" || phrase == "" {
			t.Errorf("typeset %s v: the phrase listing said no attribute — %q", letters, phrase)
		}
		if trimmed := strings.TrimSpace(words); !strings.Contains(trimmed, " ") {
			t.Errorf("typeset %s v: the word listing said no attribute — %q", letters, words)
		}
	}
}
