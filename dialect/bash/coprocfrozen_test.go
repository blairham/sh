// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A coprocess over a **frozen** name says two sentences, and the second is the
// reaping's.
//
// Publishing the ends into a readonly name is refused — that was already right.
// What was missing is that the reaping takes both names back whether or not the
// publishing put anything in them, and that take-back is an ordinary `unset`:
// a frozen array refuses it, in the words any unset uses and **spoken as the
// shell**, because no builtin is running by then.
//
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh` over
// script files, against bash 5.3.20 and bash 5.3.15, which agree, over the
// three shapes a freeze can take:
//
//	declare -r B=1; coproc B { :; }; wait
//	  `B: readonly variable`, then `B: cannot unset: readonly variable`,
//	  `B` still `-r B="1"` and `B_PID` gone
//	declare -r C_PID=2; coproc C { :; }; wait
//	  `C_PID: readonly variable`, and both names gone afterwards
//	declare -r A=1; declare -r A_PID=2; coproc A { :; }; wait
//	  `A: readonly variable` — the array alone — then
//	  `A: cannot unset: readonly variable`, `A` standing and `A_PID` gone
//
// Three things were wrong here: the refusal named **both** frozen names where
// bash names the first, the reaping never ran at all because the name was only
// recorded when publishing succeeded, and a frozen companion was left listed as
// a bare `declare -r A_PID` that bash has no such name for (#4178).
func TestACoprocessOverAFrozenNameSaysTheReapingsSentenceToo(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row `nameref11.sub` grades.
			"the reaping's refusal is spoken, without a builtin's name",
			"declare -r B=1\ncoproc B { :; }\nwait",
			": B: cannot unset: readonly variable", "unset: B: cannot unset",
		},
		{
			// And the array keeps what the script put in it.
			"the frozen array keeps its value",
			"declare -r B=1\ncoproc B { :; }\nwait\ndeclare -p B",
			`declare -r B="1"`, "",
		},
		{
			// The companion goes, letters and all.
			"the companion goes even when frozen",
			"declare -r A=1\ndeclare -r A_PID=2\ncoproc A { :; }\nwait\ndeclare -p A_PID",
			"A_PID: not found", "declare -r A_PID",
		},
		{
			// One sentence for the publish, naming the array, where both are
			// frozen.
			"the publish names the array alone when both are frozen",
			"declare -r A=1\ndeclare -r A_PID=2\ncoproc A { :; }\nwait",
			"A: readonly variable", "A_PID: readonly variable",
		},
		{
			// A frozen companion alone still names the companion, so it is
			// the *first* frozen name that speaks rather than a fixed one.
			"a frozen companion alone still names itself",
			"declare -r C_PID=2\ncoproc C { :; }\nwait",
			"C_PID: readonly variable", "cannot unset",
		},
		{
			// And both names are gone after it.
			"and both names are gone after it",
			"declare -r C_PID=2\ncoproc C { :; }\nwait\ndeclare -p C_PID",
			"C_PID: not found", "declare -r C_PID",
		},
		{
			// The control: nothing frozen, both names published and then
			// reaped, with no sentence at all.
			"an unfrozen coprocess is silent and leaves nothing",
			"coproc CP { :; }\nwait\ndeclare -p CP\ndeclare -p CP_PID",
			"CP: not found", "readonly",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
			if tc.absent != "" && strings.Contains(out, tc.absent) {
				t.Errorf("= %q, want it not to contain %q", out, tc.absent)
			}
		})
	}
}
