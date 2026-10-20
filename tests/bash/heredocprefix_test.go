// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A here-document inside `$( )` whose delimiter carries the rest of the line.
//
// syntax/heredocprefix_test.go pins the rule against the grammar flag; this is
// the whole shell, which is the only place the two routes into a
// substitution's body meet. The lexer reads that body in place where the
// grammar can find the `)`, and the interpreter reads it again at expansion
// time where it cannot — so a change that answered only one of them would pass
// a parser test and still leave `$(cat <<E … E )` wrong at a prompt.
//
// Measured 2026-09-18 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null, bash
// 5.3.20. The warning precedes every row in both columns and is already
// byte-exact here, so what these assert is the *value*.
func TestAHereDocumentDelimiterCarriesTheRestOfTheLine(t *testing.T) {
	for _, c := range []struct{ name, tail, want string }{
		// The shape #1021 was filed on: a blank between the delimiter and the
		// parenthesis, which #963's rule cannot see.
		{"a blank before the closer", "E )", "[w]\n"},
		// The remainder is program text, so it runs. `x` is not on the PATH
		// this runs under, which is what makes the row visible at all.
		{"a command after the delimiter", "E x)", "[w]\n"},
		// No blank at all — the row a rule written around a separator gets
		// wrong.
		{"no blank at all", "Ex)", "[w]\n"},
		// And a remainder that itself begins with the delimiter.
		{"the remainder begins with the delimiter", "EE)", "[w]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "v=$(cat <<E\nw\n" + c.tail + "\necho \"[$v]\"\n"
			out, _, err := preset.Combined(t, withCat(t), src)
			if err != nil {
				t.Fatalf("%q: %v", src, err)
			}
			if got := lastLine(out); got != c.want {
				t.Errorf("%q gave %q, want %q", src, got, c.want)
			}
		})
	}
}

// And the controls, which are what say the rule is a recovery at the end of a
// substitution's text rather than a prefix match on every line.
//
// The first is the one that decides the whole shape: `EXTRA` begins with the
// delimiter and the document is closed properly two lines later, so `EXTRA` is
// body. A rule asked of every line would answer `[]` here.
func TestWhatTheDelimiterPrefixMustNotReach(t *testing.T) {
	for _, c := range []struct{ name, src, want, why string }{
		{
			name: "a body line beginning with the delimiter",
			src:  "v=$(cat <<E\nEXTRA\nE\n)\necho \"[$v]\"\n",
			want: "[EXTRA]\n",
			why:  "the document is closed on its own line, so nothing is being recovered",
		},
		{
			name: "the same with the closer moved up",
			src:  "v=$(cat <<E\nEXTRA\nE )\necho \"[$v]\"\n",
			want: "[EXTRA]\n",
			why:  "the `E ` line is consumed and `EXTRA` stays body, which is the pair that says the rule is about the last line only",
		},
		{
			name: "the backquoted spelling",
			src:  "v=`cat <<E\nw\nE `\necho \"[$v]\"\n",
			want: "[w\nE ]\n",
			why:  "measured: the old-style substitution does not take this route and the line is body, exactly as it is in bash 3.2",
		},
		{
			name: "a plain file, outside any substitution",
			src:  "cat <<E\nw\nE x\n",
			want: "w\nE x\n",
			why:  "the discriminating shape puts the prefix line last; every shell in the panel gives cat the whole of it",
		},
		{
			name: "the delimiter with the closer behind it",
			src:  "v=$(cat <<E\nw\nE)\necho \"[$v]\"\n",
			want: "[w]\n",
			why:  "#963's own shape, which is answered by the stricter rule and must go on being",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, withCat(t), c.src)
			if err != nil {
				t.Fatalf("%q: %v", c.src, err)
			}
			if got := lastLines(out, strings.Count(c.want, "\n")); got != c.want {
				t.Errorf("%q gave %q, want %q — %s", c.src, got, c.want, c.why)
			}
		})
	}
}

// withCat is a scratch runner that can find `cat`, which is the one program
// these cases need: a here-document is a body given to a command, and nothing
// this shell has built in reads one and prints it back.
func withCat(t *testing.T) dialecttest.Base {
	t.Helper()
	return dialecttest.Base{Dir: t.TempDir(), Vars: map[string]string{"PATH": "/usr/bin:/bin"}}
}

// lastLine is the final line of a run's output, warning and all else dropped.
//
// The warning is on its own line in front of every row above and is asserted
// nowhere here: it was already byte-exact before this rule existed, and
// repeating it in each row would make these tests about the wording rather
// than about the value.
func lastLine(out string) string { return lastLines(out, 1) }

// lastLines is the final n lines of a run's output.
func lastLines(out string, n int) string {
	lines := strings.SplitAfter(out, "\n")
	// SplitAfter leaves an empty tail after a final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "")
}
