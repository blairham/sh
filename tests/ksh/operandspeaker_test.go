// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A declaration utility names itself for what it does to an operand it reads
// itself, and not for an operand the shell stored ahead of it. Measured
// 2026-10-31 on ksh93u+ 2012-08-01, `-c`:
//
//	typeset -A a; readonly a; typeset 'a[k]=1'    typeset: a: is read only
//	… export 'a[k]=1' / readonly 'a[k]=1'         export: … / readonly: …
//	… integer 'a[k]=1'                            typeset: a: is read only
//	typeset -a a; readonly a; typeset 'a[1]=1'    typeset: a: is read only
//	… typeset a[k]=1, export a[k]=1               a: is read only
//	typeset -A a; set -x; typeset 'a[]=1'         typeset: warning: adding empty subscript
//	… typeset a[""]=1                             warning: adding empty subscript
//	… k=; let "a[$k]=5"                           let: warning: adding empty subscript
//	… k=; (( a[$k]=5 ))                           warning: adding empty subscript
//
// The noun is the **route**, not the table or the subscript: an assignment
// word is stored by the shell before the utility runs (see
// interp/declarationinturn.go), so its refusal and its warning are the
// shell's own, where a quoted operand, or one an expansion produced, is the
// utility's to store. And `let`, which names itself for a refusal already,
// names itself for the warning too (#7188).
func TestADeclarationNamesItselfForAnOperandItReads(t *testing.T) {
	const warn = "warning: adding empty subscript"
	for _, c := range []struct{ name, src, want string }{
		{"typeset's key", `typeset -A a; readonly a; typeset 'a[k]=1'`, "typeset: a: is read only"},
		{"export's key", `typeset -A a; readonly a; export 'a[k]=1'`, "export: a: is read only"},
		{"readonly's key", `typeset -A a; readonly a; readonly 'a[k]=1'`, "readonly: a: is read only"},
		{"integer, as the typeset it is", `typeset -A a; readonly a; integer 'a[k]=1'`, "typeset: a: is read only"},
		{"an indexed element", `typeset -a a; readonly a; typeset 'a[1]=1'`, "typeset: a: is read only"},
		{"an element of a frozen scalar", `readonly b=1; typeset 'b[1]=2'`, "typeset: b: is read only"},
		{"an operand an expansion made", `typeset -A a; readonly a; x='a[k]=1'; typeset $x`, "typeset: a: is read only"},
		{"typeset's empty key", `typeset -A a; set -x; typeset 'a[]=1'`, "typeset: " + warn},
		{"export's empty key", `typeset -A a; set -x; export 'a[]=1'`, "export: " + warn},
		{"readonly's empty key", `typeset -A a; set -x; readonly 'a[]=1'`, "readonly: " + warn},
		{"a valueless operand's empty key", `typeset -A a; set -x; typeset 'a[]'`, "typeset: " + warn},
		{"let's empty key", `typeset -A a; k=; set -x; let "a[$k]=5"`, "let: " + warn},
		// Under a `function` the builtin's location comes with its name.
		{"in a function", `function f { typeset -A a; readonly a; typeset 'a[k]=1'; }; f`, "f[1]: typeset: a: is read only"},
		{"a warning in a function", `function f { typeset -A a; set -x; typeset 'a[]=1'; }; f`, "f[1]: typeset: " + warn},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if !strings.Contains(out, ": "+c.want+"\n") {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The controls the rule is drawn with: the same stores written as assignment
// words, and through the arithmetic command, name no builtin. That holds where
// a `function` body leaves the store to the utility as well, because the body
// may shadow — `export` and `readonly` make no local, so the utility refuses
// there, and ksh93u+ still writes the shell's line form. The warning a
// `getopts` store makes stays unlocated, which is #6683's and is checked
// beside it.
func TestAnAssignmentWordsRefusalNamesNoBuiltin(t *testing.T) {
	for _, c := range []struct{ name, src, want, not string }{
		{"typeset's key", `typeset -A a; readonly a; typeset a[k]=1`, "ksh: a: is read only", "typeset:"},
		{"export's key", `typeset -A a; readonly a; export a[k]=1`, "ksh: a: is read only", "export:"},
		{"an indexed element", `typeset -a a; readonly a; typeset a[1]=1`, "ksh: a: is read only", "typeset:"},
		{"typeset's empty key", `typeset -A a; set -x; typeset a[""]=1`, "ksh: warning: adding empty subscript", "typeset:"},
		{"the arithmetic command's", `typeset -A a; k=; set -x; (( a[$k]=5 ))`, "ksh: warning: adding empty subscript", "let:"},
		{"export in a function", `readonly b=1; function f { export b=2; }; f`, "f: line 1: b: is read only", "export:"},
		{"readonly in a function", `readonly b=1; function f { readonly b=2; }; f`, "f: line 1: b: is read only", "readonly:"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if !strings.Contains(out, c.want+"\n") || strings.Contains(out, c.not) {
				t.Errorf("got %q, want %q with no %q in it", out, c.want, c.not)
			}
		})
	}
}

// A name restricted mode froze keeps the mode's sentence when the builtin
// names itself, and an assignment word keeps the shell's line form. Measured
// 2026-10-31 on ksh93u+ from a script file, with `set -r` first:
//
//	f() { typeset 'PATH=x'; }; f              f[2]: typeset: PATH: restricted
//	function f { typeset 'PATH=x'; }; f       the same
//	function f { typeset PATH=x; }; f         f: line 2: PATH: restricted
func TestARestrictedNameRefusedByTheUtilityKeepsTheModesSentence(t *testing.T) {
	for _, c := range []struct{ name, src, want, not string }{
		{"a quoted operand", "set -r\nf() { typeset 'PATH=x'; }\nf\n", "typeset: PATH: restricted", "is read only"},
		{"in a function body", "set -r\nfunction f { typeset 'PATH=x'; }\nf\n", "typeset: PATH: restricted", "is read only"},
		{"an assignment word", "set -r\nfunction f { typeset PATH=x; }\nf\n", "PATH: restricted", "typeset:"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if !strings.Contains(out, ": "+c.want+"\n") || strings.Contains(out, c.not) {
				t.Errorf("got %q, want %q with no %q in it", out, c.want, c.not)
			}
		})
	}
}
