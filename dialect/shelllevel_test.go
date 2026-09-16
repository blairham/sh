// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// What each dialect does with `$SHLVL`, which is the depth of shells the
// process is standing in.
//
// One table rather than a case in each package, because the split is what the
// table is: four dialects count and one does not, and the one that does not is
// dash alone. A per-dialect assertion states four values and never states
// that — and the column this issue was filed without is exactly the one a
// per-dialect file would have left out. BusyBox ash has `SHLVL`, exports it,
// and increments it, so `cmd/ash` belongs with bash and not with dash (#3097).
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> -c …`:
//
//	                                       ${SHLVL-NONE}   in a child's environment
//	bash 5.3.20, bash-as-sh, bash 3.2.57   1               yes
//	zsh 5.9.2                              1               yes
//	ksh93u+ 2012-08-01                     1               yes
//	BusyBox ash 1.37.0 (pinned alpine)     1               yes
//	dash 0.5.12                            NONE            no
func shellLevelPresets() []struct {
	dialecttest.Preset
	policy interp.ShellLevelPolicy
} {
	return []struct {
		dialecttest.Preset
		policy interp.ShellLevelPolicy
	}{
		{dialecttest.Preset{
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		}, interp.ShellLevelCountedToACeiling},
		{dialecttest.Preset{
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		}, interp.ShellLevelCounted},
		{dialecttest.Preset{
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		}, interp.ShellLevelCounted},
		{dialecttest.Preset{
			Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
			Diagnostics: ash.Diagnostics, Apply: ash.Apply,
		}, interp.ShellLevelCounted},
		{dialecttest.Preset{
			Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
			Diagnostics: dash.Diagnostics, Apply: dash.Apply,
		}, interp.ShellLevelNotCounted},
	}
}

// The vector each dialect answers with, stated once so that a dialect that
// stops answering is a failure here rather than a silence in `make
// axis-coverage`.
func TestEachDialectAnswersTheShellLevelAxis(t *testing.T) {
	for _, p := range shellLevelPresets() {
		t.Run(p.Name, func(t *testing.T) {
			if got := p.Semantics().ShellLevel; got != p.policy {
				t.Errorf("ShellLevel = %v, want %v", got, p.policy)
			}
		})
	}
}

// The count itself, read back through a shell that was handed a depth.
//
// `${SHLVL-NONE}` rather than `$SHLVL`, because the whole of dash's answer is
// that the name is not there: a bare expansion writes an empty line for "unset"
// and for "set to nothing" alike, and those are two different bugs.
func TestTheShellLevelCountsThisShellIn(t *testing.T) {
	for _, p := range shellLevelPresets() {
		for _, c := range []struct {
			name, inherited, want string
		}{
			{"nothing inherited", "", "1"},
			{"a depth inherited", "3", "4"},
			{"a value that names no depth", "abc", "1"},
			{"the empty value", "=", "1"},
			{"zero", "0", "1"},
		} {
			t.Run(p.Name+"/"+c.name, func(t *testing.T) {
				var env []string
				switch c.inherited {
				case "":
				case "=":
					env = []string{"SHLVL="}
				default:
					env = []string{"SHLVL=" + c.inherited}
				}
				out, st, err := p.Combined(t, dialecttest.Base{Env: env},
					`echo "[${SHLVL-NONE}]"`)
				if err != nil || st != 0 {
					t.Fatalf("status %d, err %v: %s", st, err, out)
				}
				want := c.want
				if p.policy == interp.ShellLevelNotCounted {
					// dash reads the name as any other inherited name and
					// writes nothing of its own, so the empty value is
					// *set and empty* rather than absent.
					switch c.inherited {
					case "":
						want = "NONE"
					case "=":
						want = ""
					default:
						want = c.inherited
					}
				}
				if got := strings.TrimSpace(out); got != "["+want+"]" {
					t.Errorf("SHLVL read back as %s, want [%s]", got, want)
				}
			})
		}
	}
}

