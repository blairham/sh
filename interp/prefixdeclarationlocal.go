// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// An assignment prefix the shell keeps, in front of a declaration that is
// declaring locals of the running function.
//
// ksh93 keeps a prefix in front of `typeset`, because `typeset` is a special
// builtin there. Inside a `function`-form body the prefix becomes the body's
// local. A function the body calls does not see it, and the caller gets its own
// value back at the return:
//
//	function g { echo "g v=$v"; }
//	function f { v=new typeset y=1; echo "in v=$v"; g; }; v=old; f; echo "out v=$v"
//
// That is `in v=new`, `g v=old`, `out v=old` in ksh93u+. This shell stored the
// prefix in the cell the name already reached, which was the global, so all
// three read `new`. dash, BusyBox ash and zsh under `posixbuiltins` keep a
// prefix in front of `local`, and there it is an ordinary assignment. See
// Semantics.PrefixToALocalDeclarationIsALocal for the panel.
//
// **The value comes first, and then the cell.** `v=${v}x typeset y=1` gives the
// body `oldx`, so the value is expanded against the name the body could
// already see. `v+=x typeset y=1` gives it `x`, so the append joins a cell
// that is already fresh. So each entry is expanded, then made a local, then
// joined and stored. Three places do that for one command, each between the
// expansion and the join:
//
//   - the walk in front of a declaration's first operand, which holds each
//     value for the operands to see (interp/prefixoperandorder.go). Holding
//     the value in the local is what keeps a `function`-form function called
//     from an operand reading the shell's own name: with `g` above, `function
//     f { v=new typeset y=$(g); }` gives `y` the `g v=old`. It is also what
//     lets a stopped operand walk keep its prefix and still lose it at the
//     return: `trap 'echo "trap v=$v"' EXIT; function f { v=new typeset
//     y=${q?boom}; }; v=old; f` is `trap v=old` in ksh93u+, and was `trap
//     v=new` here after #6999 kept a stopped walk's prefix.
//   - the walk ahead of the redirections, which expands every value of a
//     prefix that persists, in the columns that make it
//     (interp/prefixredirorder.go). A `function`-form function called from a
//     later entry's value then reads the shell's own name: with `g` writing
//     `$v`, `function f { v=new w=$(g) typeset y; }` gives `w` the `old` in
//     ksh93u+.
//   - the builtin route, which makes the store that stays. A value neither
//     walk expanded is expanded here first — see
//     prefixEntryBecomesLocalAfterItsValue.
//
// A call that finds the cell made already leaves it as it stands.

// prefixLocalDeclaration is the declaration word this command's prefix becomes
// a local of the running function under, and "" where it is an ordinary
// assignment. ok is false where the axis was put and is unanswered, which has
// been said and stops the command.
//
// Asked only where the columns part: a prefix the shell keeps, in front of
// `typeset` or `local`, where that word would declare locals of the running
// function. `typeset` declares where its own locals go, including the
// `function`-form call a POSIX-form function runs in where the dialect gives
// only that form a scope. A prefix in front of a function named `typeset`
// asks nothing, because the prefix is not in front of the declaration.
func (r *Runner) prefixLocalDeclaration(assigns []*syntax.Assign, argv []string) (word string, ok bool) {
	if len(r.scopes) == 0 || !aPrefixIsWritten(assigns) {
		return "", true
	}
	k := traceDeclarationUtilityWord(argv)
	if k >= len(argv) {
		return "", true
	}
	word = argv[k]
	if word != "typeset" && word != "local" {
		return "", true
	}
	kind := r.prefixCommandOf(argv)
	if kind.kind != prefixBeforeSpecialBuiltin || !r.prefixPersistsAtThisBuiltin(argv[0], kind) {
		return "", true
	}
	if word == "typeset" {
		undo := r.declareInTheKeywordCallersScope()
		local := r.typesetDeclaresALocalHere()
		undo()
		if !local {
			return "", true
		}
	}
	if r.ask(r.sem().PrefixToALocalDeclarationIsALocal,
		"a kept prefix in front of a declaration being a local of the function") {
		return word, true
	}
	return "", r.sem().PrefixToALocalDeclarationIsALocal != Unspecified
}

// prefixEntryBecomesLocalAfterItsValue is prefixEntryBecomesLocal for the
// builtin route, which is the one place a value may not have been expanded
// yet: in a column that opens the redirections before it touches the prefix,
// nothing walks it ahead of the store. The value is expanded and recorded
// first, so the store reads it back rather than expanding it against the fresh
// cell. A subscripted entry is left to the store's own order, which reads its
// subscript ahead of its value.
func (r *Runner) prefixEntryBecomesLocalAfterItsValue(a *syntax.Assign, word string) {
	if word == "" {
		return
	}
	if _, done := r.prefixTraceValue(a); !done && !prefixIsSubscripted(a) {
		r.recordPrefixTraceValue(a, r.prefixExpansion(a))
	}
	r.prefixEntryBecomesLocal(a, word)
}

// prefixEntryBecomesLocal makes the cell this prefix entry is about to be held
// or stored in a local of the running function, declared the way the word it
// stands in front of declares its own. A fresh local starts with nothing: no
// value, no elements and no letters. ksh93u+ shows that in three ways.
// `typeset -i v=1; function f { v=2+3 typeset y=1; echo $v; }` writes `2+3`.
// `v+=x` joins nothing. And the cell is not exported over an exported name.
//
// The cell is the name as written, the way a declaration's own operand names
// it. Measured 2026-10-29 on ksh93u+, two rows:
//
//	a=(p q r); function f { a[1]=B typeset y=1; echo "${!a[@]} ${a[*]}"; }; f
//	                                   1 B — a fresh array, and `p q r` after
//	typeset -n r=v; function f { r=new typeset y=1; echo "$r $v"; }; v=old; f
//	                                   new old — a fresh `r`, not through the reference
//
// A name the function already holds a local of is that local, and is left as
// it stands: `function f { typeset v=mine; v=new typeset y=1; }` writes the
// body's own `v`, and `function f { typeset -n r=v; r=new typeset y=1; }`
// writes the shell's `v` through the body's own reference. Nothing is done
// where word is "", or where the value would not expand. Every caller skips an
// operand and a frozen name before it gets here. The frozen name is #7010.
func (r *Runner) prefixEntryBecomesLocal(a *syntax.Assign, word string) {
	if word == "" || r.assignmentGaveUp() {
		// Not a local, or a value that would not expand: the command is given
		// up and nothing is stored.
		return
	}
	shadow := r.shadow
	if word == "typeset" {
		defer r.declareInTheKeywordCallersScope()()
		shadow = r.shadowTypeset
	}
	if fresh, _ := shadow(a.Name); !fresh {
		return
	}
	r.emptyPrefixEntry(a.Name)
	r.hideVar(a.Name)
	// The export attribute, which the declaration's own fresh locals answer
	// the same question about.
	r.localExportAttribute(a.Name, false)
}
