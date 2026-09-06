// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// `declare` and `typeset`, which give a name an attribute as well as a value.
//
// Which of the two a shell has is a dialect's answer and not an axis: dash
// has neither, ksh93 has `typeset` alone, and bash and zsh have both under one
// implementation. So they are registered through the extension seam, the same
// way ksh93 registers `source` and takes `local` away.
//
// The attributes are the interesting half. `-r` and `-x` are `readonly` and
// `export` spelled differently and share their machinery, but `-i` is a
// property of the *name* that changes what a later assignment means:
//
//	declare -i n; n=5+2   → 7, because the value is evaluated
//	n=5+2                 → the four characters, because it is not
//
// That is why an attribute has to be recorded against the name rather than
// applied once at the point of declaring it.

// declareFlags is what a declaration asks for.
type declareFlags struct {
	integer   bool
	readonly  bool
	export    bool
	assoc     bool
	lower     bool
	upper     bool
	global    bool
	hidden    bool
	unique    bool
	function  bool
	funcNames bool
	remove    bool
	print     bool
	// inert records that a letter out of Semantics.DeclareOptionsWithoutEffect
	// was read. Nothing consults it as an attribute; it exists so that a
	// declaration carrying only such a letter is not mistaken for the bare
	// word, whose listing is a different command entirely — `typeset -F`
	// answering with the whole variable table is the one thing worse than
	// refusing the letter (#1037).
	inert bool
}

// declareOptionLetters is the set `declare` and `typeset` read where the
// dialect has not answered — the letters the substrate implemented before
// they were a question.
const declareOptionLetters = "aAiprx"

// parseDeclareFlags reads the leading option words of a declaration builtin,
// against the letters the dialect gives it. The parse is the one these
// builtins have always had rather than the shared reader's, because `+i`
// removes what `-i` adds and no other builtin spells an option with a plus.
//
// A letter outside the set goes through the shared refusal, so a letter the
// dialect has and this shell does not is named as missing rather than as
// unknown, in the dialect's words.
func (r *Runner) parseDeclareFlags(name string, args []string, known string) (rest []string, f declareFlags, code int) {
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			break
		}
		// `+i` removes the attribute where `-i` adds it, which is the one
		// place a shell spells an option with a plus.
		f.remove = a[0] == '+'
		for _, c := range a[1:] {
			if !strings.ContainsRune(known, c) {
				return nil, f, r.refuseOption(name, a, known)
			}
			if strings.ContainsRune(r.sem().DeclareOptionsWithoutEffect, c) {
				f.inert = true
				// The dialect spells the letter and this engine models
				// nothing it does, so it is taken in silence and decides
				// nothing — see Semantics.DeclareOptionsWithoutEffect for
				// which lie that is and why it is the smaller one.
				continue
			}
			switch c {
			case 'i':
				f.integer = true
			case 'r':
				f.readonly = true
			case 'x':
				f.export = true
			case 'A':
				// The associative attribute, and unlike `-a` it must be
				// recorded: it changes what a later subscript *means*, the
				// way `-i` changes what a later assignment means.
				f.assoc = true
			case 'l':
				f.lower = true
			case 'u':
				f.upper = true
			case 'g':
				// Global rather than local: the assignment reaches the
				// global cell however deep the function stack is.
				f.global = true
			case 'U':
				// Keep only the first occurrence of each element. Like
				// `-i` and the case attributes it is a property of the
				// *name* rather than of this assignment, so it is
				// recorded and consulted by every later write.
				f.unique = true
			case 'H':
				// Hide the value from listings. The name is declared, holds
				// what it holds and reads back exactly as it would without
				// the letter — only a listing that would have written
				// `=value` writes the bare name instead. Recorded rather
				// than acted on at declaration time, because it is a
				// property of the name that a later listing consults, the
				// same shape `-i` has.
				f.hidden = true
			case 'f':
				f.function = true
			case 'F':
				f.funcNames = true
			case 'p':
				// Print rather than declare. `+p` prints too — measured in
				// both shells that spell the option at all.
				f.print = true
			case 'a':
				// Accepted and recorded nowhere. An array here is dynamic,
				// so `typeset -a arr` followed by `arr[0]=x` works without
				// the attribute existing — which is what the flag is used
				// for. The compound form `typeset -a arr=(x y)` is a
				// different thing and is not built: the parser reads the
				// parenthesis as an ordinary word, and making it an array
				// literal in argument position is grammar rather than a
				// flag. Refusing the flag outright would break the common
				// use to be honest about the rare one.
			}
		}
	}
	return args[i:], f, 0
}

