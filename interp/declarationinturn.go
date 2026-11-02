// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
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
// the measured answer. Inside a `function`-form body the reference's operands
// were locals, and the call ends there and takes them with it, while the shell
// carries on. They are held in the cell the name reached, which is the global
// one, so a stop there puts them back rather than keeping them — see
// operandsInTurn.stop (#7011).
//
// Every other column walks a declaration's **array literals** and not its
// scalar operands: each literal is expanded and traced in turn, the scalar
// words between them only expanded, and the utility then stores everything
// for real. Expanding in turn is every shell's answer, so a substitution in a
// literal runs between the words written either side of it (#7075). Where
// Semantics.DeclarationArrayOperandsAreStoredInTurn says so, each literal is
// placed in turn as well, by the operand's own store over a saved cell, so a
// refusal there — an element before the first, an empty key — is reported at
// the literal's turn, ahead of the utility's own line, and ends the walk with
// what was placed before it (#6798, #6852). See
// Runner.placeLiteralOperandInTurn.
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
	// stopped says a refusal or a failed value ended the walk: nothing more
	// is expanded, the utility does not run, and what was held stays.
	stopped bool
	// literals is each array-literal operand expanded ahead of a scalar
	// operand written after it, so the two are stored in written order; a
	// nil entry is one with nothing to expand.
	literals map[*syntax.Assign]*expandedAssign
	// traced is the literal operands whose lines were written here.
	traced map[*syntax.Assign]bool
	// literalsOnly is the walk that reaches a declaration's array-literal
	// operands and leaves its scalar ones to the utility: a scalar operand is
	// expanded in turn and neither held nor traced. Every column but the one
	// that stores every operand in turn walks this way. See
	// Semantics.DeclarationArrayOperandsAreStoredInTurn.
	literalsOnly bool
}

// commit keeps what the walk held: the undo list is dropped and release has
// nothing to put back.
func (t *operandsInTurn) commit() {
	t.held.undo = nil
	t.stopped = true
}

// stop ends the walk at a refusal or a failed value. What it held stays,
// because the column measured has stored it — except where the utility would
// have made the operands locals of the running function. The failure ends
// that call and the reference's locals go with it, so what was held in the
// cell the name reached is put back rather than left as a global: measured
// 2026-10-29 on ksh93u+, `function f { typeset x=1 y=${q?boom}; }; f; echo
// "out x=${x-unset}"` is `out x=unset` (#7011), and 2026-10-30 on bash 5.3.20,
// `a=(0); f(){ declare -a a=(5) b=(6 $((1/0))); }; f` leaves the global `a`
// holding 0 where `declare -ga` and `export` leave it 5 (#6798).
func (t *operandsInTurn) stop(r *Runner, argv []string) {
	if r.operandsAreLocalsOfTheCall(argv) {
		t.held.release(r)
	}
	t.commit()
}

// operandsAreLocalsOfTheCall reports whether the declaration utility argv
// names is about to make its operands locals of the running function: a
// `typeset`, `declare` or `local` where that declares a local, without the
// global letter. `export` and `readonly` write the name the call can see.
func (r *Runner) operandsAreLocalsOfTheCall(argv []string) bool {
	if len(r.scopes) == 0 {
		return false
	}
	k := traceDeclarationUtilityWord(argv)
	if k >= len(argv) {
		return false
	}
	switch argv[k] {
	case "typeset", "declare", "local":
	default:
		return false
	}
	for _, w := range argv[k+1:] {
		if w == "--" || len(w) < 2 || (w[0] != '-' && w[0] != '+') {
			break
		}
		if w[0] == '-' && strings.ContainsRune(w[1:], 'g') {
			return false
		}
	}
	return r.typesetDeclaresALocalHere()
}

// declarationOperandsStoredInTurn reads the axis. Read and not asked, because
// the question is put to every `typeset x=1` there is: a vector that leaves
// it unanswered gets the answer four of the five columns give, which is also
// what this engine did before the axis existed, rather than a refusal of the
// commonest declaration a script writes.
func (r *Runner) declarationOperandsStoredInTurn() bool {
	return r.sem().DeclarationOperandsAreStoredInTurn == Yes
}

