// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `-g` says *where* a declaration lands and nothing else.
//
// It was written as an early exit from the declaration loop, which made it a
// second and shorter declaration: everything below the exit — the associative
// attribute, and bringing a valueless name into being — was skipped, so
// `typeset -gA m` left `m` an ordinary name and every `m[k]=v` after it read
// its subscript as an index (#989). These tests hold the two apart: the
// attributes a declaration records must not depend on the letter, and the
// scope must.

const globalLetters = "aAgiprx"

// TestGlobalLetterKeepsTheAssociativeAttribute: `-gA` is `-A`, declared
// globally. The subscript is a key either way, which is the only thing that
// tells an associative name from an indexed one.
func TestGlobalLetterKeepsTheAssociativeAttribute(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the subscript is a key at the top level",
			`typeset -gA m; m[k]=v; printf "[%s]" "${m[k]}"`, "[v]",
		},
		{
			// The one probe that tells the two attributes apart: an indexed
			// name evaluates `1+1` to 2, an associative one stores the three
			// characters.
			"the subscript is text, not arithmetic",
			`typeset -gA m; m[1+1]=x; printf "[%s][%s]" "${m[1+1]}" "${m[2]:-none}"`, "[x][none]",
		},
		{
			"the letters may be written in either order",
			`typeset -Ag m; m[k]=v; printf "[%s]" "${m[k]}"`, "[v]",
		},
		{
			"the letters may be written as two words",
			`typeset -g -A m; m[k]=v; printf "[%s]" "${m[k]}"`, "[v]",
		},
		{
			// Every operand is declared, not only the first — the shape the
			// declaration is actually written in when a script sets up its
			// tables.
			"every name on the line is declared",
			`typeset -gA m n; m[a]=1; n[b]=2; printf "[%s][%s]" "${m[a]}" "${n[b]}"`, "[1][2]",
		},
		{
			// The reason it is a P1: the declaration is inside a function and
			// the table it makes has to outlive the return, attribute and all.
			"the attribute survives the function that declared it",
			`f() { typeset -gA m; }; f; m[k]=v; printf "[%s]" "${m[k]}"`, "[v]",
		},
		{
			"a declaration and its elements from inside a function",
			`f() { typeset -gA m; m[a]=1; m[b]=2; }; f; printf "[%s]" "${m[@]}"`, "[1][2]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := declRun(t, c.src, withDeclareLetters(globalLetters), Diagnostics{})
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q, want a clean run", st, errs)
			}
			if out != c.want {
				t.Errorf("stdout = %q, want %q", out, c.want)
			}
		})
	}
}

// TestGlobalAssociativeArrayIsOneFieldPerElement: the count and the values,
// asserted exactly. A name that lost the attribute still reads back through
// `${m[@]}` — as one element under the index the subscript evaluated to — so
// nothing short of the field count and the fields catches the difference.
func TestGlobalAssociativeArrayIsOneFieldPerElement(t *testing.T) {
	src := `typeset -gA m; m[b]="x y"; m[a]=1; set -- "${m[@]}"; printf "%d" "$#"; printf "[%s]" "$@"`
	out, errs, st := declRun(t, src, withDeclareLetters(globalLetters), Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("status %d stderr %q, want a clean run", st, errs)
	}
	if out != "2[1][x y]" {
		t.Errorf("stdout = %q, want %q", out, "2[1][x y]")
	}
}

// TestGlobalLetterDoesNotShadowTheAttribute is the half `-g` really does
// change: without it a declaration inside a function is local and the
// caller's name is untouched when it returns, attribute included.
func TestGlobalLetterDoesNotShadowTheAttribute(t *testing.T) {
	local := func(s *Semantics) {
		s.DeclareOptions = globalLetters
		s.TypesetLocalNeedsKeywordFunction = No
	}
	out, errs, st := declRun(t,
		`f() { typeset -A m; m[k]=v; }; f; printf "[%s]" "${m[k]:-gone}"`,
		local, Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("status %d stderr %q, want a clean run", st, errs)
	}
	if out != "[gone]" {
		t.Errorf("stdout = %q, want the local table gone with the function", out)
	}
}

