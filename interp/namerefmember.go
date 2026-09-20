// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A **compound variable's member** is the third shape a name reference may be
// aimed at, in the one dialect that has members at all.
//
// [Runner.namerefTargetIsAName] admitted two: a name, and a name with a
// subscript. ksh93 admits a dotted member path as well, and this shell refused
// one outright — `typeset zz=(x=0); typeset -n c=zz.b` was `typeset: zz.b:
// invalid variable name` and the script ended, over a declaration ksh93 takes
// in silence. Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01
// (`/bin/ksh`), script files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with
// standard input on the null device:
//
//	typeset zz=(x=0); typeset -n c=zz.x; print -r -- "[${c}]"   [0]
//	typeset zz=(x=0); typeset -n c=zz.b; c=7; typeset -p zz     typeset -C zz=(b=7;x=0)
//	typeset zz=(x=5); typeset -n w; for w in zz.x; do echo "[$w]"; done  [5]
//	typeset zz=(x=0); typeset -n c; c=zz.x; print -r -- "[${c}]"        [0]
//
// The narrowest row is the first: the member it names **already exists**, and
// it was refused all the same — so this is the declaration's name check and
// nothing about creating a member.
//
// # The parent has to be there
//
// A member path whose **base name does not exist** is refused, and in words of
// its own rather than the bad-name sentence. Measured on the same run:
//
//	typeset -n c=qq.b          typeset: qq.b: no parent, 1, and the script ends
//	typeset zz=(x=0); …=zz.b   taken
//	zz=1; typeset -n c=zz.b    taken — a *scalar* parent is a parent
//	zz=(1 2)   / typeset -A zz taken
//	typeset zz                 taken: a declaration with no value is a parent
//	typeset zz=(x=0); …=zz.b.q taken — only the base is looked for, not the path
//	typeset -n c=1a.b          typeset: 1a.b: invalid variable name — shape first
//
// So the check is on the **first segment alone** and on its presence alone,
// which is why [Runner.namerefMemberParentIsThere] asks the four tables and
// the bare-declaration record rather than asking what kind of thing it found.
// `zz.b.q` over a compound `zz` with no `b` in it is taken, which rules out
// any reading where the whole path has to resolve.
//
// # Why presence of the wording is the gate
//
// bash has no compound variables — `zz=(x=0)` is an array of one element there
// — so a dotted target can never denote anything and bash refuses one:
// “declare: `zz.b': invalid variable name for name reference“ at 1, measured
// on 5.3.20 the same day. The dialect that has a sentence for a member path
// with no parent is exactly the dialect that has members, so
// [Diagnostics.NamerefTargetHasNoParent] gates the shape as well as wording
// it. That is the shape interp/compound.go's dotted function name already
// reads — see Diagnostics.FunctionNameDiscipline — and it keeps every column
// but one on the answer it gave before.
//
// # What this deliberately does not reach
//
// The `no parent` refusal is written at the **declaration** and nowhere else.
// ksh93 refuses the other aiming routes too, but in a different location form:
// `typeset -n c=qq.b` is `t.sh[1]: typeset: qq.b: no parent` where `typeset
// -n c; c=qq.x` is `t.sh: line 1: qq.x: no parent` — the builtin named and the
// `[line]` location against the shell's plain line form. Those routes keep the
// refusal they already made, which is the bad-name sentence at 1 with the
// script ending: the wrong noun, the right shape, and unchanged by this. The
// location form is its own question and its own row.
//
// namerefMemberPathBase splits a member path into the name it starts at, and
// reports whether the word is a member path at all.
//
// A subscript on the last member comes off before the dots are counted,
// because its own text may hold one — `zz.m[a.b]` is the member `m` of `zz` at
// the key `a.b`, not a four-segment path. A leading dot is declined rather
// than read as an empty base: `${.sh.version}` and a `namespace` member both
// begin with one and neither is this.
func namerefMemberPathBase(target string) (string, bool) {
	path := target
	if i := strings.IndexByte(target, '['); i >= 0 && strings.HasSuffix(target, "]") {
		path = target[:i]
	}
	segments := strings.Split(path, memberSep)
	if len(segments) < 2 {
		return "", false
	}
	for _, segment := range segments {
		if !isNameLike(segment) {
			return "", false
		}
	}
	return segments[0], true
}

// namerefMemberPath is the shape above asked in a dialect, which is where the
// gate above lives.
func (r *Runner) namerefMemberPath(target string) (string, bool) {
	if r.diag().NamerefTargetHasNoParent == "" {
		return "", false
	}
	return namerefMemberPathBase(target)
}

// namerefMemberParentIsThere reports whether the base of a member path names
// something this shell has.
//
// Every kind counts and none of them has to be a compound: the rows above have
// a scalar, both arrays, a compound and a valueless declaration all standing as
// parents. A compound is asked first because it is in none of the value
// tables — its value is the members under it, and markCompoundVariable took
// whatever else the name held.
func (r *Runner) namerefMemberParentIsThere(base string) bool {
	if r.isCompoundVariable(base) || r.declaredNameHolds(base) || r.declaredBare[base] {
		return true
	}
	_, set := r.getVar(base)
	return set
}

// namerefTargetIsAMember reports whether a reference may be aimed at this
// word as a compound member.
func (r *Runner) namerefTargetIsAMember(target string) bool {
	base, ok := r.namerefMemberPath(target)
	return ok && r.namerefMemberParentIsThere(base)
}

// namerefMemberWithoutAParent is the other half: a well-shaped member path
// whose base is not there, which the declaration refuses in its own words.
//
// Asked **ahead** of the bad-name check rather than behind it, because a
// target that is a member path with no parent is not a name by
// [Runner.namerefTargetIsAName] either — the parent is half of what makes it
// one — so behind it the sentence would never be reached.
func (r *Runner) namerefMemberWithoutAParent(target string) bool {
	base, ok := r.namerefMemberPath(target)
	return ok && !r.namerefMemberParentIsThere(base)
}
