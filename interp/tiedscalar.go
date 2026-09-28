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
//     `$SCA` unset. Half a tie is not a state this shell has. Whether the
//     *pairing* survives is a second question and the two kinds answer it
//     differently — see the `special` field below and unsetName.
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
	// making it with the letter, and it decides two things.
	//
	// What a *local* of one half means — see tielocal.go, which is where
	// that measurement lives — and whether `unset` forgets the pairing. A
	// script's tie is forgotten and the next assignment writes a plain
	// scalar; the shell's own pairs survive, so `unset PATH; PATH=/y` splits
	// into `path` again. Holding both kinds in one map with one answer is
	// what left a real startup with an empty `$path` nothing would refill
	// (#1631). See unsetName.
	special bool
	// wordless says the join is one the shell maintains between a pair of
	// its **own specials** rather than a tie, which changes nothing about
	// how the two halves move together and everything about what a listing
	// calls them.
	//
	// It is a second field beside `special` and not a reading of it, because
	// the panel has both kinds of shell-installed join and they describe
	// differently. Measured 2026-09-27 on zsh 5.9.2 under `-f` from a script
	// file, in one run:
	//
	//	${(t)PATH}    scalar-tied-export-special
	//	${(t)path}    array-tied-special
	//	${(t)WATCH}   scalar-special
	//	${(t)watch}   array-special
	//
	// and the second pair really is joined, in the same shell:
	// `watch=(a b)` makes `$WATCH` `a:b`, and `WATCH=cc` makes `$watch` one
	// element. So the join is not what the word turns on — `typeset +T`
	// lists the eight tied pairs and `ZSH_EVAL_CONTEXT`, and names neither
	// half of this one — which is the measurement #4907 asked for before any
	// code, and the answer is that `tied` marks the **tie table** rather
	// than the behavior a tie produces.
	//
	// So a pair here is joined and untied: it mirrors, it does not carry the
	// word, and no tie listing walks it.
	wordless bool
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

// untie forgets a tie, from either half — which is what `unset` of a
// *script's* tie leaves behind, measured: `unset SCA` then `SCA=a:b` gives a
// plain scalar and `${#sca}` stays 0. One of the shell's own pairs is not
// untied by `unset` at all; see the `special` field and unsetName.
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
	if !ok || t.scalar != name || r.mirroring || r.tieDetached(t) || r.pairHalfWasRemoved(t, t.array) {
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
	if !ok || t.array != name || r.mirroring || r.tieDetached(t) || r.pairHalfWasRemoved(t, t.scalar) {
		return
	}
	r.mirroring = true
	defer func() { r.mirroring = false }()
	r.setVar(t.scalar, strings.Join(r.readArray(a), t.sep))
}

// defaultTieSeparator is what joins and splits where the declaration named
// nothing. Measured: `typeset -T S s` then `S=a:b` is two elements.
const defaultTieSeparator = ":"

// nulTieSeparator is the separator an operand with no bytes in it names.
//
// Not a special case bolted onto the empty operand: it falls out of
// tieSeparatorOperand's rule, since the first byte of a string with no bytes
// in it is the zero byte. Named because two readers need it — the operand
// and the listing — and a second spelling of `"\x00"` is how they would come
// apart.
const nulTieSeparator = "\x00"

// tieSeparatorOperand reads the third operand of `typeset -T`: **one byte**,
// the operand's first, and NUL where the operand has none.
//
// Three readings agree on every separator anybody writes and part only at the
// edges, so the edges are what settled it. Measured 2026-09-25 on zsh 5.9.2
// with `-f`, by `typeset -T A a SEP; a=(p q); printf '%s' "$A" | od -c`:
//
//	SEP        joins           lists back
//	'ab'       p a q           `a`
//	$'x\0'     p x q           `x`
//	'é'        p \303 q        `$'\M-C'`
//	$'\0'      p \0 q          `''`
//	''         p \0 q          `''`
//
// The first two rows part "the whole operand" from the other two readings;
// the third parts "the first character" from "the first byte", since `é` is
// two bytes and only its lead one reaches the join. The last two rows are the
// same separator written two ways, which is what says the rule is keyed on
// the **byte** rather than on the operand: an operand with no bytes gives the
// zero byte exactly as one starting with a NUL does.
//
// The whole operand was the separator here, so an empty one joined with
// nothing and split a value into its characters, and `'ab'` put two bytes
// between fields (#4515).
func tieSeparatorOperand(operand string) string {
	if operand == "" {
		return nulTieSeparator
	}
	return operand[:1]
}

