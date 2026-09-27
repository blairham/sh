// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// What a declaration listing says about a **special** parameter an `unset`
// has removed.
//
// The reference passes over it in silence at 0, where a removed name of a
// script's own is `no such variable` at 1 — so this shell may not simply be
// quiet about missing names, and may not simply report them either. Measured
// 2026-09-27 on zsh 5.9.2 from a script file under `env -i
// PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME` (#4896).
//
// The attribute word this turns on is `special` and the pairs that hold the
// others still are in interp/removedshellown.go; the rows here are the ones
// that would have to move together if the rule were the kind, the export or
// the freeze instead.
func TestARemovedSpecialIsPassedOverInSilence(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The case the issue was filed on.
			"an integer special",
			`unset RANDOM; typeset -p RANDOM; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			"a scalar special",
			`unset PS1; typeset -p PS1; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			"an array special",
			`unset path; typeset -p path; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			// The produced pipeline record, which reaches the state by a
			// route of its own: `unset` ends it in this dialect, so the
			// producer is masked off and only the registration is left to
			// say the name was ever the shell's.
			"the produced pipeline record",
			`unset pipestatus; typeset -p pipestatus; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			// A name whose *letters* outlive the removal, because a window
			// size is not a script's to lose — see interp.clearAttributes.
			// Without the test being asked of the name it would be written
			// as `typeset -i10 COLUMNS`, a row with no value in it, which
			// is a different wrong answer from the one above.
			"a special whose attributes outlived it",
			`unset COLUMNS; typeset -p COLUMNS; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			// Every listing form and not the `-p` word alone.
			"the exported and frozen spellings of the same listing",
			`unset SECONDS
			 export -p SECONDS; print -r -- "e=$?"
			 readonly -p SECONDS; print -r -- "r=$?"
			 typeset + SECONDS; print -r -- "p=$?"`,
			"e=0\nr=0\np=0\n",
		},
		{
			// The slot survives the removal rather than a value doing:
			// nothing is left to print, and a second `unset` is still
			// silent.
			"nothing of the parameter is left",
			`unset RANDOM; unset RANDOM
			 typeset -p RANDOM; print -r -- "st=$? plus=${+RANDOM} t=${(t)RANDOM}"`,
			"st=0 plus=0 t=\n",
		},
		{
			// And an assignment brings the specialness back, which is why
			// the silence is "the shell has the name" rather than "the
			// shell has stopped describing it".
			"an assignment brings it back",
			`unset RANDOM; RANDOM=5; print -r -- "${(t)RANDOM}"`,
			"integer-special\n",
		},
		{
			// The control, and the reason the rows above are a finding: an
			// ordinary name an `unset` took is still reported, in the same
			// run, so this shell has not gone quiet about removed names.
			"an ordinary removed name is still reported",
			`w=1; unset w; typeset -p w 2>/dev/null; print -r -- "st=$?"`,
			"st=1\n",
		},
		{
			// The second control: a name nothing has ever heard of.
			"and a name nothing has heard of",
			`typeset -p neverheardof 2>/dev/null; print -r -- "st=$?"`,
			"st=1\n",
		},
		{
			// The third, and the one that says the word is `special`
			// rather than "the shell set it at startup": `LOGNAME` is a
			// name this shell knows and does not call its own, and it
			// takes the missing-name route in the reference too.
			"a name the shell set that is not special",
			`LOGNAME=x; unset LOGNAME; typeset -p LOGNAME 2>/dev/null; print -r -- "st=$?"`,
			"st=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
