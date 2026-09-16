// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// Attributes a *function* carries, as against the ones a variable carries
// under the same letters.
//
// One shell in the panel has the notion. `readonly f` and `readonly -f f`
// freeze two different things under one word, and the second half was the
// half that did nothing: the letter was accepted, no record was kept, and the
// function could be redefined and unset afterwards at status 0 where bash
// refuses both (#3192).
//
// The letters are the dialect's — see Semantics.FunctionAttributeLetters —
// and two of them are known here because the substrate already keeps both
// tables: `r` is Runner.readonlyFuncs and `x` is Runner.exportedFuncs. What
// the dialect supplies is *which* letters it spells and in what order a
// listing writes them, so a shell without the notion has an empty set and
// reaches none of this.

const (
	functionAttributeReadonly = 'r'
	functionAttributeExported = 'x'
)

// functionAttributes is the letters one function holds, in the dialect's
// order — the text that goes after `declare -f` in a listing.
//
// Empty for a function with none, and empty in every dialect that names no
// letters, which is what keeps `declare -F` writing the bare `declare -f
// NAME` everywhere else.
func (r *Runner) functionAttributes(name string) string {
	var out strings.Builder
	for _, c := range r.sem().FunctionAttributeLetters {
		if r.functionHoldsAttribute(name, c) {
			out.WriteRune(c)
		}
	}
	return out.String()
}

// functionHoldsAttribute reports one letter of one function.
//
// A letter the dialect named and this package does not know is *not* held, so
// a dialect that spells a third letter gets a listing missing that letter
// rather than a listing claiming every function has it.
func (r *Runner) functionHoldsAttribute(name string, letter rune) bool {
	switch letter {
	case functionAttributeReadonly:
		return r.readonlyFuncs[name]
	case functionAttributeExported:
		return r.exportedFuncs[name]
	}
	return false
}

// markFunctionAttribute puts one letter on one function.
func (r *Runner) markFunctionAttribute(name string, letter rune) {
	switch letter {
	case functionAttributeReadonly:
		if r.readonlyFuncs == nil {
			r.readonlyFuncs = map[string]bool{}
		}
		r.readonlyFuncs[name] = true
	case functionAttributeExported:
		if r.exportedFuncs == nil {
			r.exportedFuncs = map[string]bool{}
		}
		r.exportedFuncs[name] = true
	}
}

// functionAttributeLettersWritten is the subset of a declaration's letters
// this dialect calls function attributes, in the dialect's order.
//
// The order is the field's rather than the command line's, because the field
// is what a listing writes: `declare -Fxr` and `declare -Frx` are the same
// request and bash writes `rx` for both.
//
// A letter written under a **plus** does not count. `declare +fr b` takes the
// *function* attribute off in bash and is BareDeclarationListing's question
// rather than this one, so the plus spelling is read as "not written" here
// instead of as a removal this file would have to invent a meaning for. The
// sign is the last one the line gave that letter, which is the rule every
// other letter on these builtins already follows.
func (r *Runner) functionAttributeLettersWritten(f declareFlags) string {
	var out strings.Builder
	for _, c := range r.sem().FunctionAttributeLetters {
		if plus, written := f.lastSign(c); written && !plus {
			out.WriteRune(c)
		}
	}
	return out.String()
}

// functionsHoldingAttributes is the population a listing narrowed by those
// letters writes: the functions holding **any** of them, in listing order.
//
// A union and not an intersection, measured 2026-09-16 on bash 5.3.20 with
// one frozen function, one exported one and one that is both: `declare -Frx`
// lists all three. An intersection would have listed the one.
//
// The script's own and not the prelude's, which is the population every
// unnarrowed listing already walks.
func (r *Runner) functionsHoldingAttributes(letters string) []string {
	var out []string
	for _, name := range r.scriptFuncNames() {
		for _, c := range letters {
			if r.functionHoldsAttribute(name, c) {
				out = append(out, name)
				break
			}
		}
	}
	return out
}

// freezeFunctions answers `readonly -f`, and is `export -f`'s twin: the same
// shape, the same refusal for a name that is not a function, and the other
// table.
func (r *Runner) freezeFunctions(names []string) int {
	status := 0
	for _, name := range names {
		if _, ok := r.funcs[name]; !ok {
			// Refused rather than remembered, exactly as exportFuncs refuses:
			// a name that is not a function now will not become one by being
			// frozen. Measured, `readonly -f nosuchfn` is `readonly:
			// nosuchfn: not a function` at 1.
			r.diagf("%s\n", Wording(r.diag().ReadonlyNotAFunction, "readonly: %[1]s: not a function", name))
			status = 1
			continue
		}
		r.markFunctionAttribute(name, functionAttributeReadonly)
	}
	return status
}

// readonlyFunctionRedefined refuses a definition of a frozen name, reporting
// whether it did.
//
// Not fatal: measured on bash 5.3.20 and bash 3.2.57, the refusal is status 1
// and the script carries on — `( b() { :; }; echo carried on )` writes the
// refusal and then `carried on`. It is also the *definition* that is refused
// and not the name: `b` goes on being callable and goes on holding the body
// it had.
func (r *Runner) readonlyFunctionRedefined(name string) bool {
	if !r.readonlyFuncs[name] {
		return false
	}
	r.diagf("%s\n", Wording(r.diag().ReadonlyFunctionRedefined, "%[1]s: readonly function", name))
	r.status = 1
	return true
}

// readonlyFunctionUnset refuses to remove a frozen function, reporting the
// status `unset` should carry and whether it refused.
func (r *Runner) readonlyFunctionUnset(name string) (int, bool) {
	if !r.readonlyFuncs[name] {
		return 0, false
	}
	r.diagf("%s\n", Wording(r.diag().UnsetReadonlyFunction,
		"unset: %[1]s: cannot unset: readonly function", name))
	return 1, true
}

// setFunctionAttributes is the operand form of the same letters: `declare -fr
// b` freezes `b` rather than listing it.
//
// A name that is not a function is a silent 1 and the names after it are
// still done, which is the answer a `-f` listing already gives for a name it
// does not hold — measured, `declare -fr a nosuchfn` freezes `a` and answers
// 1 with nothing said.
func (r *Runner) setFunctionAttributes(names []string, letters string) int {
	status := 0
	for _, name := range names {
		if _, ok := r.reportedFunc(name); !ok {
			status = 1
			continue
		}
		for _, c := range letters {
			r.markFunctionAttribute(name, c)
		}
	}
	return status
}

// attributedFunctionListing is what `readonly -f` and `export -f` write when
// no name was given: the functions holding that one letter, bodies and
// attribute line, which is byte for byte the listing `declare -fr` and
// `declare -fx` write.
//
// One implementation rather than three, because they are one listing: bash
// 5.3.20 answers `readonly -f`, `readonly -pf` and `declare -fr` with the
// same bytes.
func (r *Runner) attributedFunctionListing(letter rune) int {
	return r.declareFunctions(r.functionsHoldingAttributes(string(letter)), true, false, false, false)
}

// functionAttributeLine is the row a body listing writes under a function
// that holds attributes — the same `declare -f<letters> NAME` a names-only
// listing writes, and empty for a function with none.
//
// Whole-table listings only. `declare -f NAME` is the body alone in bash
// 5.3.20, so the caller decides; see Runner.declareFunctions.
func (r *Runner) functionAttributeLine(name string) string {
	attrs := r.functionAttributes(name)
	if attrs == "" {
		return ""
	}
	return "declare -f" + attrs + " " + name + "\n"
}
