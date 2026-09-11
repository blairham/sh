// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/blairham/sh/interp"
)

// `compctl`: zsh's **old** completion system, superseded by `compsys` and
// still shipped, still called, and still the thing a plugin written before
// the change reaches for.
//
// # What this does and does not claim
//
// It records definitions and lists them back. It **completes nothing** —
// this shell has no completer of any shape, so a `compctl` here is a
// declaration nobody acts on.
//
// That is the bargain `dialect/zsh/setopt.go` already argues for the option
// table — *"Recording is right for a completion knob this shell has no
// completer for"* — and the reason it is the right one here is measured
// rather than assumed: on a real `~/.zshrc`, `compctl` was **eight of the
// fifteen** remaining lines of an interactive start, all eight from one Oh My
// Zsh snippet whose first ten lines are nothing but `compctl -g '*.go' …`
// (#2105). None of the eight reads a definition back.
//
// Recording rather than accepting-and-forgetting because a builtin that
// succeeds and answers nothing to `compctl -L` is the silent success
// `zmodload`'s wording exists to avoid.
//
// # The one divergence, stated rather than discovered
//
// `compctl -L` in zsh is **not** a replay of what was typed. Four things are
// normalized, and three of them are here:
//
//  1. simple flags cluster and sort — `-q -f` lists as `-fq`;
//  2. arguments are requoted — `-g "a b"` lists as `-g 'a b'`, `-S /` bare;
//  3. the three special entries carry defaults and are replaced whole.
//
// The fourth is not. An **extended spec is parsed and rewritten**: zsh lists
// `-x 'p[1]'` back as `-x 'p[1,1]'`, completing the range. That is a parser
// for the `-x` language, which this shell has no other use for, so an `-x`
// argument is listed **as it was written**. A script that defines one and
// reads it back gets its own text rather than zsh's normalization.
//
// Named that way for the reason #2079's `zcompile` names its own gap: the
// useful part of a builtin with its limit written down beats no builtin at
// all, and beats a limit somebody discovers later.
//
// # Measured on zsh 5.9.2, 2026-09-11
//
//	compctl                       COMMAND -c -tn / DEFAULT -f -tn / FIRST, 0
//	compctl -L                    the same three as `compctl -C -c -tn` …
//	compctl -g '*.go' gofmt       0, silent
//	compctl -g '*.go' g; compctl -g '*.c' g   the second replaces the first
//	compctl -f -D                 DEFAULT becomes `-f`, losing the default
//	compctl + ls                  0, and stores nothing
//	compctl -nosuchthing          0, and stores nothing
//	compctl -g                    `glob pattern expected after -g`, 1
//	compctl -M x                  `missing `:'`, 1
//	compctl -M 'm:{a-z}={A-Z}'    1, silent
//
// Entries list by name, sorted, with the three special rows after them.

// compctlStore holds the table under a name no script can spell, the way
// bindkey.go and zle.go keep theirs — which is also what gives a subshell its
// own copy. Two elements per entry: the name, then the rendered flags.
const compctlStore = ".zsh.compctl"

// compctlSpecials are the three entries that always exist, in the order they
// are listed, with the flags a fresh shell has and the letter `-L` names them
// by. Setting one replaces its flags entirely rather than merging.
var compctlSpecials = []struct{ name, letter, initial string }{
	{"COMMAND", "-C", "-c -tn"},
	{"DEFAULT", "-D", "-f -tn"},
	{"FIRST", "-T", ""},
}

// compctlArgLetters take a value; every other letter is a simple flag that
// clusters. `x` is here because its argument is the extended spec, which is
// kept verbatim — see the divergence above.
const compctlArgLetters = "kgsKSPpWMXtJVyxHu"

func registerCompctl(r *interp.Runner) { r.Register("compctl", compctlBuiltin) }

func compctlBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	listing, rest := false, args
	if len(rest) > 0 && rest[0] == "-L" {
		listing, rest = true, rest[1:]
	}
	if len(rest) == 0 {
		compctlList(r, listing, nil)
		return 0
	}

	simple, valued, names, special, code := compctlParse(r, rest)
	if code != 0 {
		return code
	}
	if listing {
		compctlList(r, true, names)
		return 0
	}
	flags := compctlRender(simple, valued)
	switch {
	case special != "":
		compctlSet(r, special, flags)
	case len(names) == 0:
		// `compctl + ls` and a letter nothing claims both land here: zsh
		// takes them at 0 and stores nothing.
		return 0
	}
	for _, name := range names {
		compctlSet(r, name, flags)
	}
	return 0
}

// compctlParse reads the flags and the names. The two results that are not
// obvious: special is the name of the one entry `-C`, `-D` or `-T` chose, and
// names is empty for a line that named none.
func compctlParse(r *interp.Runner, args []string) (simple string, valued []string, names []string, special string, code int) {
	i := 0
	for ; i < len(args); i++ {
		word := args[i]
		if word == "--" {
			i++
			break
		}
		if word == "+" {
			// A separator between alternative definitions. Nothing is stored
			// for the line, measured, so the whole thing is taken at 0.
			return "", nil, nil, "", 0
		}
		if len(word) < 2 || word[0] != '-' {
			break
		}
		for j := 1; j < len(word); j++ {
			letter := word[j]
			switch {
			case letter == 'C' || letter == 'D' || letter == 'T':
				for _, s := range compctlSpecials {
					if s.letter == "-"+string(letter) {
						special = s.name
					}
				}
			case strings.IndexByte(compctlArgLetters, letter) >= 0:
				value, taken, ok := compctlValue(word, j, args, &i)
				if !ok {
					r.Diagnosef("%s\n", compctlMissing(letter))
					return "", nil, nil, "", 1
				}
				if letter == 'M' {
					// Matching specs are refused whole, measured — `-M x` by
					// its own sentence and a well-formed one silently.
					if !strings.Contains(value, ":") {
						r.Diagnosef("missing `:'\n")
					}
					return "", nil, nil, "", 1
				}
				valued = append(valued, "-"+string(letter)+" "+compctlQuote(value))
				j = taken
			default:
				if isCompctlSimple(letter) {
					if !strings.ContainsRune(simple, rune(letter)) {
						simple += string(letter)
					}
					continue
				}
				// A letter zsh has no meaning for is taken at 0 with nothing
				// stored, which is measured and is not what most builtins do.
				return "", nil, nil, "", 0
			}
		}
	}
	return simple, valued, args[i:], special, 0
}

