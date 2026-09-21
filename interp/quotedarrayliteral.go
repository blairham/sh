// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
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
//     five characters at 1, in bash as here — with a sentence beside it, which
//     is Runner.warnQuotedCompoundAtASubscript. **Unless the array letter is
//     written on the same line**, where the subscript is dropped instead and
//     the value comes here after all: see Runner.letterDropsTheSubscript
//     (#4105).
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

// errTextIsNoLiteral is the answer that means **the re-read was never
// attempted**, and it is the distinction the whole of this file turns on.
//
// `declare -a x="(a b"` keeps its characters in bash, and so does
// `declare -a x=" (a b) "` — a text with no closing parenthesis, or with the
// parentheses not at its ends, is not a literal that lost its quotes and
// never reaches the re-read at all. `declare -a x="(a))"` does reach it, and
// there a failure is bash's own diagnostic rather than a fallback to the
// characters (#4035).
//
// The two were one branch and one boolean before, which is why the failing
// text was stored: "did not parse" and "is not of this shape" gave the same
// answer, so the only behavior the caller could have was the safe one.
var errTextIsNoLiteral = errors.New("the value is not a parenthesized literal")

// valueReadAgainAsALiteral reports whether this declaration's value is text a
// literal lost its parentheses' meaning to, and hands back the elements it
// holds.
//
// The parse is of `name=( … )` with the operand's own name, so an element
// reading a subscript reads it against the name it is going into — the same
// text the written form would have produced, which is the whole claim this
// makes.
//
// errTextIsNoLiteral means the shape test said no and the characters stand.
// Any other error is the re-read's own failure, which the caller reports.
func (r *Runner) valueReadAgainAsALiteral(name, value string) ([]*syntax.ArrayElem, error) {
	if len(value) < 2 || value[0] != '(' || value[len(value)-1] != ')' {
		return nil, errTextIsNoLiteral
	}
	if !r.arrayDeclared(name) && !r.assocDeclared(name) {
		return nil, errTextIsNoLiteral
	}
	if !r.ask(r.sem().DeclarationRereadsAParenthesizedValue,
		"a declaration's quoted `( … )` value read again as an array literal") {
		return nil, errTextIsNoLiteral
	}
	return r.parseLiteralText(name, value)
}

// parseLiteralText reads `name=( … )` and gives back the element list, the
// parse failure that stopped it, or errTextIsNoLiteral where the text was
// never the re-read's to judge.
//
// A failed parse **is** a diagnostic here, and the shape test above is what
// earns that: by the time this is called the value has the parentheses at its
// ends and the name is an array, so the text is a literal the quoting hid and
// a parser that will not take it is bash writing `array assign: syntax
// error`. See Diagnostics.QuotedArrayLiteralFailureNames.
func (r *Runner) parseLiteralText(name, text string) ([]*syntax.ArrayElem, error) {
	// isNameLike trims before it judges, and this builds source text out of
	// the name, so a name with a space in it has to be refused rather than
	// accepted on its trimmed form. Not a failure of the re-read: a name the
	// parser could not write down is one no declaration reaches the re-read
	// with, so the characters stand as they always did.
	if name != strings.TrimSpace(name) || !isNameLike(name) {
		return nil, errTextIsNoLiteral
	}
	src := name + "=" + text
	p := syntax.NewParser(src, r.dialect())
	file := p.Parse()
	if err := p.Err(); err != nil {
		return nil, err
	}
	if file == nil {
		return nil, errTextIsNoLiteral
	}
	if len(file.Stmts) != 1 {
		// `declare -a x="(a);(b)"` parses cleanly as two statements and is
		// refused there exactly as the `&&` spelling is, so the count is a
		// shape failure and not a reason to keep the characters.
		return nil, r.literalTextHasMoreThanTheAssignment(file, src)
	}
	pipe, ok := file.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 || pipe.Negated {
		return nil, r.literalTextHasMoreThanTheAssignment(file, src)
	}
	cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(cmd.Args) != 0 || len(cmd.Redirs) != 0 || len(cmd.Assigns) != 1 {
		return nil, r.literalTextHasMoreThanTheAssignment(file, src)
	}
	a := cmd.Assigns[0]
	if !a.IsArray || a.Index != nil || len(a.Members) > 0 || a.Name != name {
		return nil, r.literalTextHasMoreThanTheAssignment(file, src)
	}
	return a.Elems, nil
}

