// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"regexp"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The substrate behind the **names-only** shape of a declaration's listing:
// the sign, read where no pattern was written. Tests name the axis and the
// letter and never a shell — the measurements are in
// interp/declarebuiltin.go and dialect/zsh/namesonlylisting_test.go.
//
// Two axes carry it, because the three shells with the builtin give three
// answers between them: Semantics.SignAloneIsAnOptionWord decides whether a
// bare `-` or `+` is an option word at all, and
// Semantics.FunctionNamesUnderPlus decides whether a plus-signed `f` names
// its functions or goes on writing them.

// withNamesOnly is declRun's setter for a dialect that reads the sign the way
// the shells with a names-only listing do. The attribute-word listing comes
// with it because the bare plus writes that listing's rows without their
// values, and a dialect with no such listing could not be asked.
func withNamesOnly(s *Semantics) {
	s.DeclareOptions = "aAfgilmprux"
	s.BareTypesetListing = BareLocalListsEveryParameter
	s.DeclaredNameWithoutValueIsEmpty = Yes
	s.SignAloneIsAnOptionWord = Yes
	s.FunctionNamesUnderPlus = Yes
	// How two letters combine, which the *filter* needs and the sign does
	// not: one letter reads the same under every answer, so this only
	// decides the two-letter rows. See Semantics.DeclarationListingFilter.
	s.DeclarationListingFilter = DeclarationFilterAnyLetter
}

// The plus sense of the `f` letter, with no pattern anywhere on the line —
// the row the `-m` path answered and this one did not.
func TestAPlusSignedFunctionLetterNamesItsFunctions(t *testing.T) {
	src := "pb() { echo two; }\npa() { echo one; }\n"
	for _, tc := range []struct{ name, line, want string }{
		{"with no operand", "typeset +f", "pa\npb\n"},
		{"with an operand", "typeset +f pa", "pa\n"},
		// The *letter's* sign and not the word's, and the last one written
		// wins: the two lines below differ in nothing else.
		{"the last letter decides, plus", "typeset -f +f pa", "pa\n"},
		{"the last letter decides, minus", "typeset +f -f pa", "pa () \n{ \n  echo one\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, src+tc.line, withNamesOnly, Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.line, out, errs, st, tc.want)
			}
		})
	}
}

// The other answer to the same axis, and it is not "the bodies instead": a
// dialect that says No is one where the sign takes the function attribute
// *off*, so there is no function listing left to write. With an operand that
// is a silent 0 — the name is still a function afterwards, which the `-F`
// line here shows — and with none it falls through to the bare word's own
// listing.
//
// Both answers are asserted because a fix that ignored the axis would pass
// the row above and fail this one.
func TestAPlusSignedFunctionLetterOnlyNamesWhereTheDialectSaysSo(t *testing.T) {
	saysNo := func(s *Semantics) {
		withNamesOnly(s)
		s.FunctionNamesUnderPlus = No
		s.BareTypesetListing = BareLocalListsWhatSetLists
		s.SetListing = SetListingAssignmentsThenFunctions
		s.SetListingQuoting = ListingQuoteWhenNeededPlain
	}
	out, errs, st := declRun(t, "pa() { echo one; }\ntypeset +f pa\npa", saysNo, Diagnostics{})
	if want := "one\n"; out != want || errs != "" || st != 0 {
		t.Errorf("typeset +f pa = %q (stderr %q, status %d), want silence and the name still a function", out, errs, st)
	}
	// With no operand there is nothing to take the attribute off, and the
	// bare word's listing is what is left. The `v=1` is what says this is
	// that listing and not the function one: a names-only listing has no
	// variables in it. The rows about the environment this test process
	// happens to hold are dropped, since the subject is the shape.
	out, errs, st = declRun(t, "v=1\npa() { echo one; }\ntypeset +f", saysNo, Diagnostics{})
	want := "v=1\npa () \n{ \n  echo one\n}\n"
	if got := scriptRowsOnly(out, "v=", "pa"); got != want || errs != "" || st != 0 {
		t.Errorf("typeset +f = %q (stderr %q, status %d), want the bare listing %q", got, errs, st, want)
	}
}

// scriptRowsOnly keeps the rows of a whole-table listing that are about names
// the snippet made, and everything after the first of them — the function
// bodies at the end run over several lines and are not name-prefixed.
func scriptRowsOnly(out string, prefixes ...string) string {
	lines := strings.SplitAfter(out, "\n")
	for i, line := range lines {
		for _, p := range prefixes {
			if strings.HasPrefix(line, p) {
				return strings.Join(lines[i:], "")
			}
		}
	}
	return out
}

