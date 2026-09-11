// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"regexp"
	"strings"
	"testing"
)

// A declaration letter with no names is a *filtered listing*: the names
// carrying the attribute, each written the way `declare -p` writes it.
// Measured 2026-09-10 against bash 5.3.15 at /opt/homebrew/bin/bash,
// `--norc --noprofile -c` with a scrubbed environment.
//
// It wrote nothing at all until #1868, silently and at status 0 — the worst
// pair, since `declare -x` is a common way to dump an environment as
// re-readable declarations and a script asking which of its names are arrays
// was told *none* rather than told the question could not be answered.
//
// Two letters together is the row that discriminates, and it is the one the
// three shells answer three ways: here the *kind* letters narrow and the rest
// join — see interp.DeclarationFilterKindNarrowsAny.

// declared is the table every case below lists out of: one name per
// combination that a union, an intersection and a narrowing sort differently.
const declared = "qa=1\n" +
	"export qb=2\n" +
	"declare -i qc=3\n" +
	"declare -a qd=(p)\n" +
	"declare -ai qe=(4)\n" +
	"declare -xi qf=6\n" +
	"declare -r qg=7\n"

// qRows keeps the rows naming the table's own parameters, so that a listing
// of the whole environment can be compared as *bytes* — which is what lets a
// case assert an absence, and what catches a row that grew a command word or
// lost one.
var qRow = regexp.MustCompile(`(^|[ =(])q[a-z]([ =]|$)`)

func qRows(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		if qRow.MatchString(line) {
			keep = append(keep, line)
		}
	}
	if len(keep) == 0 {
		return ""
	}
	return strings.Join(keep, "\n") + "\n"
}

func TestADeclarationLetterWithNoNamesListsWhatCarriesIt(t *testing.T) {
	for _, c := range []struct{ name, line, want string }{
		{"exported", "declare -x", "declare -x qb=\"2\"\ndeclare -ix qf=\"6\"\n"},
		{"arrays", "declare -a", "declare -a qd=([0]=\"p\")\ndeclare -ai qe=([0]=\"4\")\n"},
		{"integers", "declare -i", "declare -i qc=\"3\"\ndeclare -ai qe=([0]=\"4\")\ndeclare -ix qf=\"6\"\n"},
		{"read-only", "declare -r", "declare -r qg=\"7\"\n"},
		// `typeset` is the same builtin under the other word.
		{"the other word", "typeset -x", "declare -x qb=\"2\"\ndeclare -ix qf=\"6\"\n"},

		// Two letters. The kind letter narrows: `-ai` is the array that is
		// also an integer, and `-ax` is nothing at all because no array here
		// is exported.
		{"a kind letter narrows", "declare -ai", "declare -ai qe=([0]=\"4\")\n"},
		{"a kind letter narrowing to nothing", "declare -ax", ""},
		{"two kind letters", "declare -aA", ""},
		// The rest join, which is the opposite reading on the same table:
		// every integer *and* every read-only name.
		{
			"two attribute letters join", "declare -ir",
			"declare -i qc=\"3\"\ndeclare -ai qe=([0]=\"4\")\ndeclare -ix qf=\"6\"\ndeclare -r qg=\"7\"\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), declared+c.line)
			if qRows(out) != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", c.line, qRows(out), st, c.want)
			}
		})
	}
}

// A name that is typed and holds nothing is a row of it too, which is the
// state a listing is likeliest to drop: the kind is all there is and nothing
// scalar records it. `local -a` is the route a script takes to it.
func TestAValuelessLocalArrayIsAmongTheFilteredRows(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "f() { local -a qz\ndeclare -a\n}\nf")
	if want := "declare -a qz\n"; qRows(out) != want || st != 0 {
		t.Errorf("declare -a = %q (status %d), want %q", qRows(out), st, want)
	}
}
