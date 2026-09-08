// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strings"

	"github.com/blairham/sh/interp"
)

// `zparseopts` is how a function of this shell reads its own flags: it takes
// the positional parameters, matches them against a list of option
// descriptions, and writes what it found into arrays or into an association.
//
// Measured 2026-09-06 against zsh 5.9.2 with a scratch HOME and no startup
// files, and again 2026-09-08 for `-M`. Read alongside the module's own manual
// page — which documents the spec grammar completely — but the manual is not
// the authority here: it calls `-M`'s results "unpredictable if the `name+`
// specifier is used inconsistently", and they are not. Every form below is a
// measured form.
//
// The panel puts the whole builtin here and nowhere else. `zparseopts` is
// zsh's alone: bash 5.3.15, that binary under argv[0] `sh`, bash 3.2.57, dash
// and ksh93 all answer `command not found` at 127, so there is no intersection
// to place in the core and no axis for the others to disagree on. It is by far
// the most used of the module's four builtins: a real plugin manager calls it
// five times before it has loaded anything.
//
// Four things about it are easy to get plausibly wrong, and each is what makes
// a wrong answer here worse than a refusal: the caller gets variables it can
// read, holding values it did not ask for, at status 0.
//
//  1. **Where an argument goes.** `a:` puts the argument in an element of its
//     own — `(-a val)` — and `a:-` and `a::` put it in the *same* element as
//     the option, `(-aval)`. A script reading `$x[2]` gets the option's
//     argument under one spelling and the next option under the other.
//  2. **Where a repeated option's answer *sits*.** Without `+` only one
//     appearance survives, and the element it leaves behind is at the position
//     of the **first** appearance carrying the **last** one's argument:
//     `-a v1 -c z -a v2` against `-a arr a: c:` is `(-a v2 -c z)`, not
//     `(-c z -a v2)`. Reading it as "keep the last occurrence" reorders the
//     array under a script that indexes it.
//  3. **When parsing stops.** At the first word no description covers,
//     unless `-E`; and always at a bare `-` or `--`, `-E` or not. A parser
//     that ran to the end of `$@` would collect flags a script meant to pass
//     on to something else.
//  4. **What `-M` moves and what it leaves.** It makes one description store
//     under another's, and the two halves come apart: the *matched*
//     description still decides whether an argument is taken from the command
//     line, and the *target* decides the shape it lands in. See
//     zparseoptsResolve.

// zparseoptsArg is what a description says about its option's argument.
type zparseoptsArg uint8

const (
	// zparseoptsNoArg is `name`: a flag.
	zparseoptsNoArg zparseoptsArg = iota
	// zparseoptsRequired is `name:`: an argument, in an element of its own.
	zparseoptsRequired
	// zparseoptsRequiredJoined is `name:-`: an argument, in the same element
	// as the option name.
	zparseoptsRequiredJoined
	// zparseoptsOptional is `name::`: an argument if there is one, in the
	// same element as the option name. "If there is one" is measured and is
	// not "in the same word": a following word is taken as the argument
	// unless it begins with `-`.
	zparseoptsOptional
)

// takesArgument reports whether a description's option can be followed by an
// argument at all, which is the question the overlap rule asks.
func (k zparseoptsArg) takesArgument() bool { return k != zparseoptsNoArg }

// joinsArgument reports whether the argument shares the option's element.
func (k zparseoptsArg) joinsArgument() bool {
	return k == zparseoptsRequiredJoined || k == zparseoptsOptional
}