// The half that is not about this shell: every child is told the depth.
//
// This is the symptom #3097 was filed for. A shell that counts and does not
// export has a number it can print and no child ever hears, so a real bash
// started underneath it reads 1 instead of 2 — and a prompt that marks a
// nested shell, a `.profile` guard that only runs at the top level and a
// script that refuses to recurse all take the wrong branch at status 0.
//
// Asked through `export -p`, which is the shell's own record of what a child
// would be told, rather than by starting one: a test that started a shell
// under `go test` would be starting the test binary.
//
// **Both rows are load-bearing and the first is the one that discriminates.**
// A name the shell was handed is already exported, because arriving in the
// environment is what carrying the attribute means — so a shell handed
// `SHLVL=4` lists the incremented value whether or not anything recorded the
// attribute, and a test with only that row passes with the export dropped.
// The shell nobody started from a shell is the one that has to record it.
func TestTheShellLevelIsInEveryChildsEnvironment(t *testing.T) {
	for _, c := range []struct {
		name, inherited, level string
	}{
		{"a shell nobody started from a shell", "", "1"},
		{"and one that was handed a depth", "4", "5"},
	} {
		for _, p := range shellLevelPresets() {
			t.Run(c.name+"/"+p.Name, func(t *testing.T) {
				var env []string
				if c.inherited != "" {
					env = []string{"SHLVL=" + c.inherited}
				}
				// The spellings the panel's `export -p` uses for one entry,
				// written out rather than matched loosely: `*SHLVL=*5*` would
				// be satisfied by any later line of the listing holding a 5.
				out, st, err := p.Combined(t, dialecttest.Base{Env: env},
					"case \"$(export -p)\" in\n"+
						"*SHLVL="+c.level+"*) echo told ;;\n"+
						"*\"SHLVL='"+c.level+"'\"*) echo told ;;\n"+
						"*'SHLVL=\""+c.level+"\"'*) echo told ;;\n"+
						"*) echo silent ;;\n"+
						"esac")
				if err != nil || st != 0 {
					t.Fatalf("status %d, err %v: %s", st, err, out)
				}
				want := "told"
				if p.policy == interp.ShellLevelNotCounted {
					// dash exports nothing of its own. An inherited entry is
					// still carried on untouched, which is why this asks
					// about the incremented value and not about the name.
					want = "silent"
				}
				if got := strings.TrimSpace(out); got != want {
					t.Errorf("a child would be %s about the depth, want %s", got, want)
				}
			})
		}
	}
}

// A subshell is the same shell, and the count says so.
//
// Measured on bash 5.3.20, zsh 5.9.2, ksh93u+ and BusyBox ash in one run: a
// `( )` subshell, a command substitution, a brace group and a pipeline element
// all read the level the shell itself has. Only starting another shell adds
// one — which is what makes the number a depth of shell *processes* rather
// than a nesting count of anything else, and is the reason the count is
// settled once per session instead of per chunk.
func TestASubshellIsNotADeeperShell(t *testing.T) {
	for _, p := range shellLevelPresets() {
		if p.policy == interp.ShellLevelNotCounted {
			continue
		}
		t.Run(p.Name, func(t *testing.T) {
			out, st, err := p.Combined(t, dialecttest.Base{Env: []string{"SHLVL=2"}},
				`( echo "parens [$SHLVL]" )`+"\n"+
					`echo "cmdsub [$(echo "$SHLVL")]"`+"\n"+
					`{ echo "brace [$SHLVL]"; }`+"\n"+
					`echo x | { echo "pipe [$SHLVL]"; }`)
			if err != nil || st != 0 {
				t.Fatalf("status %d, err %v: %s", st, err, out)
			}
			want := "parens [3]\ncmdsub [3]\nbrace [3]\npipe [3]"
			if got := strings.TrimSpace(out); got != want {
				t.Errorf("read back\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// bash's ceiling, which is the one behavior that separates the four counting
// columns from each other.
//
// Measured 2026-09-16. bash 5.3.20 takes an inherited 998 to 999 quietly and
// an inherited 999 to `warning: shell level (1000) too high, resetting to 1`
// on standard error, at status 0; zsh 5.9.2, ksh93u+ and BusyBox ash write
// 1000 and say nothing. A level that would go negative is floored at 0 in bash
// and is not in the other three — `SHLVL=-5` is 0 in bash and -4 in zsh and
// ksh93 — and the `-1` row is the control both readings share.
func TestOnlyBashRefusesADepthThatHasRunAway(t *testing.T) {
	for _, p := range shellLevelPresets() {
		if p.policy == interp.ShellLevelNotCounted {
			continue
		}
		for _, c := range []struct {
			name, inherited, capped, uncapped string
			// warned is the level named in the refusal, which is the one
			// the shell would have had rather than the one it settles for.
			warned string
			warns  bool
		}{
			{name: "just below the ceiling", inherited: "998", capped: "999", uncapped: "999"},
			{name: "at the ceiling", inherited: "999", capped: "1", uncapped: "1000", warned: "1000", warns: true},
			{name: "past it", inherited: "9999", capped: "1", uncapped: "10000", warned: "10000", warns: true},
			{name: "one below zero", inherited: "-1", capped: "0", uncapped: "0"},
			{name: "further below", inherited: "-5", capped: "0", uncapped: "-4"},
		} {
			t.Run(p.Name+"/"+c.name, func(t *testing.T) {
				out, st, err := p.Combined(t,
					dialecttest.Base{Env: []string{"SHLVL=" + c.inherited}},
					`echo "[$SHLVL]"`)
				if err != nil || st != 0 {
					t.Fatalf("status %d, err %v: %s", st, err, out)
				}
				want, warns := c.uncapped, false
				if p.policy == interp.ShellLevelCountedToACeiling {
					want, warns = c.capped, c.warns
				}
				// The whole of what the shell wrote, warning included and in
				// the order bash writes it: the refusal goes out before the
				// script's first line, and at status 0 — which is what makes
				// it invisible to `set -e` and to `||` alike.
				expect := "[" + want + "]"
				if warns {
					expect = p.Name + ": warning: shell level (" +
						c.warned + ") too high, resetting to 1\n" + expect
				}
				if got := strings.TrimSpace(out); got != expect {
					t.Errorf("wrote %q, want %q", got, expect)
				}
			})
		}
	}
}
