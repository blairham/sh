// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// `|&` — a pipe carrying the left side's standard error along with its
// standard output. Measured on bash 5.3.15, from a script file with a scratch
// HOME under `env -i`.
//
// The downstream element brackets what reaches it, so the assertion does not
// depend on which of the two streams a test harness happens to join: what is
// bracketed came through the pipe and what is not came out beside it.
//
//	$ bash five-lines.sh
//	-- plain
//	<O>
//	<E>
//	-- after 2>/dev/null
//	<O>
//	<E>
//	-- after >/dev/null
//	st=0 ps=7 0
//
// The middle block is the one that decides the implementation, and the third
// is its other half: the `2>&1` the operator stands for is the command's
// *last* redirection, not its first. Written first, `2>/dev/null` would win
// and `<E>` would be missing; written first, `>/dev/null` would leave `<E>`
// behind rather than swallowing it. Both blocks agree with appending and
// neither agrees with prepending.
func TestPipeBothStreamsCarriesStandardErrorIntoThePipe(t *testing.T) {
	const src = `f() { echo O; echo E >&2; }
echo "-- plain"; f |& while read l; do echo "<$l>"; done
echo "-- after 2>/dev/null"; f 2>/dev/null |& while read l; do echo "<$l>"; done
echo "-- after >/dev/null"; f >/dev/null |& while read l; do echo "<$l>"; done
g() { echo E >&2; return 7; }
g |& cat > /dev/null; echo "st=$? ps=${PIPESTATUS[*]}"
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

// The preset's answer, so an edit to it cannot flip the construct silently.
// bash 3.2 is the other half of this row and has no `|&` at all — it lexes
// the two bytes as a bar and an ampersand and refuses the line — so the flag
// is a statement about bash 4 and later and not about bash.
func TestPipeBothStreamsIsOn(t *testing.T) {
	if !bash.Dialect().PipeBothStreams {
		t.Error("bash 5.3 reads `a |& b` as `a 2>&1 | b`")
	}
}

// The wording half, which reaches every dialect and is not about `|&`: a bar
// with nothing after it names the token it found rather than the bar behind
// it. Measured, bash 5.3.15 and bash 3.2.57 alike, from a script file:
//
//	$ bash -n s.sh          # echo one | ; echo two
//	s.sh: line 1: syntax error near unexpected token `;'
//	$ bash -n s.sh          # echo one | & echo two
//	s.sh: line 1: syntax error near unexpected token `&'
//	$ bash -n s.sh          # echo one |\n
//	s.sh: line 2: syntax error: unexpected end of file
//
// All of them answered `expected a command after |` before #1115 — a sentence
// no shell in the panel writes, and one that blamed a token the parser had
// already read past.
//
// The last row is the shape that must *not* take this path: input that ran
// out is unfinished rather than wrong, and this shell says so in a different
// sentence and on the line after the input's last.
func TestABarWithNoCommandNamesWhatItFound(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		line      int
	}{
		{"echo one | ; echo two\n", "syntax error near unexpected token `;'", 1},
		{"echo one | & echo two\n", "syntax error near unexpected token `&'", 1},
		{"echo one | | echo two\n", "syntax error near unexpected token `|'", 1},
		{"echo one | && echo two\n", "syntax error near unexpected token `&&'", 1},
		{"echo one |& ; echo two\n", "syntax error near unexpected token `;'", 1},
		{"echo one |\n", "syntax error: unexpected end of file", 2},
		{"echo one |&\n", "syntax error: unexpected end of file", 2},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		d := bash.Diagnostics()
		if got := d.ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
		if got := d.ParseFailureLine(err); got != tc.line {
			t.Errorf("%q: blamed line %d, want %d", tc.src, got, tc.line)
		}
	}
}

// And the operator at the start of a line, where this shell names the whole
// two-byte token because it has one. A dialect without `|&` names the bar
// alone, which is exactly what bash 3.2 does with the same text — the two
// bash columns of a measurement disagree here, and the disagreement is the
// flag.
//
//	$ bash -n s.sh          # |& cat
//	s.sh: line 1: syntax error near unexpected token `|&'
func TestABarBeforeAnyCommandNamesTheWholeOperator(t *testing.T) {
	_, err := syntax.Parse("|& cat\n", bash.Dialect())
	if err == nil {
		t.Fatal("`|& cat` parsed, want a refusal")
	}
	const want = "syntax error near unexpected token `|&'"
	if got := bash.Diagnostics().ParseFailure(err); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
