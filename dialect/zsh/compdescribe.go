// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strings"

	"github.com/blairham/sh/interp"
)

// `compdescribe`: the two-pass name-and-description builder `_describe` and
// `_arguments` hand their matches to.
//
// # The two calls, measured
//
// Traced against zsh 5.9.2 on 2026-09-15 — see computil.go. `uname -` sends
// one definition call and then reads groups back until there are none:
//
//	compdescribe -I '' 40 '-- ' _expl -g _a_11 -M 'r:|[_-]=* r:|=*' \
//	             -- _a_15 -S '' -M … -- _a_111 '-qS=' -M …          → 0
//	compdescribe -g csl2 _args _tmpm _tmpd                          → 0
//	compdescribe -g csl2 _args _tmpm _tmpd                          → 1
//
// and the caller then writes `compadd "$_args[@]" -d _tmpd -a _tmpm`.
//
// The definition call is `-i` or `-I`, four arguments, and then one or more
// group definitions separated by `--`. `-I` shows descriptions and `-i` does
// not, which is the manual's one sentence about this builtin; the fourth
// argument of `-I` is the string written between a match and its
// description, and `-i` has three arguments because it has no use for one.
// The arities were measured rather than read: `compdescribe -i '' 40 '-- '
// expl …` is `invalid argument: -- `, and that is the whole of the
// difference.
//
// A group definition is an array of `name:description` strings, optionally a
// second array holding what to insert instead of the names, and then the
// `compadd` options that group wants. Measured:
//
//	compdescribe -I '' 40 '-- ' expl g1        g1=(-a:all -m:machine)
//	                                           → matches (-a -m)
//	compdescribe -I '' 40 '-- ' expl g2 g2m    g2=(alpha:one beta:two)
//	                                           g2m=(A1 B1) → matches (A1 B1)
//
// # The display strings, measured
//
// A match is padded to the width of the longest in its group, then two
// spaces, then the separator, then the description:
//
//	alpha  -- one
//	beta   -- two
//
// # One group out for one group in
//
// zsh splits its answer further than this: one group per distinct listing
// arrangement, so that its listing can draw the described matches a row each
// and pack the undescribed ones together. This hands back one group per
// definition instead.
//
// Measured on zsh 5.9.2, 2026-09-16 through a pseudo-terminal, with a function
// shadowing this builtin and the shipped `_arguments` driving it over
// `gzip -c<TAB>` — one definition in, and the two shells' answers side by
// side:
//
//	          zsh                                   here
//	group 1   -l -S '' -J -default-                 -l -S ''
//	          -cd … -cV -cS  (the described)        -cd … -cV -c1 … -c9 -cS
//	group 2   -S '' -J -default-                    (none)
//	          -c1 … -c9      (the undescribed)
//
// so it is the **same twenty-three matches**, and the difference is which of
// them share a `-l`. `-l` is "one match per line", which is what a row
// carrying a description needs and what a bare name does not; splitting on it
// is a listing arrangement, and this editor draws one listing of replacement
// words and has no second one to ask for. A person sees the same twenty-three
// words from both shells, in one block here and in two there.
//
// Descriptions are built and handed over all the same, so the day repl's
// completion seam can carry one (#3041) they are already here — and the day it
// can draw a second arrangement, the split is one comparison in group().

// describeState is one `-i`/`-I` call: the groups it defined and how far `-g`
// has read them.
type describeState struct {
	descriptions bool
	separator    string
	expl         string
	groups       []describeGroup
	at           int
}

// describeGroup is one definition: where the names and descriptions come
// from, where the matches come from, and the `compadd` options to hand back
// with them.
type describeGroup struct {
	pairs   string // the array of `name:description` strings
	matches string // the array of replacements, empty where the names are it
	options []string
}

func compdescribeBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	// Three, where the other seven declare one: `compdescribe -i a` is two
	// words and zsh refuses it for the count before it looks at the verb.
	if !compArity(r, args, 3, -1) {
		return 1
	}
	_, st, ok := computilFrom(r, ctx)
	if !ok {
		return 1
	}
	switch args[0] {
	case "-i", "-I":
		return compdescribeDefine(r, st, args[0] == "-I", args[1:])
	case "-g":
		if st.describe == nil {
			r.Diagnosef("no parsed state\n")
			return 1
		}
		return st.describe.group(r, args[1:])
	}
	r.Diagnosef("invalid option: %s\n", args[0])
	return 1
}

