// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// A namespace is a **name-resolution region over a compound**, and that is
// the whole model: there is no scope stack here and no second store.
//
// The construct was costed twice as "a table of named scopes on the Runner,
// neither of the two scoping shapes this shell has" before anybody tried the
// spelling that reads one. Measured on ksh93u+ 2012-08-01, 2026-09-19, each
// row its own script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with
// standard input on /dev/null:
//
//	written                                            ksh93u+
//	namespace ns { x=1; }; ${ns.x-unset}               unset
//	namespace ns { x=1; }; ${.ns.x-unset}              1
//	namespace ns { x=1; }; typeset -p .ns.x            .ns.x=1
//	namespace ns { f(){ echo hi; }; }; typeset +f      .ns.f()
//
// So a member is an **ordinary name spelled with a leading dot**, which is
// the same store `c=(a=1)` already puts `c.a` in — see
// interp/compoundvariable.go, where the flat dotted namespace was built for
// the compound variable and is what this construct turns out to need. Nothing
// here allocates a scope, and nothing here outlives the block except the
// members, which are ordinary variables and therefore already clone,
// subshell, list and unset correctly.
//
// What the region does is decide, for the code written between the braces,
// **which name a bare word means**:
//
//	x=OUTER; namespace ns { echo "${x-unset}"; }        OUTER
//	x=OUTER; namespace ns { x=IN; }; echo "$x"          OUTER
//	namespace ns { x=1; }; namespace ns { echo "$x"; }  1
//	namespace ns { x=1; }; namespace n2 { echo "$x"; }  unset
//	gv=GLOBAL; namespace ns { y=1; }; ${.ns.gv-unset}   GLOBAL
//	gv=A; namespace ns { y=1; }; gv=B; ${.ns.gv}        B
//
// Those six are **one rule and not two**. A read of a member the namespace
// does not hold falls through to the plain name — live, as the last row
// says — and a write always makes a member. So "the body reads the outer
// name" and "a write does not leak" are the read half and the write half of
// the same region, and the fall-through is what `${!.ns.@}` is listing when
// it answers the shell's ordinary parameters plus what the block assigned.
//
// **It is lexical, and that is measured rather than assumed.** A function
// defined *outside* and called from inside the braces sees the caller's
// scope, and a function defined *inside* carries the region wherever it is
// called from:
//
//	x=OUTER; g(){ echo "${x-unset}"; x=FROMG; }; namespace ns { g; }
//	                                        OUTER, and $x is FROMG after
//	namespace ns { x=1; f(){ echo "${x-unset}"; }; }; .ns.f
//	                                        1
//
// The second row is why the region is read off the *function's stored name*
// rather than saved and restored around a call: a function defined in a
// namespace is stored under the member name, so its name is the record of
// where it was written. A dynamic save-and-restore would have answered the
// first row wrong.
//
// **Namespaces are flat**, whatever the nesting of the blocks:
// `namespace a { namespace b { x=1; }; }` leaves it at `${.b.x}` and
// `${.a.b.x}` unset. That falls out of the region being replaced rather than
// appended to.
//
// # What is deliberately not modeled
//
// Four rows of the reference's own bookkeeping, each measured and each left
// out because this shell could not state it as a rule:
//
//   - `((n++))` inside the body writes the **global** where `n=$((n+1))`
//     makes a member — measured, `n=5; namespace ns { ((n++)); }` leaves the
//     global at 6, and the assignment form leaves it at 5 with `${.ns.n}` at
//     6. Here both make a member, which is the rule the other rows state.
//   - `unset x` inside the body leaves the *global* defined and empty there.
//   - Inside `( … )` the fall-through is not stable: a file whose second
//     subshell assigns inside the namespace makes the third subshell's read
//     of an unassigned name answer unset where the first answered the
//     global's value. Recorded on #3309 as an artifact, the way #2853's
//     surface 2 is.
//   - `typeset -p .ns` writes the whole block back as `namespace ns { … }`
//     and `${.ns}` answers the member names. Both are a *listing* of the
//     region and neither is what a compound variable writes here.
//
// Nothing above is reachable without [syntax.Dialect.NamespaceBlock], which
// one dialect sets, so the region is off in five of the six columns and the
// fast paths below are one comparison against the empty string.

