// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAProducedParameterNamesItsType is `${(t)NAME}` over the parameters this
// shell produces rather than stores.
//
// Every one of them answered `scalar` before, because the reader asks the
// attribute tables and a produced parameter is in none of them — while the
// fact it needed had been stated beside the producer since #2451 and simply
// was not asked for (#2552). Each row is zsh 5.9.2, measured 2026-09-12 with
// `env -i PATH=/usr/bin:/bin`, a scratch `HOME`, `ZDOTDIR` and `HISTFILE`,
// over a script file.
//
// The three attribute words beside the type are the control: they were
// already right, so a row that only checked the first word could not tell a
// fix from a reader that had started answering `integer` for everything.
func TestAProducedParameterNamesItsType(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, decl, want string
	}{
		{"RANDOM", "", "integer-special"},
		{"SECONDS", "", "integer-special"},
		{"ARGC", "", "integer-readonly-special"},
		{"LINENO", "", "integer-readonly-special"},
		{
			name: "EPOCHSECONDS",
			decl: "zmodload zsh/datetime; ",
			want: "integer-readonly-hide-hideval-special",
		},
		{
			name: "EPOCHREALTIME",
			decl: "zmodload zsh/datetime; ",
			want: "float-readonly-hide-hideval-special",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.decl + `print -r -- "${(t)` + tc.name + `}"`
			out, st := runZsh(t, dir, src)
			if want := tc.want + "\n"; out != want || st != 0 {
				t.Errorf("${(t)%s} = %q (status %d), want %q", tc.name, out, st, want)
			}
		})
	}
}

// TestAModuleParameterCarriesBothHidingAttributes is the second half of
// #2552, and the pair is measured rather than assumed.
//
// The two hiding attributes are genuinely two — `typeset -H v` describes as
// `hideval` alone (#2042) — but a parameter a *module* provides carries both
// in zsh, all thirty of them, readonly or not. Every registration site here
// called [interp.Runner.MarkHidden] and none called MarkHideInScope, so every
// module parameter described with `hideval` and without `hide`: the shape a
// new helper omitting what the old one carries always takes.
//
// The shell's own specials are the control on the other side. `RANDOM` and
// `ARGC` carry neither word, so a fix that marked everything special would
// fail here rather than pass.
func TestAModuleParameterCarriesBothHidingAttributes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, decl, want string
	}{
		{"builtins", "zmodload zsh/parameter; ", "association-readonly-hide-hideval-special"},
		{"reswords", "zmodload zsh/parameter; ", "array-readonly-hide-hideval-special"},
		{"parameters", "zmodload zsh/parameter; ", "association-readonly-hide-hideval-special"},
		{"epochtime", "zmodload zsh/datetime; ", "array-readonly-hide-hideval-special"},
		{"errnos", "zmodload zsh/system; ", "array-readonly-hide-hideval-special"},
		{"sysparams", "zmodload zsh/system; ", "association-readonly-hide-hideval-special"},
		{"RANDOM", "", "integer-special"},
		{"ARGC", "", "integer-readonly-special"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.decl + `print -r -- "${(t)` + tc.name + `}"`
			out, st := runZsh(t, dir, src)
			if want := tc.want + "\n"; out != want || st != 0 {
				t.Errorf("${(t)%s} = %q (status %d), want %q", tc.name, out, st, want)
			}
		})
	}
}