// zparseoptsSpec is one option description.
type zparseoptsSpec struct {
	// word is the description exactly as it was written, which is what the
	// cyclic-mapping refusal quotes back.
	word string
	// name is the option without its leading `-`, so `a` is `-a` and `-foo`
	// is `--foo`. Measured: a description of `foo` matches `-foo` and not
	// `--foo`, and one of `-foo` matches `--foo` and not `-foo`.
	name string
	// plus is `+`: every appearance is kept rather than only the last.
	plus bool
	arg  zparseoptsArg
	// target is the `=` half: an array name, or under `-M` the name of
	// another description this one stores under.
	target string
	// hasTarget is whether there was an `=` at all, which is not the same
	// question as whether target is empty. `a` takes the default array and
	// `a=` names an array called nothing — measured, `zparseopts -a A a= b`
	// is `not an identifier: ` at 1 where `zparseopts -a A a b` is silent,
	// and `zparseopts a= b` complains about `b` rather than about `a=`, so
	// the empty `=` satisfies the missing-array check and then fails the
	// name check.
	hasTarget bool
}

// option is the word this description matches, which is also the word written
// into the array: one `-` in front of the name as written.
func (s zparseoptsSpec) option() string { return "-" + s.name }

// zparseoptsOpts is what the letters asked for.
type zparseoptsOpts struct {
	remove   bool   // -D
	every    bool   // -E
	strict   bool   // -F
	keep     bool   // -K
	alias    bool   // -M
	array    string // -a
	assoc    string // -A
	hasAssoc bool
}

// zparseoptsLetters are this builtin's own letters. All six are implemented;
// there is nothing here that refuses by name.
const zparseoptsLetters = "DEFKMaA"

func registerZparseopts(r *interp.Runner) {
	r.Register("zparseopts", zparseoptsBuiltin)
}

func zparseoptsBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, rest, code := zparseoptsOptions(r, args)
	if code != 0 {
		return code
	}
	if len(rest) == 0 {
		r.Diagnosef("missing option descriptions\n")
		return 1
	}
	specs, code := zparseoptsSpecs(r, opts, rest)
	if code != 0 {
		return code
	}
	res, code := zparseoptsResolve(r, opts, specs)
	if code != 0 {
		return code
	}
	return zparseoptsRun(r, opts, specs, res)
}

// zparseoptsOptions reads this builtin's own letters.
//
// **They cannot be stacked**, and that is a fact about the syntax rather than
// a simplification: `-DEK` is indistinguishable from a description of the
// GNU-style long option `--DEK`, so zsh reads it as one and so does this.
// Measured — `zparseopts -DQ a=x` is `no default array defined: -DQ`, the
// whole word having become a description.
func zparseoptsOptions(r *interp.Runner, args []string) (opts zparseoptsOpts, rest []string, code int) {
	rest = args
	for len(rest) > 0 {
		word := rest[0]
		if word == "-" || word == "--" {
			// Both spellings end the letters, measured, and neither is a
			// description.
			return opts, rest[1:], 0
		}
		if len(word) < 2 || word[0] != '-' {
			return opts, rest, 0
		}
		letter := word[1]
		switch {
		case strings.IndexByte(zparseoptsLetters, letter) < 0:
			// Not one of ours, so the word is a description rather than a
			// bad option — which is what makes an unstackable letter set
			// observable.
			return opts, rest, 0
		case len(word) > 2 && letter != 'a' && letter != 'A':
			// A stack, and therefore a description of a long option.
			return opts, rest, 0
		}
		rest = rest[1:]
		name := word[2:]
		if letter == 'a' || letter == 'A' {
			if name == "" {
				if len(rest) == 0 {
					r.Diagnosef("missing array name\n")
					return opts, nil, 1
				}
				name, rest = rest[0], rest[1:]
			}
			if letter == 'a' {
				opts.array = name
			} else {
				opts.assoc, opts.hasAssoc = name, true
			}
			continue
		}
		switch letter {
		case 'D':
			opts.remove = true
		case 'E':
			opts.every = true
		case 'F':
			opts.strict = true
		case 'K':
			opts.keep = true
		case 'M':
			opts.alias = true
		}
	}
	return opts, rest, 0
}

