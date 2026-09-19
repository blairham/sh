// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// A compound variable is a name whose members are other names.
//
// ksh93's fourth kind, and the one thing this store had no shape for: `c=(a=1
// b=2)` is neither a scalar nor either array, and until #2620 it was kept as
// an indexed array of the two strings `a=1` and `b=2` — so `${c.a}` answered
// empty, which is what an unset name answers and is therefore indistinguishable
// from the name never having been written.
//
// The model here is the one the shell itself can be shown to hold. Measured on
// ksh93u+ 2012-08-01, 2026-09-13, `env -i PATH=/usr/bin:/bin` with a scratch
// HOME:
//
//	written                       ksh93
//	a=1; a.b=2; typeset -p a      a=1              a is a *scalar* with a child
//	a=1; a.b=2; ${!a.@}           a.b              and the child is enumerable
//	c=(a=1); typeset -p c.a       c.a=1            a member lists as a name
//	c=(typeset -i n=5); typeset -p c.n
//	                              typeset -i c.n=5 and carries its own letters
//	c=(a=1 b=2); typeset -p       one line, `typeset -C c=(a=1;b=2)`
//
// So members are *ordinary names spelled with a dot* — the flat namespace
// #2669's grammar half already reaches — and what `typeset -C` adds is a mark
// on the parent that changes how it reads, lists and enumerates. Nothing here
// keeps a member list: it is derived from the name tables, so there is exactly
// one place a member can be created and one place it can be destroyed.
//
// The member separator, here and in every name this file builds.
const memberSep = "."

// compoundVariables returns the set, allocating it on first use.
func (r *Runner) compoundVariables() map[string]bool {
	if r.compoundVariable == nil {
		r.compoundVariable = map[string]bool{}
	}
	return r.compoundVariable
}

// isCompoundVariable reports whether the name *reads* as one.
//
// Two conditions and the second is what makes the set one table rather than
// two: the name is marked, and it is holding no value of its own. A name that
// has been written through a subscript is holding an array, so it stops
// reading as a compound the moment the array is stored — while the mark stays,
// because the members under it are still there and `unset` still has to take
// them. See compoundVariableSubscripted.
func (r *Runner) isCompoundVariable(name string) bool {
	if !r.compoundVariable[name] || r.removed[name] {
		return false
	}
	if _, ok := r.Vars[name]; ok {
		return false
	}
	if _, ok := r.Arrays[name]; ok {
		return false
	}
	if _, ok := r.AssocArrays[name]; ok {
		return false
	}
	return true
}

// markCompoundVariable makes the name a compound variable, taking away
// whatever other kind of value it was holding.
//
// The kinds are exclusive — `c=(x y); c=(a=1)` lists as `typeset -C c=(a=1)`
// and not as an array with a compound inside it — so the old value goes rather
// than being left where a reader might still find it.
func (r *Runner) markCompoundVariable(name string) {
	// The mark is what makes a namespace, so it is also what turns the
	// namespace half of a shadow on — a compound with no members yet still
	// has a kind for a declaration to displace. See compoundlocal.go.
	r.memberNamesInUse = true
	// Before the mark goes on, so that the scope records the caller's answer
	// rather than the one this line is about to write. A name a declaration
	// in this scope shadowed owns its namespace from here, which is what a
	// member the body creates needs. See compoundlocal.go.
	r.claimNamespaceHere(name)
	r.localizeMemberWrite(name)
	r.compoundVariables()[name] = true
	delete(r.Vars, name)
	delete(r.Arrays, name)
	delete(r.AssocArrays, name)
	delete(r.declaredOnlyCompound, name)
	delete(r.compoundHeldAnElement, name)
	delete(r.removed, name)
}

