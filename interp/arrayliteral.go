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
	// appendValue marks the `[sub]+=value` spelling, which joins what the
	// element already holds rather than replacing it. Only ever true
	// alongside subscripted: the append spelling of a *bare* element is not
	// a shape a literal has.
	appendValue bool
	// fields is what a bare element expanded to, which may be any number of
	// words: an array is built from a command's output that way.
	fields []string
	// nested is the value a literal standing in an element's place built, and
	// nil where the element is an ordinary one. One element whatever the
	// literal held, and not a list: `a=( (1 2) (3 4) )` is two elements, each
	// an array, which is the whole of what makes it a dimension rather than a
	// splice.
	nested *Element
	// members is the **compound variable's body** a literal standing in an
	// element's place was written as, and nil for every other element —
	// `nested` included, since the same parentheses hold the two constructs
	// and the first word decides which was written. See
	// [syntax.Parser.nestedArrayLiteral].
	//
	// Carried rather than run, which is what separates it from `nested`: a
	// compound's members are ordinary names spelled with the element's own
	// subscript in front, so the body cannot be run until the placement below
	// has said which element this is. `nested` has no such need and is built
	// where it is read.
	members []*syntax.SimpleCmd
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
// readsSubscripts says whether a `[sub]=value` element names where its value
// goes or is an ordinary word with a bracket in front of it. True everywhere
// but one dialect; see [Runner.literalReadsSubscripts], which is the whole of
// what decides it and where it is measured.
//
// bareIsOneValue says the elements that carry no such head are each **one**
// field rather than as many as their expansion comes to — the keyed literal's
// reading, and the whole of Semantics.BareElementsInATableLiteralAreEachOneValue.
// See [Runner.bareLiteralElementIsOneValue], which is where the question is
// put and which is the only thing that ever passes this true.
func (r *Runner) literalElems(elems []*syntax.ArrayElem, readsSubscripts, bareIsOneValue bool) ([]literalElem, bool) {
	if parsed, ok := r.takeExpandedElements(elems); ok {
		// Already expanded, by the caller that is about to trace what they
		// came to. See Runner.assignAll.
		return parsed, true
	}
	out := make([]literalElem, 0, len(elems))
	for _, el := range elems {
		if el.Nested != nil {
			if el.Nested.Members != nil {
				// The parentheses held a compound variable's body rather than
				// an array's elements. Carried to the placement, which is
				// where the element it lands in — and so the name its members
				// hang under — is known. See literalElem.members.
				if el.Word == nil {
					out = append(out, literalElem{members: el.Nested.Members})
					continue
				}
				sub, _, appends, ok := r.assocElem(el.Word)
				if !ok {
					return nil, false
				}
				out = append(out, literalElem{
					sub: sub, subscripted: true, appendValue: appends,
					members: el.Nested.Members,
				})
				continue
			}
			// A literal of its own, which becomes one element holding what it
			// built rather than words spliced in around it. Built through the
			// same placement every literal uses, so a nested one with a
			// subscript in it leaves the same gap the outer spelling leaves.
			value, ok := r.nestedLiteral("", el.Nested.Elems)
			if !ok {
				return nil, false
			}
			if el.Word == nil {
				out = append(out, literalElem{nested: &value})
				continue
			}
			// A `[sub]=` head with the literal as its value, which places
			// what it built where the subscript says rather than at the next
			// position. The head's own halves are read by the same split
			// every other subscripted element takes.
			sub, _, appends, ok := r.assocElem(el.Word)
			if !ok {
				return nil, false
			}
			out = append(out, literalElem{
				sub: sub, subscripted: true, appendValue: appends, nested: &value,
			})
			continue
		}
		w := el.Word
		// Asked before the element is read rather than after it, so that an
		// element the shape has made a word is expanded once and not twice.
		// A discarded reading is not free: `i=0; a=(p [$((i++))]=v)` is two
		// words in the dialect that reads it that way and leaves `i` at 1,
		// measured — splitting the halves, expanding them and then throwing
		// them away for the word would leave it at 2.
		if readsSubscripts {
			if sub, value, appends, ok := r.assocElem(w); ok {
				out = append(out, literalElem{
					sub: sub, value: value, subscripted: true, appendValue: appends,
				})
				continue
			}
		}
		if bareIsOneValue {
			// Expanded as an assignment's value: no splitting, no pathname
			// expansion, and a null result kept as a field rather than
			// removed. One word in, one field out, whatever the expansion
			// came to.
			out = append(out, literalElem{fields: []string{r.expandAssignValue(w)}})
			continue
		}
		out = append(out, literalElem{fields: r.expandWord(w)})
	}
	return out, !r.failedHeading()
}

