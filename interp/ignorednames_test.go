// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The parameter whose patterns take names back out of a pathname expansion.
// Named as a facility and never as a shell: which parameter it is belongs to
// the dialect, and dialect/bash asserts that the name is `GLOBIGNORE`.

// ignoreTree lays out the fixture every assertion here is about: two names
// ending `.txt`, one that does not, two hidden names — one of which also ends
// `.txt` — and a directory holding a hidden name and a plain one.
//
// The hidden `.hid.txt` is what separates the two halves of the facility. A
// filter alone would leave it out of `*` because it is hidden; a filter that
// also reveals hidden names has to take it out by pattern, so a fixture
// without it cannot tell the two apart.
func ignoreTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.txt", "b.txt", "c.log", ".dot", ".hid.txt", "sub/x.txt", "sub/.y"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// ignoring is a runner in the fixture with the facility wired the way the one
// dialect that has it wires it.
func ignoring(dir string) func(*Runner) {
	return func(r *Runner) {
		r.Semantics.IgnoredNamesVariable = "GLOBIGNORE"
		r.Semantics.IgnoredNamesRevealHiddenNames = true
		// The three axes the two shells with this facility answer apart,
		// with the answers of the one these rows were measured on. The
		// other's are in dialect/ksh (#2748).
		r.Semantics.IgnoredNamesValueIsOnePattern = No
		r.Semantics.IgnoredNamesMatchTheLastComponent = No
		r.Semantics.IgnoredNamesFollowTheParameter = No
		r.Dir = dir
	}
}

