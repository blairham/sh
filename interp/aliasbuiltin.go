// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// `alias` and `unalias` keep the tables a shell substitutes from.
//
// This is the tables and the two builtins. *Expansion* — replacing the word
// when a line is parsed — is the other half and is deliberately separate: it
// happens in the parser rather than here, needs an input stack to splice text
// in, and is the one component on the keystroke path and under the fuzzer.
//
// Nothing here expands anything, and no test claims it does.
//
// There are three kinds of alias and two namespaces, which is one dialect's
// shape and is measured rather than assumed:
//
//   - a *regular* alias is expanded where a command word stands;
//   - a *global* alias is expanded wherever a word stands, and shares the
//     table with the regular ones — `alias -g dup=…` replaces a regular
//     `dup`, and one listing shows both;
//   - a *suffix* alias is keyed on a command word's extension rather than on
//     the whole word, and lives in a table of its own. `unalias -a` empties
//     the first table and leaves this one, which is what says they are two.

// aliasDef is one entry of the shared table: what the name stands for, and
// whether it is global.
//
// A struct rather than a second map keyed by name, because two maps are two
// places for a removal to reach one of — and `alias -g` over a regular name
// is a *replacement* in the shell this models, which two maps would have to
// keep in step by hand.
type aliasDef struct {
	value  string
	global bool
}

// aliasKind is which aliases a definition, a lookup or a listing is about.
type aliasKind uint8

const (
	// aliasEitherKind is the plain builtin: it defines a regular alias, and
	// lists the regular and the global ones together.
	aliasEitherKind aliasKind = iota
	// aliasGlobalKind is `-g`.
	aliasGlobalKind
	// aliasSuffixKind is `-s`, the second namespace.
	aliasSuffixKind
)

func init() {
	builtins["alias"] = biAlias
	builtins["unalias"] = biUnalias
}

// LookupAlias answers what a name stands for, for a parser that expands
// aliases.
//
// Exported because expansion happens when a line is *parsed* and the table
// lives here: the front end owns the parser, this owns the table, and this
// method is the whole of the seam between them. It matches syntax.Aliases.
//
// A shell that should not expand simply does not pass it, which is how the
// panel's 2v2 split is expressed — see syntax.Dialect.ExpandAliases.
//
// Global aliases are in here too, because a global alias in command position
// expands there as well: `alias -g f='echo f'` run as a command prints `f`,
// measured.
func (r *Runner) LookupAlias(name string) (string, bool) {
	a, ok := r.aliases[name]
	return a.value, ok
}

// LookupGlobalAlias answers only for the global kind, for the parser's other
// hook — the one it asks about *every* word rather than only a command word.
func (r *Runner) LookupGlobalAlias(name string) (string, bool) {
	a, ok := r.aliases[name]
	if !ok || !a.global {
		return "", false
	}
	return a.value, true
}

// LookupSuffixAlias answers for the second namespace, keyed on the extension
// of a command word rather than on the whole word.
func (r *Runner) LookupSuffixAlias(suffix string) (string, bool) {
	v, ok := r.suffixAliases[suffix]
	return v, ok
}

