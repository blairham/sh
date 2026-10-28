// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A declaration utility's operands, expanded and stored **one at a time**.
//
// `typeset x=1 y=$x` is one command with two operands, and this engine handed
// the utility both of them expanded — so `y` read the `x` the command started
// with. One column stores each operand as it reaches it, and the operand
// behind it sees the store: the value, the trace line and any warning the store
// makes all land before the next operand's own expansion runs, and the utility
// then runs over the bare names, which is what its own trace line shows.
// Measured 2026-10-27 on ksh93u+ 2012-08-01 from a script file, against bash
// 5.3.20, bash 3.2.57, zsh 5.9.2 and dash 0.5.12 on the same machine:
//
//	                                              ksh93u+          the rest
//	x=0; typeset x=1 y=$x; echo $y                1                0
//	export a=1 b=$a; echo $b                      1                (empty)
//	readonly c=1 d=$c; echo $d                    1                (empty)
//	typeset -A a; typeset a[k]=1 b=${a[k]}        b=1
//	typeset a=(1 2) x=${a[1]}                     x=2
//	typeset y=7 b=($y)                            b=( 7 )
//	typeset -i x=1+1 y=$x                         `+ y=1+1`, and 2 after
//	function f { typeset x=1 y=$x; }              both local
//	set -x; typeset x=1 a[""]=2 y=$(echo s >&2) a[k]=3
//	        `+ x=1`, the warning, `+ a['']=2`, the substitution's lines,
//	        `+ y=''`, `+ a[k]=3`, `+ typeset x a y a`
//	set -x; typeset x=1 y=${q?boom} z=2            `+ x=1`, then the complaint
//	readonly r=0; set -x; typeset x=1 r=2 y=3     `+ x=1`, `r: is read only`,
//	                                              and an EXIT trap reads x=1
//
// So the operand is stored as a plain assignment — the letters land afterwards,
// which is why `y` sees `1+1` and not `2` — and a store that is refused, or a
// value that will not expand, ends the command with the operands before it
// stored and the utility never run. The subscript is written as it resolved
// and quoted as a word, the way a bare element assignment's is in that column
// (#6582): `typeset q[1+1]=x` is `+ q[2]=x` and `typeset m[$k]=v` with `k='x
// y'` is `+ m['x y']=v`. A literal operand's subscript is written the same
// way and **unquoted** — `typeset m[$k]=(p)` is `+ m[x y]=( p )` — and a
// chain is written as the script spelled it, `+ f[1][2]=( p q )` (#6876).
//
// Here the value is held under the name while the operands behind it expand
// and the utility then makes the store for real, in the scope its letters
// decide — the arrangement interp/prefixsees.go uses for a prefix, and for the
// same reason: `typeset` inside a function makes the name local, and only the
// utility knows that. Two things follow from holding rather than storing. A
// warning the held store makes is one the real store would make again, so the
// real one is owed and swallowed (Runner.emptyKeyWarningsOwed). And an
// indexed operand's subscript is evaluated here, so the word the utility reads
// is rewritten to the position it came to — `typeset q[i++]=x` steps `i` once
// and reaches the utility as `q[1]=x`, exactly the line that column traces.
//
// When the walk stops — a frozen name, a value that would not expand — what
// was held stays, because that column has stored it: at the top level that is
// the measured answer. Inside a function the shell is on its way out either
// way, and a held value becomes a global where the reference's was a local the
// exit took with it; an EXIT trap is the one thing that can tell, and it is not
// modeled.
//
// See Semantics.DeclarationOperandsAreStoredInTurn (#6770).

// operandsInTurn is the state of one command's operand walk.
type operandsInTurn struct {
	// held is what each operand came to, visible to the operands behind it
	// and put back before the utility runs; undoBuf is its first few entries,
	// on the command's own frame, so a short declaration allocates nothing
	// for them.
	held    heldPrefix
	undoBuf [4]savedVar
	// on says the dialect stores its operands this way and the walk has
	// begun; stopped says a refusal or a failed value ended it: nothing more
	// is expanded, the utility does not run, and what was held stays.
	on, stopped bool
	// literals is each array-literal operand expanded ahead of a scalar
	// operand written after it, so the two are stored in written order; a
	// nil entry is one with nothing to expand.
	literals map[*syntax.Assign]*expandedAssign
	// traced is the literal operands whose lines were written here.
	traced map[*syntax.Assign]bool
}

// commit keeps what the walk held: the undo list is dropped and release has
// nothing to put back.
func (t *operandsInTurn) commit() {
	t.held.undo = nil
	t.stopped = true
}

