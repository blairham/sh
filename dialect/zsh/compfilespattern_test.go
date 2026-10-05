// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// compfiles answers, asked from inside a completion with chosen arguments
// (#6144). Every row was measured 2026-10-05 against zsh 5.9.2 the same way;
// the answer is handed back through `compadd -QU` so the test reads exactly
// what the builtin left.
func TestCompfilesBuildsThePatternAndNarrows(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{
			"the word becomes a pattern",
			`PREFIX=RE; tmp1=(''); compfiles -p tmp1 accex '' ' ' '' fake '*'`, "st=0 RE*",
		},
		{
			"each element, then the skipped part",
			`PREFIX=RE; tmp1=('' docs); compfiles -p tmp1 accex '/' ' ' '' fake '*'`, "st=0 /RE* docs/RE*",
		},
		{
			"directories for -P",
			`PREFIX=RE; tmp1=(''); compfiles -P tmp1 accex '' ' ' '' fake`, "st=0 RE*(-/)",
		},
		{
			"the trailing dash leaves the word out",
			`PREFIX=RE; tmp1=(''); compfiles -p- tmp1 accex '' ' ' '' fake '*'`, "st=0 *",
		},
		{
			"the file pattern goes last",
			`PREFIX=RE; tmp1=(''); compfiles -p tmp1 accex '' ' ' '' fake '*.md'`, "st=0 RE*.md",
		},
		{
			"an empty word is everything",
			`PREFIX=; tmp1=(''); compfiles -p tmp1 accex '' ' ' '' fake '*'`, "st=0 *",
		},
		{
			"glob characters are quoted, a blank is not",
			`PREFIX='a*b'; tmp1=('x y'); compfiles -p tmp1 accex '/' ' ' '' fake '*'`, `st=0 x y/a\*b*`,
		},
		{
			"a case-folding spec makes classes",
			`PREFIX=RE; tmp1=(''); compfiles -p tmp1 accex '' 'm:{a-zA-Z}={A-Za-z}' '' fake '*'`, "st=0 [Rr][Ee]*",
		},
		{
			"a spec that misses the word leaves it",
			`PREFIX=RE; tmp1=(''); compfiles -p tmp1 accex '' 'm:{a-z}={A-Z}' '' fake '*'`, "st=0 RE*",
		},
		{
			"two specs at one character keep nothing",
			`PREFIX=re; tmp1=(''); compfiles -p tmp1 accex '' 'm:{a-z}={A-Z} m:{a-z}={A-Z}' '' fake '*'`, "st=0 *",
		},
		{
			"an anchor lets anything come before it",
			`PREFIX=a.b; tmp1=(''); compfiles -p tmp1 accex '' 'r:|[._-]=* r:|=*' '' fake '*'`, "st=0 a*.b*",
		},
		{
			"anything before the start keeps nothing",
			`PREFIX=RE; tmp1=(''); compfiles -p tmp1 accex '' 'l:|=* r:|=*' '' fake '*'`, "st=0 *",
		},
		{
			"-r narrows by a finished first component",
			`tmp1=(ab/x ac/x); compfiles -r tmp1 ab/x`, "st=0 ab/x",
		},
		{
			"-r with no element of that component",
			`tmp1=(ab/x ac/x); compfiles -r tmp1 a/x`, "st=1 ab/x ac/x",
		},
		{
			"-r reads only the first component",
			`tmp1=(a/b/c a/bb/c); compfiles -r tmp1 a/b/c`, "st=0 a/b/c a/bb/c",
		},
		{
			"-r with no slash and two matches",
			`tmp1=(a ab); compfiles -r tmp1 a`, "st=1 a ab",
		},
		{
			"-r with no slash and one match",
			`tmp1=(ab); compfiles -r tmp1 a`, "st=0 ab",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			body := "local -a tmp1; " + c.body + `; compadd -QU -- "st=$? ${tmp1[*]}"`
			got := completionFor(t, widgetOf(body), "x ")
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
