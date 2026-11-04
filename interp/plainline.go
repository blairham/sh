// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A plain declaration line: `local a b c`, `local a=$1 b=$2 c`, and `typeset`
// or `declare` written the same way wherever they make locals.
//
// Each operand of such a line already takes the empty-shadow route of
// emptyshadow.go when there is nothing standing under its name. What every
// operand still paid for was the utility's line setup in front of the
// operands: the letters parsed out of words that hold none, the hide watch
// armed, the tie, listing, `-` and private shapes each asked about, every
// name checked twice for a refusal it cannot have, and the deferred
// restricted-mode and state restores. For a line of plain names each of those
// answers "nothing to do", and together they were about a quarter of what a
// five-name `local` cost in zsh (#6373).
//
// So a line written only of plain names, `name` or `name=value`, runs its
// operands through the very step the full road's operand loop runs —
// localOverNothing for `local`, typesetOverNothing for `typeset` — one at a
// time and in order, without the setup. The route decides nothing about an
// operand: shadowsNothing still answers each one, declareOverNothing still
// declares it. The first operand shadowsNothing will not take ends the route,
// and the rest of the line, that operand included, is handed to the full road
// as a line of its own. That hand-off is sound because the setup over a plain
// name is inert both ways: the operands before it were plain names over
// nothing, which the hide watch has nothing to record about, and the full
// road's setup over the rest reads only those words.
//
// What makes a line plain is decided before anything runs:
//   - every operand is a plain name, alone or with `=` and a value — no
//     letters, no `+=`, no subscript, no `-` or `+` word;
//   - the line is one the empty shadow may take at all (plainLocalLine, or
//     plainTypesetLine for the second word);
//   - no array or compound literal is written on the line, and no
//     `command` prefix runs the word;
//   - and the runner is not restricted, has no refusal waiting, and has not
//     stopped on an unanswered question.
//
// TestAPlainLineAnswersAsTheFullRoadDoes runs declarations over every edge
// those conditions draw, with the route on and off, and requires the same
// output and status. TestThePlainLineSkipsOnlyWhatItWasCheckedAgainst fails
// when the setup the route leaves out changes, which is the moment to read
// this again.

// plainLineShortcut switches the route on. A variable so a test can run the
// same script with the full road answering every line.
var plainLineShortcut = true

// plainLineWords reports whether every operand is a plain name, with or
// without `=` and a value: the shape whose setup is inert. An empty line is a
// listing, and is not one.
func plainLineWords(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if !isPlainName(name) {
			return false
		}
	}
	return true
}

// plainLineRunner reports whether the runner is in a state where a plain
// line's setup is inert: nothing restricted, no refusal waiting to be
// counted, no array or compound literal standing on the line, no `command`
// prefix running the word, and no question left unanswered.
func (r *Runner) plainLineRunner() bool {
	return plainLineShortcut && !r.unspecified && !r.restricted && r.restrictedRefusedOperand == "" &&
		!r.localUnderCommandPrefix && len(r.literalOperands) == 0 && len(r.arrayOperands) == 0 &&
		len(r.compoundOperands) == 0
}

// localPlainLine is biLocal for a line whose words plainLineWords accepted, on
// a runner plainLineRunner and plainLocalLine accepted. It declares each
// operand through localOverNothing, as the full road's loop would, until one
// is not taken; done false hands that operand and every one after it back as
// rest, for the full road to run as a line of its own.
func (r *Runner) localPlainLine(args []string) (rest []string, status int, done bool) {
	sc := r.scopes[len(r.scopes)-1]
	asked := askLetters(declareFlags{})
	// What the full road's name check resets for the line, and what its
	// operand loop sets for each operand and puts back after the line.
	r.landedPastARefusal = false
	was := r.declarationEvaluatesItsValue
	defer func() { r.declarationEvaluatesItsValue = was }()
	for i, a := range args {
		name, value, hasValue, appends := r.declarationOperand(a)
		r.declarationEvaluatesItsValue = false
		taken, stop := r.localOverNothing(sc, name, value, hasValue, appends, asked)
		if !taken {
			// Put back before the full road saves it, so its restore
			// leaves what the line found.
			r.declarationEvaluatesItsValue = was
			return args[i:], 0, false
		}
		r.plainLineOperands++
		if stop {
			return nil, r.status, true
		}
	}
	if r.assignFailed {
		return nil, 1, true
	}
	return nil, 0, true
}