// compoundVariableRetyped takes the compound mark off a name that is being
// given a value of another kind, and takes its members with it.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-13, and each row is a name that was
// a compound and is not one afterwards:
//
//	c=(a=1); c=(x y)       typeset -a c=(x y)   ${c.a} empty
//	c=(a=1); c=hello       c=hello              ${c.a} empty, ${!c.@} empty
//	c=(a=1); typeset -a c  typeset -a c         ${c.a} empty
//	c=(a=1); typeset -A c  typeset -A c=()
//
// So the members go with the mark rather than outliving it, which is what
// makes the fourth kind a kind: a name holds one value, and the tree under it
// *is* that value.
//
// Called from the five stores rather than from one place, because the store is
// the authority and there are five of them — setVarAs for a scalar, storeArray
// and markArray for the indexed kind, setAssocElem and markAssoc for the keyed
// one. That is the shape compounddeclaredonly.go already uses for the same
// reason, and it is why `unset`, `read`, `+=` and a subscripted write need no
// line of their own: every one of them arrives through one of the five.
//
// A **subscripted** write is not one of these and is the other half of the
// rule — see compoundVariableSubscripted, which is what asking
// isCompoundVariable rather than the raw mark is for: that path gives the name
// a value before the store is reached, so by the time this runs the name is no
// longer reading as a compound and the members stay.
func (r *Runner) compoundVariableRetyped(name string) {
	// Every one of the five stores arrives here with the name it is about to
	// write, which makes this the one place a member name can be recognized
	// without a sixth list to keep in step. See Runner.noteMemberName.
	r.noteMemberName(name)
	if !r.isCompoundVariable(name) {
		return
	}
	delete(r.compoundVariable, name)
	r.unsetCompoundMembers(name)
}

// compoundVariableSubscripted is what a write *through a subscript* does to a
// compound, and it is a different answer from the whole-name write above.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-13. The left column is what the
// second line of each row does to `c=(a=1)`:
//
//	written      typeset -p c                ${c.a}   ${!c.@}
//	c[0]=z       typeset -a c=(z)            1        c.a
//	c[1]=z       typeset -a c=([1]=z)        1        c.a
//	c+=(x y)     typeset -a c=([1]=x [2]=y)  1        c.a
//	c=(x y)      typeset -a c=(x y)          empty    empty
//	c=hello      c=hello                     empty    empty
//	c+=z         c=z                         empty    empty
//
// So the members survive a write that reaches *into* the name and go with one
// that replaces it. Two further facts the rows carry, and neither follows from
// the other:
//
//   - **The compound's text is never what the write builds on.** Element 0 is
//     absent after `c[1]=z`, and `c+=z` is `c=z` rather than the tree's
//     rendering with a `z` after it. A compound answers a value to `$c` and it
//     is not a value another kind's write joins.
//   - **The compound still occupies the base**, which is what `${#c[@]}`
//     answering 1 already says: `c+=(x y)` starts at subscript 1 and leaves
//     nothing at 0.
//
// Giving the name an empty array is the whole of the implementation, and that
// is the point rather than a trick: the name now holds a value, so
// isCompoundVariable answers no, the store that follows leaves the members
// alone, and `unset c` still finds them through the mark. A flag threaded
// through storeArray — which is what #2706 declined to buy for one row — would
// have been the same rule with a second home.
func (r *Runner) compoundVariableSubscripted(name string) {
	if !r.isCompoundVariable(name) {
		return
	}
	if r.Arrays == nil {
		r.Arrays = map[string]Array{}
	}
	r.Arrays[name] = Array{}
}

// compoundMemberPrefix is what a member's name begins with.
func compoundMemberPrefix(name string) string { return name + memberSep }

