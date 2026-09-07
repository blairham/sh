// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// The try-always block, `{ … } always { … }`, end to end under this dialect
// (#1216).
//
// Nine files in a real `~/.zi` plugin tree were unparseable without it, and
// they are the plugins a zsh daily driver actually loads: zsh-autosuggestions
// (`zsh-autosuggestions.zsh:628` and `src/strategies/completion.zsh:133`),
// powerlevel10k's `gitstatus.plugin.zsh:463`, `internal/worker.zsh:74`,
// `internal/wizard.zsh:363` and `internal/configure.zsh:66`, F-Sy-H's
// `F-Sy-H.plugin.zsh:148`, and zi's own `lib/zsh/autoload.zsh:1423`.
//
// Measured against zsh 5.9.2 on 2026-09-07.
func TestTheTryAlwaysBlockRuns(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The three spellings the issue measured, all of which failed
		// identically before — which is what said the keyword was the cause
		// rather than a separator detail.
		{"f(){ { echo t; } always { echo a; }; }; f", "t\na"},
		{"{ echo t; } always { echo a; }", "t\na"},
		{"{ echo t } always { echo a }", "t\na"},
		// The status is the first half's, both ways round.
		{`{ false; } always { true; }; echo "?=$?"`, "?=1"},
		{`{ true; } always { false; }; echo "?=$?"`, "?=0"},
		// `$?` inside the second half is the first half's. This is the shape
		// gitstatus.plugin.zsh opens its cleanup half with.
		{`{ (exit 5); } always { local -i ret=$?; echo "ret=$ret"; }`, "ret=5"},
		// A `return` out of the first half runs the second and still returns
		// — the shape F-Sy-H's plugin is written in.
		{`f(){ { echo t; return 3; } always { echo A; }; echo NOT-REACHED; }; f; echo "?=$?"`, "t\nA\n?=3"},
		// An `exit` runs the cleanup halves it unwinds through and skips the
		// ones at the shell's own top level.
		{"f(){ { echo t; exit 7; } always { echo A; }; }; f; echo NOT-REACHED", "t\nA"},
		{"{ echo t; exit 7; } always { echo A; }; echo NOT-REACHED", "t"},
		// break and continue reach it.
		{
			"for i in 1 2 3; do { echo t$i; [[ $i == 2 ]] && break; } always { echo A$i; }; done; echo after",
			"t1\nA1\nt2\nA2\nafter",
		},
		// Nesting, and the redirection that belongs to the whole construct.
		{"{ { echo t; } always { echo a; }; } always { echo b; }", "t\na\nb"},
		{"{ echo t; } always { echo a; } > /dev/null; echo done", "done"},
		// Either half may be empty here, which is this dialect's empty-body
		// rule reaching the construct rather than anything the construct says.
		{"{ } always { echo a; }", "a"},
		{"{ :; } always { }; echo ok", "ok"},
		// It closes itself, so a short body may follow it.
		{"if { true; } always { :; } { echo A; }; echo after", "A\nafter"},
		// A reparse under this dialect keeps the construct, which is what says
		// the runner was told which shell it is.
		{`eval '{ echo t; } always { echo a; }'`, "t\na"},
		// The keyword is an ordinary word away from a closed brace.
		{"always() { echo fn; }; always", "fn"},
		{"echo always", "always"},
		{"x=always; echo $x", "always"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}

// An error this shell reports and gives up over runs the cleanup half and is
// then re-raised behind it, and one raised *by* the cleanup half is cleared.
//
// A readonly reassignment is fatal in this dialect, which is what makes the
// rows reachable: the cleanup runs, `after-f` never does, and the shell is
// left at 1. Separated from the table above because these rows assert the
// dialect's own wording alongside the control flow.
func TestAnErrorConditionRunsTheTryAlwaysCleanup(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			"f(){ { readonly r=1; r=2; } always { echo A; }; echo NOT-REACHED; }; f",
			"f: read-only variable: r\nA",
		},
		{
			"f(){ { echo t; } always { readonly r=1; r=2; echo NOT-REACHED; }; echo after-f; }; f",
			"t\nf: read-only variable: r\nafter-f",
		},
		{
			"f(){ { echo $(( 1/0 )); } always { echo A; }; echo NOT-REACHED; }; f",
			"f: division by zero\nA",
		},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
	// `${x?word}` is the one this shell documents as exiting outright, and it
	// skips the cleanup half even inside a function — the off-diagonal against
	// the `exit` row above, which does run one.
	out, _ := answersRun(t, "f(){ { echo t; : ${nosuch_zz?boom}; } always { echo A; }; }; f; echo NOT-REACHED")
	if got := strings.TrimSpace(out); got != "t\nf: nosuch_zz: boom" {
		t.Errorf("said %q, want the complaint and no cleanup", got)
	}
}

// The shapes this shell refuses, which is what makes the keyword positional
// rather than reserved.
func TestTheTryAlwaysBlockHasABoundary(t *testing.T) {
	for _, src := range []string{
		// A separator, of either kind, takes the keyword away.
		"{ echo t; }\nalways { echo a; }\n",
		// Quoting it does too.
		"{ echo t; } \"always\" { echo a; }\n",
		"{ echo t; } \\always { echo a; }\n",
		// The second half has to be a brace group, and there is one of them.
		"{ echo t; } always echo a\n",
		"{ echo t; } always { echo a; } always { echo b; }\n",
		// A redirection between the halves ends the first one.
		"{ echo t; } > /dev/null always { echo a; }\n",
		// No other compound command takes it.
		"if true; then echo t; fi always { echo a; }\n",
		"( echo t ) always { echo a; }\n",
		"for i in a; do echo $i; done always { echo a; }\n",
		"for i in a; { echo $i; } always { echo a; }\n",
		// Nor a function definition's body, named or nameless.
		"f() { echo t; } always { echo a; }\n",
		"() { echo t; } always { echo a; }\n",
	} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err == nil {
			t.Errorf("%q parsed, want a syntax error", src)
		}
	}
	// While `always` with a separator before it is two commands rather than a
	// parse failure: the brace group runs and the word is a missing command.
	out, st := answersRun(t, "{ echo t; }; always")
	if got := strings.TrimSpace(out); !strings.HasPrefix(got, "t\n") ||
		!strings.Contains(got, "command not found: always") || st != 127 {
		t.Errorf("said %q status %d, want t and a command-not-found at 127", got, st)
	}
}
