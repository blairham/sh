// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// A compound variable held in an array element — `a[1]=(p=1 q=2)`.
//
// The same parentheses hold two constructs here as they do over a bare name,
// and the first word decides which: `a[1]=(x y)` is the nested array
// [Semantics.SubscriptedArrayLiteral] already answers for, and
// `a[1]=(p=1 q=2)` is ksh93's fourth kind standing where an element goes.
//
// **Nothing new is stored.** The members are ordinary names spelled with the
// element's own subscripted name in front — `a[1].p`, `a[1].q` — which is not
// an inference but what the reference itself says: `${!a[1].@}` answers
// `a[1].p a[1].q` there, and `typeset -p` with no operands writes `a[1].p=1`
// and `a[1].q=2` beside `typeset -a a=([1]=(p=1;q=2))`. So this file is the
// join between two mechanisms that were both already here, and the whole of
// what it adds is the *name*: interp/compoundvariable.go keeps members under
// whatever name it is handed, and it is handed `a[1]` where it is otherwise
// handed `c`.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-19, `env -i PATH=/usr/bin:/bin
// LC_ALL=C` over a script file with stdin on /dev/null, read through
// `sed -n l`:
//
//	written                               typeset -p a
//	a[1]=(p=1 q=2)                        typeset -a a=([1]=(p=1;q=2))
//	a[1]=(p=1 q=(r=2))                    typeset -a a=([1]=(p=1;q=(r=2)))
//	a[1]=(typeset -i n=5)                 typeset -a a=([1]=(typeset -i n=5))
//	a[1]=(p=1); a[1]+=(q=2)               typeset -a a=([1]=(p=1;q=2))
//	a[1]=(p=1); a[1]=(x y)                typeset -a a=([1]=(x y) )
//	typeset -A m; m[k]=(p=1 q=2)          typeset -A m=([k]=(p=1;q=2))
//
// and the reads beside them: `${a[1]}` is the whole compound over four lines,
// `${a[1].p}` is `1`, `${a[1].q.r}` is `2`, `${!a[1].@}` is `a[1].p a[1].q`,
// and `a[1].p=9` writes the member.
//
// Two controls that must not move, and they are what says the gap was the
// compound reading rather than subscripts generally: `b[1]=(x y)` lists as
// `typeset -a b=([1]=(x y) )` with `${b[1]}` of `x`, and `b[1][2]=q` lists as
// `typeset -a b=([1]=([2]=q) )`. Both are byte-exact against the reference
// before this and after it.

// elementNamespace is the name a compound held in an element hangs its
// members under: the element's own subscripted spelling.
//
// `a` with subscript `1` is `a[1]`, and the key of a table is written the
// same way — `m[k]`. Taken from the *evaluated* subscript and not from what
// was written, because two spellings of one element are one element:
// `i=1; a[$i].p=9` writes the member `a[1].p` that `a[1]=(p=1)` created,
// measured.
func elementNamespace(name, subscript string) string {
	return name + "[" + subscript + "]"
}

// assignSubscriptedCompound is `a[i]=( … )` where the parentheses hold a
// compound variable's body.
//
// The subscript is resolved exactly as the nested-array spelling resolves it —
// the same two routes, a table's key or an indexed array's arithmetic — so
// that the two readings of one syntax cannot come to disagree about which
// element is being written. See [Runner.nestElemLiteral], whose shape this
// follows line for line up to the point the value is built.
func (r *Runner) assignSubscriptedCompound(ctx context.Context, a *syntax.Assign) {
	at, ok := r.elementAddress(a.Name, a.Index, a.IndexText)
	if !ok {
		return
	}
	space := elementNamespace(a.Name, at.sub)
	if !r.fillElementCompound(ctx, space, a) {
		return
	}
	// Never as an append: `a[1]=(p=1); a[1]+=(q=2)` adds a *member* and
	// leaves one element, where the nested-array spelling's `+=` adds to the
	// list inside the element. The operator was consumed by the body above —
	// which is what keeps the members of the first literal — so the store
	// below replaces one element with one element either way.
	r.storeElementCompound(a.Name, at, space)
}