// reachOperandInTurn is the walk arriving at a declaration operand, before
// its word is expanded: the literal operands written in front of it are
// settled first, so the word's expansion sees them stored. Reports whether the
// walk is on and goes on.
//
// The state is made on the first operand of a command in the column that
// walks this way and is nil everywhere else, so a command that is not such
// a declaration allocates nothing for it. Hands the state back, and reports
// whether the walk is on and goes on.
func (r *Runner) reachOperandInTurn(
	ctx context.Context,
	c *syntax.SimpleCmd,
	w *syntax.Word,
	argv []string,
	turn *operandsInTurn,
) (*operandsInTurn, bool) {
	if turn == nil {
		if turn = r.startOperandsInTurn(c, argv); turn == nil {
			return nil, false
		}
	}
	if turn.stopped {
		return turn, false
	}
	return turn, r.settleLiteralOperandsBefore(ctx, c, w, argv, turn)
}

// startOperandsInTurn is the walk's state for one command, or nil where
// nothing is walked: outside the column that stores every operand in turn, a
// command with no array-literal operand, since the literals are all the walk
// reaches there; or `local` — it is in the substrate's list of declaration
// utilities and one column has no such command, so nothing is about to store
// the operands and nothing is held. The same gate the trace's split uses, see
// Runner.traceOperandSplitsBefore.
func (r *Runner) startOperandsInTurn(c *syntax.SimpleCmd, argv []string) *operandsInTurn {
	all := r.declarationOperandsStoredInTurn()
	if !all && !aLiteralOperandIsWritten(c.Assigns) {
		return nil
	}
	if !r.declarationUtilityIsHere(argv) {
		return nil
	}
	turn := &operandsInTurn{literalsOnly: !all}
	turn.held.undo = turn.undoBuf[:0]
	return turn
}

// aLiteralOperandIsWritten reports whether a command carries an array-literal
// operand among its words.
func aLiteralOperandIsWritten(assigns []*syntax.Assign) bool {
	for _, a := range assigns {
		if a.Operand {
			return true
		}
	}
	return false
}

// literal is the element list expanded ahead for this operand, or nil.
func (t *operandsInTurn) literal(a *syntax.Assign) *expandedAssign {
	if t == nil {
		return nil
	}
	return t.literals[a]
}

// wasTraced reports whether this literal operand's lines were written by the
// walk.
func (t *operandsInTurn) wasTraced(a *syntax.Assign) bool {
	return t != nil && t.traced[a]
}

