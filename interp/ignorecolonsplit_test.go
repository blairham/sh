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

// runIgnoring runs src in dir with the ignore facility wired the way the one
// dialect that splits its value wires it, and with quantified groups, which
// is what decides whether a `(` after a quantifier is a group at all.
func runIgnoring(t *testing.T, dir, src string, extended bool) string {
	t.Helper()
	d := syntax.Core()
	d.ExtendedPattern = extended
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "parse: " + err.Error()
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.IgnoredNamesVariable = "GLOBIGNORE"
	s.IgnoredNamesRevealHiddenNames = true
	s.IgnoredNamesValueIsOnePattern = No
	s.IgnoredNamesMatchTheLastComponent = No
	s.IgnoredNamesFollowTheParameter = No
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dir: dir, Dialect: &d, Semantics: &s})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// colonTree is a directory whose names are the pieces a value might be cut
// into as well as the names its patterns describe, so a wrong cut shows as a
// different set rather than as nothing happening.
func colonTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"(a", ":", "[a", "a", "a:b", "ab", "b", "b)", "b]"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The ignore parameter's value is cut at the colons that separate its
// patterns, which is not every colon in it.
//
// A colon is also an ordinary member of a bracket expression, the delimiter a
// character class is written with, and a character a group may hold. Cutting
// at all of them is what made `GLOBIGNORE='[![:alpha:]]'` three patterns —
// `[![`, `alpha` and `]]` — none of which is what the script wrote.
//
// Every row is measured 2026-09-22 on bash 5.3.20, by what `echo *` leaves in
// exactly the directory colonTree builds.
func TestTheIgnoreValueIsCutOnlyAtASeparatingColon(t *testing.T) {
	dir := colonTree(t)
	for _, tc := range []struct {
		name, value, want string
		extended          bool
	}{
		{
			"a bracket keeps its colons", "[a:b]",
			`[(a][[a][a:b][ab][b)][b]]`, false,
		},
		{
			// The control beside it: the same three characters with no
			// bracket round them are two patterns.
			"and a colon outside one still cuts", "a:b",
			`[(a][:][[a][a:b][ab][b)][b]]`, false,
		},
		{
			"a bracket and a word", "[a:b]:ab",
			`[(a][[a][a:b][b)][b]]`, false,
		},
		{
			// A character class is a colon delimiter inside a bracket, and
			// the row the suite file turns on.
			"a character class keeps both of its colons", "[[:alpha:]]",
			`[(a][:][[a][a:b][ab][b)][b]]`, false,
		},
		{
			"a group keeps its colons", "@(a:b)",
			`[(a][:][[a][a][ab][b][b)][b]]`, true,
		},
		{
			// The same value without the grammar that makes it a group is
			// two patterns, and the second of them names a file here.
			"where the dialect has no group it is two patterns", "@(a:b)",
			`[(a][:][[a][a][a:b][ab][b][b]]`, false,
		},
		{
			// A bare `(` is not a group in that dialect, so it does not
			// protect a colon either.
			"a bare parenthesis protects nothing", "(a:b)",
			`[:][[a][a][a:b][ab][b][b]]`, true,
		},
		{
			// Nothing closes the group, so it reaches the end of the value
			// and no colon behind it separates anything.
			"an unclosed group takes the rest of the value", "@(a:b",
			`[(a][:][[a][a][a:b][ab][b][b)][b]]`, true,
		},
		{
			"and so does an unclosed bracket", "[a:b",
			`[(a][:][[a][a][a:b][ab][b][b)][b]]`, false,
		},
		{
			"an escaped colon does not cut", `a\:b`,
			`[(a][:][[a][a][ab][b][b)][b]]`, false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `GLOBIGNORE='` + tc.value + `'; printf "[%s]" *`
			if got := runIgnoring(t, dir, src, tc.extended); got != tc.want {
				t.Errorf("GLOBIGNORE=%s = %s, want %s", tc.value, got, tc.want)
			}
		})
	}
}
