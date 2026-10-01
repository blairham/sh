// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// readNul runs src with NulInAValue set to p, and `-d` and `-n` spelled.
func readNul(t *testing.T, p NulInAValuePolicy, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		s := *r.Semantics
		s.NulInAValue = p
		s.ReadOptions = "rd:n:"
		r.Semantics = &s
	})
}

// **A record with no NUL in it asks nothing**, and neither does a NUL that is
// the delimiter: `read -d` with an empty argument ends the record at it, which is a reading of the
// option rather than of the byte.
func TestAReadWithNoNulInItAsksNothing(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"read -r v <<END\nplain\nEND\necho \"v=$v\"", "v=plain\n"},
		{`printf 'a\0b\0' > f; read -r -d '' v < f; echo "v=$v"`, "v=a\n"},
	} {
		out, _ := readNul(t, NulInAValueUnspecified, c.src)
		if !strings.HasSuffix(out, c.want) || strings.Contains(out, "disagree") {
			t.Errorf("%s gave %q, want it to end %q and refuse nothing", c.src, out, c.want)
		}
	}
}

// **A NUL in the record is where the axis is asked**, and each answer does
// what it says — the drop happening before a counted read counts.
func TestANulInTheRecordIsWhereTheAxisIsAsked(t *testing.T) {
	const src = `printf 'ab\0cd\n' > f; read -r v < f; echo "len=${#v}"; read -r -n 3 w < f; echo "w=$w"`
	if out, _ := readNul(t, NulInAValueUnspecified, `printf 'ab\0cd\n' > f; read -r v < f; echo "st=$?"`); !strings.Contains(out, "the shells disagree here and no dialect was chosen") || !strings.HasSuffix(out, "st=2\n") {
		t.Errorf("unanswered: %q, want the refusal and status 2", out)
	}
	for _, c := range []struct {
		p    NulInAValuePolicy
		want string
	}{
		{NulDropped, "len=4\nw=abc\n"},
		{NulCutInASubstitution, "len=4\nw=abc\n"},
		{NulKept, "len=5\nw=ab\x00\n"},
	} {
		if out, _ := readNul(t, c.p, src); out != c.want {
			t.Errorf("%v: %q, want %q", c.p, out, c.want)
		}
	}
}
