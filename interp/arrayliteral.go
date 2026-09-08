// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// literalElem is one element of `a=(…)` as it was written.
//
// Two shapes share the parentheses: `[sub]=value` names where the value goes,
// and a bare word takes the next position going. The shape is decided on the
// word before any expansion, for the reason `assocElem` gives — expanding
// first would hand `[2]=c` to the pattern matcher, where it is a character
// class.
type literalElem struct {
	// sub and value are the two halves of a `[sub]=value` element, each
	// already expanded as an assignment's value: no splitting and no globbing,
	// so `a=([2]=$x)` is one element however many words `$x` holds.
	sub, value string
	// subscripted distinguishes the shapes. A bare element's fields are in
	// `fields` and both strings are empty.
	subscripted bool
	// fields is what a bare element expanded to, which may be any number of
	// words: an array is built from a command's output that way.
	fields []string
}

// literalElems expands an array literal's elements once, and reports whether
// the expansion is one the assignment may be made from.
//
// Once matters: the value of an element may have side effects — `[$((i++))]=v`
// — so deciding the shape and then re-expanding to place it would run them
// twice.
//
// The second result is the whole of #1568, and it is a bool rather than a
// check inside each caller because the compiler is what makes the three of
// them agree. An element list is a *heading* in exactly the sense
// Runner.failedHeading means: it is expanded before the construct decides
// what to store, so a failure in it costs the store rather than one element
// of it. Measured 2026-09-08 in a directory where nothing matches, with
// `reply=(keep); eval "reply=(nomatch*)"; typeset -p reply`:
//
//	zsh 5.9.2            `no matches found: nomatch*`, and `reply` is `( keep )`
//	bash 5.3.15, failglob `no match: nomatch*`,          and `reply` is `([0]="keep")`
//
// — the *whole* assignment abandoned in both shells that call an unmatched
// pattern an error, not the failing element alone: `reply=(keep); eval
// "reply=(readable.sh nomatch* readable.sh)"` leaves `( keep )` in zsh and
// `([0]="keep")` in bash, and from an unset name zsh does not create `reply`
// at all — `typeset -p reply` is `no such variable`. We reported the miss and
// then stored the unexpanded pattern, which is what `~/.zi/bin/zi.zsh` then
// handed to `.` four times in a real startup.
//
// Not a dialect axis. Whether an unmatched pattern is an error already is one
// — Semantics.GlobNoMatchIsError, asked in Runner.glob — and every column that
// answers yes abandons the store. The consequence is core; only the trigger
// is dialectal.
//
// And not the glob alone, which is what makes this the store's rule rather
// than the matcher's. The same three lines with `$((1/0))` or, under `set
// -u`, `$NOPEVAR` where the pattern was leave `reply` holding `keep` in zsh,
// bash and ksh93 alike, and left us holding the words that *did* expand — at
// status 0 for the division, so the shell reported a value it had just said
// it could not compute and called it success.
func (r *Runner) literalElems(elems []*syntax.Word) ([]literalElem, bool) {
	out := make([]literalElem, 0, len(elems))
	for _, w := range elems {
		if sub, value, ok := r.assocElem(w); ok {
			out = append(out, literalElem{sub: sub, value: value, subscripted: true})
			continue
		}
		out = append(out, literalElem{fields: r.expandWord(w)})
	}
	return out, !r.failedHeading()
}

// assignArrayLiteral is `a=(…)` and `a+=(…)` on a name with no associative
// attribute.
//
// A subscripted element places its value where it says rather than becoming
// one, which is the ordinary way to build a sparse array: `a=([2]=c [0]=a)`
// is two elements, at 0 and 2, with nothing between them. It used to keep the
// text — `${a[0]}` answered the six characters `[2]=c` — and nothing reported
// it, so the array looked populated and was not.
func (r *Runner) assignArrayLiteral(name string, elems []*syntax.Word, appendTo bool) {
	parsed, ok := r.literalElems(elems)
	if !ok {
		// The elements were not read, so there is nothing to store and the
		// name keeps whatever it was holding. Before literalSubscriptIsAKey,
		// because a half-expanded list is not evidence about the shape either.
		return
	}
	if r.literalSubscriptIsAKey(parsed) {
		// The subscript is text rather than an expression, and a literal
		// written with one declares a keyed array — one concept with two
		// consequences, so the elements go where a declared name's would.
		r.markAssoc(name)
		r.assignAssocElems(name, parsed, appendTo)
		return
	}

	a := Array{}
	next := 0
	if appendTo {
		switch old, ok := r.Arrays[name]; {
		case ok:
			a = old
			// After the highest subscript rather than after the count:
			// appending to `a[0]=x a[5]=y` puts the next element at 6, which
			// is where the end is.
			next = old.pastTheEnd()
		default:
			a, next = r.appendedOverAScalar(name)
		}
	}
	if a, ok := r.literalInto(name, a, next, parsed); ok {
		r.storeArray(name, a)
	}
}

