// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package histexpand rewrites a line the way a shell's history expander does:
// `!!` becomes the previous command, `!$` its last word, `^old^new^` the
// previous command with one word changed.
//
// It is the oldest feature in any interactive shell and the one this tree had
// none of at all (#3093): `set -H` was refused, and `!!` reached the parser as
// two literal characters, so a shell used at a prompt answered a different
// command from the one its user typed.
//
// # Why a package rather than a pass in the lexer
//
// History expansion is not a stage of parsing. It happens to the **text of a
// line** before anything has looked at it — measured on bash 5.3.20,
// 2026-09-15, `echo "!!"` expands inside the double quotes and `echo '!!'`
// does not, which no parser that had already tokenized the line could tell
// apart from a parameter expansion's rules. So the input is a string and the
// output is a string, and what runs it never knows this happened.
//
// # What the panel does with it
//
// Measured through a pseudo-terminal with a two-row prompt on 2026-09-15, and
// again from a script with `set -o history; set -H`:
//
//	bash 5.3, bash 3.2, bash-as-sh   on at a prompt, off in a script
//	zsh 5.9.2                        on at a prompt, off in a script
//	ksh93u+ 2012-08-01               off everywhere; `set -H` turns it on
//	dash, BusyBox ash                no such feature; `set -H` is invalid
//
// The engine here is one engine. Which dialects reach it, and whether a
// prompt starts with it on, is [github.com/blairham/sh/interp.Semantics]'
// question — see HistoryExpansion and HistoryExpansionAtAPrompt.
package histexpand

import (
	"fmt"
	"strconv"
	"strings"
)

// Chars are the three characters history expansion is spelled with, which
// `histchars` moves: the event character, the quick-substitution character and
// the comment character. See docs/spec/history.md, where the parameter was
// measured before anything read it.
//
// A zero Event turns the whole engine off, which is what an empty `histchars`
// means in both shells that have the parameter.
type Chars struct {
	Event   rune
	Quick   rune
	Comment rune
}

// Default is what a shell starts with: `!^#`.
var Default = Chars{Event: '!', Quick: '^', Comment: '#'}

// List is the history the designators index, oldest entry first.
//
// First is the history number of Lines[0] — bash numbers from 1 and the
// numbers do not restart when old entries fall off the front, so a list that
// carried only its slice would answer `!2` with the wrong line in any session
// long enough to have trimmed one.
type List struct {
	Lines []string
	First int
}

// number is the history number of the last entry, or First-1 for an empty
// list.
func (l List) number() int { return l.First + len(l.Lines) - 1 }

// at returns the entry with history number n.
func (l List) at(n int) (string, bool) {
	i := n - l.First
	if i < 0 || i >= len(l.Lines) {
		return "", false
	}
	return l.Lines[i], true
}

// NotFound is a reference to an event the list does not hold. It is the one
// error a person meets in ordinary use — a typo in a `!string` — and every
// shell in the panel words it slightly differently, so the reference is
// carried rather than a finished sentence.
type NotFound struct{ Ref string }

func (e *NotFound) Error() string { return e.Ref + ": event not found" }

// BadModifier is a `:` followed by something that is not a modifier.
type BadModifier struct{ Mod string }

func (e *BadModifier) Error() string { return e.Mod + ": unrecognized history modifier" }

// SubstFailed is an `s/old/new/` or `^old^new^` whose left side is not in the
// event it was applied to.
//
// Two spellings, because the panel names two different things. Measured
// 2026-09-15: bash reports the *modifier* — `:s/nope/x/: substitution failed`,
// and `:s^abc^xyz^` for a quick substitution, which is the form bash rewrites
// one into — while ksh93 reports what was typed, `^abc^xyz^`, and zsh names
// nothing at all. Ref is bash's and Bare is ksh93's.
type SubstFailed struct{ Ref, Bare string }

func (e *SubstFailed) Error() string { return e.Ref + ": substitution failed" }

