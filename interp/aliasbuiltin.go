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

// AliasKind is which aliases a definition, a lookup or a listing is about.
//
// Exported because a dialect's own name-reporting builtin needs it: `whence`
// words a global and a suffix alias differently from a regular one, and it
// asks the table here rather than keeping a second notion of the kinds — see
// [Runner.AliasForName].
type AliasKind uint8

const (
	// AliasAnyKind is the plain builtin: it defines a regular alias, and
	// lists the regular and the global ones together. It is a filter rather
	// than an answer, so a lookup never returns it.
	AliasAnyKind AliasKind = iota
	// AliasGlobalKind is `-g`.
	AliasGlobalKind
	// AliasSuffixKind is `-s`, the second namespace.
	AliasSuffixKind
	// AliasRegularKind is `-r`: the aliases that are neither global nor
	// suffix. A kind of its own rather than the absence of the other two,
	// because the plain builtin lists the regular and the global ones
	// together and there is otherwise no way to ask for the plain half.
	AliasRegularKind
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

// AliasForName answers what a `type`-family builtin should say a name is,
// which is not the same question [Runner.LookupAlias] answers.
//
// Three differences, all measured:
//
//   - the *kind* comes back, because one dialect words a global and a suffix
//     alias differently from a regular one;
//   - a suffix alias is keyed on the extension rather than on the whole word,
//     so `whence -v p.txt` answers about `txt`, which is why the name to
//     print comes back too;
//   - one dialect answers only while aliases would actually expand. bash with
//     `expand_aliases` off says `type: a: not found` about an alias its own
//     `alias` builtin lists one line earlier, and turning the option on makes
//     it `a is aliased to …`. The other three answer whatever the option
//     says — zsh names one after `unsetopt aliases` — so this is
//     Semantics.TypeNamesAnAliasOnlyWhenExpanded rather than a plain read of
//     the switch.
//
// One method for all of it because there are four callers — `type`,
// `command -V`, `command -v` and this shell's `whence` — and a second copy is
// how two of them would come to disagree about a suffix.
func (r *Runner) AliasForName(name string) (display, value string, kind AliasKind, ok bool) {
	display, value, kind, ok = r.aliasTableFor(name)
	if !ok {
		// Asked only where the tables hold the name, which is the only place
		// the panel disagrees: a `type` of a builtin, a function or nothing
		// at all is the same in every column and must not be made to depend
		// on an answer about aliases.
		return "", "", AliasAnyKind, false
	}
	if r.ask(r.sem().TypeNamesAnAliasOnlyWhenExpanded, "`type` naming an alias only while aliases expand") &&
		!r.aliasExpansion {
		return "", "", AliasAnyKind, false
	}
	if r.unspecified {
		return "", "", AliasAnyKind, false
	}
	return display, value, kind, true
}

// aliasTableFor is the table half of the question above, with no axis in it.
func (r *Runner) aliasTableFor(name string) (display, value string, kind AliasKind, ok bool) {
	if a, found := r.aliases[name]; found {
		if a.global {
			return name, a.value, AliasGlobalKind, true
		}
		return name, a.value, AliasRegularKind, true
	}
	// The extension, and only where there is a word in front of the dot:
	// `whence -v .txt` is not found where `whence -v p.txt` and
	// `whence -v p.q.txt` both answer about `txt`.
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		if v, found := r.suffixAliases[name[i+1:]]; found {
			return name[i+1:], v, AliasSuffixKind, true
		}
	}
	return "", "", AliasAnyKind, false
}

// AliasSentence is the sentence `type` and `command -V` write for a name the
// tables hold, in this dialect's words and for the kind it was found under.
//
// Here rather than in the type builtin because a dialect's own `whence` wants
// the identical sentence: `type` in that shell *is* `whence -v`, measured, so
// two spellings of it would be two answers to one question.
func (r *Runner) AliasSentence(display, value string, kind AliasKind) string {
	dg := r.diag()
	format, fallback := dg.TypeAlias, "%[1]s is an alias for %[2]s"
	switch kind {
	case AliasGlobalKind:
		if dg.TypeGlobalAlias != "" {
			format, fallback = dg.TypeGlobalAlias, "%[1]s is a global alias for %[2]s"
		}
	case AliasSuffixKind:
		if dg.TypeSuffixAlias != "" {
			format, fallback = dg.TypeSuffixAlias, "%[1]s is a suffix alias for %[2]s"
		}
	case AliasAnyKind, AliasRegularKind:
	}
	if dg.TypeAliasQuotesValue {
		// One dialect quotes the body here the way its listing does, so
		// `alias a='echo hi'` and `a is an alias for 'echo hi'` agree about
		// what would have to be typed.
		value = r.quoteListedValue(r.sem().AliasQuoting, "`alias`", value)
	}
	return Wording(format, fallback, display, value)
}

