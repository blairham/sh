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
// files, and read alongside the module's own manual page — which documents
// the spec grammar completely, so every form below is a measured form and
// nothing is inferred from the wording alone.
//
// It is by far the most used of the module's four builtins: a real plugin
// manager calls it five times before it has loaded anything, and it is the
// standard way a zsh function reads its flags. Nothing in it touches the
// filesystem, the editor or completion — it reads `$@` and writes a
// parameter, which is why it is worth having on its own.
//
// Three things about it are easy to get plausibly wrong, and each is what
// makes a wrong answer here worse than a refusal: the caller gets variables
// it can read, holding values it did not ask for, at status 0.
//
//  1. **Where an argument goes.** `a:` puts the argument in an element of its
//     own — `(-a val)` — and `a:-` and `a::` put it in the *same* element as
//     the option, `(-aval)`. A script reading `$x[2]` gets the option's
//     argument under one spelling and the next option under the other.
//  2. **What "the last occurrence" means.** Without `+`, only the last
//     appearance of *that option* survives, and several options can share one
//     array — so the pruning is per description and the array is still in the
//     order the command line was in.
//  3. **When parsing stops.** At the first word no description covers,
//     unless `-E`; and always at a bare `-` or `--`, `-E` or not. A parser
//     that ran to the end of `$@` would collect flags a script meant to pass
//     on to something else.
//
// `-M` is refused by name rather than built: see zparseoptsUnimplemented.

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
	// name is the option without its leading `-`, so `a` is `-a` and `-foo`
	// is `--foo`. Measured: a description of `foo` matches `-foo` and not
	// `--foo`, and one of `-foo` matches `--foo` and not `-foo`.
	name string
	// plus is `+`: every appearance is kept rather than only the last.
	plus bool
	arg  zparseoptsArg
	// array is the `=array` half, empty when the description has none and
	// the default array is to be used.
	array string
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
	array    string // -a
	assoc    string // -A
	hasAssoc bool
}

// zparseoptsLetters are the letters implemented here, and
// zparseoptsUnimplemented is `-M`, which is refused by name.
//
// **`-M` is refused because it was measured and could not be explained**,
// which is the one reason worth writing down. It maps several descriptions
// onto one storage slot, and the manual itself says results "may be
// unpredictable if the `name+` specifier is used inconsistently". Two
// measurements need two different rules: with `-M a:=b b:=q` and `-a v -b w`
// the array `q` comes back `(-a w)` — the *first* option's name beside the
// *second* option's value — while the manual's own `-A bar -M a=foo b+: c:=b`
// example keys the association by the *target* description's name. A parser
// that picked either rule would bind the other case wrongly at status 0,
// which is exactly the failure this builtin can cause and the reason to say
// no. Nothing in the plugin manager this work is aimed at uses it.
const (
	zparseoptsLetters       = "DEFKaA"
	zparseoptsUnimplemented = "M"
)

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
	return zparseoptsRun(r, opts, specs)
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
		case strings.IndexByte(zparseoptsUnimplemented, letter) >= 0:
			r.Diagnosef("-%c is not implemented yet\n", letter)
			return opts, nil, 1
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
		if spec.array == "" && opts.array == "" && !opts.hasAssoc {
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
		if spec.array != "" && !isIdentifier(spec.array) {
			// Real zsh's own assignment machinery complains here — `not an
			// identifier: 1bad`, located as the shell rather than as this
			// builtin — and then *aborts* a non-interactive shell. The
			// sentence is reproduced and the fatality is not: see the
			// pull request.
			r.DiagnoseAsTheShellf("not an identifier: %s\n", spec.array)
			return nil, 1
		}
		specs = append(specs, spec)
	}
	return specs, 0
}

// parseZparseoptsSpec reads one description.
func parseZparseoptsSpec(word string) (zparseoptsSpec, bool) {
	var spec zparseoptsSpec
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
		return zparseoptsSpec{}, true
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
	spec.array = word[i+1:]
	return spec, true
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
type zparseoptsMatch struct {
	spec int
	// elems is what goes into the array: the option, and its argument either
	// beside it or joined to it.
	elems []string
	// value is what goes into the association, which is the argument alone.
	value string
}

// zparseoptsRun is the parse itself, and it is deliberately in two halves:
// everything is worked out against a copy first and written only if the whole
// command line parsed. That is what `-F` means — "removal and extraction are
// not performed, and option arrays are not updated" — and it is measured for
// a missing argument as well, which fails the same way with or without the
// letter.
func zparseoptsRun(r *interp.Runner, opts zparseoptsOpts, specs []zparseoptsSpec) int {
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
	zparseoptsStore(r, opts, specs, matches)
	if opts.remove {
		kept := make([]string, 0, len(params))
		for j, p := range params {
			if !consumed[j] {
				kept = append(kept, p)
			}
		}
		r.Params = kept
	}
	return 0
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
			out = append(out, zparseoptsMatch{spec: idx, elems: []string{spec.option()}})
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
		out = append(out, zparseoptsElems(idx, spec, arg, has))
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

// zparseoptsElems lays one match out the way its description says to.
func zparseoptsElems(idx int, spec zparseoptsSpec, arg string, has bool) zparseoptsMatch {
	switch {
	case !has:
		return zparseoptsMatch{spec: idx, elems: []string{spec.option()}}
	case spec.arg.joinsArgument():
		return zparseoptsMatch{spec: idx, elems: []string{spec.option() + arg}, value: arg}
	default:
		return zparseoptsMatch{spec: idx, elems: []string{spec.option(), arg}, value: arg}
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
// The order is the command line's and the pruning is per description: without
// `+` only a description's last appearance survives, and several descriptions
// may share one array — `-a arr a b` against `-a -b` is `(-a -b)`, which a
// per-array "keep the last" would have cut to one.
func zparseoptsStore(
	r *interp.Runner, opts zparseoptsOpts, specs []zparseoptsSpec, matches []zparseoptsMatch,
) {
	last := make(map[int]int, len(specs))
	for i, m := range matches {
		last[m.spec] = i
	}
	slots := map[string][]string{}
	used := map[string]bool{}
	assoc := map[string]string{}
	for i, m := range matches {
		spec := specs[m.spec]
		if !spec.plus && last[m.spec] != i {
			continue
		}
		name := spec.array
		if name == "" {
			name = opts.array
		}
		if name != "" {
			slots[name] = append(slots[name], m.elems...)
			used[name] = true
		}
		if spec.plus {
			assoc[spec.option()] += m.value
			continue
		}
		assoc[spec.option()] = m.value
	}
	// Which arrays are cleared is the whole of `-K`: without it every array
	// a description names is replaced, matched or not; with it one whose
	// descriptions never matched keeps what it held.
	for _, s := range specs {
		name := s.array
		if name == "" {
			name = opts.array
		}
		if name == "" || used[name] {
			continue
		}
		if !opts.keep {
			r.SetArray(name, nil)
		}
	}
	for name, elems := range slots {
		r.SetArray(name, elems)
	}
	if !opts.hasAssoc {
		return
	}
	table := map[string]string{}
	if opts.keep {
		// `-K` preserves the individual elements of an association, where
		// for an array it preserves the whole thing or none of it. Measured
		// both ways.
		if existing, ok := r.GetAssoc(opts.assoc); ok {
			table = existing
		}
	}
	for k, v := range assoc {
		table[k] = v
	}
	r.SetAssoc(opts.assoc, table)
}
