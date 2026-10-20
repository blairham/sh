// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// An assignment **prefix**'s value is an assignment's value here, and this
// shell has no option for any other reading: `a=*.txt cmd` hands the command
// the six characters, and so do ksh93, dash and BusyBox ash. Only zsh has a
// switch — `GLOB_ASSIGN`, [interp.Semantics.ScalarAssignmentValueIsGlobbed]
// — and this column answers it No.
//
// This shell handed the command `a.txt b.txt c.txt` until #4657, because the
// prefix took the *word* road and a word globs. The road carries brace
// expansion and IFS splitting too, so the three rows are together: a fix that
// took only the glob back would leave the other two saying the road is still
// the word's.
//
// Measured on `/opt/homebrew/bin/bash`, GNU bash 5.3.20, 2026-09-26, in the
// directory below. `go version -m` on that binary says *not a Go executable*.
// Each run carries a `printf '[%s]' *.txt` control that prints the three
// names, so a row reporting `*.txt` is reporting a value that was not globbed
// rather than an empty directory.
func TestAPrefixAssignmentValueIsNotAPattern(t *testing.T) {
	probe := `f() { printf '[%s]' "$a"; }` + "\n"
	for _, c := range []struct{ name, src, want string }{
		{"several would match", `a=*.txt f`, "[*.txt]"},
		{"one would match", `a=one.* f`, "[one.*]"},
		{"none would match", `a=*.nomatch f`, "[*.nomatch]"},
		{"no metacharacter", `a=plain f`, "[plain]"},
		{"an append", "a=x\na+=*.txt f", "[x*.txt]"},
		{
			"the second of two",
			"g() { printf '[%s]' \"$b\"; }\na=plain b=*.txt g", "[*.txt]",
		},
		// The other two the assignment road decides, measured on the same
		// binary the same day: `a={p,q} printenv a` is `{p,q}` and `IFS=:;
		// v=a:b; a=$v printenv a` is `a:b`.
		{"not brace-expanded", `a={p,q} f`, "[{p,q}]"},
		{"not split on IFS", "IFS=:\nv=a:b\na=$v f", "[a:b]"},
		{"inner spaces kept", "v='p  q'\na=$v f", "[p  q]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := prefixGlobTree(t)
			out, st := runBash(t, root, probe+c.src+"\n")
			if out != c.want || st != 0 {
				t.Errorf("got %q at %d, want %q", out, st, c.want)
			}
			// The control, in the same directory and the same run: the
			// pattern really does reach three names here.
			if ctl, _ := runBash(t, root, `printf '[%s]' *.txt`); ctl != "[a.txt][b.txt][c.txt]" {
				t.Errorf("control = %q, want the three names", ctl)
			}
		})
	}
}

// prefixGlobTree builds the directory those cases run in.
func prefixGlobTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{"a.txt", "b.txt", "c.txt", "one.only", "plain"} {
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
