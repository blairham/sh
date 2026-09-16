// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `compadd`, measured through a pseudo-terminal against zsh 5.9.2 on
// 2026-09-15 from inside a `zle -C` widget's function, with `git che` typed —
// so `$PREFIX` is `che` — and the status and `$compstate[nmatches]` printed
// after each call. The table in compadd.go's file comment is this one.
func TestCompaddOffersWhatItMatched(t *testing.T) {
	for _, c := range []struct {
		name, call string
		want       []string
		status     string
	}{
		// The candidates are filtered against `$PREFIX` as they are added,
		// so `commit` never becomes a match and `nmatches` is 2 of the 3.
		{
			"filtered against the word", "compadd checkout cherry commit",
			[]string{"checkout", "cherry"},
			"st=0 nm=2",
		},
		// Nothing matched is status 1, which is what a completion function
		// tests to decide whether to try something else.
		{"nothing matched", "compadd zzz yyy", nil, "st=1 nm=0"},
		// `-P` is inserted and is *not* part of what is matched: `checkout`
		// still matched `che` with `XX` in front of it.
		{
			"-P is not part of the match", "compadd -P XX -- checkout cherry",
			[]string{"XXcheckout", "XXcherry"},
			"st=0 nm=2",
		},
		// `-p` is inserted and *is* part of what is matched, which is the
		// whole of the difference between the two — and the one the names do
		// not carry. `HIDcheckout` does not begin with `che`.
		{"-p is part of the match", "compadd -p HID -- checkout", nil, "st=1 nm=0"},
		// `-S` goes on after the match, unescaped: it is a delimiter the
		// function chose.
		{"-S follows the match", "compadd -S = -- checkout", []string{"checkout="}, "st=0 nm=1"},
		// `-U` turns the filtering off and nothing else.
		{"-U offers everything", "compadd -U -- zzz yyy", []string{"zzz", "yyy"}, "st=0 nm=2"},
		// A match is quoted for the line unless `-Q`, because what a
		// completer returns has to survive being read back by the parser.
		{"quoted for the line", "compadd -U -- 'a b'", []string{`a\ b`}, "st=0 nm=1"},
		{"-Q leaves it alone", "compadd -U -Q -- 'a b'", []string{"a b"}, "st=0 nm=1"},
		// `-a` reads the candidates out of the arrays the words name, which
		// is how a completion function offers a list it built earlier.
		{
			"-a takes them from an array", "local -a c=(checkout cherry); compadd -a c",
			[]string{"checkout", "cherry"},
			"st=0 nm=2",
		},
		// The letters that take an argument take it, so that what follows one
		// is never offered as a candidate. `group` is not a completion.
		{
			"an option's argument is not a candidate", "compadd -J group -X 'an explanation' -- checkout",
			[]string{"checkout"},
			"st=0 nm=1",
		},
		// The same letter written joined to its argument.
		{"joined argument", "compadd -Pxx -- checkout", []string{"xxcheckout"}, "st=0 nm=1"},
		// Two calls accumulate, which is what a completion function with
		// several sources of candidates relies on — and a name offered twice
		// is offered once.
		{
			"calls accumulate", "compadd checkout; compadd cherry checkout",
			[]string{"checkout", "cherry"},
			"st=0 nm=2",
		},
		// `-O` stores the matches in an array and offers none of them: it is
		// how a completion function asks "would any of these have matched"
		// without committing to them.
		{"-O withholds", "local -a got; compadd -O got -- checkout commit", nil, "st=1 nm=0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := completionFor(t, widgetOf(c.call), "git che")
			if strings.Join(got, " ") != strings.Join(c.want, " ") {
				t.Errorf("%s offered %q, want %q", c.call, got, c.want)
			}
		})
	}
}