// bareLiteralElementIsOneValue reports whether this literal's bare elements
// are each one field, which is a question only a **keyed** literal asks.
//
// The name is what decides whether the literal is keyed, and it is asked of
// the same two things every other seam here asks: the table already declared,
// and the table letter written on the command this literal is an operand of.
// The second is what makes `typeset -A m=($k $v)` answer the same as
// `typeset -A m; m=($k $v)`, which it must — the letter and the literal being
// on one command is the ordinary spelling and the one a script writes.
//
// tableLetterAhead is that second reading at the one seam that cannot look it
// up: an operand's elements are expanded **before** the utility runs, so
// Runner.tableLetterHere has not been written yet and the letter has to come
// from the command's own words. See Runner.expandArrayOperands.
//
// An indexed literal never reaches the axis at all, which is the control the
// axis's own comment records: `a=($k)` is four fields in every column.
func (r *Runner) bareLiteralElementIsOneValue(name string, tableLetterAhead bool) bool {
	if name == "" || (!tableLetterAhead && !r.assocDeclared(name) && !r.tableLetterHere[name]) {
		return false
	}
	return r.ask(r.sem().BareElementsInATableLiteralAreEachOneValue,
		"each bare element of a keyed literal being one field rather than a word list")
}

// takeExpandedElements is an element list somebody has already expanded for
// this very assignment, and it may be taken exactly once.
//
// The identity of the *slice* is what matches, not the name or the length: the
// list handed over came out of this assignment's own tree, so the words are
// the same words. A nested literal reached while this assignment is under way
// carries a list of its own and does not match, which is what the identity
// check is for — comparing by name would have handed the outer list to
// `a=(p q)`'s namesake inside a substitution.
//
// Marked taken on the way out rather than cleared, because the trace may still
// be waiting for it: a dialect that writes one line for a whole assignment
// list cannot write it until the last value is known, which is after every
// store has run. The mark is what keeps a *nested* literal with no elements in
// it — the one shape whose slice cannot be told from the cached one — from
// taking the outer list a second time.
func (r *Runner) takeExpandedElements(elems []*syntax.ArrayElem) ([]literalElem, bool) {
	e := r.expanded
	if e == nil || !e.elemsSet || e.elemsTaken || !sameWordList(e.assign.Elems, elems) {
		return nil, false
	}
	e.elemsTaken = true
	return e.elems, true
}

// sameWordList reports whether two element lists are the same slice.
func sameWordList(a, b []*syntax.ArrayElem) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || &a[0] == &b[0]
}

