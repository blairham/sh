// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The builtins that keep an assignment written in front of them, measured as
// a **roster** rather than derived from the precommand scan.
//
// `V=1 builtin` keeps the value and `V=1 command` drops it, and both are
// precommand modifiers with no command word behind them — so no reading of
// the scan produces this set. `alias` and `hash`, which are not modifiers at
// all, are the other half of the same argument.
//
// Measured 2026-09-28 against /opt/homebrew/bin/zsh — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m`: *not a Go executable* — over
// every builtin this shell has, each run inside a `{ … }` so that a listing's
// own output is redirected and the command itself carries no redirection
// (#5048).
func TestWhichBuiltinsKeepAnAssignmentPrefix(t *testing.T) {
	for _, tc := range []struct {
		src   string
		keeps bool
	}{
		{`v=1 alias`, true},
		{`v=1 hash`, true},
		{`v=1 builtin`, true},
		{`v=1 exec`, true},
		// The discriminating row: a modifier, no command word behind it,
		// and the value is dropped.
		{`v=1 command`, false},
		{`v=1 :`, false},
		{`v=1 shift 0`, false},
		{`v=1 true`, false},
		{`v=1 export`, false},
		{`v=1 typeset`, false},
		{`v=1 cd .`, false},
		{`v=1 unalias -a`, false},
	} {
		src := "{ " + tc.src + " } >/dev/null 2>&1\nprint \"v=[$v]\""
		out, _ := runZsh(t, t.TempDir(), src)
		want := "v=[]"
		if tc.keeps {
			want = "v=[1]"
		}
		if strings.TrimSpace(out) != want {
			t.Errorf("%s = %q, want %q", tc.src, out, want)
		}
	}
}

// TestTheRosterIsAskedAboutTheCommandThatRuns: behind a transparent modifier
// the first word of the command line is not the command that runs, and it is
// the one that runs that decides.
func TestTheRosterIsAskedAboutTheCommandThatRuns(t *testing.T) {
	for _, tc := range []struct {
		src   string
		keeps bool
	}{
		{`v=1 builtin builtin`, true},
		{`v=1 noglob builtin`, true},
		{`v=1 builtin noglob`, true},
		{`v=1 builtin alias`, true},
		// The first word is on the roster and the command that runs is not.
		{`v=1 builtin echo hi`, false},
		{`v=1 builtin command`, false},
		// And the walk stops where the scan does: the word behind `command`
		// is a command name, so this is `command not found: builtin` and
		// not the builtin the roster names.
		{`v=1 command builtin`, false},
		{`v=1 command echo hi`, false},
	} {
		src := "{ " + tc.src + " } >/dev/null 2>&1\nprint \"v=[$v]\""
		out, _ := runZsh(t, t.TempDir(), src)
		want := "v=[]"
		if tc.keeps {
			want = "v=[1]"
		}
		if strings.TrimSpace(out) != want {
			t.Errorf("%s = %q, want %q", tc.src, out, want)
		}
	}
}

// TestAPrefixSendsACommandlessRedirectionDownTheAssignmentsRoad: the same
// predicate as `redirection with no command`, with the prefix choosing the
// road.
//
// Measured rather than inferred from the refusal's absence. `$_` is what
// separates the two roads: `print MARK; v=1 command >f` leaves `$_` **empty**,
// which is what a bare `v=1` leaves, where the same line with no redirection
// leaves `$_` as `MARK` — the previous command's last argument, untouched,
// because `command` ran and recorded nothing.
func TestAPrefixSendsACommandlessRedirectionDownTheAssignmentsRoad(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=1 builtin >f; print "v=[$v]"`, "v=[1]"},
		{`v=1 command >f; print "v=[$v]"`, "v=[1]"},
		{`v=1 noglob >f; print "v=[$v]"`, "v=[1]"},
		{`v=1 - >f; print "v=[$v]"`, "v=[1]"},
		// The same line with no redirection takes the other road, and the
		// pair is what says the redirection is doing the choosing.
		{`{ v=1 command } >/dev/null; print "v=[$v]"`, "v=[]"},
		// `$_` is the instrument: empty is the bare assignment's own road.
		{`print MARK; v=1 command >f; print "_=[$_]"`, "MARK\n_=[]"},
		// The pair's other half — `print MARK; v=1 command; print "_=[$_]"`
		// leaves `$_` as `MARK` in the reference and as `command` here — is
		// a `$_` row rather than this one, is unchanged by this change, and
		// is not asserted.
		// The control: with no prefix the same line is refused.
		{`NULLCMD=:; builtin >f; print after`, "zsh:1: redirection with no command"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
