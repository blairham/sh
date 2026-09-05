// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// `zstyle` is this shell's styles database: a table of (pattern, style,
// values) that anything wanting a per-context setting reads. The completion
// system is its largest reader, and this shell has no completion system — but
// a real rc file sets twenty styles before it does anything else, and a
// database that refuses to store what it is given fails every line of that
// file rather than the one feature behind it. So this is the store and the
// lookup, complete, with nothing reading it yet.
//
// Measured 2026-09-05 against zsh 5.9.2 in the oracle environment, with a
// scratch HOME and no startup files.
//
// The one part worth getting exactly right is the order, because it is what
// "most specific pattern wins" means and both the listing and every lookup
// read it. Three keys, measured by inserting the same set in several orders:
//
//  1. **More colon-separated components first.** `:a:b:c:d:*` precedes
//     `:m:n:*` precedes `:q:*`, whichever order they were set in, and
//     `abcdefgh*` — one component — comes after `:a:*` despite being longer.
//     It is the component count and not the length of the literal part:
//     `:aaaa:*` and `:b:*` tie and keep the order they were given.
//  2. **Within a component count, a pattern with no metacharacter first.**
//     `:a:b:c` precedes `:a:b:*`; `ab` precedes `a*b` even when set after it.
//     This is subordinate to the first key rather than above it: `:x:y:z:*`
//     still precedes the exact `:a:b:c`, measured both ways round.
//  3. **Otherwise the order they were set in**, which is why the storage
//     below keeps insertion order and sorts on the way out.
//
// A lookup walks that same order and takes the first pattern that matches the
// context, so `:a:b:*` beats `:a:*` for `:a:b:c` however they were set. `*`
// sorts last by the first key and is therefore the fallback, which is what
// makes `zstyle '*' …` mean what people write it to mean.
//
// The metacharacter set for the second key is measured rather than assumed,
// one punctuation character at a time: `*`, `?`, `[`, `(`, `|`, `#`, `^` and
// `<` make a pattern non-exact, and — the surprising one — `~` does not,
// though it is a pattern character in this shell and is quoted when the
// listing prints it. Quoting and specificity are two different questions here
// and the sets differ; see quoteStyleWord.

// zstyleStore is where the table lives: an indexed array under a name no
// script can reach, the way `emulate` keeps its mode.
//
// In the Runner's own table rather than in a package variable, because that
// is what makes a subshell keep its own — measured, `(zstyle -d)` leaves the
// parent's styles alone, and a package variable would be shared across the
// clone.
const zstyleStore = ".zsh.zstyle"

// styleEntry is one row of the database.
type styleEntry struct {
	pattern string
	style   string
	// eval is `-e`: the values are shell code that sets `reply`, run at
	// lookup rather than stored as an answer.
	eval   bool
	values []string
}

// readStyles is the table, in the order it was set.
//
// The encoding is flat and length-prefixed — pattern, style, kind, count,
// then the values — so nothing has to be escaped and a value may hold any
// character a word can hold, separators and newlines included.
func readStyles(r *interp.Runner) []styleEntry {
	flat, _ := r.GetArray(zstyleStore)
	var out []styleEntry
	for i := 0; i+4 <= len(flat); {
		e := styleEntry{pattern: flat[i], style: flat[i+1], eval: flat[i+2] == "e"}
		n, err := strconv.Atoi(flat[i+3])
		if err != nil || n < 0 {
			break
		}
		i += 4
		if i+n > len(flat) {
			break
		}
		e.values = append(e.values, flat[i:i+n]...)
		i += n
		out = append(out, e)
	}
	return out
}

// writeStyles puts the table back. An emptied table becomes an empty array,
// which reads back as no entries.
func writeStyles(r *interp.Runner, entries []styleEntry) {
	var flat []string
	for _, e := range entries {
		kind := "v"
		if e.eval {
			kind = "e"
		}
		flat = append(flat, e.pattern, e.style, kind, strconv.Itoa(len(e.values)))
		flat = append(flat, e.values...)
	}
	r.SetArray(zstyleStore, flat)
}

