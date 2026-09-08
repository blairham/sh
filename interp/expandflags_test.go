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

// The parenthesized expansion flags, `${(U)x}` and its family. Every
// expectation here is an oracle measurement recorded in
// docs/spec/grammar/parameter-expansion.md; the tests name the grammar flag
// and never a shell.

// promptEscapeTable is the smallest table the `%` flag tests need: the
// doubled escape, the user, and the two codes that name the file being read.
func promptEscapeTable() PromptStyle {
	return PromptStyle{
		Escape: '%',
		Codes: map[rune]PromptField{
			'%': FieldEscape,
			'n': FieldUser,
			'x': FieldSourceFile,
			'N': FieldUnitName,
		},
		TrailingEscapeIsDropped: true,
	}
}

func flagsRun(t *testing.T, src string) (string, string, int) {
	t.Helper()
	d := syntax.Core()
	d.ParamExpansionFlags = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	// The construct belongs to the dialect that neither splits nor globs
	// what an expansion produces, whose fatal expansions carry status 1 and
	// whose bare declarations set the name — so the tests measure it on
	// those settings, named as axes.
	sem.SplitParamExpansion = No
	sem.GlobExpansionResults = No
	sem.FatalErrorStatusIsOne = Yes
	sem.DeclaredNameWithoutValueIsEmpty = Yes
	var out, errs bytes.Buffer
	// The dialect travels with the runner so nested input — a sourced file,
	// an eval — parses the same grammar.
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &errs, Dialect: &d, Semantics: &sem, Name: "testsh"})
	// The `%` flag reads the prompt-escape table a dialect supplies, so a
	// test of it has to supply one. Named as a table rather than as a shell:
	// these four rows are the escapes a script uses to find its own path
	// plus the doubled escape, which is what this package carried
	// hard-coded before there was one table (#1090). Everything not in it
	// is refused by name, which several cases below are about.
	r.SetPromptStyle(promptEscapeTable())
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