// declarationOperandsStoredInTurn asks the axis, of a command that has a
// declaration operand to ask it about.
func (r *Runner) declarationOperandsStoredInTurn() bool {
	return r.ask(r.sem().DeclarationOperandsAreStoredInTurn,
		"a declaration utility's operands expanded and stored one at a time")
}

// reachOperandInTurn is the walk arriving at a declaration operand, before
// its word is expanded: the literal operands written in front of it are
// settled first, so the word's expansion sees them stored. Reports whether the
// walk is on and goes on.
func (r *Runner) reachOperandInTurn(c *syntax.SimpleCmd, w *syntax.Word, argv []string, turn *operandsInTurn) bool {
	if !turn.on && !r.declarationUtilityIsHere(argv) {
		// `local` is in the substrate's list of declaration utilities and
		// this column has no such command: nothing is about to store the
		// operands, so nothing is held — the same gate the trace's split
		// uses, see Runner.traceOperandSplitsBefore.
		return false
	}
	if !r.declarationOperandsStoredInTurn() || r.unspecified {
		return false
	}
	if !turn.on {
		turn.on = true
		turn.held.undo = turn.undoBuf[:0]
	}
	return r.settleLiteralOperandsBefore(c, w, argv, turn)
}

// declarationWouldShadow reports whether the utility is about to give this
// name a binding of its own in the running function's scope, which is what
// lets a declaration over a frozen name through: `readonly x=1; function f {
// typeset x=2; }` holds 2 inside the call in ksh93 (#1177). The same questions
// shadowTypeset and declarationShadowRefused ask, so the walk and the utility
// cannot come to disagree about which names are refused.
func (r *Runner) declarationWouldShadow(name string) bool {
	if len(r.scopes) == 0 {
		return false
	}
	sc := r.scopes[len(r.scopes)-1]
	if !sc.keyword && r.ask(r.sem().TypesetLocalNeedsKeywordFunction,
		"`typeset` needing a keyword-defined function to declare a local") {
		return false
	}
	if sc != nil && sc.shadowsValue(name) {
		return false
	}
	return r.sem().DeclarationMayShadowAReadonly == Yes
}

// storeOperandInTurn holds the declaration operand just appended to argv under
// its name and writes its trace line. argv is handed back with the operand
// rewritten where its subscript was evaluated.
func (r *Runner) storeOperandInTurn(argv []string, turn *operandsInTurn) []string {
	if !turn.on || turn.stopped {
		return argv
	}
	if r.expandErr || r.givingUpAlready() {
		// The value would not expand. The operands before it are stored, in
		// the column measured, and the command is over.
		turn.commit()
		return argv
	}
	word := argv[len(argv)-1]
	name, value, hasValue, appends := r.declarationOperand(word)
	if !hasValue || name == "" {
		return argv
	}
	base, sub, subscripted := operandSubscriptOf(name)
	if !subscripted {
		base = name
	}
	if r.nameIsFrozen(base) && !r.declarationWouldShadow(base) {
		// Refused at the store, as the bare assignment is, and the walk
		// ends: measured, `+ x=1` and then `r: is read only`, with no line
		// for the utility and nothing after it run.
		r.refuseReadonly(base, assignedByDeclaration)
		turn.commit()
		return argv
	}
	d := r.diag()
	op := "="
	if appends {
		op = "+="
	}
	switch {
	case !subscripted:
		held := value
		if appends {
			old, _ := r.getVar(name)
			held = old + value
		}
		turn.held.hold(r, name, held)
		if r.tracing() {
			r.traceOperandInTurn(name+op, value, d)
		}
	case r.assocDeclared(base):
		key := r.declarationSubscriptText(base, sub)
		if r.unspecified {
			return argv
		}
		held := value
		if appends {
			held = r.assocsAt(base)[key].scalar() + value
		}
		turn.held.undo = append(turn.held.undo, r.saveVar(base))
		r.holdingAnOperand = true
		r.setAssocElem(base, key, held)
		r.holdingAnOperand = false
		r.oweTheEmptyKeyWarning(key)
		if r.tracing() {
			r.traceOperandInTurn(base+"["+r.traceQuote(key, d.TraceQuoting, d.TraceMetacharacters)+"]"+op, value, d)
		}
	default:
		idx, err := r.subscriptValue(sub)
		if err != nil {
			// Refused at the store, in the bare assignment's words, and the
			// walk ends: measured, `typeset a["p q"]=1 a[""]=3` over an
			// undeclared `a` is `p q: arithmetic syntax error` and nothing
			// more.
			r.fatal("%s\n", r.subscriptFailure(sub, err))
			turn.commit()
			return argv
		}
		held := value
		if appends {
			if elems, ok := r.arrayElemsOfTheName(base); ok && idx >= 0 && idx < len(elems) {
				held = elems[idx] + value
			}
		}
		turn.held.undo = append(turn.held.undo, r.saveVar(base))
		r.setArrayElem(base, idx, sub, held)
		at := itoa(idx)
		if r.tracing() {
			r.traceOperandInTurn(base+"["+at+"]"+op, value, d)
		}
		// The position it came to, so the utility does not evaluate the
		// subscript a second time.
		argv[len(argv)-1] = base + "[" + at + "]" + op + value
	}
	return argv
}

