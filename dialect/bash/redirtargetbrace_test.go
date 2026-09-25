// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A redirection's target is brace-expanded here too, and what the braces make
// is an *ambiguity* rather than several targets — this dialect reads a target
// as an ordinary word, and several names there is the complaint it already
// has for `e="a b"; > $e`.
//
// The rows are here as the panel's other answer beside dialect/zsh's, which
// gained the expansion in #4455: a change keyed on the shell that fans a
// stream out to every name it was given has to leave this side where it was,
// and nothing else in the tree says so end to end. Measured 2026-09-25
// against bash 5.3.20 (`--norc --noprofile`) and bash 3.2.57, each row in an
// empty directory of its own:
//
//	: > {a,b}                 `{a,b}: ambiguous redirect`, 1, no file
//	: > d/{a,b,c}             `d/{a,b,c}: ambiguous redirect`, 1, no file
//	: > {1..3}                `{1..3}: ambiguous redirect`, 1, no file
//	f() { echo 3; }; : > {1..$(f)}   one file called `{1..3}`, 0
//	: > "{a,b}"               one file called `{a,b}`, 0
//
// The fourth row is the one worth keeping: braces are read here before a
// substitution is, so an endpoint written as one is not an endpoint, the range
// never forms, and the target is the single name the text came to. It is also
// the row that says the count is taken without running anything twice — `f`
// runs once.
func TestARedirectionTargetsBracesAreStillAmbiguousHere(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		file            string
	}{
		{name: "two names is none", src: `: > {a,b}`, want: "{a,b}: ambiguous redirect"},
		{
			name: "the line dialect/zsh was made for",
			src:  `: > d/{a,b,c}`,
			want: "d/{a,b,c}: ambiguous redirect",
		},
		{name: "a range", src: `: > {1..3}`, want: "{1..3}: ambiguous redirect"},
		{name: "text behind the group", src: `: > {a,b}c`, want: "{a,b}c: ambiguous redirect"},
		{
			// No range forms, so there is one name and it is written.
			name: "a range whose end is an expansion is one name",
			src:  `f() { echo 3; }; : > {1..$(f)}`,
			file: "{1..3}",
		},
		{name: "quoted braces are not syntax", src: `: > "{a,b}"`, file: "{a,b}"},
		{
			name: "and braces that arrived from a parameter are not either",
			src:  `e="{a,b}"; : > $e`,
			file: "{a,b}",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out, st := runBash(t, dir, tc.src)
			if tc.want != "" {
				if st == 0 || !strings.Contains(out, tc.want) {
					t.Fatalf("%s gave %q (status %d), want %q and a nonzero status", tc.src, out, st, tc.want)
				}
				// And nothing was opened: the refusal is what the shell did
				// instead of writing, not a remark beside a file.
				if names, err := os.ReadDir(dir); err != nil || len(names) != 0 {
					t.Errorf("%s left %v (%v), want the directory untouched", tc.src, names, err)
				}
				return
			}
			if out != "" || st != 0 {
				t.Fatalf("%s gave %q (status %d), want nothing at 0", tc.src, out, st)
			}
			if _, err := os.Stat(filepath.Join(dir, tc.file)); err != nil {
				names, _ := os.ReadDir(dir)
				t.Errorf("%s left %v, want one file called %q", tc.src, names, tc.file)
			}
		})
	}
}
