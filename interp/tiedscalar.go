// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A tie is two names for one value: a scalar holding a joined string and an
// array holding its fields, each reflecting the other. `typeset -T PATH path`
// is how the shell with the letter says it, and it is how that shell's own
// `PATH` and `path` are the same thing.
//
// Measured 2026-09-06 against zsh 5.9.2 with a scratch HOME and no startup
// files. It is one shell's letter: bash refuses `-T` outright, and ksh93's
// `-T` is a wholly different thing — `typeset -T tname` declares a *type* —
// so the letter rides Semantics.DeclareOptions the way `-H` and `-U` do,
// and every dialect without it goes on refusing it by name.
//
// The rules, each a measurement:
//
//   - Writing the scalar splits it on the separator and the array becomes
//     those fields; writing the array joins them and the scalar becomes that
//     string. `SCA=a:b:c` then `${#sca}` is 3, and `sca=(x y z)` then `$SCA`
//     is `x:y:z`. An append to the array joins too: `sca+=(w)` is `x:y:z:w`.
//   - The separator is the third operand and defaults to `:`. With `#`,
//     `S2=a#b#c` is three fields and `s2=(1 2)` is `1#2`.
//   - **`unset` of either name unsets both.** `unset SCA` leaves `sca` with
//     no elements *and* unset — `${+sca}` is 0 — and `unset sca` leaves
//     `$SCA` unset. Half a tie is not a state this shell has.
//   - A declaration with no value leaves the scalar set and empty and the
//     array with *no* elements, where an explicit `SCA=”` leaves it with
//     one empty element. The array is the storage and the empty string
//     splits into one field; the declaration assigns nothing and so splits
//     nothing.
//   - Tying a name that is already tied to something else is refused —
//     `can't tie already tied scalar: SCA` — and tying it to the same array
//     again is silence and 0. Tying a name to itself is `can't tie a
//     variable to itself: A`.
//
// One store, not two, is what this is not: both names are real cells and each
// write mirrors into the other. A single store with the scalar produced on
// demand would be tidier and is wrong here, because an exported tie has to
// reach a child through the environment, and what reaches a child is what is
// in the variable table.
type tie struct {
	// scalar and array are the two names, and sep joins and splits.
	scalar string
	array  string
	sep    string
	// special says the shell made this tie for itself rather than a script
	// making it with the letter, and it decides what a *local* of one half
	// means. See tielocal.go, which is where that measurement lives.
	special bool
	// depth is how many scopes were on the stack when the tie was made, so a
	// scope *deeper* than that can be told from the one the declaration
	// itself took — see tieShadowedInItsScope.
	depth int
}

// tieOf is the tie a name is half of, and whether it is one at all.
func (r *Runner) tieOf(name string) (tie, bool) {
	t, ok := r.tied[name]
	return t, ok
}

// tieNames records a tie under both of its names, so either half finds it.
func (r *Runner) tieNames(t tie) {
	if r.tied == nil {
		r.tied = map[string]tie{}
	}
	r.tied[t.scalar] = t
	r.tied[t.array] = t
}

// untie forgets a tie, from either half — which is what `unset` leaves
// behind, measured: `unset SCA` then `SCA=a:b` gives a plain scalar and
// `${#sca}` stays 0.
func (r *Runner) untie(name string) {
	t, ok := r.tied[name]
	if !ok {
		return
	}
	delete(r.tied, t.scalar)
	delete(r.tied, t.array)
}

// mirrorScalarToArray is the half of the tie a scalar assignment drives: the
// value is split on the separator and becomes the array's elements.
//
// The re-entrancy guard is the whole of what keeps this from looping, because
// the mirror in the other direction writes the scalar again. It is a field
// rather than an argument threaded through six call sites, because the two
// choke points — setVarAs and storeArray — are reached from everywhere an
// assignment can be written and neither knows who called it.
func (r *Runner) mirrorScalarToArray(name, value string) {
	t, ok := r.tied[name]
	if !ok || t.scalar != name || r.mirroring || r.tieDetached(t) {
		return
	}
	r.mirroring = true
	defer func() { r.mirroring = false }()
	r.setArray(t.array, strings.Split(value, t.sep))
}

// mirrorArrayToScalar is the other half: the elements joined with the
// separator become the scalar.
// The array rather than its elements, so that the read happens *after* the
// guard and not before the call. Almost no array is tied, and reading one to
// hand it to a function that returns without looking was, measured, an
// eighth of what a real startup spent under storeArray — a sorted walk of
// every element, on every write to every array in the shell.
func (r *Runner) mirrorArrayToScalar(name string, a Array) {
	t, ok := r.tied[name]
	if !ok || t.array != name || r.mirroring || r.tieDetached(t) {
		return
	}
	r.mirroring = true
	defer func() { r.mirroring = false }()
	r.setVar(t.scalar, strings.Join(r.readArray(a), t.sep))
}

