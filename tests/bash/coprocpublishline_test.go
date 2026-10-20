// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// Publishing a coprocess's ends is a **declaration**, so a name it cannot
// write refuses without giving up the rest of the line.
//
// The companion is published through `setVar`, which is the bare assignment's
// form, and a bare assignment's readonly refusal abandons the command list it
// is in. So a coprocess whose `_PID` name is a reference to a frozen variable
// wrote the sentence and then swallowed everything after it on that line.
//
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over script files, against bash 5.3.20 and bash 5.3.15, which agree:
//
//	declare -r RO2=a; declare -n ref_PID=RO2
//	coproc ref { :; }; wait; declare -p RO2      all on one line
//	  `RO2: readonly variable`, and the listing still runs
//
// **On separate lines nothing shows it**, because there is nothing left to
// give up — which is why this only ever appeared inside a file that puts the
// whole sequence on one line (#4178).
func TestPublishingACoprocessDoesNotGiveUpTheLine(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row: the listing after the refusal still runs.
			"the rest of the line runs after the refusal",
			"declare -r RO2=a; declare -n ref_PID=RO2; coproc ref { :; }; wait; declare -p RO2",
			`declare -r RO2="a"`, "",
		},
		{
			// And the refusal is still written.
			"and the refusal is still written",
			"declare -r RO2=a; declare -n ref_PID=RO2; coproc ref { :; }; wait",
			"RO2: readonly variable", "",
		},
		{
			// The frozen name keeps what the script put in it.
			"the frozen target is untouched",
			"declare -r RO2=a; declare -n ref_PID=RO2; coproc ref { :; }; wait\nprintf '[%s]' \"$RO2\"",
			"[a]", "",
		},
		{
			// The next line runs either way, which is the half that was
			// already right and hid the defect on a multi-line spelling.
			"the next line runs too",
			"declare -r RO2=a; declare -n ref_PID=RO2; coproc ref { :; }; wait; declare -p RO2\necho TAIL",
			"TAIL", "",
		},
		{
			// The control: nothing frozen, so nothing is refused and the
			// companion is published and reaped as usual.
			"an unfrozen coprocess publishes and reaps",
			"coproc CP { :; }; wait; declare -p CP_PID",
			"CP_PID: not found", "readonly",
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
