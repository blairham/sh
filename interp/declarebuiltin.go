// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// `declare` and `typeset`, which give a name an attribute as well as a value.
//
// Which of the two a shell has is a dialect's answer and not an axis: dash
// has neither, ksh93 has `typeset` alone, and bash and zsh have both under one
// implementation. So they are registered through the extension seam, the same
// way ksh93 registers `source` and takes `local` away.
//
// The attributes are the interesting half. `-r` and `-x` are `readonly` and
// `export` spelled differently and share their machinery, but `-i` is a
// property of the *name* that changes what a later assignment means:
//
//	declare -i n; n=5+2   → 7, because the value is evaluated
//	n=5+2                 → the four characters, because it is not
//
// That is why an attribute has to be recorded against the name rather than
// applied once at the point of declaring it.

// declareFlags is what a declaration asks for.
type declareFlags struct {
	integer   bool
	readonly  bool
	export    bool
	assoc     bool
	lower     bool
	upper     bool
	global    bool
	function  bool
	funcNames bool
	remove    bool
	print     bool
}

// declareOptionLetters is the set `declare` and `typeset` read where the
// dialect has not answered — the letters the substrate implemented before
// they were a question.
const declareOptionLetters = "aAiprx"

// parseDeclareFlags reads the leading option words of a declaration builtin,
// against the letters the dialect gives it. The parse is the one these
// builtins have always had rather than the shared reader's, because `+i`
// removes what `-i` adds and no other builtin spells an option with a plus.
//
// A letter outside the set goes through the shared refusal, so a letter the
// dialect has and this shell does not is named as missing rather than as
// unknown, in the dialect's words.
func (r *Runner) parseDeclareFlags(name string, args []string, known string) (rest []string, f declareFlags, code int) {
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			break
		}
		// `+i` removes the attribute where `-i` adds it, which is the one
		// place a shell spells an option with a plus.
		f.remove = a[0] == '+'
		for _, c := range a[1:] {
			if !strings.ContainsRune(known, c) {
				return nil, f, r.refuseOption(name, a, known)
			}
			switch c {
			case 'i':
				f.integer = true
			case 'r':
				f.readonly = true
			case 'x':
				f.export = true
			case 'A':
				// The associative attribute, and unlike `-a` it must be
				// recorded: it changes what a later subscript *means*, the
				// way `-i` changes what a later assignment means.
				f.assoc = true
			case 'l':
				f.lower = true
			case 'u':
				f.upper = true
			case 'g':
				// Global rather than local: the assignment reaches the
				// global cell however deep the function stack is.
				f.global = true
			case 'f':
				f.function = true
			case 'F':
				f.funcNames = true
			case 'p':
				// Print rather than declare. `+p` prints too — measured in
				// both shells that spell the option at all.
				f.print = true
			case 'a':
				// Accepted and recorded nowhere. An array here is dynamic,
				// so `typeset -a arr` followed by `arr[0]=x` works without
				// the attribute existing — which is what the flag is used
				// for. The compound form `typeset -a arr=(x y)` is a
				// different thing and is not built: the parser reads the
				// parenthesis as an ordinary word, and making it an array
				// literal in argument position is grammar rather than a
				// flag. Refusing the flag outright would break the common
				// use to be honest about the rare one.
			}
		}
	}
	return args[i:], f, 0
}