// Result is what an expansion produced.
type Result struct {
	// Line is the text to run, expanded.
	Line string
	// Changed says whether Line differs from what was handed in, which is
	// what decides whether the shell echoes it. Measured: bash writes the
	// expanded line to **standard error** before running it, and writes
	// nothing at all when the expansion changed nothing.
	Changed bool
	// Print is a `:p` modifier: show the expansion, remember it, run nothing.
	Print bool
}

// state is what one Expand call carries between designators: the last `?str?`
// search, which `%` names, and the last `s/old/new/`, which `&` repeats.
type state struct {
	matched string
	old     string
	new     string
}

// Quote is what the text handed to ExpandIn begins inside.
//
// A shell at a prompt hands over a whole logical line and the answer is always
// Unquoted. A shell reading a **script** hands over one physical line at a
// time, and a quote opened on an earlier line is still open when the next one
// arrives: measured on bash 5.3.20, `echo 'a` / `!!` / `b'` leaves the two
// characters alone and `echo "a` / `!!` / `b"` expands them, so the state has
// to cross the line boundary or half the rule is lost.
//
// It seeds the scanner and nothing more — the scanner still closes the quote
// when it reaches the character that closes it, which is what makes `echo 'a`
// / `b' !!` expand the reference *after* the quote ends. Measured.
type Quote uint8

const (
	// Unquoted is a line that begins outside any quote.
	Unquoted Quote = iota
	// InSingleQuotes is a line continuing a single-quoted string, where
	// nothing expands until the quote closes.
	InSingleQuotes
	// InDoubleQuotes is a line continuing a double-quoted string, where
	// references expand exactly as they do outside one.
	InDoubleQuotes
)

// Expand rewrites one whole line, which is what a prompt hands over.
func Expand(line string, hist List, c Chars) (Result, error) {
	return ExpandIn(line, Unquoted, hist, c)
}

// ExpandIn rewrites one line that begins in a known quoting state.
//
// The line is scanned left to right rather than split into words, because the
// quoting rules are the scanner's: text inside single quotes is never
// expanded, text inside double quotes is, and a `'` inside double quotes opens
// nothing — measured, `echo "it's !!"` expands where `echo '!!'` does not.
//
// A `^old^new^` quick substitution is only one when the line begins outside a
// quote. Inside one the character is ordinary text, and a line of a here
// document or of a continued string that happened to start with it would
// otherwise be rewritten into something nobody wrote.
func ExpandIn(line string, in Quote, hist List, c Chars) (Result, error) {
	if c.Event == 0 {
		return Result{Line: line}, nil
	}
	var st state
	if in == Unquoted && c.Quick != 0 && strings.HasPrefix(line, string(c.Quick)) {
		return quick(line, hist, c, &st)
	}
	src := []rune(line)
	var out strings.Builder
	res := Result{}
	single, double := in == InSingleQuotes, in == InDoubleQuotes
	for i := 0; i < len(src); {
		r := src[i]
		switch {
		case r == '\\':
			// A backslash protects whatever follows it from this pass, and
			// both characters stay: measured, `echo a\!b` prints `a!b` —
			// the backslash is taken off later, by the shell's own quote
			// removal, and never here.
			out.WriteRune(r)
			if i+1 < len(src) {
				out.WriteRune(src[i+1])
				i += 2
				continue
			}
			i++
			continue
		case r == '\'' && !double:
			single = !single
			out.WriteRune(r)
			i++
			continue
		case r == '"' && !single:
			double = !double
			out.WriteRune(r)
			i++
			continue
		case r == c.Event && !single:
			if literal(src, i, c, double) {
				out.WriteRune(r)
				i++
				continue
			}
			text, next, print, err := one(src, i, out.String(), hist, c, &st)
			if err != nil {
				return Result{}, err
			}
			out.WriteString(text)
			res.Print = res.Print || print
			res.Changed = true
			i = next
			continue
		}
		out.WriteRune(r)
		i++
	}
	res.Line = out.String()
	if !res.Changed {
		res.Line = line
	}
	return res, nil
}

