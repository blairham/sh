// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"slices"
	"strings"

	"github.com/blairham/sh/interp"
)

// `compfiles`: the four things `_path_files` asks of `zsh/computil`.
//
// The manual says it "is used by the _path_files function to optimize complex
// recursive filename generation (globbing)": with `-p` and `-P` it builds the
// glob patterns, folding in the path already handled and the word on the
// line; `-i` tests directories for the `ignore-parents` style; and `-r`
// narrows the matches when a component of them is what is on the line.
//
// This used to build no pattern at all, on the reading that the whole of it
// was an optimisation and a caller left with its own array was answered
// correctly. That was measured with an empty word on the line, where the
// pattern really is `*` and the reading holds. With a word on the line it
// does not: the array `_path_files` hands in holds the directory already
// handled — `''` for the working directory — and **the pattern is what comes
// back**, so an array left alone globbed nothing and every file completion
// through the shipped `_files` answered "no matches" (#6128's survey, measured
// 2026-10-05 against zsh 5.9.2). The rules below are each measured the same
// way, from a completion widget calling the builtin with chosen arguments.
//
// # `-p` and `-P`
//
//	compfiles -p  array accex skipped matcher sdirs fake pattern
//	compfiles -P  array accex skipped matcher sdirs fake
//
// Each element of the array becomes itself, glob-quoted, then `skipped`, then
// a pattern for the word on the line (`$PREFIX`), then the file pattern — or
// `*(-/)`, directories, for `-P`:
//
//	PREFIX=RE, array ('' d/), -p ... '*'      (RE* d/RE*)
//	the same, -P                              (RE*(-/) d/RE*(-/))
//	skipped '/', array (docs)                 (docs/RE*)
//	pattern '*.md'                            (RE*.md)
//	-p- and -P-                               (* d/*), (*(-/) d/*(-/))
//
// The word is unquoted and glob-quoted: `a*b` is `a\*b*`, and of the
// characters a pattern reads, `* ? [ ] ( ) | ~ # ^ < > =` and the backslash
// are quoted and a blank is not. An element is glob-quoted the same way.
//
// **The match specification decides how much of the word is kept**, because
// a word the specification lets match other characters cannot be a literal:
//
//	(blank)                         RE*
//	m:{a-zA-Z}={A-Za-z}             [Rr][Ee]*     each character with its partner
//	m:{a-z}={A-Z}                   RE*           nothing in RE is on the left
//	m:{a-z}={A-Z}, word re          [rR][eE]*
//	the same spec twice, word re    *             two specs at a character: none
//	r:|[._-]=* r:|=*                RE*           no anchor in the word
//	r:|[._-]=*, word a.b            a*.b*         anything before an anchor
//	l:|=* r:|=*                     *             anything before the start
//	b:a=*                           *
//
// A reading this does not model gives `*`, which the measured `l:` and `b:`
// rows show is an answer zsh gives too: every name, narrowed afterwards by the
// matching `compadd` does anyway. It is a wider glob and never a wrong one.
//
// # `-r`
//
// `compfiles -r array word` narrows by the **first component** of the word,
// and only once that component is finished — the word has a slash in it:
//
//	(ab/x ac/x)    ab/x       0, (ab/x)
//	(ab/x ac/x)    a/x        1, unchanged — nothing has that component
//	(a/b/c a/bb/c) a/b/c      0, unchanged — only the first component counts
//	(a ab)         a          1, unchanged — no slash: more than one match
//	(ab)           a          0, unchanged — no slash: one match or none
//
// # `-i`
//
// 1, "nothing was ignored": the `ignore-parents` style is not read here.

func compfilesBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if !compArity(r, args, 1, -1) {
		return 1
	}
	if _, _, ok := computilFrom(r, ctx); !ok {
		return 1
	}
	switch args[0] {
	case "-p", "-p-", "-P", "-P-":
		return compfilesPatterns(r, args)
	case "-r":
		return compfilesNarrow(r, args)
	case "-i":
		return 1
	}
	r.Diagnosef("invalid option: %s\n", args[0])
	return 1
}

