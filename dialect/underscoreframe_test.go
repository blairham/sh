// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// `$_` across a function call, in all five dialects — #3134, where a call left
// the *body's* last argument behind in two of them and `cmd/ksh` had no `$_`
// at all.
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file, with `inner() { :; }` and a `peek` that reads `$_` at its
// first line, calls `inner zz`, and reads it again:
//
//	                      	body, at entry	body, after inner	after the call
//	bash 5.3.20, as-sh, 3.2.57	`outer`   	`zz`             	`two`
//	ksh93u+ 2012-08-01    	`outer`       	`outer`          	`two`
//	zsh 5.9.2             	`two`         	`zz`             	`two`
//	dash 0.5.12           	empty — no such parameter
//	BusyBox ash 1.37.0    	empty — no such parameter
//
// Three readings and one agreement, and the agreement is the last column: the
// caller reads the **call's** own last argument whatever the body did. That is
// core and not an axis. The first column is
// Semantics.UnderscoreMovesBeforeAFunctionBody; the middle one follows from
// UnderscoreMovesOnlyBetweenInputCommands, which is ksh93's narrowing — the
// body of a function is not at the input level, so nothing inside it moves the
// parameter at all.
//
// The row that put ksh93 in the "no such parameter" group was measured through
// a `;`-list, which is the one shape where a shell that has `$_` and a shell
// that has none both answer empty.
func underscorePresets() []struct {
	dialecttest.Preset
	tracks, narrowed, beforeBody, parameter interp.Answer
	want                                    string
} {
	return []struct {
		dialecttest.Preset
		tracks, narrowed, beforeBody, parameter interp.Answer
		want                                    string
	}{
		{
			dialecttest.Preset{
				Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
				Diagnostics: bash.Diagnostics, Apply: bash.Apply,
			},
			interp.Yes, interp.No, interp.No, interp.Yes,
			"body [outer] after [zz] call [two]\n",
		},
		{
			dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			interp.Yes, interp.Yes, interp.No, interp.Yes,
			"body [outer] after [outer] call [two]\n",
		},
		{
			dialecttest.Preset{
				Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
				Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
			},
			interp.Yes, interp.No, interp.Yes, interp.Yes,
			"body [two] after [zz] call [two]\n",
		},
		{
			dialecttest.Preset{
				Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
				Diagnostics: dash.Diagnostics, Apply: dash.Apply,
			},
			interp.No, interp.No, interp.No, interp.No,
			"body [] after [] call []\n",
		},
		{
			dialecttest.Preset{
				Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
				Diagnostics: ash.Diagnostics, Apply: ash.Apply,
			},
			interp.No, interp.No, interp.No, interp.No,
			"body [] after [] call []\n",
		},
	}
}

func TestEachDialectAnswersTheUnderscoreAxes(t *testing.T) {
	for _, p := range underscorePresets() {
		t.Run(p.Name, func(t *testing.T) {
			s := p.Semantics()
			if got := s.UnderscoreTracksTheLastArgument; got != p.tracks {
				t.Errorf("UnderscoreTracksTheLastArgument = %v, want %v", got, p.tracks)
			}
			if got := s.UnderscoreMovesOnlyBetweenInputCommands; got != p.narrowed {
				t.Errorf("UnderscoreMovesOnlyBetweenInputCommands = %v, want %v", got, p.narrowed)
			}
			if got := s.UnderscoreMovesBeforeAFunctionBody; got != p.beforeBody {
				t.Errorf("UnderscoreMovesBeforeAFunctionBody = %v, want %v", got, p.beforeBody)
			}
			// The fourth, and the one the probe above cannot separate: a
			// shell that keeps the parameter empty and one that keeps no
			// such name both write `[]` here. dash and BusyBox ash are the
			// second, which `${_+x}` and `set -u` say and this does not —
			// see dialect/dash/underscore_test.go and the ash one beside it
			// for the bytes (#3380).
			if got := s.UnderscoreIsAParameterAtAll; got != p.parameter {
				t.Errorf("UnderscoreIsAParameterAtAll = %v, want %v", got, p.parameter)
			}
		})
	}
}

// The three lines themselves, which is what the fields are for: a preset
// holding the right values and writing the wrong bytes would pass the test
// above.
func TestTheUnderscoreFrameInEveryDialect(t *testing.T) {
	for _, p := range underscorePresets() {
		t.Run(p.Name, func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{},
				"inner() { :; }\n"+
					"peek() { printf 'body [%s] ' \"$_\"; inner zz; printf 'after [%s] ' \"$_\"; }\n"+
					": outer\n"+
					"peek one two\n"+
					"printf 'call [%s]\\n' \"$_\"\n")
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != p.want {
				t.Errorf("wrote %q, want %q", out, p.want)
			}
		})
	}
}

// The `;`-list the corpus measured this parameter with, and the reason the
// ksh93 column was recorded as having none.
//
// `echo one two >/dev/null; printf '[%s]' "$_"` is one line, so the shell that
// narrows writes nothing for it and answers exactly as the two that keep no
// `$_` do — three empty cells that mean two different things. The same two
// commands on their own lines are the row beside it, and there ksh93 leaves
// that group and joins bash and zsh.
//
// Both halves are asserted because either alone would pass for the wrong
// shell: the first for one that has no parameter, the second for one that
// never narrows.
func TestTheProbeThatCannotTellTheTwoEmptyColumnsApart(t *testing.T) {
	for _, p := range underscorePresets() {
		t.Run(p.Name, func(t *testing.T) {
			narrowsOrHasNone := p.tracks != interp.Yes || p.narrowed == interp.Yes
			oneLine, _, err := p.Combined(t, dialecttest.Base{},
				`echo one two >/dev/null; printf '[%s]' "$_"`)
			if err != nil {
				t.Fatalf("err %v: %s", err, oneLine)
			}
			want := "[two]"
			if narrowsOrHasNone {
				want = "[]"
			}
			if oneLine != want {
				t.Errorf("on one line wrote %q, want %q", oneLine, want)
			}
			twoLines, _, err := p.Combined(t, dialecttest.Base{},
				"echo one two >/dev/null\n"+`printf '[%s]' "$_"`+"\n")
			if err != nil {
				t.Fatalf("err %v: %s", err, twoLines)
			}
			want = "[]"
			if p.tracks == interp.Yes {
				want = "[two]"
			}
			if twoLines != want {
				t.Errorf("on two lines wrote %q, want %q", twoLines, want)
			}
		})
	}
}
