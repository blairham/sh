// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// withDash is a runner with the `-` modifier and the rest of the family a
// dialect that has one registers beside it. The names are the core's own
// table rather than any dialect's roster; see dialect/zsh for the measured
// words.
func withDash(r *Runner) {
	// The letter that asks for the same dash the modifier does, so the two
	// can be held against each other. Its composition with `-a` is asked at
	// the answer the shell with both gives — see
	// Semantics.ExecLoginPrefixesTheGivenName.
	r.Semantics.ExecTakesTheLoginLetter = Yes
	r.Semantics.ExecLoginPrefixesTheGivenName = No
	r.SetPrecommand("-", PrecommandDash)
	r.SetPrecommand("noglob", PrecommandNoGlob)
	r.SetPrecommand("builtin", PrecommandTransparent)
	r.SetPrecommand("exec", PrecommandTransparent)
}

// A command word that is exactly `-` is a **precommand modifier**: the
// command runs with a dash on the front of its argv[0].
//
// Every row here reports the name the command was started under, which is the
// one thing the grid this replaced never varied. It was read as a word to
// throw away, and `echo hi` prints `hi` under either reading — so seven rows
// keyed on *how the dash arrives* all agreed with the discard for a reason
// that had nothing to do with the discard being right (#5018).
func TestALoneDashPutsADashOnArgv0(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the control, which is what makes the rest readable",
			`sh -c 'echo "[$0]"'`, "[sh]",
		},
		{
			"the discriminating row",
			`- sh -c 'echo "[$0]"'`, "[-sh]",
		},
		{
			"the word as written and not the path it resolved to",
			`- /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]",
		},
		{
			"one dash however many were written",
			`- - - sh -c 'echo "[$0]"'`, "[-sh]",
		},
		{
			"a modifier may stand in front of it",
			`noglob - sh -c 'echo "[$0]"'`, "[-sh]",
		},
		{
			"and behind it, which a discard could not reach",
			`- noglob sh -c 'echo "[$0]"'`, "[-sh]",
		},
		{
			"through exec, the builtin that stands in front of a command",
			`- exec sh -c 'echo "[$0]"'`, "[-sh]",
		},
		{
			"and read behind exec, where it is not an option word",
			`exec - sh -c 'echo "[$0]"'`, "[-sh]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, withDash)
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s: argv[0] = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestALoneDashStopsTheOptionScanBehindIt: the second half of the modifier,
// and a fact of its own rather than a consequence of the first.
//
// A dash-word behind it becomes the **command name** — `command not found:
// -l` and not a usage error, which is what says the scan did not run at all
// rather than running and refusing. The controls are what make this one rule
// and not four: each letter reads perfectly well on its own, and the same
// letter behind a *different* modifier still reads (#5028).
func TestALoneDashStopsTheOptionScanBehindIt(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		refused         bool
	}{
		{"exec reads its login letter", `exec -l sh -c 'echo "[$0]"'`, "[-sh]", false},
		{"exec reads its name letter", `exec -a zz sh -c 'echo "[$0]"'`, "[zz]", false},
		{"and behind another modifier it still reads it", `noglob exec -l sh -c 'echo "[$0]"'`, "[-sh]", false},
		{"behind the dash it does not", `- exec -l sh -c 'echo "[$0]"'`, "-l", true},
		{"nor the name letter", `- exec -a zz sh -c 'echo "[$0]"'`, "-a", true},
		{"nor the separator", `- exec -- sh -c 'echo "[$0]"'`, "--", true},
		{"command's own letter goes the same way", `- command -p sh -c 'echo "[$0]"'`, "-p", true},
		{"wherever in the scan the dash stood", `noglob - exec -l sh -c 'echo "[$0]"'`, "-l", true},
		{"and however far behind it the modifier is", `- noglob exec -l sh -c 'echo "[$0]"'`, "-l", true},
		{
			// The discriminating pair for the noun, and the reason the rule
			// is not "a dash was asked for": `exec -l` asks for exactly the
			// dash the modifier asks for and the option reading behind it
			// carries on, so the `-a` is still read…
			"the letter that asks for the same dash does not stop it",
			`exec -l -a zz sh -c 'echo "[$0]"'`, "[zz]", false,
		},
		{
			// …and the identical letters behind a `-` are not read at all.
			// Hold the dash fixed, vary only which spelling arrived, and the
			// answer moves — so the noun is the word.
			"the same two letters behind the word are not read",
			`- exec -l -a zz sh -c 'echo "[$0]"'`, "-l", true,
		},
		{
			// And it reaches the modifiers alone: an ordinary builtin
			// behind the dash reads its own options as it always did.
			"an ordinary builtin still reads its options",
			`- echo -n hi; echo "|"`, "hi|", false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, withDash)
			if tc.refused {
				if st != 127 {
					t.Fatalf("status %d, want 127: %s", st, out)
				}
				if !strings.Contains(out, tc.want) {
					t.Errorf("%s = %q, want a refusal naming %q", tc.src, out, tc.want)
				}
				return
			}
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestALoneDashIsAModifierAndNotAName: the rows the discard was measured from,
// which all stay. They are the controls that say this is a modifier rather
// than a license to stop reading the word at all, and each is run twice —
// with the table and without it — because "the word is read off the front" and
// "the word is an ordinary command name" are the two answers the panel has.
func TestALoneDashIsAModifierAndNotAName(t *testing.T) {
	for _, tc := range []struct{ name, src, with, without string }{
		{
			"the word is read off the front and the rest runs",
			`- echo hi; echo "st=$?"`, "hi\nst=0\n", "st=127\n",
		},
		{
			"as many of them as are written",
			`- - - echo hi; echo "st=$?"`, "hi\nst=0\n", "st=127\n",
		},
		{
			"quoting does not protect it",
			`'-' echo hi; echo "st=$?"`, "hi\nst=0\n", "st=127\n",
		},
		{
			"nor does arriving through an expansion",
			`v=-; $v echo hi; echo "st=$?"`, "hi\nst=0\n", "st=127\n",
		},
		{
			"the status is the command's own",
			`- false; echo "st=$?"`, "st=1\n", "st=127\n",
		},
		{
			"the word after it is a command word, not a prefix",
			`- v=1 echo hi; echo "st=$?"`, "st=127\n", "st=127\n",
		},
		{
			"two dashes is an ordinary name",
			`-- echo hi; echo "st=$?"`, "st=127\n", "st=127\n",
		},
		{
			"nothing left is nothing run",
			`-; echo "st=$?"`, "st=0\n", "st=127\n",
		},
		{
			"and the assignments in front of it still persist",
			`v=1 -; echo "st=$? v=[$v]"`, "st=0 v=[1]\n", "st=127 v=[]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, withDash)
			if !strings.HasSuffix(out, tc.with) {
				t.Errorf("with the table: %s = %q, want it to end %q", tc.src, out, tc.with)
			}
			out, _ = run(t, tc.src, nil)
			if !strings.HasSuffix(out, tc.without) {
				t.Errorf("with no table: %s = %q, want it to end %q", tc.src, out, tc.without)
			}
		})
	}
}

// TestAModifierTakenAwayIsNotTheSameStateAsNoWordAtAll: a redirection with
// nothing left in front of it.
//
// `>f` written with no word at all opens its files, runs nothing and
// succeeds; `- >f` and `noglob >f` are `redirection with no command` and end
// the script. So the question is whether the scan **took a word**, and the
// control is what tells the two apart — a status alone cannot.
func TestAModifierTakenAwayIsNotTheSameStateAsNoWordAtAll(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		{"the dash", `- >f; echo after`, true},
		{"and any other taken modifier", `noglob >f; echo after`, true},
		{"the control: no word at all", `>f; echo after`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, withDash)
			if tc.refused {
				if st == 0 || strings.Contains(out, "after") {
					t.Errorf("%s = %q (status %d), want a refusal that ends the script", tc.src, out, st)
				}
				return
			}
			if st != 0 || !strings.Contains(out, "after") {
				t.Errorf("%s = %q (status %d), want the script to carry on", tc.src, out, st)
			}
		})
	}
}
