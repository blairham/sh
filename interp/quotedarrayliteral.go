// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A declaration utility's operand can carry an array literal the *parser*
// never saw, because quoting hid the parentheses: `typeset -a a="(1 2)"` is
// one ordinary word, and what the builtin is handed is the four characters
// `(1 2)` rather than an element list.
//
// One column reads that text again as a literal and the others keep it as a
// value, which is [Semantics.DeclarationRereadsAParenthesizedValue] — and the
// re-read is a full one. Measured 2026-09-21 on bash 5.3.20, each line its own
// `-c`, `declare -p` behind it:
//
//	declare -a a="(1 2)"              ([0]="1" [1]="2")
//	declare -A m="([k]=v [j]=w)"      ([k]="v" [j]="w")
//	declare -ai n="(1+1 2*2)"         ([0]="2" [1]="4")
//	x="a b"; declare -a d="($x)"      ([0]="a" [1]="b")
//	declare -a e="($(echo Darwin))"   ([0]="Darwin")
//	declare -a a=(x); declare a+="(y z)"
//	                                  ([0]="x" [1]="y" [2]="z")
//
// So it is not a split of the text: the words inside are expanded where they
// stand, splitting, globbing, arithmetic and command substitution included,
// which is exactly what the element list of a written literal gets. The
// narrowest way to say that is to read the text as an assignment again and
// hand the elements to the same two functions the written form uses.
//
// Four conditions, and each is a measurement rather than a convenience:
//
//   - The word has to be a **declaration utility's**. `declare -a a; a="(1 2)"`
//     leaves `([0]="(1 2)")`, so a bare assignment to a name that is already an
//     array keeps the text.
//   - The name has to be an **array or a table** — by the letter on this line
//     or by a standing attribute. `declare a="(1 2)"` with no array anywhere is
//     `declare -- a="(1 2)"`.
//   - The operand may carry **no subscript**. `declare a[1]="(var)"` stores the
//     five characters at 1, in bash as here.
//   - The text has to be a literal **as written**: `declare -a x="(a b"` and
//     `declare -a x=" (a b) "` both keep their characters, so a missing
//     parenthesis or a space outside one is not a literal that lost its quotes.
//
// The panel splits three ways rather than two, which is why this is an axis
// and not a rule. Measured 2026-09-21 from script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, `typeset -a a="(1 2)"` then
// `echo "n=${#a[@]} zero=[${a[0]}]"`:
//
//	bash 5.3.20    n=2 zero=[1]          read again
//	bash 3.2.57    n=2 zero=[1]          read again
//	bash-as-sh     n=2 zero=[1]          read again
//	ksh93u+        n=1 zero=[(1 2)]      kept as the value
//	zsh 5.9.2      a: inconsistent type for assignment — the assignment is
//	               refused before anything asks what the text means
//	dash 0.5.12    typeset: not found
//
// zsh's row is why the axis is left unanswered there rather than set to No: a
// shell that refuses the line never reaches the question, and writing an
// answer down would be recording a measurement nobody made.

// valueReadAgainAsALiteral reports whether this declaration's value is text a
// literal lost its parentheses' meaning to, and hands back the elements it
// holds.
//
// The parse is of `name=( … )` with the operand's own name, so an element
// reading a subscript reads it against the name it is going into — the same
// text the written form would have produced, which is the whole claim this
// makes.
func (r *Runner) valueReadAgainAsALiteral(name, value string) ([]*syntax.ArrayElem, bool) {
	if len(value) < 2 || value[0] != '(' || value[len(value)-1] != ')' {
		return nil, false
	}
	if !r.arrayDeclared(name) && !r.assocDeclared(name) {
		return nil, false
	}
	if !r.ask(r.sem().DeclarationRereadsAParenthesizedValue,
		"a declaration's quoted `( … )` value read again as an array literal") {
		return nil, false
	}
	return r.parseLiteralText(name, value)
}

// parseLiteralText reads `name=( … )` and gives back the element list, or
// nothing at all when the text is no assignment.
//
// A failed parse is not a diagnostic here. bash keeps the characters for
// `declare -a x="(a))"` exactly as it keeps them for `declare -a x="(a b"`, so
// text that does not read as a literal is a value and the caller stores it.
func (r *Runner) parseLiteralText(name, text string) ([]*syntax.ArrayElem, bool) {
	// isNameLike trims before it judges, and this builds source text out of
	// the name, so a name with a space in it has to be refused rather than
	// accepted on its trimmed form.
	if name != strings.TrimSpace(name) || !isNameLike(name) {
		return nil, false
	}
	p := syntax.NewParser(name+"="+text, r.dialect())
	file := p.Parse()
	if p.Err() != nil || file == nil || len(file.Stmts) != 1 {
		return nil, false
	}
	pipe, ok := file.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 || pipe.Negated {
		return nil, false
	}
	cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(cmd.Args) != 0 || len(cmd.Redirs) != 0 || len(cmd.Assigns) != 1 {
		return nil, false
	}
	a := cmd.Assigns[0]
	if !a.IsArray || a.Index != nil || len(a.Members) > 0 || a.Name != name {
		return nil, false
	}
	return a.Elems, true
}

// arrayLiteralHiddenByQuoting is the whole of what a declaration's value
// branch has to ask: it reports whether this operand was a literal, and stores
// it where the written form would have gone.
//
// The two existing functions and not a third — a table keys the elements and
// an array counts them — so a shape the written literal already gets right is
// right here too by construction, which is the half a second store would have
// had to be kept in step with by hand.
func (r *Runner) arrayLiteralHiddenByQuoting(name, value string, appends bool) bool {
	elems, literal := r.valueReadAgainAsALiteral(name, value)
	if !literal {
		return false
	}
	if r.assocDeclared(name) {
		r.assignAssocLiteral(name, elems, appends)
		return true
	}
	r.assignArrayLiteral(name, elems, appends)
	return true
}
