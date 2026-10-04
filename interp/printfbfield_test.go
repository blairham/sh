// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestABFieldIsPaddedOrRefused: a `%b` with a width is a padded field under
// one answer and a conversion the shell does not have under the other, which
// stops the format where it stands.
func TestABFieldIsPaddedOrRefused(t *testing.T) {
	for _, tc := range []struct {
		takes Answer
		want  string
	}{{Yes, "[   ab][cd]st=0"}, {No, "[st=1"}} {
		s := permissive()
		s.PrintfBTakesAField = tc.takes
		out, _ := run(t, `printf '[%5b][%s]' ab cd; echo "st=$?"`, withSem(s))
		if !strings.Contains(out, tc.want) {
			t.Errorf("takes=%v: said %q, want %q in it", tc.takes, out, tc.want)
		}
	}
	// A bare `%b` asks nothing.
	s := permissive()
	s.PrintfBTakesAField = Unspecified
	if out, _ := run(t, `printf '[%b]' ab`, withSem(s)); out != "[ab]" {
		t.Errorf("bare %%b with the axis unanswered: said %q", out)
	}
}

// TestACharConstantBehindBlanks: `" 'A"` is 65 to an integer conversion that
// reads through the blanks, and a bad number to one that does not.
func TestACharConstantBehindBlanks(t *testing.T) {
	for _, tc := range []struct {
		through Answer
		want    string
	}{{Yes, "65|"}, {No, "0|"}} {
		s := permissive()
		s.PrintfIntegerCharConstantAfterBlanks = tc.through
		s.PrintfNumberOperand = PrintfNumberWholeOperand
		out, _ := run(t, `printf '%d|' " 'A"`, withSem(s))
		if !strings.Contains(out, tc.want) {
			t.Errorf("through=%v: said %q, want %q in it", tc.through, out, tc.want)
		}
	}
}

// TestABadVerbNamesTheRestOfTheFormat: where the dialect says so, the
// complaint quotes the format from the `%` to its end rather than the
// directive alone.
func TestABadVerbNamesTheRestOfTheFormat(t *testing.T) {
	for _, tc := range []struct {
		rest bool
		want string
	}{{true, "%kb]: invalid format"}, {false, "%k: invalid format"}} {
		out, _ := run(t, `printf '[%kb]'`, func(r *Runner) {
			s := permissive()
			r.Semantics = &s
			r.Diagnostics = &Diagnostics{PrintfBadVerb: "%[2]s: invalid format", PrintfBadVerbNamesTheRestOfTheFormat: tc.rest}
		})
		if !strings.Contains(out, tc.want) {
			t.Errorf("rest=%v: said %q, want %q in it", tc.rest, out, tc.want)
		}
	}
}
