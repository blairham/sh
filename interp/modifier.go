// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"path/filepath"
	"strings"

	"github.com/blairham/sh/syntax"
)

// A substring range is not always a range.
//
// One shell reads `${x:h}` as a *modifier* — its history-modifier syntax,
// applied to a parameter — and the two spellings share every byte of their
// punctuation, so which one a range is has to be decided before either is
// read. The rule is measured and it is about the first byte and nothing else:
// a segment that begins with an unquoted letter is a modifier, and one that
// does not is an arithmetic expression.
//
//	${x:i:2}     refused — `i' names no modifier
//	${x:abc:2}   refused — and nothing is named, because `a' is one and `bc'
//	             is left over
//	${x:_q:2}    a substring, because `_' is not a letter
//	${x: i:2}    a substring, because the first byte is a space
//	${x:(i):2}   a substring, because the first byte is a parenthesis
//	${x:$i:2}    a substring, because the expansion happened first
//	${x:"h"}     a substring, because the letter was quoted
//
// This was silent, and `${x:i:n}` inside a loop is the ordinary way to walk a
// string: the substring came back where the shell refuses at 1, so a script
// that shell would have stopped ran on with a plausible value.

// modifierLetters is what the shell that has them accepts, measured a letter
// at a time. Fourteen, and the fourteenth is not a letter: `:&` repeats the
// last substitution.
//
// The value is what may follow the letter inside the same segment, because
// that is the only thing about a segment this table cannot say twice. Most
// take nothing at all; `h` and `t` take a count; `s` takes a whole
// substitution and `&` takes the one before it.
var modifierLetters = map[byte]modifierArg{
	'h': modifierCount,   // head — everything before the last slash
	't': modifierCount,   // tail — everything after it
	'r': modifierNothing, // root — the value with its suffix taken off
	'e': modifierNothing, // extension — the suffix, without its dot
	'l': modifierNothing, // lowercase
	'u': modifierNothing, // uppercase
	'a': modifierNothing, // an absolute path, against the working directory
	'A': modifierNothing, // and the same with the links resolved
	'P': modifierNothing, // the real path, which applies `..` after resolving
	'c': modifierNothing, // a command's path, from the command search
	'q': modifierNothing, // quoted, in this shell's own quoting
	'Q': modifierNothing, // and unquoted, read back
	's': modifierSubst,   // substitution, which takes a delimited pair
	'&': modifierNothing, // the substitution before it, again
}

// modifierArg is what a letter may carry after it inside its own segment.
type modifierArg int

const (
	// modifierNothing: the letter is the whole segment, and anything after it
	// is the complaint that names nothing.
	modifierNothing modifierArg = iota
	// modifierCount: an optional decimal count, which `h` and `t` alone take.
	// `${x:h2}` is the head twice over and `${x:h:2}` is a `2` that names no
	// modifier — the digit belongs to the letter or to nothing.
	modifierCount
	// modifierSubst: a delimited pattern and replacement, `s/l/r/`, whose
	// delimiter is whatever byte follows the letter.
	modifierSubst
)