func biDeclare(r *Runner, _ context.Context, args []string) int {
	name := r.inBuiltin
	if name == "" {
		name = "declare"
	}
	known := r.sem().DeclareOptions
	if known == "" {
		known = declareOptionLetters
	}
	args, f, code := r.parseDeclareFlags(name, args, known)
	if code != 0 {
		// A bad option ends the script where the dialect counts `typeset`
		// among its special builtins — ksh93, where any of the builtin's
		// failures is fatal, so the honest refusal of a letter it has and
		// this shell does not stops the script the same way.
		if r.ask(r.sem().TypesetBadOptionFatal, "a bad `typeset` option ending the script") {
			r.status = code
			r.fatalQuiet()
		}
		return code
	}

	if f.function || f.funcNames {
		// The function table rather than the variables: `-f` writes the
		// functions themselves and `-F` only names them. `-p` alongside
		// changes nothing — the flags already mean print.
		return r.declareFunctions(args, f.funcNames)
	}

	if len(args) == 0 && f == (declareFlags{}) {
		// Bare `typeset` is a listing, and not the one a bare `local` is —
		// see BareDeclarationListing. Only the truly bare word: `typeset -i`
		// with no names is a filtered listing in the shells that have it,
		// which is a different question and not built.
		return r.bareDeclarationListing()
	}

	if f.print {
		// The other letters are accepted alongside `-p` and decide nothing:
		// the shells that have them use an attribute letter to *filter* the
		// full listing, which is not built. Refusing the combination would
		// break the plain use to be honest about the rare one.
		return r.declarePrint(args)
	}

	for _, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		// Read before the attributes are applied, because `-x` on this very
		// declaration would otherwise answer a question asked about the name
		// it shadows. See shadowedExport.
		wasExported := r.isExported(name)
		// Attributes first, because `-i` changes what the assignment on the
		// same line *means* — but readonly last, because it changes whether
		// that assignment is allowed at all. `declare -r c=1` sets c and then
		// freezes it; applying both up front made the declaration refuse its
		// own value and leave the name empty.
		r.applyAttributes(name, f)
		// Declaring inside a function declares a local, which is unanimous
		// among the three shells that have the name — subject to ksh93's
		// rule about which functions have a scope at all.
		//
		// `-g` is the one thing that changes that: it takes no shadow, so
		// the attributes and the value land on the global cell and `typeset
		// -g x=new` inside a function survives its return even where a
		// `local x` is standing in front of the name.
		//
		// Not taking the shadow is the *whole* of what the letter means, and
		// every other step of the declaration is the same either way.
		// Writing it as an early exit from the loop said otherwise: it made
		// `-g` a second and shorter declaration that dropped every step
		// below the exit. The associative attribute was one of them, and
		// putting `markAssoc` back inside the exit fixed that one letter
		// while leaving the shape that lost it — a valueless `typeset -g n`
		// still brought no name into being, which is the whole of a
		// `typeset -gA a b c` setup line (#989).
		fresh := false
		if !f.global {
			fresh = r.shadowTypeset(name)
			r.localExportAttribute(name, f.export)
			if r.unspecified {
				// See biLocal: an unanswered axis refuses the declaration
				// rather than making it one way and saying so.
				return r.status
			}
			r.shadowedExport(name, wasExported)
		}
		if f.assoc && !f.remove {
			// After the shadow, so that `typeset -A` inside a function
			// declares a local table and the caller's absence comes back
			// when it returns — and, under `-g`, after the shadow that was
			// deliberately *not* taken, which is what leaves the table on
			// the global cell where the function's return cannot reach it.
			// `+A` does nothing rather than removing: two of the three
			// shells with the attribute refuse to take it off a name, the
			// same shape `+r` already has.
			r.markAssoc(name)
		}
		switch {
		case hasValue && f.global:
			r.setGlobalVar(name, value)
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
		case hasValue:
			r.setVarAs(name, value, assignedByDeclaration)
			if r.ctl == controlExit {
				// See biExport: the failure's status is the one that stands.
				return r.status
			}
			r.declarationAssignmentExport(name, f.export)
			if r.unspecified {
				return r.status
			}
		default:
			// `-g` never takes a shadow, so the cell it declares into is
			// never a fresh one — which is exactly the reading that leaves a
			// standing value alone and brings only an absent name into
			// being.
			r.declareEmpty(name, fresh)
		}
		if f.readonly && !f.remove {
			r.markReadonly(name)
		}
	}
	if r.assignFailed {
		// See biExport.
		return 1
	}
	return 0
}

