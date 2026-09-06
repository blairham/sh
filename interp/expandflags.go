// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"
)

// The parenthesized expansion flags: `${(U)x}` and its family. One grammar in
// the panel has the construct, so what it means here is that shell's answer,
// measured and recorded in docs/spec/grammar/parameter-expansion.md; the
// vendor manual's rule list gives the order the steps run in, and the steps
// below carry the rule numbers they implement.

// implementedParamFlags are the flag letters this slice carries. Anything
// else the grammar accepted is refused *by name* when the expansion is
// reached, because the only thing worse than refusing a flag is answering it
// wrong with status 0.
const implementedParamFlags = "ULfsj@kvP%qMuoOniaQcwW"

// expandFlagged answers an expansion that carries a flag group, as fields.
// It reports false only when the node carries no group, so the ordinary
// paths stay exactly as they were.
func (r *Runner) expandFlagged(s syntax.Span) ([]string, bool) {
	e := s.Param
	if e == nil || !e.HasFlags {
		return nil, false
	}
	quoted := s.Quoting != syntax.Unquoted
	words, isList, ok := r.flaggedWords(e, quoted)
	if !ok {
		return nil, true
	}
	if !isList {
		v := words[0]
		if quoted {
			return []string{globEscape(v)}, true
		}
		if v == "" {
			// An unquoted expansion of an empty value is no field at all.
			return nil, true
		}
		if !r.ask(r.sem().GlobExpansionResults, "globbing the result of an expansion") {
			v = globEscape(v)
		}
		return []string{v}, true
	}
	// Empty words are removed from a list result — measured on both sides:
	// `${(s.:.)x}` on `a::b` is two words however it is quoted, and only
	// `"${(@s.:.)x}"` keeps the third. `$@` and an `[@]` subscript keep
	// their empties in quotes without needing the flag, exactly as they do
	// without one.
	keepEmpty := quoted && r.flagKeepsFields(e)
	out := make([]string, 0, len(words))
	for _, w := range words {
		if w == "" && !keepEmpty {
			continue
		}
		if quoted || !r.ask(r.sem().GlobExpansionResults, "globbing the result of an expansion") {
			w = globEscape(w)
		}
		out = append(out, w)
	}
	return out, true
}

