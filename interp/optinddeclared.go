// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// KeepOptindThroughDeclarations says a declaration of OPTIND keeps the cell
// the shell gave it — its value and its integer attribute — where any other
// name's would be reset or made afresh.
//
// It exists for the one dialect whose OPTIND is an integer from the start
// and whose declarations otherwise reset a name's attributes (see
// Semantics.DeclarationAssignmentResetsTheAttributes). Measured 2026-10-15 on
// ksh93u+ 2012-08-01, under `-c` and `env -i PATH=/usr/bin:/bin`:
//
//	typeset OPTIND=3+4; typeset -p OPTIND         typeset -i OPTIND=7
//	export OPTIND=3+4 / readonly OPTIND=3+4       -x -i 7 / -r -i 7
//	typeset -Z3 OPTIND=5; typeset OPTIND=3+4      -Z 3 kept, value 3+4
//	function f { typeset OPTIND=x; echo $OPTIND; }; f
//	                                              0
//	function f { OPTIND=5; typeset OPTIND; echo $OPTIND; }; f
//	                                              5, the cell is kept
//	function f { unset OPTIND; typeset OPTIND; echo $OPTIND; }; f
//	                                              1, a cell like the startup one
//	unset OPTIND; function f { typeset OPTIND=3+4; }; f; echo ${OPTIND-unset}
//	                                              7 inside, unset after
//	control: typeset -i n=1; typeset n=3+4        3+4, the attribute reset
//
// So at the top a declaration over OPTIND leaves every attribute where it
// was, and in a call it declares a local that starts as the cursor the call
// already holds — integer, with the value OPTIND had there or the startup 1 —
// and is taken back at the return like any other local. `typeset +i
// OPTIND` and `unset OPTIND` at the top still take the attribute away, and a
// declaration after either is an ordinary one there (#6563).
func (r *Runner) KeepOptindThroughDeclarations() { r.optindKeptThroughDeclarations = true }

// shadowOptindAsTheCursor is shadowTypeset for OPTIND in the dialect that
// keeps it through a declaration: the local is made, and then given back the
// cell the call was holding, so the declaration reads as one over a name that
// already has a value.
func (r *Runner) shadowOptindAsTheCursor() (fresh, redeclared bool) {
	held, set := r.varsLookup("OPTIND")
	if r.attrHas("OPTIND", attrRemoved) {
		set = false
	}
	if _, redeclared = r.shadow("OPTIND"); redeclared {
		return false, true
	}
	if !set {
		held = "1"
	}
	r.attrSet("OPTIND", attrInteger)
	r.varsPut("OPTIND", held)
	r.attrClear("OPTIND", attrRemoved)
	r.optindCursorMadeHere = true
	return false, false
}

// optindDeclaredAsTheCursor is what the declaration that made a call's OPTIND
// local leaves once its letters have landed, where it gave no value: the
// call's cursor is an integer whatever the letters said, and a line that
// named no integer letter at all lists it with a base of 0 until the script
// assigns it. Measured 2026-10-15 on ksh93u+ 2012-08-01 under `-c`, each in
// `function f { …; }; f`:
//
//	typeset OPTIND; typeset -p OPTIND             typeset -i 0 OPTIND=1
//	OPTIND=5; typeset OPTIND; typeset -p OPTIND   typeset -i 0 OPTIND=5
//	typeset -x OPTIND / typeset -r OPTIND         typeset -x -i 0 … / -r -i 0
//	typeset OPTIND; getopts a o -a; typeset -p    typeset -i 0 OPTIND=2
//	typeset OPTIND; OPTIND=2+3; typeset -p        typeset -i OPTIND=5
//	typeset -i OPTIND; typeset -p OPTIND          typeset -i OPTIND=1
//	typeset +i OPTIND; OPTIND=x; echo $OPTIND     0, `typeset -i OPTIND=1`
//	typeset +i OPTIND=7; typeset -p OPTIND        OPTIND=7 — a value, so
//	                                              the letter stands
//	typeset OPTIND; typeset +i OPTIND             OPTIND=1 — a second line
//	typeset -i16 OPTIND=20; typeset +i OPTIND     OPTIND=20 — the same
//
// (#6633).
func (r *Runner) optindDeclaredAsTheCursor(df declareFlags, hasValue bool) {
	r.optindCursorMadeHere = false
	if hasValue || len(r.scopes) == 0 {
		return
	}
	switch {
	case !df.integer:
		r.scopes[len(r.scopes)-1].optindListsBaseZero = true
	case df.integerComesOff(r):
		r.attrSet("OPTIND", attrInteger)
	}
}

// optindCursorScope is the scope whose local OPTIND a listing and an
// assignment reach: the innermost one holding it, and none past a
// `function`-form call that does not, whose body reads past its callers'.
func (r *Runner) optindCursorScope() *scope {
	for i := len(r.scopes) - 1; i >= 0; i-- {
		sc := r.scopes[i]
		if sc.shadows("OPTIND") {
			return sc
		}
		if sc.keyword {
			return nil
		}
	}
	return nil
}

// KeepThroughDeclarations is KeepOptindThroughDeclarations's first half for
// another of the shell's own integer names: a declaration at the top leaves
// its attributes where they were, so `typeset PPID=7+1` still evaluates. In a
// call a declaration makes an ordinary fresh local, which is where these part
// from OPTIND, whose local starts as the call's cursor.
//
// Measured 2026-10-15 on ksh93u+ 2012-08-01 under `-c`:
//
//	typeset PPID=7+1; typeset -p PPID           typeset -i PPID=8
//	typeset TMOUT=2+3 / MAILCHECK=2+3           typeset -i … =5
//	export MAILCHECK=2+3                        typeset -x -i MAILCHECK=5
//	typeset -Z3 TMOUT=5; typeset TMOUT=3+4      typeset -Z 3 -R 3 TMOUT=3+4
//	function f { typeset MAILCHECK=2+3; …; }    MAILCHECK=2+3, no attribute
//	typeset +i TMOUT; typeset -p TMOUT          TMOUT=0, the attribute gone
//
// (#6633).
func (r *Runner) KeepThroughDeclarations(name string) {
	if r.declarationKeeps == nil {
		r.declarationKeeps = map[string]bool{}
	}
	r.declarationKeeps[name] = true
}

// optindKeptThroughThis reports whether a declaration's reset passes name by.
func (r *Runner) optindKeptThroughThis(name string) bool {
	return r.optindKeptThroughDeclarations && name == "OPTIND" || r.declarationKeeps[name]
}

// namesANumericLetter reports whether the line writes a letter that gives a
// value a numeric shape: the integer, a float, or a field width.
func (f declareFlags) namesANumericLetter() bool {
	return f.integer || f.float || f.widthLetter != 0
}