// literalTextHasMoreThanTheAssignment is the failure for a text that parsed
// and came out as something other than one array literal: `declare -a
// x="(a) (b)"` and `declare -a x="(a)&&(b)"` both hold a closing parenthesis
// in the middle, so the shape test passes and the re-read finds a second
// command behind the literal.
//
// bash refuses these exactly as it refuses a text that will not parse at all,
// and blames the parenthesis that closed the literal early — measured
// 2026-09-21 on bash 5.3.20, `declare -a x="(a) (b)"` is `syntax error near
// unexpected token `)'` quoting `a) (b`. So the token is the one the grammar
// stopped wanting words at, which is where the assignment ended.
//
// An error built here rather than one the parser handed back, because the
// parser did not refuse: what is wrong is the *shape* and the shape is this
// caller's question. Nothing else in the package needs it, so it is written
// at the one site that can say what it means.
func (r *Runner) literalTextHasMoreThanTheAssignment(file *syntax.File, src string) error {
	stop := syntax.Pos{}
	if len(file.Stmts) > 0 {
		stop = file.Stmts[0].End()
	}
	token := ")"
	if off := int(stop.Offset); off > 0 && off <= len(src) {
		// The character the literal ended on, which is its closing
		// parenthesis wherever the text put one.
		token = src[off-1 : off]
	}
	return &syntax.Error{
		Kind:  syntax.ErrUnexpected,
		Token: token,
		Pos:   stop,
	}
}

// arrayLiteralHiddenByQuoting is the whole of what a declaration's value
// branch has to ask: it reports whether this operand was a literal, and stores
// it where the written form would have gone.
//
// The two existing functions and not a third — a table keys the elements and
// an array counts them — so a shape the written literal already gets right is
// right here too by construction, which is the half a second store would have
// had to be kept in step with by hand.
//
// A text that reached the re-read and would not parse is handled here as
// well, and `true` is the answer for it: the operand has been dealt with, and
// dealing with it was refusing it. The caller must read the give-up rather
// than only the store — see Runner.operandGaveUpTheBuiltin.
func (r *Runner) arrayLiteralHiddenByQuoting(name, value string, appends bool) bool {
	elems, err := r.valueReadAgainAsALiteral(name, value)
	switch {
	case errors.Is(err, errTextIsNoLiteral):
		return false
	case err != nil:
		if r.diag().QuotedArrayLiteralFailureNames == "" {
			// This dialect re-reads and has no sentence for a text that
			// will not come out as a literal, so the characters stand —
			// which is what every column did before the wording arrived.
			return false
		}
		r.refuseTheHiddenLiteral(value, err)
		return true
	}
	if r.assocDeclared(name) {
		r.assignAssocLiteral(name, elems, appends)
		return true
	}
	r.assignArrayLiteral(name, elems, appends)
	return true
}

// refuseTheHiddenLiteral writes what bash writes for a `( … )` value the
// re-read could not take, and gives up the rest of the input line.
//
// Two lines where the failure names a token the grammar did not want and one
// where the input simply ran out, which is Diagnostics.offendingLine's own
// rule and the reason this hands it the *inside* of the parentheses: that
// text is the re-read's whole input, so indexing it by the failure's line is
// indexing the right program. Quoting the script's line instead would have
// echoed the `declare` command back, which no shell does here.
//
// The status is 1 and the give-up is controlAbandon — the command and the
// rest of its line, and the next line runs. Not the `-c`-ends-whole rule a
// bad subscript carries: measured 2026-09-21, `bash -c $'declare -a
// x="(a;b)"\necho three'` prints `three` and exits 0, where
// `bash -c 'a=(1); a[b c]=v'` never reaches its second command.
func (r *Runner) refuseTheHiddenLiteral(value string, err error) {
	d := r.diag()
	construct := d.QuotedArrayLiteralFailureNames
	inside := value[1 : len(value)-1]
	// The failure's line inside the re-read's own text, added to the line the
	// declaration was written on. A single-line value leaves the line alone,
	// which is every text a script writes.
	at := d.ParseFailureLine(err)
	was := r.line
	if at > 1 {
		r.line = was + at - 1
	}
	r.errf("%s", r.diagLineNamed(construct, "%s\n", d.ParseFailure(err)))
	if echo := d.offendingLine(at, err, inside); echo != "" {
		r.errf("%s", r.diagLineNamed(construct, "%s", echo))
	}
	r.line = was
	r.status = 1
	r.abandonTheCommand()
}