// literalReadsSubscripts reports whether this compound literal's elements name
// where their values go, or are plain words that happen to open with a
// bracket.
//
// Everywhere but one dialect they always do, and an element carrying no
// `[sub]=` head simply takes the next position going, so the two shapes mix
// freely: `a=(x [3]=y z)` is three elements in bash and in zsh.
//
// Two facts are asked here, and they are different kinds of fact even though
// one flag gates both.
//
// The first is the **grammar's**, and belongs to the flag it is read from:
// the literal is either subscripted throughout or a word list, and its first
// element settles which — see
// [syntax.Dialect.ArrayLiteralShapeFollowsTheFirstElement], where the
// measurement is and where the parser refuses the mixture outright. Nothing
// here can be reached by a literal the parser called a syntax error, so what
// is left of that half is only the word-list reading.
//
// The second is the **store's**, and it is why this is not simply the
// parser's answer carried down. An append whose name is already holding an
// indexed array cannot turn it into a keyed one, and rather than complaining
// the shell gives the subscripted reading up and keeps the elements as the
// words they were written as. Measured 2026-09-14 on ksh93u+ 2012-08-01,
// each read back with `typeset -p a`:
//
//	unset a; a+=([1]=Z [2]=Y)      typeset -A a=([1]=Z [2]=Y)
//	a=one;   a+=([1]=Z)            typeset -A a=([0]=one [1]=Z)
//	a=(p q r); a+=([1]=Z)          typeset -a a=(p q r '[1]=Z')
//	a=(p q r); a+=([5]=Z)          typeset -a a=(p q r '[5]=Z')
//	a=(p q r); a+=([1]+=Z)         typeset -a a=(p q r '[1]+=Z')
//	typeset -A m=([k]=v); m+=([j]=w)   typeset -A m=([j]=w [k]=v)
//
// — so it is the *indexed array* that refuses, and not the append: an unset
// name, a scalar and a keyed table all read the subscripts. This is the
// reachable consequence #2505 was filed for. We marked the name associative
// and stored the keyed reading over the top, so `a=(p q r); a+=([1]=Z)`
// answered `typeset -A a=([1]=Z)` — three elements gone, silently, at status
// 0, with a plausible array standing where the script's own was.
//
// The keyed name is not reached from here at all: a declared associative
// array takes the assoc path above, which is the last row and which already
// agreed.
func (r *Runner) literalReadsSubscripts(name string, elems []*syntax.ArrayElem, appendTo bool) bool {
	if !r.dialect().ArrayLiteralShapeFollowsTheFirstElement {
		return true
	}
	if !r.literalShapeReadsSubscripts(elems) {
		return false
	}
	if !appendTo {
		return true
	}
	// arrayToAppendTo rather than the store, for the reason the append
	// itself reads it that way: a *produced* array is holding elements too,
	// and they are as much in the way of a keyed reading as stored ones.
	_, indexed := r.arrayToAppendTo(name)
	return !indexed
}

// literalShapeReadsSubscripts is the grammar half of the question above, and
// the whole of it for every literal that is not an append.
//
// Its own function because three other seams read a literal — a declared
// keyed name, an element given a literal of its own, and the nested literal
// that splices — and the shape rule reaches all four. Measured on ksh93u+
// 2012-08-01: `a[1]=(p [2]=z)` is `typeset -a a=([1]=(p '[2]=z') )`, a word
// list inside the element, and `a[1]=([2]=z p)` is the same syntax error the
// whole-array spelling gives. What those three do *not* share is the store
// half, which is about what an append may turn an indexed array into.
func (r *Runner) literalShapeReadsSubscripts(elems []*syntax.ArrayElem) bool {
	if !r.dialect().ArrayLiteralShapeFollowsTheFirstElement {
		return true
	}
	// A nested literal is not a subscripted element, and the dialect that
	// asks this question has no nested literals to meet.
	return len(elems) > 0 && elems[0].Word != nil && syntax.SubscriptedElement(elems[0].Word)
}

