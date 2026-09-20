// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A `-p` listing whose own operand carries a value.
//
// `typeset -p s=5` is one command that declares and shows in one line, and
// what it shows is what it has just stored — so the listing has to run *after*
// this command's own assignment. This engine ran it before: the operand
// assignments a declaration utility is given land in Runner.assignOperands,
// which runs after callBuiltin so that the declaration decides the scope the
// value goes into (see interp/arrayoperandorder.go), and the listing read the
// name while it was still the name the shell had before the line.
//
// Measured 2026-09-20, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> z.sh` over
// a script file, standard input on the null device, AT&T ksh93u+ 2012-08-01
// (`/bin/ksh`), bash 5.3.20 and zsh 5.9.2 at `/opt/homebrew`:
//
//	written                 ksh93u+                 bash 5.3.20                        zsh 5.9.2
//	typeset -p s=5          `s=5`, s is 5           `s=5: not found` at 1, s unset     `no such variable: s` at 1, s unset
//	typeset -p e=(1 2)      `typeset -a e=(1 2)`    `declare -a e=([0]="1" [1]="2")`   `no such variable: e` at 1, e unset
//	typeset -p c=(x=1)      `typeset -C c=(x=1)`    — no compound variable             `no such variable: c`
//	typeset -p q[2]=7       `typeset -a q=([2]=7)`  `q[2]=7: not found`, q unset       `no such variable: q`
//
// dash and BusyBox ash have neither word, so the question cannot be put to
// them.
//
// So the panel gives three answers and this file is the axis that records
// them — see Semantics.DeclarePrintPerformsItsOperand.
//
// **ksh93 performs a plain assignment**, and every attribute letter on the
// line is dropped. Measured the same day, each row read back with a second
// `typeset -p` on the next line:
//
//	typeset -ip n=3       `n=3`, and `n=4+4` leaves the four characters — no `-i`
//	typeset -rp z=1       `z=1`, and `z=2` goes through — no `-r`
//	typeset -xp w=9       `w=9`, listed back with no `-x`
//	f(){ typeset -p l=7;}  `l` is 7 after `f` returns — no local
//	readonly rr=1; typeset -p rr=2   `rr: is read only` and the script ends
//
// The last row is the one that names the form: that sentence carries no
// builtin name, which is what a *bare* assignment's refusal looks like there
// — `typeset rr=2` says `typeset: rr: is read only`. So this is assignedAlone
// and not assignedByDeclaration.
//
// **bash performs the operand only where the parser kept it apart as an array
// literal**, and there it is a declaration rather than a bare store:
//
//	declare -ip ni=(3+3)  `declare -ai ni=([0]="6")` — the `-i` landed
//	declare -Ap m=([k]=v) `declare -A m=([k]="v" )` — the `-A` landed
//	declare -lp a=(AB Cd) `declare -al a=([0]="ab" [1]="cd")`
//	declare -rp ar=(1 2)  `declare -a ar=(…)`, and `ar[0]=9` goes through — no `-r`
//	declare -xp a4=(1 2)  listed back with no `-x`
//	h(){ declare -p l=(7 8);}  `l` is unset after `h` returns — a local
//
// So the letters that shape the *value* land and the two that do not — `-r`
// and `-x` — are dropped, which is why the flags this file hands the
// declaration are the line's with those two cleared rather than the line's
// whole.
//
// **The stores all run before any of the listings**, in both columns, and the
// same probe says so in each: `typeset -p c=1 c=2` writes `c=2` twice in
// ksh93u+ and `declare -p c=(1 2) c=(3 4)` writes the `(3 4)` row twice in
// bash 5.3.20. A listing per operand would have written the first value once.
// That is what makes the listing something to *hold* until the command's
// operands have landed rather than something to move a few lines up.
//
// **zsh performs nothing**, and is where this axis's third value comes from:
// every row above is `no such variable` there at status 1 with the name still
// unset. It is also the reading this engine had for all three dialects, which
// is why it is the value a dialect that has not answered the axis is left on
// once the refusal below has been asked.
//
// Two measured rows are deliberately **not modeled**, and they are written
// down here rather than left to be rediscovered:
//
//   - ksh93 performs the operand only where the word was *written* as an
//     assignment. `v1=ab; typeset -p "v1+=cd"` leaves `v1` at `ab` and
//     `typeset -p "x1[0]=q"` stores nothing, where the same words unquoted do
//     both. Nothing reaches this builtin that can tell the two apart — the
//     parser hands a scalar operand over as a plain word either way — and the
//     same quoting makes no difference at all without `-p`: `typeset "v2=7"`
//     sets `v2` in ksh93u+ exactly as `typeset v2=7` does.
//   - an appended operand is performed and then listed as nothing.
//     `typeset -p t1+=cd` leaves `t1` at `cd` and writes no row, and
//     `u1=ab; typeset -p u1+=cd u1` writes one row, from the second operand.
//     The append is performed here for that reason and its name is left out
//     of the listing.
type heldListing struct {
	// names is what the listing will walk, in the order the operands were
	// written — the whole operand word in the column that reads one as a
	// name, and the assigned variable's own name in the column that performs
	// it.
	names []string
	// builtin is the word the listing names itself by when it reports a name
	// that is not there. Carried because the listing runs after callBuiltin
	// has put Runner.inBuiltin back, and a missing-name report with an empty
	// name in front of it is a line no shell writes.
	builtin string
	// held says the listing is waiting even where it has no names to walk,
	// so that an empty operand list is not read as no listing at all.
	held bool
}