// TestGlobalValuelessDeclarationBringsTheNameIntoBeing: a `-g` declaration
// with no value is the whole of what a setup line like
// `typeset -gA a b c` is, so it has to answer the same axis the local path
// answers rather than doing nothing at all.
func TestGlobalValuelessDeclarationBringsTheNameIntoBeing(t *testing.T) {
	empty := func(s *Semantics) {
		s.DeclareOptions = globalLetters
		s.DeclaredNameWithoutValueIsEmpty = Yes
	}
	out, errs, st := declRun(t, `typeset -g n; printf "[%s]" "${n-unset}"`, empty, Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("status %d stderr %q, want a clean run", st, errs)
	}
	if out != "[]" {
		t.Errorf("stdout = %q, want the name set and empty", out)
	}

	// From inside a function, which is where the letter is written: the name
	// is global, so the return does not take it away.
	out, _, _ = declRun(t, `f() { typeset -g n; }; f; printf "[%s]" "${n-unset}"`, empty, Diagnostics{})
	if out != "[]" {
		t.Errorf("stdout = %q, want the global name to outlive the function", out)
	}

	// The shell that says a valueless declaration sets nothing gets the same
	// answer from `-g` as it does without it.
	out, _, _ = declRun(t, `typeset -g n; printf "[%s]" "${n-unset}"`,
		withDeclareLetters(globalLetters), Diagnostics{})
	if out != "[unset]" {
		t.Errorf("stdout = %q, want the axis's other answer honored under -g", out)
	}
}

// TestGlobalValuelessExportIsNotHandedToAChild: a `-g` declaration that sets
// nothing is a declaration and not an assignment, and the two part company at
// the process boundary — the shell reads the name as empty and a child is told
// nothing about it. The local path records that; `-g` has to record it too, or
// an exported name would arrive at every child carrying an empty value.
func TestGlobalValuelessExportIsNotHandedToAChild(t *testing.T) {
	empty := func(s *Semantics) {
		s.DeclareOptions = globalLetters
		s.DeclaredNameWithoutValueIsEmpty = Yes
	}
	src := `typeset -gx GLOBALVALUELESS; echo "read=[${GLOBALVALUELESS-unset}]"; ` +
		`/usr/bin/env | /usr/bin/grep -c '^GLOBALVALUELESS'`
	out, _, _ := declRun(t, src, empty, Diagnostics{})
	if out != "read=[]\n0\n" {
		t.Errorf("stdout = %q, want the shell reading it empty and the child told nothing", out)
	}
}

// TestGlobalValuelessDeclarationKeepsAnExistingValue: `-g` takes no shadow,
// so there is no fresh cell to fill in — a name that already holds something
// keeps it, which is what every shell spelling the letter does.
func TestGlobalValuelessDeclarationKeepsAnExistingValue(t *testing.T) {
	empty := func(s *Semantics) {
		s.DeclareOptions = globalLetters
		s.DeclaredNameWithoutValueIsEmpty = Yes
	}
	out, errs, st := declRun(t, `n=1; typeset -g n; printf "[%s]" "$n"`, empty, Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("status %d stderr %q, want a clean run", st, errs)
	}
	if out != "[1]" {
		t.Errorf("stdout = %q, want the standing value left alone", out)
	}

	// And a local standing in front of the name is what the caller still sees
	// on return, rather than an empty global written past it.
	out, _, _ = declRun(t,
		`n=out; f() { local n=in; typeset -g n; printf "[%s]" "$n"; }; f; printf "[%s]" "$n"`,
		empty, Diagnostics{})
	if out != "[in][out]" {
		t.Errorf("stdout = %q, want %q", out, "[in][out]")
	}
}

// TestGlobalLetterStillCarriesTheOtherAttributes guards the letters that were
// already applied before the split, so a later edit cannot lose one of them
// the way `-A` was lost.
func TestGlobalLetterStillCarriesTheOtherAttributes(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"integer", `typeset -gi n; n=5+2; printf "[%s]" "$n"`, "[7]"},
		{"export", `typeset -gx e=1; printf "[%s]" "$e"`, "[1]"},
		{"the value still lands", `typeset -g v=x; printf "[%s]" "$v"`, "[x]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := declRun(t, c.src, withDeclareLetters(globalLetters), Diagnostics{})
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q, want a clean run", st, errs)
			}
			if out != c.want {
				t.Errorf("stdout = %q, want %q", out, c.want)
			}
		})
	}

	// Readonly is applied last on both paths, so the declaration's own value
	// lands and the *next* assignment is the refusal.
	out, errs, _ := declRun(t, `typeset -gr ro=5; printf "[%s]" "$ro"; ro=6`,
		withDeclareLetters(globalLetters), Diagnostics{ReadonlyVariable: "%s: readonly"})
	if !strings.Contains(errs, "ro: readonly") {
		t.Errorf("stderr = %q, want the readonly refusal", errs)
	}
	if out != "[5]" {
		t.Errorf("stdout = %q, want the declaration's own value", out)
	}
}