// TestIgnoredNamesTakeWordsOutOfAPathnameExpansion.
//
// Measured on bash 5.3.15 and bash 3.2.57, 2026-09-13, in this fixture. The
// two builds answer alike on every row here **but one**, so the version is
// named where it matters rather than nowhere: on `a star in the pattern does
// not cross a separator` bash 3.2.57 answers `[sub/.y]`, having let the `*`
// cross the `/` and taken `sub/x.txt` out. The separator rule is 5.x's.
// (The other place the builds part is the fold — see
// TestIgnoredNamesFoldCaseWithTheExpansion.) No preset is bash 3.2, so both
// are recorded rather than made an axis; the golden record carries bash32's
// own answer on the corpus row for this.
func TestIgnoredNamesTakeWordsOutOfAPathnameExpansion(t *testing.T) {
	dir := ignoreTree(t)
	for _, tc := range []struct{ name, src, want string }{
		{
			// The pattern takes the two `.txt` names out, and the
			// assignment brings the hidden ones in — so `.dot` arrives in
			// the same breath that `a.txt` leaves, and `.hid.txt` is
			// revealed and then ignored.
			"a pattern removes names and the assignment reveals hidden ones",
			`GLOBIGNORE='*.txt'; printf "[%s]" *`, `[.dot][c.log][sub]`,
		},
		{
			// The value is a list, and this is the row that says so: with
			// the colon read as an ordinary character neither name goes.
			"the value is a colon-separated list",
			`GLOBIGNORE='a.txt:c.log'; printf "[%s]" *`, `[.dot][.hid.txt][b.txt][sub]`,
		},
		{
			// An empty element is no pattern rather than a pattern matching
			// an empty name, which is what keeps a trailing colon from
			// deleting the whole expansion.
			"an empty element ignores nothing",
			`GLOBIGNORE='a.txt:'; printf "[%s]" *`, `[.dot][.hid.txt][b.txt][c.log][sub]`,
		},
		{
			"a leading colon ignores nothing either",
			`GLOBIGNORE=':b.txt'; printf "[%s]" *`, `[.dot][.hid.txt][a.txt][c.log][sub]`,
		},
		{
			// A separator in the word has to be matched by one in the
			// pattern: the `*` stops at the `/` exactly as one in the
			// expansion's own pattern does.
			"a star in the pattern does not cross a separator",
			`GLOBIGNORE='*x.txt'; printf "[%s]" */*`, `[sub/.y][sub/x.txt]`,
		},
		{
			"a pattern with the separator in it does reach across",
			`GLOBIGNORE='*/x.txt'; printf "[%s]" */*`, `[sub/.y]`,
		},
		{
			// And no leading-period rule beside it: a `*` after the
			// separator does take the hidden name out, which is the half
			// the walk's own rule does not have.
			"there is no leading-period rule in the pattern",
			`GLOBIGNORE='sub/.*'; printf "[%s]" */*`, `[sub/x.txt]`,
		},
		{
			// The word as the expansion spelled it, `./` and all.
			"the pattern is matched against the word and not against a path",
			`GLOBIGNORE='a.txt'; printf "[%s]" ./a.txt ./b.txt`, `[./a.txt][./b.txt]`,
		},
		{
			"and the spelling the pattern wrote is what matches",
			`GLOBIGNORE='./a.txt'; printf "[%s]" ./*.txt`, `[./.hid.txt][./b.txt]`,
		},
		{
			// The trailing slash is part of the word too, so a pattern
			// without one does not reach a directory a `*/` produced.
			"a trailing slash belongs to the word",
			`GLOBIGNORE='sub'; printf "[%s]" */`, `[sub/]`,
		},
		{
			"and a pattern that writes it does reach it",
			`GLOBIGNORE='sub/'; printf "[%s]" */`, `[*/]`,
		},
		{
			// Nothing left is a pattern that matched nothing: with no
			// option asked for, the word stands as it was written.
			"a word left with nothing is a pattern that matched nothing",
			`GLOBIGNORE='*.txt'; printf "[%s]" *.txt`, `[*.txt]`,
		},
		{
			// It reaches pathname expansion and nothing else.
			"a case arm is not filtered",
			`GLOBIGNORE='*.txt'; case a.txt in *.txt) printf "[hit]" ;; *) printf "[miss]" ;; esac`,
			`[hit]`,
		},
		{
			"nor a parameter expansion's pattern operator",
			`GLOBIGNORE='*.txt'; v=a.txt; printf "[%s]" "${v%.txt}"`, `[a]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, ignoring(dir))
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}

// TestIgnoredNamesEmptiedByTheOptionsThatDecideAMiss.
//
// A word the filter left with nothing is a miss, so the two options that
// already decide what a miss means decide this as well. Measured: with
// `GLOBIGNORE='*.txt'` bash answers `*.txt` under neither option, an empty
// word under `nullglob`, and `no match: *.txt` under `failglob`.
//
// The operand is a pattern that *did* match — every `.txt` name is there —
// which is what makes this the filter's miss and not the walk's.
func TestIgnoredNamesEmptiedByTheOptionsThatDecideAMiss(t *testing.T) {
	dir := ignoreTree(t)
	for _, tc := range []struct {
		name   string
		option MatchOption
		src    string
		want   string
	}{
		{"the word is deleted", UnmatchedPatternIsEmpty, `printf "[%s]" *.txt after`, `[after]`},
		{
			// The refusal is the only output: the command does not run, so
			// neither operand is printed. Written out rather than left as
			// the empty string, which a command that simply printed nothing
			// would also satisfy.
			"the word is refused", UnmatchedPatternIsError,
			`printf "[%s]" *.txt after`, "sh: no matches found: *.txt\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `GLOBIGNORE='*.txt'; `+tc.src,
				withOption(tc.option, true, ignoring(dir)))
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}

// TestIgnoredNamesFoldCaseWithTheExpansion pins the composition the filter
// has with the fold the expansion itself is under: one question and not two,
// so the patterns fold exactly where the walk's own do.
//
// Both rows are needed and neither alone would do. A filter that always
// folded answers the first and fails the second; one that never folded
// answers the second and fails the first.
//
// Measured on bash 5.3.15, 2026-09-13, with `nocaseglob`. **bash 3.2.57 does
// not fold here** — `*.TXT` takes nothing out under either setting of the
// option there — so this is 5.x's answer rather than bash's, and is the
// second of the two rules in this file where the two builds part. No preset
// is bash 3.2, so the divergence is recorded and not given an axis.
func TestIgnoredNamesFoldCaseWithTheExpansion(t *testing.T) {
	dir := ignoreTree(t)
	for _, tc := range []struct {
		name string
		fold bool
		want string
	}{
		{"the patterns fold where the expansion folds", true, `[.dot][c.log][sub]`},
		{
			"and do not where it does not", false,
			`[.dot][.hid.txt][a.txt][b.txt][c.log][sub]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `GLOBIGNORE='*.TXT'; printf "[%s]" *`,
				withOption(GlobFoldsCase, tc.fold, ignoring(dir)))
			if out != tc.want {
				t.Errorf("with fold %v = %s, want %s", tc.fold, out, tc.want)
			}
		})
	}
}

