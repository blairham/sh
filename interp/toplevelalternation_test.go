// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// PatternTopLevelAlternation, both answers — #1497, where a `|` that arrived
// from an expansion was an alternation inside a group and a character outside
// one.
//
// Named for the flag rather than for the shell that sets it. The value is what
// carries the bar: the written spelling is a parse error in every shell
// measured, so this is a rule about a pattern a live expansion supplied.

// topLevelMatch runs a `case` whose arm is an expansion of `value`, under a
// grammar with the flag set or not, and answers whether the arm was taken.
//
// Through a `case` rather than through the matcher's own entry point because
// the *provenance* is the question: the arm's bar has to have arrived from a
// value, and an expansion result is only a live pattern where the vector says
// so.
func topLevelMatch(t *testing.T, subject, value string, topLevel bool) string {
	t.Helper()
	d := syntax.Core()
	d.PatternAlternation, d.PatternTopLevelAlternation = true, topLevel
	src := `L='` + value + `'; case "` + subject + `" in $L) echo yes;; *) echo no;; esac`
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "parse: " + err.Error()
	}
	sem := permissive()
	// The result of an expansion is a pattern here, which is the axis that
	// puts a live bar in a pattern at all.
	sem.GlobExpansionResults = Yes
	var out bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &sem, Env: testPATH()})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

func TestATopLevelBarFromAValueIsAnAlternation(t *testing.T) {
	for _, c := range []struct {
		name, subject, value string
		on, off              string
	}{
		{name: "the first arm", subject: "a", value: "a|b", on: "yes", off: "no"},
		{name: "the second arm", subject: "b", value: "a|b", on: "yes", off: "no"},
		{name: "neither arm", subject: "c", value: "a|b", on: "no", off: "no"},
		{
			// The row that says it is a split rather than an extra
			// character: with the flag on, the text the value holds stops
			// matching itself.
			name: "the value's own text", subject: "a|b", value: "a|b", on: "no", off: "yes",
		},
		{name: "an arm with a star in it", subject: "axx", value: "a*|b", on: "yes", off: "no"},
		{
			// An empty arm is allowed and matches the empty string, which is
			// measured: `L='a|'` matches both `a` and nothing.
			name: "an empty arm", subject: "", value: "a|", on: "yes", off: "no",
		},
		{name: "a bar alone", subject: "", value: "|", on: "yes", off: "no"},
		{
			// Inside a bracket the bar is an ordinary member, so a bracket is
			// stepped over exactly as a group is.
			name: "a bar inside a bracket is a member", subject: "|", value: "[a|b]", on: "yes", off: "yes",
		},
		{name: "and the bracket's other members still match", subject: "a", value: "[a|b]", on: "yes", off: "yes"},
		{
			name:    "a bar inside a bracket in the middle of a pattern",
			subject: "x|y", value: "x[a|b]y", on: "yes", off: "yes",
		},
		{
			// A group still binds tighter than the top level: `(a|b)c`
			// matches `ac`, and its bar is the group's.
			name: "a group's bar is still the group's", subject: "ac", value: "(a|b)c", on: "yes", off: "yes",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := topLevelMatch(t, c.subject, c.value, true); got != c.on {
				t.Errorf("with the flag: %q against %q = %s, want %s", c.subject, c.value, got, c.on)
			}
			if got := topLevelMatch(t, c.subject, c.value, false); got != c.off {
				t.Errorf("without it: %q against %q = %s, want %s", c.subject, c.value, got, c.off)
			}
		})
	}
}

// A quoted bar is untouched, which is what keeps the split a rule about live
// text: the escaping is the only thing that still says where a character came
// from by the time the matcher has the pattern.
func TestAQuotedBarIsNotAnAlternation(t *testing.T) {
	d := syntax.Core()
	d.PatternAlternation, d.PatternTopLevelAlternation = true, true
	sem := permissive()
	sem.GlobExpansionResults = Yes
	for _, tc := range []struct{ name, src, want string }{
		{"a quoted pattern", `case "a" in "a|b") echo yes;; *) echo no;; esac`, "no"},
		{"and it matches its own text", `case "a|b" in "a|b") echo yes;; *) echo no;; esac`, "yes"},
		{"a quoted bar inside a live value", `L='a\|b'; case "a|b" in $L) echo yes;; *) echo no;; esac`, "yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, d)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var out bytes.Buffer
			r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &sem, Env: testPATH()})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(out.String()); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}
