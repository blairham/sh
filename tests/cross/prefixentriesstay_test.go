// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// When a failure in a command ends the shell, the prefix entries it had
// already assigned are where an EXIT trap finds them, in every column but
// bash's — and a failure that is contained in the command puts them back.
// Measured 2026-10-30 from `-c` under `env -i PATH=/usr/bin:/bin` against
// ksh93u+ 2012-08-01, zsh 5.9.2, GNU bash 5.3.20, dash 0.5.12 and BusyBox ash
// 1.37.0 in the pinned image. This shell put them back in every case (#7004).
// See interp.Semantics.PrefixEntriesStayWhenAFailureEndsTheShell.
func TestAFailedPrefixLeavesTheEntriesAheadOfIt(t *testing.T) {
	const pre = `exec 2>/dev/null; trap 'echo "trap v=$v"' EXIT; v=old; f() { :; }; `
	const left, back = "trap v=new\n", "trap v=old\n"
	for _, c := range []struct {
		name, src string
		want      map[string]string
	}{
		{
			"a special builtin",
			`v=new w=${q?boom} :`,
			map[string]string{"bash": back, "ksh": left, "zsh": left, "dash": left, "ash": left},
		},
		{
			"a function",
			`v=new w=${q?boom} f`,
			map[string]string{"bash": back, "ksh": left, "zsh": left, "dash": left, "ash": left},
		},
		{
			"a regular builtin, where ksh93 carries on",
			`v=new w=${q?boom} true`,
			map[string]string{"bash": back, "ksh": back, "zsh": left, "dash": left, "ash": left},
		},
		{
			"a program, where ksh93 and zsh carry on",
			`v=new w=${q?boom} /usr/bin/true`,
			map[string]string{"bash": back, "ksh": back, "zsh": back, "dash": left, "ash": left},
		},
		{
			"eval",
			`v=new w=${q?boom} eval :`,
			map[string]string{"bash": back, "ksh": left, "zsh": left, "dash": left, "ash": left},
		},
		{
			"a declaration's operand",
			`v=new typeset x=$v y=${q?boom}`,
			map[string]string{"bash": back, "ksh": left, "zsh": left},
		},
		{
			"a refusal in front of a special builtin",
			`readonly r=0; v=new r=1 :`,
			map[string]string{"bash": back, "ksh": left, "dash": left, "ash": left},
		},
		{
			"a refusal in front of a program",
			`readonly r=0; v=new r=1 /usr/bin/true`,
			map[string]string{"bash": back, "ksh": back, "dash": left, "ash": left},
		},
		// The control: a failure in an argument is not the prefix's.
		{
			"an argument",
			`v=new /usr/bin/true ${q?boom}`,
			map[string]string{"bash": back, "ksh": back, "zsh": back, "dash": back, "ash": back},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for name, want := range c.want {
				out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, pre+c.src)
				if err != nil {
					t.Fatal(err)
				}
				if out != want {
					t.Errorf("%s: got %q, want %q", name, out, want)
				}
			}
		})
	}
}
