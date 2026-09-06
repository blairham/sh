// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A branch that never runs never raises the expression it holds.
//
// This is the half a message's *timing* does not cover, and the half that made
// the old behavior a bug rather than a nit: a program that reads an expression
// while reading the file takes itself down for a command it was never going to
// run. Measured unanimous across dash, bash 5.3, bash 3.2, bash-as-sh, ksh93
// and zsh — every one of them prints `reached st=1`.
//
// Run rather than parsed, deliberately. A parse check says the file was read,
// which is the weaker half: an implementation that read the file and then
// raised the expression anyway at the top of the run would pass it.
func TestAnExpressionInABranchThatNeverRunsIsNeverRead(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		why  string
	}{
		{
			name: "an arithmetic command",
			src:  `false && ((echo hi)); echo "reached st=$?"`,
			why:  "the construct the bug report named",
		},
		{
			name: "an arithmetic substitution",
			src:  `false && echo "$((echo hi))"; echo "reached st=$?"`,
			why:  "the substitution form, reached through a word rather than through a command",
		},
		{
			name: "a C-style for header",
			src:  `false && for ((echo hi;;)); do :; done; echo "reached st=$?"`,
			why:  "the header's three parts, which are read where the loop starts",
		},
		{
			name: "an arithmetic command with an operand missing",
			src:  `false && ((1+)); echo "reached st=$?"`,
			why:  "the other way an expression fails",
		},
		{
			name: "a float where the dialect has none",
			src:  `false && echo "$((1.5))"; echo "reached st=$?"`,
			why:  "a grammar flag inside an expression is answered at the run too",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, status := runDeferring(t, c.src)
			if out != "reached st=1\n" || status != 0 {
				t.Errorf("%s\ngave %q at %d, want %q at 0\n%s", c.src, out, status, "reached st=1\n", c.why)
			}
		})
	}
}

// And when the branch does run, the expression is raised then — so the
// deferral moved the complaint rather than losing it.
//
// What is asserted is the *order*, which is the whole of #865: the complaint
// arrives after the command before it has produced its output, so it belongs
// to the run and not to the read. A test that only checked the complaint was
// still there would pass for a program that raised it before anything ran.
func TestTheExpressionIsRaisedWhenTheBranchRuns(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		why             string
	}{
		{
			name: "an arithmetic command",
			src:  `echo before; ((echo hi)); echo "after st=$?"`,
			want: "before\n" +
				"<location>: echo hi: arithmetic syntax error in expression (error token is \"hi\")\n" +
				"after st=1\n",
			why: "the complaint sits between the two echoes, and the script goes on: a failed arithmetic command is one failed command",
		},
		{
			name: "an arithmetic substitution",
			src:  `echo before; echo "$((echo hi))"; echo after`,
			want: "before\n" +
				"<location>: echo hi: arithmetic syntax error in expression (error token is \"hi\")\n",
			why: "the complaint still comes after `before`, and nothing after it runs — a failed arithmetic expansion is fatal, which bash, ksh93 and zsh all agree on and which is the expansion's answer rather than the command's",
		},
		{
			name: "a C-style for header",
			src:  `echo before; for ((echo hi;;)); do :; done; echo "after st=$?"`,
			want: "before\n" +
				"<location>: echo hi: arithmetic syntax error in expression (error token is \"hi\")\n" +
				"after st=1\n",
			why: "the header's parts are read where the loop starts, which is after the line before it has run",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runDeferring(t, c.src)
			if got := maskLocation(out); got != c.want {
				t.Errorf("%s\ngave %q\nwant  %q\n%s", c.src, got, c.want, c.why)
			}
		})
	}
}

// runDeferring runs a snippet with the wording that names the token, which is
// the shape a failure has to be visible in for these cases to say anything.
//
// The diagnostics are built here rather than borrowed from a dialect: this
// package may not import one, and what the case needs is *a* wording that
// carries the expression and the token rather than any shell's in particular.
func runDeferring(t *testing.T, src string) (string, int) {
	t.Helper()
	d := corpusGrammar()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v — the file should read whatever the expression turns out to be", src, err)
	}
	var out strings.Builder
	sem := testSemantics()
	dg := interp.PosixDiagnostics()
	// Three verbs: the expression as written, the reason, the token blamed.
	dg.ArithError = "%[1]s: %[2]s (error token is %[3]q)"
	dg.ArithOperatorExpected = "arithmetic syntax error in expression"
	dg.ArithFailureStatus = 1
	r := newTestRunner(t, &interp.Runner{
		Semantics: &sem, Diagnostics: &dg, Dialect: &d,
		Stdout: &out, Stderr: &out, Stdin: strings.NewReader(""),
		Name: "sh", Dir: t.TempDir(), Env: testPATH(),
	})
	status, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return out.String() + "refused: " + rerr.Error(), -1
	}
	return out.String(), status
}

// maskLocation replaces the location a diagnostic carries, which is the front
// end's and is not what these cases are about.
func maskLocation(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "sh: ") {
			line = "<location>" + line[len("sh"):]
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
