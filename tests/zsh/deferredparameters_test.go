// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A parameter this shell brings into being on the script's first reference.
//
// Measured 2026-09-27 on zsh 5.9.2 from a script file under `env -i
// PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, each cell in a shell
// of its own so that no row primes the next. The roster is in
// dialect/zsh/deferredparameters.go and the mechanism in
// interp/deferredparam.go; these rows are the behavior.
//
// **Every row is a pair**, and that is the whole design of this test rather
// than thoroughness. A grid taken with a read in front of it agrees with a
// shell that defers nothing, and a grid taken with nothing in front agrees
// with a shell that has lost the freeze — the two issues this closes were
// each filed from one half of such a pair (#4895, #4899). Only a row that
// moves between the two says the state exists.
func TestAParameterArrivesOnItsFirstReference(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// #4895's one surviving row.
			"an unset with nothing in front of it is taken",
			`unset funcstack
			 print -r -- "rc=$? plus=${+funcstack}"`,
			"rc=0 plus=0\n",
		},
		{
			"and the same unset after the lightest read there is refuses",
			`: ${+funcstack}
			 unset funcstack
			 print -r -- unreached`,
			"zsh:2: read-only variable: funcstack\n",
		},
		{
			// #4899's headline case, on a parameter that is not a module
			// table at all — which is what says the mechanism is the
			// shell's own laziness rather than module loading.
			"a listing before any reference writes no row",
			`typeset -p dirstack; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			"and after one it writes the row",
			`: ${#dirstack}; typeset -p dirstack; print -r -- "st=$?"`,
			"typeset -a dirstack\nst=0\n",
		},
		{
			// The listing is itself no reference, which is the property
			// the whole roster was measured through: if asking brought the
			// parameter in, the second line would write a row.
			"and asking twice is still no reference",
			`typeset -p dirstack; typeset -p dirstack; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			// A shell-driven change to the same table is not a reference.
			"the shell filling the table is not a reference",
			`setopt autopushd; cd /; alias zz=1; f(){ :; }
			 typeset -p dirstack; typeset -p aliases; typeset -p functions
			 print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			// And a script's assignment is one, even to a table it may not
			// write — the freeze an assignment meets is the freeze its own
			// arrival put there.
			"a script's assignment is a reference",
			`aliases=(zz 1); typeset -p aliases; print -r -- "st=$?"`,
			"typeset -A aliases\nst=0\n",
		},
		{
			// An element read counts, which is the single most common
			// thing a plugin manager writes.
			"a subscripted read is a reference",
			`f(){ :; }; : ${functions[f]}; typeset -p functions; print -r -- "st=$?"`,
			"typeset -A functions\nst=0\n",
		},
		{
			// The removal of a name nothing referred to is the end of the
			// parameter and not a return to the state before it: there is
			// nothing left to arrive on a later read.
			"an unset before any reference removes the parameter outright",
			`unset funcstack
			 f(){ print -r -- "in=${#funcstack} plus=${+funcstack}"; }
			 f`,
			"in=0 plus=0\n",
		},
		{
			// The control that says this is not "the listing has gone
			// quiet": an ordinary name the script declared is written in
			// the same run, and a name nothing has heard of is reported.
			"an ordinary name is unaffected",
			`w=1; typeset -p w; typeset -p neverheardof 2>/dev/null
			 print -r -- "st=$?"`,
			"typeset w=1\nst=1\n",
		},
		{
			// And the second control, in the other direction: a name whose
			// module declares no autoloadable parameter is deliberately
			// *not* deferred, because it is not there at all. `typeset -p
			// sysparams` is `no such variable` at 1 in the reference, which
			// is neither state this mechanism models — it is the gate, and
			// #4922 closed it. Before that this row read `typeset -Ar
			// sysparams` at 0, which was the gap written down rather than
			// the reference's answer.
			"a parameter of a module with no autoloadable name is not deferred",
			`typeset -p sysparams; print -r -- "st=$?"`,
			"zsh:typeset:1: no such variable: sysparams\nst=1\n",
		},
		{
			// And it is deferred by neither route after the load, which is
			// the pair that keeps the two mechanisms apart: the parameter is
			// there and its row is written, where a *deferred* name still
			// writes nothing until something refers to it.
			"and after the load it is there and lists",
			`zmodload zsh/system; typeset -p sysparams; print -r -- "st=$?"`,
			"typeset -Ar sysparams\nst=0\n",
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

// An explicit module load is a reference too, though nothing in the script
// named the parameter.
//
// Measured 2026-09-27 on zsh 5.9.2 from a script file, in a shell that has
// read none of the names. The pair is the point again: the load is what moves
// the row, and the same `unset` without it is a silent 0.
func TestAnExplicitModuleLoadIsAReference(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"after the load the freeze is there",
			`zmodload zsh/parameter
			 unset funcstack
			 print -r -- unreached`,
			"zsh:2: read-only variable: funcstack\n",
		},
		{
			"and without it the unset is taken",
			`unset funcstack; print -r -- "rc=$?"`,
			"rc=0\n",
		},
		{
			"the same for another module's parameters",
			`zmodload zsh/zleparameter
			 unset widgets
			 print -r -- unreached`,
			"zsh:2: read-only variable: widgets\n",
		},
		{
			// The control that says the load is reaching the *module's own*
			// names rather than every deferred name at once.
			"and it reaches only that module's names",
			`zmodload zsh/zleparameter
			 unset funcstack; print -r -- "rc=$?"`,
			"rc=0\n",
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

// And the listing forms split, which is measured and is why the skip is in
// some of the loops rather than in the name's own record.
//
// Measured 2026-09-27 on zsh 5.9.2 in one shell that has referred to none of
// them: a bare `typeset` and a whole-table `typeset +` write every module
// table, `set` writes each bare name, and `readonly` and every `-p` form
// write none.
//
// **What the two forms that write such a name write** is a separate question
// and is in deferredlisting_test.go — `undefined funcstack` rather than the
// registered attributes, and the bare name for `typeset + funcstack`
// (#4923, #4924). The row below said the whole-table plus wrote *nothing*
// for it, which was a probe grepping that listing for `^funcstack$`: the
// line is there and an anchored bare name is exactly what cannot see it.
func TestTheListingFormsSplitOverADeferredParameter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the set listing writes the name",
			`set | while IFS= read -r l; do case $l in (funcstack) print -r -- "[$l]";; esac; done`,
			"[funcstack]\n",
		},
		{
			"the frozen listing writes nothing",
			`readonly | while IFS= read -r l; do case $l in (*funcstack*) print -r -- "[$l]";; esac; done
			 print -r -- "done"`,
			"done\n",
		},
		{
			// Written as the name alone, which is the row the reference
			// does not have: the line it writes is `undefined funcstack`,
			// and this is the anchored pattern that used to read that as
			// silence. See deferredlisting_test.go for the row itself.
			"and the whole-table name listing writes no bare name",
			`typeset + | while IFS= read -r l; do case $l in (funcstack) print -r -- "[$l]";; esac; done
			 print -r -- "done"`,
			"done\n",
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
