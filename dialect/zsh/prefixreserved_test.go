// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// This shell keeps a reserved word's reading behind an assignment prefix
// where the other three drop it, and takes a compound command's redirections
// in front of it as well as after it.
//
// Measured 2026-09-18, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with a scratch HOME and no startup files, against bash 5.3.20, ksh93u+ and
// dash 0.5.12 (#3560).
func TestAReservedWordKeepsItsReadingBehindAnAssignmentPrefixHere(t *testing.T) {
	d := zsh.Dialect()
	if !d.ReservedWordStandsBehindAnAssignmentPrefix {
		t.Error("ReservedWordStandsBehindAnAssignmentPrefix is false, want true")
	}
	for _, c := range []struct{ src, named string }{
		{"v=x { :; }", "{"},
		{"v=x while :; do :; done", "while"},
		{"v=x if :; then :; fi", "if"},
		{"v=x case a in a) :;; esac", "case"},
		{"v=x function f { :; }", "function"},
		{"v=x foreach i (a); :; end", "foreach"},
		// The three that were worse than a wording: `time` ran
		// /usr/bin/time, `!` was a command that could not be found, and the
		// condition reached the pattern matcher.
		{"v=x time :", "time"},
		{"v=x !", "!"},
		{"v=x [[ -n a ]]", "[["},
	} {
		_, err := syntax.Parse(c.src, d.On(syntax.RouteFromScriptFile))
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", c.src)
			continue
		}
		if want := "parse error near `" + c.named + "'"; zsh.Diagnostics().ParseFailure(err) != want {
			t.Errorf("%q: got %q, want %q", c.src, zsh.Diagnostics().ParseFailure(err), want)
		}
	}
	// The controls: `in` is an ordinary command name where a command begins,
	// and a `(` is an operator no reading was ever taken from.
	if _, err := syntax.Parse("v=x in", d); err != nil {
		t.Errorf("`v=x in`: %v, want a command called in", err)
	}
	if _, err := syntax.Parse("v=x ( : )", d); err == nil {
		t.Error("`v=x ( : )` parsed, want the refusal every column gives")
	}
}

// And a redirection in front of a compound command is ordinary zsh, which
// this shell refused for every compound it has.
func TestARedirectionMayPrecedeACompoundHere(t *testing.T) {
	d := zsh.Dialect()
	if got, want := d.RedirectionBeforeACompound, syntax.RedirectionMayPrecedeAnyCompoundCommand; got != want {
		t.Errorf("RedirectionBeforeACompound = %v, want %v", got, want)
	}
	for _, src := range []string{
		">/dev/null { echo hi; }",
		">/dev/null while false; do :; done",
		">/dev/null if true; then echo hi; fi",
		">/dev/null for i in a; do echo hi; done",
		">/dev/null case a in a) echo hi;; esac",
		">/dev/null until true; do :; done",
		">/dev/null select i in a; do break; done",
		">/dev/null repeat 2 do echo hi; done",
		">/dev/null foreach i (a); echo hi; end",
		">/dev/null ( echo hi )",
		">/dev/null (( 1 ))",
	} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err != nil {
			t.Errorf("parse %q: %v", src, err)
		}
	}
	// A reserved word that is not a compound command has nowhere to stand
	// there, and the assignment prefix takes the whole reading back.
	for _, src := range []string{
		">/dev/null ! false", ">/dev/null coproc cat", ">/dev/null then",
		"v=x >/dev/null ( echo hi )", ">/dev/null v=x { echo hi; }",
	} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err == nil {
			t.Errorf("%q parsed, want it refused", src)
		}
	}
}