// flaggedWords runs the flag pipeline and returns the resulting words, raw.
// ok is false when the expansion failed and the failure has been reported.
func (r *Runner) flaggedWords(e *syntax.ParamExpr, quoted bool) (words []string, isList, ok bool) {
	if e.FlagsErrPos > 0 {
		// A character the group could not carry, deferred here by the
		// parser: reached in a branch never taken, it is no error at all,
		// which is measured. The position counts from the `$`.
		r.diagf("%s\n", Wording(r.diag().ExpansionFlagsError,
			"error in flags near position %[1]d in '%[2]s'",
			e.FlagsErrPos, "${"+e.Src+"}"))
		r.expandErr = true
		return nil, false, false
	}
	for _, c := range e.Flags {
		if !strings.ContainsRune(implementedParamFlags, c) {
			r.diagf("${%s}: the (%c) expansion flag is not implemented\n", e.Src, c)
			r.expandErr = true
			return nil, false, false
		}
	}
	if strings.Count(e.Flags, "q") > 4 {
		r.diagf("${%s}: the (%s) expansion flag is not implemented\n",
			e.Src, strings.Repeat("q", strings.Count(e.Flags, "q")))
		r.expandErr = true
		return nil, false, false
	}

	words, set, isList := r.flagBase(e)

	// Rule 4: (P) treats the value so far as a further name, before any
	// operator runs — `${(P)x:-def}` tests the *resolved* value.
	if strings.ContainsRune(e.Flags, 'P') {
		words, set, isList = r.namedBase(strings.Join(words, " "), "")
	}

	if !set && e.Name != "" {
		r.checkNounset(e)
	}

	// Rule 5: in double quotes the words are joined — with the `j`
	// separator when one was given, else the first character of IFS —
	// unless the fields were asked for.
	//
	// A length is asked of the words *before* this, which is measured and is
	// not what the rule numbers suggest: `"${(U)#a}"` on `(abc de f)` is 3,
	// the element count, and not 8, the length of the joined text. A `j`
	// separator does not reach the count either — `"${(Uj.-.)#a}"` is 3 as
	// well — so the join is skipped rather than undone, and `(c)` reads a
	// separator of its own where it wants one.
	joined := false
	if quoted && isList && !e.Length && !r.flagKeepsFields(e) {
		words = []string{strings.Join(words, r.flagJoinSep(e))}
		isList = false
		joined = true
	}

	// Rule 7: the operator, applied to the value at this level. Measured:
	// the flags apply to what the operator leaves — `${(U)x:-def}` is DEF,
	// `${(U)u:=def}` assigns def and substitutes DEF.
	words, isList, ok = r.applyFlagOp(e, words, set, isList)
	if !ok {
		return nil, false, false
	}

	// Rule 9: length — the element count for a list and the value's own
	// length for a scalar, unless `c`, `w` or `W` said to count something
	// else. See lengthflags.go.
	if e.Length {
		words, isList = []string{itoa(r.flaggedLength(e, words, isList))}, false
	}

	hasSplit := strings.ContainsAny(e.Flags, "fs")
	// Rule 10: forced joining, ahead of a split — `${(s.:.)a}` on an array
	// joins its elements with IFS's first character and splits the result.
	if (strings.ContainsRune(e.Flags, 'j') || hasSplit) && !joined && isList {
		words = []string{strings.Join(words, r.flagJoinSep(e))}
		isList = false
	}

	// Rule 11: splitting. `f` is split-at-newlines; an empty `s` separator
	// splits into characters, which is measured.
	if hasSplit {
		var split []string
		for _, w := range words {
			split = append(split, splitFlagged(w, e)...)
		}
		words, isList = split, true
	}

	// Rules 12, 13, 14 in the manual's order: case, prompt escapes, quoting.
	for _, c := range e.Flags {
		if c == 'U' || c == 'L' {
			for i, w := range words {
				words[i] = r.convertCase(w, c == 'U')
			}
		}
	}
	if strings.ContainsRune(e.Flags, '%') {
		for i, w := range words {
			v, pok := r.promptEscapes(w, e)
			if !pok {
				return nil, false, false
			}
			words[i] = v
		}
	}
	if n := strings.Count(e.Flags, "q"); n > 0 {
		for i, w := range words {
			words[i] = quoteFlagged(w, n)
		}
	}
	// Rule 14's other half: `Q` takes one level of quoting *off*. The manual
	// lists it beside `q` and measurement says which of the two runs first —
	// `${(Qq)v}` and `${(qQ)v}` on `'a b'` are both `'a b'`, the round trip,
	// where a `Q` that ran first would have left `a\ b`. See quoteflag.go.
	if strings.ContainsRune(e.Flags, 'Q') {
		for i, w := range words {
			words[i] = r.unquoteFlagged(w)
		}
	}
	// The ordering step is last of all, which is *later* than the rule
	// numbers suggest and later than this file used to put it. Three
	// measurements fix it there rather than one:
	//
	//	a=(B a);        ${(@oU)a}   A B          after the case conversion
	//	a=("%x" "*");   ${(@%o)a}   * <path>     after the prompt escapes
	//	a=("a b" "a!"); ${(@qo)a}   a! a\ b      after the quoting
	//	a=("'z'" b);    ${(@Qo)a}   b z          and after the unquoting
	//
	// The last two are the ones that would be got wrong by reading the rule
	// list: `*` sorts ahead of a path only once `%x` has become one, and
	// `a!` ahead of `a\ b` only once the space has become a backslash —
	// both orders reverse if the sort runs first.
	if orderApplies(e) {
		words = orderWords(e, words)
	}
	return words, isList, true
}

// flagKeepsFields reports whether a double-quoted result keeps one field per
// word: the `@` flag asks for it, and `$@` and an `[@]` subscript already
// have it — `"${(U)@}"` keeps its fields exactly as `"$@"` does.
func (r *Runner) flagKeepsFields(e *syntax.ParamExpr) bool {
	if strings.ContainsRune(e.Flags, '@') || e.Name == "@" {
		return true
	}
	return e.Index != nil && r.subscriptText(e.Index) == "@"
}

// flagJoinSep is what joining uses: the `j` argument when one was given, and
// the first character of IFS — a space by default — when not.
func (r *Runner) flagJoinSep(e *syntax.ParamExpr) string {
	if strings.ContainsRune(e.Flags, 'j') {
		return e.JoinSep
	}
	return ifsFirst(r.ifs())
}