// styleMetacharacters are what make a pattern non-exact for the ordering.
//
// Measured a character at a time; `~` is deliberately absent, because a
// pattern holding one sorts with the exact ones even though this shell treats
// it as a pattern character elsewhere.
const styleMetacharacters = `*?[(|#^<`

// isExactPattern reports whether a pattern has no metacharacter, which is the
// second sort key.
func isExactPattern(p string) bool {
	return !strings.ContainsAny(p, styleMetacharacters)
}

// styleOrder sorts a copy of the table into the order this shell lists and
// looks styles up in. Stable, because the third key is the order they arrived
// in and the caller passes them in that order.
func styleOrder(entries []styleEntry) []styleEntry {
	out := append([]styleEntry(nil), entries...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].style != out[j].style {
			return out[i].style < out[j].style
		}
		ci := strings.Count(out[i].pattern, ":")
		cj := strings.Count(out[j].pattern, ":")
		if ci != cj {
			return ci > cj
		}
		ei, ej := isExactPattern(out[i].pattern), isExactPattern(out[j].pattern)
		if ei != ej {
			return ei
		}
		return false
	})
	return out
}

// lookupStyle is the values set for a style in a context, and whether any
// pattern claimed it.
//
// The first match in the sorted order wins, which is the whole of "most
// specific pattern wins": the order already says which of two patterns is
// more specific, so the search does not have to.
func lookupStyle(r *interp.Runner, ctx context.Context, context, style string) ([]string, bool) {
	for _, e := range styleOrder(readStyles(r)) {
		if e.style != style || !r.MatchPattern(e.pattern, context) {
			continue
		}
		if e.eval {
			return evaluatedStyle(r, ctx, e), true
		}
		return e.values, true
	}
	return nil, false
}

// evaluatedStyle runs an `-e` style's code and reads `reply` back.
//
// The code is a script's, so it is run through `eval` the way `emulate -c`
// runs its own: the dialect does not parse, it asks the shell to.
func evaluatedStyle(r *interp.Runner, ctx context.Context, e styleEntry) []string {
	eval, ok := r.Builtin("eval")
	if !ok {
		return nil
	}
	before, had := r.GetArray("reply")
	delete(r.Arrays, "reply")
	_ = eval(r, ctx, []string{strings.Join(e.values, " ")})
	out, _ := r.GetArray("reply")
	delete(r.Arrays, "reply")
	if had {
		r.SetArray("reply", before)
	}
	return out
}

// setStyle adds or replaces one row, keeping the position of a row it
// replaces — which is what makes re-setting a style leave the listing where
// it was rather than moving it to the end.
func setStyle(r *interp.Runner, e styleEntry) {
	entries := readStyles(r)
	for i := range entries {
		if entries[i].pattern == e.pattern && entries[i].style == e.style {
			entries[i] = e
			writeStyles(r, entries)
			return
		}
	}
	writeStyles(r, append(entries, e))
}

// registerZstyle installs the builtin.
func registerZstyle(r *interp.Runner) {
	r.Register("zstyle", zstyleBuiltin)
}

// zstyleLetters are the option letters this builtin answers to. Anything else
// is `invalid option: -z` and 1 — which is this builtin's wording and not
// `bindkey`'s `bad option`, measured, so the two are refused in their own
// words.
const zstyleLetters = "LdgsbatTme"

func zstyleBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		listStyles(r)
		return 0
	}
	mode := ""
	if a := args[0]; strings.HasPrefix(a, "-") && len(a) == 2 {
		if !strings.ContainsAny(a[1:], zstyleLetters) {
			r.Diagnosef("invalid option: %s\n", a)
			return 1
		}
		mode, args = a[1:], args[1:]
	}
	switch mode {
	case "":
		return setStyleCommand(r, args, false)
	case "e":
		return setStyleCommand(r, args, true)
	case "L":
		return listStyleCommands(r, args)
	case "d":
		return deleteStyles(r, args)
	case "g":
		return retrieveStyles(r, args)
	default:
		return testStyle(r, ctx, mode, args)
	}
}

