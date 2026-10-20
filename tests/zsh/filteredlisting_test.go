// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
	"strings"
	"testing"
)

// A declaration letter with no names is a *filtered listing* here too, and
// the row is not this shell's `typeset -p` row: the command word goes, the
// same way it goes from a bare `export`. Measured 2026-09-10 against zsh
// 5.9.2 at /opt/homebrew/bin/zsh, `-f` with no startup files and a scrubbed
// environment — `typeset -p qd` writes `typeset -a qd=( p )` where
// `typeset -a` writes `qd=( p )`.
//
// Two letters together *join*, kind letters included, which is the reading
// that parts company with both other shells on the same table — see
// interp.DeclarationFilterAnyLetter. It wrote nothing at all until #1868.

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
		{"arrays", "typeset -a", "qd=( p )\n"},
		{"integers", "typeset -i", "qc=3\nqf=6\n"},
		{"read-only", "typeset -r", "qg=7\n"},
		{"the other word", "declare -x", "qb=2\nqf=6\n"},

		// Every letter joins, the kind letter among them: `-ax` is every
		// array *and* every exported name, where the same line elsewhere is
		// the arrays that are exported.
		{"a kind letter joins", "typeset -ax", "qb=2\nqd=( p )\nqf=6\n"},
		{"two attribute letters join", "typeset -xi", "qb=2\nqc=3\nqf=6\n"},
		{"the other pair", "typeset -ir", "qc=3\nqf=6\nqg=7\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), declared+c.line)
			if qRows(out) != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", c.line, qRows(out), st, c.want)
			}
		})
	}
}

// The minus form values what the plus form names, and the two must not answer
// alike: a minus reaching the names listing would read as a working `-x` to
// anything consuming it.
func TestTheTwoSignsOfAttributeLetterAreTwoListings(t *testing.T) {
	plus, st := runZsh(t, t.TempDir(), declared+"typeset +x")
	if want := "qb\nqf\n"; qRows(plus) != want || st != 0 {
		t.Fatalf("typeset +x = %q (status %d), want %q", qRows(plus), st, want)
	}
	minus, st := runZsh(t, t.TempDir(), declared+"typeset -x")
	if want := "qb=2\nqf=6\n"; qRows(minus) != want || st != 0 {
		t.Fatalf("typeset -x = %q (status %d), want %q", qRows(minus), st, want)
	}
}

// A bare `export` and a bare `readonly` write the same row, which is how the
// compound half of it is measured from a second direction: this wrote a bare
// `qd` for an exported array until #1868, where the shell writes its elements.
func TestABareExportWritesACompoundValue(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "typeset -ax qd=(p q)\nexport")
	if want := "qd=( p q )\n"; qRows(out) != want || st != 0 {
		t.Errorf("export = %q (status %d), want %q", qRows(out), st, want)
	}
	out, st = runZsh(t, t.TempDir(), "typeset -ar qd=(p q)\nreadonly")
	if want := "qd=( p q )\n"; qRows(out) != want || st != 0 {
		t.Errorf("readonly = %q (status %d), want %q", qRows(out), st, want)
	}
}