// splitFlagged splits one word the way the group asked: `f` at newlines, `s`
// at its separator, and an empty separator into characters.
func splitFlagged(w string, e *syntax.ParamExpr) []string {
	sep := "\n"
	if strings.ContainsRune(e.Flags, 's') {
		sep = e.SplitSep
	}
	if sep == "" {
		out := make([]string, 0, len(w))
		for _, c := range w {
			out = append(out, string(c))
		}
		return out
	}
	return strings.Split(w, sep)
}

// flagBase is the value the pipeline starts from: the words, whether the
// parameter was set, and whether the value is a list rather than a scalar.
func (r *Runner) flagBase(e *syntax.ParamExpr) (words []string, set, isList bool) {
	if e.Inner != nil {
		// An expansion standing where a name would. The flags then apply to
		// what it came to, which is the same rule they follow for a name —
		// `${(U)${v}}` and `${${(U)v}}` are both `ABC`, measured.
		words, set = r.nestedWords(e)
		return words, set, false
	}
	if e.Index != nil {
		if list, lok := r.arraySubscript(e); lok {
			if wholeArraySubscript(r.subscriptText(e.Index)) {
				return list, list != nil, true
			}
			return []string{strings.Join(list, " ")}, list != nil, false
		}
	}
	return r.namedBase(e.Name, e.Flags)
}

// namedBase resolves a name to its words. flags matters for an associative
// array, where `k` substitutes the keys and `kv` key and value as two
// consecutive words each; the keys come sorted, the same deterministic order
// `${m[@]}` already yields where the shells promise none at all.
func (r *Runner) namedBase(name, flags string) (words []string, set, isList bool) {
	switch name {
	case "@", "*":
		return append([]string(nil), r.Params...), len(r.Params) > 0, true
	case "":
		return []string{""}, false, false
	}
	if a, aok := r.AssocArrays[name]; aok {
		hasK := strings.ContainsRune(flags, 'k')
		hasV := strings.ContainsRune(flags, 'v')
		switch {
		case hasK && hasV:
			keys := a.keys()
			out := make([]string, 0, 2*len(keys))
			for _, k := range keys {
				out = append(out, k, a[k])
			}
			return out, len(a) > 0, true
		case hasK:
			return a.keys(), len(a) > 0, true
		default:
			return a.values(), len(a) > 0, true
		}
	}
	if elems, pok := r.pipelineStatuses(name); pok {
		return elems, true, true
	}
	if _, aok := r.Arrays[name]; aok {
		elems, _ := r.arrayElems(name)
		return elems, true, true
	}
	if produce, dok := r.DynamicArrays[name]; dok {
		return produce(r), true, true
	}
	if v, sok := r.specialParam(&syntax.ParamExpr{Name: name}); sok {
		return []string{v}, true, false
	}
	v, vok := r.getVar(name)
	return []string{v}, vok, false
}