// A status the sign does not touch: a name nobody defined is silent at 1, and
// the 1 stands however many other names printed.
func TestANamedFunctionListingReportsOneForANameItDoesNotHold(t *testing.T) {
	src := "pa() { echo one; }\ntypeset +f pa nosuch"
	out, errs, st := declRun(t, src, withNamesOnly, Diagnostics{})
	if out != "pa\n" || errs != "" || st != 1 {
		t.Errorf("typeset +f pa nosuch = %q (stderr %q, status %d), want %q at 1", out, errs, st, "pa\n")
	}
}

// A sign carrying no letters is an option word where the axis says so, and
// the two signs are two listings: values under a minus, attribute words and
// the bare name under a plus.
func TestABareSignIsAnOptionWordAndPicksTheListing(t *testing.T) {
	src := "qa=1\ntypeset -i qb=2\n"
	for _, tc := range []struct{ name, line, want string }{
		{"a minus is the bare listing", "typeset -", "qa=\"1\"\ninteger qb=\"2\"\n"},
		{"a plus leaves the values off", "typeset +", "qa\ninteger qb\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, src+tc.line, withNamesOnly, Diagnostics{})
			if onlyQ(out) != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.line, out, errs, st, tc.want)
			}
		})
	}
}

// onlyQ keeps the rows naming the parameters a test wrote, so that a listing
// of the whole table can be compared *exactly* rather than by containment —
// which is what lets these rows assert an absence, and a listing that wrote
// values where it should have written names fails on the value rather than on
// a count.
//
// A Runner is born holding `IFS`, `OPTIND`, `PPID`, `PWD` and `TMPDIR`, and
// the last two hold a temporary directory whose generated name can contain
// any letter — so the test is on the *name* position and not on the line,
// which a plain substring search got wrong the first time.
var qNameRow = regexp.MustCompile(`(^|\s)q[a-z0-9_]*(=|$)`)

func onlyQ(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		if qNameRow.MatchString(line) {
			keep = append(keep, line)
		}
	}
	if len(keep) == 0 {
		return ""
	}
	return strings.Join(keep, "\n") + "\n"
}

// And where the axis says otherwise the same word is an operand — a name, and
// not one a script may declare. The refusal is the dialect's own and is
// asserted by its status and its silence on stdout, since the wording belongs
// to whichever preset supplied it.
func TestABareSignIsANameWhereTheDialectSaysSo(t *testing.T) {
	out, errs, st := declRun(t, "pa=1\ntypeset +", func(s *Semantics) {
		withNamesOnly(s)
		s.SignAloneIsAnOptionWord = No
	}, Diagnostics{})
	if out != "" || st == 0 {
		t.Errorf("typeset + = %q (stderr %q, status %d), want the operand refused", out, errs, st)
	}
	if !strings.Contains(errs, "+") {
		t.Errorf("stderr = %q, want the sign named as the operand it is", errs)
	}
}

// A plus-signed attribute letter with no pattern is that letter's filter over
// the whole table: the names carrying the attribute, and no attribute words in
// front of them. Two letters are a union and not an intersection, which is the
// row a single letter cannot discriminate.
func TestAPlusSignedAttributeLetterIsAFilterOverTheWholeTable(t *testing.T) {
	src := "qa=1\nexport qb=2\ntypeset -i qc=3\n"
	for _, tc := range []struct{ name, line, want string }{
		{"one letter", "typeset +x", "qb\n"},
		{"the other letter", "typeset +i", "qc\n"},
		{"two letters are a union", "typeset +xi", "qb\nqc\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, src+tc.line, withNamesOnly, Diagnostics{})
			if onlyQ(out) != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.line, out, errs, st, tc.want)
			}
		})
	}
}

// The *minus* sign on the same attribute letter is the other listing: the
// names that filter selects, each written as a row carrying its value. The
// two signs must not answer alike — a minus form reaching the *names* listing
// would look exactly like a working `-x` to anything reading it, and is a
// shape no shell writes.
//
// It wrote nothing at all, at status 0, until #1868: the letter was read as a
// filter and then given up on, so the common way to dump an environment as
// re-readable declarations came back empty.
//
// The row is BareDeclarationListing's, which is asserted here as *bytes*
// rather than by containment: a listing that grew a command word or lost one
// is the failure this catches, and a substring test cannot see either.
func TestAMinusSignedAttributeLetterListsTheSelectedValues(t *testing.T) {
	src := "qa=1\nexport qb=2\ntypeset -i qc=3\n"
	clustered := func(s *Semantics) {
		withNamesOnly(s)
		s.BareDeclarationListing = DeclareListingClustered
	}
	plain := func(s *Semantics) {
		withNamesOnly(s)
		s.BareDeclarationListing = DeclareListingPlainAssignment
		s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
	}
	for _, tc := range []struct {
		name, line string
		set        func(*Semantics)
		want       string
	}{
		{"the row the clustered form writes", "typeset -x", clustered, "declare -x qb=\"2\"\n"},
		{"the row the plain form writes", "typeset -x", plain, "qb=2\n"},
		{"the other letter", "typeset -i", plain, "qc=3\n"},
		{"two letters follow the axis", "typeset -xi", plain, "qb=2\nqc=3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, src+tc.line, tc.set, Diagnostics{})
			if onlyQ(out) != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.line, out, errs, st, tc.want)
			}
		})
	}
}