// defaultTieSeparator is what joins and splits where the declaration named
// nothing. Measured: `typeset -T S s` then `S=a:b` is two elements.
const defaultTieSeparator = ":"

// declareTie is `typeset -T SCALAR array [sep]`.
//
// Its own path out of biDeclare because its operands are not a list of names:
// the first is a scalar, the second an array and a third is the separator. A
// declaration reaching the ordinary loop would have read `sca` as a second
// name to give the attributes to, which is not what the line says.
//
// The other letters still apply, and to the halves they belong to: `-x`
// exports the scalar the way `typeset -gxTU LOG_PATH logpath` means it, and
// `-U` makes the array unique so the joined scalar has no duplicates either.
func (r *Runner) declareTie(builtin string, args []string, f declareFlags) int {
	if len(args) < 2 {
		r.diagf("%s\n", Wording(r.diag().TiedNamesRequired,
			"-T requires names of scalar and array"))
		return 1
	}
	// The scalar may carry a value — `typeset -T R=x:y r` is measured to
	// leave `r` holding `x` and `y` — and the array may carry an array
	// literal, which reaches the name by the ordinary assignment path and
	// so arrives here as a bare word. A *scalar* value on the array half is
	// the one shape refused, in the shell's own words: `typeset -T S s=plain`
	// is `second argument of tie must be array: s`.
	scalar, value, hasValue := strings.Cut(args[0], "=")
	array := args[1]
	if strings.Contains(array, "=") {
		name, _, _ := strings.Cut(array, "=")
		r.diagf("%s\n", Wording(r.diag().TieSecondMustBeArray,
			"second argument of tie must be array: %s", name))
		return 1
	}
	// Both halves must be names, and the check earns its place twice over.
	// It is what this shell says to `typeset -T A ':'` — `not valid in this
	// context: :`, measured — and it is what keeps a *reordered* operand
	// list from tying something nobody wrote: the parser lifts an array
	// literal out of the arguments and appends its bare name at the end, so
	// `typeset -T R r=(a b) ':'` reaches here as `R`, `:`, `r` and a
	// positional reading would tie `R` to `:`. Refusing the non-name is the
	// honest answer to a spelling this engine cannot see in order.
	fatal, takes := r.nameRules(builtin)
	for _, n := range []string{scalar, array} {
		if r.isBuiltinName(n, takes) {
			continue
		}
		if r.unspecified {
			return 2
		}
		return r.badBuiltinName(builtin, n, n, fatal)
	}
	sep := defaultTieSeparator
	if len(args) > 2 {
		sep = args[2]
	}
	if scalar == array {
		// Reported and then fatal, where the dialect says a declaration's
		// refused operand is — the same answer the name check above uses,
		// because this is the same kind of thing: an operand a declaration
		// will not take. Measured, and it is not the answer the *other* two
		// tie refusals get: `can't tie already tied scalar` and `second
		// argument of tie must be array` are both reported and run on.
		// That split is this shell's own and is matched rather than tidied.
		r.diagf("%s\n", Wording(r.diag().TieToItself,
			"can't tie a variable to itself: %s", scalar))
		if r.ask(fatal, "a refused declaration operand ending the script") {
			r.status = 1
			r.fatalQuiet()
		}
		return 1
	}
	if t, already := r.tieOf(scalar); already && t.array != array {
		// Tying the same pair again is silence and 0 — measured — so only a
		// tie to a *different* name is refused. The separator is not part of
		// that question: re-declaring with another separator is the same
		// pair.
		r.diagf("%s\n", Wording(r.diag().AlreadyTiedScalar,
			"can't tie already tied scalar: %s", scalar))
		return 1
	}
	// Local unless `-g`, the same rule every other declaration follows, and
	// both halves take a shadow: a function-local tie is gone on return,
	// measured — `typeset -T L1 l1` inside a function leaves `$L1` unset
	// after it and `${+l1}` 0.
	if !f.global {
		r.shadowTypeset(scalar)
		r.shadowTypeset(array)
		if r.unspecified {
			return r.status
		}
		// The tie itself is not in the variable tables, so the scope's
		// save-and-restore does not carry it. Undone by hand on the way out,
		// and only where there was a scope to undo it in.
		r.AtFunctionReturn(func() { r.untie(scalar) })
	}
	// The attributes go to the halves they belong to. `-U` and the rest go
	// to both — measured, `typeset -TU A a` lists as `export -UT` and
	// `typeset -aUT`, and `-r` as `-rT` and `-arT` — and **export goes to
	// the scalar alone**, because the scalar is what a child can be told:
	// `typeset -TUx A1 a1` lists the array back as `typeset -aUT` with no
	// `x` in it.
	r.applyAttributes(scalar, f)
	arrayFlags := f
	arrayFlags.export = false
	r.applyAttributes(array, arrayFlags)
	// The depth a *script* tie was made at, which is what tells a nested
	// function's `local` of one half from the declaration's own shadow above.
	// A `-g` tie is the global one whatever it was written inside.
	depth := 0
	if !f.global {
		depth = len(r.scopes)
	}
	r.tieNames(tie{scalar: scalar, array: array, sep: sep, depth: depth})
	// Readonly last, the same order biDeclare follows and for the same
	// reason: it decides whether the assignment below is allowed at all, and
	// applying it up front made a declaration refuse its own value.
	defer func() {
		if f.readonly && !f.readonlyOff {
			r.markReadonly(scalar)
			r.markReadonly(array)
		}
	}()
	if hasValue {
		// The declaration's own value, which splits into the array like any
		// other assignment to the scalar.
		r.setVarAs(scalar, value, assignedByDeclaration)
		return 0
	}
	// A name that already holds a value keeps it and is read back through
	// the tie that has just arrived — the same rule an integer or a case
	// attribute follows. This is what makes `PATH=…; typeset -T PATH path`
	// give `path` the fields rather than emptying both.
	if v, ok := r.getVar(scalar); ok {
		r.mirrorScalarToArray(scalar, v)
		return 0
	}
	// The scalar exists and is empty, and the array has *no* elements at
	// all — measured, and not the same as an empty scalar assigned on
	// purpose: `typeset -T S s` leaves `${#s}` 0 where a later `S=''` leaves
	// it 1, because the empty string splits into one field and a
	// declaration splits nothing. The mirror is held off for exactly that
	// reason.
	r.mirroring = true
	r.setVar(scalar, "")
	r.setArray(array, nil)
	r.mirroring = false
	return 0
}