// zparseoptsSpecs reads the option descriptions.
//
// A description is `name[+][:|::|:-][=array]`, and any of the special
// characters may appear in the name preceded by a backslash — which is why
// the scan is character by character rather than a split on `=`.
func zparseoptsSpecs(r *interp.Runner, opts zparseoptsOpts, words []string) ([]zparseoptsSpec, int) {
	specs := make([]zparseoptsSpec, 0, len(words))
	seen := make(map[string]bool, len(words))
	for _, word := range words {
		spec, ok := parseZparseoptsSpec(word)
		if !ok {
			r.Diagnosef("invalid option description: %s\n", word)
			return nil, 1
		}
		if !spec.hasTarget && opts.array == "" && !opts.hasAssoc {
			// Measured, and it is the refusal a bare `zparseopts a` gets:
			// nowhere to put what it finds. The word is quoted back as
			// written, so a mistyped letter of this builtin's own — which
			// reaches here as a description — names itself.
			r.Diagnosef("no default array defined: %s\n", word)
			return nil, 1
		}
		if spec.name != "" {
			if seen[spec.name] {
				r.Diagnosef("option defined more than once: %s\n", spec.name)
				return nil, 1
			}
			seen[spec.name] = true
		}
		specs = append(specs, spec)
	}
	return specs, 0
}

// parseZparseoptsSpec reads one description.
func parseZparseoptsSpec(word string) (zparseoptsSpec, bool) {
	spec := zparseoptsSpec{word: word}
	var name strings.Builder
	i := 0
	for ; i < len(word); i++ {
		c := word[i]
		if c == '\\' && i+1 < len(word) {
			// An escaped special character is part of the name, which is how
			// an option called `-a:b` is described at all.
			i++
			name.WriteByte(word[i])
			continue
		}
		if c == '+' || c == ':' || c == '=' {
			break
		}
		name.WriteByte(c)
	}
	spec.name = name.String()
	if spec.name == "" {
		// A description that names no option. Not an error of its own, and
		// measured twice to be sure: `zparseopts -a arr "=x"` is silently 0
		// — the description simply never matches, and two of them are still
		// 0 — while `zparseopts "=x"` is `no default array defined: =x`. So
		// it is a description with *no* array, whatever follows the `=`, and
		// the complaint about a missing default array is the one it earns.
		return zparseoptsSpec{word: word}, true
	}
	if i < len(word) && word[i] == '+' {
		spec.plus = true
		i++
	}
	if i < len(word) && word[i] == ':' {
		i++
		spec.arg = zparseoptsRequired
		if i < len(word) {
			switch word[i] {
			case ':':
				spec.arg = zparseoptsOptional
				i++
			case '-':
				spec.arg = zparseoptsRequiredJoined
				i++
			}
		}
	}
	if i == len(word) {
		return spec, true
	}
	if word[i] != '=' {
		// `a:::=x` reaches here, and is `invalid option description` in zsh.
		return spec, false
	}
	spec.target, spec.hasTarget = word[i+1:], true
	return spec, true
}

// zparseoptsResolution is where each description's matches are stored, once
// `-M` has been read.
//
// Without `-M` it is the identity: every description stores under itself, in
// its own `=array` or in the default one. `-M` is what makes it worth a pass
// of its own.
type zparseoptsResolution struct {
	// terminal[i] is the description whose *storage* description i uses. It
	// is i itself for every description that is not an alias.
	terminal []int
	// array[i] is the array a terminal description writes into, empty for a
	// description that writes into no array at all. Only a terminal's entry
	// is consulted.
	array []string
	// named[i] is whether description i wrote that array name itself, with
	// an `=`. It is not `array[i] != ""`: `a=` names an array called nothing,
	// which is a name to be refused rather than an absent one, and an alias
	// or a self-alias has an empty array that is absent rather than refused.
	named []bool
}