// applyAttributes records what a name has been declared to be.
func (r *Runner) applyAttributes(name string, f declareFlags) {
	if f.integer {
		if r.integer == nil {
			r.integer = map[string]bool{}
		}
		if f.remove {
			delete(r.integer, name)
		} else {
			r.integer[name] = true
		}
	}
	if f.export {
		if r.exported == nil {
			r.exported = map[string]bool{}
		}
		r.exported[name] = !f.remove
	}
	// The case attributes fold at assignment here, which is what bash and
	// ksh93 do. zsh stores the raw text and folds on *expansion* — every
	// read agrees with the other two, and only its `typeset -p` betrays the
	// difference by listing the raw value. That listing nuance is
	// deliberately not modeled; the fold every script observes is.
	if f.lower {
		if r.lowered == nil {
			r.lowered = map[string]bool{}
		}
		if f.remove {
			delete(r.lowered, name)
		} else {
			r.lowered[name] = true
			// The two case attributes cannot both stand: the later one
			// speaks, which is what both shells measured do.
			delete(r.uppered, name)
		}
	}
	if f.upper {
		if r.uppered == nil {
			r.uppered = map[string]bool{}
		}
		if f.remove {
			delete(r.uppered, name)
		} else {
			r.uppered[name] = true
			delete(r.lowered, name)
		}
	}
	if f.unique {
		if r.unique == nil {
			r.unique = map[string]bool{}
		}
		if f.remove {
			// `+U` drops the attribute and leaves the elements that are
			// there alone: measured, `typeset -U c=(1 2 3 2)` is `1 2 3`,
			// and `typeset +U c; c+=(1)` is `1 2 3 1` — the duplicate the
			// append brought stands.
			delete(r.unique, name)
		} else {
			r.unique[name] = true
			// The attribute applies to what the name already holds, not
			// only to what is written next: measured, `b=(1 1 2)` followed
			// by `typeset -U b` reads back `1 2`. A declaration that also
			// assigns has already stored its value by the time this runs,
			// so this one line covers both spellings.
			if a, ok := r.Arrays[name]; ok {
				r.storeArray(name, a)
			}
		}
	}
	if f.hidden {
		if r.hidden == nil {
			r.hidden = map[string]bool{}
		}
		if f.remove {
			// `+H` takes the value back out of hiding and leaves everything
			// else alone: measured, `typeset -iH n=5` lists as `typeset -i n`
			// and `typeset +H n` lists as `typeset -i n=5`.
			delete(r.hidden, name)
		} else {
			r.hidden[name] = true
		}
	}
}