// declarePrintPerformsItsOperands performs a `-p` listing's own operands where
// the dialect says it does, and holds the listing back until this command's
// operand assignments have landed.
//
// It answers whether it took the line. A false comes back for every shape the
// axis has nothing to say about — no operand carries a value and none is an
// array literal the parser kept apart — which is the ordinary `typeset -p
// name` every script writes, and for the dialect whose reading is that an
// operand is a name.
func (r *Runner) declarePrintPerformsItsOperands(name string, args []string, f declareFlags) (int, bool) {
	literals, values := false, false
	for _, a := range args {
		if r.literalOperands[a] {
			literals = true
			continue
		}
		if _, _, hasValue, _ := declarationOperand(a); hasValue {
			values = true
		}
	}
	if !literals && !values {
		return 0, false
	}
	switch r.sem().DeclarePrintPerformsItsOperand {
	case DeclarePrintOperandUnspecified:
		// Refused rather than guessed at: the three readings differ over
		// whether the name is set at all afterwards, which is the kind of
		// difference no later command can report.
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("an operand carrying a value on a `-p` listing")))
		r.status, r.unspecified = 2, true
		return r.status, true
	case DeclarePrintOperandIsANameAlone:
		return 0, false
	case DeclarePrintOperandIsDeclaredWhereItIsALiteral:
		if !literals {
			// Every other operand shape is a name in this column, whatever
			// it is holding: `declare -p s=5` and `declare -p q[2]=7` are
			// both `not found` at 1 with nothing stored.
			return 0, false
		}
		if code, done := r.declarePrintDeclaresItsLiterals(name, args, f); done {
			return code, true
		}
	case DeclarePrintOperandIsAssignedPlainly:
		if code, done := r.declarePrintAssignsItsOperands(args); done {
			return code, true
		}
		args = plainlyAssignedListingNames(args, r.literalOperands)
	}
	r.holdTheListing(args)
	return 0, true
}

// declarePrintDeclaresItsLiterals runs the declaration for the array-literal
// operands alone, which is the only shape the column that reads them this way
// performs.
//
// The declaration is the same function this one was called from, with the
// print letter off — not a second copy of it. The operand loop above decides
// the shadow, the attribute, the compound kind and the refusals in an order
// that is measured step by step, and a shorter path written beside it would be
// the second helper that omits what the first one carries.
func (r *Runner) declarePrintDeclaresItsLiterals(name string, args []string, f declareFlags) (int, bool) {
	var declared []string
	for _, a := range args {
		if r.literalOperands[a] {
			declared = append(declared, a)
		}
	}
	g := f
	g.print = false
	// The two letters measured not to land — see the rows at the top of this
	// file. Cleared here rather than inside the loop because they are this
	// *spelling's* answer and not the loop's: `declare -r ar=(1 2)` without
	// the print letter freezes `ar` in the same shell.
	g.readonly, g.readonlyOff = false, false
	g.export, g.exportForced = false, false
	if code := r.declareNames(name, declared, g); code != 0 {
		return code, true
	}
	if r.unspecified || r.ctl == controlExit {
		return r.status, true
	}
	return 0, false
}

