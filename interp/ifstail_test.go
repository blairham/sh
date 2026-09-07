// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The tail of the field-splitting rule: whether the non-whitespace separator
// that closes a value opens one last empty field.
//
// Measured 2026-09-07 with `IFS=:` against bash 5.3.15, bash 3.2.57, that
// build invoked as `sh`, dash, ksh93u+ 2012-08-01 and zsh 5.9.2 — the last
// under `setopt shwordsplit`, which is the only way to ask it there. Five
// shells absorb the separator and zsh delimits on it, so every split of such a
// value is one field short of zsh's.
//
// Every row asserts the exact field *count* with the exact field values,
// because the wrong answer here is a plausible count at status 0: an
// assertion that the command did not fail passes against it, and so does one
// that reads the fields back without counting them, since `[a]` and `[a][]`
// differ only in a boundary.

// tailRun runs src with the splitting stage on and the tail answered, and
// nothing else moved.
func tailRun(t *testing.T, src string, tail interp.Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *interp.Semantics) {
		s.SplitParamExpansion = interp.Yes
		s.GlobExpansionResults = interp.Yes
		s.UnquotedListJoinsOnIFS = interp.No
		s.TrailingSeparatorEndsAField = tail
	})
}

// The rows the issue is about, on the scalar path — which is the anchor, since
// the list readings are built out of it.
func TestATrailingSeparatorOpensAFieldOrIsAbsorbed(t *testing.T) {
	for _, tc := range []struct{ name, value, absorbed, opens string }{
		{"one separator at the tail", `a:`, `1:[a]`, `2:[a][]`},
		{"two at the tail", `a::`, `2:[a][]`, `3:[a][][]`},
		{"a separator between fields and one at the tail", `a:b:`, `2:[a][b]`, `3:[a][b][]`},
		{"both ends", `:a:`, `2:[][a]`, `3:[][a][]`},
		{"nothing but one separator", `:`, `1:[]`, `2:[][]`},
		{"nothing but two", `::`, `2:[][]`, `3:[][][]`},
		// The guards. A leading separator opens a field in all six shells,
		// so a value that has one and no trailing one must not move; and an
		// empty value is no field at all in all six, including zsh, so the
		// answer must not put one there.
		{"a leading separator alone", `:a`, `2:[][a]`, `2:[][a]`},
		{"no separator at all", `a`, `1:[a]`, `1:[a]`},
		{"nothing", ``, `0:[]`, `0:[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := count + `IFS=:; v="` + tc.value + `"; f $v`
			if out, st := tailRun(t, src, interp.No); out != tc.absorbed || st != 0 {
				t.Errorf("absorbed: got %q status %d, want %q at 0", out, st, tc.absorbed)
			}
			if out, st := tailRun(t, src, interp.Yes); out != tc.opens || st != 0 {
				t.Errorf("opens a field: got %q status %d, want %q at 0", out, st, tc.opens)
			}
		})
	}
}

// It is the closing *run* of separators that decides and not the last byte,
// which only a mixed IFS can show: with `IFS=' :'` the trailing space does not
// hide the colon in front of it, and a value ending in whitespace alone is
// absorbed under either answer.
//
// zsh 5.9.2: `'a: '` is `[a][]` and `'a  '` is `[a]`. A reading that looked at
// the last character would give `[a]` for the first row and answer the second
// one correctly by accident.
func TestTheTailIsTheRunAndNotTheLastByte(t *testing.T) {
	for _, tc := range []struct{ name, value, absorbed, opens string }{
		{"a separator then whitespace", `a: `, `1:[a]`, `2:[a][]`},
		{"whitespace then a separator", `a :`, `1:[a]`, `2:[a][]`},
		{"a separator then more whitespace", `a:  `, `1:[a]`, `2:[a][]`},
		{"whitespace alone", `a  `, `1:[a]`, `1:[a]`},
		{"whitespace at both ends", ` a `, `1:[a]`, `1:[a]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := count + `IFS=" :"; v="` + tc.value + `"; f $v`
			if out, st := tailRun(t, src, interp.No); out != tc.absorbed || st != 0 {
				t.Errorf("absorbed: got %q status %d, want %q at 0", out, st, tc.absorbed)
			}
			if out, st := tailRun(t, src, interp.Yes); out != tc.opens || st != 0 {
				t.Errorf("opens a field: got %q status %d, want %q at 0", out, st, tc.opens)
			}
		})
	}
}