// tieSeparatorWord is the separator as a *word*, for a listing that has to be
// the declaration it read back.
//
// Every byte but one is written by the ordinary value quoting. The NUL is the
// one no word can carry, so the quoting has nowhere to put it — and it does
// not have to: by tieSeparatorOperand's rule the empty word names that byte,
// and it is the only word that does, so the listing round-trips through the
// one spelling there is. Measured 2026-09-25 on zsh 5.9.2:
//
//	typeset -T A a $'\0'   lists back with an empty word for the separator,
//	                       and reading that line again gives the same one
//
// This shell wrote the byte as an escape instead, which is the readable
// answer and the wrong one: pasting `$'\C-@'` back declares a tie separated
// by an escape's *first byte* — a `$` — rather than by a NUL.
func (r *Runner) tieSeparatorWord(sep string) string {
	if sep == nulTieSeparator {
		return "''"
	}
	return r.declareQuoted(sep, ListedValueAlone)
}

// declareTie is `typeset -T SCALAR array [sep]`.
//
// Its own path out of biDeclare because its operands are not a list of names:
// the first is a scalar, the second an array and a third is the separator. A
// declaration reaching the ordinary loop would have read `sca` as a second
// name to give the attributes to, which is not what the line says.
//
// The other letters still apply, and to the halves they belong to: `-x`
// exports the scalar the way `typeset -gxTU LOG_PATH logpath` means it, and
// `-U` goes to both names, where it deduplicates whichever of them a script
// writes — see interp/tiedunique.go, which is the whole of that letter on a
// tie and the reason it is not a property of the pair.
// tieDeclaration answers the shapes the `-T` letter has of its own, for every
// declaration word that carries the letter. ok is false where the line is not
// a tie at all and the caller's ordinary loop should have it.
//
// **Folded rather than copied**, which is #5074's lesson one letter along.
// There, `local` was the one word whose loop did not carry the
// inconsistent-type refusal, because the refusal had been written at each
// word's own site and `local`'s was the site that never got one. Here the
// same word was the one that never reached the tie: `typeset -T IN in=(a b)`
// and `declare -T` tie, `local -T` declared two ordinary names and left them
// untied, measured 2026-09-28 on zsh 5.9.2 (#5095). The three shapes live
// here so that a third word with the letter cannot be given two of them.
//
//	-T with no operands 	the listing of every tie, both halves of each
//	+T with operands    	a refusal — `use unset to remove tied variables`
//	-T with operands    	the declaration itself
//
// The sign picks the listing's shape as it does everywhere else on these
// builtins: `+T` alone writes the tied names and no values.
func (r *Runner) tieDeclaration(word string, args []string, f declareFlags,
	wordTakesAScope bool,
) (int, bool) {
	if !f.tie {
		return 0, false
	}
	if len(args) == 0 {
		// A tie is the one attribute whose *listing* is how a script finds
		// the pairs at all, which is why this filter is built where every
		// other attribute letter's is not.
		return r.tieListing(f.remove), true
	}
	if f.remove {
		// The plus form **with operands**, which is a refusal and not an
		// untie — see Runner.refuseUntie.
		return r.refuseUntie(word), true
	}
	// The export letter asks for `-g` as well, under the words that answer
	// yes to it — and a tie is a declaration like any other. Measured
	// 2026-09-28 on zsh 5.9.2, `f(){ typeset -xT A a=(x y) }` leaves `$A` as
	// `x:y` after the call and `${(t)A}` without `local` in it, where the
	// same line without the letter is gone on return.
	//
	// **Asked here and of the caller**, because the exemption that used to
	// carry this was structural: exportLetterDeclaresAGlobal's own doc says
	// `local` never comes through it since biLocal has a loop of its own.
	// Both words reach this helper now, so the loop no longer says which
	// word is which and the caller does. `local -xT` stays local, measured
	// the same day.
	if !f.global && !wordTakesAScope && len(args) > 0 {
		scalar, _, _ := strings.Cut(args[0], "=")
		if r.exportLetterDeclaresAGlobal(scalar, f) {
			f.global = true
		}
	}
	// The operands of `-T` are not a list of names: they are a scalar, an
	// array and — where a third is given — the separator.
	return r.declareTie(word, args, f), true
}

