// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A local declaration takes the name's **members** with it.
//
// A compound variable's members are ordinary names spelled with a dot — see
// compoundvariable.go, which is the whole of that design — and a declaration
// that makes `c` local therefore has a second job: `c.a`, `c.b.q` and every
// other name under `c.` are part of what the declaration displaces. Shadowing
// the bare name alone left the members in the shell's own tables, so a
// function writing `typeset c=(a=1)` wrote `c.a` into its **caller's** scope
// and, worse, took the caller's members away first — silently, at status 0.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-19, `env -i PATH=/usr/bin:/bin
// LC_ALL=C /bin/ksh x.sh` over a script file with stdin on `/dev/null`:
//
//	c=(z=9); function f { typeset c=(a=1); }; f      out=[${c.a}][${c.z}] → [][9]
//	function f { typeset c=(a=1); }; f               ${c.a} empty, ${c+yes} empty
//	c=(z=9); function f { typeset c; c.a=1; }; f     [][9]
//	c=(z=9); function f { typeset -C c; … }          ${c.z} empty inside, 9 after
//	a=1; a.b=2; function f { typeset a=5; … }        ${a.b} empty inside, 2 after
//	c=(z=(q=7)); function f { typeset c=(a=1); }; f  ${c.z.q} is 7 after
//
// The last two are what say the rule is about the **prefix** and not about the
// compound mark: a plain scalar with a child has its child shadowed too, and a
// member nested two deep comes back with the rest. So what a declaration
// shadows is the name *and the namespace under it*.
//
// The controls that say the rule is no wider than that, measured in the same
// run and answered correctly both before this file and after it:
//
//	c=(z=9); function f { c.a=1; }; f                [1][9] — no declaration
//	c=(z=9); f() { typeset c=(a=1); }; f             [1][]  — POSIX-style f()
//	a=(9 9 9); function f { typeset a=(1 2); }; f    (9 9 9) — an array literal
//	s=keep; function f { typeset s=new; }; f         keep   — a scalar
//
// The second is the one that keeps this honest: `typeset` in a `f() { … }`
// body is not local in ksh93 at all (Semantics.TypesetLocalNeedsKeywordFunction),
// so nothing here may fire for it — and nothing does, because it hangs off
// Runner.shadow, which that axis already turns away (#3824).

// noteMemberName records that this shell has a name with a member separator in
// it, or a compound variable to hold one.
//
// A sticky flag and not a count, because it gates work rather than describing
// state: once a member name has existed, a declaration has to ask the question
// even if `unset` has since taken the name away. It is read on the hot path —
// Runner.shadow runs for every name every declaration in every dialect
// touches — and is false for the whole of bash, zsh, dash and ash, which have
// no spelling that could set it.
func (r *Runner) noteMemberName(name string) {
	if !r.memberNamesInUse && strings.Contains(name, memberSep) {
		r.memberNamesInUse = true
	}
}

// shadowCompoundNamespace is the second half of Runner.shadow: the members
// under the name are shadowed with it, and the fresh binding starts empty.
//
// Each member is shadowed through Runner.shadow itself rather than through a
// record of its own, which is what makes the rest of the machinery work
// without knowing this file exists: a shadowed member is an ordinary shadowed
// name, so the scope's exit puts it back, `unset` of a caller's local finds
// it, and the static seal a ksh93 `function` call puts up reads past it.
//
// Empty rather than inherited, which is measured: `c=(z=9); function f {
// typeset -C c; print "${c.z}"; }` writes an empty line there, so the body
// does not see the caller's members through its own declaration.
func (r *Runner) shadowCompoundNamespace(sc *scope, name string) {
	if sc.memberNamespaces == nil {
		sc.memberNamespaces = map[string]bool{}
	}
	sc.memberNamespaces[name] = true
	sc.rememberCompoundMark(name, r.compoundVariable[name])
	delete(r.compoundVariable, name)
	for _, m := range r.liveNamesUnder(name) {
		r.ownNameInNamespace(sc, m)
		// Recursive, and it terminates: the guard in Runner.shadow lets a
		// name through once per scope, and every name reached here is longer
		// than the one that reached it. A nested compound's own members are
		// therefore shadowed by the call this one makes for it.
		r.shadow(m)
		delete(r.Vars, m)
		delete(r.Arrays, m)
		delete(r.AssocArrays, m)
		delete(r.removed, m)
	}
}

