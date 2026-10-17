// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// An assignment *prefix* — `name=value cmd`, the value a command is handed
// for the length of one command — and what its right-hand side is.
//
// **It is an assignment's value, and the noun is the assignment.** That is
// the whole of this file, and it is the reading the panel is unanimous on:
// the value is expanded the way the statement form's is, so it is not split,
// not brace-expanded and **not matched against the filesystem**. Measured
// 2026-09-26 in a directory holding `a.txt b.txt c.txt one.only plain`, with
// an `echo *.txt` beside every row printing the three names so the literal
// answers are falsifiable:
//
//	                    a=*.txt printenv a   a=one.* printenv a
//	zsh 5.9.2           *.txt                one.*
//	bash 5.3.20         *.txt                one.*
//	ksh93u+ 2012-08-01  *.txt                one.*
//	dash (/bin/dash)    *.txt                one.*
//	BusyBox ash v1.37.0 *.txt                one.*
//
// This shell matched all five, in every dialect, and handed the child the
// names (#4657). The road was Runner.expandWord — the *word* pipeline, which
// globs because an ordinary word does — where the statement form has always
// taken Runner.expandAssignValue.
//
// **The wrong noun is "an assignment that stores into the shell".** It agrees
// with the right one on every row a statement can produce, and the prefix is
// exactly where the two part: `a=*.txt cmd` leaves this shell's own `a`
// untouched — `a=kept; a=*.txt printenv a; typeset -p a` is `typeset a=kept`
// in zsh 5.9.2 with the option in either state — and its value is globbed
// under the option all the same. So the pair that keys the rule is
// `a=*.txt` against `a=*.txt cmd`, which hold the noun fixed and differ in
// whether anything is stored, against `typeset a=*.txt`, which keeps the
// characters. See [Semantics.ScalarAssignmentValueIsGlobbed].
//
// Under the option the prefix follows the statement form in every measured
// respect, and the child is where the consequence shows:
//
//	setopt globassign
//	a=one.*     printenv a    one.only          a scalar, and exported
//	a=*.txt     printenv a    (nothing, 1)      an array, and arrays do not export
//	a=*.nomatch printenv a    no matches found  the command never runs
//	setopt nullglob; a=*.nomatch printenv a     an empty scalar, exported as `a=`
//
// The missing name is not a rule of its own. zsh exports no array at all —
// `a=(x y); export a; printenv a` is status 1 — so a prefix whose pattern
// matched more than one name has nothing to hand over, and the name is
// **absent** from the child's environment rather than left showing what the
// shell had: `export a=old; setopt globassign; a=*.txt printenv a` is status
// 1 and not `old`. Inside a function, which forks nothing, the same prefix is
// visible as `typeset -g -a a=( a.txt b.txt c.txt )`.

// prefixGlobMatch is one prefix assignment's match, held for the length of
// the command it stands in front of.
//
// A slice rather than a map, the arrangement prefixTraceAssigns gives its
// reasons for: a prefix is one, two or three assignments, so the scan is
// shorter than hashing a pointer, and a map on the Runner is a *table* every
// clone then has to own.
//
// An array literal written as a prefix is held here too, in the dialects that
// read it as an array: elems is what its elements expanded to and literal says
// which of the two this entry is. One record rather than two, because both are
// "this prefix is a value the text alone does not say", both are dropped at
// the end of the command, and both are an array no child's environment can
// carry (#6547).
type prefixGlobMatch struct {
	assign  *syntax.Assign
	fields  []string
	elems   []literalElem
	literal bool
}

// prefixAssignValue is a prefix assignment's own value, expanded once.
//
// The text, and — where this shell asked for the reading and the value turned
// out to be a pattern — the names it matched. The fields are recorded against
// the assignment rather than returned, because the four routes a prefixed
// command takes each reach the expansion through Runner.prefixExpansion and
// only three of them care what the match was.
func (r *Runner) prefixAssignValue(a *syntax.Assign) string {
	// Who a failure in here is located as, in the one column that chooses:
	// armed for the expansion and put away after it, so a builtin that runs
	// commands of its own does not lend them a speaker. See
	// interp/prefixfailurelocation.go.
	defer r.speakForAPrefixValue()()
	if a.IsArray && a.Index == nil && len(a.Members) == 0 {
		return r.expandPrefixArrayLiteral(a)
	}
	value, fields, globbed := r.expandScalarAssignValue(a.Value)
	if globbed {
		r.prefixGlobMatches = append(r.prefixGlobMatches, prefixGlobMatch{assign: a, fields: fields})
	}
	return value
}