// The status and `$compstate[nmatches]` are the other half of what a call
// says, and a completion function acts on both — `compadd … || _describe …`
// is how a fallback is written.
//
// Asked separately from the candidates above because they can disagree: `-U`
// offers a candidate that does not match what was typed, so it is status 0
// on a word it has nothing to do with, and that row is the reason this is not
// folded into one assertion.
//
// **The `-O` row is corrected here, and it was wrong in the direction that
// matters.** This file used to assert status 0 for a call that stores rather
// than offers, on the argument that `-O`'s caller wants to know whether
// anything matched. Measured again on zsh 5.9.2, 2026-09-15, from inside a
// widget with `PREFIX` of `al` and candidates `alpha zzz alright`, `-D`, `-O`
// and `-A` are **all 1** while filling their arrays with the two that match:
// the manual's sentence is about matches *added*, and nothing is added. It
// matters because the shipped `_arguments` writes three `compadd -D` calls in
// a row — see computil.go — and reads the status of none of them, so a 0
// there is a claim that something reached the line.
func TestCompaddReportsWhetherAnythingMatched(t *testing.T) {
	for _, c := range []struct{ call, want string }{
		{"compadd checkout cherry commit", "st=0 nm=2"},
		{"compadd zzz yyy", "st=1 nm=0"},
		{"compadd -p HID -- checkout", "st=1 nm=0"},
		{"compadd -U -- zzz", "st=0 nm=1"},
		{"local -a got; compadd -O got -- checkout commit", "st=1 nm=0"},
		// And `-D`, which strikes through rather than fills, and is 1 for
		// the same reason. What it does to the array is asserted in
		// computil_test.go, where the array can be read back.
		{"local -a d=(A B); compadd -D d -- checkout commit", "st=1 nm=0"},
	} {
		t.Run(c.call, func(t *testing.T) {
			got := completionFor(t, widgetOf(
				c.call+`; local s=$?; compadd -U -Q -- "st=$s nm=$compstate[nmatches]"`,
			),
				"git che")
			if len(got) == 0 || got[len(got)-1] != c.want {
				t.Errorf("%s reported %q, want %q", c.call, got, c.want)
			}
		})
	}
}

// `-O`'s array really is written, which is the half of that row the candidate
// assertion cannot see: a call that withholds everything and stores nothing
// would pass it.
func TestCompaddStoresWhatItWithheld(t *testing.T) {
	got := completionFor(t, widgetOf(
		"local -a kept; compadd -O kept -- checkout commit cherry; "+
			`compadd -U -Q -- "kept=${(j:,:)kept}"`,
	), "git che")
	if want := "kept=checkout,cherry"; len(got) != 1 || got[0] != want {
		t.Errorf("stored %q, want %q", got, want)
	}
}

// `-o` is the one letter whose argument is optional, and what decides is the
// word rather than its presence — see compaddOrders for the measurement.
//
// Asked with an empty word so that everything offered is counted: a case
// against `git che` could not tell an order eaten from an order offered and
// filtered, which is the shape of non-discriminating probe this repository
// keeps finding.
func TestCompaddReadsAnOrderOnlyWhenTheWordIsOne(t *testing.T) {
	for _, c := range []struct {
		name, call string
		want       []string
	}{
		{"an order is eaten", "compadd -o nosort -- alpha", []string{"alpha"}},
		{"every order is eaten", "compadd -o match -- alpha", []string{"alpha"}},
		{"`--` is not an order", "compadd -o -- alpha", []string{"alpha"}},
		{"a candidate is not an order", "compadd -o alpha", []string{"alpha"}},
		// And the row that pins it from the other side: a word that is not an
		// order ends the options, so the `--` after it is an ordinary
		// candidate and all three are offered.
		{"a word that is not an order", "compadd -o zzz -- alpha", []string{"zzz", "--", "alpha"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := completionFor(t, widgetOf(c.call), "git ")
			if strings.Join(got, " ") != strings.Join(c.want, " ") {
				t.Errorf("%s offered %q, want %q", c.call, got, c.want)
			}
		})
	}
}