func biAlias(r *Runner, _ context.Context, args []string) int {
	print := false
	kind := aliasEitherKind
	// Two questions, because dash answers the first differently from the
	// other three: it reads no options for `alias` at all, so `alias -p` is a
	// *name* there and the answer is "-p not found". zsh does read options
	// and simply has no `-p`, which is a refusal rather than a lookup.
	if r.ask(r.sem().AliasParsesOptions, "`alias` reading options at all") {
		known := r.aliasOptionLetters()
		if r.unspecified {
			return 2
		}
		rest, opts, code := r.builtinOptions("alias", args, known)
		if code != 0 {
			return code
		}
		if strings.ContainsRune(opts, 'g') && strings.ContainsRune(opts, 's') {
			// The two kinds are two namespaces, so one word cannot ask for
			// both: measured `illegal combination of options`, status 1,
			// whether the call defines or lists.
			r.diagf("%s\n", Wording(r.diag().AliasIllegalOptionCombination,
				"alias: illegal combination of options"))
			return 1
		}
		switch {
		case strings.ContainsRune(opts, 'g'):
			kind = aliasGlobalKind
		case strings.ContainsRune(opts, 's'):
			kind = aliasSuffixKind
		}
		args, print = rest, strings.ContainsRune(opts, 'p')
	} else if r.unspecified {
		return 2
	}
	if len(args) == 0 {
		for _, name := range r.aliasNames(kind) {
			r.printf("%s\n", r.aliasLine(name, kind, print))
		}
		return 0
	}
	missing := 0
	for _, a := range args {
		name, value, isDefinition := strings.Cut(a, "=")
		if isDefinition {
			r.defineAlias(name, value, kind)
			continue
		}
		found, listed := r.lookupForListing(name, kind)
		if !found {
			r.aliasNotFound("alias", name)
			missing++
			continue
		}
		// Found, and printed only where it is of the kind asked for: `alias
		// -g` naming a *regular* alias reports success and says nothing,
		// measured. The tables are what "found" is about and the letter is a
		// filter on the listing.
		if listed {
			r.printf("%s\n", r.aliasLine(name, kind, print))
		}
	}
	if missing == 0 {
		return 0
	}
	// ksh93 answers with how many it could not find — `alias n1 n2 n3` is 3
	// there — where the other three answer 1 however many were missing. Its
	// own `unalias` does not count, which is why this is asked here rather
	// than in aliasNotFound.
	if r.ask(r.sem().AliasNotFoundStatusCounts, "`alias` counting the names it could not find") {
		return missing
	}
	return 1
}

// aliasOptionLetters is the set `alias` takes in this dialect, which is the
// paired half of Diagnostics.UnimplementedOptionLetters: a letter is either
// here or there, and one that is in neither reads as "no shell has this"
// rather than "this shell has not got it yet" (#2081).
func (r *Runner) aliasOptionLetters() string {
	known := ""
	if r.ask(r.sem().AliasHasPrintOption, "`alias -p`") {
		known += "p"
	}
	if r.ask(r.sem().GlobalAliases, "global aliases, `alias -g`") {
		known += "g"
	}
	if r.ask(r.sem().SuffixAliases, "suffix aliases, `alias -s`") {
		known += "s"
	}
	return known
}

// defineAlias writes one entry of whichever table the kind names.
func (r *Runner) defineAlias(name, value string, kind aliasKind) {
	if kind == aliasSuffixKind {
		if r.suffixAliases == nil {
			r.suffixAliases = map[string]string{}
		}
		r.suffixAliases[name] = value
		return
	}
	if r.aliases == nil {
		r.aliases = map[string]aliasDef{}
	}
	r.aliases[name] = aliasDef{value: value, global: kind == aliasGlobalKind}
}

// lookupForListing answers the two questions a named operand asks: whether
// the tables hold it at all, which is what the status is about, and whether
// it is of the kind the letter asked for, which is what the listing is about.
func (r *Runner) lookupForListing(name string, kind aliasKind) (found, listed bool) {
	if kind == aliasSuffixKind {
		_, ok := r.suffixAliases[name]
		return ok, ok
	}
	a, ok := r.aliases[name]
	if !ok {
		return false, false
	}
	return true, kind == aliasEitherKind || a.global
}