// applyFlagOp runs the expansion's operator over the flagged value — the
// same operators expandParam applies, elementwise where the value is a list.
// ok is false when the expansion was fatal.
func (r *Runner) applyFlagOp(e *syntax.ParamExpr, words []string, set, isList bool) ([]string, bool, bool) {
	fires := !set
	if e.Colon {
		fires = !set || strings.Join(words, "") == ""
	}
	switch e.Op {
	case syntax.ParamNone:
	case syntax.ParamDefault:
		if fires {
			return []string{r.joinWord(e.Arg)}, false, true
		}
	case syntax.ParamAssign:
		if fires {
			v := r.joinWord(e.Arg)
			// The side effect stores the word as written; the flags apply
			// only to what is substituted — measured, `${(U)u:=def}` leaves
			// `def` behind and expands to `DEF`.
			if e.Index != nil && !wholeArraySubscript(r.subscriptText(e.Index)) {
				r.assignSubscript(e, v)
			} else if e.Name != "" {
				r.setVar(e.Name, v)
			}
			return []string{v}, false, true
		}
	case syntax.ParamAlternate:
		if fires {
			return []string{""}, false, true
		}
		return []string{r.joinWord(e.Arg)}, false, true
	case syntax.ParamError:
		if fires {
			r.fatalExpansion("%s\n", Wording(r.diag().ParamErrorMessage, "%[1]s: %[2]s",
				e.Name, r.paramErrorWord(e, set)))
			return nil, false, false
		}
	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong,
		syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong:
		pattern := r.patternOf(e.Arg)
		// Rule: `M` substitutes what the pattern *took* rather than what it
		// left. The same operator and the same match, read from the other
		// side — measured, `${(M)v#h*l}` on `hello` is `hel` and
		// `${(M)v##h*l}` is `hell`, so the shortest/longest choice is still
		// the operator's.
		take := r.trimWith
		if matchingFlag(e) {
			take = r.matchedWith
		}
		for i, w := range words {
			words[i] = take(w, pattern, e.Op)
		}
	case syntax.ParamReplace:
		pattern, with := r.patternOf(e.Arg), r.joinWord(e.Arg2)
		for i, w := range words {
			words[i] = r.replaceWith(w, pattern, with, e)
		}
	case syntax.ParamSubstring:
		if isList {
			return sliceElems(words, r.numOf(e.Arg, e, e.Arg2), e, r), true, true
		}
		words[0] = r.substringRange(words[0], e)
	case syntax.ParamExclude, syntax.ParamSetDifference, syntax.ParamSetIntersection:
		if isList {
			return r.selectElements(e, words), true, true
		}
		// Not a list: `${(U)v:#p}` asks the same question of one value, and
		// the answer is that value or nothing. It stays a scalar rather than
		// becoming an empty list, so `"${v:#p}"` is one empty field the way
		// `"${v#p}"` is.
		words[0] = r.selectScalar(e, words[0])
	case syntax.ParamUpper, syntax.ParamLower, syntax.ParamToggle,
		syntax.ParamUpperFirst, syntax.ParamLowerFirst, syntax.ParamToggleFirst:
		for i, w := range words {
			words[i] = r.changeCase(w, e)
		}
	}
	return words, isList, true
}

// convertCase is the `U` and `L` flags: every letter, under the same locale
// policy the case-changing operators follow — an explicit C locale narrows
// to ASCII and anything else is Unicode-aware.
func (r *Runner) convertCase(v string, upper bool) string {
	convert := unicode.ToUpper
	if !upper {
		convert = unicode.ToLower
	}
	if r.localeIsC() {
		wide := convert
		convert = func(c rune) rune {
			if c < 0x80 {
				return wide(c)
			}
			return c
		}
	}
	return strings.Map(convert, v)
}

// promptEscapes is the `%` flag over one word. Only the escapes that name
// the file being read are carried — `%x` and `%N`, the ones scripts use to
// find their own path, plus the literal `%%` — and anything else is refused
// by name rather than answered wrong: the construct's home shell implements
// its entire prompt language here, and this slice does not pretend to.
func (r *Runner) promptEscapes(v string, e *syntax.ParamExpr) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] != '%' || i+1 >= len(v) {
			b.WriteByte(v[i])
			continue
		}
		i++
		switch v[i] {
		case '%':
			b.WriteByte('%')
		case 'x':
			// The file being read: the sourced file, the script, or — under
			// `-c`, where there is no file — what the shell calls itself.
			if f := r.currentFile(); f != "" {
				b.WriteString(f)
			} else {
				b.WriteString(r.name())
			}
		case 'N':
			b.WriteString(r.promptUnitName())
		case 'n':
			// The user the shell runs as. Answered only where somebody told
			// this runner who that is (SetPromptUser); a runner nobody told
			// refuses it with the rest rather than expanding to nothing,
			// which would be a wrong answer wearing a success.
			//
			// Spelled as a call rather than a `break` into the default: a
			// `break` inside a Go switch leaves the switch, so it would have
			// produced exactly the silence this is here to avoid.
			if r.promptUser == "" {
				return r.refusePromptEscape(e, v[i])
			}
			b.WriteString(r.promptUser)
		default:
			return r.refusePromptEscape(e, v[i])
		}
	}
	return b.String(), true
}

// refusePromptEscape says, by name, that an escape is not carried here.
//
// One place rather than two, because the wording is the promise: it names the
// escape the script asked for, so a reader can tell which of several in one
// word was the one this shell could not answer.
func (r *Runner) refusePromptEscape(e *syntax.ParamExpr, c byte) (string, bool) {
	r.diagf("${%s}: the %%%c prompt escape is not implemented\n", e.Src, c)
	r.expandErr = true
	return "", false
}