// liveNamesUnder lists every name the stores hold under the namespace, at any
// depth. The name itself is not among them.
//
// Wider than Runner.compoundDescendants, deliberately: that one is for reading
// a compound's members and skips a name `unset` has hidden, while this one is
// for *displacing* everything under the prefix and a hidden name is part of
// what has to come back.
func (r *Runner) liveNamesUnder(name string) []string {
	prefix := compoundMemberPrefix(name)
	seen := map[string]bool{}
	add := func(k string) {
		if len(k) > len(prefix) && strings.HasPrefix(k, prefix) {
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
	for k := range r.removed {
		add(k)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}

// localizeMemberWrite makes a member local to the scope that owns its
// namespace, before the write that would otherwise create it outside.
//
// This is what a member the *body* creates needs, and the headline case is
// exactly that: `function f { typeset c=(a=1); }` shadows `c` when nothing
// called `c.a` exists yet, so `c.a` is made by the body and has nothing saved
// for it. Without this the name outlived the call — `${c.a}` read `1` and
// `${c+yes}` read `yes` in the caller, where ksh93 has both empty.
//
// Called from the five stores a name can be written through, which is the same
// list Runner.compoundVariableRetyped hangs off and for the same reason: the
// store is the authority and there are five of them.
func (r *Runner) localizeMemberWrite(name string) {
	if strings.IndexByte(name, memberSep[0]) < 0 {
		// The overwhelming majority of names, and the whole of four dialects:
		// a name with no separator in it can be under no namespace.
		return
	}
	r.noteMemberName(name)
	if len(r.scopes) == 0 {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	for i := 1; i < len(name); i++ {
		if name[i] != memberSep[0] {
			continue
		}
		parent := name[:i]
		if !sc.memberNamespaces[parent] && !r.claimNamespaceHere(parent) {
			continue
		}
		r.ownNameInNamespace(sc, name)
		r.shadow(name)
		sc.rememberCompoundMark(name, r.compoundVariable[name])
		return
	}
}

// restoreCompoundNamespace puts a namespace back the way the scope found it:
// whatever the call left under the prefix goes, and the compound mark the
// caller's name carried comes back.
//
// The members themselves are *not* put back here. Each one was shadowed as a
// name of its own, so each one is in the scope's list and is restored by the
// ordinary loop — this is only the sweep for what the call created and the
// mark, which no other record carries.
func (r *Runner) restoreCompoundNamespace(sc *scope, name string) {
	if sc.memberNamespaces[name] {
		for _, m := range r.liveNamesUnder(name) {
			if sc.namespaceOwned[m] {
				// The scope has a copy of this one and the list will put it
				// back. Asked of namespaceOwned rather than of sc.saved,
				// because restoreShadowedName deletes its records as it goes
				// and the list is in no order: a member restored before its
				// parent had already lost its entry there, and the sweep
				// deleted the caller's value it had just put back. That was
				// order-dependent and so was intermittent — rows with two
				// members passed while rows with one failed.
				continue
			}
			delete(r.Vars, m)
			delete(r.Arrays, m)
			delete(r.AssocArrays, m)
			delete(r.compoundVariable, m)
			delete(r.removed, m)
		}
		delete(sc.memberNamespaces, name)
	}
	if was, ok := sc.compoundMarkBefore[name]; ok {
		if was {
			if r.compoundVariable == nil {
				r.compoundVariable = map[string]bool{}
			}
			r.compoundVariable[name] = true
		} else {
			delete(r.compoundVariable, name)
		}
		delete(sc.compoundMarkBefore, name)
	}
}

// ownNameInNamespace records that this scope holds a copy of one member.
//
// Separate from the scope's other records because it must outlive them:
// restoreShadowedName deletes what it restores, and the namespace sweep runs
// against a list in no particular order, so the question "did this scope
// shadow that member" has to survive the member's own restore.
func (r *Runner) ownNameInNamespace(sc *scope, name string) {
	if sc.namespaceOwned == nil {
		sc.namespaceOwned = map[string]bool{}
	}
	sc.namespaceOwned[name] = true
}

// claimNamespaceHere makes the innermost scope own the namespace under a name
// it has already shadowed, for the case where there was nothing under it when
// the declaration ran.
//
// That case is the issue's second reduction: `function f { typeset c=(a=1); };
// f` shadows a name with no members, and `c.a` is made by the **body**. With
// no namespace recorded there was nothing to say the new member belonged to
// the call, and it outlived it — `${c.a}` read `1` in the caller where ksh93
// has it empty.
//
// It reports whether the scope owns the namespace afterwards, which is false
// for a name no declaration in this scope shadowed — a bare `c.a=1` in a
// function body with no `typeset` is a write to the shell's own name, measured
// and unchanged.
func (r *Runner) claimNamespaceHere(parent string) bool {
	if len(r.scopes) == 0 {
		return false
	}
	sc := r.scopes[len(r.scopes)-1]
	if sc.memberNamespaces[parent] {
		return true
	}
	if !sc.shadows(parent) {
		return false
	}
	r.shadowCompoundNamespace(sc, parent)
	return true
}

// rememberCompoundMark records whether a name was a compound variable at the
// moment this scope displaced it.
//
// Written directly rather than through setBool, which deletes on false: absent
// and false are different answers here, exactly as they are for
// scope.savedReadonly. Absent means this scope never shadowed the name;
// **false** means it shadowed one that was not a compound, and is what takes
// back a mark the *call* added — `function f { typeset c=(a=1); }; f` must
// leave the caller with no `c` at all, and a record that collapsed the two
// left it holding `typeset -C c=()`.
func (sc *scope) rememberCompoundMark(name string, was bool) {
	if _, seen := sc.compoundMarkBefore[name]; seen {
		return
	}
	if sc.compoundMarkBefore == nil {
		sc.compoundMarkBefore = map[string]bool{}
	}
	sc.compoundMarkBefore[name] = was
}