// The two signs are two listings, which is the pair this asserts rather than
// either row: the plus names the selection and the minus values it.
func TestTheTwoSignsOfAnAttributeLetterAreTwoListings(t *testing.T) {
	src := "qa=1\nexport qb=2\n"
	plain := func(s *Semantics) {
		withNamesOnly(s)
		s.BareDeclarationListing = DeclareListingPlainAssignment
		s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
	}
	plus, errs, st := declRun(t, src+"typeset +x", plain, Diagnostics{})
	if onlyQ(plus) != "qb\n" || errs != "" || st != 0 {
		t.Fatalf("typeset +x = %q (stderr %q, status %d), want %q", plus, errs, st, "qb\n")
	}
	minus, errs, st := declRun(t, src+"typeset -x", plain, Diagnostics{})
	if onlyQ(minus) != "qb=2\n" || errs != "" || st != 0 {
		t.Fatalf("typeset -x = %q (stderr %q, status %d), want %q", minus, errs, st, "qb=2\n")
	}
	if onlyQ(minus) == onlyQ(plus) {
		t.Errorf("typeset -x and typeset +x both wrote %q: the sign picks between two "+
			"listings and the minus one carries values, so answering them alike is a "+
			"third shape no shell writes", onlyQ(plus))
	}
}

// A name that is typed and holds nothing is a row of the filtered listing,
// which is the state #1664 added and the one a listing is likeliest to drop:
// the kind is all there is, and nothing scalar records it.
//
// It was dropped, and by a *second* reading of "is this still a declaration"
// — declarableNames kept its own list of the attributes that outlive a value
// and that list had no room for a compound, so `typeset -p q` wrote the row
// from the same state the listing passed over.
func TestAValuelessCompoundIsAmongTheFilteredRows(t *testing.T) {
	kept := func(s *Semantics) {
		withNamesOnly(s)
		s.BareDeclarationListing = DeclareListingClustered
		s.LocalOptions = "aAgilprux"
		// The other reading of a valueless declaration: the name is left
		// typed and unset rather than empty, which is the state this is
		// about — an empty compound has elements to print and never reaches
		// the question.
		s.DeclaredNameWithoutValueIsEmpty = No
	}
	// A local is the route that reaches the state: the name is typed, the
	// scope has taken its value away, and the kind is the whole of what is
	// left.
	out, errs, st := declRun(t, "f() { local -a qz\ntypeset -a\n}\nf", kept, Diagnostics{})
	if onlyQ(out) != "declare -a qz\n" || errs != "" || st != 0 {
		t.Errorf("typeset -a = %q (stderr %q, status %d), want %q",
			out, errs, st, "declare -a qz\n")
	}
}

// How two letters combine is the axis, and it is asked only where two were
// written: the three readings agree on one letter, so a dialect that has not
// chosen still answers `typeset -x`.
//
// The table is one name per interesting combination, so that a union, an
// intersection and a narrowing each write something the other two do not.
func TestTheAttributeLettersCombineTheWayTheAxisSays(t *testing.T) {
	src := "qa=1\nexport qb=2\ntypeset -i qc=3\ntypeset -a qd=(p)\ntypeset -ai qe=(4)\n"
	with := func(form DeclarationFilterForm) func(*Semantics) {
		return func(s *Semantics) {
			withNamesOnly(s)
			s.BareDeclarationListing = DeclareListingPlainAssignment
			s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
			s.DeclarationListingFilter = form
		}
	}
	for _, tc := range []struct {
		name, line string
		form       DeclarationFilterForm
		want       string
	}{
		// One letter, under every answer: the row that says the axis is not
		// asked on the line a script actually writes.
		{"one letter, any", "typeset -i", DeclarationFilterAnyLetter, "qc=3\nqe=( 4 )\n"},
		{"one letter, every", "typeset -i", DeclarationFilterEveryLetter, "qc=3\nqe=( 4 )\n"},
		{"one letter, narrowing", "typeset -i", DeclarationFilterKindNarrowsAny, "qc=3\nqe=( 4 )\n"},
		{"one letter, unanswered", "typeset -i", DeclarationFilterUnspecified, "qc=3\nqe=( 4 )\n"},

		// Two attribute letters: a join, and the intersection that is its
		// opposite on the same table.
		{"two letters joined", "typeset -xi", DeclarationFilterAnyLetter, "qb=2\nqc=3\nqe=( 4 )\n"},
		{"two letters intersected", "typeset -xi", DeclarationFilterEveryLetter, ""},

		// A kind letter beside an attribute one, which is where the third
		// answer parts company with the first: joined it writes every array
		// and every integer, narrowed only the array that is an integer.
		{"a kind letter joins", "typeset -ai", DeclarationFilterAnyLetter, "qc=3\nqd=( p )\nqe=( 4 )\n"},
		{"a kind letter narrows", "typeset -ai", DeclarationFilterKindNarrowsAny, "qe=( 4 )\n"},
		{"a kind letter intersects", "typeset -ai", DeclarationFilterEveryLetter, "qe=( 4 )\n"},

		// And the narrowing keeps the join among the letters it narrows:
		// `-axi` is the arrays that are exported *or* integers.
		{"the narrowed letters still join", "typeset -axi", DeclarationFilterKindNarrowsAny, "qe=( 4 )\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, src+tc.line, with(tc.form), Diagnostics{})
			if onlyQ(out) != tc.want || errs != "" || st != 0 {
				t.Errorf("%s under %v = %q (stderr %q, status %d), want %q",
					tc.line, tc.form, out, errs, st, tc.want)
			}
		})
	}
}

