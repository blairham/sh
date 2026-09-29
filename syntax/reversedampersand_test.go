// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// reversedDialect is a core dialect with the both-streams operators written
// with the ampersand last, and the marker with it, since four of the five
// spellings are the marker on that family.
func reversedDialect() Dialect {
	d := Core()
	d.ReversedAmpersandRedirect = true
	d.ClobberOverrideMarker = true
	return d
}

// TestReversedAmpersandReadsEverySpelling covers the five the flag adds, each
// as the operator of a single redirection rather than merely "it parsed".
//
// `>&` is the control and it is not the flag's: it is POSIX duplication, core
// in every dialect, and it has to keep reading as itself while the family
// around it appears. The row that would have caught the obvious mistake — a
// lexer that finds `>&` first and leaves the rest as a word — is `>>&`, since
// the longest match has to beat both `>>` and `>&`.
func TestReversedAmpersandReadsEverySpelling(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src string
		op  Kind
	}{
		{"echo hi >>& f", TokDGreatAmp},
		{"echo hi >&| f", TokGreatAmpClobber},
		{"echo hi >&! f", TokGreatAmpBang},
		{"echo hi >>&| f", TokDGreatAmpClobber},
		{"echo hi >>&! f", TokDGreatAmpBang},
		{"echo hi >& f", TokGreatAmp},
	} {
		rs, args := redirsOf(t, tc.src, reversedDialect())
		if len(rs) != 1 || rs[0].Op != tc.op {
			t.Errorf("%q: got %v, want one redirection with op %v", tc.src, opsOf(rs), tc.op)
			continue
		}
		if target := litOf(rs[0].Word); target != "f" {
			t.Errorf("%q: redirection target %q, want f", tc.src, target)
		}
		if len(args) != 2 {
			t.Errorf("%q: command words %v, want just the name and hi — the target leaked into the arguments",
				tc.src, args)
		}
	}
}

// TestWithoutTheFlagTheBangSpellingIsAWordAndNotAnError is the silent
// fallback, and it is the reason this is a flag rather than something always
// on. With the family off, `>&! f` is a `>&` onto a file *called* `!` with
// `f` left as an argument — which parses, runs, reports success and writes
// the wrong file. Measured: bash 5.3.20 and bash 3.2.57 do exactly that, and
// so does BusyBox ash 1.36.1.
//
// **Only this one spelling is silent**, and the reason is that `>&` is the
// only member of the family that is core. `>>&!` falls back to `>>` and then
// a bare `&`, which has no reading and is refused — so the silent half of
// the hazard is one row and the loud half is the other four. The two were
// written as one set here at first and the package run caught it.
func TestWithoutTheFlagTheBangSpellingIsAWordAndNotAnError(t *testing.T) {
	t.Parallel()
	for _, src := range []string{"echo hi >&! f"} {
		d := Core()
		d.ClobberOverrideMarker = true
		rs, args := redirsOf(t, src, d)
		if len(rs) != 1 {
			t.Errorf("%q: got %v, want exactly one redirection", src, opsOf(rs))
			continue
		}
		if target := litOf(rs[0].Word); target != "!" {
			t.Errorf("%q: redirection target %q, want the bang as a filename", src, target)
		}
		if len(args) != 3 || args[2] != "f" {
			t.Errorf("%q: command words %v, want the target left as a third word", src, args)
		}
	}
}

// TestWithoutTheFlagThePipeSpellingAndTheAppendAreRefused is the other
// fallback and it is not silent: `>&|` is a pipe with nothing on its left,
// and `>>&` has no reading at all. Both are syntax errors in bash 5.3.20,
// bash 3.2.57, dash, ksh93 and BusyBox ash, measured 2026-09-29.
func TestWithoutTheFlagThePipeSpellingAndTheAppendAreRefused(t *testing.T) {
	t.Parallel()
	d := Core()
	d.ClobberOverrideMarker = true
	for _, src := range []string{
		"echo hi >&| f", "echo hi >>&| f", "echo hi >>& f", "echo hi >>&! f",
	} {
		mustFail(t, src, d, "without the reversed family these have no reading")
	}
}

