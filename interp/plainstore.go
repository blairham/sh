// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// A bare assignment that stores a value and does nothing else.
//
// `x=`, `x=0`, `x=$y`: most of the assignments a script runs put a value in an
// ordinary variable, and nothing about them is a question. The full road asks
// about thirty anyway — a reference to follow, a freeze, a hidden kind to
// watch, a compound to retype, a discipline to run, an attribute to fold the
// value through, a width to learn, a tie to mirror, a producer to feed, a
// guard and an action registered for the name, a frame selected elsewhere —
// and every one answers "nothing to do". That was about half of what `x=`
// cost: 0.28 µs against zsh's 0.04 in the zsh dialect, where the variable is
// written in a few nanoseconds once the questions are over (#6423).
//
// storesAPlainScalar settles every one of those questions before anything is
// expanded, and storePlainScalar then does what the full road does when each
// of them answers no: the scope lint, if it is on, the store, and
// allexport. Anything it is not sure of goes to the
// full road having read nothing, the same arrangement plainParamValue has.
//
// The conditions are the full road's own, read from its code rather than
// from a list of what seemed likely. That is also the hazard: a question
// added to the full road tomorrow is not asked here. Two tests stand against
// that. TestAPlainScalarStoreAnswersAsTheFullRoadDoes runs scripts that reach
// each question with the shortcut on and off and requires the same output,
// and TestTheFullRoadOfAnAssignmentIsTheOneThePlainStoreWasCheckedAgainst
// fails when the calls the full road makes change, which is the moment this
// file has to be read again.

// plainScalarStoreShortcut switches the shortcut on. A variable so a test can
// run the same script with the full road answering every assignment.
var plainScalarStoreShortcut = true

// storesAPlainScalar reports whether the assignment is a plain store that
// storePlainScalar may make in the full road's place.
func (r *Runner) storesAPlainScalar(a *syntax.Assign) bool {
	if a.Index != nil || a.IsArray || a.Members != nil || len(a.Leading) > 0 ||
		a.Member != "" || a.Append || a.EmptySubscript || a.Operand || a.IndexFlags != nil {
		return false
	}
	// A valueless declaration's store is the other caller of storesPlainly,
	// and it is the one place this is set — see Runner.declareEmpty.
	return !r.declaringWithoutAValue && r.storesPlainly(a.Name) && r.valueOnlyReads(a.Value)
}

// storesPlainly reports whether every question the full road asks of a store
// into name answers "nothing to do", whatever the value: the questions about
// the name and about what the runner is in the middle of, without the
// assignment's own shape or its value.
//
// Its other caller is a valueless declaration in a dialect where one sets the
// name to the empty string (Runner.declareEmpty). The full road there is
// setVarAs with assignedAsTheCompoundView, which differs from a bare
// assignment's in three places and none of them is a question this answers:
// it skips the scope lint and allexport, which belong to an assignment, and
// it skips the guard registered for the name, which a name accepted here does
// not have (#6373).
func (r *Runner) storesPlainly(name string) bool {
	if !plainScalarStoreShortcut {
		return false
	}
	// An identifier, so not a positional parameter and not a compound
	// member; and none of the names the store itself treats specially.
	if !identifierName(name) || name == "OPTIND" || name == "PATH" || name == r.sem().IgnoredNamesVariable {
		return false
	}
	// What the runner is in the middle of: a traced assignment's value is
	// expanded by the caller, and each of the rest is a road of its own.
	if r.expanded != nil || r.unspecified || r.namespace != "" || r.frameSelected ||
		r.storingSelfNameref != "" || r.retypingFrozen != "" ||
		r.viewIsAlreadyFolded || r.mirroring || r.caseFoldHeld != "" || r.privateDeclared ||
		len(r.privateShield) > 0 || len(r.nameref) > 0 || r.scalarAssignmentGlobs() {
		return false
	}
	if r.LocaleCharset != nil {
		for _, v := range localeVariables {
			if v == name {
				return false
			}
		}
	}
	// The name holds nothing but a scalar, carries no attribute, and no
	// table of the shell's has an entry for it.
	if c := r.cell(name); c != nil && (c.has&^slotScalar != 0 || c.attrs != 0) {
		return false
	}
	if r.disciplined[name] || r.compoundVariable[name] || r.capitalized[name] {
		return false
	}
	// The tables the shell registers its own names in. One test of a filter
	// answers for all eight where the name was never registered, which is
	// nearly every name a script writes.
	if r.specialNames.mayHold(name) {
		if r.deferredParams[name] {
			return false
		}
		if _, ok := r.kindFixed[name]; ok {
			return false
		}
		if _, ok := r.assignmentGuards[name]; ok {
			return false
		}
		if _, ok := r.valueRefusals[name]; ok {
			return false
		}
		if _, ok := r.assignmentActions[name]; ok {
			return false
		}
		if _, ok := r.dynamicWriters[name]; ok {
			return false
		}
		if _, ok := r.dynamicArrayWriters[name]; ok {
			return false
		}
		if _, ok := r.tied[name]; ok {
			return false
		}
	}
	if _, ok := r.floatPrecision[name]; ok {
		return false
	}
	if _, ok := r.fieldWidth[name]; ok {
		return false
	}
	return true
}

