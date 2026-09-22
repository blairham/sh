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

	// DoubleQuotesProtect makes a double-quoted string as safe from this
	// pass as a single-quoted one. Not a character, but carried beside them
	// because it is the same kind of fact: how the text being scanned is
	// read. See Semantics.HistoryExpansionSparesDoubleQuotesInPosixMode for
	// the shell that sets it and when.
	DoubleQuotesProtect bool

	// Words is how an event is cut into words. See Words.
	Words Words

	// QuoteInPlace applies a `:q` or `:x` where it is written in a chain of
	// modifiers, rather than once the chain has run. Measured 2026-09-16
	// over the word `two.three`: `:q:r` is `'two'` in bash 5.3.20 and
	// ksh93u+, which quote last, and `'two` in zsh 5.9.2, which quotes in
	// place and so takes the root of the quoted word.
	QuoteInPlace bool

	// CommentStops ends expansion for the rest of the line at a Comment
	// character that begins a word outside quotes. See
	// Semantics.HistoryCommentStopsExpansion.
	CommentStops bool

	// QuoteIsText makes a single quote or a backquote against the event
	// character ordinary text, where the two columns without it read the
	// quote as the first letter of an event's name. See
	// Semantics.HistoryQuoteEndsAnEventReference.
	QuoteIsText bool

	// EventCharClosesAnEventName ends an event name at an event character
	// inside it, that character included. See
	// Semantics.HistoryEventCharClosesAnEventName.
	EventCharClosesAnEventName bool

	// BracedEvent reads `!{…}` as an event reference in braces. See
	// Semantics.HistoryBracedEventReference.
	BracedEvent bool

	// LastWordEndsTheDesignator ends a word designator at a `$`, so that
	// nothing after it is read as a range. See
	// Semantics.HistoryLastWordEndsTheDesignator.
	LastWordEndsTheDesignator bool

	// FirstWordEndsARange reads the Quick character as word one where a
	// range's end is written. See Semantics.HistoryFirstWordEndsARange.
	FirstWordEndsARange bool

	// WordwiseSubstitution is the `G` before an `s` or an `&`, which
	// substitutes once in each word. See
	// Semantics.HistoryWordwiseSubstitutionModifier.
	WordwiseSubstitution bool
}

// Default is what a shell starts with: `!^#`.
var Default = Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}

// List is the history the designators index, oldest entry first.
//
// First is the history number of Lines[0] — bash numbers from 1 and the
// numbers do not restart when old entries fall off the front, so a list that
// carried only its slice would answer `!2` with the wrong line in any session
// long enough to have trimmed one.
type List struct {
	Lines []string
	First int

	// Memory is what earlier expansions left for later ones, and nil gives
	// each call a fresh one. See Memory.
	Memory *Memory
}