// setGlobalVar assigns to a name's global cell, past any local shadowing it.
//
// In this engine there is one table and a stack of saved outer values, so
// the global cell is either the table itself — no scope saved the name — or
// the copy held by the *oldest* scope that did, which is the value the last
// return will put back. Whether `-g` really reaches past a local is the one
// disagreement here, and it is asked only where a local stands in the way —
// with none, both shells that spell the letter write the global.
func (r *Runner) setGlobalVar(name, value string) {
	for _, sc := range r.scopes {
		if _, saved := sc.saved[name]; !saved {
			continue
		}
		if !r.ask(r.sem().DeclareGlobalReachesPastALocal, "`declare -g` writing past a local of the same name") {
			if r.unspecified {
				return
			}
			// The visible cell — the local — which is what a plain
			// assignment would have written.
			break
		}
		sc.saved[name] = value
		sc.existed[name] = true
		if sc.removedBefore != nil {
			sc.removedBefore[name] = false
		}
		return
	}
	r.setVarAs(name, value, assignedByDeclaration)
}

// declareFunctions is `declare -f` and `-F`: the functions themselves, or
// only their names.
//
// One shell has each of these under `declare` and prints `-F` in its own two
// shapes — `declare -f name` per function when nothing narrows it, the bare
// name when an operand asked — so the shapes are written here the way `-t`'s
// kind words are: there is no second engine to hold a wording for.
func (r *Runner) declareFunctions(names []string, namesOnly bool) int {
	named := len(names) > 0
	if !named {
		// The script's own and not the prelude's: this listing is what a
		// state capture reads, and the prelude's functions are the shell's
		// (#1035, scriptFuncNames). A name asked for is still answered,
		// which is why only this branch narrows.
		names = r.scriptFuncNames()
	}
	status := 0
	for _, name := range names {
		fn, ok := r.funcs[name]
		if !ok {
			// Silent, and 1 stands however many other names printed —
			// measured in both shells that can be asked.
			status = 1
			continue
		}
		switch {
		case !namesOnly:
			r.printf("%s\n", r.listedFunction(name, fn))
		case named:
			r.printf("%s\n", name)
		default:
			r.printf("declare -f %s\n", name)
		}
	}
	return status
}

// listedFunction is a function said back whole, in the dialect's arrangement:
// the header the dialect writes — see Diagnostics.FunctionListingHeader —
// and the body laid out by its function layout.
func (r *Runner) listedFunction(name string, fn *syntax.FuncDecl) string {
	return Wording(r.diag().FunctionListingHeader, "%[1]s () \n%[2]s",
		name, syntax.PrintWith(fn.Body, r.functionLayout))
}

// markReadonly freezes a name.
//
// Never undone: a readonly name cannot be made writable again in any shell in
// the panel, so `+r` does nothing rather than reversing it — which is what
// they do.
//
// A name the running declaration is also assigning to as an operand is frozen
// *after* that assignment rather than now: see the freezing field and
// applyDeferredFreeze. `declare -ar A=(x y)` is one command, and the value it
// carries cannot be refused by the attribute it carries beside it.
func (r *Runner) markReadonly(name string) {
	if r.freezing[name] {
		r.freezeAfter = append(r.freezeAfter, name)
		return
	}
	if r.readonly == nil {
		r.readonly = map[string]bool{}
	}
	r.readonly[name] = true
}

// applyDeferredFreeze freezes the names markReadonly held back, now that the
// declaration's own operand assignments have landed.
//
// Held as a list rather than a set because nothing here depends on the order
// and a list says so: the same name twice freezes once either way.
func (r *Runner) applyDeferredFreeze() {
	for _, name := range r.freezeAfter {
		if r.readonly == nil {
			r.readonly = map[string]bool{}
		}
		r.readonly[name] = true
	}
	r.freezeAfter = nil
}

// operandNames is the set of names a command assigns to as operands — the
// `a=(x y)` written after a declaration utility's own word.
//
// Nil when there are none, which is every command that is not a declaration
// with an array in it, so the lookup markReadonly makes costs a nil map read.
func operandNames(c *syntax.SimpleCmd) map[string]bool {
	var names map[string]bool
	for _, a := range c.Assigns {
		if !a.Operand {
			continue
		}
		if names == nil {
			names = map[string]bool{}
		}
		names[a.Name] = true
	}
	return names
}

