// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// An operand-less `exit` or `return` reports a status, and the panel does not
// agree about which one.
//
// Six of the seven columns report `$?` — whatever the shell was holding when
// the word was reached, however it got there. ksh93 reports the last status
// **this execution unit** produced, which is 0 in a unit that has run no
// command of its own, and `$?` in the same place still reads the inherited
// value. Measured 2026-09-18, a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with standard input on /dev/null:
//
//	                                            ksh93u+   bash 5.3.20, dash
//	false; a=${ exit; };        echo $?         0         (no spelling / ends)
//	false; a=${ true; exit; };  echo $?         0         —
//	false; a=${ false; exit; }; echo $?         1         —
//	false; a=$( exit );         echo $?         0         1
//	false; a=$( false; exit );  echo $?         1         1
//	false; ( exit );            echo $?         0         1
//	g() { exit; }; false; g                     0         1
//	g() { return; }; false; g;  echo $?         0         1
//	false; eval exit                            0         1
//	false; . f   (f holding `return`); echo $?  0         1
//	false; { exit; }                            1         1
//	false; exit                                 1         1
//
// The last two rows are the controls and they are what make this a *unit*
// rather than a frame count: a brace group starts no unit, and neither does
// the top of the script — both report the 1 the `false` left. The `$?` row
// is the other control: `(exit 3); j=${ echo "saw=$?"; }` is `saw=3` in
// ksh93 too, so the body reads the inherited value out of `$?` while a bare
// `exit` on the next word reports 0. Two registers, not one.
//
// #3184 filed the `${ …;}` rows alone and read them as a property of that
// spelling. They are one case of six: the same reset happens at a subshell, a
// command substitution, a function call, an `eval` and a sourced file.

// enterExecutionUnit starts a unit whose bare `exit` and `return` report a
// status of its own, and hands back the call that ends it.
//
// The bool is cleared rather than a status being saved, because the answer for
// a unit that has run nothing is 0 in every row measured and not the status on
// the way in — which is the whole of what tells this register from `$?`.
func (r *Runner) enterExecutionUnit() func() {
	saved := r.unitRanACommand
	r.unitRanACommand = false
	return func() { r.unitRanACommand = saved }
}

// operandLessStatus is what a bare `exit` or `return` reports.
//
// Asked only where no operand was written, which is the only spelling that can
// tell the two readings apart: `exit 7` is 7 in every column.
func (r *Runner) operandLessStatus() int {
	// **Read rather than asked**, which is the rare shape and wants its
	// reason. A bare `exit` is the one word a Runner built with no vector at
	// all still has to obey — `driver.Shell{}` carries a zero Semantics and
	// its sessions end with it — so an unanswered axis here would refuse the
	// shell's own way out. The unanswered state therefore means the reading
	// six of the seven columns hold and the standard states, which is what
	// PosixSemantics and CoreSemantics write down explicitly; only the
	// column with a register of its own says otherwise.
	if r.sem().BareExitReportsTheUnitsOwnStatus != Yes {
		return r.status
	}
	if r.unitRanACommand {
		return r.status
	}
	return 0
}