// TestIgnoredNamesFollowTheAssignmentAndNotTheValue.
//
// The one surprising thing about the facility, and the reason this shell
// models it as a state rather than as a lookup: a value the shell was started
// with is carried, is readable, and is not read. Measured —
// `env GLOBIGNORE='*.txt' bash -c 'echo *'` lists the `.txt` names and
// reports `dotglob` off, and assigning the parameter *its own value* starts
// both halves.
//
// The second row is what makes the first falsifiable. A run that only checked
// the inherited value could be passing because the filter is broken.
func TestIgnoredNamesFollowTheAssignmentAndNotTheValue(t *testing.T) {
	dir := ignoreTree(t)
	inherited := func(r *Runner) {
		ignoring(dir)(r)
		r.Env = append(r.Env, "GLOBIGNORE=*.txt")
	}
	for _, tc := range []struct{ name, src, want string }{
		{"an inherited value is not read", `printf "[%s]" *`, `[a.txt][b.txt][c.log][sub]`},
		{
			"assigning it its own value starts it",
			`GLOBIGNORE=$GLOBIGNORE; printf "[%s]" *`, `[.dot][c.log][sub]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, inherited)
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}

// TestIgnoredNamesWriteTheHiddenNameSwitch.
//
// The switch an assignment writes is the one the shell already has for
// hidden names rather than a second question the expansion asks. A shell that
// merely *behaved* as though hidden names were on while the parameter held a
// value would answer the first two rows alike and the last one wrongly: the
// unset writes the switch off, so a shell that started with it on does not
// get it back.
//
// That a *script* can write it back, through the name its dialect gives the
// switch, is asserted in dialect/bash — the core has no such name and must
// not learn one.
func TestIgnoredNamesWriteTheHiddenNameSwitch(t *testing.T) {
	dir := ignoreTree(t)
	for _, tc := range []struct {
		name   string
		hidden bool
		src    string
		want   string
	}{
		{
			"a non-null assignment turns hidden names on",
			false, `GLOBIGNORE='zzz'; printf "[%s]" *`,
			`[.dot][.hid.txt][a.txt][b.txt][c.log][sub]`,
		},
		{
			// An assignment of nothing writes neither half, so a null value
			// is not the same state as no value at all.
			"assigning nothing leaves the switch where it was",
			false, `GLOBIGNORE='zzz'; GLOBIGNORE=; printf "[%s]" *`,
			`[.dot][.hid.txt][a.txt][b.txt][c.log][sub]`,
		},
		{
			// And the row that makes the one above an assertion. It starts
			// from the switch already **on**, so "leaves it where it was"
			// and "turns it on" answer it alike — it proves the null
			// assignment was noticed and nothing more. This one starts from
			// the switch off, which is the only place the two part company.
			//
			// Measured on bash 5.3.15 and bash 3.2.57, 2026-09-13:
			// `GLOBIGNORE=` lists no hidden name and reports `dotglob` off.
			"assigning nothing does not turn it on either",
			false, `GLOBIGNORE=; printf "[%s]" *`,
			`[a.txt][b.txt][c.log][sub]`,
		},
		{
			// And the unset writes it off whoever turned it on — here the
			// shell was started with it on and does not keep it.
			"unsetting turns it off however it was set",
			true, `GLOBIGNORE='zzz'; unset GLOBIGNORE; printf "[%s]" *`,
			`[a.txt][b.txt][c.log][sub]`,
		},
		{
			// The control for the row above: with nothing unset, the shell
			// that started with hidden names on still has them.
			"and leaves it alone where nothing unset it",
			true, `printf "[%s]" *`,
			`[.dot][.hid.txt][a.txt][b.txt][c.log][sub]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src,
				withOption(PatternsMatchHidden, tc.hidden, ignoring(dir)))
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}

// TestIgnoredNamesLastAsLongAsTheirBinding.
//
// A `local` of the parameter is the facility's lifetime as well as the
// value's: the call sees its own patterns, and the caller's come back when it
// returns. Measured on bash 5.3.15 with `GLOBIGNORE='*.log'` outside the
// function.
//
// The third row is the one that needs the fixture's hidden names. A
// declaration with no value leaves the name *unset*, which is the unset half
// of the hook rather than the null one — so hidden names go with it, and come
// back at the return.
func TestIgnoredNamesLastAsLongAsTheirBinding(t *testing.T) {
	dir := ignoreTree(t)
	const outer = `GLOBIGNORE='*.log'; `
	for _, tc := range []struct{ name, src, want string }{
		{
			"a local of it is the call's",
			`f(){ local GLOBIGNORE='*.txt'; printf "[in %s]" *; }; f; printf "[out %s]" *`,
			`[in .dot][in c.log][in sub][out .dot][out .hid.txt][out a.txt][out b.txt][out sub]`,
		},
		{
			"a declaration with no value is the unset half",
			`f(){ local GLOBIGNORE; printf "[in %s]" *; }; f`,
			`[in a.txt][in b.txt][in c.log][in sub]`,
		},
		{
			"and the caller's comes back at the return",
			`f(){ local GLOBIGNORE; printf "[in %s]" *; }; f; printf "[out %s]" *`,
			`[in a.txt][in b.txt][in c.log][in sub]` +
				`[out .dot][out .hid.txt][out a.txt][out b.txt][out sub]`,
		},
		{
			// An `unset` inside a function is not a binding of its own, so
			// it does not come back.
			"an unset inside a call is not a binding",
			`f(){ unset GLOBIGNORE; }; f; printf "[%s]" *`,
			`[a.txt][b.txt][c.log][sub]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, outer+tc.src, ignoring(dir))
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}

// TestIgnoredNamesAreOffWhereTheDialectNamesNoParameter is the control, and
// it is what stops this facility from reaching a shell that has not got it:
// with the field empty the parameter is an ordinary variable and `GLOBIGNORE`
// means nothing at all.
func TestIgnoredNamesAreOffWhereTheDialectNamesNoParameter(t *testing.T) {
	dir := ignoreTree(t)
	out, _ := run(t, `GLOBIGNORE='*.txt'; printf "[%s]" *`, inDir(dir))
	const want = `[a.txt][b.txt][c.log][sub]`
	if out != want {
		t.Errorf("with no parameter named = %s, want %s", out, want)
	}
}

// The three axes the two shells with this facility answer apart, asked of a
// runner that is neither of them.
//
// The rows above are one dialect's whole model; these are the seams, each
// with both answers and the refusal that says the question has to be asked.
func TestTheIgnoreFacilityHasThreeAxes(t *testing.T) {
	dir := ignoreTree(t)
	// A colon is a separator, or it is a character in one pattern.
	list := func(r *Runner) {
		ignoring(dir)(r)
		r.Semantics.IgnoredNamesValueIsOnePattern = No
	}
	one := func(r *Runner) {
		ignoring(dir)(r)
		r.Semantics.IgnoredNamesValueIsOnePattern = Yes
	}
	src := `GLOBIGNORE='a.txt:c.log'; printf "[%s]" *`
	if out, _ := run(t, src, list); out != "[.dot][.hid.txt][b.txt][sub]" {
		t.Errorf("as a list: %q", out)
	}
	if out, _ := run(t, src, one); out != "[.dot][.hid.txt][a.txt][b.txt][c.log][sub]" {
		t.Errorf("as one pattern: %q", out)
	}
	// The subject is the word, or it is the entry the listing gave.
	word := func(r *Runner) {
		ignoring(dir)(r)
		r.Semantics.IgnoredNamesMatchTheLastComponent = No
	}
	entry := func(r *Runner) {
		ignoring(dir)(r)
		r.Semantics.IgnoredNamesMatchTheLastComponent = Yes
	}
	src = `GLOBIGNORE='x.txt'; printf "[%s]" sub/*`
	if out, _ := run(t, src, word); out != "[sub/.y][sub/x.txt]" {
		t.Errorf("against the word: %q", out)
	}
	if out, _ := run(t, src, entry); out != "[sub/.y]" {
		t.Errorf("against the entry: %q", out)
	}
	// And the facility is a state an assignment latched, or the parameter as
	// it stands. An inherited value is where the two part without a script
	// being able to see it.
	latched := func(r *Runner) {
		ignoring(dir)(r)
		r.Semantics.IgnoredNamesFollowTheParameter = No
		r.Env = []string{"GLOBIGNORE=*.txt"}
	}
	follows := func(r *Runner) {
		ignoring(dir)(r)
		r.Semantics.IgnoredNamesFollowTheParameter = Yes
		r.Env = []string{"GLOBIGNORE=*.txt"}
	}
	src = `printf "[%s]" *`
	if out, _ := run(t, src, latched); out != "[a.txt][b.txt][c.log][sub]" {
		t.Errorf("latched: %q", out)
	}
	if out, _ := run(t, src, follows); out != "[.dot][c.log][sub]" {
		t.Errorf("following the parameter: %q", out)
	}
	// A null value is the other half of the same axis: it shows the hidden
	// names where a latched assignment leaves the switch alone.
	nullValue := `GLOBIGNORE=''; printf "[%s]" *`
	if out, _ := run(t, nullValue, func(r *Runner) {
		ignoring(dir)(r)
		r.Semantics.IgnoredNamesFollowTheParameter = Yes
	}); out != "[.dot][.hid.txt][a.txt][b.txt][c.log][sub]" {
		t.Errorf("a null value following the parameter: %q", out)
	}
	if out, _ := run(t, nullValue, func(r *Runner) {
		ignoring(dir)(r)
		r.Semantics.IgnoredNamesFollowTheParameter = No
	}); out != "[a.txt][b.txt][c.log][sub]" {
		t.Errorf("a null value latched: %q", out)
	}
}

// And the same for the listing, which is not the facility's question at all:
// three of the six columns put `.` and `..` beside the entries a directory
// holds and three do not.
func TestTheListingHasDotAndDotDotOrItDoesNot(t *testing.T) {
	dir := ignoreTree(t)
	with := func(r *Runner) {
		r.Dir = dir
		r.Semantics.GlobListsDotAndDotDot = Yes
	}
	without := func(r *Runner) {
		r.Dir = dir
		r.Semantics.GlobListsDotAndDotDot = No
	}
	if out, _ := run(t, `printf "[%s]" .*`, with); out != "[.][..][.dot][.hid.txt]" {
		t.Errorf("with: %q", out)
	}
	if out, _ := run(t, `printf "[%s]" .*`, without); out != "[.dot][.hid.txt]" {
		t.Errorf("without: %q", out)
	}
	// The leading-period rule is what keeps them out of an ordinary `*`, so
	// the axis changes nothing there.
	for _, set := range []func(*Runner){with, without} {
		if out, _ := run(t, `printf "[%s]" *`, set); out != "[a.txt][b.txt][c.log][sub]" {
			t.Errorf("an ordinary star: %q", out)
		}
	}
}
