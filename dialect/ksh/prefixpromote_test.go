// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// What this shell does with the name an assignment prefix stands in front of
// at a builtin. Measured 2026-09-16 on ksh93u+ 2012-08-01, `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, from a script file with stdin closed (#3437).

// The attribute comes *off* for the length of the command, which is the third
// answer and this shell alone: a prefix here is an ordinary assignment to the
// shell, and an ordinary assignment is in nobody's environment. The same
// direction this shell moves a prefix to a function in.
//
// Read off the listing rather than a child's environment, so the probe starts
// no program: `z=2 eval 'env | grep "^z="'` shows the child nothing here and
// `z=2` in the other five columns, and the bare `z=2` below is that same fact
// where the shell can answer it itself.
func TestAPrefixAtABuiltinUnexportsTheNameHere(t *testing.T) {
	if got := ksh.Semantics().PrefixExportAtABuiltin; got != interp.PrefixExportAtABuiltinOff {
		t.Errorf("PrefixExportAtABuiltin = %v, want %v", got, interp.PrefixExportAtABuiltinOff)
	}
	out, st := runKsh(t, t.TempDir(), `export z=1
z=2 typeset -p z
c=1
c=2 typeset -p c`)
	want := "z=2\nc=2\n"
	if out != want || st != 0 {
		t.Errorf("a prefix at a builtin = %q (status %d), want %q", out, st, want)
	}
}

// And a declaration keeps nothing by this axis. Every row a keeping shell is
// measured on already reads the prefix's value here, because a prefix persists
// on a special builtin and on `typeset` alike — which is a different question
// with a different answer, and the `-i` row is what tells the two apart: no
// attribute is named there and the value stays all the same.
func TestADeclarationKeepsNoneOfItsPrefixHere(t *testing.T) {
	if got := ksh.Semantics().DeclarationPromotesThePrefixEntry; got != interp.No {
		t.Errorf("DeclarationPromotesThePrefixEntry = %v, want No", got)
	}
	out, st := runKsh(t, t.TempDir(), `b=7; b=8 readonly b;   typeset -p b
y=1; y=2 typeset -r y; typeset -p y
i=1; i=2 typeset -i i; typeset -p i
x=1; x=2 typeset x;    typeset -p x`)
	want := "typeset -r b=8\ntypeset -r y=2\ntypeset -i i=2\nx=2\n"
	if out != want || st != 0 {
		t.Errorf("a declaration over its own prefix = %q (status %d), want %q", out, st, want)
	}
}
