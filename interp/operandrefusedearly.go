// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A declaration's array literal that its store will refuse, refused before the
// command opens its redirections and before the utility runs.
//
// Measured 2026-10-03 on bash 5.3.20, `-c`:
//
//	readonly q=1; typeset q=(b) 2>/dev/null; echo st=$?
//	    `q: readonly variable` on the shell's own standard error — the
//	    redirection never applied — no `st=`, and the next line runs at 1
//	readonly q=1; typeset q=b 2>/dev/null; echo st=$?
//	    `st=1` and nothing else: a scalar operand is the utility's, under
//	    its redirection, and gives nothing up
//	typeset -A h; typeset -a h=(x) 2>/dev/null
//	    `h: cannot convert associative to indexed array`, the same way
//	typeset -a c=(x); typeset -A c=([k]=v) 2>/dev/null
//	    `c: cannot convert indexed to associative array`
//	typeset -A h; local -a h=(x)   at the top level: the conversion, not
//	    `local: can only be used in a function`
//	readonly q=1; export q=(b), readonly q=(b), typeset -r q=(b), q+=(b)
//	    each the readonly refusal, the same way
//
// So the literal is an assignment the line makes ahead of the command, and
// its refusal is a bare assignment's: the sentence names no builtin and the
// rest of the line is given up. The letters it reads are the ones on the
// line, before the utility has read them.
//
// **Inside a function it is not this rule**, and that is measured rather than
// left out: `typeset -a u=(1 2); eee() { typeset -gA u=([k]=v); echo
// "st=$?"; }; eee` writes the assignment's sentence under the function's name
// and then the builtin's own, and the `echo` runs at 1 — see
// declare/a-refused-literal-conversion-inside-a-function — and a function's
// `local q=(b)` over a frozen global is `f: q: readonly variable` with the
// function carrying on. So a command run from a function body asks nothing
// here and keeps the store's own answer.
//
// zsh takes a frozen scalar's literal, retyping it, and converts the other
// kind of array; ksh93 refuses before the redirections too but ends the
// script and treats a scalar operand the same way, which is not this rule.
// See Semantics.ArrayOperandRefusedBeforeTheCommand.

// refuseArrayOperandsEarly reports whether one of the command's array-literal
// operands was refused ahead of the command, which then does not run.
func (r *Runner) refuseArrayOperandsEarly(argv []string) bool {
	if len(r.arrayOperands) == 0 || len(argv) == 0 {
		return false
	}
	if r.inFunc != "" {
		return false
	}
	indexed, table := declarationLettersAhead(argv)
	for i := range r.arrayOperands {
		a := r.arrayOperands[i].assign
		var refuse func()
		switch {
		case r.readonly[a.Name]:
			refuse = func() {
				r.refuseWithTheReadonlySentence(a.Name, assignedAlone, r.sem().ReadonlyReassignmentFatal)
			}
		case indexed && !a.Append && r.assocDeclared(a.Name):
			refuse = func() {
				r.diagf("%s\n", Wording(r.diag().CannotConvertTableToArrayAtTheAssignment,
					"%[1]s: cannot convert associative to indexed array", a.Name))
				r.status, r.assignFailed = 1, true
				r.abandonTheCommand()
			}
		case table && !a.Append && r.arrayDeclared(a.Name):
			refuse = func() {
				r.diagf("%s\n", Wording(r.diag().CannotConvertArrayToTableAtTheAssignment,
					"%[1]s: cannot convert indexed to associative array", a.Name))
				r.status, r.assignFailed = 1, true
				r.abandonTheCommand()
			}
		default:
			continue
		}
		if !r.ask(r.sem().ArrayOperandRefusedBeforeTheCommand,
			"a declaration's array literal refused before the command runs") {
			return r.unspecified
		}
		refuse()
		return true
	}
	return false
}

// declarationLettersAhead reads the option words in front of a declaration's
// first operand for the two kind letters, written under a minus, that the
// early refusal needs.
func declarationLettersAhead(argv []string) (indexed, table bool) {
	for _, w := range argv[1:] {
		if w == "--" || len(w) < 2 || (w[0] != '-' && w[0] != '+') {
			break
		}
		if w[0] == '-' {
			indexed = indexed || strings.ContainsRune(w[1:], 'a')
			table = table || strings.ContainsRune(w[1:], 'A')
		}
	}
	return indexed, table
}