// compctlValue is an argument letter's value: the rest of the word it is in,
// or the word after it. The second result is how far into the word was used.
func compctlValue(word string, j int, args []string, i *int) (string, int, bool) {
	if j+1 < len(word) {
		return word[j+1:], len(word), true
	}
	if *i+1 < len(args) {
		*i++
		return args[*i], len(word), true
	}
	return "", 0, false
}

// compctlMissing is the sentence for an argument letter with nothing after
// it. Only `-g` was measured; the rest take its shape with their own noun.
func compctlMissing(letter byte) string {
	switch letter {
	case 'g':
		return "glob pattern expected after -g"
	case 'k':
		return "variable name expected after -k"
	case 'K':
		return "function name expected after -K"
	case 'M':
		return "missing `:'"
	}
	return "argument expected after -" + string(letter)
}

// isCompctlSimple reports whether a letter is one of the no-argument flags.
// Letters, and only letters: a digit or a symbol is not a flag here.
func isCompctlSimple(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// compctlRender is the flags as `-L` writes them: the simple ones clustered
// and sorted into one word, then the argument-taking ones in the order they
// were given.
func compctlRender(simple string, valued []string) string {
	var parts []string
	if simple != "" {
		letters := strings.Split(simple, "")
		sort.Strings(letters)
		parts = append(parts, "-"+strings.Join(letters, ""))
	}
	parts = append(parts, valued...)
	return strings.Join(parts, " ")
}

// compctlQuote spells an argument the way the listing does: bare where every
// character is ordinary, and single-quoted otherwise. The same rule
// interp.ListingQuoteWhenNeededEscaped applies to an alias, which is this
// dialect's answer for every other listing — written here because that
// helper is not exported to a dialect.
func compctlQuote(v string) string {
	bare := v != ""
	for i := 0; i < len(v) && bare; i++ {
		c := v[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("_-./:@+,%^", c) >= 0:
		default:
			bare = false
		}
	}
	if bare {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// compctlList writes the table. `only` narrows it to the names asked for,
// which is what `compctl -L name` does; empty is everything.
func compctlList(r *interp.Runner, dashL bool, only []string) {
	stored := compctlRead(r)
	names := make([]string, 0, len(stored))
	for name := range stored {
		// The three special entries are listed by the loop below, in their
		// own order and with their own letters, so a set one must not also
		// appear among the names — `compctl -f -D` lists one `-D` row and
		// not a `DEFAULT` row beside it.
		if !isCompctlSpecial(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(only) > 0 {
		names = only
	}
	for _, name := range names {
		flags, ok := stored[name]
		if !ok {
			continue
		}
		_, _ = fmt.Fprint(r.Out(), compctlLine(dashL, name, "", flags))
	}
	if len(only) > 0 {
		// A named listing is only those names: the three special rows are
		// not printed, measured.
		return
	}
	for _, s := range compctlSpecials {
		flags, ok := stored[s.name]
		if !ok {
			flags = s.initial
		}
		_, _ = fmt.Fprint(r.Out(), compctlLine(dashL, s.name, s.letter, flags))
	}
}

// isCompctlSpecial reports whether a name is one of the three entries that
// always exist and are listed with a letter of their own.
func isCompctlSpecial(name string) bool {
	for _, s := range compctlSpecials {
		if s.name == name {
			return true
		}
	}
	return false
}

// compctlLine is one row in either spelling: `NAME flags` bare, and
// `compctl flags NAME` under `-L`, with a special entry naming its letter
// where the bare form names the entry.
func compctlLine(dashL bool, name, letter, flags string) string {
	if dashL {
		head := "compctl"
		if letter != "" {
			head += " " + letter
		}
		if flags != "" {
			head += " " + flags
		}
		if letter == "" {
			head += " " + name
		}
		return head + "\n"
	}
	if flags == "" {
		return name + "\n"
	}
	return name + " " + flags + "\n"
}

// The table, as a flat array of pairs — the shape bindkey.go and zle.go use,
// so nothing in either half needs escaping.
func compctlRead(r *interp.Runner) map[string]string {
	flat, _ := r.GetArray(compctlStore)
	out := make(map[string]string, len(flat)/2)
	for i := 0; i+2 <= len(flat); i += 2 {
		out[flat[i]] = flat[i+1]
	}
	return out
}

func compctlSet(r *interp.Runner, name, flags string) {
	flat, _ := r.GetArray(compctlStore)
	for i := 0; i+2 <= len(flat); i += 2 {
		if flat[i] == name {
			// A second definition **replaces**, measured: two `-g` for one
			// command leave only the second.
			flat[i+1] = flags
			r.SetArray(compctlStore, flat)
			return
		}
	}
	r.SetArray(compctlStore, append(flat, name, flags))
}