// release puts back what the walk held, where there was a walk, and then what
// the assignment prefix held beneath it while the operands expanded — see
// interp/prefixoperandorder.go. In that order, because an operand over a
// prefix's own name saved the prefix's value as the one to put back.
//
// A walk that stopped keeps both. The column with the walk stores the prefix
// ahead of the first operand, so the prefix is among what it had stored when
// the walk ended: measured 2026-10-29 on ksh93u+ from `-c` and a script file,
// `trap 'echo "v=$v x=$x"' EXIT; readonly r=0; v=old; v=new typeset x=$v
// r=2` writes `v=new x=new`, and `v=new typeset x=$v y=${q?boom}` the same
// (#6999). Inside a `function`-form body the prefix was held in the body's own
// local, so what is kept goes with the call: `function f { v=new typeset
// y=${q?boom}; }; v=old; f` leaves `old`, as ksh93u+ does. See
// interp/prefixdeclarationlocal.go (#7002). The operands were held in the
// global cell, and a stop there has already put them back (#7011). A walk
// over the literals alone stores no prefix, so a stop there keeps what it
// placed and puts the prefix back, as the end of the command does.
func (t *operandsInTurn) release(r *Runner, prefix *heldPrefix) {
	if t != nil {
		if t.stopped && !t.literalsOnly {
			prefix.undo = nil
			return
		}
		t.held.release(r)
	}
	prefix.release(r)
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
	if turn == nil || turn.stopped {
		return argv
	}
	if r.expandErr || r.givingUpAlready() {
		// The value would not expand. The operands before it are stored, in
		// the column measured, and the command is over.
		turn.stop(r, argv)
		return argv
	}
	if turn.literalsOnly {
		// A scalar operand is the utility's to store, in the column that
		// walks only its literals.
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
		turn.stop(r, argv)
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
			turn.stop(r, argv)
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

// declarationWithAssignmentWords is c where it is a declaration utility in the
// column that stores an operand written as an assignment ahead of the
// utility, and nil otherwise — the record Runner.operandWrittenAsAnAssignment
// reads.
func (r *Runner) declarationWithAssignmentWords(c *syntax.SimpleCmd, argv []string) *syntax.SimpleCmd {
	if !r.declarationOperandsStoredInTurn() || len(argv) == 0 || !r.declarationCommand(c, argv) {
		// Not a declaration utility, so a `name=value` word is an ordinary
		// argument: `let x=2` is the builtin's own operand and its refusal is
		// `let: x: is read only` (#3568).
		return nil
	}
	return c
}

// operandWrittenAsAnAssignment reports whether one of the running
// declaration's operands was **written** as an assignment to name — an array
// literal, or a word whose name and `=` stand unquoted in the source — in the
// column that stores such an operand ahead of the utility.
//
// In that column a refusal of such a name is the shell's own and names no
// builtin, wherever this engine makes it: measured 2026-10-31 on ksh93u+,
// `readonly b=1; function f { export b=2; }; f` is `f: line 1: b: is read
// only` and `set -r; function f { typeset PATH=x; }; f` is `f: line 1: PATH:
// restricted`, though the walk above leaves both to the utility because a
// `function` body may shadow. The same operand written as a word the utility
// reads, `export 'b=2'`, is `f[1]: export: b: is read only`. See
// Runner.readonlyRefusalNamesBuiltin (#7188).
//
// Read off the written words rather than off Runner.declarationOperands,
// because that record holds only the operands this engine expands as
// assignments, and ksh93's assignment words are wider: a chain,
// `typeset r[1+1][2]=z`, and a dotted name, `typeset .sh=1`, are refused there
// with no builtin named as well. Asked only by a refusal, so the words are
// read then and not for every declaration.
func (r *Runner) operandWrittenAsAnAssignment(name string) bool {
	c := r.assignmentWordsOf
	if c == nil {
		return false
	}
	if r.literalOperands[name] {
		return true
	}
	for i, w := range c.Args {
		if i == 0 {
			continue
		}
		if written, ok := writtenAssignmentName(w); ok && written == name {
			return true
		}
	}
	return false
}

// writtenAssignmentName is the name a word assigns to where it is written as
// an assignment: an unquoted name, dotted or not, then any number of
// subscripts, then an unquoted `=` or `+=`. Only the subscripts may hold
// quoting or an expansion; anything else in front of the `=` makes the word an
// ordinary one, whose text the utility reads for itself.
func writtenAssignmentName(w *syntax.Word) (string, bool) {
	if w == nil || len(w.Spans) == 0 {
		return "", false
	}
	head := w.Spans[0]
	if head.Kind != syntax.Literal || head.Quoting != syntax.Unquoted {
		return "", false
	}
	n := 0
	for n < len(head.Value) && (isNameByte(head.Value[n]) || head.Value[n] == '.') {
		n++
	}
	if n == 0 {
		return "", false
	}
	depth := 0
	for i, s := range w.Spans {
		if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
			if depth == 0 {
				return "", false
			}
			continue
		}
		from := 0
		if i == 0 {
			from = n
		}
		for j := from; j < len(s.Value); j++ {
			switch c := s.Value[j]; {
			case c == '[':
				depth++
			case c == ']' && depth > 0:
				depth--
			case depth > 0:
			case c == '=', c == '+' && j+1 < len(s.Value) && s.Value[j+1] == '=':
				return head.Value[:n], true
			default:
				return "", false
			}
		}
	}
	return "", false
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
// behind a literal sees it stored — and with w nil, every one still waiting,
// which is the walk reaching the literals written after the command's last
// word. Reports whether the walk goes on.
func (r *Runner) settleLiteralOperandsBefore(
	ctx context.Context,
	c *syntax.SimpleCmd,
	w *syntax.Word,
	argv []string,
	turn *operandsInTurn,
) bool {
	for _, a := range c.Assigns {
		if !a.Operand || (w != nil && !w.Pos().After(a.Pos())) {
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
		if a.Members != nil {
			// A compound body performs its members after the utility and
			// the command's line waits for them (see
			// interp/compoundoperandorder.go).
			continue
		}
		if turn.literalsOnly {
			if !r.settleALiteralOperand(ctx, a, argv, turn) {
				return false
			}
			continue
		}
		if r.nameIsFrozen(a.Name) && !r.declarationWouldShadow(a.Name) {
			r.refuseReadonly(a.Name, assignedByDeclaration)
			turn.stop(r, argv)
			return false
		}
		tableAhead := tableLetterAmongTheOptions(argv)
		e := &expandedAssign{assign: a, elemsAreOperands: true}
		// An empty literal has nothing to expand, and is held and written
		// where a populated one is: `a=(1); typeset -a a=() b=(${#a[@]})`
		// gives `b` the 0 in ksh93u+.
		parsed, ok := []literalElem(nil), true
		if len(a.Elems) > 0 {
			// The subscript's side first, words and arithmetic, as the
			// column that nests an element literal does — see
			// elementLiteralBuilt.
			r.resolveLiteralOperandSubscript(a, e)
			if r.unspecified || r.givingUpAlready() {
				turn.stop(r, argv)
				return false
			}
			parsed, ok = r.literalElems(a.Elems,
				r.literalReadsSubscripts(a.Name, a.Elems, a.Append),
				r.bareLiteralElementIsOneValue(a.Name, a.Elems, tableAhead))
		}
		if !ok {
			r.keepWhatAFailedLiteralOperandStored(a, argv, parsed, tableAhead)
			turn.stop(r, argv)
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

// keepWhatAFailedLiteralOperandStored is Runner.keepWhatAFailedLiteralStored
// for a declaration's operand, with the container letter read off the
// utility's words.
func (r *Runner) keepWhatAFailedLiteralOperandStored(
	a *syntax.Assign,
	argv []string,
	parsed []literalElem,
	tableAhead bool,
) {
	how := failedLiteral{appendTo: a.Append, operand: true}
	switch {
	case tableAhead:
		how.letter = 'A'
	case indexedLetterAmongTheOptions(argv):
		how.letter = 'a'
	}
	r.keepWhatAFailedLiteralStored(a.Name, a.Elems, parsed, how)
}

// settleALiteralOperand is one literal operand reached by the walk that
// reaches only the literals: refused there if the line refuses it, then
// expanded, traced, and placed — in that order, which is the order the
// reference writes them: `set -x; declare -a q=([$(true)]=x)` is the
// substitution's line, the literal's own, and then the refusal (#6852).
// Reports whether the walk goes on.
//
// The expansion is every column's, and the rest is the axis's. Where the
// literal is not placed in turn its refusal is the store's, after the
// utility, and its line is written with the command's where the trace puts it
// there; but its words are still expanded here, between the words written
// either side of it. Measured 2026-10-30 from script files under `env -i
// PATH=/usr/bin:/bin`, against GNU bash 5.3.20, bash 3.2.57, ksh93u+
// 2012-08-01 and zsh 5.9.2, every one of them:
//
//	typeset -a a=($(echo A >&2)) y=$(echo s >&2)               A, s
//	typeset y=$(echo s >&2) a=($(echo A >&2)) z=$(echo t >&2)  s, A, t
//
// zsh then refuses `y` as `inconsistent type for assignment`, after both.
// The literals used to be expanded after every word in that column, `s`
// before `A` (#7075).
func (r *Runner) settleALiteralOperand(
	ctx context.Context,
	a *syntax.Assign,
	argv []string,
	turn *operandsInTurn,
) bool {
	indexed, table := declarationLettersAhead(argv)
	placed := r.sem().DeclarationArrayOperandsAreStoredInTurn == Yes
	switch {
	case !placed:
		// Expanded in turn and stored by the utility, which is where a
		// refusal is made: nothing to ask here.
	case r.inFunc == "":
		// A frozen name, or one of the other kind of array, refused as a
		// bare assignment is, with the literals in front of it in place:
		// `readonly q=1; a=(0); declare -a a=(5) q=(b)` leaves `a` holding 5.
		// See interp/operandrefusedearly.go.
		if r.refuseArrayOperandEarly(a, indexed, table) || r.unspecified {
			turn.stop(r, argv)
			return false
		}
	case r.attrHas(a.Name, attrReadonly):
		// Inside a function the store answers, and the walk goes on past it:
		// a function's `local q=(b)` over a frozen global is the store's
		// complaint with the call carrying on.
		placed = false
	}
	tableAhead := tableLetterAmongTheOptions(argv)
	parsed, ok := []literalElem(nil), true
	if len(a.Elems) > 0 {
		// An empty literal has nothing to expand and is written and placed
		// all the same: `a=(1); declare -a a=() b=(${#a[@]})` gives `b` the 0.
		parsed, ok = r.literalElems(a.Elems,
			r.literalReadsSubscripts(a.Name, a.Elems, a.Append),
			r.bareLiteralElementIsOneValue(a.Name, a.Elems, tableAhead))
	}
	if !ok {
		r.keepWhatAFailedLiteralOperandStored(a, argv, parsed, tableAhead)
		turn.stop(r, argv)
		return false
	}
	e := &expandedAssign{assign: a, elemsAreOperands: true, elems: parsed, elemsSet: true}
	turn.literals[a] = e
	if r.tracing() && r.diag().TraceDeclarationArrayOperand == TraceOperandSplitBefore {
		// Off the command's line, where the trace writes it in front.
		turn.traced[a] = true
		r.writeArrayOperandLines(arrayOperand{assign: a, expanded: e}, tableAhead, r.diag())
	}
	if placed && !r.placeLiteralOperandInTurn(ctx, &turn.held, a, e, table || r.assocDeclared(a.Name), indexed) {
		turn.stop(r, argv)
		return false
	}
	return true
}

// placeLiteralOperandInTurn stores a literal operand where its name stands
// now, for the operands behind it to see, having saved the cell so that the
// walk's release puts it back and the utility makes the store that stays. The
// store is the operand's own, so what it refuses and what a refusal keeps are
// the same here as there. Reports whether the placement went through; a
// refusal has been reported.
//
// Placed only where the cell the walk would save is the one the store writes
// and the kind the letters are about to give it: a name that is not a tie or a
// produced parameter, and, under a letter for the other kind of array, one
// holding nothing yet. Anything else is left to the store alone, as it was
// before the walk.
func (r *Runner) placeLiteralOperandInTurn(
	ctx context.Context,
	h *heldPrefix,
	a *syntax.Assign,
	e *expandedAssign,
	table, indexed bool,
) bool {
	if a.Index != nil || len(a.Leading) > 0 {
		return true
	}
	// Through a reference the store lands on what it names, so that is the
	// cell saved — `declare -n ref=tgt; declare -a ref=(1 2) z=(${tgt[1]})`
	// gives `z` the 2 — and a reference aimed at nothing, round a loop or at
	// an element is left to the store.
	name := a.Name
	if r.isNameref(name) {
		target, cycle, aimed := r.namerefWalk(name)
		if !aimed || cycle || strings.ContainsRune(target, '[') {
			return true
		}
		name = target
	}
	if _, tied := r.tieOf(name); tied {
		return true
	}
	if _, produced := r.producersLookup(name); produced {
		return true
	}
	if _, produced := r.arrayProducersLookup(name); produced {
		return true
	}
	_, holds := r.getVar(name)
	holds = holds || r.nameIsAnArray(name)
	switch {
	case table && !r.assocDeclared(name) && holds:
		return true
	case indexed && !table && r.assocDeclared(name):
		return true
	}
	if !table {
		// Each subscript read once, here, so the placement and the store that
		// stays put an element where the same evaluation said: `i=0; declare
		// m=([i++]=x [i++]=y) w=$i` leaves `w` at 2 and `m` at 0 and 1.
		r.placeLiteralSubscriptsOnce(e.elems)
	}
	h.undo = append(h.undo, r.saveVar(name))
	if table && !r.assocDeclared(name) {
		r.markAssoc(name)
	}
	place := *e
	r.withPreparedValue(ctx, &place)
	return !r.expandErr && !r.givingUpAlready() && !r.unspecified
}

// placeLiteralSubscriptsOnce evaluates a literal's subscripts in order and
// records each position on its element, stopping at the first that does not
// come to a number: that one, and the rest, are left to the store, which owns
// the refusal.
func (r *Runner) placeLiteralSubscriptsOnce(elems []literalElem) {
	for i := range elems {
		el := &elems[i]
		if !el.subscripted || el.subPlaced {
			continue
		}
		if strings.TrimSpace(el.sub) == "" {
			return
		}
		idx, err := r.subscriptValue(el.sub)
		if err != nil {
			return
		}
		el.subPlaced, el.subAt = true, idx
	}
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
