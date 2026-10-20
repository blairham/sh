// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// refusalLine is the one line of out that carries the coprocess refusal, with
// everything the script itself printed thrown away.
//
// Written as a search rather than as a whole-output comparison because the
// rows in front of the refusal print, and what is being pinned is the location
// and nothing else.
func refusalLine(t *testing.T, out string) string {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "process already exists") {
			return line
		}
	}
	t.Fatalf("no refusal in %q", out)
	return ""
}

// The refusal is sited at the last statement the shell *entered*, which is not
// the operator that was refused and is not the statement in front of it
// either.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-18, from a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null device. Every
// file below ends with a second `cat |&`, and the number is the line the
// sentence names:
//
//	cat |& / cat |&                             line 1
//	echo a / echo b / the pair                  line 2
//	echo a / if true; then / the pair / fi      line 2
//	the same with while, until, for and case    line 2
//	echo a / ( cat |& / cat |& )                line 1
//	echo a / { cat |& / cat |& }                line 1
//	echo a / f(){ the pair } / f on line 6      line 6
//	echo a / cat |& / { echo z; } / cat |&      line 3
//	echo a / cat |& / ( echo z ) / cat |&       line 1
//	echo a / { echo p / echo q; } / the pair    line 3
//	echo a / ( echo p / echo q ) / the pair     line 1
//
// One rule accounts for all of them: a coprocess statement does not advance
// the count, a grouping's head — `(` or `{` — is not a statement, a compound's
// keyword head is, and what runs inside a `( … )` advances a copy that goes
// away with the subshell while what runs inside a `{ … }` advances this one.
//
// The rows that answer line 1 are written here as a location with no number in
// it, because this shell leaves `line 1` out of a location on the route these
// tests take — measured the same day, `ksh -c 'nosuchcmd'` being `ksh:
// nosuchcmd: not found` and the same on line 2 carrying `line 2`.
func TestTheCoprocessRefusalNamesTheLastStatementEntered(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"nothing has been entered", "cat |&\ncat |&\n",
			"ksh: process already exists",
		},
		{
			"the statement before the pair", "echo a\necho b\ncat |&\ncat |&\n",
			"ksh: line 2: process already exists",
		},
		{
			"a keyword head, and not the body", "echo a\nif true; then\ncat |&\ncat |&\nfi\n",
			"ksh: line 2: process already exists",
		},
		{
			"a loop's keyword head", "echo a\nwhile true; do\ncat |&\ncat |&\nbreak\ndone\n",
			"ksh: line 2: process already exists",
		},
		{
			"a for head", "echo a\nfor i in 1; do\ncat |&\ncat |&\ndone\n",
			"ksh: line 2: process already exists",
		},
		{
			"a case head", "echo a\ncase x in x)\ncat |&\ncat |&\n;; esac\n",
			"ksh: line 2: process already exists",
		},
		{
			// A subshell's head is not a statement and its body's lines go
			// with the subshell, so both rows stand where `echo a` left the
			// count.
			"a subshell holding the pair", "echo a\n( cat |&\ncat |& )\n",
			"ksh: process already exists",
		},
		{
			"a subshell that ran a command of its own", "echo a\ncat |&\n( echo z )\ncat |&\n",
			"ksh: process already exists",
		},
		{
			"a two-line subshell", "echo a\n( echo p\necho q )\ncat |&\ncat |&\n",
			"ksh: process already exists",
		},
		{
			// A brace group's head is not a statement either — but what is
			// inside one is at this level, so the pair alone leaves the count
			// where it was and an ordinary command in one moves it.
			"a brace group holding the pair", "echo a\n{ cat |&\ncat |& }\n",
			"ksh: process already exists",
		},
		{
			"a brace group that ran a command of its own", "echo a\ncat |&\n{ echo z; }\ncat |&\n",
			"ksh: line 3: process already exists",
		},
		{
			"a two-line brace group", "echo a\n{ echo p\necho q; }\ncat |&\ncat |&\n",
			"ksh: line 3: process already exists",
		},
		{
			// The call, not the body: the two coprocesses inside it never
			// advance the count and the `f` on line 6 does.
			"a function call", "echo a\nf(){\ncat |&\ncat |&\n}\nf\n",
			"ksh: line 6: process already exists",
		},
		{
			// The control for the rule's own shape: an ordinary statement
			// between the two operators does move it, so this is not a rule
			// about the first coprocess's line.
			"an ordinary statement between the two", "echo a\ncat |&\necho b\ncat |&\n",
			"ksh: line 3: process already exists",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runKshWithTools(t, tc.src)
			if got := refusalLine(t, out); got != tc.want {
				t.Errorf("%q\n got %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}

// And the dialect's own prelude is not the script's text, so its lines are not
// a statement the script entered.
//
// The row that needs it is the shortest one there is — `cat |&` twice and
// nothing else — because that is the only shape where nothing in the script
// has advanced the count and whatever the prelude left behind is what a
// diagnostic would read. Without the clear, the shipped binary blamed line 22
// of text no script ever saw.
func TestThePreludesOwnLinesAreNotAStatementTheScriptEntered(t *testing.T) {
	out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{
		Dir:  t.TempDir(),
		Vars: map[string]string{"PATH": "/usr/bin:/bin"},
	}, "cat |&\ncat |&\n")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := refusalLine(t, out), "ksh: process already exists"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
