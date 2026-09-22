// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A `name=( … )` operand is a *grammar* question and what becomes of it is
// not: [syntax.Dialect.DeclarationUtilities] says which command words the
// parser may read one behind, and some of those words do not declare anything.
//
// The one in the panel is `eval`. Measured 2026-09-20 from script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, one word per file, `<word> foo=(1 2)`
// followed by `echo OK`:
//
//	word        bash 5.3.20            ksh93u+        dash 0.5.12
//	eval        OK                     syntax error   syntax error
//	declare     OK                     syntax error   —
//	typeset     OK                     OK             —
//	export      OK                     OK             —
//	readonly    OK                     OK             —
//	local       can only be used …     —              —
//	alias       alias: foo: not found  syntax error   —
//	echo        syntax error           syntax error   syntax error
//	command     syntax error           syntax error   syntax error
//	printf      syntax error           syntax error   syntax error
//	:           syntax error           syntax error   syntax error
//	set         syntax error           syntax error   syntax error
//	unset       syntax error           syntax error   syntax error
//	a function  syntax error           syntax error   syntax error
//	/bin/echo   syntax error           syntax error   syntax error
//
// So the word list is neither "every builtin" nor "the declarations": it is a
// list, and `eval` is on bash's and on nobody else's.
//
// What `eval` receives is **one word**, the assignment written out again with
// each element expanded and the elements joined by a space — not the name, and
// not a declaration this shell performs afterwards. Three measurements pin
// that shape, all bash 5.3.20 and all from script files:
//
//	eval foo=(1 "a b"); declare -p foo    ([0]="1" [1]="a" [2]="b")
//	x="p q"; eval foo=($x)                ([0]="p" [1]="q")
//	x='a"b'; eval foo=("$x")              unexpected EOF while looking for matching `"'
//
// The first says the quotes are gone by the time `eval` reads the word — a
// quoted `a b` would be one element if the word were re-quoted, and is two.
// The third is the one that separates the two readings that survive the first:
// the elements are expanded **outside** and joined, so a quote a parameter was
// holding lands in the text `eval` then parses. Had the word been handed on as
// written, with `$x` expanding inside `eval` instead, that line would have
// stored the three characters and reported 0.
//
// An empty literal, a keyed element and the appending spelling all follow from
// the same rule and were measured beside it: `eval foo=()` leaves an empty
// array, `eval foo=([2]=z)` leaves one element at 2, and `foo=(x); eval
// foo+=(y z)` leaves three.

// RejoinArrayOperand says this utility takes a `name=( … )` operand as one
// word rather than as a declaration to perform.
//
// The seam a dialect registers the word at, beside [Runner.RegisterBuiltin]:
// the grammar already had to be told the word may be followed by a compound
// assignment, and this is the other half — without it the operand reaches the
// utility as the bare name and the array is stored afterwards, which is what a
// declaration wants and turns `eval foo=(1 2)` into a run of the command
// `foo`.
//
// Names rather than a predicate on the builtin, because it is a fact about the
// *word* and not about what the word resolves to: `eval` is a builtin here and
// the question is asked of the command line before anything has been looked
// up.
func (r *Runner) RejoinArrayOperand(name string) {
	if r.rejoinedOperands == nil {
		r.rejoinedOperands = map[string]bool{}
	}
	r.rejoinedOperands[name] = true
}

// rejoinsArrayOperand reports whether the utility this command names takes its
// array-literal operands as words.
func (r *Runner) rejoinsArrayOperand(word string) bool {
	return r.rejoinedOperands[word]
}

// rejoinedArrayOperand is the assignment written out again, with each element
// expanded — and it is **words** rather than one word, because the utility is
// handed an ordinary word and an ordinary word is field-split.
//
// Nothing is quoted on the way out, which is the measurement above and not an
// omission — the text is what the utility goes on to read, so a quote a value
// was carrying is syntax there.
//
// The literal text of the assignment is not a split point and the fields an
// expansion produced are: `a=(`, the blank between two elements and `)` are
// written where they stand, and a break is taken only *between* the fields of
// one element. Measured 2026-09-21 on bash 5.3.20, with a shell function
// standing in for the builtin and `x="p q"`:
//
//	eval a=("p q" r)   <a=(p q r)>        one field each, so one word
//	eval a=($x)        <a=(p><q)>         the break the expansion made
//	eval a=($x r)      <a=(p><q r)>       ` r)` is literal and joins `q`
//	eval a=(r $x)      <a=(r p><q)>       the same from the other side
//	eval a=($x $y)     <a=(p><q s><t)>    two elements, `y="s t"`
//	x=""; eval a=($x r)   <a=( r)>        the separator stands with no field
//
// The third row is the discriminator between the two readings that survive
// the second: a rule that split one word per *element* would give three words
// there and bash gives two. The last row is why the separator is written per
// element rather than before each field — an element that expanded to nothing
// still had a blank written after it.
//
// The subscripted spelling stays one field: `[sub]=value` is read with the
// value expanded as an assignment's, so `eval a=([2]=$x r)` is `a=([2]=p q r)`
// here where bash — which never reads the subscript in a word it is only
// going to hand on — splits it. The two are the same text once a rejoining
// utility joins its arguments with a blank, which is what `eval` does, so the
// difference is reachable only through `let`. Not modeled, and recorded here
// rather than left silent.
func (r *Runner) rejoinedArrayOperand(a *syntax.Assign) []string {
	var b strings.Builder
	b.WriteString(a.Name)
	if a.Append {
		b.WriteString("+")
	}
	b.WriteString("=(")
	words := []string{}
	// open is the word still being written, which every literal run joins and
	// only a field boundary closes.
	open := func(s string) { b.WriteString(s) }
	brk := func(s string) {
		words = append(words, b.String())
		b.Reset()
		b.WriteString(s)
	}
	elems, ok := r.literalElems(a.Elems,
		r.literalReadsSubscripts(a.Name, a.Elems, a.Append),
		r.bareLiteralElementIsOneValue(a.Name, a.Elems, false))
	if !ok {
		// The element list failed — a division by zero, an unmatched pattern
		// where the dialect calls that an error. The command does not run
		// either way; the word is closed so that what is handed on is at
		// least a shape, rather than an unterminated parenthesis that would
		// make the utility's own diagnostic the one a reader sees.
		open(")")
		return append(words, b.String())
	}
	for i, el := range elems {
		if i > 0 {
			// The blank between two elements, which is the assignment's own
			// text and so never a split point.
			open(" ")
		}
		if el.subscripted {
			op := "="
			if el.appendValue {
				op = "+="
			}
			open("[" + el.sub + "]" + op + el.value)
			continue
		}
		for j, f := range el.fields {
			if j == 0 {
				open(f)
				continue
			}
			brk(f)
		}
	}
	open(")")
	return append(words, b.String())
}