// compfilesPatterns is `-p` and `-P`, with or without the trailing `-` that
// leaves the word out.
func compfilesPatterns(r *interp.Runner, args []string) int {
	dirs := strings.HasPrefix(args[0], "-P")
	want := 8
	if dirs {
		want = 7
	}
	if len(args) < want {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	name, skipped, matcher := args[1], args[3], args[4]
	tail := "*(-/)"
	if !dirs {
		tail = args[7]
	}
	word := ""
	if !strings.HasSuffix(args[0], "-") {
		prefix, _ := r.GetVar("PREFIX")
		word = wordPattern(unquoteBackslashes(prefix), matcher)
	}
	elems, _ := r.GetArray(name)
	out := make([]string, len(elems))
	for i, e := range elems {
		out[i] = globQuote(e) + skipped + word + tail
	}
	r.SetArray(name, out)
	return 0
}

// compfilesNarrow is `-r`. See the file comment for the rule.
func compfilesNarrow(r *interp.Runner, args []string) int {
	if len(args) < 3 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	name, word := args[1], args[2]
	elems, _ := r.GetArray(name)
	first, _, finished := strings.Cut(word, "/")
	if !finished {
		if len(elems) > 1 {
			return 1
		}
		return 0
	}
	var kept []string
	for _, e := range elems {
		component, _, _ := strings.Cut(e, "/")
		if component == first {
			kept = append(kept, e)
		}
	}
	if len(kept) == 0 {
		return 1
	}
	if len(kept) < len(elems) {
		r.SetArray(name, kept)
	}
	return 0
}

// globSpecials are the characters a glob reads, which a literal in a pattern
// has to have quoted. A blank is not among them: the pattern is used as a
// pattern, not split.
const globSpecials = "*?[]()|~#^<>=\\"

func globQuote(s string) string {
	if !strings.ContainsAny(s, globSpecials) {
		return s
	}
	var b strings.Builder
	for _, c := range s {
		if strings.ContainsRune(globSpecials, c) {
			b.WriteByte('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// unquoteBackslashes takes the backslashes off the word as it was typed, which
// is the form `$PREFIX` holds it in: `a\ b` is the name `a b`.
func unquoteBackslashes(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	escaped := false
	for _, c := range s {
		if c == '\\' && !escaped {
			escaped = true
			continue
		}
		escaped = false
		b.WriteRune(c)
	}
	return b.String()
}

// wordPattern is the pattern for the word on the line under a match
// specification, or "" where the specification lets it match anything — see
// the table in the file comment.
func wordPattern(word, matcher string) string {
	if word == "" {
		return ""
	}
	runes := []rune(word)
	// classes[i] is what character i may be besides itself; anchors[i] that
	// anything may come before it.
	classes := make([][]rune, len(runes))
	affected := make([]int, len(runes))
	anchors := make([]bool, len(runes))
	for _, spec := range strings.Fields(matcher) {
		kind, rest, ok := strings.Cut(spec, ":")
		if !ok {
			return ""
		}
		switch kind {
		case "m", "M":
			line, other, ok := strings.Cut(rest, "=")
			if !ok {
				return ""
			}
			from, okFrom := matcherChars(line)
			to, okTo := matcherChars(other)
			if !okFrom || !okTo || len(from) != len(to) {
				return ""
			}
			for i, c := range runes {
				if j := slices.Index(from, c); j >= 0 && to[j] != c {
					classes[i] = append(classes[i], to[j])
					affected[i]++
				}
			}
		case "r":
			// r:|anchor=* — anything may come before an anchor character.
			// An empty anchor is the end of the word, which is after it.
			left, right, ok := strings.Cut(rest, "|")
			if !ok || left != "" {
				return ""
			}
			anchor, pattern, ok := strings.Cut(right, "=")
			if !ok || pattern != "*" {
				return ""
			}
			if anchor == "" {
				continue
			}
			set, ok := anchorSet(anchor)
			if !ok {
				return ""
			}
			for i, c := range runes {
				if i > 0 && slices.Contains(set, c) {
					anchors[i] = true
				}
			}
		default:
			// `l:`, `b:`, `e:` and their capitals reach the start of the word
			// or both ends, which leaves no literal to keep.
			return ""
		}
	}
	var b strings.Builder
	for i, c := range runes {
		if affected[i] > 1 {
			return ""
		}
		if anchors[i] {
			b.WriteByte('*')
		}
		if len(classes[i]) == 0 {
			b.WriteString(globQuote(string(c)))
			continue
		}
		set := append([]rune{c}, classes[i]...)
		for _, m := range set {
			if strings.ContainsRune("]^-!\\", m) {
				return ""
			}
		}
		b.WriteByte('[')
		b.WriteString(string(set))
		b.WriteByte(']')
	}
	return b.String()
}

// matcherChars is one side of an `m:` specification as the characters it
// lists in order: `{a-zA-Z}` is the 52 letters, and a bare character is
// itself. A `[...]` class, which pairs any of its characters with any of the
// other side's, is not modeled.
func matcherChars(s string) ([]rune, bool) {
	if inner, ok := strings.CutPrefix(s, "{"); ok {
		inner, ok = strings.CutSuffix(inner, "}")
		if !ok {
			return nil, false
		}
		return expandRanges([]rune(inner)), true
	}
	runes := []rune(s)
	if len(runes) != 1 || strings.ContainsRune("[]?*", runes[0]) {
		return nil, false
	}
	return runes, true
}

// anchorSet is an `r:` anchor's characters: a `[...]` class or one character.
func anchorSet(s string) ([]rune, bool) {
	if inner, ok := strings.CutPrefix(s, "["); ok {
		inner, ok = strings.CutSuffix(inner, "]")
		if !ok || strings.HasPrefix(inner, "^") || strings.HasPrefix(inner, "!") {
			return nil, false
		}
		return expandRanges([]rune(inner)), true
	}
	runes := []rune(s)
	if len(runes) != 1 || strings.ContainsRune("?*", runes[0]) {
		return nil, false
	}
	return runes, true
}

// expandRanges spells out `a-z` inside a class; a `-` first or last is itself.
func expandRanges(in []rune) []rune {
	var out []rune
	for i := 0; i < len(in); i++ {
		if i+2 < len(in) && in[i+1] == '-' && in[i] <= in[i+2] {
			for c := in[i]; c <= in[i+2]; c++ {
				out = append(out, c)
			}
			i += 2
			continue
		}
		out = append(out, in[i])
	}
	return out
}