// elementAddress is the one element a subscript names, in the pieces every
// writer here needs: the subscript as the namespace spells it, and — for an
// indexed array — the text and the number the store rounds against.
//
// One function rather than one per writer, which is the point: the literal
// `a[1]=(p=1)` and the member `a[1].p=1` have to agree about *which* element
// they mean or the members hang under a name nothing holds, and two copies of
// the two routes is exactly how they would come to disagree.
type elementAddress struct {
	// sub is the subscript as [elementNamespace] spells it: a table's key as
	// written, an indexed array's as the number the arithmetic gave.
	sub string
	// subject is the indexed subscript as it was *written*, which the store
	// needs for the rounding an unexpanded operand gets, and idx is the
	// arithmetic's answer. Both are meaningless for a table.
	subject string
	idx     int
	assoc   bool
}

// elementAddress resolves a subscript for a write, by the name's own two
// routes and with the refusals each of them makes.
func (r *Runner) elementAddress(name string, index *syntax.Word, written string) (elementAddress, bool) {
	if r.assocDeclared(name) {
		key, keyed := r.assocAssignKey(name, index)
		if !keyed {
			return elementAddress{}, false
		}
		return elementAddress{sub: key, assoc: true}, true
	}
	text := r.joinWord(index)
	return r.indexedElementAddress(subscriptSubject(written, text), text)
}

// indexedElementAddress is the indexed half of the above, taken on its own so
// that a caller holding the subscript as **text** can reach it.
func (r *Runner) indexedElementAddress(subject, text string) (elementAddress, bool) {
	idx, err := r.subscriptValueAsWritten(subject, text)
	if err != nil {
		r.fatal("%s\n", r.subscriptFailure(text, err))
		return elementAddress{}, false
	}
	return elementAddress{sub: itoa(idx), subject: subject, idx: idx}, true
}

// resolvedElementAddress is [Runner.elementAddress] for an element named by a
// **resolved text** rather than by a parsed subscript — `a[1]` as a name
// reference holds it, which is the only way a subscript reaches this shell
// without a [syntax.Word] behind it.
//
// The two routes are the same two, asked of the base in the same order. A
// table's key is the text as it stands, which is how every other write
// through a reference to an element already keys one — see
// [Runner.storeThroughNamerefElement], whose pair of stores this address is
// resolved to reach.
func (r *Runner) resolvedElementAddress(base, sub string) (elementAddress, bool) {
	if r.assocDeclared(base) {
		return elementAddress{sub: sub, assoc: true}, true
	}
	return r.indexedElementAddress(sub, sub)
}

// assignCompoundBodyIntoElement is a compound variable's body written through
// a name reference **aimed at an element**: the members go into the element's
// namespace, as they already did, and the element itself is made to hold the
// compound.
//
// Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01 (`/bin/ksh`), script
// files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the
// null device, over `a=(p q r); typeset -n e='a[1]'`:
//
//	written        typeset -p a                  "${a[1]}"        ${a[1].x}
//	typeset e=(x=1)   typeset -a a=(p (x=1) r)   the body, over   1
//	                                             three lines
//
// The third column was already right — the members reach `a[1].x` because
// [Runner.compoundMemberThroughAReference] resolves `e.x` to it — and the
// first two were the element keeping its old `q`. So the whole of what was
// missing is the element's own value, which is [Element] of kind
// [ElementHoldsACompound] pointing at the namespace the members are already
// under.
//
// The control that must not move: `typeset e=Z` through the same reference is
// `typeset -a a=(p Z r)` in both, so it is the compound body alone and not
// the element redirect. That one goes through [Runner.setVarAs], which
// resolves the reference and reaches [Runner.storeThroughNamerefElement] —
// the scalar store this is the compound's counterpart of.
//
// **The address is resolved again rather than taken from the target's text.**
// `space` is built from what the arithmetic makes of the subscript, which is
// the same rule `a[1+1]=(x=1)` follows, so the members and the element cannot
// hang under two spellings of one cell — the failure interp/subcompound.go's
// [elementAddress] exists to prevent, arrived at from a third caller.
func (r *Runner) assignCompoundBodyIntoElement(ctx context.Context, a *syntax.Assign, base, sub string) {
	at, ok := r.resolvedElementAddress(base, sub)
	if !ok {
		return
	}
	space := elementNamespace(base, at.sub)
	if !r.fillElementCompound(ctx, space, a) {
		return
	}
	r.storeElementCompound(base, at, space)
}

