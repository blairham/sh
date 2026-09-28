// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"strings"
	"testing"
)

// What every produced parameter in this dialect says about *itself* — #4812.
//
// `${(t)aliases}` was `association-special` where the reference says
// `association-hide-hideval-special`, and so was every other table this shell
// produces without marking: #4762 gave `$dirstack` the two hiding letters
// through `MarkHidden` and `MarkHideInScope` and the alias, function, option,
// command and named-directory tables were left with the `special` mark alone.
// The letters are a description of the name rather than of its value, so
// nothing a script computes moved; what read them wrong is a framework
// branching on the type word.
//
// Measured 2026-09-27 on `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, run `-f` under `env -i PATH=/usr/bin:/bin`
// with `zmodload zsh/parameter zsh/mapfile zsh/system` so the module
// parameters are live; `go version -m` says *not a Go executable* for it and
// `github.com/blairham/sh/cmd/zsh` for ours. Every name this shell has is a
// row, and the whole population was read in one run rather than the two the
// issue named:
//
//	association-hide-hideval-special            commands, options, aliases,
//	                                            galiases, saliases, functions,
//	                                            dis_aliases, dis_galiases,
//	                                            dis_saliases, dis_functions,
//	                                            nameddirs, mapfile
//	association-readonly-hide-hideval-special   parameters, builtins, history,
//	                                            dis_functions_source, jobdirs,
//	                                            jobtexts, jobstates, sysparams
//	array-hide-hideval-special                  dirstack
//	array-readonly-hide-hideval-special         reswords, dis_reswords,
//	                                            dis_patchars, funcstack,
//	                                            functrace, funcsourcetrace,
//	                                            funcfiletrace, errnos
//
// **The readonly letter is part of the same row**, which is what the sweep
// added to the issue's list: `funcstack` was the one produced view registered
// with neither the freeze nor the letters, so `funcstack=(a b)` and `unset
// funcstack` were both taken here where the reference answers `read-only
// variable: funcstack`. See TestAProducedViewRefusesAWriteAndAnUnset.
//
// The rows that are *not* here are the six the reference has and this shell
// answers `parameter not implemented yet` for — `dis_builtins`, `patchars`,
// `modules`, `userdirs`, `usergroups` and `historywords`. A row asserting the
// empty string for one of those would be a row about a missing feature
// wearing this question's clothes.
func TestEveryProducedParameterCarriesTheHidingLetters(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, want string }{
		{"commands", "association-hide-hideval-special"},
		{"options", "association-hide-hideval-special"},
		{"aliases", "association-hide-hideval-special"},
		{"galiases", "association-hide-hideval-special"},
		{"saliases", "association-hide-hideval-special"},
		{"functions", "association-hide-hideval-special"},
		{"dis_aliases", "association-hide-hideval-special"},
		{"dis_galiases", "association-hide-hideval-special"},
		{"dis_saliases", "association-hide-hideval-special"},
		{"dis_functions", "association-hide-hideval-special"},
		{"nameddirs", "association-hide-hideval-special"},
		{"mapfile", "association-hide-hideval-special"},
		{"parameters", "association-readonly-hide-hideval-special"},
		{"builtins", "association-readonly-hide-hideval-special"},
		{"history", "association-readonly-hide-hideval-special"},
		{"dis_functions_source", "association-readonly-hide-hideval-special"},
		{"jobdirs", "association-readonly-hide-hideval-special"},
		{"jobtexts", "association-readonly-hide-hideval-special"},
		{"jobstates", "association-readonly-hide-hideval-special"},
		{"sysparams", "association-readonly-hide-hideval-special"},
		{"dirstack", "array-hide-hideval-special"},
		{"reswords", "array-readonly-hide-hideval-special"},
		{"dis_reswords", "array-readonly-hide-hideval-special"},
		{"dis_patchars", "array-readonly-hide-hideval-special"},
		{"funcstack", "array-readonly-hide-hideval-special"},
		{"functrace", "array-readonly-hide-hideval-special"},
		{"funcsourcetrace", "array-readonly-hide-hideval-special"},
		{"funcfiletrace", "array-readonly-hide-hideval-special"},
		{"errnos", "array-readonly-hide-hideval-special"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := gatedModuleOf(tc.name) + `print -r -- "${(t)` + tc.name + `}"`
			out, st := runZsh(t, dir, src)
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("${(t)%s} = %q (status %d), want %q", tc.name, out, st, tc.want+"\n")
			}
		})
	}
}

// The control that keeps the rows above from being "put the letters on
// everything": a name a script declares carries none of them, and one given
// only `-H` carries `hideval` without `hide`.
//
// Measured in the same run on zsh 5.9.2. Without this every row above would
// still pass in a shell that described every parameter as the shell's own.
func TestAnOrdinaryNameCarriesNeitherHidingLetter(t *testing.T) {
	const src = `v=1
typeset -H h=2
print -r -- "v=[${(t)v}] h=[${(t)h}]"`
	const want = "v=[scalar] h=[scalar-hideval]\n"
	out, st := runZsh(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}

// `$funcstack` was the one produced view with no freeze on it, so the two
// things the freeze refuses were both taken here.
//
// Measured 2026-09-27 on zsh 5.9.2 under `-f`, with the module loaded:
// `funcstack=(a b)` and `unset funcstack` are each `read-only variable:
// funcstack` and fatal, where every other produced array beside it —
// `functrace`, `funcsourcetrace`, `funcfiletrace`, `reswords`, `errnos` —
// already refused both here.
//
// The sweep that found it is the one #4811 asked for and is why that issue's
// own row did not survive: see TestUnsettingAProducedTableLeavesTheAliasesStanding
// for the tables the reference really does let a script unset.
//
// **The `unset` row needs the read in front of it and the line above says
// why**: the freeze is on a parameter, and until something refers to the name
// there is no parameter for it to be on. `unset funcstack` as the *first*
// line of a script is a silent 0 in the reference and was the whole of #4895;
// `${+funcstack}` before it is what the measurement above was taken with, and
// the set test is the lightest reference there is. The assignment needs no
// such line, because an assignment is itself a reference — which is the pair
// that says the read is arming the freeze rather than hiding a refusal.
func TestAProducedViewRefusesAWriteAndAnUnset(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"assignment", `funcstack=(a b); print "unreached"`},
		{"unset after a reference", `: ${+funcstack}` + "\n" + `unset funcstack; print "unreached"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			line := strings.Count(tc.src[:strings.Index(tc.src, "unreached")], "\n") + 1
			want := fmt.Sprintf("zsh:%d: read-only variable: funcstack\n", line)
			if out != want || st != 1 {
				t.Errorf("%s = %q (status %d), want %q at status 1", tc.src, out, st, want)
			}
		})
	}
}

// And the other side of it, which is the row that keeps the read above from
// looking like a way of making a test pass: with nothing in front of it the
// same `unset` is a silent 0 and the script goes on, exactly as it does in the
// reference (#4895).
func TestAProducedViewTakesAnUnsetNothingHasReferredTo(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `unset funcstack`+"\n"+`print -r -- "reached=${+funcstack}"`)
	if want := "reached=0\n"; out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q at 0", out, st, want)
	}
}