// operandSubscriptOf splits a declaration operand's name into the base and
// the text between its one pair of brackets.
func operandSubscriptOf(name string) (base, sub string, ok bool) {
	base, rest, found := strings.Cut(name, "[")
	if !found || base == "" || !strings.HasSuffix(rest, "]") {
		return name, "", false
	}
	return base, rest[:len(rest)-1], true
}

// oweTheEmptyKeyWarning notes that a store under the empty key is about to
// be made twice — held here, then made for real by the utility — so the
// second warning is swallowed. See Runner.warnEmptyTableKeyTraced.
func (r *Runner) oweTheEmptyKeyWarning(key string) {
	if key == "" && r.diag().EmptyTableKeyTraced != "" && r.tracing() {
		r.emptyKeyWarningsOwed++
	}
}

// traceOperandInTurn writes one operand's line as it is held, and marks the
// command's operands as traced so the command's own line does not write
// them again.
func (r *Runner) traceOperandInTurn(target, value string, d *Diagnostics) {
	r.operandsTracedInTurn = true
	r.awaitTraceTurn()
	r.traceLine(target+r.traceQuote(value, d.TraceQuoting, d.TraceMetacharacters), d)
	r.releaseTraceTurn()
}

// settleLiteralOperandsBefore expands, holds and traces every array-literal
// operand written in front of the word just expanded, so a scalar operand
// behind a literal sees it stored. Reports whether the walk goes on.
func (r *Runner) settleLiteralOperandsBefore(c *syntax.SimpleCmd, w *syntax.Word, argv []string, turn *operandsInTurn) bool {
	for _, a := range c.Assigns {
		if !a.Operand || !w.Pos().After(a.Pos()) {
			continue
		}
		if _, done := turn.literals[a]; done {
			continue
		}
		if turn.literals == nil {
			turn.literals = map[*syntax.Assign]*expandedAssign{}
			turn.traced = map[*syntax.Assign]bool{}
		}
		turn.literals[a] = nil
		if a.Members != nil || len(a.Elems) == 0 {
			// A compound body performs its members after the utility and
			// the command's line waits for them (see
			// interp/compoundoperandorder.go); an empty literal has nothing
			// to expand and nothing to write.
			continue
		}
		if r.nameIsFrozen(a.Name) && !r.declarationWouldShadow(a.Name) {
			r.refuseReadonly(a.Name, assignedByDeclaration)
			turn.commit()
			return false
		}
		tableAhead := tableLetterAmongTheOptions(argv)
		e := &expandedAssign{assign: a, elemsAreOperands: true}
		// The subscript's side first, words and arithmetic, as the column
		// that nests an element literal does — see elementLiteralBuilt.
		r.resolveLiteralOperandSubscript(a, e)
		if r.unspecified || r.givingUpAlready() {
			turn.commit()
			return false
		}
		parsed, ok := r.literalElems(a.Elems,
			r.literalReadsSubscripts(a.Name, a.Elems, a.Append),
			r.bareLiteralElementIsOneValue(a.Name, a.Elems, tableAhead))
		if !ok {
			how := failedLiteral{appendTo: a.Append, operand: true}
			switch {
			case tableAhead:
				how.letter = 'A'
			case indexedLetterAmongTheOptions(argv):
				how.letter = 'a'
			}
			r.keepWhatAFailedLiteralStored(a.Name, a.Elems, parsed, how)
			turn.commit()
			return false
		}
		e.elems, e.elemsSet = parsed, true
		turn.literals[a] = e
		r.holdLiteralOperand(&turn.held, a, e, parsed)
		if r.tracing() {
			turn.traced[a] = true
			r.operandsTracedInTurn = true
			r.writeArrayOperandLines(arrayOperand{assign: a, expanded: e}, tableAhead, r.diag())
		}
	}
	return true
}