// localOverNothing is one `local` operand on a line plainLocalLine accepted,
// declared over a name with nothing to shadow — the step biLocal's operand
// loop takes and localPlainLine takes in its place. taken is false where
// shadowsNothing will not have the operand, and nothing has been written;
// stop is true where the builtin is over, at r.status.
func (r *Runner) localOverNothing(sc *scope, name, value string, hasValue, appends bool, asked lettersAsked,
) (taken, stop bool) {
	if appends || !r.shadowsNothing(sc, name) {
		return false, false
	}
	r.declareOverNothing(sc, name, value, hasValue, declareFlags{}, asked, assignedAnyhow)
	if hasValue && r.ctl == controlExit {
		return true, true
	}
	if hasValue {
		// setVar's form, so the store marks nothing: see the other store in
		// biLocal for why the mark is this word's own.
		r.markForAllexport(name)
	}
	return true, r.unspecified
}

// typesetPlainLine is biDeclare for a line whose words plainLineWords
// accepted, on a runner plainLineRunner and plainTypesetLine accepted: each
// operand through the head and the step declareNames' operand loop takes, until
// one is not taken. done false hands that operand and the rest back, for the
// full road to run as a line of its own; every field the full road saves and
// restores around its loop has been put back by then, so its restores leave
// what the line found.
func (r *Runner) typesetPlainLine(args []string) (rest []string, status int, done bool) {
	sc := r.scopes[len(r.scopes)-1]
	asked := askLetters(declareFlags{})
	// The name check's reset, and the fields declareNames saves around its
	// loop — and the one biDeclare clears after it.
	r.landedPastARefusal = false
	saved := r.saveTypesetLine()
	defer r.restoreTypesetLine(saved)
	r.caseFoldMayBeHeld = true
	for i, a := range args {
		name, value, hasValue, appends := r.typesetOperandHead(a, declareFlags{})
		r.rereadingAQuotedLiteral = hasValue && r.operandHidesALiteral(name, value, declareFlags{}, false)
		taken, stop := r.typesetOverNothing(sc, name, value, hasValue, appends, asked)
		if !taken {
			r.restoreTypesetLine(saved)
			return args[i:], 0, false
		}
		r.plainLineOperands++
		if stop {
			return nil, r.status, true
		}
	}
	return nil, 0, true
}

// typesetLineState is what declareNames saves before its operand loop and puts
// back after it, and what biDeclare clears.
type typesetLineState struct {
	retyping, spokenAs                  string
	rereading, evaluates, caseFoldMayBe bool
}

func (r *Runner) saveTypesetLine() typesetLineState {
	return typesetLineState{
		retyping: r.retypingFrozen, spokenAs: r.refusalSpokenAs,
		rereading: r.rereadingAQuotedLiteral, evaluates: r.declarationEvaluatesItsValue,
		caseFoldMayBe: r.caseFoldMayBeHeld,
	}
}

func (r *Runner) restoreTypesetLine(s typesetLineState) {
	r.retypingFrozen, r.refusalSpokenAs = s.retyping, s.spokenAs
	r.rereadingAQuotedLiteral, r.declarationEvaluatesItsValue = s.rereading, s.evaluates
	r.caseFoldMayBeHeld, r.caseFoldHeld = s.caseFoldMayBe, ""
	r.listsOneElementPerLine = false
}

// typesetOperandHead is what declareNames' loop sets for each operand before it
// asks anything of it: the operand's case fold forgotten, its parts, whether
// a numeric letter evaluates its value, and whether it has one.
func (r *Runner) typesetOperandHead(a string, f declareFlags) (name, value string, hasValue, appends bool) {
	r.caseFoldHeld = ""
	name, value, hasValue, appends = r.declarationOperand(a)
	r.declarationEvaluatesItsValue = hasValue && (f.integer || f.float)
	r.caseFoldOperandValued = hasValue
	return name, value, hasValue, appends
}

// typesetOverNothing is localOverNothing for a `typeset` or `declare` operand
// on a line plainTypesetLine accepted: the step declareNames' operand loop
// takes and typesetPlainLine takes in its place.
func (r *Runner) typesetOverNothing(sc *scope, name, value string, hasValue, appends bool, asked lettersAsked,
) (taken, stop bool) {
	if appends || !r.shadowsNothing(sc, name) {
		return false, false
	}
	// The first thing shadowTypeset does for a name it shadows, kept so the
	// operand leaves no OPTIND mark standing for the next.
	r.optindCursorMadeHere = false
	r.declareOverNothing(sc, name, value, hasValue, declareFlags{}, asked, assignedByDeclaration)
	return true, hasValue && r.ctl == controlExit
}