// zparseoptsResolve reads `-M` and settles, for every description, where its
// matches go.
//
// **What `-M` changes is one character's meaning and nothing else**: the `=`
// half. Without it `a=b` names the array `b`; with it, `a=b` names the
// *description* `b` when there is one and the array `b` when there is not.
// Measured both ways — `zparseopts -M -a A a=foo` with no description called
// `foo` fills `foo`, exactly as the same line without `-M` does, so the letter
// is not a mode the spec grammar switches into. The manual's own example
// leans on this: `-A bar -M a=foo b+: c:=b` aliases `c` onto `b` and puts `-a`
// in an array called `foo`, in the same command.
//
// Links follow transitively — `a:=b b:=c c:=q` puts a `-a` in `q` — and a
// cycle of two or more is `cyclic option mapping:` quoting the description
// that closes it, at 1, with nothing parsed and nothing stored. A description
// aliased to *itself* is not a cycle and not an error: it is a description
// that stores in no array at all, and `zparseopts -M -a A a=a` with `-a` on
// the command line leaves `A` empty at status 0. It still reaches an
// association, keyed by its own option, which is how the two halves are told
// apart.
//
// Nothing here asks whether an array *name* is a name. That check is not
// static at all — see zparseoptsStore — which is also what lets an alias name
// a description whose spelling is no identifier: `-M -a A a=-b -b` is the
// long-option pair `--b`/`--a` and is silent.
func zparseoptsResolve(
	r *interp.Runner, opts zparseoptsOpts, specs []zparseoptsSpec,
) (zparseoptsResolution, int) {
	res := zparseoptsResolution{
		terminal: make([]int, len(specs)),
		array:    make([]string, len(specs)),
		named:    make([]bool, len(specs)),
	}
	link := make([]int, len(specs))
	for i := range link {
		link[i] = -1
		res.terminal[i] = i
	}
	if opts.alias {
		byName := make(map[string]int, len(specs))
		for i, s := range specs {
			if s.name != "" {
				byName[s.name] = i
			}
		}
		for i, s := range specs {
			// The guard is hasTarget rather than a non-empty target so that
			// it reads the same way as the missing-array check above, and
			// the two are not distinguishable here on purpose: a description
			// that names no option is never in byName, so an empty target
			// never finds a link either way.
			if !s.hasTarget {
				continue
			}
			if j, ok := byName[s.target]; ok {
				link[i] = j
			}
		}
	}
	for i := range specs {
		seen := map[int]bool{i: true}
		j := i
		for link[j] >= 0 && link[j] != j {
			next := link[j]
			if seen[next] {
				// The description that closes the cycle, as written — which
				// is `b=a` for `a=b b=a` and `c=a` for `a=b b=c c=a`, and
				// `b=a` again for `x=a a=b b=a`, where the walk enters the
				// cycle from outside it.
				r.Diagnosef("cyclic option mapping: %s\n", specs[j].word)
				return zparseoptsResolution{}, 1
			}
			seen[next] = true
			j = next
		}
		res.terminal[i] = j
	}
	for i, s := range specs {
		switch {
		case link[i] == i:
			// Aliased to itself: no array, and not the default one either.
		case link[i] >= 0:
			// An alias names a description rather than an array, so it names
			// no array — which is also why it never earns the missing-array
			// complaint the descriptions pass raises.
		case s.hasTarget:
			res.array[i], res.named[i] = s.target, true
		default:
			res.array[i] = opts.array
		}
	}
	return res, 0
}

// isIdentifier reports whether a name is one a script could read back.
func isIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// zparseoptsMatch is one appearance of one option on the command line, in the
// order it appeared.
//
// It records what was *matched* and not what will be written. Which array the
// match lands in, what shape it takes and which option spells it are all
// questions about the description it stores under, which `-M` can make a
// different one — so they are settled once, in zparseoptsStore, rather than
// laid out here where the group is not yet known.
type zparseoptsMatch struct {
	spec int
	// option is the option as this word spelled it, `-` and the description's
	// name.
	option string
	// value is the argument, and has says whether there was one — which is
	// not the same as an empty one: `-a ""` and a bare `-a` under `a::` give
	// different arrays.
	value string
	has   bool
}