// The axis is asked only where the two readings differ, which is what keeps a
// script that never sets IFS from demanding a dialect for it.
//
// Left unanswered on purpose: an unanswered axis that is asked says so and
// refuses, so the fields coming back at status 0 are the whole assertion.
func TestAValueWithNoSeparatorAtTheTailNeverAsks(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a whitespace IFS", count + `v="a b"; f $v`, `2:[a][b]`},
		{"whitespace at the tail", count + `v="a b "; f $v`, `2:[a][b]`},
		{"a separator in the middle only", count + `IFS=:; v="a:b"; f $v`, `2:[a][b]`},
		{"a leading separator only", count + `IFS=:; v=":ab"; f $v`, `2:[][ab]`},
		{"whitespace alone at the tail", count + `IFS=" :"; v="a "; f $v`, `1:[a]`},
		// Set and empty disables the stage, so there is no tail to ask about
		// even though the value ends in what IFS would otherwise name.
		{"IFS set and empty", count + `IFS=; v="a:"; f $v`, `1:[a:]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := tailRun(t, tc.src, interp.Unspecified)
			if strings.Contains(out, "no dialect was chosen") {
				t.Errorf("asked the tail where it changes nothing: %q", out)
			}
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// And it *is* asked where the readings differ, which is the other half of the
// same claim: a dialect that never answered this must be told so rather than
// handed one of the two readings.
func TestAnUnansweredTailIsRefusedByName(t *testing.T) {
	out, _ := tailRun(t, count+`IFS=:; v="a:"; f $v`, interp.Unspecified)
	if !strings.Contains(out, "a trailing IFS separator opening a field of its own") {
		t.Errorf("got %q, want the axis refused by name", out)
	}
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("got %q, want the refusal to say no dialect was chosen", out)
	}
}

// The list readings are the scalar one applied per element, so the tail
// question reaches each of them: an element that ends in a separator opens a
// field of its own where the answer is yes.
//
// zsh 5.9.2 under `setopt shwordsplit`, which does not join: `a=("x:" y)` is
// `[x][][y]` and `a=(x "y:")` is `[x][y][]`. The first coincides with what
// bash's *join* produces for the same elements, which is why the second row
// is here — no join can put a field after the last element.
func TestTheTailReachesEachElementOfAList(t *testing.T) {
	for _, tc := range []struct{ name, src, absorbed, opens string }{
		{
			"an element ending in a separator",
			count + `IFS=:; set -- "x:" y; f $@`,
			`2:[x][y]`, `3:[x][][y]`,
		},
		{
			"the last element ending in one",
			count + `IFS=:; set -- x "y:"; f $@`,
			`2:[x][y]`, `3:[x][y][]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := tailRun(t, tc.src, interp.No); out != tc.absorbed || st != 0 {
				t.Errorf("absorbed: got %q status %d, want %q at 0", out, st, tc.absorbed)
			}
			if out, st := tailRun(t, tc.src, interp.Yes); out != tc.opens || st != 0 {
				t.Errorf("opens a field: got %q status %d, want %q at 0", out, st, tc.opens)
			}
		})
	}
}