// valueOnlyReads reports whether expanding w can do nothing but read: no
// substitution of any kind, and no `~` or `=` for the tilde and equals passes
// to act on. A read of a plain `$name` is admitted where plainParamValue
// answers it, which is a read of an ordinary scalar and nothing else.
func (r *Runner) valueOnlyReads(w *syntax.Word) bool {
	if w == nil {
		return true
	}
	for i := range w.Spans {
		s := &w.Spans[i]
		if s.Kind != syntax.Literal || strings.ContainsAny(s.Value, "~=") {
			if len(w.Spans) == 1 {
				if r.plainTrimOf(s) || r.plainArithOf(s) {
					return true
				}
				_, ok := r.plainParamValue(w)
				return ok
			}
			return false
		}
	}
	return true
}

// storePlainScalar is the full road for an assignment storesAPlainScalar
// accepted, with every question answered: what is left is the lint, the store
// and allexport.
func (r *Runner) storePlainScalar(ctx context.Context, a *syntax.Assign) {
	value, ok := r.plainParamValue(a.Value)
	if !ok {
		reread := r.arithValuesReread
		if w := a.Value; plainArithShortcut && w != nil && len(w.Spans) == 1 && w.Spans[0].Kind == syntax.ArithSubst {
			// storesAPlainScalar took it through plainArithOf, which is the
			// one road it admits an arithmetic expansion by, so it is not
			// asked again.
			value = r.readPlainArith(w)
		} else {
			value = r.expandAssignValue(a.Value)
		}
		if r.arithValuesReread != reread || r.expandErr || r.ctl != controlNone || r.unspecified {
			// The expansion did something besides read — see plainArithOf —
			// so the questions this road answered before it may have other
			// answers now. The full road asks them again of the value already
			// expanded, the arrangement a traced assignment uses, so nothing
			// is expanded twice.
			r.plainStoreHandedOn++
			r.withPreparedValue(ctx, &expandedAssign{assign: a, value: value})
			return
		}
	}
	if !r.scopeWarningsAreOff() {
		r.warnAboutTheScope(a.Name, scopeWarningScalar)
	}
	// The full road clears the declared-empty and unset marks here; both are
	// attributes in the cell now (#6442), and a name storesAPlainScalar
	// accepted carries none.
	// And it lets go of a held environment copy as the full road does: a
	// name exported by having arrived in the environment carries no attribute
	// in the cell, so one can be held for a name this road accepts.
	r.releaseEnvCopy(a.Name)
	r.varsPut(a.Name, value)
	r.markForAllexport(a.Name)
}

// plainTrimOf reports whether a value's one span is a trim of an ordinary
// scalar by written text — `${s#*/}`, `${s%%.*}` — which reads and does
// nothing else.
//
// The value is still expanded by the full road (storePlainScalar falls back
// to expandAssignValue), so nothing here decides what the trim answers. What
// this settles is that expanding it cannot change any of the answers
// storesAPlainScalar has already taken about the name being written: the
// subject is a name plainParamValue would read, in a state where it would
// read it, and the operand is text with nothing in it to expand. Two things a
// trim's operand can do besides match are left to the full road by name. A
// `(#b)` or `(#m)` flag publishes `$match` and `$MATCH`, which a script can
// be assigning — measured on zsh 5.9.2, `s=ab; match=${s#(#b)(a)}` leaves
// `typeset match=b`, where taking the plain store here left the array the
// publish wrote. And a `~` reaches the tilde pass, and a `~(…)` group the
// matcher's own reading of one.
//
// The record a pattern match writes where `=~` writes one is not on that
// list: it is written to a name no assignment spells (`.sh.match`), and a
// reference that could alias it already sends every store to the full road.
//
// This is the trim row of #6353's construct table: before it, `x=${s#*/}`
// asked the full road's thirty questions about `x` on every iteration.
func (r *Runner) plainTrimOf(s *syntax.Span) bool {
	if s.Kind != syntax.ParamExp || s.Quoting != syntax.Unquoted || s.Param == nil || !r.readsAreOnlyReads() {
		return false
	}
	e := s.Param
	switch e.Op {
	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong, syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong:
	default:
		return false
	}
	if e.Arg == nil || !bareApartFromTheOperator(e) || !r.ordinaryScalarName(e.Name) {
		return false
	}
	var last byte
	for i := range e.Arg.Spans {
		a := &e.Arg.Spans[i]
		if a.Kind != syntax.Literal || strings.IndexByte(a.Value, '~') >= 0 ||
			strings.Contains(a.Value, "(#") || (last == '(' && strings.HasPrefix(a.Value, "#")) {
			return false
		}
		if a.Value != "" {
			last = a.Value[len(a.Value)-1]
		}
	}
	return true
}