// storeElementCompound puts the element that holds the compound at that
// address, by whichever of the two stores the name has.
func (r *Runner) storeElementCompound(name string, at elementAddress, space string) {
	value := Element{Kind: ElementHoldsACompound, Str: space}
	if at.assoc {
		r.storeAssocElement(name, at.sub, value)
		return
	}
	r.storeArrayElement(name, at.idx, at.subject, value, false)
}

// startElementCompound brings an element into being as an empty compound
// where a member names one that is not there — `a[1].p=5` with no `a`.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-20, and the rule is which side of
// *set* the element falls on rather than which table the name has:
//
//	written                                   typeset -p
//	a[1].p=5                                  typeset -a a=([1]=(p=5))
//	a[1].q.r=5                                typeset -a a=([1]=(q=(r=5)))
//	a=(x y); a[5].p=9                         typeset -a a=([0]=x [1]=y [5]=(p=9))
//	a=1; a[1].p=9                             typeset -a a=(1 (p=9))
//	typeset -A m=([k]=v); m[z].p=9            typeset -A m=([k]=v [z]=(p=9))
//
// An element that **is** there is the other half and is not this: the
// reference folds the old value into a member name there —
// `typeset -A m=([k]=v); m[k].p=9` is `typeset -A m=([k]=(p=9.=v))` — which
// is its own bookkeeping rather than a rule a shell could state, so an
// occupied element is left alone and the member write falls through to the
// ordinary store. Recorded on #2853.
//
// Only the *first* link needs this. `a[1].q.r=5` reaches the ordinary
// compound machinery once `a[1]` stands, which creates `a[1].q` exactly as
// `c.q.r=5` creates `c.q` under a bare compound.
func (r *Runner) startElementCompound(name string, at elementAddress) {
	if r.elementIsSet(name, at.sub, true) || r.unspecified {
		return
	}
	space := elementNamespace(name, at.sub)
	r.compoundVariables()[space] = true
	delete(r.removed, space)
	r.storeElementCompound(name, at, space)
}

// assignTableCompoundBase is `typeset -A c=(p=1 q=2)` — a compound body
// standing where a **table's** literal goes, with no subscript written.
//
// The `A` letter does not take the compound reading off the literal, which is
// what separates it from `a`, and the value it makes is an element rather than
// the name's own compound: measured on ksh93u+ 2012-08-01, 2026-09-19,
// `typeset -A c=(a=1 b=2)` lists as `typeset -A c=([0]=(a=1;b=2))` and
// `${c[0].a}` answers 1, where `typeset -a c=(a=1 b=2)` is an index array of
// the two *strings* `a\=1` and `b\=2`.
//
// The key is `0` and is not the subscript arithmetic's answer: it is the key
// the reference writes, in a table whose keys are otherwise whatever a script
// wrote — `typeset -A c=(a=1); c[z]=9` lists `[0]=(a=1) [z]=9` there.
//
// The name's table-ness and not the command's letters is what decides, which
// is measured a second way: `typeset -A c; c=(a=1 b=2)` with no letter on the
// second line gives the same `typeset -A c=([0]=(a=1;b=2))`. So this is asked
// of the store.
//
// Two shapes of the same spelling are *not* this, and both are measured:
//
//	typeset -A h=([k]=v); h=()       typeset -C h=()
//	typeset -A h=([k]=v); h+=()      typeset -A h=([k]=v)
//
// An **empty** body replaces the table with a compound, where a non-empty one
// becomes an element of it; and `+=` with an empty body adds nothing to a
// container that is already there, which is the rule the table already has.
// So this takes only a plain `=` with something written inside, and the other
// two reach the paths they reached before.
//
// One row of the family is recorded rather than reproduced. `${#c[@]}` is `0`
// there while `${c[0].a}` reads back and `typeset -p` writes the element, and
// a later `c[z]=9` makes the count `1` against two keys — a table that counts
// one fewer element than it lists. Every other route to a compound element
// counts it: `typeset -A m; m[k]=(p=1)` answers `1`, measured. That is the
// reference's own bookkeeping about a half-built table rather than a rule this
// shell could state, so the count here is the ordinary one. Recorded on #2853.
func (r *Runner) assignTableCompoundBase(ctx context.Context, a *syntax.Assign) {
	const baseKey = "0"
	// The whole table goes first, exactly as a keyed literal's does:
	// `typeset -A h=([k]=v); h=(a=1)` is `typeset -A h=([0]=(a=1))` there and
	// not the old key beside the new one, measured. The namespace of any
	// compound the old elements held is taken with it by the sweep the store
	// below makes.
	if r.AssocArrays == nil {
		r.AssocArrays = map[string]AssocArray{}
	}
	r.AssocArrays[a.Name] = AssocArray{}
	space := elementNamespace(a.Name, baseKey)
	if !r.fillElementCompound(ctx, space, a) {
		return
	}
	r.storeAssocElement(a.Name, baseKey, Element{Kind: ElementHoldsACompound, Str: space})
}