// integerValue evaluates an assignment to a name declared integer.
//
// An empty assignment is zero rather than nothing, and a name that is not set
// is zero rather than an error — `n=abc` leaves 0 and says nothing, because
// `abc` is a perfectly good expression whose value happens to be unset. Text
// that will not parse *is* an error, and a fatal one: bash and zsh both stop
// the script rather than store something.
func (r *Runner) integerValue(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "0", true
	}
	p := syntax.NewParser("", r.dialect())
	e := p.ParseArithFor(text, syntax.Pos{})
	if err := p.Err(); err != nil {
		r.fatal("%s\n", r.diag().ParseFailure(err))
		return "", false
	}
	v, err := r.evalArith(e)
	if err != nil {
		r.fatal("%v\n", err)
		return "", false
	}
	return itoa(v), true
}

// declareEmpty is what declaring a name without a value does.
//
// The name is now local, or attributed, or both — but whether it also *exists*
// is a dialect's answer, so this is the one place that decides it and both
// `local` and `typeset` come through here.
func (r *Runner) declareEmpty(name string, fresh bool) {
	if r.ask(r.sem().DeclaredNameWithoutValueIsEmpty, "a declaration without a value setting the name") {
		if !fresh && r.declaredNameHolds(name) {
			// The axis is about a name the declaration *creates*, not about
			// one that is already there: `typeset -x v` on a `v=abc` leaves
			// `abc` alone in all four shells that spell the builtin, and so
			// does `typeset -H h` on an `h=hid`. Emptying it here made a
			// declaration that only meant to add an attribute destroy the
			// value it was adding it to. Inside a function the cell the
			// shadow just made is new whatever the caller held, which is
			// what fresh says — measured `in=[0]` for `typeset -i v` under
			// an outer `v=5`, against `[5]` for the same line at the top.
			//
			// What it keeps is read back through the attribute that has
			// just arrived, which is not the same as leaving it untouched:
			// `a=5+2; typeset -i a` is 7 and `d=MiXeD; typeset -u d` is
			// MIXED. A scalar only — an array keeps its elements as they
			// are under both case letters, measured `a b` from
			// `arr=(a b); typeset -u arr`. Not an assignment either, so a
			// readonly name is re-read rather than refused: measured
			// `typeset -r r=1; typeset -i r` as 1 with status 0.
			if v, ok := r.Vars[name]; ok {
				if folded, ok := r.attributeFolded(name, v); ok {
					r.Vars[name] = folded
				}
			}
			return
		}
		r.setVar(name, "")
		// Set by a declaration and not by an assignment, which the shell's
		// own reads cannot tell apart and a child can: see
		// Runner.declaredEmpty.
		if r.declaredEmpty == nil {
			r.declaredEmpty = map[string]bool{}
		}
		r.declaredEmpty[name] = true
		return
	}
	if r.unspecified {
		return
	}
	// The name exists unset here — but whether the *outer* value still shows
	// through is a second disagreement, and it only arises where a shadow
	// was actually taken: `declare u` at the top level leaves the global
	// alone in every shell measured.
	//
	// Taken *by this declaration*, which is what `fresh` says and what the
	// question turns on. A second declaration of a name its own scope already
	// declared has no outer value in front of it — the value it would hide is
	// the local the first declaration made — and no shell hides that.
	// Measured 2026-09-06, `env -i`, from a file and through `-c` alike:
	// `f() { local FOO=x; local FOO; echo "[${FOO-UNSET}]"; }` reads `x` in
	// bash 5.3.15, bash as `sh`, bash 3.2.57 and zsh 5.9.2, and the ksh93
	// spelling `typeset FOO=x; typeset FOO` reads `x` in all four including
	// ksh93u+. `typeset -g FOO` after a `local FOO=x` is the same shape and
	// the same answer. It is a *different* function's declaration that hides:
	// `f() { local FOO=x; g; }; g() { local FOO; ... }` reads UNSET, and that
	// is the case this axis is about.
	//
	// Asking the scope instead of the declaration answered the two the same
	// way, so the value a function had just put in its own local was thrown
	// away by a line that only meant to name it again.
	if !fresh {
		return
	}
	if r.ask(r.sem().ValuelessDeclarationHidesTheOuterValue, "a declaration without a value hiding the outer value") {
		r.hideVar(name)
	}
}