// plainArithOf reports whether a value's one span is an arithmetic expansion
// whose written text can only read: numbers, names, and operators that do not
// assign — `$((i + 1))`, `"$((n * 2))"`. That is the counter in a loop, and
// it was the commonest assignment the full road answered: the CI gate's
// workload is about two thirds `x=$((x + 1))` (#6496).
//
// As with plainTrimOf, the value is still expanded by the full road's
// expansion, so nothing here decides what the expression answers. What the
// text cannot show is a name whose *value* is read again as an expression —
// `x='i=7'; i=$((x))` assigns `i` from inside the expansion, and in zsh 5.9.2
// leaves it `typeset -i`. That is settled after the fact rather than before:
// storePlainScalar hands the value to the full road when any value was read
// again as an expression while it was expanded (arithValuesReread), or when
// the expansion failed.
//
// Every name is asked to be one plainParamValue could read, which keeps out a
// produced parameter (`$((SECONDS))`), a reference, an array and a parameter
// that arrives on first use, each of which reads with an effect of its own.
func (r *Runner) plainArithOf(s *syntax.Span) bool {
	if s.Kind != syntax.ArithSubst || s.Arith == nil || !r.readsAreOnlyReads() ||
		(s.Quoting != syntax.Unquoted && s.Quoting != syntax.DoubleQuoted) ||
		r.arithPrecedenceOptionInCharge() {
		return false
	}
	// A byte at a time rather than strings.ContainsAny, which on amd64 is a
	// vector search set up afresh for each of its two characters over a text
	// a few bytes long.
	for i := 0; i < len(s.Value); i++ {
		if c := s.Value[i]; c == '$' || c == '`' {
			return false
		}
	}
	return r.arithOnlyReads(s.Arith)
}

// arithOnlyReads reports whether evaluating the parsed expression e can do
// nothing but read ordinary scalars and compute; see plainArithOf.
func (r *Runner) arithOnlyReads(e syntax.ArithExpr) bool {
	switch x := e.(type) {
	case *syntax.ArithNum:
		return true
	case *syntax.ArithVar:
		return !x.Special && identifierName(x.Name) && r.ordinaryScalarName(x.Name)
	case *syntax.ArithUnary:
		return x.Op != "++" && x.Op != "--" && r.arithOnlyReads(x.X)
	case *syntax.ArithBinary:
		return r.arithOnlyReads(x.X) && r.arithOnlyReads(x.Y)
	case *syntax.ArithCond:
		return r.arithOnlyReads(x.Cond) && r.arithOnlyReads(x.Then) && r.arithOnlyReads(x.Else)
	}
	return false
}

// plainArithShortcut switches plainArithValue on. A variable so a test can run
// the same script with the word road answering every arithmetic expansion.
var plainArithShortcut = true

// plainArithValue is the text of a word that is one arithmetic expansion
// plainArithOf admits — `$((i + 1))`, `"$((n * 2))"` — read without the word
// road around it: the span's own expansion, and nothing else. What the road
// would add is nothing for such a word: no literal for the tilde passes, no
// subscript to arm, no field to split or glob, and a number's text is the
// same after the escape a result is given and the unescape that takes it off
// again. That road was about half of what `x=$((x + 1))` cost (#6496).
//
// The expansion is subscriptArith, which is the word road's own call for the
// span, so a held result and a failure are answered exactly as there; a
// failed one is the empty text the road leaves.
func (r *Runner) plainArithValue(w *syntax.Word) (string, bool) {
	if !plainArithShortcut || w == nil || len(w.Spans) != 1 || !r.plainArithOf(&w.Spans[0]) {
		return "", false
	}
	return r.readPlainArith(w), true
}

// readPlainArith is plainArithValue's read, for a word already admitted.
func (r *Runner) readPlainArith(w *syntax.Word) string {
	defer r.inWord(w)()
	v, ok := r.subscriptArith(w, w.Spans[0])
	if !ok {
		return ""
	}
	return v
}
