// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// This shell renders the whole chain of borrowed texts into a diagnostic's
// prefix, each frame with the line in it that entered the next:
// `./n.sh[2]: .[2]: .: line 3: …` (#2461).
//
// Measured 2026-09-12 on ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin` with
// a scratch HOME. Over a script file:
//
//	s.sh line 2 sources p.sh          ./s.sh[2]: .: line 3: …
//	t.sh line 1 defines f that sources ./t.sh[1]: .: line 3: …
//	n.sh sources s.sh sources p.sh    ./n.sh[2]: .[2]: .: line 3: …
//	e.sh line 2 runs a 4-line eval    ./e.sh[2]: eval: line 3: …
//	bs.sh line 2 sources a bad file   ./bs.sh[2]: .: syntax error at line 2: …
//	be.sh line 2 evals bad text       ./be.sh[2]: eval: syntax error at line 2: …
//
// and over `-c`, which is the route these cases take:
//
//	. ./p.sh          /bin/ksh: .: line 3: …
//	. ./s.sh          /bin/ksh: .[2]: .: line 3: …
//	f(){ . ./p.sh; }  /bin/ksh: .: line 3: …
//	. ./bad.sh        /bin/ksh: .: syntax error at line 2: …
//	. ./bs.sh         /bin/ksh: .[2]: .: syntax error at line 2: …
//
// Three things in those rows are the whole of the rule, and each was a way to
// get it wrong:
//
//   - **A function frame is not a component.** `t.sh` and the `-c` function
//     row say so: a function that sources a file adds no `f[…]`, and the
//     bracket is the line the `.` itself was written on.
//   - **The innermost component carries the location and the rest carry a
//     bracket**, and a parse failure's innermost carries neither, because the
//     message already says `at line N` and this shell does not say it twice.
//   - **The outermost component takes no bracket under `-c`**, because this
//     shell names no line for a `-c` program at all — its plain complaint
//     there is `/bin/ksh: NOPE: parameter not set`.
func TestTheCallStackIsRenderedIntoThePrefix(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("p.sh", "echo one\necho two\necho \"$NOPE\"\n")
	write("s.sh", "echo s1\n. ./p.sh\n")
	write("bad.sh", "if true\n")
	write("bs.sh", "echo b1\n. ./bad.sh\n")
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "one sourced file",
			src:  "set -u\n. ./p.sh\n",
			want: "ksh: .: line 3: NOPE: parameter not set\n",
		},
		{
			name: "a file that sources a file",
			src:  "set -u\n. ./s.sh\n",
			want: "ksh: .[2]: .: line 3: NOPE: parameter not set\n",
		},
		{
			name: "a function that sources adds no component",
			src:  "set -u\nf() { . ./p.sh; }\nf\n",
			want: "ksh: .: line 3: NOPE: parameter not set\n",
		},
		{
			name: "text eval is running",
			src:  "set -u\neval \"echo a\necho \\$NOPE\"\n",
			want: "ksh: eval: line 2: NOPE: parameter not set\n",
		},
		{
			// The innermost component has no location on the parse path.
			name: "a sourced file that will not parse",
			src:  ". ./bad.sh\n",
			want: "ksh: .: syntax error at line 2: `if' unmatched\n",
		},
		{
			name: "and one two levels down",
			src:  ". ./bs.sh\n",
			want: "ksh: .[2]: .: syntax error at line 2: `if' unmatched\n",
		},
		{
			name: "text eval cannot parse",
			src:  "eval \"if true\"\n",
			want: "ksh: eval: syntax error at line 1: `if' unmatched\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runKsh(t, dir, tc.src)
			if !containsWholeLine(out, tc.want) {
				t.Errorf("out = %q, want the whole line %q", out, tc.want)
			}
		})
	}
}

// containsWholeLine is a line-exact check rather than a Contains of the
// sentence: what this suite is about is everything *before* the sentence, and
// a fragment check cannot see a prefix at all.
func containsWholeLine(out, want string) bool {
	for _, line := range splitLines(out) {
		if line+"\n" == want {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	return lines
}