// zparseoptsRun is the parse itself, and it is deliberately in two halves:
// everything is worked out against a copy first and written only if the whole
// command line parsed. That is what `-F` means — "removal and extraction are
// not performed, and option arrays are not updated" — and it is measured for
// a missing argument as well, which fails the same way with or without the
// letter.
func zparseoptsRun(
	r *interp.Runner, opts zparseoptsOpts, specs []zparseoptsSpec, res zparseoptsResolution,
) int {
	params := r.Params
	var matches []zparseoptsMatch
	consumed := make([]bool, len(params))
	i := 0
	for ; i < len(params); i++ {
		word := params[i]
		if word == "-" || word == "--" {
			// Always a stop, `-E` or not. With `-D` and without `-E` the
			// word itself goes too; with `-E` it stays.
			if opts.remove && !opts.every {
				consumed[i] = true
			}
			break
		}
		if len(word) < 2 || word[0] != '-' {
			if opts.every {
				continue
			}
			break
		}
		match, next, ok := zparseoptsWord(r, opts, specs, word, params, i)
		if !ok {
			return 1
		}
		if match == nil {
			// An option no description covers. `-F` refuses it and has
			// already said so; otherwise parsing stops here without taking
			// the word.
			if opts.strict {
				return 1
			}
			if opts.every {
				continue
			}
			break
		}
		matches = append(matches, match...)
		for j := i; j <= next; j++ {
			consumed[j] = true
		}
		i = next
	}
	code := zparseoptsStore(r, opts, specs, res, matches)
	if opts.remove {
		kept := make([]string, 0, len(params))
		for j, p := range params {
			if !consumed[j] {
				kept = append(kept, p)
			}
		}
		r.Params = kept
	}
	// After the removal and not instead of it: a store that refuses a name
	// still leaves `-D`'s work done, measured — `zparseopts -D -a A a=1bad`
	// on `-a v` reports `not an identifier` and leaves `(v)` in `$@`.
	return code
}

// zparseoptsWord matches one command-line word, which may hold several
// options: `-ab` is two when both are flags.
//
// It returns the matches, the index of the last positional parameter the word
// used up — its own, or the next one when the argument came from there — and
// whether the parse may continue. A nil match list with ok is a word no
// description covers.
func zparseoptsWord(
	r *interp.Runner, opts zparseoptsOpts, specs []zparseoptsSpec,
	word string, params []string, at int,
) (out []zparseoptsMatch, next int, ok bool) {
	next = at
	text := word[1:]
	for text != "" {
		idx := zparseoptsLongest(specs, text)
		if idx < 0 {
			if opts.strict {
				// `-F` names the whole word as written, measured: the
				// complaint is about `-z` and not about the letter left of
				// it.
				r.Diagnosef("bad option: %s\n", word)
			}
			return nil, at, true
		}
		spec := specs[idx]
		rest := text[len(spec.name):]
		if spec.arg == zparseoptsNoArg {
			out = append(out, zparseoptsMatch{spec: idx, option: spec.option()})
			if rest == "" {
				return out, next, true
			}
			// Clustering: what is left of the word is read as though it had
			// its own `-`.
			text = rest
			continue
		}
		arg, has, code := zparseoptsArgument(r, spec, rest, params, at, &next)
		if code != 0 {
			return nil, at, false
		}
		out = append(out, zparseoptsMatch{spec: idx, option: spec.option(), value: arg, has: has})
		return out, next, true
	}
	return out, next, true
}

// zparseoptsArgument finds a described option's argument.
//
// Measured, and the two rules differ in exactly one place: a **mandatory**
// argument is taken from the next word whatever it looks like — `-a --` puts
// `--` in the array — and an **optional** one is taken from the next word
// only when that word does not begin with `-`.
func zparseoptsArgument(
	r *interp.Runner, spec zparseoptsSpec, rest string,
	params []string, at int, next *int,
) (arg string, has bool, code int) {
	if rest != "" {
		return rest, true, 0
	}
	following := at + 1
	switch spec.arg {
	case zparseoptsOptional:
		if following < len(params) && !strings.HasPrefix(params[following], "-") {
			*next = following
			return params[following], true, 0
		}
		return "", false, 0
	default:
		if following >= len(params) {
			r.Diagnosef("missing argument for option: %s\n", spec.option())
			return "", false, 1
		}
		*next = following
		return params[following], true, 0
	}
}

