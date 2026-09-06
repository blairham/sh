// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// zsh reads `|&` the way bash 5.3 does, and not the way ksh93 does. Measured
// on zsh 5.9.2 with the same script bash's own case uses, plus `setopt
// nomultios` — with MULTIOS on, `f >/dev/null 2>&1` tees rather than
// replacing, which hides the ordering question entirely and is a separate
// axis from this one.
//
//	$ zsh five-lines.sh
//	-- plain
//	<O>
//	<E>
//	-- after 2>/dev/null
//	<O>
//	<E>
//	-- after >/dev/null
//	st=0 ps=7 0
//
// Byte for byte what bash 5.3 answers, which is what makes this one flag
// rather than two: the two shells that have this reading agree about it down
// to the ordering of the redirection it stands for.
func TestPipeBothStreamsCarriesStandardErrorIntoThePipe(t *testing.T) {
	const src = `f() { echo O; echo E >&2; }
echo "-- plain"; f |& while read l; do echo "<$l>"; done
echo "-- after 2>/dev/null"; f 2>/dev/null |& while read l; do echo "<$l>"; done
echo "-- after >/dev/null"; f >/dev/null |& while read l; do echo "<$l>"; done
g() { echo E >&2; return 7; }
g |& cat > /dev/null; echo "st=$? ps=${pipestatus[*]}"
`
	const want = `-- plain
<O>
<E>
-- after 2>/dev/null
<O>
<E>
-- after >/dev/null
st=0 ps=7 0
`
	out, st := answersRun(t, src)
	if out != want || st != 0 {
		t.Errorf("got status %d and\n%s\nwant status 0 and\n%s", st, out, want)
	}
}

func TestPipeBothStreamsIsOn(t *testing.T) {
	if !zsh.Dialect().PipeBothStreams {
		t.Error("zsh reads `a |& b` as `a 2>&1 | b`")
	}
}

// The wording half. This shell quotes the token and gives no "unexpected"
// after it, and answers the end of a line with the newline itself.
//
//	$ zsh -n s.sh          # echo one | & echo two
//	s.sh:1: parse error near `&'
//	$ zsh -n s.sh          # `echo one |&' with no newline after it
//	s.sh:1: parse error near `|&'
//	$ zsh -n s.sh          # the same line with one
//	s.sh:2: parse error near `\n'
//
// The last two are one measurement, not two: a trailing newline is a token
// here and is what the shell then names, on the line after the input's last.
// Both are pinned because a fix that always named the operator would pass the
// first and be wrong about the second, which is the file a script actually is.
//
// The operator is quoted whole, which a dialect without it could not say —
// it never lexes one token there.
func TestABarWithNoCommandNamesWhatItFound(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo one | & echo two\n", "parse error near `&'"},
		{"echo one | | echo two\n", "parse error near `|'"},
		{"echo one |&", "parse error near `|&'"},
		{"echo one |", "parse error near `|'"},
		{"echo one |&\n", "parse error near `\\n'"},
		{"echo one |\n", "parse error near `\\n'"},
		{"|& cat\n", "parse error near `|&'"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