// TestTheMarkerOnTheReversedFamilyNeedsTheFamily pins the composition, which
// is the same rule the `&>` four already follow: a marker is a marker *on* an
// operator, so it needs the operator.
//
// It is worth a test rather than an assertion in the lexer because `>&` is
// core, so the marked spellings *look* reachable with the marker flag alone.
// They are not, and the reason is measured rather than tidy: a marker after
// `>&` is not a marker on the duplication. `>&|2` and `>&!2` write a file
// called `2` where `>&2` duplicates the descriptor, so the marked forms
// belong to the family that treats its target as a name.
func TestTheMarkerOnTheReversedFamilyNeedsTheFamily(t *testing.T) {
	t.Parallel()
	d := Core()
	d.ClobberOverrideMarker = true
	d.ReversedAmpersandRedirect = false
	// The bang spelling still parses — as `>&` onto a file called `!` —
	// which the test above covers. The pipe spelling is the one that shows
	// the operator was not read, since nothing else can take a bare `|`.
	mustFail(t, "echo hi >&| f", d, "the marker alone does not reach the reversed family")
	// And with the family on but the marker off, the append is there and the
	// marked forms are not.
	d = Core()
	d.ReversedAmpersandRedirect = true
	d.ClobberOverrideMarker = false
	rs, _ := redirsOf(t, "echo hi >>& f", d)
	if len(rs) != 1 || rs[0].Op != TokDGreatAmp {
		t.Errorf("got %v, want >>& read from the family flag alone", opsOf(rs))
	}
	mustFail(t, "echo hi >>&| f", d, "without the marker the trailing | opens a pipe")
}

// TestTheReversedFamilyIsARedirectKind guards the classifier the parser
// lifts redirections out of a word list with. A kind missing from IsRedirect
// lexes and then is not treated as a redirection at all.
func TestTheReversedFamilyIsARedirectKind(t *testing.T) {
	t.Parallel()
	for _, k := range []Kind{
		TokDGreatAmp, TokGreatAmpClobber, TokGreatAmpBang,
		TokDGreatAmpClobber, TokDGreatAmpBang,
	} {
		if !k.IsRedirect() {
			t.Errorf("%v is not classified as a redirection", k)
		}
		if k.String() == "unknown token" {
			t.Errorf("%v has no spelling in the operator table", Kind(k))
		}
	}
}

// TestTheReversedFamilyBindsToADescriptorNumber covers the place a
// redirection's operator is easiest to lose: an explicit descriptor in front
// of it.
func TestTheReversedFamilyBindsToADescriptorNumber(t *testing.T) {
	t.Parallel()
	rs, args := redirsOf(t, "echo hi 2>>& f", reversedDialect())
	if len(rs) != 1 || rs[0].Op != TokDGreatAmp {
		t.Fatalf("got %v, want one >>& redirection", opsOf(rs))
	}
	if rs[0].N == nil || litOf(rs[0].N) != "2" {
		t.Errorf("descriptor %v, want 2", rs[0].N)
	}
	if len(args) != 2 {
		t.Errorf("command words %v, want the target out of the arguments", args)
	}
}

// TestEveryReversedSpellingPrintsBackAsWritten keeps the printer honest. One
// dialect spells both-streams twice over and the two spellings mean the same
// thing to it, so a shared kind would round-trip `>>&` as `&>>` — text that
// means the same to that dialect and is a syntax error to every other.
//
// The `-` target is the row that matters most here: `>&-` is a close and the
// printer normalizes a close to `0>&-`, while `>>&-` is a *write to a file
// called `-`* and must not be caught by that normalization.
func TestEveryReversedSpellingPrintsBackAsWritten(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"echo hi >>& f", "echo hi >&| f", "echo hi >&! f",
		"echo hi >>&| f", "echo hi >>&! f",
		"echo hi >>& 2", "echo hi >>& -", "echo hi >&| 2",
	} {
		f, err := Parse(src, reversedDialect())
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if got := Print(f); got != src {
			t.Errorf("Print(%q) = %q, want it back unchanged", src, got)
		}
	}
}