// literalElementCompound is a compound body that stood where an element of a
// **literal** goes — `a=(x (p=1 q=2))`, `a=([1]=(p=1 q=2))` — run into the
// namespace that element's subscript spells, and the value the element then
// holds.
//
// The same two pieces the subscripted assignment above uses, in the same
// order, and deliberately the same two functions: a literal's element and
// `a[1]=(p=1 q=2)` are one construct reached by two routes, so a second
// filler here is how the two would come to disagree about what `+=` keeps or
// which name the members hang under.
//
// The subscript arrives as text because the placement has already settled it —
// the next position going for a bare element, the arithmetic's answer for a
// subscripted one — where [Runner.assignSubscriptedCompound] resolves it
// itself. That is the whole of the difference between the two callers.
//
// The context is the run's rather than a parameter, for the reason
// Runner.ctx carries: a member's value may hold a command substitution, and
// threading a context through the literal's placement to reach one call would
// be worse. See Runner.ShellContext.
func (r *Runner) literalElementCompound(name, sub string, members []*syntax.SimpleCmd) (Element, bool) {
	space := elementNamespace(name, sub)
	body := &syntax.Assign{Name: space, IsArray: true, Members: members}
	if !r.fillElementCompound(r.ShellContext(), space, body) {
		return Element{}, false
	}
	r.compoundVariables()[space] = true
	return Element{Kind: ElementHoldsACompound, Str: space}, true
}

// fillElementCompound runs the body into the element's namespace and reports
// whether the store that follows should happen.
//
// `+=` keeps what is there and a bare `=` does not, which is the rule the
// unsubscripted spelling already has; it arrives here as the same two calls
// because a member of `a[1]` is a member in exactly the sense a member of `c`
// is.
func (r *Runner) fillElementCompound(ctx context.Context, space string, a *syntax.Assign) bool {
	body := *a
	body.Name = space
	body.Index, body.IndexFlags, body.IndexText, body.Leading = nil, nil, "", nil
	r.assignCompoundVariable(ctx, &body)
	return !r.unspecified && r.ctl == controlNone
}

// elementCompoundGone takes the members of a compound an element held away,
// for every route that stops the element holding it.
//
// Three of them, and each is measured: the element is overwritten with
// something else (`a[1]=(p=1); a[1]=(x y)` leaves `${a[1].p}` empty), the
// array is unset whole (`a[1]=(p=1); unset a` likewise), and the element is
// unset on its own. A compound's members outliving the value they belonged to
// is the same failure `unset c` on a bare compound already guards against —
// the names are real names, and nothing else would ever take them.
func (r *Runner) elementCompoundGone(e Element) {
	if e.Kind != ElementHoldsACompound || e.Str == "" {
		return
	}
	delete(r.compoundVariable, e.Str)
	r.unsetCompoundMembers(e.Str)
}

// elemText is what a read of one element gives.
//
// Every element but one answers from its own value, which is what
// [Element.scalar] is. A compound answers from the names under it, so it needs
// the store to say anything at all — and it answers the same four-line
// rendering `"$c"` gives for a bare compound, measured: `a[1]=(p=1 q=2)` then
// `printf '[%s]' "${a[1]}"` is `[(`, a tab and `p=1`, a tab and `q=2`, `)]`
// on ksh93u+ 2012-08-01, byte for byte what `c=(p=1 q=2); "$c"` gives there.
func (r *Runner) elemText(e Element) string {
	if e.Kind == ElementHoldsACompound {
		return r.compoundVariableText(e.Str)
	}
	return e.scalar()
}

// memberName is the flat name a subscripted member expansion or assignment
// reaches — `a[1].p` for `${a[1].p}` and for `a[1].p=9` alike.
//
// The member path arrives with its leading dot, so the three pieces join with
// nothing between them. See [syntax.ParamExpr.Member].
func memberName(name, subscript, member string) string {
	return elementNamespace(name, subscript) + member
}

