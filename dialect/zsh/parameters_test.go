// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestParametersDescribesEveryKindAndAttribute is `$parameters`, and every
// row was measured against zsh 5.9.2 under `-c`.
//
// It is one table because the answers are one string built in one order, and
// the order is the part a caller cannot see from a single row: the type word
// first, then tied, case, readonly, export, hide, unique and special, which
// is what the combinations at the bottom pin down.
func TestParametersDescribesEveryKindAndAttribute(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, decl, want string
	}{
		{"a plain scalar", `typeset -g v=1`, "scalar"},
		{"an array", `typeset -ga v=(1)`, "array"},
		{"an association", `typeset -gA v=(k x)`, "association"},
		{"an integer", `typeset -gi v=1`, "integer"},
		{"a float", `typeset -gF v=1.0`, "float"},
		{"exported", `typeset -gx v=1`, "scalar-export"},
		{"readonly", `typeset -gr v=1`, "scalar-readonly"},
		{"lower", `typeset -gl v=A`, "scalar-lower"},
		{"upper", `typeset -gu v=a`, "scalar-upper"},
		{"a unique array", `typeset -gaU v=(1)`, "array-unique"},
		{
			// readonly before export, which is the reverse of the order the
			// letters are usually written in.
			name: "readonly comes before export",
			decl: `typeset -gxr v=1`,
			want: "scalar-readonly-export",
		},
		{
			name: "case comes before both",
			decl: `typeset -gxrl v=A`,
			want: "scalar-lower-readonly-export",
		},
		{
			name: "unique comes last",
			decl: `typeset -gaxrU v=(1)`,
			want: "array-readonly-export-unique",
		},
		{
			// The container wins over the numeric attribute: an integer
			// array describes as an array and says nothing about integers.
			name: "an integer array is just an array",
			decl: `typeset -gia v; v=(1 2)`,
			want: "array",
		},
		{
			name: "a float array is just an array",
			decl: `typeset -gFa v; v=(1.5)`,
			want: "array",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "zmodload zsh/parameter; " + tc.decl +
				`; print -r -- "${parameters[v]}"`
			out, st := runZsh(t, dir, src)
			if want := tc.want + "\n"; out != want || st != 0 {
				t.Errorf("parameters[v] = %q (status %d), want %q", out, st, want)
			}
		})
	}
}

// TestParametersAnswersAboutTheNamesThemselves is what most of the callers
// actually do with the table — powerlevel10k tests membership in seven
// places and reads a value in one.
func TestParametersAnswersAboutTheNamesThemselves(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			name: "a name that is not there is empty",
			src:  `zmodload zsh/parameter; print -r -- "[${parameters[nosuchvar]}]"`,
			want: "[]\n",
		},
		{
			name: "and reports absent to the existence test",
			src:  `zmodload zsh/parameter; v=1; print -r -- "${+parameters[v]} ${+parameters[nosuchvar]}"`,
			want: "1 0\n",
		},
		{
			// The module's own parameters are in the table, described as the
			// shell's rather than a script's.
			name: "a produced table is described and marked special",
			src:  `zmodload zsh/parameter; print -r -- "${parameters[options]}"`,
			want: "association-special\n",
		},
		{
			name: "a produced array is too",
			src:  `zmodload zsh/parameter; print -r -- "${parameters[funcstack]}"`,
			want: "array-special\n",
		},
		{
			// zsh answers `parameters[x]=y` with `read-only variable`, and a
			// produced table without the attribute would take the assignment
			// into a stored table that then shadows the producer.
			name: "it refuses to be written",
			src:  `zmodload zsh/parameter; parameters[foo]=bar`,
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZsh(t, dir, tc.src)
			if tc.want == "" {
				if !strings.Contains(out, "read-only") {
					t.Errorf("out = %q, want a read-only refusal", out)
				}
				return
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// TestParametersCoversTheNamesTheShellHas is the enumeration rather than a
// lookup, and it is deliberately not a count: the set a shell holds depends
// on what its front end put there, so a number here would be a fact about
// today's preset. What is asserted is that a name the script made and a name
// the shell provides are both in the keys, and a name nothing made is not.
func TestParametersCoversTheNamesTheShellHas(t *testing.T) {
	dir := t.TempDir()
	src := `zmodload zsh/parameter
mine=1
k=(${(k)parameters})
print -r -- "mine=[${k[(r)mine]}] provided=[${k[(r)options]}] absent=[${k[(r)nosuchvar]}]"`
	out, st := runZsh(t, dir, src)
	const want = "mine=[mine] provided=[options] absent=[]\n"
	if out != want || st != 0 {
		t.Errorf("keys = %q (status %d), want %q", out, st, want)
	}
}