// resolveLiteralOperandSubscript reads an element literal's subscript once,
// ahead of the store, and records what it came to so that the line can show
// it and the store need not read it again: `typeset f[1+1]=(p)` is `+ f[2]=(
// p )` and `typeset f[i++]=(p)` steps `i` once.
func (r *Runner) resolveLiteralOperandSubscript(a *syntax.Assign, e *expandedAssign) {
	if a.Index == nil {
		return
	}
	if r.assocDeclared(a.Name) {
		key, ok := r.assocAssignKey(a.Name, a.Index)
		if !ok {
			return
		}
		e.subscriptAhead, e.subscriptAheadSet = key, true
		return
	}
	text := r.joinWord(a.Index)
	e.subscriptAhead, e.subscriptAheadSet = text, true
	if r.givingUpAlready() {
		return
	}
	idx, err := r.subscriptValueAsWritten(subscriptSubject(r.writtenIndex(a), text), text)
	if err != nil {
		// The store reports it, from the same text.
		return
	}
	e.indexAhead, e.indexAheadSet = idx, true
}

// holdLiteralOperand holds what an array-literal operand places, for the
// operands behind it. A literal that nests into an element is held only where
// its own elements are plain words; everything else is left to the store.
func (r *Runner) holdLiteralOperand(h *heldPrefix, a *syntax.Assign, e *expandedAssign, parsed []literalElem) {
	if a.Index == nil {
		if len(a.Leading) > 0 || a.Append {
			return
		}
		r.holdPrefixLiteral(h, a, parsed)
		return
	}
	if len(a.Leading) > 0 {
		return
	}
	for _, el := range parsed {
		if el.subscripted || el.nested != nil || el.members != nil {
			return
		}
	}
	built, ok := r.literalInto(a.Name, NewArray(0), 0, parsed, false)
	if !ok {
		return
	}
	if built == nil {
		built = NewArray(0)
	}
	value := Element{Nested: built}
	switch {
	case r.assocDeclared(a.Name) && e.subscriptAheadSet:
		h.undo = append(h.undo, r.saveVar(a.Name))
		r.holdingAnOperand = true
		r.storeAssocElement(a.Name, e.subscriptAhead, value)
		r.holdingAnOperand = false
		r.oweTheEmptyKeyWarning(e.subscriptAhead)
	case e.indexAheadSet && e.indexAhead >= r.arrayBase():
		h.undo = append(h.undo, r.saveVar(a.Name))
		r.storeArrayElement(a.Name, e.indexAhead, e.subscriptAhead, value, a.Append)
	}
}

// holdPrefixLiteral holds a whole-array literal the way a prefix's is held:
// the words it came to, or the array or table its subscripts place.
func (r *Runner) holdPrefixLiteral(h *heldPrefix, a *syntax.Assign, parsed []literalElem) {
	var values []string
	for _, el := range parsed {
		if el.subscripted || el.nested != nil || el.members != nil {
			r.holdPlacedLiteral(h, a, a.Name, parsed)
			return
		}
		values = append(values, el.fields...)
	}
	h.holdArray(r, a.Name, values)
}

// writeArrayOperandLines writes one array-literal operand's trace lines, with
// the warning its store under an empty key would make in front of that
// element's own line, in the column where the store comes first.
func (r *Runner) writeArrayOperandLines(op arrayOperand, tableAhead bool, d *Diagnostics) {
	lines, perElement := r.traceArrayOperandLines(op, d)
	if len(lines) == 0 {
		return
	}
	inTurn := r.sem().DeclarationOperandsAreStoredInTurn == Yes
	table := tableAhead || r.assocDeclared(op.assign.Name)
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	for i, line := range lines {
		if inTurn && perElement && table && op.expanded.elems[i].sub == "" {
			r.writeEmptyTableKeyWarning()
			r.oweTheEmptyKeyWarning("")
		}
		r.traceLine(line, d)
	}
}

// subscriptReadAhead is an element literal operand's subscript as its words
// expanded, where the walk read it ahead of the store, and false everywhere
// else. See Runner.resolveLiteralOperandSubscript.
func (r *Runner) subscriptReadAhead(a *syntax.Assign) (string, bool) {
	e := r.expanded
	if e == nil || e.assign != a || !e.subscriptAheadSet {
		return "", false
	}
	return e.subscriptAhead, true
}

// indexReadAhead is the position that subscript evaluated to, where the walk
// evaluated it, and false everywhere else.
func (r *Runner) indexReadAhead(a *syntax.Assign) (int, bool) {
	e := r.expanded
	if e == nil || e.assign != a || !e.indexAheadSet {
		return 0, false
	}
	return e.indexAhead, true
}

// declarationUtilityIsHere reports whether the utility argv names is one this
// shell has, past the words that only say where to look for it.
func (r *Runner) declarationUtilityIsHere(argv []string) bool {
	k := traceDeclarationUtilityWord(argv)
	if k >= len(argv) {
		return false
	}
	_, ok := r.lookupBuiltin(argv[k])
	return ok
}
