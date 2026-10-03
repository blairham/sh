// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **Which signals a shell reads joined to `kill -s` and `kill -n`** —
// Semantics.KillReadsASignalJoinedToItsOption and
// KillJoinsANumberToTheNameOption together, by the status of each probe.
//
// Measured 2026-10-03 with
// `kill -s0 $$; echo a=$?; kill -sCONT $$; echo b=$?; kill -n0 $$; echo c=$?`:
//
//	dash 0.5.12        a=0 b=0 c=2   -s joins anything; -n is no option
//	ksh93u+            a=0 b=0 c=0   both join anything they would take spaced
//	bash 5.3.20        a=1 b=0 c=0   -s joins a name and -n a number
//	zsh 5.9.2          a=1 b=1 c=1   nothing is joined
//
// Every probe aims at the shell itself with 0 or CONT, so nothing is stopped
// or killed whichever way a word is read.
func TestEachDialectReadsWhatItJoinsToKillsOptions(t *testing.T) {
	src := "kill -s0 $$ 2>/dev/null; echo a=$?; kill -sCONT $$ 2>/dev/null; echo b=$?; kill -n0 $$ 2>/dev/null; echo c=$?\n"
	for name, want := range map[string]string{
		"dash": "a=0\nb=0\nc=2\n",
		"ksh":  "a=0\nb=0\nc=0\n",
		"bash": "a=1\nb=0\nc=0\n",
		"zsh":  "a=1\nb=1\nc=1\n",
	} {
		t.Run(name, func(t *testing.T) {
			out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
			if err != nil {
				t.Fatal(err)
			}
			if out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}