// namespaceMemberName is the store key a namespace's member is kept under:
// the namespace's name with a dot in front of it, then the member's own.
//
// The leading dot is the spelling the reference uses and it is not decoration
// — `${ns.x}` is unset there where `${.ns.x}` is the member, so a namespace
// and a compound variable of the same name are different names.
func namespaceMemberName(ns, name string) string {
	return "." + ns + "." + name
}

// splitNamespaceMember reads a store key back: the namespace it belongs to
// and the member's own name, for a key this runner knows names a namespace.
//
// The split is at the **last** dot rather than the first, because a
// namespace's name may not hold one — `namespace a.b { … }` is refused for
// not being an identifier — while a member's may: `namespace ns { c=(m=1); }`
// leaves `.ns.c.m` in the store.
func (r *Runner) splitNamespaceMember(name string) (ns, member string, ok bool) {
	if len(name) < 2 || name[0] != '.' || len(r.namespaces) == 0 {
		return "", "", false
	}
	rest := name[1:]
	i := strings.IndexByte(rest, '.')
	if i <= 0 || i == len(rest)-1 {
		return "", "", false
	}
	if !r.namespaces[rest[:i]] {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// isNamespaceName reports whether a word may name a namespace.
//
// The reference judges the *word as written* and does so when the clause
// runs: `namespace .ns { x=1; }` is `.ns: is not an identifier` at status 1
// and `n=ns; namespace $n { x=1; }` is `.$n: invalid variable name`, so
// neither is a parse error and neither is expanded first. See
// [syntax.NamespaceClause.Name].
func isNamespaceName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// namespaceHoldsTheName reports whether a store key is one this shell is
// holding something under.
//
// The tables directly, and never through a reader: this is what decides which
// name a read *is*, so asking a reader would be circular.
func (r *Runner) namespaceHoldsTheName(name string) bool {
	if r.removed[name] {
		return false
	}
	if _, ok := r.Vars[name]; ok {
		return true
	}
	if _, ok := r.Arrays[name]; ok {
		return true
	}
	if _, ok := r.AssocArrays[name]; ok {
		return true
	}
	return r.compoundVariable[name]
}

// namespaceReadName is the store key a read of name comes from.
//
// One function for both directions, because the fall-through is one rule: a
// member the namespace does not hold reads the plain name, whether the read
// was written inside the braces as `$x` or outside them as `${.ns.x}`.
func (r *Runner) namespaceReadName(name string) string {
	if r.namespace != "" && isNamespaceName(name) {
		if member := namespaceMemberName(r.namespace, name); r.namespaceHoldsTheName(member) {
			return member
		}
		return name
	}
	if _, member, ok := r.splitNamespaceMember(name); ok && !r.namespaceHoldsTheName(name) {
		return member
	}
	return name
}

// namespaceWriteName is the store key a write of name lands on: always the
// member, which is what keeps a write from leaving the region.
func (r *Runner) namespaceWriteName(name string) string {
	if r.namespace == "" || !isNamespaceName(name) {
		return name
	}
	return namespaceMemberName(r.namespace, name)
}

// namespaceFuncName is namespaceWriteName for a function definition. Separate
// only so that the definition site reads as one: a function is stored under
// the member name, which is what `typeset +f` writes back and what
// [Runner.namespaceOfFunction] reads the region out of later.
func (r *Runner) namespaceFuncName(name string) string {
	return r.namespaceWriteName(name)
}

// namespaceFuncLookup is the name a *call* written inside the braces looks
// up: the member where one is defined, and the plain name otherwise.
//
//	namespace ns { f(){ echo IN; }; f; }          IN
//	g(){ echo OUTERFN; }; namespace ns { g; }     OUTERFN
//	namespace ns { f(){ echo hi; }; }; f          f: not found, 127
func (r *Runner) namespaceFuncLookup(name string) string {
	if r.namespace == "" || !isNamespaceName(name) {
		return name
	}
	member := namespaceMemberName(r.namespace, name)
	if _, ok := r.funcs[member]; ok {
		return member
	}
	return name
}

// namespaceOfFunction is the region a function's body runs in, read off the
// name it is stored under.
//
// This is the whole of what makes the region lexical. See the file comment:
// a function defined inside a namespace and called from outside still
// resolves through it, and one defined outside and called from inside does
// not — so the region cannot be the caller's and has to be the callee's.
func (r *Runner) namespaceOfFunction(name string) string {
	ns, _, ok := r.splitNamespaceMember(name)
	if !ok {
		return ""
	}
	return ns
}

// namespaceClause runs `namespace NAME { list }`.
func (r *Runner) namespaceClause(ctx context.Context, c *syntax.NamespaceClause) error {
	if !isNamespaceName(c.Name) {
		r.refuseNamespaceName(c.Name)
		return nil
	}
	if r.namespaces == nil {
		r.namespaces = map[string]bool{}
	}
	// The namespace outlives the block, which is what two blocks of one name
	// sharing a member requires — and the record of it is this set plus the
	// members themselves, both of which are ordinary runner state.
	r.namespaces[c.Name] = true
	saved := r.namespace
	// Replaced rather than appended to, which is what makes namespaces flat:
	// `namespace a { namespace b { x=1; }; }` leaves it at `${.b.x}`.
	r.namespace = c.Name
	defer func() { r.namespace = saved }()
	// The redirections belong to the construct and reach the whole body, the
	// way a brace group's do — measured, `namespace ns { echo a; } > out.txt`
	// leaves the line in the file and writes nothing to the terminal.
	return r.withRedirs(ctx, c.Redirs, func() error { return r.runList(ctx, c.List) })
}

// refuseNamespaceName reports a word that cannot name a namespace.
//
// A runtime refusal and not a parse one, at the dialect's ordinary fatal
// status: measured, `namespace .ns { x=1; }; echo after` writes
// `.ns: is not an identifier`, exits 1 and never reaches the `echo`, where
// `namespace ns; echo after` is a syntax error at 3.
//
// Three sentences, and which one is used is decided by the word rather than
// by the dialect — see [Diagnostics.NamespaceNameNotAVariable] for the rows.
// A word with a dot in it is named as it stands, because the dot is the
// character that would have made the store name ambiguous; anything else is
// named as the store name the shell was about to make, which is the word with
// a dot in front of it.
func (r *Runner) refuseNamespaceName(word string) {
	switch {
	case strings.ContainsRune(word, '['):
		r.fatal("%s\n", Wording(r.diag().NamespaceNameSubscripted,
			"%[1]s: cannot be an array", "."+word))
	case strings.ContainsRune(word, '.'):
		r.fatal("%s\n", Wording(r.diag().NamespaceNameInvalid,
			"%[1]s: is not an identifier", word))
	default:
		r.fatal("%s\n", Wording(r.diag().NamespaceNameNotAVariable,
			"%[1]s: invalid variable name", "."+word))
	}
}

// namespaceUnsetTarget reads a namespace's own name out of a store key, for
// the one caller that needs it: `unset .ns` has to take the members with it,
// exactly as `unset c` on a compound variable does.
//
// Measured: `namespace ns { x=1; }; unset .ns; ${.ns.x-unset}` is `unset`.
func (r *Runner) namespaceUnsetTarget(name string) (string, bool) {
	if len(name) < 2 || name[0] != '.' {
		return "", false
	}
	ns := name[1:]
	if !r.namespaces[ns] {
		return "", false
	}
	return ns, true
}

// namespaceKeys is what a prefix listing of `.ns.` adds beyond the members
// actually stored: the plain names the region reads through to, spelled as
// members of it.
//
// Empty for every prefix that is not a namespace's, which is every prefix in
// five of the six columns and nearly all of them in the sixth.
func (r *Runner) namespaceKeys(prefix string) []string {
	if len(r.namespaces) == 0 || len(prefix) < 3 || prefix[0] != '.' ||
		prefix[len(prefix)-1] != '.' {
		return nil
	}
	ns := prefix[1 : len(prefix)-1]
	if !r.namespaces[ns] || strings.ContainsRune(ns, '.') {
		return nil
	}
	var out []string
	for name := range r.Vars {
		if isNamespaceName(name) && !r.removed[name] {
			out = append(out, prefix+name)
		}
	}
	return out
}