// assignElementMember is `a[1].p=9`: a write to one member of the compound an
// element holds.
//
// The subscript is evaluated and the three pieces joined into the one name the
// member actually is, and then it is an ordinary assignment — which is the
// whole of why a member carries its own attributes, can be appended to, and is
// read back by every route that reads a name. `a[$i].p=7` writes the same cell
// `a[1].p=7` does when `i` is 1, measured.
//
// A member written where no compound stands is left to the ordinary store
// rather than refused. ksh93's own answer there is its bookkeeping rather than
// a rule — `a=(x y); a[1].p=9` lists as `typeset -a a=(x (p=9.=y))`, a value
// with the old element's text folded into a member name — and a shell cannot
// state that, so what this does instead is create the name and leave the
// element alone. Recorded on #2853 rather than modeled.
func (r *Runner) assignElementMember(ctx context.Context, a *syntax.Assign) {
	at, ok := r.elementAddress(a.Name, a.Index, a.IndexText)
	if !ok {
		return
	}
	// An element that is not there becomes an empty compound first, so that
	// the member has a value to be a member *of* — see startElementCompound
	// for the measured rows and for the occupied element it leaves alone.
	r.startElementCompound(a.Name, at)
	member := *a
	member.Name = memberName(a.Name, at.sub, a.Member)
	member.Member = ""
	member.Index, member.IndexFlags, member.IndexText, member.Leading = nil, nil, "", nil
	r.assign(ctx, &member)
}

// elementMemberAsAName rewrites `${a[1].p}` to the expansion of the plain name
// `a[1].p`, which is the name that member actually is.
//
// The rewrite rather than a path of its own, for the reason every other
// rewrite in the expander is one: a member takes the whole of the grammar with
// it — `${a[1].p:-D}`, `${#a[1].p}`, `${a[1].p+SET}` and `${a[1].q.r}` all
// read on ksh93u+ 2012-08-01 — and a path of its own would have to grow each
// of those back one at a time.
//
// `${!a[1].@}` arrives here as a member path of a lone dot, which joins to the
// prefix `a[1].` and is then the prefix listing that already answers `${!c.@}`
// — measured, the reference answers `a[1].p a[1].q` there, the same shape it
// answers `c.p c.q` with.
func (r *Runner) elementMemberAsAName(e *syntax.ParamExpr) (*syntax.ParamExpr, bool) {
	if e.Member == "" || e.Index == nil || e.Inner != nil {
		return nil, false
	}
	sub, ok := r.elementSubscriptRead(e.Name, e.Index)
	if !ok {
		return nil, false
	}
	named := *e
	named.Name = memberName(e.Name, sub, e.Member)
	named.Member = ""
	named.Index, named.IndexFlags, named.IndexText = nil, nil, ""
	named.IndexRange, named.IndexDots, named.Leading = nil, nil, nil
	named.BareIndexText = nil
	return &named, true
}

// elementSubscriptRead is elementSubscriptText for a *read*, which differs in
// one way and only one: nothing is refused.
//
// An assignment's empty key is a diagnostic in the column that has this, and a
// read's is not — `${m[].p}` asks about an element and gets nothing back, the
// same answer `${m[]}` already gives. So the two share the arithmetic and part
// over the complaint, rather than the read borrowing a writer's refusal.
func (r *Runner) elementSubscriptRead(name string, index *syntax.Word) (string, bool) {
	if r.assocDeclared(name) {
		return r.assocKey(index), true
	}
	idx, err := r.subscriptValue(r.joinWord(index))
	if err != nil {
		return "", false
	}
	return itoa(idx), true
}