// setStyleCommand is the bare form and `-e`: a pattern, a style and its
// values. Two arguments are the fewest that say anything — `zstyle ':a:*'`
// alone is `not enough arguments`, measured — and a style may be set to no
// values at all.
func setStyleCommand(r *interp.Runner, args []string, eval bool) int {
	if len(args) < 2 {
		return notEnoughStyleArguments(r)
	}
	setStyle(r, styleEntry{pattern: args[0], style: args[1], eval: eval, values: args[2:]})
	return 0
}

// deleteStyles is `-d`. With nothing it empties the table, with a pattern it
// drops every style set for it, and with styles after the pattern it drops
// those. Always 0, even for a pattern nothing was ever set for — measured.
func deleteStyles(r *interp.Runner, args []string) int {
	if len(args) == 0 {
		writeStyles(r, nil)
		return 0
	}
	pattern, styles := args[0], args[1:]
	entries := readStyles(r)
	kept := entries[:0:0]
	for _, e := range entries {
		if e.pattern == pattern && (len(styles) == 0 || containsWord(styles, e.style)) {
			continue
		}
		kept = append(kept, e)
	}
	writeStyles(r, kept)
	return 0
}

// retrieveStyles is `-g`, which is the one form that reads a pattern rather
// than matching against it: `zstyle -g out ':a:b' v` finds nothing when the
// style was set for `:a:*`, measured, where every other retrieval would.
//
// With no pattern it answers with the patterns themselves, which is how a
// script asks what contexts have been styled at all.
func retrieveStyles(r *interp.Runner, args []string) int {
	if len(args) == 0 {
		return notEnoughStyleArguments(r)
	}
	name, rest := args[0], args[1:]
	switch len(rest) {
	case 0:
		var seen []string
		for _, e := range styleOrder(readStyles(r)) {
			if !containsWord(seen, e.pattern) {
				seen = append(seen, e.pattern)
			}
		}
		setStyleArray(r, name, seen)
		return 0
	case 1:
		var seen []string
		for _, e := range styleOrder(readStyles(r)) {
			if e.pattern == rest[0] && !containsWord(seen, e.style) {
				seen = append(seen, e.style)
			}
		}
		setStyleArray(r, name, seen)
		return 0
	default:
		for _, e := range styleOrder(readStyles(r)) {
			if e.pattern == rest[0] && e.style == rest[1] {
				setStyleArray(r, name, e.values)
				return 0
			}
		}
		return 1
	}
}

// testStyle is every form that looks a style up in a context: `-s`, `-b`,
// `-a`, `-t`, `-T` and `-m`.
//
// They differ in what they do with the values and in what a style nobody set
// means. `-t` is the one with three statuses — 0 true, 1 false and **2 not
// set at all**, measured — because a completion function has to be able to
// tell "off" from "unsaid", and `-T` is the same test with the unsaid case
// answering 0.
func testStyle(r *interp.Runner, ctx context.Context, mode string, args []string) int {
	need := 3
	if mode == "t" || mode == "T" {
		need = 2
	}
	if len(args) < need {
		return notEnoughStyleArguments(r)
	}
	values, found := lookupStyle(r, ctx, args[0], args[1])
	switch mode {
	case "s":
		sep := " "
		if len(args) > 3 {
			sep = args[3]
		}
		if found {
			r.SetVar(args[2], strings.Join(values, sep))
		}
	case "a":
		if found {
			setStyleArray(r, args[2], values)
		}
	case "b":
		// Measured: the variable is set either way, and a style nobody set
		// leaves it `no` and reports 1.
		r.SetVar(args[2], styleBoolWord(found && truthyStyle(values, nil)))
	case "m":
		return styleStatus(found && len(values) > 0 && r.MatchPattern(args[2], values[0]))
	case "t", "T":
		if !found {
			if mode == "T" {
				return 0
			}
			return 2
		}
		return styleStatus(truthyStyle(values, args[2:]))
	}
	return styleStatus(found)
}

