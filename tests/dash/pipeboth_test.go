// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// dash has no `|&`, so it reads those two bytes as a bar and an ampersand and
// blames the ampersand — the same sentence it gives for `a | & b`, which is
// the same two tokens with a blank between them.
//
// Measured, dash from a script file under `env -i`, five shapes rather than
// one because #1083 found this shell's operator slot standing wherever a
// pattern belongs and our refusals wrong in five of twenty shapes for it:
//
//	$ dash -n s.sh
//	s.sh: 1: Syntax error: "&" unexpected
//
// The substrate used to answer all five with `expected a command after |`,
// which is a sentence no shell in the panel writes and which blamed the wrong
// token besides: the bar is read and was never the problem (#1115).
func TestNoPipeBothStreams(t *testing.T) {
	if dash.Dialect().PipeBothStreams {
		t.Error("dash has no `|&`")
	}
	for _, src := range []string{
		"echo one |& echo two\n",
		"{ echo one; } |& cat\n",
		"echo a |& cat |& cat\n",
		"for i in 1; do echo $i |& cat; done\n",
		"echo one |&\n",
		"echo one | & echo two\n",
	} {
		_, err := syntax.Parse(src, dash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", src)
			continue
		}
		const want = `Syntax error: "&" unexpected`
		if got := dash.Diagnostics().ParseFailure(err); got != want {
			t.Errorf("%q:\n got %q\nwant %q", src, got, want)
		}
	}
}

// A bar at the *start* is the one shape where this shell names the bar, and
// it does so because the bar is the first thing it could not take. The two
// answers are the same rule seen from either side, so both are pinned: a fix
// that always blamed the ampersand would pass the case above and fail here.
//
//	$ dash -n s.sh
//	s.sh: 1: Syntax error: "|" unexpected
func TestABarBeforeAnyCommandNamesTheBar(t *testing.T) {
	_, err := syntax.Parse("|& cat\n", dash.Dialect())
	if err == nil {
		t.Fatal("`|& cat` parsed, want a refusal")
	}
	const want = `Syntax error: "|" unexpected`
	if got := dash.Diagnostics().ParseFailure(err); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// And the tokens that are not an ampersand, which reach the same path. All
// four were `expected a command after |` before #1115.
//
//	$ dash -n s.sh          # echo one | ; echo two
//	s.sh: 1: Syntax error: ";" unexpected
//	$ dash -n s.sh          # echo one |\n
//	s.sh: 2: Syntax error: end of file unexpected
func TestABarWithNoCommandNamesWhatItFound(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo one | ; echo two\n", `Syntax error: ";" unexpected`},
		{"echo one | | echo two\n", `Syntax error: "|" unexpected`},
		{"echo one | && echo two\n", `Syntax error: "&&" unexpected`},
		{"echo one |\n", `Syntax error: end of file unexpected`},
		// A reserved word after a bar is *reserved*, because a command may
		// begin there — so this shell quotes it rather than naming its
		// class. It is the one dialect that words the two apart, which is
		// what makes it the one that can tell the mistake:
		//
		//	$ dash -n s.sh          # echo a | fi
		//	s.sh: 1: Syntax error: "fi" unexpected
		//
		// Not `Syntax error: word unexpected`, which is what asking for the
		// operand form here gives — the form for a place no command may
		// begin, which is not this one.
		{"echo a | fi\n", `Syntax error: "fi" unexpected`},
		{"echo a | done\n", `Syntax error: "done" unexpected`},
		{"echo a | esac\n", `Syntax error: "esac" unexpected`},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := dash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
