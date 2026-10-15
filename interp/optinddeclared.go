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
	return false, false
}

// optindKeptThroughThis reports whether a declaration's reset passes name by.
func (r *Runner) optindKeptThroughThis(name string) bool {
	return r.optindKeptThroughDeclarations && name == "OPTIND"
}

// namesANumericLetter reports whether the line writes a letter that gives a
// value a numeric shape: the integer, a float, or a field width.
func (f declareFlags) namesANumericLetter() bool {
	return f.integer || f.float || f.widthLetter != 0
}