// rangeSegmentIsAModifier reports whether a range segment is read as a
// modifier rather than as an expression.
//
// The first byte, unquoted and literal. Quoting is what separates `${x:h}`
// from `${x:"h"}`, and the second is arithmetic on an unset `h` in that shell
// as it is everywhere else — so the test is on the word as written and not on
// what it expands to.
func rangeSegmentIsAModifier(w *syntax.Word) bool {
	if w == nil || len(w.Spans) == 0 {
		return false
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted || s.Value == "" {
		return false
	}
	c := s.Value[0]
	// `&` is the fourteenth modifier — the last substitution again — and is
	// the one that is not a letter. It cannot begin an expression either, so
	// reading it this way takes nothing away from the other spelling.
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '&'
}

// modifierSegments is the modifier list a range holds.
//
// The parser splits a range once, at its first colon, so a list of three
// arrives as one word and a word holding the rest — `${x:h:t:r}` is `h` and
// `t:r`. The two are put back together and re-split here rather than in the
// parser, which keeps the range's shape a question about this dialect and not
// about the grammar.
//
// It is not a split on `:`, and that is the whole reason this is a scanner.
// `s` takes its delimiter from the byte after it, and that byte may itself be
// a colon: `${x:s:Dir:OTHER:}` is one modifier, and splitting on colons first
// turns it into four things none of which is one. Measured — every byte
// serves as a delimiter, `/ | # , :` alike.
func modifierSegments(first, rest *syntax.Word) []string {
	text := first.Literal()
	if rest != nil {
		text += ":" + rest.Literal()
	}
	var segs []string
	for {
		seg, remainder, more := scanOneModifier(text)
		segs = append(segs, seg)
		if !more {
			return segs
		}
		text = remainder
	}
}

// scanOneModifier reads one modifier off the front of text, answering it, what
// is left after its colon, and whether there was one.
func scanOneModifier(text string) (seg, rest string, more bool) {
	i := 0
	if i+1 < len(text) && text[i] == 'g' {
		i++
	}
	if i < len(text) && text[i] == 's' && i+1 < len(text) {
		// A substitution: the delimiter is the byte after the letter, and the
		// modifier runs to the closing delimiter of its second field. What
		// follows that is the next modifier, after its colon.
		end := endOfSubstitution(text, i+1)
		if end >= len(text) {
			return text, "", false
		}
		if text[end] == ':' {
			return text[:end], text[end+1:], true
		}
		// Something other than a colon after a complete substitution, which
		// is not a separator — leave it in the segment and let the parse of
		// it complain, naming nothing, the way any other trailing text does.
		return text, "", false
	}
	if j := strings.IndexByte(text[i:], ':'); j >= 0 {
		return text[:i+j], text[i+j+1:], true
	}
	return text, "", false
}

// endOfSubstitution is the offset just past a substitution body that begins
// with its delimiter at from. It is len(text) where the body ran out, which
// the parse of it reports.
func endOfSubstitution(text string, from int) int {
	delim := text[from]
	i := from + 1
	for fields := 0; fields < 2; fields++ {
		for i < len(text) {
			if text[i] == '\\' && i+1 < len(text) {
				i += 2
				continue
			}
			if text[i] == delim {
				break
			}
			i++
		}
		if i >= len(text) {
			// The closing delimiter is optional on the *second* field, so
			// running out there is a complete substitution and running out in
			// the first is not. Either way there is nothing after it.
			return len(text)
		}
		i++
	}
	return i
}

// applyModifiers runs a modifier list over a value, left to right.
//
// A segment is one modifier and its letter is the whole of it. Anything left
// over in the segment is a failure, and the shell that has these says so
// without naming anything — measured: `${x:ha}` complains and names nothing
// where `${x:i}` names `i`, because in the first the letter *was* a modifier
// and it is the remainder that is not.
func (r *Runner) applyModifiers(value string, segs []string, e *syntax.ParamExpr) (string, bool) {
	for _, seg := range segs {
		next, ok := r.applyModifierSegment(value, seg, e)
		if !ok {
			return "", false
		}
		value = next
	}
	return value, true
}

// applyModifierSegment reads one segment and performs it.
//
// The shape is `[g]<letter>[argument]`, and the `g` is a prefix rather than a
// letter of its own: `${x:g}` names `g` as no modifier, `${x:gh}` is the head,
// and it changes the answer only for `s` and `&`, where it means every
// occurrence instead of the first.
func (r *Runner) applyModifierSegment(value, seg string, e *syntax.ParamExpr) (string, bool) {
	global := false
	if len(seg) > 1 && seg[0] == 'g' {
		global, seg = true, seg[1:]
	}
	arg, known := modifierArg(0), false
	if seg != "" {
		arg, known = modifierLetters[seg[0]]
	}
	if !known {
		// Unrecognized, and *one byte* is named — `${x:zz}` names `z`, not
		// `zz`, because the complaint is about the letter and the rest has
		// not been looked at. An empty segment names nothing, having no
		// letter to name.
		if seg == "" {
			r.refuseModifier(e, "")
		} else {
			r.refuseModifier(e, seg[:1])
		}
		return "", false
	}
	letter, rest := seg[0], seg[1:]
	switch arg {
	case modifierNothing:
		if rest != "" {
			// The letter was a modifier and what follows it is not, which is
			// the same complaint with nothing named.
			r.refuseModifier(e, "")
			return "", false
		}
		if letter == '&' {
			return r.repeatSubstitution(value, global, e)
		}
		return r.applyModifier(value, letter, e)
	case modifierCount:
		n, ok := modifierCountOf(rest)
		if !ok {
			r.refuseModifier(e, "")
			return "", false
		}
		if n == 0 {
			return applyPureModifier(value, letter), true
		}
		if letter == 'h' {
			return modifierHeadCount(value, n), true
		}
		return modifierTailCount(value, n), true
	default:
		return r.substituteModifier(value, rest, global, e)
	}
}

// modifierCountOf reads the count after `h` or `t`, where 0 stands for "none
// given". Nothing and `0` are the same answer and `1` is a different one,
// which is measured: on `/a/b/c/d/e`, `:h` and `:h0` are both `/a/b/c/d` and
// `:h1` is `/`.
func modifierCountOf(rest string) (int, bool) {
	if rest == "" {
		return 0, true
	}
	n := 0
	for i := 0; i < len(rest); i++ {
		if rest[i] < '0' || rest[i] > '9' {
			return 0, false
		}
		n = n*10 + int(rest[i]-'0')
		if n > len(rest)+maxPathComponents {
			// Any count past the number of separators a path can hold is the
			// same answer as one that runs off the end, and stopping here
			// keeps a long digit run from overflowing on the way to it.
			return maxPathComponents, true
		}
	}
	return n, true
}

// maxPathComponents is where counting stops being a different answer.
const maxPathComponents = 1 << 20

// modifierHeadCount is `:h<n>` — everything before the n-th separator from
// the left, and the value unchanged where there is no n-th one.
//
// Not `:h` applied n times, which is what it looks like and is a different
// answer at every n but 3 on a five-part path: `${x:h1}` on `/a/b/c/d/e` is
// `/`, where the head three times over is `/a/b`. It counts *from the left*.
//
// A separator is a run of slashes with something after it. A trailing run
// separates nothing, so `/a/b//` has two and `:h3` on it is the whole value —
// which is also why this cannot be a count of slash characters.
func modifierHeadCount(s string, n int) string {
	seen := 0
	for i := 0; i < len(s); {
		if s[i] != '/' {
			i++
			continue
		}
		run := i
		for i < len(s) && s[i] == '/' {
			i++
		}
		if i == len(s) {
			// Trailing, so it separates nothing.
			break
		}
		seen++
		if seen == n {
			if run == 0 {
				// The leading separator is the root, which is its own head.
				return "/"
			}
			return s[:run]
		}
	}
	return s
}

// modifierTailCount is `:t<n>` — everything after the n-th separator from the
// right, with the trailing slashes taken off first the way `:t` takes them.
// The trimmed value where there is no n-th separator.
func modifierTailCount(s string, n int) string {
	t := strings.TrimRight(s, "/")
	seen := 0
	for i := len(t) - 1; i >= 0; {
		if t[i] != '/' {
			i--
			continue
		}
		end := i
		for i >= 0 && t[i] == '/' {
			i--
		}
		seen++
		if seen == n {
			return t[end+1:]
		}
	}
	return t
}

// refuseModifier reports a modifier the dialect does not have. name is empty
// where the shell names nothing.
func (r *Runner) refuseModifier(e *syntax.ParamExpr, name string) {
	d := r.diag()
	sentence := Wording(d.UnrecognizedModifierAlone, "unrecognized modifier")
	if name != "" {
		sentence = Wording(d.UnrecognizedModifier, "unrecognized modifier: %[1]s", name)
	}
	r.diagf("%s\n", Wording(d.SubstringRangeError, "%[2]s", r.paramSubject(e), sentence))
	r.expandErr = true
}

// applyModifier performs one modifier.
//
// A method rather than a function because seven of the thirteen need
// something a string does not carry — the working directory, the disk, the
// command search, this shell's own quoting — and there is no honest way to
// answer those from the value alone. It reports rather than returning the
// value untouched, because handing back a value that was never computed is
// the silent kind of wrong.
func (r *Runner) applyModifier(value string, letter byte, e *syntax.ParamExpr) (string, bool) {
	switch letter {
	case 'a':
		return r.modifierAbsolute(value), true
	case 'A':
		// Lexical first and the disk second, which is the whole difference
		// from `:P`: `link/..` is the link's parent by name here and the
		// parent of what it points at there.
		if value == "" {
			return "", true
		}
		return r.physicalPrefix(filepath.Clean(r.absoluteAgainstCwd(value))), true
	case 'P':
		return r.modifierRealPath(value), true
	case 'c':
		return r.modifierCommandPath(value), true
	case 'q':
		// quoteWithBackslashes answers `''` for an empty value, which is what
		// the `(q)` expansion flag wants and not what the modifier does:
		// measured in one shell, `${x:q}` on empty is empty and `${(q)x}` is
		// `''`.
		if value == "" {
			return "", true
		}
		return quoteWithBackslashes(value), true
	case 'Q':
		return r.unquoteFlagged(value), true
	}
	return applyPureModifier(value, letter), true
}

// applyPureModifier performs one of the six that are a pure function of the
// string, which is also the set a count may repeat.
func applyPureModifier(value string, letter byte) string {
	switch letter {
	case 'h':
		return modifierHead(value)
	case 't':
		return modifierTail(value)
	case 'r':
		return modifierRoot(value)
	case 'e':
		return modifierExtension(value)
	case 'l':
		return strings.ToLower(value)
	case 'u':
		return strings.ToUpper(value)
	}
	return value
}

// modifierHead is `:h` — everything before the last slash, with the trailing
// slashes taken off first, and `.` where there is no slash at all.
//
// Measured: `/a/b//` → `/a`, `a//b` → `a`, `a/` → `.`, `/` → `/`, “ → `.`.
func modifierHead(s string) string {
	t := strings.TrimRight(s, "/")
	if t == "" {
		if s == "" {
			return "."
		}
		// Nothing but slashes, which is the root and is its own head.
		return "/"
	}
	i := strings.LastIndexByte(t, '/')
	if i < 0 {
		return "."
	}
	// A run of slashes belongs to neither side.
	if h := strings.TrimRight(t[:i], "/"); h != "" {
		return h
	}
	return "/"
}

// modifierTail is `:t` — everything after the last slash, with the trailing
// slashes taken off first. Measured: `/a/b//` → `b`, `/` → nothing.
func modifierTail(s string) string {
	t := strings.TrimRight(s, "/")
	if i := strings.LastIndexByte(t, '/'); i >= 0 {
		return t[i+1:]
	}
	return t
}

// suffixDot is where `:r` cuts and `:e` begins: the last dot in the part after
// the last slash, counting one that begins that part.
//
// Counting the leading dot is why this is not "the last dot with something in
// front of it". Measured: `.hidden` has an empty root and an extension of
// `hidden`, and `x/.hidden` has a root of `x/`.
func suffixDot(s string) int {
	from := strings.LastIndexByte(s, '/') + 1
	if i := strings.LastIndexByte(s[from:], '.'); i >= 0 {
		return from + i
	}
	return -1
}

// modifierRoot is `:r` — the value with its suffix taken off, and unchanged
// where the last part has no dot at all: `a/b.c/d` keeps its `b.c`.
func modifierRoot(s string) string {
	if i := suffixDot(s); i >= 0 {
		return s[:i]
	}
	return s
}

// modifierExtension is `:e` — what `:r` takes off, without its dot, and
// nothing where there is no suffix.
func modifierExtension(s string) string {
	if i := suffixDot(s); i >= 0 {
		return s[i+1:]
	}
	return ""
}

// absoluteAgainstCwd puts a relative value on the *physical* working
// directory, which is what the shell with these modifiers uses.
//
// Measured: with `/tmp` a link to `/private/tmp`, `cd /tmp; ${x:a}` on `rel/f`
// answers `/private/tmp/rel/f`, and setting `PWD` to something invented does
// not change it. So it is neither `$PWD` nor the Runner's logical directory
// after a `cd` through a link — it is where the shell physically is.
//
// The resolution is skipped where the value is already absolute, which is
// most of the time, so `:a` touches the disk only when it has to.
func (r *Runner) absoluteAgainstCwd(value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(r.physicalPrefix(r.workDir()), value)
}

// modifierAbsolute is `:a` — an absolute path, made lexically. `..` and `.`
// are cancelled by name and no symlink is followed, which is the whole
// difference from `:A`.
//
// An empty value stays empty rather than becoming the working directory.
// That is `:P`'s answer and not this one, and the two are measured apart.
func (r *Runner) modifierAbsolute(value string) string {
	if value == "" {
		return ""
	}
	return filepath.Clean(r.absoluteAgainstCwd(value))
}

// modifierRealPath is `:P` — the real path, with `..` applied to what has
// already been resolved rather than cancelled by name.
//
// Two things separate it from `:A`, both measured. An empty value is the
// working directory here and stays empty there. And a trailing slash survives
// on a path that could not be fully resolved: `/no/such/` keeps it, where a
// path that does resolve loses it in both.
func (r *Runner) modifierRealPath(value string) string {
	if value == "" {
		return r.physicalPrefix(r.workDir())
	}
	out := r.physicalPrefix(r.absoluteAgainstCwd(value))
	if strings.HasSuffix(value, "/") && !strings.HasSuffix(out, "/") {
		// Only where something was left unresolved — a path that resolved
		// all the way is a directory this walk named without one.
		if _, err := r.lstat(out); err != nil {
			out += "/"
		}
	}
	return out
}

// modifierCommandPath is `:c` — the path the command search would find, and
// the value unchanged where it would find nothing.
//
// Silently unchanged, which is measured and is the opposite of what `=cmd`
// does with the same lookup: that reports and abandons the expansion, and
// this is a modifier whose answer for "no such command" is the name.
//
// Three narrower differences from the ordinary search, all measured:
//
//   - A name containing a slash is left exactly as written. `./mycmd` stays
//     `./mycmd` where the search would make it absolute.
//   - A function is not a command here. `myfn(){ :; }; ${x:c}` on `myfn`
//     answers `myfn`, and a builtin resolves to the *external* file of that
//     name — `echo` is `/bin/echo`.
//   - An empty PATH finds nothing, where the same shell's command *lookup*
//     runs a `mycmd` sitting in the current directory. So this cannot go
//     through pathElements, whose empty-PATH reading is the other one.
func (r *Runner) modifierCommandPath(value string) string {
	if value == "" || strings.ContainsRune(value, '/') {
		return value
	}
	path, _ := r.getVar("PATH")
	if path == "" {
		return value
	}
	for _, dir := range strings.Split(path, ":") {
		if dir == "" {
			continue
		}
		candidate := r.absolute(filepath.Join(dir, value))
		if err := r.runnable(candidate); err == nil {
			return filepath.Join(dir, value)
		}
	}
	return value
}