func biDeclare(r *Runner, _ context.Context, args []string) int {
	name := r.inBuiltin
	if name == "" {
		name = "declare"
	}
	known := r.sem().DeclareOptions
	if known == "" {
		known = declareOptionLetters
	}
	args, f, code := r.parseDeclareFlags(name, args, known)
	if code != 0 {
		// A bad option ends the script where the dialect counts `typeset`
		// among its special builtins — ksh93, where any of the builtin's
		// failures is fatal, so the honest refusal of a letter it has and
		// this shell does not stops the script the same way.
		if r.ask(r.sem().TypesetBadOptionFatal, "a bad `typeset` option ending the script") {
			r.status = code
			r.fatalQuiet()
		}
		return code
	}

	if f.function || f.funcNames {
		// The function table rather than the variables: `-f` writes the
		// functions themselves and `-F` only names them. `-p` alongside
		// changes nothing — the flags already mean print.
		return r.declareFunctions(args, f.funcNames)
	}

	if f.print {
		// The other letters are accepted alongside `-p` and decide nothing:
		// the shells that have them use an attribute letter to *filter* the
		// full listing, which is not built. Refusing the combination would
		// break the plain use to be honest about the rare one.
		return r.declarePrint(args)
	}

	for _, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		// Read before the attributes are applied, because `-x` on this very
		// declaration would otherwise answer a question asked about the name
		// it shadows. See shadowedExport.
		wasExported := r.isExported(name)
		// Attributes first, because `-i` changes what the assignment on the
		// same line *means* — but readonly last, because it changes whether
		// that assignment is allowed at all. `declare -r c=1` sets c and then
		// freezes it; applying both up front made the declaration refuse its
		// own value and leave the name empty.
		r.applyAttributes(name, f)
		if f.global {
			// `-g` reaches past every local: the assignment lands on the
			// global cell and no shadow is taken, so `declare -g x=new`
			// inside a function survives its return even where a `local x`
			// is standing in front of the name.
			if hasValue {
				r.setGlobalVar(name, value)
				if r.unspecified || r.ctl == controlExit {
					return r.status
				}
			}
			if f.readonly && !f.remove {
				r.markReadonly(name)
			}
			continue
		}
		// Declaring inside a function declares a local, which is unanimous
		// among the three shells that have the name — subject to ksh93's
		// rule about which functions have a scope at all.
		r.shadowTypeset(name)
		r.localExportAttribute(name, f.export)
		if r.unspecified {
			// See biLocal: an unanswered axis refuses the declaration
			// rather than making it one way and saying so.
			return r.status
		}
		r.shadowedExport(name, wasExported)
		if f.assoc && !f.remove {
			// After the shadow, so that `typeset -A` inside a function
			// declares a local table and the caller's absence comes back
			// when it returns. `+A` does nothing rather than removing: two
			// of the three shells with the attribute refuse to take it off
			// a name, the same shape `+r` already has.
			r.markAssoc(name)
		}
		switch {
		case hasValue:
			r.setVarAs(name, value, assignedByDeclaration)
			if r.ctl == controlExit {
				// See biExport: the failure's status is the one that stands.
				return r.status
			}
		default:
			r.declareEmpty(name)
		}
		if f.readonly && !f.remove {
			r.markReadonly(name)
		}
	}
	if r.assignFailed {
		// See biExport.
		return 1
	}
	return 0
}

// applyAttributes records what a name has been declared to be.
func (r *Runner) applyAttributes(name string, f declareFlags) {
	if f.integer {
		if r.integer == nil {
			r.integer = map[string]bool{}
		}
		if f.remove {
			delete(r.integer, name)
		} else {
			r.integer[name] = true
		}
	}
	if f.export {
		if r.exported == nil {
			r.exported = map[string]bool{}
		}
		r.exported[name] = !f.remove
	}
	// The case attributes fold at assignment here, which is what bash and
	// ksh93 do. zsh stores the raw text and folds on *expansion* — every
	// read agrees with the other two, and only its `typeset -p` betrays the
	// difference by listing the raw value. That listing nuance is
	// deliberately not modeled; the fold every script observes is.
	if f.lower {
		if r.lowered == nil {
			r.lowered = map[string]bool{}
		}
		if f.remove {
			delete(r.lowered, name)
		} else {
			r.lowered[name] = true
			// The two case attributes cannot both stand: the later one
			// speaks, which is what both shells measured do.
			delete(r.uppered, name)
		}
	}
	if f.upper {
		if r.uppered == nil {
			r.uppered = map[string]bool{}
		}
		if f.remove {
			delete(r.uppered, name)
		} else {
			r.uppered[name] = true
			delete(r.lowered, name)
		}
	}
}

// setGlobalVar assigns to a name's global cell, past any local shadowing it.
//
// In this engine there is one table and a stack of saved outer values, so
// the global cell is either the table itself — no scope saved the name — or
// the copy held by the *oldest* scope that did, which is the value the last
// return will put back. Whether `-g` really reaches past a local is the one
// disagreement here, and it is asked only where a local stands in the way —
// with none, both shells that spell the letter write the global.
func (r *Runner) setGlobalVar(name, value string) {
	for _, sc := range r.scopes {
		if _, saved := sc.saved[name]; !saved {
			continue
		}
		if !r.ask(r.sem().DeclareGlobalReachesPastALocal, "`declare -g` writing past a local of the same name") {
			if r.unspecified {
				return
			}
			// The visible cell — the local — which is what a plain
			// assignment would have written.
			break
		}
		sc.saved[name] = value
		sc.existed[name] = true
		if sc.removedBefore != nil {
			sc.removedBefore[name] = false
		}
		return
	}
	r.setVarAs(name, value, assignedByDeclaration)
}