// noExpandAfter are the characters that make the event character ordinary
// text when they follow it, measured one character at a time on bash 5.3.20 on
// 2026-09-15 by asking `printf '%s\n' "T<c>|!<c>|"` after a seeded history:
// these came back untouched and every other printable character was taken as
// the start of an event reference.
//
// The quote characters are here for a reason of their own rather than for
// bash's list: a `!` against a closing quote has an empty event name, and an
// empty name expands to nothing in every shell that has the feature.
const noExpandAfter = " \t\n\r=|&;()<>\"'"

// literal reports whether the event character at i is ordinary text.
//
// Three rules, each measured rather than read:
//
//   - what follows it is in noExpandAfter, or it is the last character of the
//     line — `echo end!` and `echo hi ! there` are not expansions;
//   - it is the first character inside a bracket expression, so `[!a-z]` is
//     still a glob that matches one character outside the range. Only
//     *immediately* after the bracket: `[a!s]` is an event reference, and
//     measured it is one;
//   - it stands directly after `${`, so `${!v}` is still an indirect
//     expansion. Measured against `$ {!s}`, which *is* an event reference, so
//     the rule is the two characters together and not the brace.
func literal(src []rune, i int, c Chars, double bool) bool {
	if i+1 >= len(src) || strings.ContainsRune(noExpandAfter, src[i+1]) {
		return true
	}
	if i > 0 && src[i-1] == '[' {
		return true
	}
	if i > 1 && src[i-1] == '{' && src[i-2] == '$' {
		return true
	}
	_ = c
	_ = double
	return false
}

// one expands the single reference beginning at the event character at i,
// returning the text it became and the index just past it.
//
// sofar is the line built so far, which `!#` names.
func one(src []rune, i int, sofar string, hist List, c Chars, st *state) (string, int, bool, error) {
	j := i + 1
	// `!{...}` puts the whole reference in braces so that what follows it
	// cannot be read as part of it.
	if j < len(src) && src[j] == '{' {
		end := j + 1
		for end < len(src) && src[end] != '}' {
			end++
		}
		if end >= len(src) {
			return "", 0, false, &NotFound{Ref: string(src[i:])}
		}
		inner := append([]rune{c.Event}, src[j+1:end]...)
		text, _, print, err := one(inner, 0, sofar, hist, c, st)
		if err != nil {
			return "", 0, false, err
		}
		return text, end + 1, print, nil
	}
	ref := func(end int) string { return string(src[i:end]) }

	var words []string
	// raw is the event's own text, which is what a reference with no word
	// designator on it becomes. Kept beside the words rather than rebuilt
	// from them: measured, `echo   spaced    words` recalled by `!!` comes
	// back with its spacing intact, where `!!:*` joins the words with one
	// space each. A prompt never showed the difference because a line typed
	// at one is a line; a **script** puts whole multi-line commands in the
	// list, and joining those with spaces turns a here-document into gibberish.
	var raw string
	var haveWords bool
	switch {
	case j < len(src) && src[j] == '#':
		// The line up to here, which is the one event that is not in the
		// list at all.
		words, raw = fields(sofar), sofar
		haveWords = true
		j++
	case j < len(src) && (src[j] == c.Event):
		entry, ok := hist.at(hist.number())
		if !ok {
			return "", 0, false, &NotFound{Ref: ref(j + 1)}
		}
		words, raw = fields(entry), entry
		haveWords = true
		j++
	case j < len(src) && isWordDesignator(src[j], c):
		// `!$`, `!^`, `!*` and `!:n` are the previous event with a word
		// designator on it — the event was never written.
		entry, ok := hist.at(hist.number())
		if !ok {
			return "", 0, false, &NotFound{Ref: ref(j + 1)}
		}
		words, raw = fields(entry), entry
		haveWords = true
	case j < len(src) && src[j] == '?':
		k := j + 1
		for k < len(src) && src[k] != '?' && src[k] != '\n' {
			k++
		}
		want := string(src[j+1 : k])
		if k < len(src) && src[k] == '?' {
			k++
		}
		entry, ok := search(hist, want, true)
		if !ok {
			return "", 0, false, &NotFound{Ref: ref(k)}
		}
		st.matched = want
		words, raw = fields(entry), entry
		haveWords = true
		j = k
	default:
		k := j
		if k < len(src) && src[k] == '-' {
			k++
		}
		digits := k
		for digits < len(src) && src[digits] >= '0' && src[digits] <= '9' {
			digits++
		}
		if digits > k {
			n, err := strconv.Atoi(string(src[j:digits]))
			if err != nil {
				return "", 0, false, &NotFound{Ref: ref(digits)}
			}
			if n < 0 {
				n = hist.number() + 1 + n
			}
			entry, ok := hist.at(n)
			if !ok {
				return "", 0, false, &NotFound{Ref: ref(digits)}
			}
			words, raw = fields(entry), entry
			haveWords = true
			j = digits
			break
		}
		k = j
		for k < len(src) && !strings.ContainsRune(eventEnd, src[k]) && src[k] != c.Event {
			k++
		}
		if k == j {
			// Nothing to search for: the character is ordinary text.
			return string(src[i : i+1]), i + 1, false, nil
		}
		want := string(src[j:k])
		entry, ok := search(hist, want, false)
		if !ok {
			return "", 0, false, &NotFound{Ref: ref(k)}
		}
		words, raw = fields(entry), entry
		haveWords = true
		j = k
	}
	if !haveWords {
		return "", 0, false, &NotFound{Ref: ref(j)}
	}

	chosen, j, ok, err := designate(src, j, words, c, st)
	if err != nil {
		return "", 0, false, err
	}
	text := raw
	if ok {
		text = strings.Join(chosen, " ")
	}
	text, j, print, err := modifiers(src, j, text, ref(j), st)
	if err != nil {
		return "", 0, false, err
	}
	return text, j, print, nil
}

