// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// readEdges runs src with `read`'s two edge questions under the test's
// control and everything else answered flat.
//
// The array target is the only one that can see either of them, so every row
// below reads the element count back rather than a name.
func readEdges(t *testing.T, src string, whitespace, none interp.Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
	}, func(r *interp.Runner) {
		sem := interp.CoreSemantics()
		sem.ReadOptions = "rA"
		sem.LastPipelineElementInCurrentShell = interp.Yes
		sem.TrailingSeparatorEndsAField = interp.No
		sem.ReadTrailingWhitespaceEndsAField = whitespace
		sem.ReadNoFieldsIsOneEmptyElement = none
		// Reading an empty array back through a quoted `[@]` is an axis of
		// its own, and the rows below have to read an empty array. Answered
		// flat as the shells with the letter answer it — no field.
		sem.EmptyArrayAtIsOneEmptyField = interp.No
		r.Semantics = &sem
	})
}

const countElems = `printf 'n=%s' "${#r[@]}"; for e in "${r[@]}"; do printf '[%s]' "$e"; done`

// A closing run of IFS whitespace opens a field of its own in `read`, where
// the dialect says it does — the question the splitter's own tail does not
// ask, because on an expansion the same shell absorbs it.
func TestReadIntoAnArrayAsksAboutAClosingWhitespaceRun(t *testing.T) {
	for _, tc := range []struct{ name, src, absorbed, opens string }{
		{
			"one trailing space",
			`printf 'a \n' | { read -r -A r; ` + countElems + `; }`,
			`n=1[a]`, `n=2[a][]`,
		},
		{
			// The run rather than the byte, the same rule the neighboring
			// axis follows: two spaces open one field and not two.
			"a run of trailing spaces",
			`printf 'a  \n' | { read -r -A r; ` + countElems + `; }`,
			`n=1[a]`, `n=2[a][]`,
		},
		{
			"a trailing tab",
			`printf 'a\t\n' | { read -r -A r; ` + countElems + `; }`,
			`n=1[a]`, `n=2[a][]`,
		},
		{
			// Whitespace at *both* ends: the leading run is absorbed under
			// either answer, so the asymmetry is at the tail alone.
			"whitespace at both ends",
			`printf ' a \n' | { read -r -A r; ` + countElems + `; }`,
			`n=1[a]`, `n=2[a][]`,
		},
		{
			// An escaped separator is data and ends the run, so there is no
			// closing run to ask about.
			"an escaped trailing space",
			`printf 'a\\ \n' | { read -A r; ` + countElems + `; }`,
			`n=1[a ]`, `n=1[a ]`,
		},
		{
			// A closing *non-whitespace* separator is the neighboring
			// question's and not this one's, so this answer decides nothing
			// there — the helper answers that one No, and the count stays
			// one however this is answered.
			"a closing non-whitespace separator",
			`IFS=:; printf 'a:\n' | { read -r -A r; ` + countElems + `; }`,
			`n=1[a]`, `n=1[a]`,
		},
		{
			// Nothing at the tail to ask about, so the two answers agree —
			// the control that keeps the rows above honest.
			"no trailing separator",
			`printf 'a\n' | { read -r -A r; ` + countElems + `; }`,
			`n=1[a]`, `n=1[a]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := readEdges(t, tc.src, interp.No, interp.No); out != tc.absorbed || st != 0 {
				t.Errorf("absorbed: got %q status %d, want %q at 0", out, st, tc.absorbed)
			}
			if out, st := readEdges(t, tc.src, interp.Yes, interp.No); out != tc.opens || st != 0 {
				t.Errorf("opens a field: got %q status %d, want %q at 0", out, st, tc.opens)
			}
		})
	}
}

// A line that splits into no fields at all leaves one empty element or none.
func TestReadIntoAnArrayAsksAboutALineWithNoFields(t *testing.T) {
	for _, tc := range []struct{ name, src, none, one string }{
		{
			"an empty line",
			`printf '\n' | { read -r -A r; ` + countElems + `; }`,
			`n=0`, `n=1[]`,
		},
		{
			"a line of nothing but whitespace",
			`printf '   \n' | { read -r -A r; ` + countElems + `; }`,
			`n=0`, `n=1[]`,
		},
		{
			// The control: one field is not none, so the question is not put
			// and the two answers agree.
			"a line with a field in it",
			`printf 'a\n' | { read -r -A r; ` + countElems + `; }`,
			`n=1[a]`, `n=1[a]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := readEdges(t, tc.src, interp.No, interp.No); out != tc.none || st != 0 {
				t.Errorf("no elements: got %q status %d, want %q at 0", out, st, tc.none)
			}
			if out, st := readEdges(t, tc.src, interp.No, interp.Yes); out != tc.one || st != 0 {
				t.Errorf("one empty element: got %q status %d, want %q at 0", out, st, tc.one)
			}
		})
	}
}

// The order of the two is load-bearing, and a line of nothing but whitespace
// is the only shape that can show it: the shell that opens a field on the
// closing run has one by then and must not be asked for a second.
func TestTheClosingRunIsAskedBeforeTheEmptyLine(t *testing.T) {
	src := `printf '   \n' | { read -r -A r; ` + countElems + `; }`
	if out, st := readEdges(t, src, interp.Yes, interp.Yes); out != "n=1[]" || st != 0 {
		t.Errorf("both answered yes: got %q status %d, want %q at 0", out, st, "n=1[]")
	}
	// And an empty line has no closing run, so the second question is what
	// answers it in that same shell.
	empty := `printf '\n' | { read -r -A r; ` + countElems + `; }`
	if out, st := readEdges(t, empty, interp.Yes, interp.Yes); out != "n=1[]" || st != 0 {
		t.Errorf("an empty line: got %q status %d, want %q at 0", out, st, "n=1[]")
	}
	if out, st := readEdges(t, empty, interp.Yes, interp.No); out != "n=0" || st != 0 {
		t.Errorf("an empty line, second answer no: got %q status %d, want %q at 0", out, st, "n=0")
	}
}

// Neither question is put where nothing can see it. A list of names takes the
// last field as the remainder of the *line*, and the closing whitespace comes
// off that remainder anyway — so these run unanswered, without a word.
//
// This is the half that a test of the two answers alone cannot see: both
// answers give `[a]` here, so only the *unanswered* run says whether the
// question was put at all.
func TestNeitherReadEdgeQuestionReachesAListOfNames(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"one name, a closing whitespace run",
			`printf 'a  \n' | { read -r x; printf '[%s]' "$x"; }`,
			`[a]`,
		},
		{
			"two names, a closing whitespace run",
			`printf 'a b  \n' | { read -r x y; printf '[%s][%s]' "$x" "$y"; }`,
			`[a][b]`,
		},
		{
			"an empty line into a name",
			`printf '\n' | { read -r x; printf '[%s]' "$x"; }`,
			`[]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := readEdges(t, tc.src, interp.Unspecified, interp.Unspecified)
			if out != tc.want || st != 0 {
				t.Errorf("unanswered: got %q status %d, want %q at 0 with nothing said", out, st, tc.want)
			}
		})
	}
}

// An IFS set to nothing splits nothing, so neither question is reached
// however the line ends.
func TestAnEmptyIFSReachesNeitherReadEdgeQuestion(t *testing.T) {
	src := `IFS=; printf 'a b \n' | { read -r -A r; ` + countElems + `; }`
	out, st := readEdges(t, src, interp.Unspecified, interp.Unspecified)
	if want := "n=1[a b ]"; out != want || st != 0 {
		t.Errorf("an empty IFS: got %q status %d, want %q at 0 with nothing said", out, st, want)
	}
}
