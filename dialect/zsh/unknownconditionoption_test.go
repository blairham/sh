// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// `posixbuiltins` makes `[[ -o name ]]` with a name this shell does not have
// the plain silent false the other columns give, instead of this shell's own
// complaint and third status.
//
// `C02cond.ztst`'s "-o invalidoption" chunk is what needs it, and the chunk
// names the tell itself: it writes `line 1: no warning` to standard error
// after the first probe, so the *absence* of the sentence is graded and not
// only the status.
//
// Measured 2026-09-29 on zsh 5.9.2, each probe in a subshell of its own so
// the option state cannot leak between them. Three fields kept apart,
// because with the option on the status and the stream move together and
// with it off neither does:
//
//	                            on            off
//	[[ -o invalidoption ]]      1, silent     3, `no such option: invalidoption`
//	[[ ! -o invalidoption ]]    0             3
//	[[ -o invalidoption || x ]] 0             0
//	[[ -o invalidoption && x ]] 1             3
//	[[ -o xtrace ]]             1, silent     1, silent
func TestPosixBuiltinsMakesAnUnknownConditionOptionAPlainFalse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, probe string
		on, off     int
		// unknown is whether the probe names an option this shell does not
		// have, which is what decides the *sentence*. Carried as its own
		// field rather than read off the status: the disjunction row is 0
		// with the option off and the sentence is still written, so a rule
		// keyed on the number would have called that row silent.
		unknown bool
	}{
		{"the bare test", `[[ -o invalidoption ]]`, 1, 3, true},
		// The combining operators are what say the two readings differ in
		// *kind* and not only in number: with the option on the value
		// combines like any other false, and with it off the third status
		// passes through `!` and `&&` untouched.
		{"negated", `[[ ! -o invalidoption ]]`, 0, 3, true},
		{"in a conjunction", `[[ -o invalidoption && -n x ]]`, 1, 3, true},
		// `||` goes on past it either way, which is the row that keeps the
		// pair above from reading as "the option makes it false".
		{"in a disjunction", `[[ -o invalidoption || -n x ]]`, 0, 0, true},
		// A name the shell **has** is untouched in either state, which is
		// the control that keeps this to the unknown name.
		{"a known option that is off", `[[ -o xtrace ]]`, 1, 1, false},
		{"a known option that is on", `[[ -o noxtrace ]]`, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, state := range []struct {
				setopt string
				want   int
				quiet  bool
			}{
				// With the option on nothing is ever said; with it off
				// the sentence is written for exactly the unknown names.
				{"setopt posixbuiltins", tc.on, true},
				{"unsetopt posixbuiltins", tc.off, !tc.unknown},
			} {
				src := state.setopt + "\n" + tc.probe + "\n"
				_, st, errs := runZshSplitOnRoute(t, interp.RouteScriptFile, src)
				if st != state.want {
					t.Errorf("%s; %s = %d, want %d (stderr %q)",
						state.setopt, tc.probe, st, state.want, errs)
				}
				said := strings.Contains(errs, "no such option")
				if said == state.quiet {
					verb := map[bool]string{true: "said", false: "did not say"}[said]
					t.Errorf("%s; %s %s `no such option`, want the opposite",
						state.setopt, tc.probe, verb)
				}
			}
		})
	}
}

// And the option is read off its axis rather than a stored bit, so the state
// stays inside a subshell — which is how the ztst chunk puts the two states
// side by side in one script.
func TestPosixBuiltinsUnknownConditionOptionStaysInItsSubshell(t *testing.T) {
	t.Parallel()
	src := "(setopt posixbuiltins; [[ -o invalidoption ]]; echo set:$?)\n" +
		"[[ -o invalidoption ]]; echo after:$?\n"
	out, _, _ := runZshSplitOnRoute(t, interp.RouteScriptFile, src)
	if out != "set:1\nafter:3\n" {
		t.Errorf("out = %q, want the subshell at 1 and the parent still at 3", out)
	}
}