// eventEnd are the characters that end a `!string` event reference. Whitespace
// and the word-designator separator, plus the operators a command line is
// built out of — a `!vi` at the end of `foo && !vi` names `vi` and not
// `vi` with the shell's own punctuation glued on.
const eventEnd = " \t\n\r:^$*%=|&;()<>\"'`\\"

// isWordDesignator reports whether r is one of the designators that may follow
// the event character with no event of its own — `!$` and `!^` and `!*` and
// `!%`, plus the `:` that introduces a numbered one.
func isWordDesignator(r rune, c Chars) bool {
	return r == '$' || r == '*' || r == '%' || r == ':' || (c.Quick != 0 && r == c.Quick)
}

// search finds the most recent entry starting with (or, for substring,
// containing) want.
func search(hist List, want string, substring bool) (string, bool) {
	if want == "" {
		return "", false
	}
	for i := len(hist.Lines) - 1; i >= 0; i-- {
		if substring {
			if strings.Contains(hist.Lines[i], want) {
				return hist.Lines[i], true
			}
			continue
		}
		if strings.HasPrefix(hist.Lines[i], want) {
			return hist.Lines[i], true
		}
	}
	return "", false
}

// fields splits an entry into the words a designator indexes. Whitespace, the
// way the history library splits it — this is not the shell's own word
// splitting, and it runs over text that has not been expanded at all.
func fields(entry string) []string { return strings.Fields(entry) }

