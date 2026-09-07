// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A quoted expansion is exactly one field, and an element that is not there
// does not change that: `"${a[5]}"` on a gap is one empty field, the same as
// `"$unset"` is.
//
// It produced no field at all. That is the worst shape a wrong answer takes,
// because nothing fails — `set -- "${a[0]}" "${a[1]}" "${a[5]}"` came back with
// `$#` of 2 and every argument after the gap moved up one, so the script kept
// running with everything off by one.
func TestAQuotedGapIsOneField(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x); a[5]=y; set -- "${a[0]}" "${a[1]}" "${a[5]}"; echo "n=$#"`, "n=3"},
		// Where the empty field lands, which a count alone cannot say.
		{`a=(x); a[5]=y; printf "[%s]" "${a[0]}" "${a[1]}" "${a[5]}"`, "[x][][y]"},
		// Past the end rather than inside a gap — the same question.
		{`a=(x y); set -- "${a[9]}"; echo "n=$#"`, "n=1"},
		// A subscript on a name that was never an array at all.
		{`set -- "${b[3]}"; echo "n=$#"`, "n=1"},
		// A key nothing was stored under, where the subscript is a key.
		{`typeset -A m; m[k]=v; set -- "${m[nokey]}"; echo "n=$#"`, "n=1"},
		// Inside a larger word the field was never lost, which is why this
		// went unnoticed for as long as it did.
		{`a=(x); echo "[p${a[9]}q]"`, "[pq]"},
	} {
		out, st := runArray(t, c.src)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// The other side, and the reason the rule is about quoting rather than about
// arrays: unquoted, an empty expansion is no field at all.
func TestAnUnquotedGapIsNoField(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x); a[5]=y; set -- ${a[0]} ${a[1]} ${a[5]}; echo "n=$#"`, "n=2"},
		{`set -- ${b[3]}; echo "n=$#"`, "n=0"},
	} {
		if out, _ := runArray(t, c.src); strings.TrimSpace(out) != c.want {
			t.Errorf("%s = %q, want %q", c.src, strings.TrimSpace(out), c.want)
		}
	}
}

// How many fields a quoted `[@]` on a name that holds *nothing* makes is a
// different question, and the only one of the group a dialect answers. Asking
// that axis about a single subscript is what made a gap disappear.
func TestAQuotedAtOnAnAbsentNameIsTheAxisAlone(t *testing.T) {
	for _, c := range []struct {
		name string
		one  Answer
		want string
	}{
		{"no field", No, "n=0"},
		{"one empty field", Yes, "n=1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := runUnsetAtAxis(t, c.one, `set -- "${a[@]}"; echo "n=$#"`)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
			// An array that exists and has no elements is no field under
			// either answer: emptiness is not what the axis asks about, and
			// no column in the panel gives it a field.
			empty := runUnsetAtAxis(t, c.one, `a=(); set -- "${a[@]}"; echo "n=$#"`)
			if empty != "n=0" {
				t.Errorf("a declared empty array gave %q under this answer, want n=0", empty)
			}
			// A single subscript is one field under either answer, because
			// the axis is not its question.
			gap := runUnsetAtAxis(t, c.one,
				`a=(x); a[5]=y; set -- "${a[0]}" "${a[1]}" "${a[5]}"; echo "n=$#"`)
			if gap != "n=3" {
				t.Errorf("a gap gave %q under this answer, want n=3", gap)
			}
			// `[*]` joins, so a quoted one is a single field with nothing to
			// join — unanimous, and so not the axis either.
			star := runUnsetAtAxis(t, c.one, `set -- "${a[*]}"; echo "n=$#"`)
			if star != "n=1" {
				t.Errorf("a star gave %q under this answer, want n=1", star)
			}
			// Nor is the count: `${#a[@]}` is zero on an absent name in
			// every column, whatever the field question answers.
			n := runUnsetAtAxis(t, c.one, `echo "n=${#a[@]}"`)
			if n != "n=0" {
				t.Errorf("a length gave %q under this answer, want n=0", n)
			}
			// Nor the unquoted spelling, which splitting empties either way.
			bare := runUnsetAtAxis(t, c.one, `set -- ${a[@]}; echo "n=$#"`)
			if bare != "n=0" {
				t.Errorf("an unquoted at gave %q under this answer, want n=0", bare)
			}
		})
	}
}

// A list that exists and is empty without any *array store* behind it, which
// is the third shape the guard has to get right and the one no assignment can
// reach: a produced array whose producer has nothing to produce yet.
//
// Written because a guard keyed on the array table rather than on what the
// name holds passes every stored-array row above and fails only here — and
// the shells this substrate presets have such parameters, empty until a
// `disable` or an `alias -g` fills them, so the dialect that answers yes
// would hand every read of one a field it has not got.
func TestAProducedArrayWithNothingToProduceIsNoField(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		out, _ := runGrammar(t, `set -- "${produced[@]}"; echo "n=$#"`, nil, func(r *Runner) {
			sem := *r.Semantics
			sem.UnsetNameAtIsOneEmptyField = answer
			r.Semantics = &sem
			r.SetDynamicArray("produced", func(*Runner) []string { return nil })
		})
		if got := strings.TrimSpace(out); got != "n=0" {
			t.Errorf("%v: got %q, want n=0 — the name holds a list, it is only empty", answer, got)
		}
	}
	// The control: the same producer with something to produce keeps its
	// fields, so the row above is not passing because the parameter is
	// unreachable.
	out, _ := runGrammar(t, `set -- "${produced[@]}"; echo "n=$#"`, nil, func(r *Runner) {
		r.SetDynamicArray("produced", func(*Runner) []string { return []string{"x", "y"} })
	})
	if got := strings.TrimSpace(out); got != "n=2" {
		t.Errorf("a producer with two elements gave %q, want n=2", got)
	}
}

// runUnsetAtAxis runs src with UnsetNameAtIsOneEmptyField set to answer.
func runUnsetAtAxis(t *testing.T, answer Answer, src string) string {
	t.Helper()
	out, _ := runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.UnsetNameAtIsOneEmptyField = answer
		r.Semantics = &sem
	})
	return strings.TrimSpace(out)
}