// prefixGlobFields is what this prefix's value matched, and whether it was
// read as a pattern at all.
//
// The second return rather than a nil check, for the reason
// expandedAssign.fieldsSet exists: no fields is a real answer — `setopt
// globassign nullglob; a=*.nomatch cmd` hands the child an empty `a` — and it
// is not the same answer as "this prefix never globbed".
func (r *Runner) prefixGlobFields(a *syntax.Assign) ([]string, bool) {
	for _, m := range r.prefixGlobMatches {
		if m.assign == a && !m.literal {
			return m.fields, true
		}
	}
	return nil, false
}

// prefixArrayLiteral is what this prefix's array literal expanded to, in the
// dialects that read one as an array, and whether it was read that way.
func (r *Runner) prefixArrayLiteral(a *syntax.Assign) ([]literalElem, bool) {
	for _, m := range r.prefixGlobMatches {
		if m.assign == a && m.literal {
			return m.elems, true
		}
	}
	return nil, false
}

// prefixStoresAnArray reports whether this prefix puts an **array** under its
// name: a value that matched more than one name, or an array literal read as
// the array it spells. An array is what no child's environment can carry, and
// what no export attribute goes on — measured 2026-10-15 on zsh 5.9.2,
// `export b=old; f(){ typeset -p b; }; b=(x y) f` lists `typeset -g -a b=( x
// y )`, the attribute gone, where `b=z f` lists it exported (#6547).
func (r *Runner) prefixStoresAnArray(a *syntax.Assign) bool {
	if _, ok := r.prefixArrayLiteral(a); ok {
		return true
	}
	if _, ok := r.prefixAppendsAnElement(a); ok {
		return true
	}
	fields, ok := r.prefixGlobFields(a)
	return ok && len(fields) > 1
}

// prefixAppendsAnElement reports whether this prefix is a scalar `+=` onto a
// name holding an indexed array in the dialect where that adds an element,
// and hands back the array it adds to.
//
// The statement's own reading, Semantics.ScalarAppendedToAnArrayBecomesANewElement,
// read as a prefix: measured 2026-10-15 on zsh 5.9.2 `-f`, `a=(x y); a+=z f`
// lists `typeset -g -a a=( x y z )` in the body and `a+=z /usr/bin/printenv
// a` hands the child no `a` at all — an array, which no environment
// carries. The columns that join the first element store the scalar they
// always did (#6751).
func (r *Runner) prefixAppendsAnElement(a *syntax.Assign) (Array, bool) {
	if !a.Append || a.IsArray || a.Index != nil || len(a.Leading) > 0 {
		return nil, false
	}
	if _, globbed := r.prefixGlobFields(a); globbed {
		return nil, false
	}
	name := r.prefixEntryName(a.Name)
	if r.assocDeclared(name) {
		return nil, false
	}
	arr, ok := r.arraysLookup(name)
	if !ok || r.sem().ScalarAppendedToAnArrayBecomesANewElement != Yes {
		return nil, false
	}
	return arr, true
}

// storeGlobbedPrefix puts a matched prefix where the name is, for the length
// of the command.
//
// The statement form's store in every respect but the cell it lands in — see
// Runner.storeGlobbedScalarAssign, which this is deliberately the twin of.
// **How many names matched decides the kind**, the numeric attributes go
// whichever it is, and the append is not an append: measured on zsh 5.9.2,
// 2026-09-26, `setopt globassign; a=x; a+=one.* printenv a` is `one.only`
// and not `xone.only`, so a match replaces the name whichever operator was
// written — which is why a.Append is not consulted here.
//
// A single match is a *scalar* and not a one-element array, and zero under
// `nullglob` is an **empty scalar**: `a=*.nomatch(N) f` shows the body an
// exported `a` holding nothing. The three-way split is the row an
// implementation that rewrote the prefix as an array gets wrong at status 0.
func (r *Runner) storeGlobbedPrefix(a *syntax.Assign, fields []string) {
	r.clearTypeAttributes(a.Name)
	if len(fields) > 1 {
		r.setArray(a.Name, fields)
		return
	}
	value := ""
	if len(fields) == 1 {
		value = fields[0]
	}
	r.setVarAs(a.Name, value, assignedAlone)
}

// prefixGlobTraced is the record `set -x` needs to render a matched prefix,
// and nil for every prefix that was not read as a pattern.
//
// The trace says what the command was handed rather than what the script
// typed: measured on zsh 5.9.2, 2026-09-26, `setopt globassign xtrace;
// a=*.txt true` writes `a=( a.txt b.txt c.txt )` — the element list a written
// literal gets, from a line with no parentheses in it — while one match
// writes the value bare and no match writes an empty quoted value. The same
// rendering the statement form reaches, and reached the same way. See
// Runner.traceAssign.
func (r *Runner) prefixGlobTraced(a *syntax.Assign) *expandedAssign {
	if elems, ok := r.prefixArrayLiteral(a); ok {
		// What the elements came to, for the columns whose trace writes
		// that: `set -x; x=(a $(echo b)) true` is `x=( a b )` in zsh 5.9.2
		// and ksh93u+, measured 2026-10-15 (#6547).
		return &expandedAssign{assign: a, elems: elems, elemsSet: true}
	}
	fields, ok := r.prefixGlobFields(a)
	if !ok {
		return nil
	}
	return &expandedAssign{assign: a, fields: fields, fieldsSet: true}
}

