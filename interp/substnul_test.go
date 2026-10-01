// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// substNul runs src with NulInAValue set to p.
func substNul(t *testing.T, p NulInAValuePolicy, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		s := *r.Semantics
		s.NulInAValue = p
		r.Semantics = &s
	})
}

// **A substitution with no NUL in what it took in asks nothing**, through each
// spelling that captures output — the axis is asked where the byte arrives and
// not on the common path every `$( … )` takes.
func TestASubstitutionWithNoNulAsksNothing(t *testing.T) {
	for _, src := range []string{
		`v=$(printf 'a\nb\n\n'); echo "[$v]"`,
		"v=`echo a`; echo \"[$v]\"",
		`printf 'x\n' > f; v=$(<f); echo "[$v]"`,
	} {
		out, st := substNul(t, NulInAValueUnspecified, src)
		if strings.Contains(out, "disagree") || st != 0 || !strings.HasSuffix(out, "]\n") {
			t.Errorf("%s gave %q (status %d), want it to refuse nothing", src, out, st)
		}
	}
}

// **A NUL in a substitution is where the axis is asked**, and an unanswered
// vector refuses by name at status 2, with an empty value.
func TestANulInASubstitutionIsWhereTheAxisIsAsked(t *testing.T) {
	out, _ := substNul(t, NulInAValueUnspecified, `v=old; v=$(printf 'a\0b'); echo "st=$? v=[$v]"`)
	if !strings.Contains(out, "a NUL byte in what a command substitution takes in") ||
		!strings.Contains(out, "the shells disagree here and no dialect was chosen") ||
		!strings.HasSuffix(out, "st=2 v=[]\n") {
		t.Errorf("unanswered: %q, want the refusal and status 2", out)
	}
}

// **Each answer does what it says**, and the order it does it in is part of
// the answer: a dropped NUL goes before the trailing newlines do, so the
// newline it was holding off the end goes too; a NUL that stays stops the
// newlines and is then cut, once the field is whole.
func TestEachNulAnswerShapesTheValue(t *testing.T) {
	const src = `v=$(printf 'a\0b'); printf '<%s>' "$v" ${#v}; echo
v=$(printf 'a\n\0'); printf '<%s>' "$v"; echo
printf '<%s>' "x$(printf 'a\0b')y" $(printf 'p\0q r'); echo
v=abc; printf '<%s>' "${v#$(printf 'a\0b')}"; echo
printf 'f\0g\n' > in; v=$(<in); printf '<%s>' "$v"; echo`
	for _, c := range []struct {
		p    NulInAValuePolicy
		want string
	}{
		{NulDropped, "<ab><2>\n<a>\n<xaby><pq><r>\n<c>\n<fg>\n"},
		{NulCutInASubstitution, "<a><1>\n<a\n>\n<xa><p><r>\n<bc>\n<f>\n"},
		{NulKept, "<a\x00b><3>\n<a\n\x00>\n<xa\x00by><p\x00q><r>\n<abc>\n<f\x00g>\n"},
	} {
		if out, st := substNul(t, c.p, src); out != c.want || st != 0 {
			t.Errorf("%v: %q (status %d), want %q", c.p, out, st, c.want)
		}
	}
}

// **The cut takes the escape that marked the NUL and leaves a written
// backslash alone.** A field still carries its escaped form when it is cut,
// and a backslash the value held in front of the NUL is content.
func TestTheCutKeepsABackslashTheValueHeld(t *testing.T) {
	out, st := substNul(t, NulCutInASubstitution, `printf '<%s>' "$(printf 'a\\\0b')"z $(printf 'c\\\0d'); echo`)
	if want := "<a\\><c\\>\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// **A redirection target ends at the NUL too**, since a target is a word like
// any other: measured on ksh93u+, `echo hi > "$(printf 'f\0g')h"` writes the
// file `f`.
func TestTheCutReachesARedirectionTarget(t *testing.T) {
	out, st := substNul(t, NulCutInASubstitution, `: > "$(printf 'o\0g')h"; test -e o && echo cut; : > $(printf 'p\0q'); test -e p && echo cut`)
	if want := "cut\ncut\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