// truthyStyle is what `-t` and `-b` call true.
//
// With no words to test against it is the four spellings measured — `true`,
// `yes`, `on` and `1`, and nothing else, so `random` is false rather than
// "set". With words, it is whether the style's first value is one of them.
func truthyStyle(values, against []string) bool {
	if len(values) == 0 {
		return false
	}
	if len(against) > 0 {
		return containsWord(against, values[0])
	}
	switch values[0] {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}

// listStyles is the bare command: each style once, then the patterns it was
// set for indented under it with their values.
//
// The styles come out in alphabetical order, which is measured rather than
// incidental — three set as s3, s1, s2 list as s1, s2, s3 — and the patterns
// under each are in the specificity order the rest of this file is about.
func listStyles(r *interp.Runner) {
	entries := styleOrder(readStyles(r))
	var styles []string
	for _, e := range entries {
		if !containsWord(styles, e.style) {
			styles = append(styles, e.style)
		}
	}
	sort.Strings(styles)
	for _, s := range styles {
		_, _ = fmt.Fprintf(r.Out(), "%s\n", s)
		for _, e := range entries {
			if e.style != s {
				continue
			}
			_, _ = fmt.Fprintf(r.Out(), "        %s\n", strings.Join(append([]string{e.pattern}, e.values...), " "))
		}
	}
}

// listStyleCommands is `-L`: the table written back as the commands that
// would set it, which is what a person types to see what a framework did.
//
// A pattern and a style may narrow it, and neither is matched as a pattern —
// `zstyle -L ':nomatch:*'` is silence and 0, not an error.
func listStyleCommands(r *interp.Runner, args []string) int {
	for _, e := range styleOrder(readStyles(r)) {
		if len(args) > 0 && e.pattern != args[0] {
			continue
		}
		if len(args) > 1 && e.style != args[1] {
			continue
		}
		words := []string{"zstyle"}
		if e.eval {
			words = append(words, "-e")
		}
		words = append(words, quoteStyleWord(e.pattern), e.style)
		for _, v := range e.values {
			words = append(words, quoteStyleWord(v))
		}
		_, _ = fmt.Fprintf(r.Out(), "%s\n", strings.Join(words, " "))
	}
	return 0
}

// styleSafe are the characters a word may be made of and still be printed
// bare by `-L`. Measured one character at a time, and it is not the same set
// as the metacharacters the ordering asks about: `~` and `=` are quoted here
// and sort as exact there, `!` and `%` are bare here.
const styleSafe = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_./:,+@%!-"

// quoteStyleWord spells one word the way `-L` does: bare when it can be, and
// otherwise in single quotes with any quote of its own broken out.
func quoteStyleWord(s string) string {
	if s != "" && strings.Trim(s, styleSafe) == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// setStyleArray puts a list into an indexed array, which is what `-g` and
// `-a` answer with.
//
// Through the core's own setter rather than into the map, because the
// subscript the first element answers to is a dialect's answer and not this
// file's: writing at 0 under this shell's one-based arrays reads back empty.
func setStyleArray(r *interp.Runner, name string, values []string) {
	r.SetArray(name, values)
}

// notEnoughStyleArguments is this builtin's one usage complaint. Measured: it
// is the same sentence whatever was short, and it is 1 rather than 2.
func notEnoughStyleArguments(r *interp.Runner) int {
	r.Diagnosef("not enough arguments\n")
	return 1
}

func containsWord(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

func styleStatus(ok bool) int {
	if ok {
		return 0
	}
	return 1
}

func styleBoolWord(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}