func (r *Runner) declareTie(builtin string, args []string, f declareFlags) int {
	if len(args) < 2 {
		r.diagf("%s\n", Wording(r.diag().TiedNamesRequired,
			"-T requires names of scalar and array"))
		return 1
	}
	// And a **fourth** operand, which is the other end of the same sentence:
	// `-T` takes a scalar, an array and at most a separator, so a list longer
	// than three is refused before anything in it is read.
	//
	// **First of the refusals**, which is the opposite of where the readonly
	// one stands and is measured a row at a time for the same reason. Every
	// other operand refusal loses to it once there are four operands:
	// `typeset -T A A b c` is this and not `can't tie a variable to itself`,
	// `typeset -T A ':' b c` and `typeset -T ':' a b c` are this and not `not
	// valid in this context`, `typeset -T A a=v b c` is this and not `second
	// argument of tie must be array`, and `typeset -r RO=v; typeset -T RO ro
	// x y` is this and not `read-only variable`. Each of those four is the
	// winner at *three* operands, where there is no count to refuse.
	//
	// Reported and run on, like the two name refusals and unlike the frozen
	// scalar: `typeset -T A a b c; print $?` writes the sentence and then `1`,
	// and the pair is not made — `typeset -p A a` afterwards is `no such
	// variable` for both. Measured 2026-09-28 on zsh 5.9.2 under `-f` from a
	// script file, and under every word that reaches here: `declare`,
	// `export` and `readonly` each write their own name in the location
	// (#5100).
	//
	// **The count is not disturbed by the operand reordering** this engine
	// does — the parser lifts an array literal out and appends its bare name,
	// which permutes the list without changing its length — so this row is
	// right for `typeset -T A a=(1 2) b c` as well, and does not wait on
	// #5096. That is why the two are separate changes.
	if len(args) > 3 {
		r.diagf("%s\n", Wording(r.diag().TieTakesThreeOperands,
			"too many arguments for -T"))
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
		if r.isBuiltinName(builtin, n, takes) {
			continue
		}
		if r.unspecified {
			return 2
		}
		return r.badBuiltinName(builtin, n, n, fatal)
	}
	sep := defaultTieSeparator
	if len(args) > 2 {
		sep = tieSeparatorOperand(args[2])
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
		scalarFresh, _ := r.shadowTypeset(scalar)
		arrayFresh, _ := r.shadowTypeset(array)
		if r.unspecified {
			return r.status
		}
		// **A shadow makes a new cell; it does not empty the one the name
		// was reading.** Every other declaration word does that for itself
		// through declareEmpty, and this path did not — so a local tie over
		// an outer `S=v` read `v` inside the call where the reference reads
		// nothing. Measured 2026-09-28 on zsh 5.9.2, `S=v` then
		// `f(){ typeset -T S s; print "[$S]" }` is `[]` there and was `[v]`
		// here (#5095).
		//
		// Every half this line makes a new cell for, and **not** only the
		// ones it gives no value to. A half that is about to be assigned
		// could be skipped and was, until a mutant dropping the test
		// survived: both values are written below this point, so emptying
		// first changes nothing for them. One condition rather than two,
		// because the second could not be graded.
		for _, half := range []struct {
			name  string
			fresh bool
		}{{scalar, scalarFresh}, {array, arrayFresh}} {
			if !half.fresh {
				continue
			}
			r.declareEmpty(half.name, half.fresh, f.export || f.readonly,
				withoutMatching(f) != (declareFlags{}),
				f.leavesAnAttribute(),
				f.inherit || r.LocalInheritsTheOuterValue(), false)
		}
		// And the **export attribute**, which the shadow deliberately does
		// not take off: whether a local inherits it is the dialect's answer
		// and localExportAttribute asks it. The ordinary declaration loop
		// asks; this path did not, so a local tie over an exported global of
		// the same name kept the attribute — `export outer=old` then
		// `f(){ local -xT OUTER outer; outer=(i n) }` left `${(t)outer}` as
		// `array-local-tied-export` where the reference says
		// `array-local-tied`, and listed the half as `local -axT` rather
		// than `typeset -aT` (#5098).
		//
		// **The letter belongs to the scalar and not to the pair**, which is
		// what the two calls say: `-xT` exports the scalar the script named
		// and leaves the array half an ordinary local, so only the scalar
		// counts the letter as its own. Measured the same day — with no
		// exported global in the way, the array half is `array-local-tied`
		// under `-xT` already, so nothing here is giving it the letter; this
		// is only about what it inherits.
		r.localExportAttribute(scalar, f.export)
		r.localExportAttribute(array, false)
		if r.unspecified {
			return r.status
		}
		// The tie itself is not in the variable tables, so the scope's
		// save-and-restore does not carry it. Undone by hand on the way out,
		// and only where there was a scope to undo it in.
		r.AtFunctionReturn(func() { r.untie(scalar) })
	}
	// A **frozen scalar** refuses the tie, and it is the readonly refusal
	// rather than a fourth tie refusal of its own: the sentence is
	// `read-only variable: S` with no builtin in the location, which is what
	// `export x=2` over a frozen name already says here, and the script is
	// over. Measured 2026-09-25 and again 2026-09-26 on zsh 5.9.2 under `-f`
	// from a script file — `typeset -r S=v; typeset -T S s` writes
	// `z.sh:2: read-only variable: S` and nothing after it runs, where this
	// shell took the line and listed the pair back as `typeset -rT S s=( v )`
	// (#4503).
	//
	// **Behind the shadow**, which is where the measurement puts it and not
	// where a declaration's other operand checks are. A local tie is a local
	// declaration, and a local declaration shadows a frozen global rather
	// than being refused by it: `typeset -r A=v; f(){ typeset -T A a }` is
	// taken at 0 there, `$A` is empty inside the call and `v` again after it,
	// and only `typeset -gT` — which takes no shadow — refuses. Asking ahead
	// of the shadow refused the local form too, which is a row that agreed
	// before this change and would have stopped agreeing because of it.
	//
	// **Last of the refusals** otherwise, which is measured a row at a time
	// rather than assumed, because every other one of them wins over it:
	// `typeset -r RO=v; typeset -T RO ro=x` is `second argument of tie must
	// be array: ro`, `typeset -T RO RO` is `can't tie a variable to itself`,
	// `typeset -T Z1` is `-T requires names of scalar and array`,
	// `typeset -T Z2 ':'` is `not valid in this context: :`, and a frozen
	// *already tied* scalar is `can't tie already tied scalar`.
	//
	// The **array** half is not asked, and that is measured too: `typeset -ar
	// arr=(1 2); typeset -T SS arr` is taken at 0 there, and the pair works
	// afterwards — `SS=q:r` leaves `arr` holding `q` and `r`, through the
	// freeze. Only the scalar refuses.
	//
	// One row of the four is deliberately left where it was: a refusal
	// *inside a subshell* ends that subshell at **0** in the reference where
	// every other readonly refusal there ends it at 1 — `readonly x=1;
	// (export x=2); print $?` is 1 and `(false; typeset -T S s); print $?` is
	// 0, measured the same day — and the script's own exit is 1 either way.
	// A fatality with a status of nobody's is not a rule this engine has, and
	// the row it replaces was a silent tie of a frozen name.
	if r.refuseReadonly(scalar, assignedByDeclaration) {
		return 1
	}
	// A tie **starts both names over**: every attribute either name carried
	// before is dropped and only export survives. Measured 2026-09-25 on zsh
	// 5.9.2 with `-f`, one letter to a run — `typeset -X S; typeset -T S s;
	// typeset -p S` lists a plain `typeset -T S s` for `X` in `u`, `l`, `U`,
	// `L`, `R`, `Z`, `i`, `E`, `F` and `t`, and the array half answers the
	// same way (`typeset -it s` then the tie lists `typeset -aT S s`), while
	// `typeset -x S` stays `export -T S s` and `export s` stays
	// `typeset -axT S s`.
	//
	// It is load-bearing for the letters written on the declaration's *own*
	// line, which is why it arrived with them rather than as a tidy-up:
	// `typeset -TU S=a:b:a s` is `a:b` because the `-U` came with the tie,
	// and `typeset -U S; typeset -T S=a:b:a s` is `a:b:a` because the tie
	// threw that `-U` away before the value reached it. Without this, the
	// second spelling would be deduplicated by an attribute this shell was
	// measured to have already dropped.
	//
	// Re-declaring the **same** pair keeps what it has — measured,
	// `typeset -TU S=a:b:a s; typeset -T S s` still lists `-UT` — so the
	// drop is for a name arriving at a tie it is not already half of.
	if _, already := r.tieOf(scalar); !already {
		r.dropNameAttributes(scalar)
		r.dropNameAttributes(array)
		// **The hide-in-scope letter goes with them**, and it is not in that
		// list: `dropNameAttributes` is the shadow's list, and a shadow
		// deliberately keeps the letter — see localattributes.go. A tie is
		// not a shadow. Measured 2026-09-27 on zsh 5.9.2 under `-f` from a
		// script file, `typeset -h TT; typeset -T TT tt` is `scalar-tied`
		// and `array-tied` there with no `hide` on either half, where this
		// shell kept the one the first line wrote (#4876).
		r.dropHideInScope(scalar)
		r.dropHideInScope(array)
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
	// And the hide-in-scope letter, which is a step of its own everywhere it
	// is applied — see Runner.setHideInScope for why it cannot ride in
	// applyAttributes. It reaches **both** halves, which is measured and is
	// not what the letter written on a line of its own does: 2026-09-27 on
	// zsh 5.9.2, `typeset -hT TT tt` is `scalar-tied-hide` and
	// `array-tied-hide`, where `typeset -T TT tt; typeset -h TT` leaves the
	// array half plain. So the two spellings are not one command and the
	// difference is the tie line carrying the letter to the pair (#4876).
	//
	// Behind the shadow above and ahead of the value below, which is where
	// every other caller puts it and for the same two reasons.
	r.setHideInScope(scalar, f)
	r.setHideInScope(array, f)
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
			// The **array** half's freeze is not deferred, because an array
			// literal written on that half is not this declaration's own
			// value — the value a tie carries is the scalar's. See
			// Runner.freezeWithoutDeferring for the three rows (#4874).
			r.freezeWithoutDeferring(array)
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
	for name, t := range r.tied {
		if t.wordless {
			// A pair the shell maintains between two of its own specials is
			// not in the tie table as far as any listing is concerned:
			// measured 2026-09-27, `typeset +T` in zsh 5.9.2 names the eight
			// tied pairs and `ZSH_EVAL_CONTEXT`, and neither `WATCH` nor
			// `watch`. See the `wordless` field (#4907).
			continue
		}
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

// TieProduced is [Runner.Tie] for a pair whose value is **produced** rather
// than stored: the names are tied and nothing is seeded.
//
// The seeding is the whole of the difference and it is not an optimization.
// Tie lays a value down — the scalar's, split into the array, or an empty
// pair where the scalar had nothing — and a stored value is exactly what
// shadows a producer: `r.setArray(array, nil)` left this shell answering
// `${(j:,:)zsh_eval_context}` with nothing while the scalar half, which was
// never written over, answered correctly. Two halves of one parameter
// disagreeing is the shape that is hard to see, because the half a probe
// reads first looks right.
//
// A dialect calls this before it registers the producers, and neither half
// can be assigned afterwards anyway: the one pair in the panel that needs it
// is readonly on both halves. See dialect/zsh/evalcontext.go.
// PairNames joins two of the shell's **own** specials the way a tie joins
// two names, and gives the join no name of its own.
//
// For a dialect whose shell maintains such a pair: see the `wordless` field
// for the measurement that says the two mechanisms are two. Nothing is
// seeded, because both halves of the one pair in the panel start empty and a
// stored value is what shadows anything a dialect registers over the name
// afterwards — the same reason [Runner.TieProduced] seeds nothing.
//
// `special` with it, which is measured rather than carried over from Tie: an
// `unset` of either half of this pair does **not** dissolve the pairing in
// the reference — `unset watch; watch=(q r)` writes `$WATCH` again, exactly
// as `unset path; path=(/q)` writes `$PATH`. What it does do is leave the
// *other* half standing where a tie takes both names away, and that half is
// not modeled here; see dialect/zsh/watchpair.go, where the row is written
// down.
func (r *Runner) PairNames(scalar, array, sep string) {
	if sep == "" {
		sep = defaultTieSeparator
	}
	r.tieNames(tie{scalar: scalar, array: array, sep: sep, special: true, wordless: true})
	// Both halves laid down empty, which is the pair's own measurement and
	// not Tie's seeding copied over: `${+WATCH}` and `${+watch}` are both 1
	// in a fresh zsh 5.9.2 and `$WATCH` is empty with `${#watch}` at 0 — so
	// the parameter is there and holds nothing, which a registration alone
	// would not have said. The mirror is held off for the reason
	// declareTie holds it off: an empty scalar splits into one field and a
	// declaration splits nothing.
	r.mirroring = true
	r.setVar(scalar, "")
	r.setArray(array, nil)
	r.mirroring = false
}

func (r *Runner) TieProduced(scalar, array, sep string) {
	if sep == "" {
		sep = defaultTieSeparator
	}
	r.tieNames(tie{scalar: scalar, array: array, sep: sep, special: true})
}

// refuseUntie is `typeset +T`, which is not the tie's undoing but a refusal.
//
// The plus form of every other declaration letter takes the attribute off, so
// this reads as "untie" and is not: the shell that has the letter refuses the
// spelling outright and says where the answer is. Measured 2026-09-26 on zsh
// 5.9.2 under `-f` from a script file:
//
//	typeset -T SCALAR arr; typeset +T SCALAR   use unset to remove tied
//	                                           variables, and the script is over
//	typeset +T v, `v` not tied at all          the same sentence, the same end
//	typeset +T, with no operands               the *listing* of tied names, at 0
//
// The second row is the sharper one: the refusal is about the **option
// letter** and not about the parameter's state, so there is nothing to look
// up before saying it. The third is what keeps this off the bare form, which
// is a listing in both shells and is reached before this.
//
// It was accepted here and did nothing, at status 0 — so a script that
// believed it had untied a pair carried on with the pair still tied (#4598).
func (r *Runner) refuseUntie(builtin string) int {
	r.diagf("%s\n", Wording(r.diag().UntieRefused,
		"use unset to remove tied variables"))
	fatal, _ := r.nameRules(builtin)
	if r.ask(fatal, "a refused declaration operand ending the script") {
		r.status = 1
		r.fatalQuiet()
	}
	return 1
}

// pairHalfWasRemoved reports whether the half a mirror is about to write has
// been taken away by an `unset`, which is a state only a **wordless** pair can
// be in — see the unset branch in interp/builtin.go.
//
// The mirror writes into a half that still exists and does nothing where the
// half is gone, which is the one sentence the four cells of the measurement
// come to. On zsh 5.9.2, from a pair holding `(a b)`, one shell per row:
//
//	unset watch;  WATCH=x:y      ${+watch} 0 — the array does not come back
//	unset watch;  watch=(q r)    ${+watch} 1, $WATCH `q:r` — writing the half
//	                             that was removed re-creates it, and the
//	                             surviving half takes the mirror
//	unset WATCH;  WATCH=x:y      ${+WATCH} 1, $watch `x y` — the same, the
//	                             other way round
//	unset WATCH;  watch=(q r)    ${+WATCH} 0 — the scalar does not come back
//
// The grid varies **which half was removed** and **which half is written**,
// which is the pair of nouns the rule is keyed on: a reading that mirrored
// unconditionally gets rows one and four wrong, and one that stopped
// mirroring altogether gets rows two and three wrong.
// **Only the wordless kind**, and that is the control rather than a
// narrowing: both halves of a *worded* tie go away together, and the pairing
// is kept so that a write to either re-makes it — measured, `unset path;
// PATH=/y` splits into `path` again in the reference and in this shell, and
// a gate that asked "is the other half removed" without asking which kind of
// tie this is left `${+path}` at 0.
func (r *Runner) pairHalfWasRemoved(t tie, name string) bool {
	return t.wordless && r.removed[name] && !r.nameIsSet(name)
}

// pairHalfKeepsItsMark reports whether this name is a half of a wordless pair
// that the shell still owns, so that its removal must not take the mark with
// it.
//
// Only the wordless kind, and only while the name is actually owned: a tie a
// script made with `typeset -T` is not the shell's own in either half, and
// both halves of one go away together anyway.
func (r *Runner) pairHalfKeepsItsMark(name string) bool {
	t, tied := r.tieOf(name)
	return tied && t.wordless && r.shellOwn[name]
}