// promptUnitName is `%N`: the name of the function, sourced file or script
// being read — the function's *name* where `%x` stays its defining file.
func (r *Runner) promptUnitName() string {
	if len(r.frames) > 0 {
		f := r.frames[len(r.frames)-1]
		if f.Name != "" && f.Name != sourceFrameName {
			return f.Name
		}
		if f.File != "" {
			return f.File
		}
	}
	if r.scriptFile != "" {
		return r.scriptFile
	}
	return r.name()
}

// quoteFlagged is the `q` family, one style per count — all measured:
// backslashes, then single quotes, double quotes, and `$'…'`.
func quoteFlagged(v string, count int) string {
	switch count {
	case 1:
		return quoteWithBackslashes(v)
	case 2:
		return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
	case 3:
		var b strings.Builder
		b.WriteByte('"')
		for i := 0; i < len(v); i++ {
			if strings.IndexByte("\\`\"$", v[i]) >= 0 {
				b.WriteByte('\\')
			}
			b.WriteByte(v[i])
		}
		b.WriteByte('"')
		return b.String()
	default:
		var b strings.Builder
		b.WriteString("$'")
		eachQuotableByte(v, func(c byte) {
			switch {
			case c == '\'':
				b.WriteString(`\'`)
			case c == '\\':
				b.WriteString(`\\`)
			case c == '!':
				b.WriteString(`\!`)
			case c < 0x20 || c >= 0x7f:
				b.WriteString(controlEscape(c))
			default:
				b.WriteByte(c)
			}
		}, func(raw string) { b.WriteString(raw) })
		b.WriteString("'")
		return b.String()
	}
}

// quoteWithBackslashes is the single-`q` style: the characters the shell
// gives meaning to are escaped, each control or non-UTF-8 byte becomes its
// own `$'…'` segment, and an empty value is `”` — every detail measured.
func quoteWithBackslashes(v string) string {
	if v == "" {
		return "''"
	}
	const specials = " `$\"'\\*?[](){}<>|;&~#^="
	var b strings.Builder
	eachQuotableByte(v, func(c byte) {
		switch {
		case c < 0x20 || c >= 0x7f:
			// Control bytes and bytes that are not UTF-8 alike — measured,
			// `$'\177'` and `$'\377'`.
			b.WriteString("$'" + controlEscape(c) + "'")
		case strings.IndexByte(specials, c) >= 0:
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}, func(raw string) { b.WriteString(raw) })
	return b.String()
}

// eachQuotableByte walks a string handing single bytes — ASCII, and any byte
// that is not part of a valid multibyte rune — to one function and whole
// multibyte runes to the other, because quoting escapes bytes while UTF-8
// passes through untouched.
func eachQuotableByte(v string, one func(byte), run func(string)) {
	for i := 0; i < len(v); {
		if v[i] < utf8.RuneSelf {
			one(v[i])
			i++
			continue
		}
		c, size := utf8.DecodeRuneInString(v[i:])
		if c == utf8.RuneError && size == 1 {
			one(v[i])
			i++
			continue
		}
		run(v[i : i+size])
		i += size
	}
}

// controlEscape writes one control byte the way `$'…'` spells it: the seven
// named escapes by name and everything else as three-digit octal — `$'\033'`
// for escape rather than `\e`, which is measured.
func controlEscape(c byte) string {
	switch c {
	case '\a':
		return `\a`
	case '\b':
		return `\b`
	case '\f':
		return `\f`
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	case '\v':
		return `\v`
	}
	return fmt.Sprintf(`\%03o`, c)
}

// matchingFlag reports whether the `M` flag was written, which turns the
// operators that *remove* what a pattern matched into ones that keep it.
//
// It reaches exactly two of them, measured across every operator the flag
// group may stand in front of: the four trims, where it substitutes the
// matched part, and `:#`, where it keeps the matching elements instead of
// dropping them. On `/`, `:|`, `:*`, a substring, the conditionals and an
// expansion with no operator at all it does nothing — which is why there is
// no third call site rather than an oversight.
func matchingFlag(e *syntax.ParamExpr) bool {
	return e != nil && strings.ContainsRune(e.Flags, 'M')
}