// compdescribeDefine reads the four leading arguments and the `--`-separated
// group definitions.
func compdescribeDefine(r *interp.Runner, st *computilState, descriptions bool, args []string) int {
	lead := 3
	if descriptions {
		lead = 4
	}
	if len(args) < lead {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	d := &describeState{descriptions: descriptions, at: -1}
	if descriptions {
		d.separator = args[2]
	}
	d.expl = args[lead-1]
	for _, words := range splitOnDoubleDash(args[lead:]) {
		group, ok := describeGroupOf(r, words)
		if !ok {
			return 1
		}
		d.groups = append(d.groups, group)
	}
	st.describe = d
	return boolStatus(len(d.groups) > 0)
}

// splitOnDoubleDash breaks the group definitions apart.
func splitOnDoubleDash(args []string) [][]string {
	var out [][]string
	start := 0
	for i, word := range args {
		if word == "--" {
			out = append(out, args[start:i])
			start = i + 1
		}
	}
	return append(out, args[start:])
}

// describeGroupOf reads one definition: the leading flags this builtin eats
// itself, then one or two array names, then the options the group carries.
//
// `-g` is eaten rather than passed on, which is measured: the shipped
// `_arguments` writes `-g _a_11 -M …` and the `_args` array it reads back
// holds the `-M` and not the `-g`.
func describeGroupOf(r *interp.Runner, words []string) (describeGroup, bool) {
	var g describeGroup
	i := 0
	for ; i < len(words) && (words[i] == "-g" || words[i] == "-o" || words[i] == "-t"); i++ {
	}
	if i >= len(words) {
		r.Diagnosef("not enough arguments\n")
		return g, false
	}
	g.pairs = words[i]
	i++
	if i < len(words) && !strings.HasPrefix(words[i], "-") {
		g.matches = words[i]
		i++
	}
	g.options = words[i:]
	return g, true
}

// group is `-g`: the next group's listing style, its `compadd` options, its
// matches and its display strings, or 1 where there is none left.
//
// An empty group is stepped over rather than handed back, which is measured:
// `uname -` defines three and the caller is given one, because the other two
// arrays are empty by the time `compadd -D` has filtered them.
func (d *describeState) group(r *interp.Runner, into []string) int {
	if len(into) < 4 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	for {
		d.at++
		if d.at >= len(d.groups) {
			return 1
		}
		g := d.groups[d.at]
		pairs, _ := r.GetArray(g.pairs)
		if len(pairs) == 0 {
			continue
		}
		var matches []string
		if g.matches != "" {
			matches, _ = r.GetArray(g.matches)
		}
		words, displays := describeRows(pairs, matches, d)
		// `$compstate[list]` is left as it was: this editor draws one
		// listing and has no second arrangement to ask for.
		r.SetVar(into[0], "")
		r.SetArray(into[1], d.options(g))
		r.SetArray(into[2], words)
		r.SetArray(into[3], displays)
		return 0
	}
}

// options is the `compadd` options one group is answered with: this builtin's
// own listing flag, the group's options, and whatever the explanation array
// holds.
func (d *describeState) options(g describeGroup) []string {
	var out []string
	if d.descriptions {
		// `-l` lists the display strings one per line, which is what a
		// listing with a description on every row needs. Measured: it is in
		// the `_args` array zsh answers `uname -` with.
		out = append(out, "-l")
	}
	out = append(out, g.options...)
	return out
}

// describeRows is the matches and the display strings of one group: the
// replacement for each name, and the name padded out and followed by its
// description where this call was asked for descriptions.
func describeRows(pairs, matches []string, d *describeState) ([]string, []string) {
	names := make([]string, 0, len(pairs))
	descrs := make([]string, 0, len(pairs))
	width := 0
	for _, pair := range pairs {
		name, descr, _ := strings.Cut(pair, ":")
		names = append(names, name)
		descrs = append(descrs, descr)
		if len(name) > width {
			width = len(name)
		}
	}
	displays := make([]string, 0, len(names))
	for i, name := range names {
		if !d.descriptions || descrs[i] == "" {
			displays = append(displays, name)
			continue
		}
		displays = append(displays,
			name+strings.Repeat(" ", width-len(name))+"  "+d.separator+descrs[i])
	}
	if len(matches) > 0 {
		return matches, displays
	}
	return names, displays
}
