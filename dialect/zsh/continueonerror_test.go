// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runZshSplitOnRoute is runZshSplit told **where the program came from**, which is
// the fact this whole file turns on. A Runner nobody tells is
// RouteUnspecified, which is neither of the three, so the route has to be
// written down rather than left to the zero value.
func runZshSplitOnRoute(t *testing.T, route interp.Route, src string) (out string, status int, errs string) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	dir := t.TempDir()
	r := &interp.Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Vars: map[string]string{"PATH": dir},
		Dialect: presetDialect(), Route: route,
	}
	zsh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), st, e.String()
}

// `continueonerror` makes a fatal error cost the statement rather than the
// shell, and **the route decides whether it applies**.
//
// `A04redirect.ztst` stops on "failed assignment on posix special,
// CONTINUE_ON_ERROR", where a refused assignment prefix must leave the shell
// running. Measured 2026-09-29 on zsh 5.9.2, three fields kept apart because
// the sentence on standard error is byte-identical either way — what carries
// this is the status and whether the next command ran:
//
//	readonly foo; foo=bar set output; print after
//	    plain                     no `after`, 1, `read-only variable: foo`
//	    setopt continueonerror    `after`,    0, the same sentence
//
// The rows below set the option **inside the program**, so its state is
// identical on every one of them and the only thing that moves is the route.
// That matters because `[[ -o continueonerror ]]` is *true* under `-c`: the
// option takes there and does not apply, which is a different thing from the
// option failing to set, and a grid that passed `-o` on the command line
// could not tell those apart.
func TestContinueOnErrorIsDecidedByTheRoute(t *testing.T) {
	const src = "setopt continueonerror\nreadonly foo\nfoo=bar set output\nprint -r -- after\n"
	for _, tc := range []struct {
		name     string
		route    interp.Route
		wantOut  string
		wantStat int
	}{
		{"a script file carries on", interp.RouteScriptFile, "after\n", 0},
		{"standard input carries on", interp.RouteStandardInput, "after\n", 0},
		{"and `-c` does not", interp.RouteCommandString, "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplitOnRoute(t, tc.route, src)
			if out != tc.wantOut || st != tc.wantStat {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.wantOut, tc.wantStat)
			}
			// The third field, and the reason the first two carry the test:
			// the refusal is written on every row, including the ones that
			// carry on. A test that graded the message could not see this
			// rule at all.
			if errs == "" {
				t.Errorf("no diagnostic: the refusal is written whichever way this goes")
			}
		})
	}
}

// Without the option the same program ends the shell on every route, which is
// what keeps the rows above about the option rather than about the route.
func TestWithoutTheOptionEveryRouteEndsTheShell(t *testing.T) {
	const src = "readonly foo\nfoo=bar set output\nprint -r -- after\n"
	for _, route := range []interp.Route{
		interp.RouteScriptFile, interp.RouteStandardInput, interp.RouteCommandString,
	} {
		t.Run(route.String(), func(t *testing.T) {
			out, st, _ := runZshSplitOnRoute(t, route, src)
			if out != "" || st == 0 {
				t.Errorf("out %q status %d, want the shell to end", out, st)
			}
		})
	}
}

// **`posixbuiltins` outranks it, and only for a special builtin.**
//
// This is the pair that says the rule is keyed on the builtin's kind and not
// on the options: both options are on in both rows, the refusal is the same
// refusal, and the only thing that moves is whether the command in front of
// the prefix is special. Measured 2026-09-29 with the script on standard
// input:
//
//	setopt posixbuiltins continueonerror
//	readonly foo; foo=bar set output;  print after    no `after`, 1
//	readonly foo; foo=bar echo output; print after    `after`,    0
//
// A failed redirection on a special builtin is the same rule reached by
// another door — `posixbuiltins` is what makes it fatal at all (#5124) — and
// the switch does not rescue that either. See interp.Runner.fatalPosixSpecialQuiet.
func TestPosixBuiltinsOutranksTheSwitchForASpecialBuiltin(t *testing.T) {
	for _, tc := range []struct {
		name      string
		src       string
		wantEnded bool
	}{
		{
			"a failed redirection on a special builtin is not rescued",
			"setopt posixbuiltins continueonerror\nexec 3< ./no/x\nprint -r -- after\n", true,
		},
		{
			"nor is a `.` that cannot read its file",
			"setopt posixbuiltins continueonerror\n. ./no/x\nprint -r -- after\n", true,
		},
		{
			"while the same redirection without the option is not fatal at all",
			"setopt continueonerror\nexec 3< ./no/x\nprint -r -- after\n", false,
		},
		{
			"and a refused prefix in front of an ordinary builtin is rescued",
			"setopt posixbuiltins continueonerror\nreadonly foo\nfoo=bar print -r -- output\nprint -r -- after\n", false,
		},
		// **Not asserted here: the same prefix in front of a *special*
		// builtin.** `setopt posixbuiltins continueonerror; readonly foo;
		// foo=bar set output` ends the reference and is rescued here, and
		// that row is left diverging rather than pinned wrong. It needs the
		// option to reach the prefix-refusal rule, which is a third
		// mechanism and its own front; residue on #4436. The row above is
		// its control and is the half that already agrees.
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, _ := runZshSplitOnRoute(t, interp.RouteScriptFile, tc.src)
			ended := out == "" && st != 0
			if ended != tc.wantEnded {
				t.Errorf("out %q status %d: ended=%v, want %v", out, st, ended, tc.wantEnded)
			}
		})
	}
}

// The option reports itself, and `unsetopt` puts the shell back — the half
// that keeps it a switch rather than a latch.
func TestTheOptionReportsItselfAndComesBackOff(t *testing.T) {
	out, st, _ := runZshSplitOnRoute(t, interp.RouteScriptFile,
		"if [[ -o continueonerror ]]; then print -r -- on; else print -r -- off; fi\n"+
			"setopt continueonerror\n"+
			"if [[ -o continueonerror ]]; then print -r -- on; else print -r -- off; fi\n"+
			"unsetopt continueonerror\n"+
			"if [[ -o continueonerror ]]; then print -r -- on; else print -r -- off; fi\n")
	if out != "off\non\noff\n" || st != 0 {
		t.Errorf("out %q status %d, want off/on/off at 0", out, st)
	}
	// And taking it off again restores the fatality, which is the behavior
	// behind the report rather than the report itself.
	out, st, _ = runZshSplitOnRoute(t, interp.RouteScriptFile,
		"setopt continueonerror\nunsetopt continueonerror\nreadonly foo\nfoo=bar set output\nprint -r -- after\n")
	if out != "" || st == 0 {
		t.Errorf("out %q status %d, want the shell to end once the option is off", out, st)
	}
}