// zparseoptsLongest picks the description an option word matches.
//
// Two rules, both measured, and which applies depends on the candidates
// rather than on the word: **when every candidate is a flag the longest name
// wins**, whichever order they were written in — `-foo` and `-foobar` against
// `--foobar` is `-foobar` both ways round — and **when any candidate takes an
// argument the last one written wins**, which is observable precisely because
// it is order-dependent: `-foo: -foobar` against `--foobar` matches
// `-foobar`, and `-foobar -foo:` matches `-foo` with an argument of `bar`.
func zparseoptsLongest(specs []zparseoptsSpec, text string) int {
	best, bestLen, anyArg := -1, -1, false
	for i, s := range specs {
		// A description naming no option matches nothing rather than
		// everything, which is what an unguarded prefix test would make of
		// an empty name.
		if s.name == "" || !strings.HasPrefix(text, s.name) {
			continue
		}
		if s.arg.takesArgument() {
			anyArg = true
		}
		if best < 0 || len(s.name) > bestLen {
			best, bestLen = i, len(s.name)
		}
	}
	if !anyArg || best < 0 {
		return best
	}
	for i := len(specs) - 1; i >= 0; i-- {
		if specs[i].name != "" && strings.HasPrefix(text, specs[i].name) {
			return i
		}
	}
	return best
}

