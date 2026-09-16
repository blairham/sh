// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// `compadd`: the builtin a completion function offers a candidate with.
//
// # Measured on zsh 5.9.2, 2026-09-15
//
// Through a pseudo-terminal, from inside a `zle -C` widget's function, with
// `git che` typed — so `PREFIX` is `che` — and the status and
// `$compstate[nmatches]` printed after each call:
//
//	compadd checkout cherry commit     0, nmatches 2, lists `checkout cherry`
//	compadd -P XX -- checkout cherry   0, nmatches 2, line becomes `XXche`
//	compadd -S = -- checkout           0, nmatches 1, line becomes `checkout=`
//	compadd -U -- zzz yyy              0, nmatches 2
//	compadd -- 'a b'                   1, nmatches 0
//	compadd -Q -- 'a b'                1, nmatches 0
//	compadd -p HID -- checkout         1, nmatches 0
//	compadd -d '(one two)' -- checkout cherry   0, lists `one two`
//
// and outside a completion function, on a `-c` line with `zmodload
// zsh/complete` first:
//
//	compadd x       can only be called from completion function, 1
//
// Four rules come out of that, and three of them are ones a reading of the
// manual gets wrong in the same direction:
//
//  1. **The candidates are filtered against `$PREFIX` as they are added.**
//     `commit` never becomes a match, and `nmatches` says 2 rather than 3. So
//     the status is "did anything match", not "was anything handed over".
//  2. **`-P` is not part of the match and `-p` is.** `-P XX` still matched
//     `checkout` against `che`; `-p HID` did not, because the candidate being
//     matched was `HIDcheckout`. Both are inserted. That is the whole of the
//     difference between the two, and it is the one worth measuring because
//     the names do not carry it.
//  3. **`-U` turns the filtering off** and nothing else: `zzz` and `yyy` are
//     both matches against a `che` that neither begins with.
//  4. **A match is quoted for the line unless `-Q`**, which is why `'a b'`
//     would have gone in as `a\ b` had it matched at all.
//
// # What this does not do
//
// **Descriptions are dropped.** `-d`, `-X` and `-x` are read and their
// argument consumed, and the listing this editor draws is names only — repl's
// completion seam is answered with replacement words and has nowhere to put a
// description. That is the visible difference between a listing here and
// zsh's `checkout -- checkout branch or paths to working tree`, and it is
// #3041 rather than this file.
//
// **Grouping, menus and match specifications are read and ignored.** `-J`,
// `-V`, `-1`, `-2`, `-o` and `-M` name behavior this editor has not got —
// there is no menu completion here and no second sort order — so they are
// consumed rather than refused, for compctl.go's reason: a builtin that
// refused every call naming one would stop functions that are otherwise
// entirely servable.

func registerCompadd(r *interp.Runner) { r.Register("compadd", compaddBuiltin) }

// compaddArgumentOptions are the letters that take a following word, so that
// what follows one is never mistaken for a candidate. Getting this list short
// is how `compadd -J group -- x` comes to offer `group` as a completion.
const compaddArgumentOptions = "PSpsiIWdJVXxrRDOAFMEy"

// compaddFlagOptions are the letters that stand alone and may cluster.
const compaddFlagOptions = "akqQfenUl12CTuzo"

// compaddOptions is one call's letters, gathered.
type compaddOptions struct {
	prefix, suffix       string // -P, -S: inserted, not matched
	hiddenPre, hiddenSuf string // -p, -s: inserted *and* matched
	arrays               bool   // -a: the words name arrays
	keys                 bool   // -k: the words name associations
	unfiltered           bool   // -U: do not match against PREFIX
	raw                  bool   // -Q: insert the candidate unquoted
	into                 string // -O or -A: store rather than offer
	withhold             bool   // -O, -A or -D: add nothing
}

func compaddBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	cs, completing := completionFrom(ctx)
	if !completing {
		r.Diagnosef("can only be called from completion function\n")
		return 1
	}
	opts, rest, ok := compaddParse(r, args)
	if !ok {
		return 1
	}
	candidates := compaddCandidates(r, opts, rest)
	added := cs.add(r, opts, candidates)
	if added == 0 {
		return 1
	}
	return 0
}