// A quoted `${=spec}` keeps the fields at *both* edges unconditionally, which
// is a rule of its own and not this axis. Where it is in force the field
// behind the last separator is already there, so the tail must not be asked —
// and must not add a second one.
//
// Left unanswered in every row, so an ask would announce itself.
func TestTheEdgeKeepingSplitFlagDoesNotAskTheTail(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a separator at the tail", count + `IFS=:; v="a:"; f "${=v}"`, `2:[a][]`},
		{"two at the tail", count + `IFS=:; v="a::"; f "${=v}"`, `3:[a][][]`},
		{"nothing but a separator", count + `IFS=:; v=":"; f "${=v}"`, `2:[][]`},
		{"an empty value", count + `IFS=:; v=""; f "${=v}"`, `1:[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, splitGrammar, func(r *interp.Runner) {
				sem := interp.CoreSemantics()
				sem.SplitParamExpansion = interp.No
				sem.GlobExpansionResults = interp.No
				r.Semantics = &sem
			})
			if strings.Contains(out, "no dialect was chosen") {
				t.Errorf("asked an axis the edge-keeping rule settles: %q", out)
			}
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The unquoted spelling of the same flag runs the ordinary rules, so it does
// ask — the flag turns splitting on and the tail question is the splitter's.
func TestTheUnquotedSplitFlagAsksTheTail(t *testing.T) {
	const src = count + `IFS=:; v="a:"; f ${=v}`
	for _, tc := range []struct {
		tail interp.Answer
		want string
	}{
		{interp.No, `1:[a]`},
		{interp.Yes, `2:[a][]`},
	} {
		out, st := runGrammar(t, src, splitGrammar, func(r *interp.Runner) {
			sem := interp.CoreSemantics()
			sem.SplitParamExpansion = interp.No
			sem.GlobExpansionResults = interp.No
			sem.TrailingSeparatorEndsAField = tc.tail
			r.Semantics = &sem
		})
		if out != tc.want || st != 0 {
			t.Errorf("tail=%v: got %q status %d, want %q at 0", tc.tail, out, st, tc.want)
		}
	}
}

// `read` feeds the same splitter, and an array target takes what comes out
// *as fields* — so the count moves with the answer there too.
//
// zsh 5.9.2: `IFS=:; printf 'a:b:\n' | read -A arr` fills three elements
// where bash's `read -a` fills two. And the escape mask has to reach the
// question: `a\:` is the one element `a:` under either answer, because an
// escaped separator is data and is not part of the closing run at all.
func TestReadIntoAnArrayAsksTheTail(t *testing.T) {
	readRun := func(t *testing.T, src string, tail interp.Answer) (string, int) {
		t.Helper()
		return runGrammar(t, src, func(d *syntax.Dialect) {
			d.ArraySubscript = true
			d.ArrayLiteral = true
		}, func(r *interp.Runner) {
			sem := interp.CoreSemantics()
			sem.ReadOptions = "rA"
			// `read` needs a line to read and the last element of a
			// pipeline has to run here for its variable to survive, which
			// is an axis of its own and answered so these rows reach the
			// one they are about.
			sem.LastPipelineElementInCurrentShell = interp.Yes
			sem.TrailingSeparatorEndsAField = tail
			r.Semantics = &sem
		})
	}
	for _, tc := range []struct{ name, src, absorbed, opens string }{
		{
			"a trailing separator",
			`IFS=:; printf 'a:b:\n' | { read -A arr; printf "%d:" "${#arr[@]}"; printf "[%s]" "${arr[@]}"; }`,
			`2:[a][b]`, `3:[a][b][]`,
		},
		{
			"an escaped one is data",
			`IFS=:; printf 'a\\:\n' | { read -A arr; printf "%d:" "${#arr[@]}"; printf "[%s]" "${arr[@]}"; }`,
			`1:[a:]`, `1:[a:]`,
		},
		{
			"and raw, the backslash is a character of the field",
			`IFS=:; printf 'a\\:\n' | { read -rA arr; printf "%d:" "${#arr[@]}"; printf "[%s]" "${arr[@]}"; }`,
			`1:[a\]`, `2:[a\][]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := readRun(t, tc.src, interp.No); out != tc.absorbed || st != 0 {
				t.Errorf("absorbed: got %q status %d, want %q at 0", out, st, tc.absorbed)
			}
			if out, st := readRun(t, tc.src, interp.Yes); out != tc.opens || st != 0 {
				t.Errorf("opens a field: got %q status %d, want %q at 0", out, st, tc.opens)
			}
		})
	}
}

