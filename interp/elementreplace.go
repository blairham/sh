// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"github.com/blairham/sh/syntax"
)

// `${a:/pattern/replacement}`, from docs/spec/grammar/parameter-expansion.md:
// the fourth operator of the whole-element family and the only one that
// writes. One grammar in the panel has it
// (syntax.Dialect.ParamWholeElementReplace), so what it means here is that
// shell's answer, measured.
//
//	${a:/pattern}              drop the elements the pattern matches whole
//	${a:/pattern/replacement}  replace them with the replacement
//
// It is **not** the span replacement with a colon in front. `${x:/foo/Z}` on
// `foobar` is `foobar`, where `${x/foo/Z}` is `Zbar` — the pattern is matched
// against the whole element, exactly as `:#` matches one, which is why the
// test here is elementKeeper's and not replaceWith's.
//
// Two rules make the difference between a fix that demos and a fix that runs
// a startup:
//
//   - **A value the pattern does not match is left alone, silently.** No
//     complaint, status 0, on a scalar as well as on an array — `${x:/b/Z}`
//     on `/a/b/c` is `/a/b/c`. `compaudit`, `compdump` and `_p9k_must_init`
//     all reach this operator on a real startup and most of what they hand it
//     does not match.
//   - **An element replaced by *nothing* is dropped**, where an element that
//     was already empty and did not match is kept. `${^~fpath:/.}` is how
//     both completion files strike `.` out of `fpath`, and it is spelled with
//     no replacement at all.
//
// A scalar is not a list and keeps its empty value: `x=foo; ${x:/foo}` is one
// empty field where `${(@)a:/foo}` on `(foo)` is no field at all — the same
// split selectScalar already records for `:#`.

// replacesElements reports the operator whose matches become something else.
func replacesElements(op syntax.ParamOp) bool {
	return op == syntax.ParamElementReplace
}

// reshapesElements reports every operator that changes *which* elements a
// list has rather than what each one becomes — the three selectors and this
// replacement together.
//
// One predicate for the four, because the callers' question is about the
// shape of the answer and not about the operator: each of them applies the
// operator before the `[*]` join and outside the elementOp mapping, whose
// whole form is one output per input. A second predicate beside the first
// would have to repeat that placement and the quoting rule with it, and the
// two would drift.
func reshapesElements(op syntax.ParamOp) bool {
	return selectsElements(op) || replacesElements(op)
}

// reshapeElements applies one of the four to a list.
func (r *Runner) reshapeElements(e *syntax.ParamExpr, elems []string) []string {
	if replacesElements(e.Op) {
		return r.replaceElements(e, elems)
	}
	return r.selectElements(e, elems)
}

// reshapeScalar is the same four against a value that is one string.
func (r *Runner) reshapeScalar(e *syntax.ParamExpr, value string) string {
	if replacesElements(e.Op) {
		return r.replaceElementScalar(e, value)
	}
	return r.selectScalar(e, value)
}

// replaceElements replaces the elements the pattern matches whole and drops
// the ones it replaces with nothing.
func (r *Runner) replaceElements(e *syntax.ParamExpr, elems []string) []string {
	replace := r.elementReplacer(e)
	out := make([]string, 0, len(elems))
	for _, el := range elems {
		with, matched := replace(el)
		switch {
		case !matched:
			out = append(out, el)
		case with != "":
			out = append(out, with)
		}
	}
	return out
}

// replaceElementScalar is the operator against one value.
//
// The emptied value stays a field, which is where a scalar parts company with
// a list: `x=foo; set -- "${x:/foo}"` leaves `$#` at 1 where
// `a=(foo); set -- "${(@)a:/foo}"` leaves it at 0.
func (r *Runner) replaceElementScalar(e *syntax.ParamExpr, value string) string {
	with, matched := r.elementReplacer(e)(value)
	if !matched {
		return value
	}
	return with
}

// elementReplacer expands the operands and returns the test one element goes
// through, with what it becomes when it passes.
//
// The pattern is expanded once for the whole list, as every other operator
// inside `${ }` does — a command substitution in it runs a single time.
//
// The **replacement** follows replaceWith's rule rather than that one, and it
// is the same rule for the same reason: a pattern that reports fills `$MATCH`
// or `$match` before each replacement, so each one has to be read again to
// see them, and a pattern that reports nothing leaves the replacement the
// same text every time. Measured on zsh 5.9.2 with `a=(x y z)`:
// `i=0; ${(@)a:/*/$((++i))}` is `1 1 1` and `i=0; ${(@)a:/(#m)*/$((++i))}` is
// `1 2 3`. matchPatternR publishes what it matched, so `$MATCH` is already
// standing by the time the replacement is read.
func (r *Runner) elementReplacer(e *syntax.ParamExpr) func(string) (string, bool) {
	// A pattern, so the *word* is what the matcher needs rather than its
	// text — the same reading `:#` takes, and the same axis that keeps
	// `p="t*"; echo $p` from globbing.
	pattern := r.patternOf(e.Arg)
	repl := r.replacementWord(e)
	if r.patternReports(pattern) {
		return func(el string) (string, bool) {
			if !r.matchPatternR(pattern, el, false) {
				return "", false
			}
			return r.replacementOf(repl), true
		}
	}
	with := r.replacementOf(repl)
	return func(el string) (string, bool) {
		if !r.matchPatternR(pattern, el, false) {
			return "", false
		}
		return with, true
	}
}
