// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
)

// TestAnUnknownOptionIsReportedPerLetter is the other half of #3167.
//
// This shell's option parser splits a dash-word into its characters and
// complains about each, then writes its usage block once. It is not specific
// to `kill` — it is how that parser reports — and this shell named the whole
// word once. Measured 2026-09-17 on ksh93u+ 2012-08-01, macOS arm64, under a
// matching `argv[0]`, from a script file:
//
//	kill -9x <pid>       kill: -9:  then  kill: -x:      + usage,  2
//	kill -0x <pid>       kill: -0:  then  kill: -x:      + usage,  2
//	kill -99x <pid>      kill: -9:, kill: -9:, kill: -x: + usage,  2
//	kill -NOPE <pid>     -N, -O, -P, -E                  + usage,  2
//	kill -SIGNOPE <pid>  all seven letters               + usage,  2
//	kill -Q <pid>        kill: -Q: unknown option        + usage,  2
//
// Three details are measured and none is decoration: a **repeat is repeated**,
// the **case is kept** as written, and the **usage block is written once**
// however many complaints came before it — which is what took it out of the
// wording, where it had been concatenated.
//
// The block carries no location, exactly as the block a bare `kill` writes
// does, which is what Diagnostics.KillUsageUnprefixed already said.
func TestAnUnknownOptionIsReportedPerLetter(t *testing.T) {
	d := ksh.Diagnostics()
	if !d.KillIllegalOptionPerLetter {
		t.Error("KillIllegalOptionPerLetter is false, want true")
	}
	if strings.Contains(d.KillIllegalOption, "Usage:") {
		t.Errorf("KillIllegalOption is %q, want the sentence alone — the block is written once", d.KillIllegalOption)
	}
	if d.KillIllegalOptionUsage == "" {
		t.Error("KillIllegalOptionUsage is empty, want the block that follows the complaints")
	}
	for _, c := range []struct {
		src     string
		letters []string
	}{
		{`kill -9x 999999`, []string{"kill: -9: unknown option", "kill: -x: unknown option"}},
		{`kill -NOPE 999999`, []string{"kill: -N: unknown option", "kill: -O: unknown option", "kill: -P: unknown option", "kill: -E: unknown option"}},
		{`kill -Q 999999`, []string{"kill: -Q: unknown option"}},
	} {
		out, _ := answersRun(t, c.src+` 2>&1 >/dev/null; echo "st=$?"`)
		for _, want := range c.letters {
			if !strings.Contains(out, want) {
				t.Errorf("%s said %q, want %q in it", c.src, out, want)
			}
		}
		if strings.Count(out, "Usage:") != 1 {
			t.Errorf("%s said %q, want the usage block exactly once", c.src, out)
		}
		if !strings.Contains(out, "st=2") {
			t.Errorf("%s said %q, want status 2", c.src, out)
		}
	}
	// A repeat is repeated, which is what says this is the word's characters
	// and not a set of them.
	out, _ := answersRun(t, `kill -99x 999999 2>&1 >/dev/null`)
	if strings.Count(out, "kill: -9: unknown option") != 2 {
		t.Errorf("kill -99x said %q, want the -9 complaint twice", out)
	}
	// And the block carries no location, as the bare `kill` usage does not.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Usage:") {
			return
		}
	}
	t.Errorf("kill -99x said %q, want the usage block with nothing in front of it", out)
}