// commandVAliasLine is what `command -v` writes for a name the tables hold,
// which is not the sentence `command -V` writes and not the plain listing
// either.
//
// Measured 2026-09-12. Three dialects write a line that would *define* the
// alias back — `alias a='echo hi'`, and `alias -g UP='…'` where the entry is
// global — and ksh93 writes the quoted body with nothing in front of it. The
// suffix kind is the exception in the one dialect that has it: `command -v
// p.txt` is `cat -n`, the body alone, where `command -V p.txt` is the full
// sentence.
func (r *Runner) commandVAliasLine(display, value string, kind AliasKind) string {
	if kind == AliasSuffixKind {
		return value
	}
	if kind == AliasGlobalKind {
		// The kind's own letter, so the line really would define it back.
		// Only one dialect has a kind to put here.
		display = "-g " + display
	}
	return Wording(r.diag().CommandVAlias, "alias %[1]s=%[2]s", display,
		r.quoteListedValue(r.sem().AliasQuoting, "`alias`", value))
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

// aliasForm is how one entry is written out: which kind was asked for, and
// which of the three shapes a line takes.
//
// One value rather than three booleans threaded through four functions,
// because the shapes are exclusive and the precedence between them is
// measured: `alias -L +g` writes the definition line, so `-L` wins over the
// names-only reading rather than combining with it.
type aliasForm struct {
	kind      AliasKind
	prefixed  bool // `-p`: the listing with `alias ` in front of it.
	defining  bool // `-L`: a line that would define the alias back.
	namesOnly bool // `+g` and its siblings: the name and nothing else.
}

func biAlias(r *Runner, _ context.Context, args []string) int {
	var form aliasForm
	patterns := false
	// Two questions, because dash answers the first differently from the
	// other three: it reads no options for `alias` at all, so `alias -p` is a
	// *name* there and the answer is "-p not found". zsh does read options
	// and simply has no `-p`, which is a refusal rather than a lookup.
	if r.ask(r.sem().AliasParsesOptions, "`alias` reading options at all") {
		known := r.aliasOptionLetters()
		if r.unspecified {
			return 2
		}
		rest, opts, namesOnly, code := r.readAliasPlusWords(args, known)
		if code != 0 {
			return code
		}
		args = rest
		minus, code := "", 0
		args, minus, code = r.builtinOptions("alias", args, known)
		if code != 0 {
			return code
		}
		opts += minus
		if countKindLetters(opts) > 1 {
			// The kinds are namespaces and a filter over one table, so one
			// call cannot ask for two: measured `illegal combination of
			// options`, status 1, whether the call defines or lists, and
			// `-r` joins the rule `-g` and `-s` already had.
			r.diagf("%s\n", Wording(r.diag().AliasIllegalOptionCombination,
				"alias: illegal combination of options"))
			return 1
		}
		switch {
		case strings.ContainsRune(opts, 'g'):
			form.kind = AliasGlobalKind
		case strings.ContainsRune(opts, 's'):
			form.kind = AliasSuffixKind
		case strings.ContainsRune(opts, 'r'):
			form.kind = AliasRegularKind
		}
		form.prefixed = strings.ContainsRune(opts, 'p')
		form.defining = strings.ContainsRune(opts, 'L')
		form.namesOnly = namesOnly
		patterns = strings.ContainsRune(opts, 'm')
	} else if r.unspecified {
		return 2
	}
	if len(args) == 0 {
		// `alias -m` with no pattern is the plain listing rather than a
		// refusal, measured — unlike `unalias -m`, where a removal with no
		// pattern would be a removal of everything.
		for _, name := range r.aliasNames(form.kind) {
			r.printf("%s\n", r.aliasLine(name, form))
		}
		return 0
	}
	if patterns {
		return r.aliasPatternListing(args, form)
	}
	missing, refused := 0, false
	for _, a := range args {
		name, value, isDefinition := strings.Cut(a, "=")
		if r.aliasNameIsRefused(name) {
			// Checked ahead of everything the operand could otherwise do,
			// which is what the two shells that check both show: nothing is
			// defined and no lookup is attempted for a name they will not
			// take.
			taken, stop := r.reportAliasName(name, a, isDefinition)
			if stop {
				return 1
			}
			if taken {
				refused = true
				continue
			}
		}
		if isDefinition {
			r.defineAlias(name, value, form.kind)
			continue
		}
		found, listed := r.lookupForListing(name, form.kind)
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
			r.printf("%s\n", r.aliasLine(name, form))
		}
	}
	if refused {
		// 1 however many names were refused and however many of the rest
		// were fine, which is measured: `alias 'a$b'=echo x=1 'a b'=echo`
		// defines `x`, complains twice and reports 1.
		return 1
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

// countKindLetters is how many of the three kind letters one call asked for.
//
// They are namespaces and a filter over one table rather than modifiers, so
// two of them in one call is `illegal combination of options` — which is the
// rule `-g` and `-s` already had, and `-r` joins it rather than getting a
// second one of its own.
func countKindLetters(opts string) int {
	n := 0
	for _, c := range "gsr" {
		if strings.ContainsRune(opts, c) {
			n++
		}
	}
	return n
}

// readAliasPlusWords takes the `+` words off the front of the option run.
//
// They are not option letters in the ordinary sense, which is why they are
// read here rather than by builtinOptions: `+g`, `+r` and `+s` name a kind
// and ask for the names without the values, and a bare `+` asks for the same
// thing *and ends the option list*. Measured 2026-09-12 on zsh 5.9.2, where
// `alias + -L` looks up an alias called `-L` and `alias -L +` lists
// everything in the `-L` form.
//
// The `-` words are left in place for builtinOptions to read, so a call may
// mix the two in either order — `alias -L +g` and `alias +g -L` are the same
// call — and nothing else in the tree grows a plus form it does not have.
//
// A bare `+` is replaced by `--` rather than being dropped, which is how the
// "and ends the option list" half reaches builtinOptions: the `-` words
// before it are still options and everything after it is an operand.
func (r *Runner) readAliasPlusWords(args []string, known string) (rest []string, opts string, namesOnly bool, code int) {
	if !r.ask(r.sem().AliasPlusPrintsNamesOnly, "`alias +g` printing names without values") {
		if r.unspecified {
			return nil, "", false, 2
		}
		return args, "", false, 0
	}
	rest = make([]string, 0, len(args)+1)
	for i, a := range args {
		switch {
		case a == "+":
			rest = append(rest, "--")
			return append(rest, args[i+1:]...), opts, true, 0
		case strings.HasPrefix(a, "+"):
			for j := 1; j < len(a); j++ {
				if _, ok := optionLetter(known, a[j]); !ok {
					return nil, "", false, r.refuseOption("alias", a, known)
				}
				opts += string(a[j])
			}
			namesOnly = true
		case len(a) > 1 && a[0] == '-':
			rest = append(rest, a)
		default:
			// An operand, and every word after it is one too — including a
			// later `-L`, measured.
			return append(rest, args[i:]...), opts, namesOnly, 0
		}
	}
	return rest, opts, namesOnly, 0
}

// aliasPatternListing is `-m`: every operand is a pattern rather than a name.
//
// Pattern by pattern and sorted within each, which is the order the table's
// plain listing walks and the order every other `-m` in this tree uses. A
// pattern that matches nothing is not a failure — `alias -m 'z*'` is 0 with
// no output, where `alias z` is 1 — because a pattern says how many names it
// expects and a name says one.
func (r *Runner) aliasPatternListing(patterns []string, form aliasForm) int {
	names := r.aliasNames(form.kind)
	for _, pattern := range patterns {
		o := r.patternOpts(pattern)
		for _, name := range names {
			if matchPattern(pattern, name, o) {
				r.printf("%s\n", r.aliasLine(name, form))
			}
		}
	}
	return 0
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
	if r.ask(r.sem().AliasListsAsDefinitions, "`alias -L`") {
		known += "L"
	}
	if r.ask(r.sem().AliasRestrictsToRegularKind, "`alias -r`") {
		known += "r"
	}
	if r.ask(r.sem().AliasOperandsCanBePatterns, "`alias -m`") {
		known += "m"
	}
	return known
}

// defineAlias writes one entry of whichever table the kind names.
func (r *Runner) defineAlias(name, value string, kind AliasKind) {
	if kind == AliasSuffixKind {
		if r.suffixAliases == nil {
			r.suffixAliases = map[string]string{}
		}
		r.suffixAliases[name] = value
		return
	}
	if r.aliases == nil {
		r.aliases = map[string]aliasDef{}
	}
	r.aliases[name] = aliasDef{value: value, global: kind == AliasGlobalKind}
}

// lookupForListing answers the two questions a named operand asks: whether
// the tables hold it at all, which is what the status is about, and whether
// it is of the kind the letter asked for, which is what the listing is about.
func (r *Runner) lookupForListing(name string, kind AliasKind) (found, listed bool) {
	if kind == AliasSuffixKind {
		_, ok := r.suffixAliases[name]
		return ok, ok
	}
	a, ok := r.aliases[name]
	if !ok {
		return false, false
	}
	if kind == AliasRegularKind {
		return true, !a.global
	}
	return true, kind == AliasAnyKind || a.global
}

// AliasNames is every alias of one kind, sorted — what a listing walks, and
// what a dialect that presents the table as a *parameter* enumerates.
//
// Exported for the second of those: bash's `BASH_ALIASES` is the alias table
// written as an association, so `${!BASH_ALIASES[@]}` has to ask this
// question from outside the package. It is the same function the `alias`
// listing uses rather than a second walk, which is what keeps the two from
// disagreeing about which aliases exist.
func (r *Runner) AliasNames(kind AliasKind) []string { return r.aliasNames(kind) }

// DefineAlias writes one alias, which is what `alias name=value` does.
//
// Exported for the same reason AliasNames is, and it is the write half of the
// same parameter: `BASH_ALIASES[w]=date` defines an alias in bash, measured,
// so a produced association that could only be read would be half the name.
func (r *Runner) DefineAlias(name, value string, kind AliasKind) {
	r.defineAlias(name, value, kind)
}

// aliasNames is what a listing walks, in order.
func (r *Runner) aliasNames(kind AliasKind) []string {
	var names []string
	if kind == AliasSuffixKind {
		names = make([]string, 0, len(r.suffixAliases))
		for name := range r.suffixAliases {
			names = append(names, name)
		}
	} else {
		names = make([]string, 0, len(r.aliases))
		for name, a := range r.aliases {
			if kind == AliasGlobalKind && !a.global {
				continue
			}
			if kind == AliasRegularKind && a.global {
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
	if r.ask(r.sem().AliasOperandsCanBePatterns, "`unalias -m`") {
		known += "m"
	}
	if r.unspecified {
		return 2
	}
	args, opts, code := r.builtinOptions("unalias", args, known)
	if code != 0 {
		return code
	}
	kind := AliasAnyKind
	if strings.ContainsRune(opts, 's') {
		kind = AliasSuffixKind
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
		if kind == AliasSuffixKind {
			r.suffixAliases = nil
		} else {
			r.aliases = nil
		}
		return 0
	}
	if len(args) == 0 {
		// `unalias -m` with nothing after it is short of arguments rather
		// than the no-operand answer the plain form gives: a removal with no
		// pattern would be a removal of everything, which is what `-a` is
		// for. Measured `zsh:unalias:1: not enough arguments`, 1.
		if strings.ContainsRune(opts, 'm') {
			r.diagf("%s\n", Wording(r.diag().UnaliasNoPattern,
				"unalias: not enough arguments"))
			return 1
		}
		return r.unaliasWithNothingToRemove()
	}
	if strings.ContainsRune(opts, 'm') {
		return r.unaliasByPattern(args, kind)
	}
	status := 0
	for _, name := range args {
		if !r.removeAlias(name, kind) {
			status = r.aliasNotFound("unalias", name)
		}
	}
	return status
}

// unaliasByPattern is `unalias -m`: the operands are patterns and every name
// they pick out goes.
//
// A pattern that matches nothing is 1 and says nothing, which is the same
// answer a name that is not there gets in this dialect — measured — so the
// status is about "nothing was removed" rather than about a name.
func (r *Runner) unaliasByPattern(patterns []string, kind AliasKind) int {
	status := 1
	for _, pattern := range patterns {
		o := r.patternOpts(pattern)
		for _, name := range r.aliasNames(kind) {
			if matchPattern(pattern, name, o) && r.removeAlias(name, kind) {
				status = 0
			}
		}
	}
	return status
}

// removeAlias takes one name out of whichever table the kind names, and
// reports whether it was there.
func (r *Runner) removeAlias(name string, kind AliasKind) bool {
	if kind == AliasSuffixKind {
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

// aliasLine spells one entry in whichever of the three shapes was asked for.
func (r *Runner) aliasLine(name string, form aliasForm) string {
	prefix := r.diag().AliasListPrefix
	switch {
	case form.defining:
		// `-L` writes a command that would define the entry back, so the
		// kind's own letter has to be in it — and it comes from the *entry*
		// rather than from the letter that was asked for, which is what
		// makes a plain `alias -L` distinguish its global rows from its
		// regular ones.
		prefix = "alias "
		switch {
		case form.kind == AliasSuffixKind:
			prefix += "-s "
		case r.aliases[name].global:
			prefix += "-g "
		}
	case form.prefixed:
		// `-p` prints the prefix even in the dialect whose plain listing has
		// none, which is what makes the option worth having there.
		prefix = "alias "
	case form.namesOnly:
		return name
	}
	value := ""
	if form.kind == AliasSuffixKind {
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

// AliasExpansionBase is the answer the dialect gave, for a caller that has to
// tell "off because this shell does not expand" from "off because somebody
// turned it off".
func (r *Runner) AliasExpansionBase() bool { return r.aliasExpansionBase }

// SetAliasExpansionBase records whether this shell expands aliases with nobody
// having asked — the dialect's syntax.Dialect.AliasesExpandUnlessTold, or true
// at a prompt, where the whole panel expands whatever the dialect says about a
// script.
//
// Not the route. Which routes' *program text* expands is
// syntax.Dialect.ExpandAliasesInProgramText, and the front end keeps that to
// itself: it decides whether the program's own parser is handed a table, and
// nothing nested inside the program asks it. #2109 is why the two are
// separate.
//
// It sets the live switch too, except that POSIX mode keeps it on: the front
// end reads the route before it reads the name the shell was invoked under,
// and `sh` means both. Writing the base without regard to order is what keeps
// the two independent.
func (r *Runner) SetAliasExpansionBase(on bool) {
	r.aliasExpansionBase = on
	r.aliasExpansion = on || r.posixMode
}

// aliasNameIsRefused reports whether a name holds a character an alias may
// not carry here.
//
// The set is the dialect's — see Semantics.AliasNameRefusedCharacters — and
// an empty one is a shell that takes any name at all, which is three of the
// five. Not an axis with a set beside it: the two shells that check do not
// agree on what is in the set, so the set *is* the answer and an empty one
// is the whole of "this shell does not check".
func (r *Runner) aliasNameIsRefused(name string) bool {
	set := r.sem().AliasNameRefusedCharacters
	return set != "" && strings.ContainsAny(name, set)
}

// reportAliasName complains about a name an alias may not carry.
//
// taken says the name was refused and the operand is finished with; stop says
// the script ends here. A shell that checks only a definition answers false
// to both for a bare lookup, which then goes on to be looked up and reported
// as not found — measured, that is exactly what bash says for `alias 'a$b'`.
func (r *Runner) reportAliasName(name, operand string, isDefinition bool) (taken, stop bool) {
	if !isDefinition {
		if !r.ask(r.sem().AliasNameCheckReachesALookup,
			"`alias` checking the name of a lookup as well as of a definition") {
			return r.unspecified, false
		}
	}
	d := r.diag()
	line := Wording(d.AliasInvalidName, "alias: %[2]s: invalid alias name",
		"alias", name, operand)
	if d.AliasInvalidNameUnprefixed {
		r.errf("%s\n", line)
	} else {
		r.diagf("%s\n", line)
	}
	if r.ask(r.sem().AliasInvalidNameFatal, "a name an alias may not carry ending the script") {
		r.status = 1
		r.fatalQuiet()
		return true, true
	}
	return true, false
}