// Memory is the last substitution and the last `?string?` search, which
// outlive the line that wrote them.
//
// Measured 2026-09-16 on bash 5.3.20 from a script, and on zsh 5.9.2 and
// ksh93u+ at a prompt: after `!!:s/o/0/` on one line, `!!:&` on the **next**
// repeats it and `!!:s//X/` replaces the `o` it named — in all three. So the
// state is the shell's, kept by whoever calls Expand for it, rather than one
// line's.
type Memory struct {
	state
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

// BadWordSpecifier is a word designator naming a word the event does not have,
// or a range running backwards. Ref is the designator as written, from its
// colon where it has one — bash and ksh93 both say `:2-9: bad word specifier`
// and `-9: bad word specifier` — and zsh names nothing.
type BadWordSpecifier struct{ Ref string }

func (e *BadWordSpecifier) Error() string { return e.Ref + ": bad word specifier" }

// NoPreviousSubstitution is a `:&`, or a substitution with an empty left side,
// in a shell that has not substituted anything yet. Ref is the modifier as
// written with the colon and any `g` in front of it: measured 2026-09-16,
// bash says `:g&: no previous substitution` and `:s//X/: no previous
// substitution`, and `:s^^X^` for a quick substitution; ksh93 says the same
// of `:g&`; zsh says `no previous substitution` and names nothing.
type NoPreviousSubstitution struct{ Ref string }

func (e *NoPreviousSubstitution) Error() string { return e.Ref + ": no previous substitution" }

// chainRef is a failed modifier as the shells that name one name it: the
// **whole chain** from its first colon up to and including the modifier that
// failed.
//
// Measured 2026-09-18 after `echo one/two.one`, in a script on bash 5.3.20 and
// at a prompt on ksh93u+ (#3422): `!!:t:gs/x/y/` is `:t:gs/x/y/: substitution
// failed` in both, and `!!:q:&` names `:q:&` in both. Naming the failing
// modifier alone — `:s/x/y/`, `:&` — was this engine's answer and is nobody's.
// zsh 5.9.2 names nothing at all, which its wording handles by ignoring the
// reference rather than by carrying a different one.
func chainRef(src []rune, chain, end int) string {
	return string(src[chain:min(end, len(src))])
}

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

// previousOld is what an empty left side stands for: the last substitution's
// left side, or where nothing has been substituted, the last `?string?`
// searched for. Measured 2026-09-16 on bash 5.3.20 from a script: after
// `echo !?two?%`, `!!:s//X/` and `^^X^` each replace `two`, and with neither
// before them both are `no previous substitution`.
func (st *state) previousOld() string {
	if st.old != "" {
		return st.old
	}
	return st.matched
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
// A command substitution restarts that last part. Inside a `$(…)` or a pair of
// backquotes a `'` is a quote again however the substitution is written, so
// `echo "$( echo '!zz' )"` prints the text where `echo "'!zz'"` is an event
// nobody has. Measured on bash 5.3.20, 2026-09-22.
//
// A `^old^new^` quick substitution is only one when the line begins outside a
// quote. Inside one the character is ordinary text, and a line of a here
// document or of a continued string that happened to start with it would
// otherwise be rewritten into something nobody wrote.
func ExpandIn(line string, in Quote, hist List, c Chars) (Result, error) {
	if c.Event == 0 {
		return Result{Line: line}, nil
	}
	st := &state{}
	if hist.Memory != nil {
		st = &hist.Memory.state
	}
	if in == Unquoted && c.Quick != 0 && strings.HasPrefix(line, string(c.Quick)) {
		return quick(line, hist, c, st)
	}
	src := []rune(line)
	var out strings.Builder
	res := Result{}
	single, double := in == InSingleQuotes, in == InDoubleQuotes
	// parens is one entry per open parenthesis, true where the parenthesis
	// opened a command substitution. backquote is the other spelling of the
	// same thing. Together they answer inSub, which is what makes a single
	// quote special again inside a substitution written inside double quotes.
	var parens []bool
	backquote := false
	inSub := func() bool {
		if backquote {
			return true
		}
		for _, sub := range parens {
			if sub {
				return true
			}
		}
		return false
	}
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
		case r == '\'' && (!double || inSub()):
			single = !single
			out.WriteRune(r)
			i++
			continue
		case r == '"' && !single:
			double = !double
			out.WriteRune(r)
			i++
			continue
		case r == '`' && !single:
			backquote = !backquote
			out.WriteRune(r)
			i++
			continue
		case r == '(' && !single:
			parens = append(parens, i > 0 && src[i-1] == '$')
			out.WriteRune(r)
			i++
			continue
		case r == ')' && !single && len(parens) > 0:
			parens = parens[:len(parens)-1]
			out.WriteRune(r)
			i++
			continue
		case c.CommentStops && c.Comment != 0 && r == c.Comment && !single && !double && (i == 0 || isBlank(byte(src[i-1])) || isOperatorByte(byte(src[i-1]))):
			// The rest of the line is a comment and is left as written.
			out.WriteString(string(src[i:]))
			i = len(src)
			continue
		case r == c.Event && !single && (!double || !c.DoubleQuotesProtect):
			if literal(src, i, c, double) {
				out.WriteRune(r)
				i++
				continue
			}
			text, next, print, err := one(src, i, out.String(), hist, c, st)
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
// The double quote is here for a reason of its own rather than for bash's
// list: a `!` against a closing quote has an empty event name, and an empty
// name expands to nothing in every shell that has the feature.
//
// The **single** quote and the backquote were here for that same reason and
// are not, which is #3421. Re-measured 2026-09-18 over the same probe, inside
// double quotes and outside them alike: `echo T!'xE'` is `!'xE': event not
// found` in bash 5.3.20 and ksh93u+, and `echo T!` with a backquote after it
// is a reference in both — so the empty-name reasoning was a guess that the
// probe's own quoting had never disturbed. zsh 5.9.2 is the column that
// leaves both alone, which is Chars.QuoteIsText.
const noExpandAfter = " \t\n\r=|&;()<>\""

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
	if c.QuoteIsText && (src[i+1] == '\'' || src[i+1] == '`') {
		return true
	}
	if i > 0 && src[i-1] == '[' {
		return true
	}
	if i > 1 && src[i-1] == '{' && src[i-2] == '$' {
		return true
	}
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
	// cannot be read as part of it — where a shell has the form at all.
	// Measured 2026-09-18 (#3220): `X!{!!}Y` is `!{!!}Y: event not found`
	// in bash 5.3.20 and 3.2.57 from a script and in ksh93u+ at a prompt,
	// both of which take the whole run after the `!` as an event *name*,
	// and `!{x}` is the event `x` in zsh 5.9.2 alone.
	if c.BracedEvent && j < len(src) && src[j] == '{' {
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
		words, raw = fields(sofar, c.Words), sofar
		haveWords = true
		j++
	case j < len(src) && (src[j] == c.Event):
		entry, ok := hist.at(hist.number())
		if !ok {
			return "", 0, false, &NotFound{Ref: ref(j + 1)}
		}
		words, raw = fields(entry, c.Words), entry
		haveWords = true
		j++
	case j < len(src) && isWordDesignator(src[j], c):
		// `!$`, `!^`, `!*` and `!:n` are the previous event with a word
		// designator on it — the event was never written.
		entry, ok := hist.at(hist.number())
		if !ok {
			return "", 0, false, &NotFound{Ref: ref(j + 1)}
		}
		words, raw = fields(entry, c.Words), entry
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
		words, raw = fields(entry, c.Words), entry
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
			words, raw = fields(entry, c.Words), entry
			haveWords = true
			j = digits
			break
		}
		k = j
		if k < len(src) && src[k] == '-' {
			// A leading `-` that no number followed is still the start of
			// the string; only a later one ends it.
			k++
		}
		for k < len(src) && !strings.ContainsRune(eventEnd, src[k]) && src[k] != '-' {
			// A `-` ends the string because it begins a word range with
			// no colon: measured 2026-09-16, `!ech-2` after `echo a b c d`
			// is `echo a b` in bash 5.3.20, zsh 5.9.2 and ksh93u+ alike.
			if src[k] == c.Event {
				// An event character inside a name is part of it in two
				// of the three columns, and closes the name — itself
				// included — in the third. Measured 2026-09-18 with
				// `X!ab!cdY`: `!ab!cdY: event not found` in bash 5.3.20
				// from a script and in ksh93u+ at a prompt, `event not
				// found: ab!` in zsh 5.9.2 at one. See
				// Chars.EventCharClosesAnEventName.
				if c.EventCharClosesAnEventName {
					k++
					break
				}
			}
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
		words, raw = fields(entry, c.Words), entry
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
	text, j, print, err := modifiers(src, j, text, st, c)
	if err != nil {
		return "", 0, false, err
	}
	return text, j, print, nil
}

// eventEnd are the characters that end a `!string` event reference. Whitespace
// and the word-designator separator, plus the operators a command line is
// built out of — a `!vi` at the end of `foo && !vi` names `vi` and not
// `vi` with the shell's own punctuation glued on.
//
// A backslash is **not** one of them, which is #3421. Measured 2026-09-18 in
// a script and again at a prompt, `echo T!\xE` is `!\xE: event not found` in
// bash 5.3.20, zsh 5.9.2 and ksh93u+ alike, and `echo "T!\x E"` names `!\x`
// in all three — so the character the panel agrees is ordinary text before a
// `!` (`echo a\!b`, which the scanner still steps over) was being read as
// punctuation after one. A single quote and a backquote are not here either,
// for the reason noExpandAfter gives; see Chars.QuoteIsText for the column
// that leaves a `!` against one alone instead.
const eventEnd = " \t\n\r:^$*%=|&;()<>\""

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

// Words is how an event is cut into the words a designator indexes.
//
// Not the shell's own word splitting — it runs over text nothing has expanded
// — and not one answer either. Measured 2026-09-16 by recalling single words
// of one command: bash 5.3.20 from a script with `history -p`, zsh 5.9.2 and
// ksh93u+ at a prompt through a pseudo-terminal with `:q` on the word.
//
//	event                  bash            zsh             ksh93
//	echo "a b"c d      :1  "a b"c          "a b"c          "a b"c
//	echo a\ b c        :1  a\ b            a\ b            a\
//	echo $(echo x y) z :1  $(echo x y)     $(echo x y)     $(echo
//	echo ${v:-a b} z   :1  ${v:-a          ${v:-a b}       ${v:-a
//	echo x;echo b      :2  ;               ;               b (x;echo is :1)
//	echo a 2>/dev/null :2  2>              2>              2>/dev/null
//
// So a quoted string is one word everywhere, and the rest is three readings.
type Words uint8

const (
	// WordsQuotes cuts at blanks outside a single- or double-quoted string,
	// and nowhere else: ksh93's reading, and the zero value because it is
	// the part all three agree on.
	WordsQuotes Words = iota
	// WordsShell is bash's: a backslash, a command or arithmetic
	// substitution, a backquoted one and a process substitution each hold
	// their blanks, and an operator is a word of its own — `;`, `|`, `&&`,
	// `>`, `>>`, `&>`, `>|`, `2>&1`, with a numeral standing alone in front
	// of a redirection joining it (`12>`), measured. A `(` or `)` standing
	// alone is a word too, which is what a `case` pattern's closer is.
	WordsShell
	// WordsShellBraces is zsh's: WordsShell, and a `${ }` holds its blanks
	// as well.
	WordsShellBraces
)

// fields splits an entry into the words a designator indexes, the way rule
// reads it.
func fields(entry string, rule Words) []string {
	if rule == WordsQuotes {
		return quoteFields(entry)
	}
	return shellFields(entry, rule == WordsShellBraces)
}

func isBlank(b byte) bool { return b == ' ' || b == '\t' || b == '\n' }

// quoteFields is WordsQuotes.
func quoteFields(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		for i < len(s) && isBlank(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		for i < len(s) && !isBlank(s[i]) {
			if s[i] == '\'' || s[i] == '"' {
				i = closeQuote(s, i)
			}
			i++
		}
		out = append(out, s[start:min(i, len(s))])
	}
	return out
}

// closeQuote is the index of the quote that closes the one opened at i, or
// the last index of s where nothing does. A backslash escapes the next
// character inside double quotes only.
func closeQuote(s string, i int) int {
	q := s[i]
	for j := i + 1; j < len(s); j++ {
		switch {
		case q == '"' && s[j] == '\\':
			j++
		case s[j] == q:
			return j
		}
	}
	return len(s) - 1
}

// closeParen is the index of the `)` that balances the `(` at i, stepping
// over quotes, or the last index of s where nothing does.
func closeParen(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '\'', '"', '`':
			j = closeQuote(s, j)
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(s) - 1
}

func isOperatorByte(b byte) bool {
	return b == ';' || b == '&' || b == '|' || b == '<' || b == '>' || b == '(' || b == ')'
}

// operatorsByLength are the operators WordsShell reads as one word, longest
// first so that `>>` is not read as two.
var operatorsByLength = []string{"<<<", "<<-", "&>>", ";;", "&&", "||", ">>", "<<", "&>", ">|", ">&", "<&", "<>"}

// operatorEnd is the index just past the operator beginning at i. A `>&` or
// `<&` takes the descriptor after it, and a `<(` or `>(` is not an operator —
// the caller has already taken those.
func operatorEnd(s string, i int) int {
	for _, op := range operatorsByLength {
		if strings.HasPrefix(s[i:], op) {
			end := i + len(op)
			if op == ">&" || op == "<&" {
				for end < len(s) && (s[end] >= '0' && s[end] <= '9' || s[end] == '-') {
					end++
				}
			}
			return end
		}
	}
	return i + 1
}

// shellFields is WordsShell, and WordsShellBraces where braces is set.
func shellFields(s string, braces bool) []string {
	var out []string
	i := 0
	for i < len(s) {
		for i < len(s) && isBlank(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		if isOperatorByte(s[i]) && !startsProcessSubstitution(s, i) {
			i = operatorEnd(s, i)
			out = append(out, s[start:i])
			continue
		}
		for i < len(s) && !isBlank(s[i]) {
			c := s[i]
			if startsProcessSubstitution(s, i) {
				i = closeParen(s, i+1) + 1
				continue
			}
			if isOperatorByte(c) {
				if (c == '<' || c == '>') && allDigits(s[start:i]) {
					// A descriptor number in front of a redirection is
					// part of it.
					i = operatorEnd(s, i)
				}
				break
			}
			switch {
			case c == '\\':
				i += 2
				continue
			case c == '\'' || c == '"' || c == '`':
				i = closeQuote(s, i) + 1
				continue
			case strings.IndexByte("$+@*?!", c) >= 0 && i+1 < len(s) && s[i+1] == '(':
				// A command or arithmetic substitution, and an extended
				// glob's group: measured, `echo /+(one|two)/x y` has
				// `/+(one|two)/x` as its first word. With extglob on: off,
				// the line is a syntax error and never an event at all.
				i = closeParen(s, i+1) + 1
				continue
			case braces && c == '$' && i+1 < len(s) && s[i+1] == '{':
				i = closeBrace(s, i+1) + 1
				continue
			}
			i++
		}
		i = min(i, len(s))
		out = append(out, s[start:i])
	}
	return out
}

// startsProcessSubstitution reports whether s holds `<(` or `>(` at i.
func startsProcessSubstitution(s string, i int) bool {
	return i+1 < len(s) && (s[i] == '<' || s[i] == '>') && s[i+1] == '('
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// closeBrace is closeParen for a `${`.
func closeBrace(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '\'', '"', '`':
			j = closeQuote(s, j)
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(s) - 1
}

// designate applies a word designator, reporting whether one was written.
//
// Measured 2026-09-16 over `echo a b c d e` on bash 5.3.20 from a script, and
// on zsh 5.9.2 and ksh93u+ at a prompt through a pseudo-terminal, where the
// three agree on every rule below:
//
//   - `x-$` runs to the last word — `!!:2-$` is `b c d e`;
//   - a range with no start begins at word zero — `!!:-3` is `echo a b c`,
//     `!!:-` is `echo a b c d` — and so does one written with no colon at
//     all, `!!-3` and `!-2-3` alike;
//   - `*` is not a range start, so `!!:*-` is `a b c d e-` with the `-` left
//     as text;
//   - a word the event does not have is an error that stops the line — `:9`,
//     `:2-9`, `:3-2`, `:9-` and `:9*` are each refused — rather than the
//     nothing this used to put in its place.
func designate(src []rune, j int, words []string, c Chars, st *state) ([]string, int, bool, error) {
	if j >= len(src) {
		return nil, j, false, nil
	}
	from := j
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
	// ranged says a `-` or `*` after the start makes a range of it, which
	// every start but `*` and `%` does.
	ranged := true
	bad := func(to int) error { return &BadWordSpecifier{Ref: string(src[from:to])} }
	switch r := src[j]; {
	case c.Quick != 0 && r == c.Quick:
		start, end = 1, 1
		j++
	case r == '$':
		start, end = last, last
		j++
		if c.LastWordEndsTheDesignator {
			// The last word is the whole designator, so nothing after it
			// is read: measured 2026-09-18 over `echo a b c d e`,
			// `!!:$-3` is `e-3`, `!!:$-` is `e-` and `!!:$*` is `e*` in
			// bash 5.3.20 from a script and in ksh93u+ at a prompt, where
			// zsh 5.9.2 refuses the first two as `no such word in event`
			// and answers the third `e`. See
			// Chars.LastWordEndsTheDesignator.
			ranged = false
		}
	case r == '*':
		if last < 1 {
			return nil, j + 1, true, nil
		}
		start, end = 1, last
		ranged = false
		j++
	case r == '%':
		// The word the search matched in, reading from the end of the event:
		// measured on bash 5.3.20, `!?e?%` against `echo two.three` is
		// `two.three`, not the `echo` in front of it.
		for i := len(words) - 1; i >= 0; i-- {
			if st.matched != "" && strings.Contains(words[i], st.matched) {
				start, end = i, i
				break
			}
		}
		if start < 0 {
			return nil, j + 1, true, nil
		}
		ranged = false
		j++
	case r >= '0' && r <= '9':
		k := j
		for k < len(src) && src[k] >= '0' && src[k] <= '9' {
			k++
		}
		n, _ := strconv.Atoi(string(src[j:k]))
		start, end = n, n
		j = k
	case r == '-':
		// No start written, so the range begins at word zero. The `-` is
		// read by the range below.
		start, end = 0, 0
	default:
		if colon {
			// A `:` that named no word is a modifier's colon; hand it back.
			return nil, j - 1, false, nil
		}
		return nil, j, false, nil
	}
	// A range: `x-y`, `x-$`, `x-` (up to the last but one) or `x*` (up to
	// the last).
	if ranged && j < len(src) && src[j] == '-' {
		k := j + 1
		for k < len(src) && src[k] >= '0' && src[k] <= '9' {
			k++
		}
		switch {
		case k > j+1:
			n, _ := strconv.Atoi(string(src[j+1 : k]))
			if start > last || n > last || start > n {
				return nil, k, false, bad(k)
			}
			return pick(start, n), k, true, nil
		case k < len(src) && src[k] == '$':
			if start > last {
				return nil, k + 1, false, bad(k + 1)
			}
			return pick(start, last), k + 1, true, nil
		case c.FirstWordEndsARange && c.Quick != 0 && k < len(src) && src[k] == c.Quick:
			// The Quick character names word one where a range's end is
			// written, exactly as it does where its start is. Measured
			// 2026-09-18 over `echo a b c d e`: `!!:1-^` is `a` in bash
			// 5.3.20 from a script and in zsh 5.9.2 at a prompt, and
			// `!!:2-^` is a backwards range both refuse; ksh93u+ leaves
			// the character as text and answers `a b c d^`. See
			// Chars.FirstWordEndsARange.
			if start > last || start > 1 {
				return nil, k + 1, false, bad(k + 1)
			}
			return pick(start, 1), k + 1, true, nil
		}
		if start > last {
			return nil, j + 1, false, bad(j + 1)
		}
		if last-1 >= start {
			return pick(start, last-1), j + 1, true, nil
		}
		return nil, j + 1, true, nil
	}
	if ranged && j < len(src) && src[j] == '*' {
		if start > last {
			return nil, j + 1, false, bad(j + 1)
		}
		return pick(start, last), j + 1, true, nil
	}
	if start > last {
		return nil, j, false, bad(j)
	}
	return pick(start, end), j, true, nil
}

// modifiers applies the `:h`, `:t`, `:r`, `:e`, `:p`, `:q`, `:x`, `:s` and
// `:&` chain, and reports whether `:p` was among them.
func modifiers(src []rune, j int, text string, st *state, c Chars) (string, int, bool, error) {
	print := false
	// chain is where the whole chain began, which is what a failed modifier
	// is named after. See chainRef.
	chain := j
	// quote is the `q` or `x` the chain asked for, applied once the rest of
	// it has run where the shell quotes last. See Chars.QuoteInPlace. A
	// second quoting modifier replaces the first.
	var quote rune
	for j < len(src) && src[j] == ':' {
		j++
		if j >= len(src) {
			return "", 0, false, &BadModifier{Mod: ""}
		}
		global, wordwise := false, false
		for j < len(src) && (src[j] == 'g' || src[j] == 'a' || (c.WordwiseSubstitution && src[j] == 'G')) {
			// A `G` stands alone. Measured 2026-09-18, `!!:gGs/o/0/` is
			// `G: unrecognized history modifier` in bash 5.3.20 and
			// `!!:Ggs/o/0/` is `g: unrecognized history modifier`, so the
			// letter that arrives second is the one named.
			if wordwise || (global && src[j] == 'G') {
				return "", 0, false, &BadModifier{Mod: string(src[j])}
			}
			if src[j] == 'G' {
				wordwise = true
			} else {
				global = true
			}
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
		case 'q', 'x':
			quote = src[j]
			j++
			if c.QuoteInPlace {
				text, quote = quoteText(text, quote), 0
			}
		case 's', '&':
			var err error
			text, j, err = substitute(src, j, chain, text, global, wordwise, st)
			if err != nil {
				return "", 0, false, err
			}
		default:
			return "", 0, false, &BadModifier{Mod: string(src[j])}
		}
	}
	return quoteText(text, quote), j, print, nil
}

// quoteText applies `:q` (the whole text as one quoted word) or `:x` (each
// blank-separated word quoted), or nothing for any other letter.
func quoteText(text string, quote rune) string {
	switch quote {
	case 'q':
		return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
	case 'x':
		parts := strings.Fields(text)
		for i, p := range parts {
			parts[i] = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
		}
		return strings.Join(parts, " ")
	}
	return text
}

// substitute applies `s/old/new/` or the `&` that repeats the last one.
//
// The delimiter is whatever character follows the `s`, which is what makes
// `:s,a,b,` work when the text is a path.
func substitute(src []rune, j, chain int, text string, global, wordwise bool, st *state) (string, int, error) {
	if src[j] == '&' {
		j++
		if st.old == "" {
			return "", 0, &NoPreviousSubstitution{Ref: chainRef(src, chain, j)}
		}
		out, err := apply(text, st.old, st.new, global, wordwise, chainRef(src, chain, j))
		return out, j, err
	}
	j++ // past the `s`
	if j >= len(src) {
		ref := chainRef(src, chain, len(src))
		return "", 0, &SubstFailed{Ref: ref, Bare: ref}
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
		old = st.previousOld()
	}
	if old == "" {
		return "", 0, &NoPreviousSubstitution{Ref: chainRef(src, chain, j)}
	}
	repl = replacement(repl, old)
	st.old, st.new = old, repl
	out, err := apply(text, old, repl, global, wordwise, chainRef(src, chain, j))
	return out, j, err
}

// replacement reads the right side of a substitution: an `&` in it is the text
// being replaced, and a backslash before one makes it an ordinary `&`.
//
// Unanimous, measured 2026-09-16 on bash 5.3.20 from a script and on zsh
// 5.9.2 and ksh93u+ at a prompt: after `echo one two one`, `:s/o/&&/` is
// `echoo one two one` in all three, and `:s/o/\&/` puts a lone `&` where the
// `o` was — which, being an `&`, then runs `ech` in the background in both
// columns whose output could be read. Any other backslash stays as written.
func replacement(raw, old string) string {
	if !strings.Contains(raw, "&") {
		return raw
	}
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		switch {
		case raw[i] == '\\' && i+1 < len(raw) && raw[i+1] == '&':
			b.WriteByte('&')
			i++
		case raw[i] == '&':
			b.WriteString(old)
		default:
			b.WriteByte(raw[i])
		}
	}
	return b.String()
}

// apply is the substitution itself, which fails rather than doing nothing when
// the left side is not there: measured, `^nosuch^x^` is `substitution failed`
// in bash, zsh and ksh93 alike and the line does not run.
//
// wordwise is the `G` of Chars.WordwiseSubstitution: one substitution in each
// blank-separated word rather than one in the whole text. Measured 2026-09-18
// on bash 5.3.20 after `echo foo boo`, `!!:Gs/o/0/` is `ech0 f0o b0o` — every
// word changed once, and `foo` keeps its second `o` where a `g` would not.
func apply(text, old, repl string, global, wordwise bool, ref string) (string, error) {
	if !strings.Contains(text, old) {
		return "", &SubstFailed{Ref: ref, Bare: ref}
	}
	if wordwise {
		words := strings.Split(text, " ")
		for i, w := range words {
			words[i] = strings.Replace(w, old, repl, 1)
		}
		return strings.Join(words, " "), nil
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
		old = st.previousOld()
	}
	if old == "" {
		return Result{}, &NoPreviousSubstitution{Ref: ":s" + written}
	}
	if !strings.Contains(entry, old) {
		return Result{}, &SubstFailed{Ref: ":s" + written, Bare: written}
	}
	repl = replacement(repl, old)
	st.old, st.new = old, repl
	out := strings.Replace(entry, old, repl, 1)
	// Anything after the closing character is a modifier chain on the result.
	if i < len(src) {
		var err error
		var print bool
		out, _, print, err = modifiers(src, i, out, st, c)
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