// sweepElementCompounds drops the namespace of every compound the name's
// elements no longer hold.
//
// Asked of the name after a write rather than of the element at each writer,
// and that is the point: a string over the element, a nested array over it, a
// literal replacing the whole array, an `unset` of the name or of one element
// — five spellings, one question, and a sixth spelling added later gets the
// answer without knowing this exists. The alternative, comparing the array
// before against the array after, cannot be written here at all: an element
// write mutates the map it was handed, so the two are the same map by the
// time a store sees them.
//
// Measured on ksh93u+ 2012-08-01, and each row is a route:
//
//	a[1]=(p=1); a[1]=(x y);   ${a[1].p}   empty
//	a[1]=(p=1); a[1]=z;       ${a[1].p}   empty
//	a[1]=(p=1); a=(x y);      ${a[1].p}   empty
//	a[1]=(p=1); unset a;      ${a[1].p}   empty
//	a[1]=(p=1); unset "a[1]"; ${a[1].p}   empty
func (r *Runner) sweepElementCompounds(name string) {
	if len(r.compoundVariable) == 0 {
		return
	}
	prefix := name + "["
	var gone []string
	for space := range r.compoundVariable {
		if !strings.HasPrefix(space, prefix) || !strings.HasSuffix(space, "]") {
			continue
		}
		if r.anElementHoldsTheCompound(name, space) {
			continue
		}
		gone = append(gone, space)
	}
	for _, space := range gone {
		r.elementCompoundGone(Element{Kind: ElementHoldsACompound, Str: space})
	}
}

// anElementHoldsTheCompound reports whether either of the name's tables still
// has an element claiming that namespace.
func (r *Runner) anElementHoldsTheCompound(name, space string) bool {
	for _, e := range r.Arrays[name] {
		if e.Kind == ElementHoldsACompound && e.Str == space {
			return true
		}
	}
	for _, e := range r.AssocArrays[name] {
		if e.Kind == ElementHoldsACompound && e.Str == space {
			return true
		}
	}
	return false
}

// elementMemberSplit takes a name spelled `base[subscript].member` apart, and
// reports whether it was spelled that way at all.
//
// The `].` is what the subscript ends at, which is the whole of the rule: a
// key may hold a `]` of its own and a member may not hold a `[`, so looking
// for the pair rather than for either character alone is what keeps
// `m[a]b].p` one name with the key `a]b`.
//
// The member comes back with its leading dot, as [syntax.ParamExpr.Member]
// carries it, so the three pieces join back with nothing between them.
func elementMemberSplit(name string) (base, sub, member string, ok bool) {
	open := strings.IndexByte(name, '[')
	if open <= 0 {
		return "", "", "", false
	}
	end := strings.LastIndex(name, "].")
	if end < open {
		return "", "", "", false
	}
	return name[:open], name[open+1 : end], name[end+1:], true
}

// isElementNamespace reports whether the name is the namespace of a compound
// an array element holds — `a[1]`, `m[k]` — rather than an ordinary name.
//
// The brackets are the whole of the test, because that is the whole of what
// makes the spelling: [elementNamespace] writes the name and nothing else in
// this shell puts a `[` in one. A member hanging under it does not answer yes
// — `a[1].p` ends at the member and not at the bracket — which is what keeps
// the two rules below apart.
func isElementNamespace(name string) bool {
	open := strings.IndexByte(name, '[')
	return open > 0 && strings.HasSuffix(name, "]")
}

// listsBesideItsElement reports whether the name is a member the whole-shell
// listing writes as a row of its own.
//
// A compound's members are written *inside* it and nowhere else, which is the
// rule `c=(p=1 q=2); typeset -p` follows — one line, `typeset -C c=(p=1;q=2)`.
// A compound an **element** holds inverts it: the element's own value is
// already written inside the array's row, so the namespace is not a row and
// its direct members are. Measured on ksh93u+ 2012-08-01, 2026-09-19:
//
//	a[1]=(p=1 q=2)            typeset -a a=([1]=(p=1;q=2))
//	                          a[1].p=1
//	                          a[1].q=2
//	a[1]=(p=1 q=(r=2))        typeset -a a=([1]=(p=1;q=(r=2)))
//	                          a[1].p=1
//	                          typeset -C a[1].q=(r=2)
//	typeset -A m; m[k]=(p=1)  typeset -A m=([k]=(p=1))
//	                          m[k].p=1
//
// Direct members only, and the second row is what says so: `a[1].q.r` is
// written inside `a[1].q` exactly as `c.b.y` is written inside `c.b`, so the
// ordinary rule takes over one link down and only the first link is the
// exception.
func (r *Runner) listsBesideItsElement(name string) bool {
	base, sub, member, ok := elementMemberSplit(name)
	if !ok || sub == "" || base == "" {
		return false
	}
	space := elementNamespace(base, sub)
	if !r.compoundVariable[space] || r.removed[space] {
		return false
	}
	return strings.Count(member, memberSep) == 1
}