// appendedOverAScalar is what `name+=(…)` starts from where the name is not
// holding an array: the value it *is* holding becomes the first element, and
// the literal's words go after it.
//
// Core rather than an axis, and it is one of the few places the whole panel
// agrees on something this implementation was getting wrong. Measured
// 2026-09-08 with `a=1; a+=(2); typeset -p a`:
//
//	bash 5.3.15          declare -a a=([0]="1" [1]="2")
//	bash 5.3.15 as sh    declare -a a=([0]="1" [1]="2")
//	bash 3.2.57          declare -a a=([0]="1" [1]="2")
//	ksh93                typeset -a a=(1 2)
//	zsh 5.9.2            typeset -a a=( 1 2 )
//
// dash has no arrays at all and reports the parenthesis as a syntax error,
// which is the absence rather than a sixth answer. We built the array from
// the literal's words alone and answered `2`, at status 0 with a plausible
// array standing where the script's own value had been.
//
// The value lands at the *base* — the first element, whichever number that
// dialect calls it — which is what the store's positions already mean and
// what the panel says: `a=1; a+=(2); echo "[${a[0]}][${a[1]}]"` is `[1][2]`
// in bash and ksh93 and `[][1]` in zsh, and `${a[1]}` is `2` in the first
// two and `1` in the third. One rule, two spellings of it, and nothing here
// has to ask which.
//
// An empty scalar counts as a value and an unset name does not, which is the
// distinction the store already draws for us: `a=; a+=(2)` is two elements
// with an empty one in front, `unset a; a+=(2)` is the one element. The
// inherited environment counts too — `a=1 sh -c 'a+=(2)'` is `1 2` in every
// column — so the question asked is what the name *reads back as*, which is
// getVar, and not what this runner happens to have stored.
//
// Deliberately not at storeArray and not on assignForm. #1390 is the converse
// fault — a scalar written over an array leaving the array standing — and its
// fix belongs on the caller's intent, because the callers of the scalar store
// genuinely differ about whether they mean to replace the name. Here they do
// not: every route to an array-literal append wants the value kept, and what
// is being decided is what the *operator* means over a scalar. Putting this
// at the store would also promote the whole-array form `a=(2)`, which
// replaces and is already right. The two issues are converse faults on one
// pair of stores and they take their fixes at different layers.
//
// `set -A name value` is not this, though it also writes an array over a
// scalar: measured, `a=1; set -A a Q` and `a=1; set +A a Q` are both `(Q)` in
// ksh93 and zsh, the two shells with the letter. The promotion belongs to the
// append operator and not to "an array store finding a scalar", which is why
// this is a helper called from one place rather than a rule inside
// storeArray. See setArrayOperands.
// getVar rather than the two tables under it, and that choice is deliberate
// beyond the environment: a *produced* parameter is holding a value too, and
// bash promotes it — `RANDOM+=(2)` lists as `declare -ai RANDOM=([0]="19721"
// [1]="2")`, two elements. No corpus row records it, because the value is a
// new random number every run and there is nothing stable to record; a
// mutation run reading `Vars` plus `inheritedValue` instead therefore survives
// every test in the package, and it is left surviving rather than pinned with
// a test that would have to know what the generator said. Everything else that
// mutant changes is already covered: `unset` reaches the environment through
// inheritedEnv, so the two spellings agree there.
func (r *Runner) appendedOverAScalar(name string) (Array, int) {
	v, held := r.getVar(name)
	if !held {
		return Array{}, 0
	}
	return Array{0: v}, 1
}

// literalInto places a literal's elements into an array, starting at next.
//
// Taken out of assignArrayLiteral because a second construct builds the same
// value: `a[i]=(p q)` is that literal, spliced in where the subscript points,
// and it has to expand and place exactly as the whole-array spelling does —
// `a[2]=([3]=p)` fills three positions in the shell that has it, the same
// three `a=([3]=p)` fills. Folded rather than written twice, so a rule about
// what a literal *is* cannot come to differ between the two spellings.
//
// The second result is false where the placement was refused, in which case
// the script has already been ended and nothing should be stored.
func (r *Runner) literalInto(name string, a Array, next int, parsed []literalElem) (Array, bool) {
	for _, e := range parsed {
		if !e.subscripted {
			for _, f := range e.fields {
				a[next] = f
				next++
			}
			continue
		}
		idx, err := r.subscriptValue(e.sub)
		if err != nil {
			// The last of the places a subscript is read, and the one #649
			// missed: it kept a wording of its own and carried on, where the
			// two shells that evaluate a literal's subscript report the
			// arithmetic failure and end the script. The third reads the
			// text as a key and never reaches this.
			r.fatal("%s\n", r.subscriptFailure(e.sub, err))
			return nil, false
		}
		pos, ok := r.elemPos(a, idx)
		if !ok {
			// Before the first element. Refused and fatal, as the plain form
			// is — and worded apart from it by both shells that get here:
			// one names the element as written and the other the subscript.
			wording := r.diag().BadArrayLiteralSubscript
			if wording == "" {
				wording = r.diag().BadArraySubscript
			}
			r.fatal("%s\n", Wording(wording,
				"%[1]s[%[2]s]: bad array subscript", name, e.sub, e.value))
			return nil, false
		}
		a[pos] = e.value
		// A bare element after a subscripted one continues from there rather
		// than from where the count had reached: `a=(x [3]=y z)` puts z at 4.
		// Measured in both shells that accept the mixture, and it follows the
		// *written* subscript through the base, so the same literal fills the
		// same positions whichever number the first element answers to.
		next = pos + 1
	}
	return a, true
}

// literalSubscriptIsAKey asks whether a subscript inside a literal is the text
// between the brackets or an expression to evaluate.
//
// Asked only where the two readings differ. A subscript spelled as a plain
// decimal numeral evaluates to itself, so `a=([2]=c)` fills the same slot
// either way and the common form asks nothing — which is what keeps the
// construct usable in a core that has chosen no shell. `[1+1]`, `[i]` and
// `[k]` are where the answers part.
func (r *Runner) literalSubscriptIsAKey(parsed []literalElem) bool {
	for _, e := range parsed {
		if !e.subscripted || isDecimalSubscript(e.sub) {
			continue
		}
		return r.ask(r.sem().ArrayLiteralSubscriptIsAKey,
			"a subscript inside an array literal being a key rather than an expression")
	}
	return false
}

// isDecimalSubscript reports whether the text is a plain decimal integer, and
// so means the same thing read either way.
func isDecimalSubscript(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
}