// zparseoptsStore writes what was found.
//
// A description owns **one slot** in its array, and the slot sits where the
// description's *first* match was: `-a v1 -c z -a v2` against `-a arr a: c:`
// is `(-a v2 -c z)`, the second `-a` having replaced the first in place rather
// than moving to the end. `+` is the opposite and simpler rule — every match
// appends, in command-line order, and no slot is shared.
//
// Under `-M` the slot belongs to the *group*: every description aliased onto
// one terminal shares the terminal's slot, its array, its `+` and its element
// shape. That is what makes the two halves of a match come apart, and both
// halves are measured:
//
//   - `zparseopts -M a:=b b:=q` with `-a v -b w` leaves `q` as `(-a w)` — the
//     option the **first** match spelled beside the **last** match's argument.
//     Neither `(-a v)` nor `(-b w)` is the answer, and both are what a reading
//     that kept one whole match would give.
//   - where the terminal joins its argument, the option in the element is the
//     **terminal's** and not the match's: `-M a:=b b:-=q` with `-a v` is
//     `(-bv)`. With `+` on that terminal, every element is spelled the
//     terminal's way — `(-bv1 -bw -bv2)` — where an unjoined terminal spells
//     each element the way its own match was written, `(-b w -a v)`.
//
// Which arrays are cleared is the whole of `-K`: without it every array a
// terminal description names is replaced, matched or not, **and so is the
// default array whether or not any description names it** — `A=(pre);
// zparseopts -a A a=q` with no arguments at all leaves `A` empty. With `-K` an
// array whose descriptions never matched keeps what it held.
func zparseoptsStore(
	r *interp.Runner, opts zparseoptsOpts, specs []zparseoptsSpec,
	res zparseoptsResolution, matches []zparseoptsMatch,
) int {
	lists := map[string][][]string{}
	used := map[string]bool{}
	slotAt := make(map[int]int, len(specs))
	firstOption := make(map[int]string, len(specs))
	assoc := map[string]string{}
	for _, m := range matches {
		group := res.terminal[m.spec]
		target := specs[group]
		if _, ok := firstOption[group]; !ok {
			firstOption[group] = m.option
		}
		option := m.option
		if !target.plus {
			// The slot keeps the spelling that opened it; only the argument
			// is replaced. Without `-M` this is the same word either way,
			// which is why the rule is invisible until an alias is in play.
			option = firstOption[group]
		}
		elems := zparseoptsElems(target, option, m.value, m.has)
		if name := res.array[group]; name != "" {
			used[name] = true
			at, ok := slotAt[group]
			switch {
			case target.plus:
				lists[name] = append(lists[name], elems)
			case ok:
				lists[name][at] = elems
			default:
				slotAt[group] = len(lists[name])
				lists[name] = append(lists[name], elems)
			}
		}
		// The association is keyed by the *terminal's* option however the
		// match was spelled, which is the manual's own example: `c:=b` puts
		// `-c3`'s argument under `-b`.
		if target.plus {
			assoc[target.option()] += m.value
			continue
		}
		assoc[target.option()] = m.value
	}
	bad, refused := "", false
	for _, name := range zparseoptsArrays(opts, specs, res) {
		if !used[name] && opts.keep {
			// Never written, so never checked: `zparseopts -K -a A a=1bad`
			// with nothing to match is a silent 0, and the same line with a
			// `-a` on the command line is not.
			continue
		}
		if !isIdentifier(name) {
			if !refused {
				bad, refused = name, true
			}
			continue
		}
		var elems []string
		for _, slot := range lists[name] {
			elems = append(elems, slot...)
		}
		r.SetArray(name, elems)
	}
	if opts.hasAssoc {
		if !isIdentifier(opts.assoc) {
			if !refused {
				bad, refused = opts.assoc, true
			}
		} else {
			table := map[string]string{}
			if opts.keep {
				// `-K` preserves the individual elements of an association,
				// where for an array it preserves the whole thing or none of
				// it. Measured both ways.
				if existing, ok := r.GetAssoc(opts.assoc); ok {
					table = existing
				}
			}
			for k, v := range assoc {
				table[k] = v
			}
			r.SetAssoc(opts.assoc, table)
		}
	}
	if !refused {
		return 0
	}
	// Real zsh's own assignment machinery complains here — `not an
	// identifier: 1bad`, located as the shell rather than as this builtin —
	// and then *aborts* a non-interactive shell. The sentence is reproduced
	// and the fatality is not: see the pull request.
	r.DiagnoseAsTheShellf("not an identifier: %s\n", bad)
	return 1
}

// zparseoptsArrays is every array this call writes, in the order the
// not-an-identifier refusal picks its name from — which is the only thing the
// order is observable through, since the writes themselves are to different
// names.
//
// The order is measured and it is not the descriptions': the arrays a
// description named with `=` come **last first**, and the default array comes
// after all of them. `zparseopts -a A a=1bad b=2bad c=3bad` names `3bad`, the
// same line with the three descriptions reversed names `1bad`, and
// `zparseopts -a 1bad a=2bad b` names `2bad` though the last description is
// the one using the default array. A description with no `=` contributes the
// default array and nothing of its own, which is why it does not pull the
// default forward.
func zparseoptsArrays(
	opts zparseoptsOpts, specs []zparseoptsSpec, res zparseoptsResolution,
) []string {
	var names []string
	seen := map[string]bool{}
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for i := len(specs) - 1; i >= 0; i-- {
		if res.named[i] {
			add(res.array[i])
		}
	}
	if opts.array != "" {
		add(opts.array)
	}
	return names
}

// zparseoptsElems lays one match out the way the description it stores under
// says to. option is the spelling the element carries where the shape leaves
// room for one — see zparseoptsStore for which spelling that is.
func zparseoptsElems(target zparseoptsSpec, option, value string, has bool) []string {
	switch {
	case !has:
		// No argument this time, so the slot is the option alone — even
		// where an earlier match of the same group had one: `-M a=b b:=q`
		// with `-b w -a` leaves `(-b)` and not `(-b w)`.
		return []string{option}
	case target.arg.joinsArgument():
		return []string{target.option() + value}
	default:
		return []string{option, value}
	}
}
