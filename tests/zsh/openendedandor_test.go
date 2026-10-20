// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// This shell alone takes an and-or list that ends with its operator. Measured
// 2026-09-07 on zsh 5.9.2, `-n` and then a run, over a script file under
// `env -i` with a scratch HOME, ZDOTDIR and HISTFILE:
//
//	$ zsh s.sh          # { : || \n }\n echo after
//	after
//	$ bash s.sh
//	s.sh: line 2: syntax error near unexpected token `}'
//	$ dash s.sh
//	s.sh: 2: Syntax error: "}" unexpected
//	$ ksh s.sh
//	s.sh: syntax error at line 2: `}' unexpected
//
// `~/.zi/bin/lib/zsh/install.zsh` line 2048 ends with `|| \` and the line
// after it closes the block, which is the file this was found on (#1174).

// The preset answers the flag, which is the half a parse cannot reach: the
// parser takes the flag from whatever dialect it is handed, so a test that
// only parsed would pass with this shell answering no.
func TestThisShellTakesAnAndOrThatEndsWithItsOperator(t *testing.T) {
	if !zsh.Dialect().OpenEndedAndOr {
		t.Error("zsh takes an and-or list that ends with its operator")
	}
}

// Running them, so the flag is asserted through what the shell does. The
// dangling operator is *dropped*: the status is the left-hand side's, which is
// what separates it from an implicit success — that would make `false ||`
// answer 0 — and from an implicit failure, which would make `true &&`
// non-zero.
func TestADanglingOperatorIsDroppedRatherThanStoodIn(t *testing.T) {
	for _, tc := range []struct {
		src  string
		out  string
		want int
	}{
		{"{ echo one ||\n}\necho after", "one\nafter\n", 0},
		{"{ false ||\n}", "", 1},
		{"{ false &&\n}", "", 1},
		{"{ true &&\n}", "", 0},
		{"{ true ||\n}", "", 0},
		{"( false ||\n)", "", 1},
		{"if true; then false ||\nfi", "", 1},
		{"for i in 1 2; do echo $i &&\ndone", "1\n2\n", 0},
		{"case x in x) false ||\n;; esac", "", 1},
	} {
		out, st, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		if out != tc.out {
			t.Errorf("%q: out = %q, want %q", tc.src, out, tc.out)
		}
		if st != tc.want {
			t.Errorf("%q: status = %d, want %d", tc.src, st, tc.want)
		}
	}
}

// The pipeline does not take it here either, which is the discriminating half:
// a flag that covered every joining operator would accept four lines this
// shell rejects. The whole rendered refusal, because this shell words a parse
// failure with the line inside the sentence rather than in front of it.
//
//	$ zsh -n s.sh       # ( : | )
//	s.sh:1: parse error near `)'
func TestAnOpenEndedPipelineIsRefusedHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"( : | )\n", "s.sh:1: parse error near `)'\n"},
		{"{ : |\n}\n", "s.sh:2: parse error near `}'\n"},
		{"{ : | }\n", "s.sh:1: parse error near `}'\n"},
		{"while false; do : |\ndone\n", "s.sh:2: parse error near `done'\n"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", tc.src)
			continue
		}
		d := zsh.Diagnostics().ForScript()
		if got := d.ParseDiagnostic("s.sh", "", err, tc.src); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// A terminator does not close the list for this purpose, and neither does a
// stop word with nothing open. Both refusals name the token found — the half
// of #1115 the and-or was still getting wrong, which invented "expected a
// command after &&" in every dialect.
//
//	$ zsh -n s.sh       # : &&\n&\n
//	s.sh:2: parse error near `&'
func TestADanglingOperatorStillRefusesWhatThisShellRefuses(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{": &&\n&\n", "s.sh:2: parse error near `&'\n"},
		{": || & :\n", "s.sh:1: parse error near `&'\n"},
		{": &&\n;;\n", "s.sh:2: parse error near `;;'\n"},
		{"echo a && fi\n", "s.sh:1: parse error near `fi'\n"},
		{"echo a || done\n", "s.sh:1: parse error near `done'\n"},
		{": && | :\n", "s.sh:1: parse error near `|'\n"},
		{": && && :\n", "s.sh:1: parse error near `&&'\n"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", tc.src)
			continue
		}
		d := zsh.Diagnostics().ForScript()
		if got := d.ParseDiagnostic("s.sh", "", err, tc.src); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