// withoutPrefixArrayNames takes the named entries out of the environment a
// child is about to be given.
//
// **An array reaches no child's environment**, and taking the name out is not
// the same as leaving it off: a prefix whose pattern matched more than one
// name has to *shadow* whatever the shell was exporting under it. Measured on
// zsh 5.9.2, 2026-09-26: `export a=old; setopt globassign; a=*.txt printenv
// a` prints nothing and exits 1, where the same line with the option off
// prints `*.txt` and the same line over `a=one.*` prints `one.only`. So the
// child is shown neither the match nor the old value.
//
// The mechanism is zsh's and not this construct's — `a=(x y); export a;
// printenv a` exits 1 there too — which is why the removal is written as
// "this name carries an array now" rather than as a rule about prefixes.
func withoutPrefixArrayNames(env, names []string) []string {
	if len(names) == 0 {
		return env
	}
	kept := env[:0:0]
	for _, entry := range env {
		drop := false
		for _, name := range names {
			if len(entry) > len(name) && entry[len(name)] == '=' && entry[:len(name)] == name {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, entry)
		}
	}
	return kept
}

// expandPrefixArrayLiteral expands an array literal written as a prefix and
// hands back the scalar the routes carry for it.
//
// **The elements are expanded in every shell that reads the literal there**,
// refused name or not. Measured 2026-10-15 from `-c` and from a script file
// under `env -i PATH=/usr/bin:/bin`:
//
//	x=($(echo s >&2) b) /bin/echo ran              bash 5.3.20, ksh93u+, zsh 5.9.2: `s`, `ran`
//	readonly r=1; r=($(echo s >&2)) true           zsh: `s`, then the refusal
//	readonly r=1; r=($(echo s >&2)) /bin/echo ran  zsh, ksh93: `s`, then the refusal
//	HOME=($(echo s >&2)) true                      zsh: `s`, then the array sentence
//
// The value read for a literal was its scalar text, which a literal does not
// have, so no substitution inside the parentheses ever ran (#6512). Whether
// the elements are expanded at all for a refused name is still
// Semantics.FrozenPrefixIsCheckedBeforeItsValue's to say: every route that
// skips a frozen name's value skips it before reaching here.
//
// **What the prefix stores** is Semantics.PrefixArrayLiteralIsAnArray's. An
// array is expanded through literalElems, which is how every literal's
// elements are read, and held for the store and the trace — the scalar handed
// back is never stored. The text is a word of its own, made of the elements'
// spans with a space between each and the parentheses around them, expanded
// as an assignment's value is: nothing in it is split or globbed, and a
// tilde or a brace at the front of an element is not at the front of the word
// (#6547).
func (r *Runner) expandPrefixArrayLiteral(a *syntax.Assign) string {
	if !r.ask(r.sem().PrefixArrayLiteralIsAnArray,
		"an array literal written as an assignment prefix being an array") {
		if r.unspecified {
			return ""
		}
		return r.expandAssignValue(prefixArrayLiteralText(a))
	}
	elems, ok := r.literalElems(a.Elems, r.literalShapeReadsSubscripts(a.Elems), false)
	if !ok {
		return ""
	}
	r.prefixGlobMatches = append(r.prefixGlobMatches,
		prefixGlobMatch{assign: a, elems: elems, literal: true})
	return ""
}

// prefixArrayLiteralText is the word an array literal is read as where it is
// read as its text: `(`, each element's spans with a space between them, and
// `)`. The three added spans are quoted so that nothing in them is read: the
// parenthesis at the front is what keeps an element's tilde from being the
// start of the value.
func prefixArrayLiteralText(a *syntax.Assign) *syntax.Word {
	quoted := func(text string) syntax.Span {
		return syntax.Span{Kind: syntax.Literal, Quoting: syntax.SingleQuoted, Value: text, Pos: a.Pos()}
	}
	spans := []syntax.Span{quoted("(")}
	for i, el := range a.Elems {
		if i > 0 {
			spans = append(spans, quoted(" "))
		}
		if el.Word != nil {
			spans = append(spans, el.Word.Spans...)
		}
	}
	spans = append(spans, quoted(")"))
	return &syntax.Word{Spans: spans, Start: a.Pos(), Stop: a.End()}
}