// aliasNames is what a listing walks, in order.
func (r *Runner) aliasNames(kind aliasKind) []string {
	var names []string
	if kind == aliasSuffixKind {
		names = make([]string, 0, len(r.suffixAliases))
		for name := range r.suffixAliases {
			names = append(names, name)
		}
	} else {
		names = make([]string, 0, len(r.aliases))
		for name, a := range r.aliases {
			if kind == aliasGlobalKind && !a.global {
				continue
			}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func biUnalias(r *Runner, _ context.Context, args []string) int {
	known := "a"
	// The same letter `alias` takes, for the same reason: a suffix alias is
	// not reachable from the plain form. `unalias txt` with a suffix alias
	// called `txt` is "no such hash table element" and leaves it defined,
	// measured — so without the letter there is no way to remove one.
	//
	// There is no `-g` here and that is not an oversight: `unalias -g` is a
	// bad option in the shell that has global aliases, because they share
	// the table the plain form already empties.
	if r.ask(r.sem().SuffixAliases, "suffix aliases, `unalias -s`") {
		known += "s"
	}
	if r.unspecified {
		return 2
	}
	args, opts, code := r.builtinOptions("unalias", args, known)
	if code != 0 {
		return code
	}
	kind := aliasEitherKind
	if strings.ContainsRune(opts, 's') {
		kind = aliasSuffixKind
	}
	if strings.ContainsRune(opts, 'a') {
		// zsh takes `-a` to mean the whole table and nothing else, so a name
		// beside it is too many arguments — and it clears nothing. The other
		// three ignore the names and empty the table.
		if len(args) > 0 && r.ask(r.sem().UnaliasAllRefusesOperands, "`unalias -a` with a name beside it") {
			r.diagf("%s\n", Wording(r.diag().UnaliasAllWithOperands,
				"unalias: -a: too many arguments"))
			return 1
		}
		// One namespace each: `unalias -a` leaves the suffix aliases
		// standing and `unalias -s -a` leaves the others, measured.
		if kind == aliasSuffixKind {
			r.suffixAliases = nil
		} else {
			r.aliases = nil
		}
		return 0
	}
	if len(args) == 0 {
		return r.unaliasWithNothingToRemove()
	}
	status := 0
	for _, name := range args {
		if !r.removeAlias(name, kind) {
			status = r.aliasNotFound("unalias", name)
		}
	}
	return status
}

// removeAlias takes one name out of whichever table the kind names, and
// reports whether it was there.
func (r *Runner) removeAlias(name string, kind aliasKind) bool {
	if kind == aliasSuffixKind {
		if _, ok := r.suffixAliases[name]; !ok {
			return false
		}
		delete(r.suffixAliases, name)
		return true
	}
	if _, ok := r.aliases[name]; !ok {
		return false
	}
	delete(r.aliases, name)
	return true
}

// aliasNotFound reports a name the table does not hold, for whichever of the
// two builtins asked.
//
// Two wordings and two answers about whether to speak at all, because the
// panel does not pair them: ksh93 complains about `alias nope` and says
// nothing about `unalias nope`, and zsh does exactly the reverse.
func (r *Runner) aliasNotFound(builtin, name string) int {
	d := r.diag()
	speaks, wording, unprefixed := r.sem().AliasReportsNotFound, d.AliasNotFound, d.AliasNotFoundUnprefixed
	if builtin == "unalias" {
		speaks, wording, unprefixed = r.sem().UnaliasReportsNotFound, d.UnaliasNotFound, d.UnaliasNotFoundUnprefixed
	}
	if r.ask(speaks, "`"+builtin+"` naming an alias it does not have") {
		line := Wording(wording, "%[1]s: %[2]s: not found", builtin, name)
		// Two of the four write this without the shell and the line in
		// front of it, which no other diagnostic of theirs does.
		if unprefixed {
			r.errf("%s\n", line)
		} else {
			r.diagf("%s\n", line)
		}
	}
	return 1
}

// unaliasWithNothingToRemove is `unalias` with no operand and no -a.
//
// Four answers: bash and ksh93 print a usage line, zsh says it is short of
// arguments, and dash takes it as a no-op and reports success.
func (r *Runner) unaliasWithNothingToRemove() int {
	d := r.diag()
	if d.UnaliasUsage == "" {
		return orDefault(d.UnaliasNoOperandStatus, 0)
	}
	if d.UnaliasUsageUnprefixed {
		r.errf("%s\n", d.UnaliasUsage)
	} else {
		r.diagf("%s\n", d.UnaliasUsage)
	}
	status := orDefault(d.UnaliasNoOperandStatus, 2)
	if r.ask(r.sem().BadOptionToSpecialBuiltinFatal, "a special builtin's usage error ending the script") {
		r.status = status
		r.fatalQuiet()
	}
	return status
}

// aliasLine spells one entry the way `alias` lists it.
func (r *Runner) aliasLine(name string, kind aliasKind, forcePrefix bool) string {
	prefix := r.diag().AliasListPrefix
	if forcePrefix {
		// `-p` prints the prefix even in the dialect whose plain listing has
		// none, which is what makes the option worth having there.
		prefix = "alias "
	}
	value := ""
	if kind == aliasSuffixKind {
		value = r.suffixAliases[name]
	} else {
		value = r.aliases[name].value
	}
	return prefix + name + "=" + r.quoteListedValue(r.sem().AliasQuoting, "`alias`", value)
}

// ExpandingAlias is the parser's hook: what a name stands for when this shell
// is expanding aliases at all, and nothing when it is not.
//
// [Runner.LookupAlias] is the table and this is the table plus the switch, and
// they are two methods because two callers want different things. A builtin
// that speaks for aliases — `alias`, `type`, `command -v` — asks about the
// table, and gets the same answer whether or not a parse would use it: bash
// with `expand_aliases` off still prints its aliases and still says `a is
// aliased to …`, measured. A parser asks about the switch as well.
//
// The front end passes this one unconditionally now. It used to pass
// LookupAlias only where the dialect's route table said the shell expands,
// which made the answer a fact about how the program arrived and nothing
// else — so `shopt -s expand_aliases` had nowhere to reach even once the name
// was accepted.
func (r *Runner) ExpandingAlias(name string) (string, bool) {
	if !r.aliasExpansion {
		return "", false
	}
	return r.LookupAlias(name)
}

// ExpandingGlobalAlias and ExpandingSuffixAlias are the same hook for the
// other two kinds, and they carry the same switch: the shell that has them
// expands no alias of any kind under `-c`, measured, so all three go quiet
// together rather than each deciding for itself.
func (r *Runner) ExpandingGlobalAlias(name string) (string, bool) {
	if !r.aliasExpansion {
		return "", false
	}
	return r.LookupGlobalAlias(name)
}

// ExpandingSuffixAlias takes the extension rather than the whole word.
func (r *Runner) ExpandingSuffixAlias(suffix string) (string, bool) {
	if !r.aliasExpansion {
		return "", false
	}
	return r.LookupSuffixAlias(suffix)
}

// ParseWithAliases builds a parser for text this shell is about to read, with
// all three alias tables already on it.
//
// One place, because there are six of them: `eval`, a sourced file, a command
// substitution, a backquoted one, a trap body being run and a trap action
// being checked. Each was a `syntax.NewParser` that assigned nothing, so an
// alias that worked at the top of a script stopped working one level down —
// and a fix applied to one of them would have left the other five (#2096).
//
// The dialect is a parameter because the callers do not agree on it: borrowed
// text is read as the *route it arrived by*, and `eval`'s text is a command
// string where a sourced file is a file.
func (r *Runner) ParseWithAliases(src string, d syntax.Dialect) *syntax.Parser {
	p := syntax.NewParser(src, d)
	r.ExpandAliasesIn(p)
	return p
}

// ExpandAliasesIn hands a parser the three tables, for a caller that built one
// itself.
//
// Through the three Expanding* methods rather than the tables directly, so
// that a shell whose option is off — `shopt -u expand_aliases`, `unsetopt
// aliases` — expands nothing here either. Measured: both of those turn
// expansion off inside a sourced file and inside `eval` as well as at the top
// level, so the option is what gates borrowed text.
func (r *Runner) ExpandAliasesIn(p *syntax.Parser) {
	p.Aliases = r.ExpandingAlias
	p.GlobalAliases = r.ExpandingGlobalAlias
	p.SuffixAliases = r.ExpandingSuffixAlias
}

// AliasExpansion reports whether a line parsed now would have its alias words
// replaced, for the builtin that presents the switch under a name.
func (r *Runner) AliasExpansion() bool { return r.aliasExpansion }

// SetAliasExpansion moves the switch, which is what `shopt -s expand_aliases`
// does. It does not move the base: a mode that turns it on and off again puts
// back what the route decided, not this.
func (r *Runner) SetAliasExpansion(on bool) { r.aliasExpansion = on }

// AliasExpansionBase is the answer the route gave, for a dialect whose option
// is a second input to the same question: one shell has an `aliases` option
// that reads `on` even where the route does not expand, so turning it back on
// restores what the route said rather than forcing expansion.
func (r *Runner) AliasExpansionBase() bool { return r.aliasExpansionBase }

// SetAliasExpansionBase records the answer the route gives — the dialect's
// syntax.Dialect.ExpandAliases against the route the program arrived by, or
// true at a prompt, where the whole panel expands whatever the dialect says
// about a script.
//
// It sets the live switch too, except that POSIX mode keeps it on: the front
// end reads the route before it reads the name the shell was invoked under,
// and `sh` means both. Writing the base without regard to order is what keeps
// the two independent.
func (r *Runner) SetAliasExpansionBase(on bool) {
	r.aliasExpansionBase = on
	r.aliasExpansion = on || r.posixMode
}