// Two letters where the axis is unanswered is a refusal and not a guess, on
// the discipline the whole substrate runs on: the three shells disagree and
// nothing chose. The control is one letter on the same dialect, which still
// answers.
func TestTwoAttributeLettersAreRefusedWhereNoDialectChose(t *testing.T) {
	unchosen := func(s *Semantics) {
		withNamesOnly(s)
		s.BareDeclarationListing = DeclareListingPlainAssignment
		s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
		s.DeclarationListingFilter = DeclarationFilterUnspecified
	}
	src := "export qb=2\ntypeset -i qc=3\n"
	out, errs, st := declRun(t, src+"typeset -xi", unchosen, Diagnostics{})
	if out != "" || st != 2 {
		t.Errorf("typeset -xi = %q (stderr %q, status %d), want the axis refused", out, errs, st)
	}
	if !strings.Contains(errs, "the shells disagree here and no dialect was chosen") {
		t.Errorf("stderr = %q, want the unanswered-axis refusal", errs)
	}
	// The same dialect, one letter: answered, because all three readings say
	// the same thing there.
	out, errs, st = declRun(t, src+"typeset -x", unchosen, Diagnostics{})
	if onlyQ(out) != "qb=2\n" || errs != "" || st != 0 {
		t.Errorf("typeset -x = %q (stderr %q, status %d), want %q", out, errs, st, "qb=2\n")
	}
}

// A letter the dialect spells and this engine records nothing for is still an
// attribute to select on: nothing carries it, so the listing is empty rather
// than whole. The control is the letter that says where a declaration lands
// rather than what a name is — that one drops out and the whole table stands.
func TestALetterWithoutEffectSelectsNothingAndTheGlobalLetterSelectsEverything(t *testing.T) {
	inert := func(s *Semantics) {
		withNamesOnly(s)
		s.DeclareOptions = "aAfgilmpruxz"
		s.DeclareOptionsWithoutEffect = "z"
	}
	src := "qa=1\n"
	out, errs, st := declRun(t, src+"typeset -z", inert, Diagnostics{})
	if out != "" || errs != "" || st != 0 {
		t.Errorf("typeset -z = %q (stderr %q, status %d), want silence at 0", out, errs, st)
	}
	out, errs, st = declRun(t, src+"typeset +z", inert, Diagnostics{})
	if out != "" || errs != "" || st != 0 {
		t.Errorf("typeset +z = %q (stderr %q, status %d), want silence at 0", out, errs, st)
	}
	// The same shape with the letter that filters nothing, which is what
	// keeps the row above from reading as "any letter this engine does not
	// model prints nothing".
	for _, line := range []string{"typeset -g", "typeset +g"} {
		out, errs, st = declRun(t, src+line, inert, Diagnostics{})
		if onlyQ(out) != "qa=\"1\"\n" || errs != "" || st != 0 {
			t.Errorf("%s = %q (stderr %q, status %d), want the whole table", line, out, errs, st)
		}
	}
	// And the inert letter still *declares* where a name was written, which
	// is the half that says it was taken rather than skipped.
	out, errs, st = declRun(t, "typeset -z qz\ntypeset -p qz", inert, Diagnostics{})
	if !strings.Contains(out, "qz") || errs != "" || st != 0 {
		t.Errorf("typeset -z qz = %q (stderr %q, status %d), want the name declared", out, errs, st)
	}
}