// compoundDescendants lists every name stored under the compound, at any
// depth, sorted. The compound's own name is not among them.
//
// Derived from the tables rather than kept, which is the whole of the design:
// a member written by an assignment, by `typeset`, by `read` or by a loop
// variable is found here without any of them knowing this file exists.
func (r *Runner) compoundDescendants(name string) []string {
	prefix := compoundMemberPrefix(name)
	seen := map[string]bool{}
	add := func(k string) {
		if strings.HasPrefix(k, prefix) && len(k) > len(prefix) && !r.removed[k] {
			seen[k] = true
		}
	}
	for k := range r.Vars {
		add(k)
	}
	for k := range r.Arrays {
		add(k)
	}
	for k := range r.AssocArrays {
		add(k)
	}
	for k := range r.compoundVariable {
		add(k)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// compoundMembers lists the compound's *direct* members, sorted by name.
//
// A grandchild contributes its parent rather than itself: `c.b.y` with no
// `c.b` of its own still makes `b` a member of `c`, which is the reading
// `${!c.@}` takes — it answers `c.a c.b` for `c=(a=1 b=(y=2))` and not
// `c.b.y`.
func (r *Runner) compoundMembers(name string) []string {
	prefix := compoundMemberPrefix(name)
	seen := map[string]bool{}
	var out []string
	for _, full := range r.compoundDescendants(name) {
		rest := full[len(prefix):]
		if i := strings.Index(rest, memberSep); i >= 0 {
			rest = rest[:i]
		}
		if seen[rest] {
			continue
		}
		seen[rest] = true
		out = append(out, prefix+rest)
	}
	sort.Strings(out)
	return out
}

// unsetCompoundMembers takes every name stored under the compound away.
//
// `unset c` on a compound is not one deletion: the members are names of their
// own and would otherwise outlive the parent, so `unset c; ${c.a}` would read
// back the value the shell had just been told to forget.
func (r *Runner) unsetCompoundMembers(name string) {
	for _, m := range r.compoundDescendants(name) {
		delete(r.compoundVariable, m)
		r.unsetOneName(m)
	}
}

// assignCompoundVariable is `c=( … )` read as a compound variable's body, and
// `c+=( … )` adding to one.
//
// The body's items are run in order and each one writes into the compound's
// namespace: an item's `a=1` is an assignment to `c.a`, and a declaration
// command's operands are the same names with the same prefix. So a member is
// made by the paths that make any other name, and a nested literal —
// `c=(a=1 b=(y=2))` — reaches this function again under the name `c.b`.
func (r *Runner) assignCompoundVariable(ctx context.Context, a *syntax.Assign) {
	name := a.Name
	if a.Append && len(a.Members) == 0 && r.nameHoldsAContainer(name) {
		// `name+=()` over a name that is already an array or a table adds
		// nothing and changes nothing — measured, `a=(x y); a+=()` is
		// `typeset -a a=(x y)` there and `typeset -A h=([k]=v); h+=()` keeps
		// its element. Only an *empty* body: what a body with members in it
		// does to an array is a different row and a different answer
		// (`a=(x y); a+=(p=1)` appends the string `p=1` there), which is
		// #2620's and is not modeled.
		//
		// Everything else takes the declaration: `a=1; a+=()` and
		// `unset a; a+=()` are both `typeset -C a=()`, so a scalar is
		// discarded by it where a container is not.
		return
	}
	if !a.Append {
		// A literal replaces the whole compound rather than merging into it:
		// `c=(a=1 b=2); c=(d=3)` is `typeset -C c=(d=3)` there, with no `a`
		// left. `+=` is the spelling that keeps what is present.
		r.unsetCompoundMembers(name)
	}
	r.markCompoundVariable(name)
	prefix := compoundMemberPrefix(name)
	for _, item := range a.Members {
		if r.unspecified || r.ctl != controlNone {
			return
		}
		r.compoundBodyItem(ctx, prefix, item)
	}
}

// compoundBodyItem runs one declaration of a compound variable's body.
//
// Two shapes and they are the two the parser kept apart: bare assignments,
// which set members directly, and a declaration command, whose name and
// options are applied to the member names before its assignments land. The
// second is why the body is closer to a small program than to a word list —
// a member carries its own attributes, and `typeset -i n=5` is how it gets
// them.
func (r *Runner) compoundBodyItem(ctx context.Context, prefix string, item *syntax.SimpleCmd) {
	if len(item.Args) == 0 {
		for _, m := range item.Assigns {
			r.assignCompoundMember(ctx, prefix, m)
		}
		return
	}
	argv := r.compoundDeclarationArgv(prefix, item)
	if argv == nil || r.unspecified {
		return
	}
	// The builtin runs against the prefixed names, so `typeset -i n=5` inside
	// `c=( … )` declares `c.n`. It reads nothing from the context — see
	// biDeclare, whose context parameter is discarded — so the body needs no
	// context of its own to reach this far.
	outer := r.inBuiltin
	r.inBuiltin = "typeset"
	r.callBuiltin(ctx, "typeset", biDeclare, argv[1:])
	r.inBuiltin = outer
	for _, m := range item.Assigns {
		r.assignCompoundMember(ctx, prefix, m)
	}
}

// compoundDeclarationArgv builds the command line a body item's declaration
// runs as, with every operand name moved into the compound's namespace.
//
// The word that opened the item is mapped to `typeset` and its letters,
// because three of this shell's declaration commands are that one under
// another name — `integer` is `typeset -l -i`, `float` is `typeset -l -E`,
// `compound` is `typeset -C` and `nameref` is `typeset -n`. Mapping here
// rather than running the command word keeps the body a declaration: nothing
// in it is looked up as a command, so a function named `typeset` cannot be
// what a literal calls.
func (r *Runner) compoundDeclarationArgv(prefix string, item *syntax.SimpleCmd) []string {
	argv := []string{"typeset"}
	word := item.Args[0].Literal()
	switch word {
	case "integer":
		argv = append(argv, "-l", "-i")
	case "float":
		argv = append(argv, "-l", "-E")
	case "compound":
		argv = append(argv, "-C")
	case "nameref":
		argv = append(argv, "-n")
	case "export":
		argv = append(argv, "-x")
	case "readonly":
		argv = append(argv, "-r")
	}
	for _, w := range item.Args[1:] {
		for _, f := range r.expandWordAsAssignment(w) {
			if strings.HasPrefix(f, "-") || strings.HasPrefix(f, "+") {
				argv = append(argv, f)
				continue
			}
			argv = append(argv, prefix+f)
		}
	}
	// An array-valued operand reaches the utility as the bare name and its
	// value lands afterwards, exactly as `typeset -a q=(1 2)` does at command
	// position. See Runner.assignOperands for the rule this mirrors.
	for _, m := range item.Assigns {
		if m.Operand {
			argv = append(argv, prefix+m.Name)
		}
	}
	if r.failedHeading() {
		return nil
	}
	return argv
}

// expandWordAsAssignment expands one word of a body item the way an
// assignment's value is expanded: no splitting, so `typeset -i n=$x` is one
// operand however many blanks the value holds.
func (r *Runner) expandWordAsAssignment(w *syntax.Word) []string {
	return []string{r.expandAssignValue(w)}
}

// assignCompoundMember performs one of a body item's assignments against the
// compound's namespace.
//
// A copy of the node rather than the node itself: the tree is the program and
// a runner must not write to it — the same literal may be run again by a loop
// or a function, and a name rewritten in place would be prefixed twice.
// A compound literal is traced as the assignments its body performs rather
// than as the parentheses it was written with, and this is where that happens:
// `set -x; c=(a=1 b=2)` writes `+ c.a=1` and `+ c.b=2`, measured on ksh93u+
// 2012-08-01, 2026-09-14. A member's own name is the traced one, prefix and
// all, so a nested body writes `c.b.y=2`.
//
// It is a correction rather than an axis, for the reason every rule about this
// construct is: one dialect in the panel has a fourth kind, so there is no
// second answer to choose between.
//
// The consequence worth naming is the one #1959 measured from the other end:
// `set -x; b=(); echo after` writes **nothing** for the assignment there,
// where bash writes `b=()` and zsh writes `b=( )`. That is not a rule about
// an empty array literal — `b=()` is an empty *compound* in that shell, so it
// performs no assignments and there is nothing to trace. A reading that
// special-cased the empty literal would have written one line for
// `c=(a=1 b=2)` and passed the row it was measured from.
func (r *Runner) assignCompoundMember(ctx context.Context, prefix string, m *syntax.Assign) {
	cp := *m
	cp.Name = prefix + strings.TrimPrefix(m.Name, memberSep)
	cp.Operand = false
	// Through assignAll rather than assign, which is the one line that makes
	// the member traceable: the trace belongs to the *list* of assignments a
	// command carries, and a member is a list of one.
	r.assignAll(ctx, []*syntax.Assign{&cp})
}

// memberOfACompoundVariable reports whether the name is stored inside one.
//
// Every dotted name whose head is a compound, at any depth: `c.b.y` is a
// member of `c` as much as `c.a` is. A dotted name whose head is *not* a
// compound is not one — `a=1; a.b=2` leaves two ordinary scalars, which is the
// shell's own reading — so this asks the store rather than the spelling.
//
// The **mark** rather than isCompoundVariable, and the pair of rows that says
// so is measured: after `a=1; a.b=2; a=(x y)` the whole listing writes
// `a.b=2` beside the array, and after `c=(a=1); c[0]=z` it writes the array
// alone — the same shape twice, and what tells them apart is only that `c` was
// a compound once. A subscripted write takes the compound *reading* away and
// leaves the subtree; a name inside that subtree is still listed inside its
// parent and so nowhere, which is what the filter is for.
//
// One row in the same family is left disagreeing and is not this function's:
// a dotted name *named* on a listing is written nowhere at all when its head
// holds an array — `a=1; a.b=2; a=(x y); typeset -p a.b` is silent there and
// writes `a.b=2` here — which predates the compound and holds for a head that
// was never one.
func (r *Runner) memberOfACompoundVariable(name string) bool {
	for i := strings.Index(name, memberSep); i >= 0; {
		if r.compoundVariable[name[:i]] && !r.removed[name[:i]] {
			return true
		}
		next := strings.Index(name[i+1:], memberSep)
		if next < 0 {
			return false
		}
		i += 1 + next
	}
	return false
}

// compoundVariableBody writes a compound's members the way `typeset -p` does,
// between the parentheses and without them.
//
// The `;` rule is measured and is not the one a symmetry argument gives.
// ksh93u+ 2012-08-01, 2026-09-13:
//
//	written                          typeset -p c
//	c=(a=1; b=2)                     (a=1;b=2)
//	c=(a=1; b=(y=2); z=3)            (a=1;b=(y=2;)z=3)
//	c=(z=(y=2); a=3)                 (a=3;z=(y=2))
//	c=(a=(p=1); b=(q=2))             (a=(p=1;)b=(q=2))
//	c=(a=(x=1); b=2; z=(y=3))        (a=(x=1;)b=2;z=(y=3))
//	c=(a=(p q); z=3)                 (typeset -a a=(p q);z=3)
//	c=(z=(p q); a=3)                 (a=3;typeset -a z=(p q);)
//	c=(a=1; m=(typeset -A h; h[k]=v))
//	                                 (a=1;m=(typeset -A h=([k]=v);))
//
// Three facts hold all of that, and each of them is a row a simpler rule gets
// wrong. A `;` follows every member **except the last one written**, and
// `last` is what carries that down: a nested compound standing last takes the
// exemption with it — `z=(y=2)` with no `;` inside — where the same compound
// standing anywhere else does not. A member whose value is *parenthesized and
// not a compound* — an array or a table — always writes its `;`, even last:
// that is the only place a body ends in one. And a member with attributes but
// a scalar value follows the ordinary rule, which is what
// `(a=1;typeset -i n=5)` says.
//
// Members are written in sorted order whatever order they were assigned in,
// which is the shell's own: `c=(b=2 a=1)` lists as `(a=1;b=2)`.
func (r *Runner) compoundVariableBody(name string, last bool) string {
	members := r.compoundMembers(name)
	var b strings.Builder
	for i, full := range members {
		b.WriteString(r.compoundMemberListing(full, last && i == len(members)-1))
	}
	return b.String()
}

// compoundMemberListing is one member of a body, with the `;` that follows it
// where one does. See Runner.compoundVariableBody for the rule.
func (r *Runner) compoundMemberListing(full string, last bool) string {
	short := full[strings.LastIndex(full, memberSep)+1:]
	if r.nameHoldsACompound(full) {
		// A member that is a compound of its own is written with its body
		// between its own parentheses, and those parentheses are what
		// terminates it: nothing follows the `)`.
		return short + "=(" + r.compoundVariableBody(full, last) + ")"
	}
	d, ok := r.declarationOf(full)
	if !ok {
		return ""
	}
	d.name = short
	text := r.bareAssignmentDeclaration(d)
	if d.isArr || d.isAssoc || !last {
		return text + ";"
	}
	return text
}

// nameHoldsACompound reports whether the name reads as a compound in a
// listing: one that was declared, and one that was never declared but has
// members under it.
//
// The second is not a special case but the same fact reached the other way:
// `c=(a=1); c.b.y=3` lists as `(a=1;b=(y=3))` in the shell, so a name with
// something stored beneath it renders as a compound whether or not anything
// declared it one.
func (r *Runner) nameHoldsACompound(name string) bool {
	if r.isCompoundVariable(name) {
		return true
	}
	if _, ok := r.Vars[name]; ok {
		return false
	}
	if _, ok := r.Arrays[name]; ok {
		return false
	}
	if _, ok := r.AssocArrays[name]; ok {
		return false
	}
	return len(r.compoundDescendants(name)) > 0
}

// compoundVariableText is what a compound answers to a plain `$c`, and to
// `${c[0]}` and `${c[@]}` with it.
//
// The tree laid out over several lines, one member to a line, indented with a
// tab per level and wrapped in bare parentheses. Measured:
//
//	c=(a=1; b=(y=2))   →   "(\n\ta=1\n\tb=(\n\t\ty=2\n\t)\n)"
//	c=()               →   "(\n)"
//
// A member holding an array or a table is written with the same head its
// one-line listing has — `typeset -a arr=(` — and an element to a line inside
// it, which is why the head is a function of its own rather than spelled
// twice. See bareAssignmentHead.
func (r *Runner) compoundVariableText(name string) string {
	var b strings.Builder
	b.WriteString("(\n")
	r.compoundTreeInto(&b, name, 1)
	b.WriteString(")")
	return b.String()
}

func (r *Runner) compoundTreeInto(b *strings.Builder, name string, depth int) {
	pad := strings.Repeat("\t", depth)
	for _, full := range r.compoundMembers(name) {
		short := full[strings.LastIndex(full, memberSep)+1:]
		if r.nameHoldsACompound(full) {
			if len(r.compoundMembers(full)) == 0 {
				// An empty member is written nowhere, which is the shell's
				// own answer in both forms: `c=(a=1; e=())` renders and lists
				// with `a` alone.
				continue
			}
			b.WriteString(pad + short + "=(\n")
			r.compoundTreeInto(b, full, depth+1)
			b.WriteString(pad + ")\n")
			continue
		}
		d, ok := r.declarationOf(full)
		if !ok {
			continue
		}
		d.name = short
		if elems, ok := r.bareAssignmentElements(d); ok {
			b.WriteString(pad + bareAssignmentHead(bareAssignmentFlags(d), short) + "=(\n")
			for _, e := range elems {
				b.WriteString(pad + "\t" + e + "\n")
			}
			b.WriteString(pad + ")\n")
			continue
		}
		b.WriteString(pad + r.bareAssignmentDeclaration(d) + "\n")
	}
}

// nameHoldsAContainer reports whether the name is holding an array or a table
// — the two kinds an empty compound body adds nothing to. See
// assignCompoundVariable.
func (r *Runner) nameHoldsAContainer(name string) bool {
	if r.removed[name] {
		return false
	}
	if _, ok := r.Arrays[name]; ok {
		return true
	}
	_, ok := r.AssocArrays[name]
	return ok
}