// compaddParse reads the letters off the front, stopping at `--`, at a word
// that is not an option, and at a `-` on its own — which is a candidate.
func compaddParse(r *interp.Runner, args []string) (compaddOptions, []string, bool) {
	var o compaddOptions
	i := 0
	for ; i < len(args); i++ {
		word := args[i]
		if word == "--" {
			i++
			break
		}
		if len(word) < 2 || word[0] != '-' {
			break
		}
		letters := word[1:]
		for j := 0; j < len(letters); j++ {
			letter := letters[j]
			switch {
			case strings.IndexByte(compaddFlagOptions, letter) >= 0:
				compaddFlag(&o, letter)
			case strings.IndexByte(compaddArgumentOptions, letter) >= 0:
				// The rest of the word if there is any, and the next word
				// otherwise — the two spellings every letter here accepts.
				value := letters[j+1:]
				if value == "" {
					if i+1 >= len(args) {
						r.Diagnosef("argument expected after -%c\n", letter)
						return o, nil, false
					}
					i++
					value = args[i]
				}
				compaddArgument(&o, letter, value)
				j = len(letters)
			default:
				r.Diagnosef("bad option: -%c\n", letter)
				return o, nil, false
			}
		}
	}
	return o, args[i:], true
}

func compaddFlag(o *compaddOptions, letter byte) {
	switch letter {
	case 'a':
		o.arrays = true
	case 'k':
		o.keys = true
	case 'U':
		o.unfiltered = true
	case 'Q':
		o.raw = true
	}
}

func compaddArgument(o *compaddOptions, letter byte, value string) {
	switch letter {
	case 'P':
		o.prefix = value
	case 'S':
		o.suffix = value
	case 'p':
		o.hiddenPre = value
	case 's':
		o.hiddenSuf = value
	case 'O', 'A':
		o.into, o.withhold = value, true
	case 'D':
		o.withhold = true
	}
}

// compaddCandidates is the words themselves, which are the arguments unless
// `-a` or `-k` said they are the names of parameters holding them.
//
// `-k` takes the *keys* of an association and the elements of an ordinary
// array, which is what makes it usable against either — a completion function
// writes `compadd -k mymap` without checking which it has.
func compaddCandidates(r *interp.Runner, o compaddOptions, words []string) []string {
	if !o.arrays && !o.keys {
		return words
	}
	var out []string
	for _, name := range words {
		if o.keys {
			if assoc, ok := r.GetAssoc(name); ok {
				for key := range assoc {
					out = append(out, key)
				}
				continue
			}
		}
		if values, ok := r.GetArray(name); ok {
			out = append(out, values...)
			continue
		}
		if value, ok := r.GetVar(name); ok && value != "" {
			out = append(out, value)
		}
	}
	return out
}

// add takes the candidates that match, and answers how many did.
//
// The count is of candidates that *matched* and not of ones offered, which is
// the measured rule and is also what `-O` needs: a call that stores rather
// than offers still reports whether anything matched.
func (cs *completionState) add(r *interp.Runner, o compaddOptions, candidates []string) int {
	var matched []string
	for _, candidate := range candidates {
		// What is matched is the hidden prefix and suffix around the
		// candidate; what `-P` and `-S` add is not part of it. Measured —
		// see the file comment.
		subject := o.hiddenPre + candidate + o.hiddenSuf
		if !o.unfiltered && !strings.HasPrefix(subject, cs.prefix) {
			continue
		}
		matched = append(matched, subject)
		if !o.withhold {
			cs.offer(subject, o.prefix, o.suffix, o.raw)
		}
	}
	if o.into != "" {
		r.SetArray(o.into, matched)
	}
	cs.state["nmatches"] = strconv.Itoa(len(cs.matches))
	return len(matched)
}

// offer writes one whole replacement word, quoted for the line unless the
// caller asked for it raw.
//
// Whole, because that is what repl's completion seam is answered with: a
// completer returns replacement words and not suffixes, so everything already
// on the line that this completion keeps — the opening quote, and whatever
// `compset` moved into `IPREFIX` — has to be written out again here.
//
// `-P` and `-S` go on unescaped and the candidate between them does not. They
// are delimiters a completion function chose — `=`, `/`, `:` — and escaping
// them would insert a backslash the function did not ask for; the candidate is
// a name and has to survive being read back by the parser.
//
// Duplicates are dropped rather than offered twice, which is what a listing
// with one entry per name needs and what makes a second Tab fill in from a
// set rather than from a bag.
func (cs *completionState) offer(subject, pre, suf string, raw bool) {
	body := cs.iprefix + pre + subject
	word := cs.c.Escape(body) + suf
	if raw {
		word = cs.qiprefix + body + suf
	}
	for _, have := range cs.matches {
		if have == word {
			return
		}
	}
	cs.matches = append(cs.matches, word)
}
