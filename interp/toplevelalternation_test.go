// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
// carries the bar, and that is the rule rather than an accident of what can be
// written: a bar *written* in a `${…}` operand is not a parse error — the
// braces keep it out of the command grammar — and zsh reads it as an ordinary
// character there. This file used to say the written spelling was a parse
// error in every shell measured, which is true of a condition and of a `case`
// arm and false of the one place the question can be asked (#2168). See
// TestAWrittenBarIsNotAnAlternation below.

// topLevelMatch runs a `case` whose arm is an expansion of `value`, under a
// grammar with the flag set or not, and answers whether the arm was taken.
//
// Through a `case` rather than through the matcher's own entry point because
// the *provenance* is the question: the arm's bar has to have arrived from a
// value, and an expansion result is only a live pattern where the vector says
// so.
func topLevelMatch(t *testing.T, subject, value string, topLevel syntax.TopLevelAlternation) string {
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
			if got := topLevelMatch(t, c.subject, c.value, syntax.TopLevelAlternationFromAValue); got != c.on {
				t.Errorf("with the flag: %q against %q = %s, want %s", c.subject, c.value, got, c.on)
			}
			if got := topLevelMatch(t, c.subject, c.value, syntax.NoTopLevelAlternation); got != c.off {
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
	d.PatternAlternation, d.PatternTopLevelAlternation = true, syntax.TopLevelAlternationFromAValue
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

// A written bar is an ordinary character even where a live one splits, which
// is the other half of the same rule and the half nothing checked. Measured on
// zsh 5.9.2, 2026-09-12, with `v=abc` — and the whole point is that both
// spellings of the pattern are the same three characters:
//
//	${v#a|ab}                     abc, so the bar matched nothing
//	L='a|ab'; ${v#${~L}}          bc, so the same bar from a value split
//	setopt globsubst; ${v#a|ab}   abc, so the option does not reach it
//	w='a|b'; ${w#a|b}             empty, so the written bar matched itself
//
// Through the trim operator rather than through a `case`, because a `case`
// arm's bar is the grammar's separator and never reaches the matcher at all.
func TestAWrittenBarIsNotAnAlternation(t *testing.T) {
	d := syntax.Core()
	d.PatternAlternation, d.PatternTopLevelAlternation = true, syntax.TopLevelAlternationFromAValue
	sem := permissive()
	sem.GlobExpansionResults = Yes
	for _, tc := range []struct{ name, src, want string }{
		{"a written bar trims nothing", `v=abc; printf "%s" "${v#a|ab}"`, "abc"},
		{"and matches itself", `w='a|b'; printf "%s" "${w#a|b}"`, ""},
		{"a bar from a value still splits", `v=abc; L='a|ab'; printf "%s" "${v#$L}"`, "bc"},
		{"a written group still splits", `v=abc; printf "%s" "${v#(a|ab)}"`, "bc"},
		{
			// The written bar stays ordinary even with a live expansion on
			// either side of it, so the rule is about the character's own
			// source and not about whether the pattern holds a value.
			"a written bar between two values", `v=abc; e=a; f=ab; printf "%s" "${v#$e|$f}"`, "abc",
		},
		{
			// A live bar's arms take in the written text beside them rather
			// than stopping at the value's edges, which is measured: with
			// `I='ab|x'`, `${v#${~I}z}` is `c` in zsh, so the arms are `ab`
			// and `xz`. Read as concatenation it would be `abc`.
			"a live bar splits the whole pattern", `v=abc; i='ab|x'; printf "%s" "${v#${i}z}"`, "c",
		},
		{
			// And from the other side: a written `a` in front of `x|abc`
			// joins the first arm, so the second matches the whole subject.
			"written text joins the first arm", `v=abc; n='x|abc'; printf "%s" "${v#a$n}"`, "",
		},
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
			if got := out.String(); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// The other reading: a bar outside every group is an alternation **however it
// arrived**, so the provenance rule the tests above pin does not apply to it.
//
// Two readings rather than one setting of the same one, which is the whole
// reason [syntax.TopLevelAlternation] is not a bool. Neither contains the
// other: this one takes a written bar where the other leaves it ordinary, and
// leaves a bar out of pathname expansion where the other reads it there.
// Measured 2026-09-13 on the two shells that have them (#2528); the values are
// named here rather than the shells, as everything under interp is.
func TestTheOtherReadingTakesAWrittenBarToo(t *testing.T) {
	for _, tc := range []struct{ name, src, fromAValue, wherever string }{
		{
			// The row the two readings split on, and the reason a bool
			// could not carry both.
			name: "a written bar", src: `v=abc; printf "%s" "${v#a|ab}"`,
			fromAValue: "abc", wherever: "bc",
		},
		{
			// The value stops matching its own text, which is what says
			// the bar became syntax rather than one more character.
			name: "and the value no longer matches itself", src: `w='a|b'; printf "%s" "${w#a|b}"`,
			fromAValue: "", wherever: "|b",
		},
		{
			// Where they agree. A bar out of a value splits under both, so
			// a table of only this row would report one reading.
			name: "a bar from a value splits under both", src: `v=abc; L='a|ab'; printf "%s" "${v#$L}"`,
			fromAValue: "bc", wherever: "bc",
		},
		{
			// And the control that says neither reading is simply "the
			// character is special": escaped, it is text again in both.
			name: "an escaped bar is text under both", src: `w='a|b'; printf "%s" "${w#a\|b}"`,
			fromAValue: "", wherever: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, r := range []struct {
				name string
				v    syntax.TopLevelAlternation
				want string
			}{
				{"from a value", syntax.TopLevelAlternationFromAValue, tc.fromAValue},
				{"wherever written", syntax.TopLevelAlternationWhereverWritten, tc.wherever},
			} {
				d := syntax.Core()
				d.PatternAlternation, d.PatternTopLevelAlternation = true, r.v
				sem := permissive()
				sem.GlobExpansionResults = Yes
				f, err := syntax.Parse(tc.src, d)
				if err != nil {
					t.Fatalf("%s: parse: %v", r.name, err)
				}
				var out bytes.Buffer
				run := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &sem, Env: testPATH()})
				if _, err := run.Run(context.Background(), f); err != nil {
					t.Fatalf("%s: %v", r.name, err)
				}
				if got := out.String(); got != r.want {
					t.Errorf("%s under %s = %q, want %q", tc.src, r.name, got, r.want)
				}
			}
		})
	}
}

// A top-level bar reaches pathname expansion under one reading and not the
// other, which is the second axis the two differ on and the one a matcher test
// cannot see. Measured 2026-09-13 against a control in the same run: the
// reading that leaves the bar out of globbing still expands `a*` from the same
// value to two fields, so it is the bar that stops here and not the value
// failing to be a pattern (#2528).
func TestOnlyOneReadingReachesPathnameExpansion(t *testing.T) {
	for _, r := range []struct {
		name   string
		v      syntax.TopLevelAlternation
		barGot string
	}{
		{"from a value", syntax.TopLevelAlternationFromAValue, "2 aa b"},
		{"wherever written", syntax.TopLevelAlternationWhereverWritten, "1 aa|b"},
	} {
		t.Run(r.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, n := range []string{"aa", "ab", "b"} {
				if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			d := syntax.Core()
			d.PatternAlternation, d.PatternTopLevelAlternation = true, r.v
			sem := permissive()
			sem.GlobExpansionResults = Yes
			// The other half of "a value may carry pattern syntax", and it
			// has to be answered or the run stops on an unchosen axis
			// rather than on this one (#2474).
			sem.ExpansionResultSuppliesGroupSyntax = Yes
			for _, probe := range []struct{ src, want string }{
				{`P='aa|b'; set -- $P; printf "%d %s" "$#" "$*"`, r.barGot},
				// The control. Both readings glob a `*` out of the same
				// kind of value, so a bar that does not is the bar.
				{`S='a*'; set -- $S; printf "%d %s" "$#" "$*"`, "2 aa ab"},
			} {
				f, err := syntax.Parse(probe.src, d)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				var out bytes.Buffer
				run := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &sem, Env: testPATH(), Dir: dir})
				if _, err := run.Run(context.Background(), f); err != nil {
					t.Fatalf("%v", err)
				}
				if got := out.String(); got != probe.want {
					t.Errorf("%s under %s = %q, want %q", probe.src, r.name, got, probe.want)
				}
			}
		})
	}
}