// declareFunctions is `declare -f` and `-F`: the functions themselves, or
// only their names.
//
// One shell has each of these under `declare` and prints `-F` in its own two
// shapes — `declare -f name` per function when nothing narrows it, the bare
// name when an operand asked — so the shapes are written here the way `-t`'s
// kind words are: there is no second engine to hold a wording for.
func (r *Runner) declareFunctions(names []string, namesOnly bool) int {
	named := len(names) > 0
	if !named {
		names = make([]string, 0, len(r.funcs))
		for name := range r.funcs {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	status := 0
	for _, name := range names {
		fn, ok := r.funcs[name]
		if !ok {
			// Silent, and 1 stands however many other names printed —
			// measured in both shells that can be asked.
			status = 1
			continue
		}
		switch {
		case !namesOnly:
			r.printf("%s\n", r.listedFunction(name, fn))
		case named:
			r.printf("%s\n", name)
		default:
			r.printf("declare -f %s\n", name)
		}
	}
	return status
}

// listedFunction is a function said back whole, in the dialect's arrangement:
// the header the dialect writes — see Diagnostics.FunctionListingHeader —
// and the body laid out by its function layout.
func (r *Runner) listedFunction(name string, fn *syntax.FuncDecl) string {
	return Wording(r.diag().FunctionListingHeader, "%[1]s () \n%[2]s",
		name, syntax.PrintWith(fn.Body, r.functionLayout))
}

// markReadonly freezes a name.
//
// Never undone: a readonly name cannot be made writable again in any shell in
// the panel, so `+r` does nothing rather than reversing it — which is what
// they do.
func (r *Runner) markReadonly(name string) {
	if r.readonly == nil {
		r.readonly = map[string]bool{}
	}
	r.readonly[name] = true
}

// integerValue evaluates an assignment to a name declared integer.
//
// An empty assignment is zero rather than nothing, and a name that is not set
// is zero rather than an error — `n=abc` leaves 0 and says nothing, because
// `abc` is a perfectly good expression whose value happens to be unset. Text
// that will not parse *is* an error, and a fatal one: bash and zsh both stop
// the script rather than store something.
func (r *Runner) integerValue(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "0", true
	}
	p := syntax.NewParser("", r.dialect())
	e := p.ParseArithFor(text, syntax.Pos{})
	if err := p.Err(); err != nil {
		r.fatal("%s\n", r.diag().ParseFailure(err))
		return "", false
	}
	v, err := r.evalArith(e)
	if err != nil {
		r.fatal("%v\n", err)
		return "", false
	}
	return itoa(v), true
}

// declareEmpty is what declaring a name without a value does.
//
// The name is now local, or attributed, or both — but whether it also *exists*
// is a dialect's answer, so this is the one place that decides it and both
// `local` and `typeset` come through here.
func (r *Runner) declareEmpty(name string) {
	if r.ask(r.sem().DeclaredNameWithoutValueIsEmpty, "a declaration without a value setting the name") {
		r.setVar(name, "")
		// Set by a declaration and not by an assignment, which the shell's
		// own reads cannot tell apart and a child can: see
		// Runner.declaredEmpty.
		if r.declaredEmpty == nil {
			r.declaredEmpty = map[string]bool{}
		}
		r.declaredEmpty[name] = true
		return
	}
	if r.unspecified {
		return
	}
	// The name exists unset here — but whether the *outer* value still shows
	// through is a second disagreement, and it only arises where a shadow
	// was actually taken: `declare u` at the top level leaves the global
	// alone in every shell measured.
	if len(r.scopes) == 0 {
		return
	}
	if _, shadowed := r.scopes[len(r.scopes)-1].saved[name]; !shadowed {
		return
	}
	if r.ask(r.sem().ValuelessDeclarationHidesTheOuterValue, "a declaration without a value hiding the outer value") {
		r.hideVar(name)
	}
}

// localExportAttribute answers whether the local a declaration just took
// inherits the export attribute of the name it shadows.
//
// The question is only there when the shadowed name is exported and a scope
// was actually taken; explicit says the declaration named the attribute
// itself, which answers it outright. Taking the attribute off is a change to
// a record the scope has to put back, so it is saved the way the value is.
func (r *Runner) localExportAttribute(name string, explicit bool) {
	if explicit {
		return
	}
	if len(r.scopes) == 0 {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	if _, shadowed := sc.saved[name]; !shadowed {
		// No scope was taken — a declaration at the top level, or one this
		// dialect gives no scope to — so there is no local to export.
		return
	}
	if !r.isExported(name) {
		// Nothing to inherit, and so nothing to disagree about.
		return
	}
	if r.ask(r.sem().LocalInheritsTheExportAttribute,
		"a local declaration inheriting the export attribute of the name it shadows") {
		return
	}
	if r.unspecified {
		return
	}
	if sc.exportedSpoken == nil {
		sc.exportedSpoken = map[string]bool{}
		sc.savedExported = map[string]bool{}
	}
	if _, seen := sc.exportedSpoken[name]; !seen {
		on, spoken := r.exported[name]
		sc.exportedSpoken[name] = spoken
		sc.savedExported[name] = on
	}
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	r.exported[name] = false
}

// shadowedExport remembers what an exported name held when a declaration took
// a scope in front of it.
//
// The shadowed binding does not stop being exported for having something
// standing in front of it, so a command is told its value for as long as the
// declaration has none of its own: the shell reads `${FOO-UNSET}` as unset
// inside the function and a child is still handed `FOO=bar`. Measured on both
// halves — an exported name and one that arrived in the environment — and on
// two levels, where what a child is told is the *caller's* local rather than
// the global behind it.
//
// Told what the name was rather than asking, because the attributes on this
// very declaration have been applied by the time a scope exists to record
// into: `FOO=bar; f() { local -x FOO; }` would otherwise read its own `-x`
// as the shadowed name's and hand a child a value no shell hands it. Which is
// the difference the measurement turns on — an exported name shadowed by a
// valueless declaration reaches a child, and an unexported one does not,
// whatever the declaration says about the local's own attribute. `+x` is the
// proof that it is the shadowed binding speaking and not the local: it takes
// the attribute off the local outright, and the child is still told the outer
// value.
//
// Reached only where a local carries the attribute of the name it shadows at
// all. The shell that answers LocalInheritsTheExportAttribute no tells a
// child nothing under that name, and it tells it nothing here either — a
// valueless declaration of an exported name is that same question and not a
// second one. The axis is read rather than asked because the declaration
// above has just asked it, and asking twice would report an unanswered one
// twice.
//
// Not written once per scope: a second declaration of the same name hides
// whatever the first one left, and the value that goes to a child is the one
// that was just taken away rather than the one the scope will put back.
func (r *Runner) shadowedExport(name string, exported bool) {
	if !exported || len(r.scopes) == 0 {
		return
	}
	if r.sem().LocalInheritsTheExportAttribute != Yes {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	if _, shadowed := sc.saved[name]; !shadowed {
		// No scope was taken — a declaration at the top level, or one this
		// dialect gives no scope to — so nothing is standing in front of
		// anything.
		return
	}
	value, ok := r.Vars[name]
	if !ok {
		if value, ok = r.inheritedValue(name); !ok {
			// Exported with no value anywhere — `export FOO` and nothing
			// more — and an exported name with no value reaches no child in
			// any shell measured.
			return
		}
	}
	if sc.exportedShadow == nil {
		sc.exportedShadow = map[string]string{}
	}
	sc.exportedShadow[name] = value
}

// hideVar takes a name out of view entirely — the tables and the environment
// fallback alike — so `${name-UNSET}` fires its default. The caller's shadow
// is what brings the outer value back.
func (r *Runner) hideVar(name string) {
	delete(r.Vars, name)
	delete(r.Arrays, name)
	if r.removed == nil {
		r.removed = map[string]bool{}
	}
	r.removed[name] = true
}

// shadowTypeset is shadow for `typeset`, which unlike `local` does not always
// get a scope to declare into.
//
// ksh93 is the reason: only a function defined with the `function` word has
// one, and in a POSIX-style function the assignment is ordinary and reaches
// the caller. The axis is asked only when the two answers differ — inside a
// keyword-defined function they do not, so that case needs no dialect.
func (r *Runner) shadowTypeset(name string) {
	if len(r.scopes) == 0 {
		return
	}
	if !r.scopes[len(r.scopes)-1].keyword &&
		r.ask(r.sem().TypesetLocalNeedsKeywordFunction, "`typeset` needing a keyword-defined function to declare a local") {
		return
	}
	r.shadow(name)
}

// shadow saves a name in the innermost scope so the function's exit puts it
// back, which is what makes a declaration local.
func (r *Runner) shadow(name string) {
	if len(r.scopes) == 0 {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	if _, seen := sc.saved[name]; !seen {
		old, existed := r.Vars[name]
		sc.saved[name] = old
		sc.existed[name] = existed
		if sc.removedBefore == nil {
			sc.removedBefore = map[string]bool{}
			sc.declaredEmptyBefore = map[string]bool{}
		}
		sc.removedBefore[name] = r.removed[name]
		sc.declaredEmptyBefore[name] = r.declaredEmpty[name]
	}
	// Arrays live in a table of their own, so a name has to be saved from
	// both. Saving only the scalar left `f() { local a; a=(x y); }` writing a
	// global array: `local` shadowed nothing an array assignment then wrote
	// to, and the value outlived the function.
	if _, seen := sc.arrayExisted[name]; !seen {
		old, existed := r.Arrays[name]
		if sc.savedArrays == nil {
			sc.savedArrays = map[string]Array{}
			sc.arrayExisted = map[string]bool{}
		}
		// Copied rather than kept: an Array is a map, so saving the value
		// would save a reference to the very table the function is about to
		// write into, and putting it back would put back the changes.
		if existed {
			kept := make(Array, len(old))
			for k, v := range old {
				kept[k] = v
			}
			old = kept
		}
		sc.savedArrays[name] = old
		sc.arrayExisted[name] = existed
	}
	// And a third table for the associative kind, for the same reason as the
	// second — and here the *attribute* is what is being shadowed as much as
	// the value: `typeset -A m` in a function must not leave the caller's
	// `m` reading its subscripts as strings.
	if _, seen := sc.assocExisted[name]; !seen {
		old, existed := r.AssocArrays[name]
		if sc.savedAssoc == nil {
			sc.savedAssoc = map[string]AssocArray{}
			sc.assocExisted = map[string]bool{}
		}
		if existed {
			kept := make(AssocArray, len(old))
			for k, v := range old {
				kept[k] = v
			}
			old = kept
		}
		sc.savedAssoc[name] = old
		sc.assocExisted[name] = existed
	}
}

// declarationUtilities are the commands whose `name=value` arguments are
// assignments rather than ordinary words.
//
// The list is the core's: `export` and `readonly` are POSIX, and `local` and
// `typeset` are here because every shell in the panel that has them treats
// them the same way. A dialect adds its own names with SetDeclaring — bash and
// zsh add `declare` — because the rule has to follow the name into the shell
// that has it and must not apply in the shell that does not, where the same
// word is an ordinary command.
var declarationUtilities = map[string]bool{
	"export": true, "readonly": true, "local": true, "typeset": true,
}

// SetDeclaring makes a command's `name=value` arguments expand as assignments.
func (r *Runner) SetDeclaring(name string) {
	if r.declaring == nil {
		r.declaring = map[string]bool{}
	}
	r.declaring[name] = true
}

func (r *Runner) declares(name string) bool {
	return declarationUtilities[name] || r.declaring[name]
}

// assignShaped reports whether a word begins with a literal `name=`.
//
// The name has to be literal: `$x=1` is not an assignment in any shell, and
// neither is `"a"=1`. Only what the word says before any expansion counts.
func assignShaped(w *syntax.Word) bool {
	if w == nil || len(w.Spans) == 0 {
		return false
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
		return false
	}
	name, _, ok := strings.Cut(s.Value, "=")
	return ok && name != "" && isNameLike(name)
}

// expandAssignArg expands `name=value` given to a declaration utility, leaving
// the name alone and expanding the value as an assignment's.
func (r *Runner) expandAssignArg(w *syntax.Word) string {
	head := w.Spans[0]
	name, first, _ := strings.Cut(head.Value, "=")
	rest := *w
	rest.Spans = append([]syntax.Span{{
		Kind: syntax.Literal, Value: first, Quoting: head.Quoting, Pos: head.Pos,
	}}, w.Spans[1:]...)
	return name + "=" + r.expandAssignValue(&rest)
}