// A list of names takes the last one as the remainder of the *line*, so the
// extra field the tail can open changes a value there only at one count: where
// the line held exactly one field per name. One more and the remainder is the
// same text under either answer; one fewer and the name gets the empty string
// either way. `read`'s names ask at that count and nowhere else, which is what
// keeps the axis off `IFS=: read -r user rest` on an ordinary passwd line.
//
// Measured 2026-09-07: `IFS=:; printf 'a:b:\n' | { read -r x y; }` gives `b`
// in five shells and `b:` in zsh, and it is the same fact as the array row
// above seen through a different target.
func TestReadIntoNamesAsksTheTailOnlyAtTheCount(t *testing.T) {
	readRun := func(t *testing.T, src string, tail interp.Answer) (string, int) {
		t.Helper()
		return run(t, src, func(r *interp.Runner) {
			sem := interp.CoreSemantics()
			sem.LastPipelineElementInCurrentShell = interp.Yes
			sem.TrailingSeparatorEndsAField = tail
			r.Semantics = &sem
		})
	}
	for _, tc := range []struct{ name, src, absorbed, opens string }{
		{
			"one field per name, and the last one is a field",
			`IFS=:; printf 'a:b:\n' | { read -r x y; printf "[%s][%s]" "$x" "$y"; }`,
			`[a][b]`, `[a][b:]`,
		},
		{
			"one name and one field is the same count",
			`IFS=:; printf 'a:\n' | { read -r l; printf "[%s]" "$l"; }`,
			`[a]`, `[a:]`,
		},
		{
			"a line of nothing but separators reaches it too",
			`IFS=:; printf '::\n' | { read -r x y; printf "[%s][%s]" "$x" "$y"; }`,
			`[][]`, `[][:]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := readRun(t, tc.src, interp.No); out != tc.absorbed || st != 0 {
				t.Errorf("absorbed: got %q status %d, want %q at 0", out, st, tc.absorbed)
			}
			if out, st := readRun(t, tc.src, interp.Yes); out != tc.opens || st != 0 {
				t.Errorf("opens a field: got %q status %d, want %q at 0", out, st, tc.opens)
			}
		})
	}
	// The guard, and the half a test of the two answers alone cannot see:
	// away from that count the question is not put at all, so the *core* —
	// which answers it with nothing — still runs these without a word and
	// without a refusal.
	for _, tc := range []struct{ name, src, want string }{
		{
			"more fields than names: the remainder holds the run either way",
			`IFS=:; printf 'a:b:c::\n' | { read -r x y; printf "[%s][%s]" "$x" "$y"; }`,
			`[a][b:c::]`,
		},
		{
			"fewer fields than names: the name it would fill is empty either way",
			`IFS=:; printf 'a:\n' | { read -r x y z; printf "[%s][%s][%s]" "$x" "$y" "$z"; }`,
			`[a][][]`,
		},
		{
			"a whitespace IFS never reaches it",
			`printf 'a b \n' | { read -r x y; printf "[%s][%s]" "$x" "$y"; }`,
			`[a][b]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := readRun(t, tc.src, interp.Unspecified)
			if out != tc.want || st != 0 {
				t.Errorf("unanswered: got %q status %d, want %q at 0 with nothing said", out, st, tc.want)
			}
		})
	}
}

// The presets. POSIX.1-2024 2.6.5 answers this one — "once the input is empty,
// the candidate shall become an output field if and only if it is not empty" —
// so the specification's preset absorbs, and dash, bash and ksh93 comply.
//
// The core leaves it unanswered, because five shells against one is a
// disagreement and not a majority to be counted.
func TestThePresetsAnswerTheTailAsMeasured(t *testing.T) {
	if got := interp.PosixSemantics().TrailingSeparatorEndsAField; got != interp.No {
		t.Errorf("PosixSemantics().TrailingSeparatorEndsAField = %v, want %v", got, interp.No)
	}
	if got := interp.CoreSemantics().TrailingSeparatorEndsAField; got != interp.Unspecified {
		t.Errorf("CoreSemantics().TrailingSeparatorEndsAField = %v, want it unanswered", got)
	}
}

// A redirection target read as an ordinary word is read as *fields*, so the
// tail question reaches that view too: `IFS=:; v='a:'; echo hi > $v` is one
// field and a file named `a` under one answer, and two fields — hence no one
// place to write — under the other.
//
// Both axes are named, and the combination is deliberately one no dialect in
// the panel has: bash reads the target as a word and absorbs the separator,
// zsh does the opposite of each. The substrate answers per axis rather than
// per shell, which is exactly what a row nobody's preset reaches is for — and
// without it this call site is unreachable with the tail answered yes, so
// nothing would notice it going unasked.
func TestARedirectionTargetReadAsFieldsAsksTheTail(t *testing.T) {
	src := `IFS=:; v="a:"; echo hi > $v; echo "st=$?"`
	target := func(t *testing.T, tail interp.Answer) (string, string) {
		t.Helper()
		dir := t.TempDir()
		out, _ := run(t, src, func(r *interp.Runner) {
			sem := interp.CoreSemantics()
			sem.SplitParamExpansion = interp.Yes
			sem.GlobExpansionResults = interp.No
			sem.RedirectTargetIsAnOrdinaryWord = interp.Yes
			sem.TrailingSeparatorEndsAField = tail
			r.Semantics, r.Dir = &sem, dir
		})
		return out, dir
	}

	out, dir := target(t, interp.No)
	if !strings.Contains(out, "st=0") {
		t.Errorf("absorbed: got %q, want the redirection to have succeeded", out)
	}
	if got := readFile(t, dir, "a"); got != "hi\n" {
		t.Errorf("absorbed: a = %q, want the one field to be the name", got)
	}

	out, _ = target(t, interp.Yes)
	if !strings.Contains(out, "ambiguous redirect") {
		t.Errorf("opens a field: got %q, want two fields to be no one place to write", out)
	}
	if strings.Contains(out, "st=0") {
		t.Errorf("opens a field: got %q, want a non-zero status", out)
	}
}