func TestExpansionFlagValues(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"U uppercases", `x=abC; printf "[%s]" "${(U)x}"`, "[ABC]"},
		{"L lowercases", `x=abC; printf "[%s]" "${(L)x}"`, "[abc]"},
		{"U is elementwise on an array", `a=(ab cd); printf "[%s]" "${(U)a}"`, "[AB CD]"},
		{"U converts a substituted default", `unset u; printf "[%s]" "${(U)u:-def}"`, "[DEF]"},
		{"U converts what an operator leaves", `x=hello; printf "[%s]" "${(U)x#h}"`, "[ELLO]"},
		{"assignment stores the word as written", `unset u; printf "[%s][%s]" "${(U)u:=def}" "$u"`, "[DEF][def]"},
		{"the flags apply behind a length", `x=abc; printf "[%s]" "${(U)#x}"`, "[3]"},
		{"an empty name is an empty value", `printf "[%s]" "${(U)}"`, "[]"},
		{"empty parens change nothing", `x=ab; printf "[%s]" "${()x}"`, "[ab]"},

		{"q escapes with backslashes", `x='a b$c'; printf "[%s]" "${(q)x}"`, `[a\ b\$c]`},
		{"q leaves the unspecial alone", `x=a:b/c.d; printf "[%s]" "${(q)x}"`, `[a:b/c.d]`},
		{"q writes an empty value as quotes", `x=""; printf "[%s]" "${(q)x}"`, `['']`},
		{"q gives a newline its own segment", `x="$(printf 'a\nb')"; printf "[%s]" "${(q)x}"`, `[a$'\n'b]`},
		{"qq single-quotes unconditionally", `x=plain; printf "[%s]" "${(qq)x}"`, `['plain']`},
		{"qq escapes an embedded quote", `x="don't"; printf "[%s]" "${(qq)x}"`, `['don'\''t']`},
		{"qqq double-quotes", `x='a $b'; printf "[%s]" "${(qqq)x}"`, `["a \$b"]`},
		{"qqqq spells the dollar-quote form", `x="a b"; printf "[%s]" "${(qqqq)x}"`, `[$'a b']`},
		{"q applies after the operator", `unset u; printf "[%s]" "${(q)u:-a b}"`, `[a\ b]`},

		{"j joins an array", `a=(x y z); printf "[%s]" "${(j.,.)a}"`, "[x,y,z]"},
		{"j on a scalar changes nothing", `x=abc; printf "[%s]" "${(j.,.)x}"`, "[abc]"},

		{"P reads the value as a name", `y=hello; x=y; printf "[%s]" "${(P)x}"`, "[hello]"},
		{"P on a missing target is empty", `x=nosuch; printf "[%s]" "${(P)x}"`, "[]"},
		{"P resolves before the test", `x=y; unset y; printf "[%s]" "${(P)x:-def}"`, "[def]"},

		{"k yields the keys sorted", `typeset -A m; m=(k1 v1 k2 v2); printf "[%s]" "${(k)m}"`, "[k1 k2]"},
		{"kv interleaves key and value", `typeset -A m; m=(k1 v1 k2 v2); printf "[%s]" "${(kv)m}"`, "[k1 v1 k2 v2]"},
		{"v alone is the values", `typeset -A m; m=(k1 v1 k2 v2); printf "[%s]" "${(v)m}"`, "[v1 v2]"},
		{"k on a plain array is a no-op", `a=(x y); printf "[%s]" "${(k)a}"`, "[x y]"},

		{"percent doubles to a literal", `printf "[%s]" "${(%):-100%%}"`, "[100%]"},
		{"percent applies to the value", `x="%%"; printf "[%s]" "${(%)x}"`, "[%]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

func TestExpansionFlagFields(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{
			"f splits at newlines",
			`x="$(printf 'a\nb\nc')"; for w in ${(f)x}; do printf "[%s]" "$w"; done`,
			"[a][b][c]",
		},
		{
			"s splits at its separator",
			`x=a:b:c; for w in ${(s.:.)x}; do printf "[%s]" "$w"; done`,
			"[a][b][c]",
		},
		{
			"the separator may be several characters",
			`x=aXXbXXc; for w in ${(s:XX:)x}; do printf "[%s]" "$w"; done`,
			"[a][b][c]",
		},
		{
			"an empty separator splits into characters",
			`x=ab; for w in ${(s::)x}; do printf "[%s]" "$w"; done`,
			"[a][b]",
		},
		{
			"an unquoted split drops empty words",
			`x=a::b; for w in ${(s.:.)x}; do printf "[%s]" "$w"; done`,
			"[a][b]",
		},
		{
			"splitting forces fields even in quotes",
			`x=a::b; for w in "${(s.:.)x}"; do printf "[%s]" "$w"; done`,
			"[a][b]",
		},
		{
			"at keeps the empty word in quotes",
			`x=a::b; for w in "${(@s.:.)x}"; do printf "[%s]" "$w"; done`,
			"[a][][b]",
		},
		{
			"at keeps an array's fields in quotes",
			`a=(x "y z" ""); for w in "${(@)a}"; do printf "[%s]" "$w"; done`,
			"[x][y z][]",
		},
		{
			"without at a quoted array joins",
			`a=(ab cd); for w in "${(U)a}"; do printf "[%s]" "$w"; done`,
			"[AB CD]",
		},
		{
			"positional parameters keep their fields in quotes",
			`set -- "a b" c; for w in "${(U)@}"; do printf "[%s]" "$w"; done`,
			"[A B][C]",
		},
		{
			"an array joins before it splits",
			`a=(a:b c:d); for w in ${(s.:.)a}; do printf "[%s]" "$w"; done`,
			"[a][b c][d]",
		},
		{
			"case then split compose",
			`x="$(printf 'a\nB')"; for w in ${(Lf)x}; do printf "[%s]" "$w"; done`,
			"[a][b]",
		},
		{
			"an empty value splits to nothing unquoted",
			`x=""; n=0; for w in ${(f)x}; do n=$((n+1)); done; printf "n=%s" "$n"`,
			"n=0",
		},
		{
			"and to one empty field quoted with at",
			`x=""; n=0; for w in "${(@f)x}"; do n=$((n+1)); done; printf "n=%s" "$n"`,
			"n=1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// A character the group cannot carry is a runtime error with the measured
// wording and position — and no error at all in a branch never taken.
func TestExpansionFlagErrors(t *testing.T) {
	out, errs, st := flagsRun(t, `x=a; echo "${(!)x}"; echo after`)
	if !strings.Contains(errs, "error in flags near position 4 in '${(!)x}'") {
		t.Errorf("stderr = %q, want the flags error with its position", errs)
	}
	if strings.Contains(out, "after") || st != 1 {
		t.Errorf("out=%q st=%d, want the line abandoned with status 1", out, st)
	}

	out, errs, st = flagsRun(t, `if false; then echo "${(!)x}"; fi; echo ok`)
	if !strings.Contains(out, "ok") || errs != "" || st != 0 {
		t.Errorf("out=%q errs=%q st=%d, want a silent ok for a branch never taken", out, errs, st)
	}

	out, errs, st = flagsRun(t, `x=a; echo "${(Ux}"; echo after`)
	if !strings.Contains(errs, "error in flags near position 5 in '${(Ux}'") {
		t.Errorf("stderr = %q, want the position just past the end", errs)
	}
	if strings.Contains(out, "after") || st != 1 {
		t.Errorf("out=%q st=%d, want the line abandoned with status 1", out, st)
	}
}

// A flag the grammar accepts and this interpreter does not carry is refused
// by name, because a quiet wrong answer is the one thing worse.
func TestAnUnimplementedExpansionFlagIsRefusedByName(t *testing.T) {
	out, errs, st := flagsRun(t, `x=b; echo "${(e)x}"; echo after`)
	if !strings.Contains(errs, "the (e) expansion flag is not implemented") {
		t.Errorf("stderr = %q, want the flag named", errs)
	}
	if strings.Contains(out, "after") || st != 1 {
		t.Errorf("out=%q st=%d, want the line abandoned with status 1", out, st)
	}

	_, errs, st = flagsRun(t, `echo "${(A)=r::=a b c}"`)
	if !strings.Contains(errs, "the (A) expansion flag is not implemented") || st != 1 {
		t.Errorf("errs=%q st=%d, want the (A) flag refused by name", errs, st)
	}
}

// The `%` flag carries the escapes a script uses to find its own path, and
// refuses the rest of the prompt language by name.
func TestThePercentFlagNamesTheFileBeingRead(t *testing.T) {
	// Under a command string there is no file, so the shell names itself —
	// measured; the wild idiom `${(%):-%x}` reads the *sourced* file's path,
	// which the source test below pins.
	out, errs, st := flagsRun(t, `printf "[%s][%s]" "${(%):-%x}" "${(%):-%N}"`)
	if out != "[testsh][testsh]" || errs != "" || st != 0 {
		t.Errorf("out=%q errs=%q st=%d, want the shell's own name twice", out, errs, st)
	}

	out, errs, st = flagsRun(t, `fn() { printf "[%s]" "${(%):-%N}"; }; fn`)
	if out != "[fn]" || errs != "" || st != 0 {
		t.Errorf("out=%q errs=%q st=%d, want the function's name", out, errs, st)
	}

	out, errs, st = flagsRun(t, `echo "${(%):-%M}"; echo after`)
	if !strings.Contains(errs, "the %M prompt escape is not implemented") {
		t.Errorf("stderr = %q, want the escape named", errs)
	}
	if strings.Contains(out, "after") || st != 1 {
		t.Errorf("out=%q st=%d, want the line abandoned with status 1", out, st)
	}
}

// The wild idiom: `${(%):-%x}` inside a sourced file is that file's path —
// how a script finds its own directory in the dialect that has the construct.
func TestThePercentFlagNamesTheSourcedFile(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "inner.sh", `printf "[%s][%s]" "${(%):-%x}" "${(%):-%N}"`+"\n")
	out, errs, st := flagsRun(t, ". "+path)
	if out != "["+path+"]["+path+"]" || errs != "" || st != 0 {
		t.Errorf("out=%q errs=%q st=%d, want the sourced file's path twice", out, errs, st)
	}
}
