// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The declaration commands are reserved words in this shell, and the rule that
// makes their `name=value` operands assignments belongs to the word as written
// rather than to the builtin that runs. See Semantics.DeclarationCommandWord
// and #3315.
//
// Measured 2026-09-16 on zsh 5.9.2 under `-f`, from a script file, for every
// form below and every one of the seven words; integer and float are the
// next test, because their values are evaluated. `setopt shwordsplit` is in
// every row because without it this shell splits no unquoted parameter at
// all, and both readings would keep `x y` whole. A split operand also leaves
// its second word behind as a declaration of its own, which is why `y` is
// reported: it is set and empty only where the value was split.
//
// An alias for the word keeps the rule too, measured, and is not a row here:
// this harness parses the whole source before running any of it, so an alias
// defined in it is never seen by the line that uses it.
func TestADeclarationIsTheWordAsWrittenInThisDialect(t *testing.T) {
	dir := t.TempDir()
	forms := []struct {
		name, line string
		split      bool
	}{
		{"the literal word", `%s v=$b`, false},
		{"the reserved nocorrect in front", `nocorrect %s v=$b`, false},
		{"an expansion", `cmd=%s; $cmd v=$b`, true},
		{"a backslash", `\%s v=$b`, true},
		{"single quotes", `'%s' v=$b`, true},
		{"double quotes", `"%s" v=$b`, true},
		{"noglob in front", `noglob %s v=$b`, true},
		{"builtin in front", `builtin %s v=$b`, true},
	}
	for _, word := range []string{"typeset", "declare", "export", "local", "readonly"} {
		for _, form := range forms {
			t.Run(word+"/"+form.name, func(t *testing.T) {
				value, left := "[x y]", " y=[UNSET]"
				if form.split {
					value, left = "[x]", " y=[]"
				}
				report, want := `print -rn -- "[$v] y=[${y-UNSET}]"`, value+left
				if word == "export" {
					// `export y` on an unset name sets it empty in zsh and
					// not here, which is #3345 and not this rule — so the
					// row reads only the value.
					report, want = `print -rn -- "[$v]"`, value
				}
				src := "setopt shwordsplit\nb='x y'\nf() {\n" +
					strings.ReplaceAll(form.line, "%s", word) + "\n" + report + "\n}\nf"
				out, st := runZsh(t, dir, src)
				if out != want || st != 0 {
					t.Errorf("%s wrote %q (status %d), want %q at 0", src, out, st, want)
				}
			})
		}
	}
}

// integer and float evaluate their value, so the split shows as which
// expression was evaluated: the whole `3 y` is a bad expression, and `3` alone
// is not — and the `y` that falls out of the split is declared numeric too.
func TestANumericDeclarationIsTheWordAsWrittenInThisDialect(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"integer, an expansion", `cmd=integer; $cmd v=$b; print -rn -- "[$v] [$y]"`, "[3] [0]"},
		{"integer, quoted", `'integer' v=$b; print -rn -- "[$v] [$y]"`, "[3] [0]"},
		{"float, a backslash", `\float v=$b; print -rn -- "[$v] [$y]"`, "[3.000000000e+00] [0.000000000e+00]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "setopt shwordsplit\nb='3 y'\n" + tc.src
			out, st := runZsh(t, dir, src)
			if out != tc.want || st != 0 {
				t.Errorf("%s wrote %q (status %d), want %q at 0", src, out, st, tc.want)
			}
		})
	}
}
