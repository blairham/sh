// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The construct or the builtin that raised it goes in front of the empty
// subscript of a write, the way it goes in front of every other math failure
// this shell reports (#3901).
//
// The naming rule was not missing — `(( 1/0 ))` is `((: 1/0 : division by 0 …`
// and `let "1/0"` is `let: 1/0: …`, both byte-identical to bash already. It
// was *not applied here*, because every other math failure travels back up as
// an error and is named at the construct's own site, while this one sentence
// is written where it is raised: the reporting column reports it and lets the
// expression carry on, so there is nothing left for the construct to word.
//
// Measured 2026-09-20, `env -i PATH=/usr/bin:/bin LC_ALL=C bash h.sh` over a
// script file with standard input on /dev/null, bash 5.3.20 at
// `/opt/homebrew`, with `m=(1 2 3)` on the line above each probe.
func TestTheEmptySubscriptOfAWriteIsNamedByWhatRaisedIt(t *testing.T) {
	const setup = "m=(1 2 3)\n"
	for _, tc := range []struct{ name, src, want, why string }{
		{
			"the arithmetic command",
			"(( m[] = 4 ))", "((: `m[]': not a valid identifier",
			"the construct names itself here as it does for `(( 1/0 ))`",
		},
		{
			"let",
			`let "m[] = 4"`, "let: `m[]': not a valid identifier",
			"the builtin names itself here as it does for `let \"1/0\"`",
		},
		{
			"the C-style for header",
			"for (( m[] = 4; 0; )); do :; done", "((: `m[]': not a valid identifier",
			"the header words its parts through the same construct name",
		},
		{
			"a declaration whose value is evaluated",
			`typeset -i q='m[]=4'`, "typeset: `m[]': not a valid identifier",
			"the builtin that is speaking, which is the rule mathFatalf already " +
				"follows for `typeset -i a=1+`",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, setup+tc.src+"\n")
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q — %s", out, tc.want, tc.why)
			}
		})
	}
}

// The controls, and all three of them move under a wrong fix rather than
// merely standing still: each is a route bash leaves bare, and each is bare
// for a different reason.
//
// Measured in the same run.
func TestTheEmptySubscriptOfAWriteIsBareWhereNothingIsSpeaking(t *testing.T) {
	const setup = "m=(1 2 3)\n"
	for _, tc := range []struct{ name, src, why string }{
		{
			"an expansion in an assignment",
			"x=$(( m[] = 4 ))",
			"the expansion route has no construct — bash names one on the command " +
				"routes and not on this one, which is the split " +
				"Diagnostics.ArithErrorNamesTheConstruct already records",
		},
		{
			"an expansion as an argument",
			"echo $(( m[] = 4 ))",
			"and it is the route and not the assignment that decides it",
		},
		{
			"a plain assignment through the integer attribute",
			"declare -i y\ny='m[]=4'",
			"the same evaluator, reached with no builtin speaking — there is no " +
				"name to write, which is why the builtin half is asked through " +
				"Runner.speaking and not through the attribute",
		},
		{
			"an expansion nested inside an arithmetic command",
			"(( z = $(( m[] = 4 )) ))",
			"the inner text is expanded before the construct starts evaluating, " +
				"which is what says the marker is set around the evaluation and " +
				"not around the expansion that produces the text",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, setup+tc.src+"\n")
			if !strings.Contains(out, "`m[]': not a valid identifier") {
				t.Errorf("got %q, want the sentence itself — %s", out, tc.why)
			}
			for _, name := range []string{"((:", "let:", "typeset:", "declare:", "echo:"} {
				if strings.Contains(out, name) {
					t.Errorf("got %q, want no %q in it — %s", out, name, tc.why)
				}
			}
		})
	}
}

// The rows the filer used as the discriminator, kept as a regression: the
// division sentences are byte-identical to bash on both routes and must stay
// that way, because they are what says the naming mechanism was already there
// and only this one sentence was going round it.
func TestTheDivisionSentencesKeepTheirNames(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the arithmetic command", "(( 1/0 ))", `((: 1/0 : division by 0 (error token is "0 ")`},
		{"let", `let "1/0"`, `let: 1/0: division by 0 (error token is "0")`},
		{"an expansion", "x=$(( 1/0 ))", `1/0 : division by 0 (error token is "0 ")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src+"\n")
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

// And the construct wins over a builtin that happens to be running, which is
// the one place the two naming rules could both answer. Measured:
// `eval '(( m[] = 4 ))'` names the construct and never `eval`.
func TestTheConstructOutranksABuiltinItIsRunningUnder(t *testing.T) {
	out, _ := answersRun(t, "m=(1 2 3)\neval '(( m[] = 4 ))'\n")
	if want := "((: `m[]': not a valid identifier"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
	if strings.Contains(out, "eval:") {
		t.Errorf("got %q, want no eval in it", out)
	}
}
