// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A command word that expanded to exactly `-` is thrown away in one dialect
// and looked up in the rest — see
// Semantics.LoneDashInCommandPositionIsDiscarded.
//
// Both answers over the same snippets, because that is the whole of what the
// axis is. The `--` row is the control and answers the same either way, which
// is what says this is the word being exactly one dash rather than a leading
// one.
func TestACommandWordThatIsExactlyADash(t *testing.T) {
	for _, c := range []struct{ name, src, discarded, kept string }{
		{
			"the word is dropped and the rest runs",
			`- echo hi; echo "st=$?"`,
			"hi\nst=0\n",
			"st=127\n",
		},
		{
			"as many of them as are written",
			`- - - echo hi; echo "st=$?"`,
			"hi\nst=0\n",
			"st=127\n",
		},
		{
			"quoting does not protect it",
			`'-' echo hi; echo "st=$?"`,
			"hi\nst=0\n",
			"st=127\n",
		},
		{
			"nor does arriving through an expansion",
			`v=-; $v echo hi; echo "st=$?"`,
			"hi\nst=0\n",
			"st=127\n",
		},
		{
			"the status is the command's own",
			`- false; echo "st=$?"`,
			"st=1\n",
			"st=127\n",
		},
		{
			"the word after it is a command word, not a prefix",
			`- v=1 echo hi; echo "st=$?"`,
			"st=127\n",
			"st=127\n",
		},
		{
			"two dashes is an ordinary name",
			`-- echo hi; echo "st=$?"`,
			"st=127\n",
			"st=127\n",
		},
		{
			"nothing left is nothing run",
			`-; echo "st=$?"`,
			"st=0\n",
			"st=127\n",
		},
		{
			"and the assignments in front of it still persist",
			`v=1 -; echo "st=$? v=[$v]"`,
			"st=0 v=[1]\n",
			"st=127 v=[]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := permissive()
			sem.LoneDashInCommandPositionIsDiscarded = Yes
			if got := prefixAssignRun(t, c.src, sem); got != c.discarded {
				t.Errorf("discarded: %s = %q, want %q", c.src, got, c.discarded)
			}
			sem.LoneDashInCommandPositionIsDiscarded = No
			if got := prefixAssignRun(t, c.src, sem); got != c.kept {
				t.Errorf("kept: %s = %q, want %q", c.src, got, c.kept)
			}
		})
	}
}