// declarationAssignmentExport is what a declaration that *assigns* does to the
// export attribute of the name it assigned to.
//
// One shell resets it: the name keeps the value and no child is told about it
// again, at the top level and in a function whose declarations reach the
// caller alike. The other two leave it alone, so this is a switch and not a
// rule — see Semantics.DeclarationAssignmentClearsTheExportAttribute.
//
// namesTheAttribute is `-x` or `+x` on the declaration itself, which settles
// the question outright and is not this one; `export NAME=value` is the same
// case by another spelling, which is why that builtin never comes here.
//
// Asked only where a scope was *not* taken. Where one was, the question is
// LocalInheritsTheExportAttribute — the same shell's answer from the other
// side, already applied and already undone when the function returns. Both
// firing would take the attribute off for good where a keyword function only
// takes it off for its own duration, which is measurably not what happens.
func (r *Runner) declarationAssignmentExport(name string, namesTheAttribute bool) {
	if namesTheAttribute || !r.isExported(name) {
		return
	}
	if len(r.scopes) > 0 {
		if _, shadowed := r.scopes[len(r.scopes)-1].saved[name]; shadowed {
			return
		}
	}
	if !r.ask(r.sem().DeclarationAssignmentClearsTheExportAttribute,
		"a declaration that assigns taking the export attribute off the name") {
		return
	}
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	r.exported[name] = false
}

// declaredNameHolds reports whether the name already has something to lose:
// a scalar, either array, or a value it was born with in the environment.
// A name `unset` took away holds nothing, however many attributes survive it.
func (r *Runner) declaredNameHolds(name string) bool {
	if r.removed[name] {
		return false
	}
	if _, ok := r.AssocArrays[name]; ok {
		return true
	}
	if _, ok := r.Arrays[name]; ok {
		return true
	}
	if _, ok := r.Vars[name]; ok {
		return true
	}
	_, ok := r.inheritedValue(name)
	return ok
}

