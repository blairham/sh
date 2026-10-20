// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A duplication from a descriptor nothing is open on is reported as the
// failed call rather than as a number: both descriptors, in the order the
// syscall takes them, and the errno after (#3909).
//
// Measured 2026-09-20, BusyBox v1.37.0 in the digest-pinned alpine image,
// `env -i PATH=/usr/bin:/bin LC_ALL=C /bin/sh case.sh`:
//
//	cat <&19        dup2(19,0): Bad file descriptor
//	cat 0<&19       dup2(19,0): Bad file descriptor
//	echo hi >&19    dup2(19,1): Bad file descriptor
//	echo hi 2>&19   dup2(19,2): Bad file descriptor
//	echo hi 3>&19   dup2(19,3): Bad file descriptor
//	exec 5>&19      dup2(19,5): Bad file descriptor
//
// The second number is what makes this a third verb rather than a wording:
// it is the descriptor the redirection was *aiming at*, and it is not
// derivable from the source. Every other column in the panel names the source
// alone — bash's three write `19: Bad file descriptor`, zsh lowercases it,
// ksh93 brackets the errno — so Diagnostics.DuplicationSourceNotOpen grew a
// %[3]s that only this dialect spends.
//
// **Descriptor 10 is not a probe to use here.** In a script file BusyBox
// answers `10: Bad file descriptor` for `<&10`, because 10 is where it keeps
// the script itself; 4, 9, 11 and 19 all answer `dup2(N,…)`. A measurement
// taken only at 10 reads as agreement with bash, which is how this column
// came to hold bash's sentence.
func TestADuplicationFromAClosedNumberNamesTheCallAndBothNumbers(t *testing.T) {
	for _, tc := range []struct{ name, src, want, why string }{
		{
			"a bare read side",
			"cat <&19\n", "dup2(19,0): Bad file descriptor",
			"an unwritten number is 0 for `<&`, which no part of the source says",
		},
		{
			"the read side written out",
			"cat 0<&19\n", "dup2(19,0): Bad file descriptor",
			"the same line as the bare spelling, which is what says the 0 above is the target and not a default",
		},
		{
			"a bare write side",
			"echo hi >&19\n", "dup2(19,1): Bad file descriptor",
			"and 1 for a bare `>&` — the row that a target derived from the source could not produce",
		},
		{
			"the error stream",
			"echo hi 2>&19\n", "dup2(19,2): Bad file descriptor",
			"a number written in front of the operator is the target",
		},
		{
			"a number no stream is named by",
			"echo hi 3>&19\n", "dup2(19,3): Bad file descriptor",
			"and it need not be one of the three the shell starts with",
		},
		{
			"through exec, where the redirection outlives the command",
			"exec 5>&19\n", "dup2(19,5): Bad file descriptor",
			"the sentence is the redirection's and does not change with what made it",
		},
		{
			"a single-digit source",
			"cat <&4\n", "dup2(4,0): Bad file descriptor",
			"the source is not quoted or padded — it is the number as the script wrote it",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runIn(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q — %s", out, tc.want, tc.why)
			}
		})
	}
}

// The control, and it is a real one: the three rows above that differ only in
// their target would all pass a sentence that printed the source twice, or
// one that printed a constant. Asserting that the six targets are six
// different strings is what says the verb is the target rather than anything
// else on the line.
func TestTheSecondNumberIsTheTargetAndNotTheSource(t *testing.T) {
	seen := map[string]string{}
	for _, tc := range []struct{ src, target string }{
		{"cat <&19\n", "0"},
		{"echo hi >&19\n", "1"},
		{"echo hi 2>&19\n", "2"},
		{"echo hi 3>&19\n", "3"},
		{"exec 5>&19\n", "5"},
		{"exec 7>&19\n", "7"},
	} {
		out, _ := runIn(t, tc.src)
		if prev, dup := seen[out]; dup {
			t.Errorf("%q and %q both say %q — the target is not being read",
				prev, tc.src, out)
		}
		seen[out] = tc.src
		if want := "dup2(19," + tc.target + ")"; !strings.Contains(out, want) {
			t.Errorf("got %q for %q, want it to contain %q", out, tc.src, want)
		}
	}
}

// And the near miss the sentence must not swallow: a word that is no
// descriptor at all keeps this shell's own two sentences, which are neither
// the duplication's failure nor a number. A change that routed every `>&`
// failure through the new wording would write `dup2(qq,2)` here.
func TestAWordThatNamesNoDescriptorKeepsItsOwnSentence(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a word that came to something", "echo hi 2>&qq\n", "redir error"},
		{"a word that came to nothing", "echo hi 2>&$UNSETZZ\n", "syntax error: bad fd number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runIn(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
			if strings.Contains(out, "dup2(") {
				t.Errorf("got %q, want no dup2 in it — nothing was duplicated", out)
			}
		})
	}
}