// designate applies a word designator, reporting whether one was written.
func designate(src []rune, j int, words []string, c Chars, st *state) ([]string, int, bool, error) {
	if j >= len(src) {
		return nil, j, false, nil
	}
	colon := false
	if src[j] == ':' {
		colon = true
		j++
	}
	if j >= len(src) {
		if colon {
			return nil, j, false, &BadModifier{Mod: ""}
		}
		return nil, j, false, nil
	}
	last := len(words) - 1
	pick := func(from, to int) []string {
		if from < 0 {
			from = 0
		}
		if to > last {
			to = last
		}
		if from > to {
			return nil
		}
		return words[from : to+1]
	}
	// A number, or one of the single-character designators.
	start, end := -1, -1
	switch r := src[j]; {
	case c.Quick != 0 && r == c.Quick:
		start, end = 1, 1
		j++
	case r == '$':
		start, end = last, last
		j++
	case r == '*':
		if last < 1 {
			return nil, j + 1, true, nil
		}
		start, end = 1, last
		j++
	case r == '%':
		for i, w := range words {
			if st.matched != "" && strings.Contains(w, st.matched) {
				start, end = i, i
				break
			}
		}
		if start < 0 {
			return nil, j + 1, true, nil
		}
		j++
	case r >= '0' && r <= '9':
		k := j
		for k < len(src) && src[k] >= '0' && src[k] <= '9' {
			k++
		}
		n, _ := strconv.Atoi(string(src[j:k]))
		start, end = n, n
		j = k
	default:
		if colon {
			// A `:` that named no word is a modifier's colon; hand it back.
			return nil, j - 1, false, nil
		}
		return nil, j, false, nil
	}
	// A range: `x-y`, `x-` (up to the last but one) or `x*` (up to the last).
	if j < len(src) && src[j] == '-' {
		k := j + 1
		for k < len(src) && src[k] >= '0' && src[k] <= '9' {
			k++
		}
		if k > j+1 {
			n, _ := strconv.Atoi(string(src[j+1 : k]))
			return pick(start, n), k, true, nil
		}
		if last-1 >= start {
			return pick(start, last-1), j + 1, true, nil
		}
		return nil, j + 1, true, nil
	}
	if j < len(src) && src[j] == '*' {
		return pick(start, last), j + 1, true, nil
	}
	return pick(start, end), j, true, nil
}

// modifiers applies the `:h`, `:t`, `:r`, `:e`, `:p`, `:q`, `:x`, `:s` and
// `:&` chain, and reports whether `:p` was among them.
func modifiers(src []rune, j int, text, ref string, st *state) (string, int, bool, error) {
	print := false
	for j < len(src) && src[j] == ':' {
		j++
		if j >= len(src) {
			return "", 0, false, &BadModifier{Mod: ""}
		}
		global := false
		for j < len(src) && (src[j] == 'g' || src[j] == 'a') {
			global = true
			j++
		}
		if j >= len(src) {
			return "", 0, false, &BadModifier{Mod: ""}
		}
		switch src[j] {
		case 'h':
			text = head(text)
			j++
		case 't':
			text = tail(text)
			j++
		case 'r':
			text = root(text)
			j++
		case 'e':
			text = ext(text)
			j++
		case 'p':
			print = true
			j++
		case 'q':
			text = "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
			j++
		case 'x':
			parts := strings.Fields(text)
			for i, p := range parts {
				parts[i] = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
			}
			text = strings.Join(parts, " ")
			j++
		case 's', '&':
			var err error
			text, j, err = substitute(src, j, text, ref, global, st)
			if err != nil {
				return "", 0, false, err
			}
		default:
			return "", 0, false, &BadModifier{Mod: string(src[j])}
		}
	}
	return text, j, print, nil
}

// substitute applies `s/old/new/` or the `&` that repeats the last one.
//
// The delimiter is whatever character follows the `s`, which is what makes
// `:s,a,b,` work when the text is a path.
func substitute(src []rune, j int, text, ref string, global bool, st *state) (string, int, error) {
	start := j
	if src[j] == '&' {
		j++
		if st.old == "" {
			return "", 0, &SubstFailed{Ref: ":" + string(src[start:j])}
		}
		out, err := apply(text, st.old, st.new, global, ":"+string(src[start:j]))
		return out, j, err
	}
	j++ // past the `s`
	if j >= len(src) {
		return "", 0, &SubstFailed{Ref: ":" + string(src[start:])}
	}
	delim := src[j]
	j++
	read := func() (string, bool) {
		var b strings.Builder
		for j < len(src) {
			if src[j] == '\\' && j+1 < len(src) && src[j+1] == delim {
				b.WriteRune(delim)
				j += 2
				continue
			}
			if src[j] == delim {
				j++
				return b.String(), true
			}
			b.WriteRune(src[j])
			j++
		}
		return b.String(), false
	}
	old, _ := read()
	repl, _ := read()
	if old == "" {
		old = st.old
	}
	if old == "" {
		return "", 0, &SubstFailed{Ref: ref}
	}
	st.old, st.new = old, repl
	out, err := apply(text, old, repl, global, ":"+string(src[start:j]))
	return out, j, err
}

