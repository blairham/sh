// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The integer attribute belongs to the name, so an assignment made later is an
// expression. That is the whole reason it cannot be applied once at the point
// of declaring and forgotten.
func TestTheIntegerAttributeEvaluatesALaterAssignment(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -i n; n=5+2; echo "$n"`, "7"},
		{`typeset -i n=3*3; echo "$n"`, "9"},
		{`n=5+2; echo "$n"`, "5+2"},
		// Not an error: `abc` is an expression whose value is an unset name.
		{`typeset -i n; n=abc; echo "$n"`, "0"},
		{`typeset -i n; n=; echo "$n"`, "0"},
		// `+i` takes it away again and the next assignment is text.
		{`typeset -i n=1; typeset +i n; n=5+2; echo "$n"`, "5+2"},
	} {
		if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// An expression that will not parse stops the script rather than storing
// something, which is what both shells with the attribute do.
func TestAnUnparsableIntegerAssignmentIsFatal(t *testing.T) {
	out, status := run(t, `typeset -i n; n=1+; echo alive`, nil)
	if strings.Contains(out, "alive") {
		t.Errorf("the script carried on: %q", out)
	}
	if status == 0 {
		t.Errorf("status = 0, want a failure")
	}
}

// The declaration assigns and then freezes. Applying both at once made the
// declaration refuse its own value.
func TestReadonlyAttributeAllowsItsOwnValue(t *testing.T) {
	out, _ := run(t, `typeset -r c=1; echo "[$c]"`, nil)
	if strings.TrimSpace(out) != "[1]" {
		t.Errorf("got %q, want the value the declaration was given", out)
	}
	out, _ = run(t, `typeset -r c=1; c=2; echo "[$c]"`, nil)
	if !strings.Contains(out, "[1]") {
		t.Errorf("a later assignment got through: %q", out)
	}
	if strings.Count(out, "readonly") != 1 {
		t.Errorf("want exactly one complaint, got %q", out)
	}
}

// An assignment's value is a tilde context and is neither a splitting nor a
// globbing one. All three were wrong: the value was split on IFS and rejoined
// on a space, and a value that looked like a pattern became the directory
// listing.
func TestAnAssignmentIsNotAGlobbingOrSplittingContext(t *testing.T) {
	// The names matter. Pathname expansion acts on the *whole* field, so
	// `export E=*` matches nothing in a directory of plain names and comes
	// back unchanged whether or not the value was protected — which is a test
	// that passes against the bug. A file the whole word can match is what
	// makes the assertion bite.
	dir := fileDir(t, "a.txt", "b.txt", "E=x", "R=x", "T=x", "L=x")
	inDir := func(r *Runner) { r.Dir = dir }
	for _, tc := range []struct{ src, want string }{
		{`n=*; echo "$n"`, "*"},
		{`n=*.txt; echo "$n"`, "*.txt"},
		{`IFS=:; y=a:b; x=$y; echo "$x"`, "a:b"},
		// Array elements are ordinary words and do glob, which is what makes
		// this a rule about assignment values rather than about `=`.
		{`a=(*.txt); echo "${#a[@]}"`, "2"},
		// A declaration utility's arguments are assignments too.
		{`export E=*; echo "$E"`, "*"},
		{`readonly R=*; echo "$R"`, "*"},
		{`typeset T=*; echo "$T"`, "*"},
		{`f() { local L=*; echo "$L"; }; f`, "*"},
		// Only a word shaped like one: an argument that is not `name=value`
		// is an ordinary word and still globs.
		{`typeset -i n=1; echo *.txt`, "a.txt b.txt"},
	} {
		if out, _ := run(t, tc.src, inDir); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// The rule follows the name, and a dialect adds its own. Without this, a shell
// that has no such command would still treat its arguments as assignments.
func TestADialectAddsItsOwnDeclarationUtility(t *testing.T) {
	dir := fileDir(t, "a.txt", "x=a.txt")
	echoArgs := func(rr *Runner, _ context.Context, args []string) int {
		_, _ = fmt.Fprintln(rr.Stdout, strings.Join(args, " "))
		return 0
	}
	// Registered but not declared: its argument is an ordinary word.
	ordinary := func(r *Runner) {
		r.Dir = dir
		r.Register("mydecl", echoArgs)
	}
	if out, _ := run(t, `mydecl x=*.txt`, ordinary); strings.TrimSpace(out) != "x=a.txt" {
		t.Errorf("got %q, want the pattern expanded", out)
	}
	// Declared: the same argument is an assignment.
	declaring := func(r *Runner) {
		r.Dir = dir
		r.Register("mydecl", echoArgs)
		r.SetDeclaring("mydecl")
	}
	if out, _ := run(t, `mydecl x=*.txt`, declaring); strings.TrimSpace(out) != "x=*.txt" {
		t.Errorf("got %q, want the pattern left alone", out)
	}
	// Still only the assignment-shaped ones.
	if out, _ := run(t, `mydecl *.txt`, declaring); strings.TrimSpace(out) != "a.txt x=a.txt" {
		t.Errorf("got %q, want a plain argument to glob", out)
	}
}

// fileDir is a directory holding empty files with the given names.
func fileDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Both sides of the axis that says which functions have a scope for `typeset`
// to declare into. The test names the axis rather than a shell, as this
// package's rule requires; which shell answers which way is asserted in
// dialect/.
func TestTypesetLocalNeedsKeywordFunctionIsAnAxis(t *testing.T) {
	answer := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.TypesetLocalNeedsKeywordFunction = a
			r.Semantics = &s
		}
	}
	const posix = `x=outer; f() { typeset x=inner; }; f; echo "$x"`
	const keyword = `x=outer; function f { typeset x=inner; }; f; echo "$x"`
	for _, tc := range []struct {
		name string
		a    Answer
		src  string
		want string
	}{
		{"a POSIX function declares a local", No, posix, "outer"},
		{"a POSIX function does not", Yes, posix, "inner"},
		// The keyword form is where the answers agree, which is why the pair
		// is the whole of the axis and either answer serves here.
		{"a keyword function declares a local either way", No, keyword, "outer"},
		{"and with the other answer too", Yes, keyword, "outer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, answer(tc.a)); strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// Whether a name declared without a value counts as set.
func TestDeclaredNameWithoutValueIsEmptyIsAnAxis(t *testing.T) {
	answer := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.DeclaredNameWithoutValueIsEmpty = a
			r.Semantics = &s
		}
	}
	for _, tc := range []struct {
		a    Answer
		src  string
		want string
	}{
		{Yes, `f() { local u; echo "[${u-UNSET}]"; }; f`, "[]"},
		{No, `f() { local u; echo "[${u-UNSET}]"; }; f`, "[UNSET]"},
		{Yes, `typeset u; echo "[${u-UNSET}]"`, "[]"},
		{No, `typeset u; echo "[${u-UNSET}]"`, "[UNSET]"},
	} {
		if out, _ := run(t, tc.src, answer(tc.a)); strings.TrimSpace(out) != tc.want {
			t.Errorf("%v: %s = %q, want %q", tc.a, tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}