// assignArrayLiteral is `a=(…)` and `a+=(…)` on a name with no associative
// attribute.
//
// A subscripted element places its value where it says rather than becoming
// one, which is the ordinary way to build a sparse array: `a=([2]=c [0]=a)`
// is two elements, at 0 and 2, with nothing between them. It used to keep the
// text — `${a[0]}` answered the six characters `[2]=c` — and nothing reported
// it, so the array looked populated and was not.
func (r *Runner) assignArrayLiteral(name string, elems []*syntax.ArrayElem, appendTo bool) {
	parsed, ok := r.literalElems(elems, r.literalReadsSubscripts(name, elems, appendTo),
		r.bareLiteralElementIsOneValue(name, false))
	if !ok {
		// The elements were not read, so there is nothing to store and the
		// name keeps whatever it was holding. Before literalSubscriptIsAKey,
		// because a half-expanded list is not evidence about the shape either.
		return
	}
	if r.literalSubscriptIsAKey(name, parsed) {
		// The subscript is text rather than an expression, and a literal
		// written with one declares a keyed array — one concept with two
		// consequences, so the elements go where a declared name's would.
		if appendTo {
			r.keyedLiteralOverAScalar(name)
		}
		r.markAssoc(name)
		r.assignAssocElems(name, parsed, appendTo)
		return
	}

	a := Array{}
	next := 0
	if appendTo {
		// arrayForWrite rather than the store, so an append to a *produced*
		// array goes after the elements the producer reports rather than
		// after nothing: `set -- a b c; argv+=(d)` leaves four parameters.
		switch old, ok := r.arrayToAppendTo(name); {
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
	built, ok := r.literalInto(name, a, next, parsed, true)
	if !ok {
		return
	}
	if !appendTo && nestingRetypesTheLiteral(parsed) {
		r.storeRetypedNestedLiteral(name, built)
		return
	}
	r.storeArray(name, built)
}

// nestingRetypesTheLiteral reports whether a literal holding an element that
// is a literal of its own leaves a **keyed table** rather than an indexed
// array.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-19, a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin on /dev/null:
//
//	a=( (1 2) (3 4) )      typeset -a a=((1 2) (3 4) )
//	a=( (1 2) x (3 4) )    typeset -a a=((1 2) x (3 4) )
//	a=( (1 2) x y )        typeset -a a=((1 2) x y)
//	a=( x (1 2) )          typeset -A a=([0]=x [1]=(1 2) )
//	a=( x y (1 2) )        typeset -A a=([0]=x [1]=y [2]=(1 2) )
//	a=( "" (1 2) )         typeset -A a=([0]='' [1]=(1 2) )
//	a=( x y )              typeset -a a=(x y)
//
// So it is the **first** element that decides, and nothing else: a literal
// whose first element is a literal of its own stays indexed however many
// plain elements follow it, and one whose first element is plain becomes a
// table as soon as any element nests. The last row is the control that keeps
// it about nesting rather than about the elements.
//
// The keys are the positions written out, so nothing a script reads moves —
// `${#a[@]}`, `${!a[@]}`, `${a[@]}` and `${a[1][0]}` answer the same either
// way. What changes is the attribute a listing shows and what a later
// non-numeric subscript may do: `a=( x (1 2) y ); a[zz]=Q` puts a key in
// there, where an indexed array would refuse it.
//
// No axis: only the dialect with nested literals can reach this at all.
func nestingRetypesTheLiteral(parsed []literalElem) bool {
	if len(parsed) == 0 || elementIsAValueOfItsOwn(parsed[0]) {
		return false
	}
	for _, e := range parsed {
		if elementIsAValueOfItsOwn(e) {
			return true
		}
	}
	return false
}

// elementIsAValueOfItsOwn reports whether an element was written as its own
// pair of parentheses, whichever of the two constructs they held.
//
// The rule above counts both, measured in the same run: `a=( x (p=1 q=2) )`
// is `typeset -A a=([0]=x [1]=(p=1;q=2))` on ksh93u+ 2012-08-01 exactly as
// `a=( x (1 2) )` is `typeset -A a=([0]=x [1]=(1 2) )`, and
// `a=( (p=1) (q=2) )` stays `typeset -a` exactly as `a=( (1 2) (3 4) )` does.
// So what retypes the literal is the parentheses and not what they were
// found to hold (#3864).
func elementIsAValueOfItsOwn(e literalElem) bool {
	return e.nested != nil || e.members != nil
}

// storeRetypedNestedLiteral stores what the rule above decided: the same
// elements, under their positions written out as keys.
func (r *Runner) storeRetypedNestedLiteral(name string, a Array) {
	r.markAssoc(name)
	if r.AssocArrays == nil {
		r.AssocArrays = map[string]AssocArray{}
	}
	table := make(AssocArray, len(a))
	for _, i := range a.subscripts() {
		table[strconv.Itoa(i)] = a[i]
	}
	r.AssocArrays[name] = table
	r.sweepElementCompounds(name)
	// Written to, so the name leaves the declared-only set and its scalar
	// view comes back — the same two notes every keyed write makes, and for
	// the same reasons. See setAssocElem.
	r.compoundWasAssigned(name)
	r.nameIsBack(name)
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
	if r.isCompoundVariable(name) {
		// ksh93's fourth kind is holding a value and it is not one an
		// element write builds on: the base is occupied — `${#c[@]}` is 1 —
		// but nothing lands there. `c=(a=1); c+=(x y)` is
		// `typeset -a c=([1]=x [2]=y)` with element 0 absent, and
		// `c=(a=1); c[1]=z` is `typeset -a c=([1]=z)` rather than the tree's
		// rendering in front of it. The one place both routes pass through,
		// so the transition is stated once — see compoundVariableSubscripted
		// for what the members do and why the call belongs here.
		r.compoundVariableSubscripted(name)
		return Array{}, 1
	}
	v, held := r.getVar(name)
	if !held {
		return Array{}, 0
	}
	return Array{0: Scalar(v)}, 1
}

// keyedLiteralOverAScalar is appendedOverAScalar's question asked on the other
// route out of assignArrayLiteral: `a+=([1]=Z)` rather than `a+=(2)`.
//
// One operator over one scalar, and the only thing that sends the two
// spellings down different paths is whether the literal's first element
// carries a subscript — so the value the name was holding has to survive both.
// It did not: the keyed path went straight to markAssoc, which builds an empty
// table, and the scalar went with it at status 0 (#2785). Measured 2026-09-14
// on ksh93u+ 2012-08-01, the only dialect whose literal subscripts are keys:
//
//	a=one; a+=([1]=Z); typeset -p a    typeset -A a=([0]=one [1]=Z)
//	a=one; a+=([k]=Z); typeset -p a    typeset -A a=([0]=one [k]=Z)
//	a=one; a+=(2);     typeset -p a    typeset -a a=(one 2)
//	unset a; a+=([1]=Z)                typeset -A a=([1]=Z)
//	a=;      a+=([1]=Z)                typeset -A a=([0]='' [1]=Z)
//
// So an empty scalar counts as a value and an unset name does not, which is
// the distinction getVar already draws and the reason this asks it rather than
// reading Runner.Vars — see appendedOverAScalar for the environment half of
// the same argument.
//
// The base is spelled `0` because a table has no positions: it is a key like
// any other, and it is the same key the *declared* route writes — `a=one;
// typeset -A a` is `typeset -A a=([0]=one)` in both promoting columns. That is
// why this reads ScalarUnderATableDeclaration rather than inventing an axis
// beside it. The append declares the table, and what a table declaration does
// to a scalar already has an answer; a second axis would be the same fact
// written down twice, free to drift. Only ksh93 reaches this route today —
// ArrayLiteralSubscriptIsAKey is no in bash and zsh — and its answer to that
// axis is the promoting one, which is what the rows above show.
//
// The compound variable is **not** this, and is deliberately left alone.
// `a=(b=1); a+=([1]=Z)` is `typeset -A a=([0]=() [1]=Z)` there — an *emptied
// compound* standing at the base, whose `${a[0]}` reads back as the two lines
// `(` and `)` — which is a value kind this engine's tables have no way to
// hold. Recorded rather than modeled, as the same shape is on the indexed
// route in appendedOverAScalar.
func (r *Runner) keyedLiteralOverAScalar(name string) {
	if r.sem().ScalarUnderATableDeclaration != ScalarUnderACompoundBecomesTheFirstElement {
		return
	}
	if r.isCompoundVariable(name) {
		return
	}
	if _, ok := r.arrayToAppendTo(name); ok {
		// Appending to an array, not over a scalar. The indexed store is
		// asked through arrayToAppendTo rather than Runner.Arrays so a
		// *produced* array counts as one here exactly as it does there.
		return
	}
	if r.assocDeclared(name) {
		// The name is already a table, so there is no scalar under it: the
		// literal's elements join the ones it holds.
		return
	}
	v, held := r.getVar(name)
	if !held {
		return
	}
	r.markAssoc(name)
	r.setAssocElem(name, "0", v)
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
func (r *Runner) literalInto(name string, a Array, next int, parsed []literalElem, folds bool) (Array, bool) {
	// The positions this literal writes, which are not always every position
	// the array ends up with: an *appending* literal is handed the elements
	// the name is already holding and adds to them. The name's folding
	// attributes reach what a write introduces and leave the rest alone —
	// measured 2026-09-20, `b=(p q r); typeset -i b; b+=(7)` is `p q r 7` in
	// bash 5.3.20 and was `0 0 0 7` here, because the fold used to be taken
	// at the store over whatever array it was handed (#3888). See the loop
	// at the end, and elementValueFolded, which is the same question the
	// one-element spelling asks.
	//
	// **folds is false where the literal is not a write to this name.** A
	// nested literal and a splice's word list are built through here too,
	// and a nested literal's words are measurably not values the name's
	// attribute reaches: `typeset -i a; a[1]=(5+5)` is
	// `typeset -a -i a=([1]=(5+5) )` on ksh93u+, unevaluated, where the same
	// attribute *arriving* over it folds to `([1]=(10) )`. The splice hands
	// its words back to a store that asks for itself.
	wrote := map[int]bool{}
	for _, e := range parsed {
		if e.members != nil && !e.subscripted {
			// A compound variable's body standing where an element goes, at
			// the next position going. The body is run only now, because the
			// members hang under the element's own subscripted spelling and
			// this is where the subscript is settled.
			value, ok := r.literalElementCompound(name, itoa(next), e.members)
			if !ok {
				return nil, false
			}
			a[next], wrote[next] = value, true
			next++
			continue
		}
		if e.nested != nil {
			// A literal of its own becomes **one** element holding what it
			// built, wherever the next position is. Not spliced: that is the
			// whole of what makes `a=( (1 2) (3 4) )` two elements rather
			// than four, and `${a[1][0]}` reach the `3`.
			a[next], wrote[next] = *e.nested, true
			next++
			continue
		}
		if !e.subscripted {
			for _, f := range e.fields {
				a[next], wrote[next] = Scalar(f), true
				next++
			}
			continue
		}
		if e.members != nil {
			// A subscripted head whose value is a compound body — `a=([1]=(p=1
			// q=2))` — placed where the subscript says. The same arithmetic
			// the nested-array spelling below reads, so the two readings of
			// one syntax cannot disagree about which element is meant.
			idx, err := r.subscriptValue(e.sub)
			if err != nil {
				r.failedSubscript("%s\n", r.subscriptFailure(e.sub, err))
				return nil, false
			}
			pos, ok := r.elemPos(a, idx)
			if !ok {
				wording := r.diag().BadArrayLiteralSubscript
				if wording == "" {
					wording = r.diag().BadArraySubscript
				}
				r.failedSubscript("%s\n", Wording(wording,
					"%[1]s[%[2]s]: bad array subscript", name, e.sub, ""))
				return nil, false
			}
			value, ok := r.literalElementCompound(name, itoa(pos), e.members)
			if !ok {
				return nil, false
			}
			a[pos], wrote[pos] = value, true
			if pos >= next {
				next = pos + 1
			}
			continue
		}
		if e.nested != nil {
			// A subscripted head whose value is a literal, placed where the
			// subscript says. Read below by the same arithmetic every other
			// subscripted element uses.
			idx, err := r.subscriptValue(e.sub)
			if err != nil {
				r.failedSubscript("%s\n", r.subscriptFailure(e.sub, err))
				return nil, false
			}
			pos, ok := r.elemPos(a, idx)
			if !ok {
				wording := r.diag().BadArrayLiteralSubscript
				if wording == "" {
					wording = r.diag().BadArraySubscript
				}
				r.failedSubscript("%s\n", Wording(wording,
					"%[1]s[%[2]s]: bad array subscript", name, e.sub, ""))
				return nil, false
			}
			a[pos], wrote[pos] = nestedAppended(a[pos], *e.nested, e.appendValue), true
			if pos >= next {
				next = pos + 1
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
			r.failedSubscript("%s\n", r.subscriptFailure(e.sub, err))
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
			r.failedSubscript("%s\n", Wording(wording,
				"%[1]s[%[2]s]: bad array subscript", name, e.sub, e.value))
			return nil, false
		}
		value := e.value
		if e.appendValue {
			// `a=(p q r); a+=( [1]+=Z )` joins the element rather than
			// replacing it, and through appendedValue rather than with `+`
			// because the name's attribute decides which join this is —
			// `typeset -ia n=(1 2 3); n+=( [1]+=5 )` is 7 and not 25, the
			// same answer `n[1]+=5` gives on its own line.
			//
			// The value joined is the one this literal has put there, which
			// for `a=(…)` is whatever the literal itself wrote — the array
			// started empty — and for `a+=(…)` is the element that was
			// already standing. Unanimous in the panel's indexed arrays, and
			// not the question KeyedLiteralAppendJoinsTheReplacedValue asks:
			// see keyedLiteralAppend, where one column reads the replaced
			// table instead.
			v, ok := r.appendedValue(name, a[pos].scalar(), value)
			if !ok {
				return nil, false
			}
			value = v
		}
		a[pos], wrote[pos] = Scalar(value), true
		// A bare element after a subscripted one continues from there rather
		// than from where the count had reached: `a=(x [3]=y z)` puts z at 4.
		// Measured in both shells that accept the mixture, and it follows the
		// *written* subscript through the base, so the same literal fills the
		// same positions whichever number the first element answers to.
		next = pos + 1
	}
	// And what the name's attributes make of each value this literal put
	// there. Here rather than at the store, which is handed the elements the
	// name was already holding as well: a fold taken there re-read them, so
	// one appended word zeroed an array of text under `typeset -i` (#3888).
	//
	// A nested array is passed over for the reason compoundElemsFolded gives:
	// a literal's words are not the name's values on a *write*, measured
	// `typeset -i a; a[1]=(5+5)` keeping `5+5` on ksh93u+ where the same
	// attribute *arriving* over it folds.
	if !folds {
		return a, true
	}
	for pos := range wrote {
		if a[pos].Nested != nil || a[pos].Kind == ElementHoldsACompound {
			continue
		}
		v, ok := r.elementValueFolded(name, a[pos].scalar())
		if !ok {
			// The evaluation failed and has said so, or the axis went
			// unanswered. Either way nothing is stored.
			return nil, false
		}
		a[pos] = Scalar(v)
	}
	return a, true
}

// literalSubscriptIsAKey asks whether a subscript inside a literal is the text
// between the brackets or an expression to evaluate.
//
// A subscript spelled as a plain decimal numeral evaluates to itself, so
// `a=([2]=c)` fills slot 2 under either reading — but only the *slot* is the
// same, and that is as far as the old shortcut here was right. It skipped the
// axis entirely for a decimal subscript, which cost the shells that answer
// yes the one thing the answer decides: what kind of array the literal
// *creates*. Measured 2026-09-12 against ksh93u+ 2012-08-01, where the
// letter and the keys move together:
//
//	a=([5]=q)        typeset -A a=([5]=q)        ${a[05]} empty, ${a[5]} is q
//	a=([05]=q)       typeset -A a=([05]=q)       ${a[05]} is q, ${a[5]} empty
//	a=([0]=x [1]=y)  typeset -A a=([0]=x [1]=y)  ${a[01]} empty
//	a[5]=q           typeset -a a=([5]=q)        ${a[05]} is q
//
// — so the literal is what builds the association, not the gap in it. #1659
// was filed reading the first row as a *listing* rule about sparseness, and
// it is neither: the third row is dense and still `-A`, the fourth is sparse
// and still `-a`, and a rule keyed on the gap prints the wrong letter for
// both. The subscript being a key is the whole of it, and the letter follows
// from having keys rather than the other way round.
//
// So an answered axis decides every subscripted literal, decimal or not, and
// only an *unanswered* one still leans on the shortcut: a core that has
// chosen no shell keeps `a=([2]=c)` usable rather than diagnosing it, and
// complains where the two readings visibly part — `[1+1]`, `[i]`, `[k]`.
func (r *Runner) literalSubscriptIsAKey(name string, parsed []literalElem) bool {
	answer := r.sem().ArrayLiteralSubscriptIsAKey
	if r.indexedLetterHere[name] {
		// `typeset -a a=([5]=q)` — the indexed letter on the same command as
		// the literal, which is the one route that puts the subscript back to
		// being an expression. Not the attribute: every other way of reaching
		// an already-indexed name still reads the subscripts as keys.
		return false
	}
	for _, e := range parsed {
		if !e.subscripted {
			continue
		}
		if answer == Unspecified && isDecimalSubscript(e.sub) {
			continue
		}
		return r.ask(answer,
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

// arrayToAppendTo is what `name+=(…)` adds to, and whether the name holds an
// array at all. A produced array counts as one — its elements are what an
// append goes after, exactly as a stored array's are.
func (r *Runner) arrayToAppendTo(name string) (Array, bool) {
	if a, stored := r.Arrays[name]; stored {
		return a, true
	}
	if _, produced := r.DynamicArrays[name]; produced {
		return r.arrayForWrite(name), true
	}
	return nil, false
}