// apply is the substitution itself, which fails rather than doing nothing when
// the left side is not there: measured, `^nosuch^x^` is `substitution failed`
// in bash, zsh and ksh93 alike and the line does not run.
func apply(text, old, repl string, global bool, ref string) (string, error) {
	if !strings.Contains(text, old) {
		return "", &SubstFailed{Ref: ref, Bare: strings.TrimPrefix(ref, ":s")}
	}
	if global {
		return strings.ReplaceAll(text, old, repl), nil
	}
	return strings.Replace(text, old, repl, 1), nil
}

// quick expands `^old^new^rest`, which is the previous event with one
// substitution and is only a substitution when it opens the line.
func quick(line string, hist List, c Chars, st *state) (Result, error) {
	entry, ok := hist.at(hist.number())
	if !ok {
		return Result{}, &NotFound{Ref: line}
	}
	src := []rune(line)
	q := c.Quick
	read := func(i int) (string, int) {
		var b strings.Builder
		for i < len(src) {
			if src[i] == '\\' && i+1 < len(src) && src[i+1] == q {
				b.WriteRune(q)
				i += 2
				continue
			}
			if src[i] == q {
				return b.String(), i + 1
			}
			b.WriteRune(src[i])
			i++
		}
		return b.String(), i
	}
	old, i := read(1)
	repl, i := read(i)
	written := string(src[:i])
	if old == "" {
		return Result{}, &SubstFailed{Ref: ":s" + written, Bare: written}
	}
	if !strings.Contains(entry, old) {
		return Result{}, &SubstFailed{Ref: ":s" + written, Bare: written}
	}
	st.old, st.new = old, repl
	out := strings.Replace(entry, old, repl, 1)
	// Anything after the closing character is a modifier chain on the result.
	if i < len(src) {
		var err error
		var print bool
		out, _, print, err = modifiers(src, i, out, line, st)
		if err != nil {
			return Result{}, err
		}
		return Result{Line: out, Changed: true, Print: print}, nil
	}
	return Result{Line: out, Changed: true}, nil
}

func head(s string) string {
	if i := strings.LastIndexByte(s, '/'); i > 0 {
		return s[:i]
	} else if i == 0 {
		return "/"
	}
	return s
}

func tail(s string) string {
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// root is `:r` and ext is `:e`, and both are about the last `.` in the **whole
// word** rather than in its last path component — which is measured and is the
// opposite of what a basename-first reading gives. Eight words on bash 5.3.20,
// 2026-09-16, each against a fresh one-line history:
//
//	word          :r          :e
//	plain         plain       plain
//	c.txt         c           .txt
//	/a/b/c.txt    /a/b/c      .txt
//	/a/b/c        /a/b/c      /a/b/c
//	a.b.c         a.b         .c
//	.hidden       (empty)     .hidden
//	/a.b/c        /a          .b/c
//	x.            x           .
//
// `/a.b/c` is the discriminator for the whole-word reading and `.hidden` for
// the position-zero one; `plain` and `/a/b/c` say that a word with no `.` at
// all comes back **whole** from both, which is the answer neither name
// suggests and is why it is written down rather than reasoned about.
func root(s string) string {
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		return s[:i]
	}
	return s
}

func ext(s string) string {
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		return s[i:]
	}
	return s
}

// String renders the three characters the way `histchars` holds them, which is
// what a dialect reads the parameter back out as.
func (c Chars) String() string {
	return fmt.Sprintf("%c%c%c", c.Event, c.Quick, c.Comment)
}
