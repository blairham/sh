// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"regexp"
	"strings"
	"testing"
)

// A declaration letter with no names is a *filtered listing* here too, and
// the row drops the command word the way a bare `export` does — `typeset -p
// qd` writes `typeset -a qd=(p)` where `typeset -a` writes `qd=(p)`.
// Measured 2026-09-10 against ksh93 (Version AJM 93u+ 2012-08-01) at
// /bin/ksh, `-c` with a scrubbed environment.
//
// Two letters together *intersect* here, which is the opposite of both other
// shells on the same table: `typeset -xi` writes the one name that is
// exported *and* an integer, and `typeset -ir` writes nothing at all where
// bash writes four rows. See interp.DeclarationFilterEveryLetter. All of it
// wrote nothing until #1868.

const declared = "qa=1\n" +
	"export qb=2\n" +
	"typeset -i qc=3\n" +
	"typeset -a qd=(p)\n" +
	"typeset -xi qf=6\n" +
	"typeset -r qg=7\n"

var qRow = regexp.MustCompile(`(^|[ =(])q[a-z]([ =]|$)`)

// qRows keeps the rows naming the table's own parameters, so the listing is
// compared as bytes rather than by containment.
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

func TestATypesetLetterWithNoNamesListsWhatCarriesIt(t *testing.T) {
	for _, c := range []struct{ name, line, want string }{
		{"exported", "typeset -x", "qb=2\nqf=6\n"},
		{"arrays", "typeset -a", "qd=(p)\n"},
		{"integers", "typeset -i", "qc=3\nqf=6\n"},
		{"read-only", "typeset -r", "qg=7\n"},

		// Every letter must hold, which is what tells this reading from the
		// other two: the union of these pairs is four rows and three.
		{"two letters intersect", "typeset -xi", "qf=6\n"},
		{"and intersect to nothing", "typeset -ir", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), declared+c.line)
			if qRows(out) != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", c.line, qRows(out), st, c.want)
			}
		})
	}
}

// A gap keeps its subscript here where the other plain-row shell writes the
// empty elements out, which is the half of the row the dialect decides: this
// is that shell's own `-p` value with the command word taken off.
func TestAFilteredRowSpellsACompoundTheWayThisShellPrintsOne(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "typeset -a qd\nqd[3]=x\ntypeset -a")
	if want := "qd=([3]=x)\n"; qRows(out) != want || st != 0 {
		t.Errorf("typeset -a = %q (status %d), want %q", qRows(out), st, want)
	}
}