// declarePrintAssignsItsOperands performs each operand that carries a value as
// a bare assignment, which is the whole of what the column reading them that
// way does with the line's letters: nothing.
func (r *Runner) declarePrintAssignsItsOperands(args []string) (int, bool) {
	for _, a := range args {
		if r.literalOperands[a] {
			// The parenthesized operands are landed by
			// Runner.assignOperands after this builtin returns, carrying
			// none of the line's letters — this loop never recorded one —
			// which is the same bare store the words below make.
			continue
		}
		base, value, hasValue, appends := declarationOperand(a)
		if !hasValue || !isPlainName(subscriptedOperandBase(base)) {
			// Not a name this shell would take, so nothing is performed and
			// nothing is left to list: measured 2026-09-20, `typeset -p 'bad
			// name=1'` is a silent 0 in ksh93u+ with no such parameter
			// afterwards. The check is on the *base*, which is what makes
			// `q[2]=7` a store to `q` and `1x[0]=v` nothing at all — the same
			// reading Runner.builtinNames makes of the same two shapes.
			continue
		}
		switch name, subs, subscripted := r.operandSubscripts(r.inBuiltin, base); {
		case subscripted:
			r.declareElement(name, subs[:len(subs)-1], subs[len(subs)-1], value, appends, declareFlags{}, true)
		case appends:
			// Joined through the same two steps a declaration's append takes
			// — the compound first, then the scalar through appendedValue,
			// which is what makes `typeset -i` add rather than concatenate.
			// Stored as a bare assignment rather than as a declaration's,
			// which is the only thing this spelling changes.
			if !r.appendOverCompound(base, value) {
				v, ok := r.appendedScalar(base, value, false)
				if !ok {
					break
				}
				r.setVarAs(base, v, assignedAlone)
			}
		default:
			r.setVarAs(base, value, assignedAlone)
		}
		if r.unspecified || r.ctl == controlExit {
			return r.status, true
		}
	}
	return 0, false
}

// plainlyAssignedListingNames is what the held listing walks in the column
// that performs its operands: the variable each assignment reached, and the
// operand itself where it reached none.
//
// An appended operand contributes no name at all, which is measured rather
// than a simplification — see the rows at the top of this file.
func plainlyAssignedListingNames(args []string, literals map[string]bool) []string {
	names := make([]string, 0, len(args))
	for _, a := range args {
		if literals[a] {
			names = append(names, a)
			continue
		}
		base, _, hasValue, appends := declarationOperand(a)
		switch {
		case !hasValue:
			names = append(names, a)
		case appends:
		default:
			names = append(names, subscriptedOperandBase(base))
		}
	}
	return names
}

// subscriptedOperandBase is the variable a subscripted assignment operand
// wrote, which is the name its listing is about: `typeset -p w1[0]=q` writes
// the whole `typeset -a w1=(q)` row, where a bare `typeset -p w1[0]` writes
// nothing at all.
func subscriptedOperandBase(name string) string {
	if i := strings.IndexByte(name, '['); i > 0 {
		return name[:i]
	}
	return name
}

// holdTheListing keeps a `-p` listing back until this command's own operand
// assignments have landed. See Runner.listingHeldForItsOperands, which is the
// other end and stands beside Runner.assignOperands.
// An empty name list is not held at all: a `-p` walk with no names is the
// whole table, and the one shape that reaches this with none — a line whose
// every operand appended — writes nothing there.
func (r *Runner) holdTheListing(names []string) {
	if len(names) == 0 {
		return
	}
	r.heldListing = heldListing{names: names, builtin: r.inBuiltin, held: true}
}

// listingHeldForItsOperands writes a listing that was held back, now that the
// command's operand assignments have landed, and answers its status.
//
// The builtin's own name is put back for the length of it, because a missing
// name is reported under the word the script wrote and Runner.inBuiltin has
// been restored by the time this runs.
func (r *Runner) listingHeldForItsOperands() (int, bool) {
	held := r.heldListing
	r.heldListing = heldListing{}
	if !held.held {
		return 0, false
	}
	outer := r.inBuiltin
	r.inBuiltin = held.builtin
	defer func() { r.inBuiltin = outer }()
	return r.declarePrint(held.names), true
}