// tieListing is `typeset -T` with no operands: every tied name, both halves
// of each.
//
// Plain assignments rather than `typeset -p`'s shape — measured, and this is
// the correction a first reading needed: it is `A=1:2` and `a=( 1 2 )`, not
// `typeset -T A a=( 1 2 )`. Not quite the *bare* listing either, which in
// this shell writes scalars alone: an array half is written here, in the
// parenthesised form, so both halves of every tie appear.
//
// Sorted by name, which puts each pair's scalar and array apart rather than
// together: `A`, `CDPATH`, … then `a`, `cdpath`, because the upper-case
// halves sort first.
func (r *Runner) tieListing(namesOnly bool) int {
	seen := make(map[string]bool, len(r.tied))
	for name := range r.tied {
		seen[name] = true
	}
	for _, name := range sortedNames(seen) {
		d, ok := r.declarationOf(name)
		if !ok {
			continue
		}
		if namesOnly {
			// `typeset +T`, the same walk with the values left off — the
			// sign means here what it means on every other letter of this
			// builtin. Measured 2026-09-10 on zsh 5.9.2: both halves of
			// every tie, one name to a line, sorted, and no attribute words.
			r.printf("%s\n", name)
			continue
		}
		r.printf("%s=%s\n", name, r.listedDeclarationValue(d))
	}
	return 0
}

// Tie makes a scalar and an array two names for one value, from a dialect
// rather than from a script.
//
// The seam the *built-in* ties need. A shell whose own `PATH` and `path` are
// the same thing has to say so before the first command runs, and
// `typeset -T`'s own path is the builtin's — it takes flags, takes a scope,
// refuses a name that is already tied and refuses one that is not an
// identifier, none of which a startup call has any use for.
//
// The seeding is the same rule the builtin follows and is the reason this is
// safe over `PATH`: a scalar that already holds a value keeps it and the
// array is filled from it. A scalar with no value leaves the array with no
// elements rather than one empty one, which is what the shell being copied
// does — measured, `zsh -f` with no `CDPATH` in the environment has
// `${#cdpath}` 0 and `$CDPATH` empty.
//
// Nothing about the export attribute is decided here, deliberately. Measured:
// `PATH` is `export -T` when the environment supplied it and plain
// `typeset -T` when it did not, and writing `cdpath` never exports `CDPATH`.
// So a tie *inherits* whatever the scalar already was and confers nothing.
func (r *Runner) Tie(scalar, array, sep string) {
	if sep == "" {
		sep = defaultTieSeparator
	}
	// special, and that is the whole of what this seam adds over the letter:
	// see tielocal.go for the two answers a `local` of one half gets.
	r.tieNames(tie{scalar: scalar, array: array, sep: sep, special: true})
	if v, ok := r.getVar(scalar); ok {
		r.mirrorScalarToArray(scalar, v)
		return
	}
	r.mirroring = true
	r.setVar(scalar, "")
	r.setArray(array, nil)
	r.mirroring = false
}
