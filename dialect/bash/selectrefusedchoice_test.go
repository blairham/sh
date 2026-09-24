// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A `select` whose variable will not take the chosen item **ends the loop**.
//
// The only way that store can fail is the reference one: a `select` variable
// that is a reference with nothing to point at is aimed by the item it chose,
// and an item that is no possible name aims it nowhere. This shell reported the
// refusal and went round again, so a second `#? ` reached the error stream and
// the EOF pass wrote a line to the output that bash never writes.
//
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh` over
// script files, against bash 5.3.20 and bash 5.3.15, which agree:
//
//	declare -n r; select r in /; do :; done <<< 1
//	  the menu, then `` `/': not a valid identifier ``, and **no prompt**
//	declare -n r; select r in tgt; do echo body; done <<< 1
//	  the menu, a prompt, `body`, a second prompt, then EOF
//
// The second is the control: a choice the name takes leaves the loop running
// exactly as before (#4178).
func TestASelectEndsWhenTheNameWillNotTakeTheChoice(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row `nameref11.sub` grades: **one** prompt, for the read
			// that chose, and no second one. Counted rather than matched,
			// because the prompt before the read is legitimate and only the
			// one after the refusal is the defect.
			"exactly one prompt, not two",
			"declare -n r\n{ select r in /; do :; done <<< 1; } 2>&1 | grep -c '#?'",
			"1", "",
		},
		{
			// And the menu is still written, so the loop did start.
			"the menu is still written",
			"declare -n r\nselect r in /; do :; done <<< 1",
			"1) /", "",
		},
		{
			// The command after it runs: ending the loop is not ending the
			// script.
			"the next command still runs",
			"declare -n r\nselect r in /; do :; done <<< 1\necho TAIL",
			"TAIL", "",
		},
		{
			// The control: a choice the name takes runs the body and prompts
			// again.
			"a good choice runs the body and prompts again",
			"declare -n r\n{ select r in tgt; do echo body; done <<< 1; } 2>&1 | grep -c '#?'",
			"2", "",
		},
		{
			// A plain name is untouched by any of this.
			"a plain name is unchanged",
			"select p in one; do echo body; done <<< 1\necho TAIL",
			"body", "not a valid identifier",
		},
		{
			// **A failure earlier in the script does not end the loop**,
			// which is why the flag is read as a delta rather than as a
			// value: `assignFailed` is a mark the builtin around a store
			// reads, not a per-store result, and it is still set here.
			// Without the delta the body never runs at all.
			"an earlier failed assignment does not end the loop",
			"declare -n q\nq=/\nselect p in one; do echo body; done <<< 1\necho TAIL",
			"body", "",
		},
		{
			// A reply naming no item leaves the name empty and still runs
			// the body — the store succeeds, so the loop is not ended.
			"a reply out of range still runs the body",
			"select p in one; do printf '[%s]' \"$p\"; done <<< 9\necho TAIL",
			"[]", "not a valid identifier",
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
