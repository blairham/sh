// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The numbered tilde: `~1`, `~+1` and `~-1` name an entry of the directory
// stack rather than somebody's home.
//
// Named by the axis and not by a shell, which is what makes the rows here
// about the rule rather than about bash: the stack is whatever array
// [Semantics.DirectoryStackParameter] names, so the test supplies one and
// says what each spelling should pull out of it.
//
// The stack is written `top mid bot` throughout, so a row's answer says which
// end it counted from without the reader having to hold three names.
func runStackTilde(t *testing.T, param, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArrayLiteral = true
		d.ArraySubscript = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.DirectoryStackParameter = param
		r.Semantics = &sem
	})
}

func TestANumberedTildeNamesADirectoryStackEntry(t *testing.T) {
	const stack = "stack=(top mid bot)\n"
	for _, c := range []struct {
		word string
		want string
	}{
		{"~0", "top"},
		{"~1", "mid"},
		{"~2", "bot"},
		// Past the end is the word as written, and it is **not** then looked
		// for as a name: a shell that fell through to the user database here
		// could be handed a home directory for a login called `3`.
		{"~3", "~3"},
		{"~+0", "top"},
		{"~+2", "bot"},
		{"~+3", "~+3"},
		// The minus form counts from the bottom, so zero is the last entry.
		{"~-0", "bot"},
		{"~-1", "mid"},
		{"~-2", "top"},
		{"~-3", "~-3"},
		// Leading zeros are a number like any other, and a name with a digit
		// in it is not a number at all.
		{"~01", "mid"},
		{"~+01", "mid"},
		{"~1a", "~1a"},
		// What follows the first slash is the tail, exactly as it is for a
		// home directory.
		{"~1/x", "mid/x"},
	} {
		t.Run(c.word, func(t *testing.T) {
			out, status := runStackTilde(t, "stack", stack+"echo "+c.word)
			if status != 0 {
				t.Fatalf("status %d, output %q", status, out)
			}
			if out != c.want+"\n" {
				t.Errorf("echo %s = %q, want %q", c.word, out, c.want+"\n")
			}
		})
	}
}

// And with no parameter named, a numbered tilde is text.
//
// The empty name is the whole of "this shell has no numbered tilde", so this
// is the row that says the feature is the dialect's rather than the core's.
// Without it the rule would be on in every preset and the field would be
// documentation rather than a switch.
func TestANumberedTildeIsTextWithNoDirectoryStackNamed(t *testing.T) {
	out, status := runStackTilde(t, "", "stack=(top mid bot)\necho ~1")
	if status != 0 {
		t.Fatalf("status %d, output %q", status, out)
	}
	if out != "~1\n" {
		t.Errorf("echo ~1 = %q, want %q", out, "~1\n")
	}
}