// localExportAttribute answers whether the local a declaration just took
// inherits the export attribute of the name it shadows.
//
// The question is only there when the shadowed name is exported and a scope
// was actually taken; explicit says the declaration named the attribute
// itself, which answers it outright. Taking the attribute off is a change to
// a record the scope has to put back, so it is saved the way the value is.
func (r *Runner) localExportAttribute(name string, explicit bool) {
	if explicit {
		return
	}
	if len(r.scopes) == 0 {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	if _, shadowed := sc.saved[name]; !shadowed {
		// No scope was taken — a declaration at the top level, or one this
		// dialect gives no scope to — so there is no local to export.
		return
	}
	if !r.isExported(name) {
		// Nothing to inherit, and so nothing to disagree about.
		return
	}
	if r.ask(r.sem().LocalInheritsTheExportAttribute,
		"a local declaration inheriting the export attribute of the name it shadows") {
		return
	}
	if r.unspecified {
		return
	}
	if sc.exportedSpoken == nil {
		sc.exportedSpoken = map[string]bool{}
		sc.savedExported = map[string]bool{}
	}
	if _, seen := sc.exportedSpoken[name]; !seen {
		on, spoken := r.exported[name]
		sc.exportedSpoken[name] = spoken
		sc.savedExported[name] = on
	}
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	r.exported[name] = false
}

// shadowedExport remembers what an exported name held when a declaration took
// a scope in front of it.
//
// The shadowed binding does not stop being exported for having something
// standing in front of it, so a command is told its value for as long as the
// declaration has none of its own: the shell reads `${FOO-UNSET}` as unset
// inside the function and a child is still handed `FOO=bar`. Measured on both
// halves — an exported name and one that arrived in the environment — and on
// two levels, where what a child is told is the *caller's* local rather than
// the global behind it.
//
// Told what the name was rather than asking, because the attributes on this
// very declaration have been applied by the time a scope exists to record
// into: `FOO=bar; f() { local -x FOO; }` would otherwise read its own `-x`
// as the shadowed name's and hand a child a value no shell hands it. Which is
// the difference the measurement turns on — an exported name shadowed by a
// valueless declaration reaches a child, and an unexported one does not,
// whatever the declaration says about the local's own attribute. `+x` is the
// proof that it is the shadowed binding speaking and not the local: it takes
// the attribute off the local outright, and the child is still told the outer
// value.
//
// Reached only where a local carries the attribute of the name it shadows at
// all. The shell that answers LocalInheritsTheExportAttribute no tells a
// child nothing under that name, and it tells it nothing here either — a
// valueless declaration of an exported name is that same question and not a
// second one. The axis is read rather than asked because the declaration
// above has just asked it, and asking twice would report an unanswered one
// twice.
//
// Not written once per scope: a second declaration of the same name hides
// whatever the first one left, and the value that goes to a child is the one
// that was just taken away rather than the one the scope will put back.
func (r *Runner) shadowedExport(name string, exported bool) {
	if !exported || len(r.scopes) == 0 {
		return
	}
	if r.sem().LocalInheritsTheExportAttribute != Yes {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	if _, shadowed := sc.saved[name]; !shadowed {
		// No scope was taken — a declaration at the top level, or one this
		// dialect gives no scope to — so nothing is standing in front of
		// anything.
		return
	}
	value, ok := r.Vars[name]
	if !ok {
		if value, ok = r.inheritedValue(name); !ok {
			// Exported with no value anywhere — `export FOO` and nothing
			// more — and an exported name with no value reaches no child in
			// any shell measured.
			return
		}
	}
	if sc.exportedShadow == nil {
		sc.exportedShadow = map[string]string{}
	}
	sc.exportedShadow[name] = value
}

// hideVar takes a name out of view entirely — the tables and the environment
// fallback alike — so `${name-UNSET}` fires its default. The caller's shadow
// is what brings the outer value back.
func (r *Runner) hideVar(name string) {
	delete(r.Vars, name)
	delete(r.Arrays, name)
	if r.removed == nil {
		r.removed = map[string]bool{}
	}
	r.removed[name] = true
}

// shadowTypeset is shadow for `typeset`, which unlike `local` does not always
// get a scope to declare into.
//
// ksh93 is the reason: only a function defined with the `function` word has
// one, and in a POSIX-style function the assignment is ordinary and reaches
// the caller. The axis is asked only when the two answers differ — inside a
// keyword-defined function they do not, so that case needs no dialect.
//
// The result reports whether this call is what took the scope's copy — see
// shadow, and declareEmpty, which is the one caller that needs to know.
func (r *Runner) shadowTypeset(name string) (fresh bool) {
	if len(r.scopes) == 0 {
		return false
	}
	if !r.scopes[len(r.scopes)-1].keyword &&
		r.ask(r.sem().TypesetLocalNeedsKeywordFunction, "`typeset` needing a keyword-defined function to declare a local") {
		return false
	}
	return r.shadow(name)
}

// shadow saves a name in the innermost scope so the function's exit puts it
// back, which is what makes a declaration local.
//
// The result reports whether the copy was taken *here*: with it, the cell the
// declaration is about to write is new and holds nothing, whatever the outer
// name held. A second declaration of the same name in the same scope finds
// the copy already made and is writing over a cell that is its own.
func (r *Runner) shadow(name string) (fresh bool) {
	if len(r.scopes) == 0 {
		return false
	}
	sc := r.scopes[len(r.scopes)-1]
	if _, seen := sc.saved[name]; !seen {
		fresh = true
		old, existed := r.Vars[name]
		sc.saved[name] = old
		sc.existed[name] = existed
		if sc.removedBefore == nil {
			sc.removedBefore = map[string]bool{}
		}
		sc.removedBefore[name] = r.removed[name]
	}
	// Arrays live in a table of their own, so a name has to be saved from
	// both. Saving only the scalar left `f() { local a; a=(x y); }` writing a
	// global array: `local` shadowed nothing an array assignment then wrote
	// to, and the value outlived the function.
	if _, seen := sc.arrayExisted[name]; !seen {
		old, existed := r.Arrays[name]
		if sc.savedArrays == nil {
			sc.savedArrays = map[string]Array{}
			sc.arrayExisted = map[string]bool{}
		}
		// Copied rather than kept: an Array is a map, so saving the value
		// would save a reference to the very table the function is about to
		// write into, and putting it back would put back the changes.
		if existed {
			kept := make(Array, len(old))
			for k, v := range old {
				kept[k] = v
			}
			old = kept
		}
		sc.savedArrays[name] = old
		sc.arrayExisted[name] = existed
	}
	// And a third table for the associative kind, for the same reason as the
	// second — and here the *attribute* is what is being shadowed as much as
	// the value: `typeset -A m` in a function must not leave the caller's
	// `m` reading its subscripts as strings.
	if _, seen := sc.assocExisted[name]; !seen {
		old, existed := r.AssocArrays[name]
		if sc.savedAssoc == nil {
			sc.savedAssoc = map[string]AssocArray{}
			sc.assocExisted = map[string]bool{}
		}
		if existed {
			kept := make(AssocArray, len(old))
			for k, v := range old {
				kept[k] = v
			}
			old = kept
		}
		sc.savedAssoc[name] = old
		sc.assocExisted[name] = existed
	}
	return fresh
}

// declarationUtilities are the commands whose `name=value` arguments are
// assignments rather than ordinary words.
//
// The list is the core's: `export` and `readonly` are POSIX, and `local` and
// `typeset` are here because every shell in the panel that has them treats
// them the same way. A dialect adds its own names with SetDeclaring — bash and
// zsh add `declare` — because the rule has to follow the name into the shell
// that has it and must not apply in the shell that does not, where the same
// word is an ordinary command.
var declarationUtilities = map[string]bool{
	"export": true, "readonly": true, "local": true, "typeset": true,
}

// SetDeclaring makes a command's `name=value` arguments expand as assignments.
func (r *Runner) SetDeclaring(name string) {
	if r.declaring == nil {
		r.declaring = map[string]bool{}
	}
	r.declaring[name] = true
}

func (r *Runner) declares(name string) bool {
	return declarationUtilities[name] || r.declaring[name]
}

// assignShaped reports whether a word begins with a literal `name=`.
//
// The name has to be literal: `$x=1` is not an assignment in any shell, and
// neither is `"a"=1`. Only what the word says before any expansion counts.
func assignShaped(w *syntax.Word) bool {
	if w == nil || len(w.Spans) == 0 {
		return false
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
		return false
	}
	name, _, ok := strings.Cut(s.Value, "=")
	return ok && name != "" && isNameLike(name)
}

// expandAssignArg expands `name=value` given to a declaration utility, leaving
// the name alone and expanding the value as an assignment's.
func (r *Runner) expandAssignArg(w *syntax.Word) string {
	head := w.Spans[0]
	name, first, _ := strings.Cut(head.Value, "=")
	rest := *w
	rest.Spans = append([]syntax.Span{{
		Kind: syntax.Literal, Value: first, Quoting: head.Quoting, Pos: head.Pos,
	}}, w.Spans[1:]...)
	return name + "=" + r.expandAssignValue(&rest)
}
