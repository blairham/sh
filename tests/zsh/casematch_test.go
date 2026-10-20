// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `casematch` is the `=~` operator's option and **nothing else's**, which is
// the whole reason it is not the same wire as bash's option of the same name.
//
// A name two shells share need not be one feature. Measured 2026-09-13, with
// each shell's own `nocasematch` turned on:
//
//	                       zsh 5.9.2   bash 5.3.15
//	[[ ABC =~ ^abc$ ]]     yes         yes
//	[[ ABC == abc ]]       no          yes
//	case A in a)           exact       hit
//	v=ABC; ${v//b/X}       ABC         AXC
//
// So the core keeps two switches and each dialect wires the one its name
// means. Reading them as one switch would have made three of these four rows
// wrong here, silently and in the permissive direction (#2622).
func TestCasematchReachesTheRegexOperatorAndNothingElse(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the regex operator folds",
			`setopt nocasematch; [[ ABC =~ ^abc$ ]] && echo yes || echo no`, "yes",
		},
		{
			"and the whole expression folds with it",
			`setopt nocasematch; [[ ABC =~ ^[[:lower:]]+$ ]] && echo yes || echo no`, "yes",
		},
		{
			"a bracket expression too",
			`setopt nocasematch; [[ ABC =~ ^[abc]+$ ]] && echo yes || echo no`, "yes",
		},

		// The three bash folds under the same name and this shell does not.
		{
			"the pattern operator does not",
			`setopt nocasematch; [[ ABC == abc ]] && echo yes || echo no`, "no",
		},
		{
			"nor does a case statement",
			`setopt nocasematch; case A in a) echo hit;; *) echo exact;; esac`, "exact",
		},
		{
			"nor does a substitution",
			`setopt nocasematch; v=ABC; echo ${v//b/X}`, "ABC",
		},

		// Written the other way round, since `casematch` is on with nothing
		// said and the interesting state is the one a script turns off.
		{
			"unsetopt is the same switch",
			`unsetopt casematch; [[ ABC =~ ^abc$ ]] && echo yes || echo no`, "yes",
		},
		{
			"and on is where it starts",
			`[[ ABC =~ ^abc$ ]] && echo yes || echo no`, "no",
		},
		{
			"turned back on again",
			`setopt nocasematch; setopt casematch; [[ ABC =~ ^abc$ ]] && echo yes || echo no`, "no",
		},

		// `caseglob` is a third thing again and reaches neither — it is
		// pathname expansion alone, so the fold is read off the option that
		// governs it rather than off whichever one happened to be on.
		{
			"caseglob is not that option",
			`unsetopt caseglob; [[ ABC =~ ^abc$ ]] && echo yes || echo no`, "no",
		},

		// The listing has to follow the switch, or `setopt` describes a shell
		// that is not the one running.
		{
			"the listing follows the switch",
			`setopt nocasematch; [[ -o casematch ]] && echo on || echo off`, "off",
		},
		{
			"and follows it back",
			`setopt nocasematch; setopt casematch; [[ -o casematch ]] && echo on || echo off`, "on",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if got := strings.TrimRight(out, "\n"); got != tc.want {
				t.Errorf("%s = %q status %d, want %q", tc.src, got, st, tc.want)
			}
		})
	}
}
