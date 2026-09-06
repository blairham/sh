// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// ParamOp is the operator inside `${ … }`.
type ParamOp uint8

const (
	// ParamNone is a plain reference, `${x}`.
	ParamNone ParamOp = iota
	// ParamDefault is `-`: substitute the word when the test fires.
	ParamDefault
	// ParamAssign is `=`: assign the word, then substitute it. The only
	// expansion with a side effect.
	ParamAssign
	// ParamError is `?`: write the word as an error and exit.
	ParamError
	// ParamAlternate is `+`: substitute the word when the test does *not*
	// fire.
	ParamAlternate
	// ParamTrimPrefix is `#`, ParamTrimPrefixLong `##`. Doubling the
	// operator is what selects the longer match; there is no greediness
	// syntax inside the pattern.
	ParamTrimPrefix
	ParamTrimPrefixLong
	// ParamTrimSuffix is `%`, ParamTrimSuffixLong `%%`.
	ParamTrimSuffix
	ParamTrimSuffixLong
	// ParamReplace is `/`, with All and Anchor saying which spelling.
	ParamReplace
	// ParamSubstring is `${x:off:len}`.
	ParamSubstring
	// ParamUpperFirst is `^`, ParamLowerFirst `,` and ParamToggleFirst `~`:
	// the first character only, and only when it matches the operator's
	// pattern. `${x^b}` on `abc` is `abc`, because the first character is
	// not a `b`.
	ParamUpperFirst
	ParamLowerFirst
	ParamToggleFirst
	// ParamToggle is `~~`, which swaps the case of every character the
	// pattern matches, as ParamUpper and ParamLower do in one direction.
	ParamToggle
	// ParamUpper is `^^` and ParamLower `,,`. bash alone, so they are
	// rejected unless the dialect has them.
	ParamUpper
	ParamLower
	// ParamExclude is `:#`, ParamKeepOnly its inverse under a flag, and
	// ParamSetDifference `:|` and ParamSetIntersection `:*`. All four choose
	// which *elements* survive, where ParamSubstring chooses which
	// characters do — and the `:` is shared, which is the whole difficulty:
	// `${a:#p}` is an exclusion in one grammar and an offset of `#p` in
	// another, so the character after the colon decides which construct was
	// written before any of it can be evaluated.
	//
	// ParamExclude takes a pattern, matched against a whole element.
	// ParamSetDifference and ParamSetIntersection take the *name* of another
	// array, and compare elements for equality rather than by pattern.
	ParamExclude
	ParamSetDifference
	ParamSetIntersection
	// ParamTransform is `@` followed by one letter naming a transformation
	// of the value — `${x@Q}` quotes it for reuse as input. The letter rides
	// on [ParamExpr.Transform]; the set is fixed, and a letter outside it is
	// a bad substitution rather than a shape of its own.
	ParamTransform
)

func (o ParamOp) String() string {
	switch o {
	case ParamDefault:
		return "-"
	case ParamAssign:
		return "="
	case ParamError:
		return "?"
	case ParamAlternate:
		return "+"
	case ParamTrimPrefix:
		return "#"
	case ParamTrimPrefixLong:
		return "##"
	case ParamTrimSuffix:
		return "%"
	case ParamTrimSuffixLong:
		return "%%"
	case ParamReplace:
		return "/"
	case ParamSubstring:
		return ":"
	case ParamExclude:
		return ":#"
	case ParamSetDifference:
		return ":|"
	case ParamSetIntersection:
		return ":*"
	case ParamUpperFirst:
		return "^"
	case ParamLowerFirst:
		return ","
	case ParamToggleFirst:
		return "~"
	case ParamToggle:
		return "~~"
	case ParamUpper:
		return "^^"
	case ParamLower:
		return ",,"
	case ParamTransform:
		return "@"
	}
	return ""
}

// paramTransformLetters is the fixed set a `@` operator may carry. Measured on
// bash 5.3, which has no others: `${x@Z}` is a bad substitution there exactly
// as `${x@Q}` is in a dialect without the family.
const paramTransformLetters = "QEPAaKkLUu"

// ParamExpr is a parsed `${ … }`.
type ParamExpr struct {
	// Name is the parameter: a name, a digit, or a special such as @ * # ?.
	Name string
	// Index is the `[…]` subscript, nil when absent. Its base is a dialect
	// question — bash and ksh93 count from 0, zsh from 1 — so nothing here
	// interprets it.
	Index *Word
	// Length is `${#x}`.
	Length bool
	// Indirect is `${!x}`. bash alone means indirection by it; ksh93 means
	// something else and does not error, so a dialect that lacks it must
	// refuse rather than guess.
	Indirect bool
	// Prefix is `${!name@}` or `${!name*}`, which yields the *names* of the
	// variables beginning with name rather than any value. It carries the
	// `@` or the `*`, which differ exactly as `$@` and `$*` do: one field
	// each, or one field joined.
	Prefix byte

	// Inner is the expansion standing where a name would — the `${v}` of
	// `${${v}#a}` — and is nil for the ordinary shape. Name is empty when it
	// is set, because the inner expansion is the whole of that position:
	// `${x${v}}` is a bad substitution rather than a name with one appended.
	//
	// A *word* rather than a ParamExpr, so the shape says only "an expansion
	// stands here" — the inner may be a parameter expansion, a command
	// substitution or an arithmetic one, and all three were measured
	// accepted in the same position.
	Inner *Word

	Op ParamOp
	// Colon records the `:` that extends the test from "unset" to "unset or
	// empty". It is the whole difference between the two rows of
	// conditionals.
	Colon bool

	// Arg is the operand: the word for `-=?+`, the pattern for `#%/`, the
	// offset for a substring. It is a *word* and not a string, because it is
	// itself expanded — `${u:-$(echo sub)}` yields sub.
	Arg *Word
	// Arg2 is the replacement for `/`, or the length for a substring.
	Arg2 *Word
	// All is `//`, replacing every match rather than the first.
	All bool
	// Anchor is '#' for `/#` or '%' for `/%`, and 0 otherwise.
	Anchor byte
	// Transform is ParamTransform's letter — one of Q E P A a K k L U u.
	// It is a byte rather than a word because it is part of the operator:
	// nothing expands it, and `${x@$op}` is a bad substitution.
	Transform byte

	// HasFlags marks a parenthesized flag group at the front of the
	// expansion — `${(U)x}` — which one dialect's grammar has and the rest
	// call a bad substitution. It is a marker of its own because the group
	// may legally be empty: `${()x}` is `${x}`.
	HasFlags bool
	// Flags is the group's letters, in written order, with any separator
	// arguments stripped: `${(Uq)x}` carries "Uq" and `${(s.:.)x}` "s".
	Flags string
	// SplitSep and JoinSep are the arguments of the `s` and `j` flags. An
	// empty SplitSep with an `s` in Flags is meaningful — it splits into
	// characters — which HasFlags plus the letter already distinguish from
	// no `s` at all.
	SplitSep string
	JoinSep  string
	// FlagsErrPos is the 1-based position, counted from the `$`, of the
	// first character the flag group could not read, and 0 when it read
	// cleanly. The error is the interpreter's to report — reached in a
	// branch never taken, it is no error at all, which is measured — so the
	// node carries the position and Src carries the text.
	FlagsErrPos int

	// Bad marks an expansion whose operator the grammar did not recognize,
	// in a dialect that diagnoses that when the expansion is reached rather
	// than when it is read. Src holds the inside of the braces for the
	// report; every other field on a Bad node is meaningless — except Name
	// and Index, which are read before the operator is and stay filled in.
	Bad bool
	// BadTransform narrows Bad to the `@` family in a grammar that *has* the
	// family: the operator was read as `@` and what followed it is not one
	// of the letters. Every other unreadable operator leaves this false,
	// including `@` in a grammar without the family, where the whole
	// construct is unknown rather than the letter.
	//
	// The distinction is not cosmetic. Where the family exists, a bad letter
	// is a failure of the *expansion* — checked against the value, at the
	// point the value is read — and it carries the status a failed expansion
	// carries, while every other unreadable operator is a failure to read
	// the word. Measured: the two exit differently from the same invocation.
	BadTransform bool
	// Src is the raw text between the braces, kept for the diagnostic.
	Src string

	Start Pos
	Stop  Pos
}

func (p *ParamExpr) Pos() Pos { return p.Start }
func (p *ParamExpr) End() Pos { return p.Stop }

// specialParams are the one-character parameters that are not names.
const specialParams = "@*#?-$!0123456789"

// parseParamExp parses the text between `${` and `}`.
//
// The lexer already found the matching brace, tracking quoting so a `}` inside
// quotes did not end it early, so src here is exactly the inside.
func (p *Parser) parseParamExp(src string, start Pos) *ParamExpr {
	e := &ParamExpr{Start: start, Stop: start}
	s := src

	if strings.HasPrefix(s, "(") {
		if !p.dialect.ParamExpansionFlags {
			// The whole expansion is a bad substitution to a grammar without
			// the group, and *when* that is said follows the same split every
			// other unreadable expansion follows: one dialect refuses while
			// reading, the rest defer to the run — measured, a flag group in
			// a branch never taken is never diagnosed there.
			if p.dialect.BadSubstitutionAtParseTime {
				p.failKind(ErrBadSubstitution, "unknown operator in ${%s}", src)
				if pe, isErr := p.err.(*Error); isErr {
					pe.Token = "("
				}
				return e
			}
			e.Bad, e.Src = true, src
			return e
		}
		s = p.scanParamFlags(e, src)
		if e.FlagsErrPos > 0 {
			return e
		}
	}

	switch {
	case strings.HasPrefix(s, "#") && len(s) > 1:
		e.Length = true
		s = s[1:]
	case strings.HasPrefix(s, "!") && len(s) > 1 && !strings.ContainsAny(s[1:2], ":-+=?"):
		// Only a `!` that could begin a name is indirection: with an
		// operator right after it, the `!` is the parameter — `${!:+set}`
		// is `$!` with `:+` applied, in all four shells.
		if !p.dialect.ParamIndirection {
			// Refused rather than guessed: ksh93 accepts this and means
			// something else, so a dialect without it cannot pretend.
			p.failKind(ErrBadSubstitution, "${!name} is not available in this dialect")
			return e
		}
		e.Indirect = true
		s = s[1:]
	}

	if p.dialect.NestedParamExpansion && strings.HasPrefix(s, "$") {
		if inner, rest, ok := p.scanNestedExpansion(s, start); ok {
			// Src as well as the node: a diagnostic about a nested expansion
			// names the text it was written as, and there is no parameter
			// name here for it to name instead.
			e.Inner, s, e.Src = inner, rest, src
		}
	}

	if e.Inner == nil {
		e.Name, s = scanParamName(s)
	}
	if e.Name == "" && !e.HasFlags && e.Inner == nil {
		if !p.dialect.BadSubstitutionAtParseTime {
			// The majority defers an unreadable expansion to the run, the
			// same way an unknown operator is deferred: a `${%x}` in a
			// branch never taken is not an error at all.
			e.Bad, e.Src = true, src
			return e
		}
		p.failKind(ErrBadSubstitution, "expected a parameter name in ${%s}", src)
		if pe, isErr := p.err.(*Error); isErr && s != "" {
			// The character standing where the name belonged, for the one
			// dialect that words this as a syntax error naming the token —
			// left empty, it printed `' unexpected.
			pe.Token = firstRune(s)
		}
		return e
	}
	// With a flag group the name may be empty — `${(U)}` is an empty string
	// and `${(%):-%x}` is all operator — so an operator may still follow.

	// `${!name@}` and `${!name*}` are the names beginning with name, not a
	// value at all. Only after `!`, and only when the whole rest is the one
	// character — `${!name@U}` is an operator on an indirection and not this.
	if e.Indirect && (s == "@" || s == "*") {
		e.Prefix = s[0]
		return e
	}

	// A subscript after a nested expansion is read here and refused by name
	// at the run, rather than left unconsumed: leaving it would make the
	// operator scan fail and call the whole thing a bad substitution, which
	// says the grammar cannot read what the grammar plainly can.
	if p.dialect.ArraySubscript && strings.HasPrefix(s, "[") &&
		(e.Inner != nil || p.subscriptableName(e.Name)) {
		if i := closingBracket(s); i > 0 {
			e.Index = p.wordFrom(s[1:i], start)
			s = s[i+1:]
		}
	}
	if s == "" {
		return e
	}

	op, rest, ok := p.scanParamOp(s, e)
	if !ok {
		if !p.dialect.BadSubstitutionAtParseTime || s[0] == '@' {
			// The majority defers: the node is kept, marked, and diagnosed
			// only if the expansion is ever reached — an unrecognized
			// operator in a branch never taken is not an error at all.
			//
			// The `@` family is deferred even by the one grammar that
			// otherwise refuses while reading. Measured: ksh93 reads
			// `${x@Q}` in a branch never taken without complaint and calls
			// it `${x@Q}: bad substitution` only when the expansion is
			// reached, while `${x^^}` in the same branch is a parse-time
			// syntax error there.
			//
			// A grammar that *has* the family marks the narrower failure on
			// the node as well: there the letter is the only thing wrong,
			// and the run decides what to do about it against the value.
			e.Bad, e.Src = true, src
			e.BadTransform = p.dialect.ParamTransformations && s[0] == '@'
			return e
		}
		// The operator itself travels with the failure: one dialect does not
		// call this a bad substitution at all, but a syntax error naming the
		// character it could not read.
		p.failKind(ErrBadSubstitution, "unknown operator in ${%s}", src)
		if pe, isErr := p.err.(*Error); isErr {
			pe.Token = firstRune(s)
		}
		return e
	}
	e.Op = op
	p.fillParamArgs(e, rest, start)
	return e
}

// subscriptableName reports whether the parameter just named may carry a
// subscript in this grammar.
//
// A name always may, where the grammar has subscripts at all. A parameter that
// is *not* a name — `@`, `*`, `0`, a positional digit, `?`, `-`, `$` — needs
// the flag: one shell reads `${@[1]}` as a positional parameter and the rest
// of the panel refuse the expansion outright. Leaving the bracket unconsumed
// is what produces that refusal, in each grammar's own words and at each
// grammar's own moment.
func (p *Parser) subscriptableName(name string) bool {
	if isParamName(name) {
		return true
	}
	return p.dialect.SpecialParamSubscript
}

// isParamName reports whether a parameter was spelled as a name rather than as
// one of the specials or a positional number. `${10[1]}` is a positional
// parameter in the shells that read two digits there, so a run of digits is
// not a name however long it is.
func isParamName(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// paramFlagArgs says how many delimited arguments a flag letter may read:
// the separators of `s` and `j`, and the argument groups of the padding and
// grouping flags — scanned so the group's closing parenthesis is still
// found, even though the interpreter refuses the flags themselves.
var paramFlagArgs = map[byte]int{
	's': 1, 'j': 1, 'g': 1, 'I': 1, 'Z': 1, '_': 1, 'l': 3, 'r': 3,
}

// paramFlagChars is every character the flag group may carry, taken from the
// vendor manual's inventory. Reading the full alphabet and refusing the
// unimplemented members at run time keeps the two failure shapes apart: a
// character outside this set is an "error in flags" with a position, which
// is measured, and a letter inside it that this interpreter does not carry
// is refused by name.
const paramFlagChars = "#%@AabcCDefFgiIjklLmMnNoOpPqQrRsStuUvVwWXxzZ0~^=*BE-+_"

// scanParamFlags reads the parenthesized group src opens with, filling the
// flag fields, and returns the text after the closing parenthesis.
//
// On a character it cannot read it records the 1-based position counted
// from the `$` — position 4 is the first character inside the parentheses —
// because the report belongs to the run and not to the read: a bad flag in
// a branch never taken is diagnosed nowhere, which is measured. Running out
// of text before the closing parenthesis is the same failure at the
// position just past the end, which is also measured: `${(Ux}` errors at 5.
func (p *Parser) scanParamFlags(e *ParamExpr, src string) string {
	e.HasFlags = true
	e.Src = src
	i := 1
	for i < len(src) {
		c := src[i]
		if c == ')' {
			return src[i+1:]
		}
		if strings.IndexByte(paramFlagChars, c) < 0 {
			e.FlagsErrPos = i + 3
			return ""
		}
		e.Flags += string(c)
		i++
		argMax := paramFlagArgs[c]
		var open byte
		for n := 0; n < argMax && i < len(src); n++ {
			if n == 0 {
				if src[i] == ')' {
					// An argument-taking flag with no argument: the group
					// cannot mean anything, and the failure points at the
					// parenthesis that arrived instead.
					e.FlagsErrPos = i + 3
					return ""
				}
				open = src[i]
			} else if src[i] != open {
				// The optional later arguments arrive only behind the same
				// delimiter again.
				break
			}
			closing := matchingFlagDelimiter(open)
			j := strings.IndexByte(src[i+1:], closing)
			if j < 0 {
				e.FlagsErrPos = i + 3
				return ""
			}
			arg := src[i+1 : i+1+j]
			switch c {
			case 's':
				e.SplitSep = arg
			case 'j':
				e.JoinSep = arg
			}
			i += j + 2
		}
	}
	e.FlagsErrPos = len(src) + 2
	return ""
}

// matchingFlagDelimiter is the character that closes a flag argument: the
// partner for the four matched pairs, and the same character again for
// everything else.
func matchingFlagDelimiter(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	case '<':
		return '>'
	}
	return open
}

// scanParamName reads the parameter, which is either a name or one of the
// single characters that are parameters in their own right.
func scanParamName(s string) (name, rest string) {
	if s == "" {
		return "", ""
	}
	if strings.IndexByte(specialParams, s[0]) >= 0 && !isNameStart(s[0]) {
		// A digit may begin a multi-digit positional parameter.
		if s[0] >= '0' && s[0] <= '9' {
			i := 0
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			return s[:i], s[i:]
		}
		return s[:1], s[1:]
	}
	i := 0
	for i < len(s) && isNameByte(s[i], i) {
		i++
	}
	return s[:i], s[i:]
}

// closingBracket finds the `]` that closes the subscript s opens with, or -1.
//
// Matched rather than *last*, which is what this used to take. A colon-less
// operator whose value holds another subscripted expansion —
// `${tags[@]+${tags[@]}}`, the standard way to expand a possibly-empty array
// under `set -u` — put a second `]` in the string, and taking the last one
// swallowed everything between. The simple `${a[@]+x}` worked, which is why
// it went unnoticed until a real script used the nested form.
//
// Depth counting, because a subscript may itself hold one: `${a[b[0]]}`.
func closingBracket(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameByte(c byte, i int) bool {
	return isNameStart(c) || (i > 0 && c >= '0' && c <= '9')
}

// scanParamOp reads the operator and reports the text after it.
func (p *Parser) scanParamOp(s string, e *ParamExpr) (ParamOp, string, bool) {
	// The colon-prefixed conditionals first, so `:-` is not read as a
	// substring whose offset begins with a minus.
	if len(s) >= 2 && s[0] == ':' && strings.IndexByte("-=?+", s[1]) >= 0 {
		e.Colon = true
		return condOp(s[1]), s[2:], true
	}
	switch {
	// A `:` followed by one of these three is an operator rather than the
	// start of an offset, and that one character is the whole
	// disambiguation. Before the substring case, because everything here
	// would otherwise be read as arithmetic — which is exactly what a
	// grammar without the flag does, and what the error `operand expected
	// at \`#fig_precmd\'` was.
	case p.dialect.ParamElementSelection && len(s) >= 2 && s[0] == ':' &&
		strings.IndexByte("#|*", s[1]) >= 0:
		return elementSelectOp(s[1]), s[2:], true
	case s[0] == ':':
		if !p.dialect.ParamSubstring {
			return 0, "", false
		}
		return ParamSubstring, s[1:], true
	case strings.IndexByte("-=?+", s[0]) >= 0:
		return condOp(s[0]), s[1:], true
	case strings.HasPrefix(s, "##"):
		return ParamTrimPrefixLong, s[2:], true
	case s[0] == '#':
		return ParamTrimPrefix, s[1:], true
	case strings.HasPrefix(s, "%%"):
		return ParamTrimSuffixLong, s[2:], true
	case s[0] == '%':
		return ParamTrimSuffix, s[1:], true
	case s[0] == '/':
		if !p.dialect.ParamSubstitution {
			return 0, "", false
		}
		rest := s[1:]
		switch {
		case strings.HasPrefix(rest, "/"):
			e.All = true
			rest = rest[1:]
		case strings.HasPrefix(rest, "#"), strings.HasPrefix(rest, "%"):
			e.Anchor = rest[0]
			rest = rest[1:]
		}
		return ParamReplace, rest, true
	case s[0] == '@':
		if !p.dialect.ParamTransformations {
			return 0, "", false
		}
		// One letter from the fixed set and nothing after it: `${x@}`,
		// `${x@QQ}` and `${x@ Q}` are bad substitutions in the shell that
		// has the family, measured, so the grammar refuses exactly what the
		// runtime there refuses.
		if len(s) != 2 || strings.IndexByte(paramTransformLetters, s[1]) < 0 {
			return 0, "", false
		}
		e.Transform = s[1]
		return ParamTransform, "", true
	case strings.HasPrefix(s, "^"), strings.HasPrefix(s, ","), strings.HasPrefix(s, "~"):
		if !p.dialect.ParamCaseChange {
			return 0, "", false
		}
		// Doubled is every character, single is the first — and what follows
		// either is a *pattern* saying which characters count, so the rest is
		// handed on rather than required to be empty.
		if len(s) > 1 && s[1] == s[0] {
			switch s[0] {
			case '^':
				return ParamUpper, s[2:], true
			case ',':
				return ParamLower, s[2:], true
			}
			return ParamToggle, s[2:], true
		}
		switch s[0] {
		case '^':
			return ParamUpperFirst, s[1:], true
		case ',':
			return ParamLowerFirst, s[1:], true
		}
		return ParamToggleFirst, s[1:], true
	}
	return 0, "", false
}

func condOp(c byte) ParamOp {
	switch c {
	case '-':
		return ParamDefault
	case '=':
		return ParamAssign
	case '?':
		return ParamError
	}
	return ParamAlternate
}

// elementSelectOp names the operator the character after the colon asked for.
func elementSelectOp(c byte) ParamOp {
	switch c {
	case '|':
		return ParamSetDifference
	case '*':
		return ParamSetIntersection
	}
	return ParamExclude
}

// fillParamArgs splits the operand text according to the operator.
func (p *Parser) fillParamArgs(e *ParamExpr, rest string, start Pos) {
	switch e.Op {
	case ParamReplace:
		// The separator is an unquoted slash, so a slash inside quotes or
		// after a backslash belongs to the pattern.
		if i := indexUnquoted(rest, '/'); i >= 0 {
			e.Arg = p.wordFrom(rest[:i], start)
			e.Arg2 = p.wordFrom(rest[i+1:], start)
		} else {
			// Omitting the replacement deletes the match.
			e.Arg = p.wordFrom(rest, start)
		}
	case ParamSubstring:
		if i := indexUnquoted(rest, ':'); i >= 0 {
			e.Arg = p.wordFrom(rest[:i], start)
			e.Arg2 = p.wordFrom(rest[i+1:], start)
		} else {
			e.Arg = p.wordFrom(rest, start)
		}
	case ParamUpper, ParamLower, ParamToggle,
		ParamUpperFirst, ParamLowerFirst, ParamToggleFirst:
		// What follows is the pattern saying which characters to convert.
		// Empty means every one of them, which is what `?` would say.
		if rest != "" {
			e.Arg = p.wordFrom(rest, start)
		}
	default:
		if rest != "" {
			e.Arg = p.wordFrom(rest, start)
		}
	}
}

// indexUnquoted finds c outside quotes and not backslash-escaped.
func indexUnquoted(s string, c byte) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		switch ch := s[i]; {
		case ch == '\\' && quote != '\'':
			i++
		case quote != 0:
			if ch == quote {
				quote = 0
			}
		case ch == '\'' || ch == '"':
			quote = ch
		case ch == c:
			return i
		}
	}
	return -1
}

// wordFrom lexes text as a word, so an operand keeps its structure: the word
// in `${x:-word}` is itself expanded, and `${u:-$(echo sub)}` yields sub.
func (p *Parser) wordFrom(text string, at Pos) *Word {
	if text == "" {
		return &Word{Start: at, Stop: at}
	}
	if p.depth >= maxParamDepth {
		p.fail("expansions nested too deeply")
		return &Word{Start: at, Stop: at}
	}
	p.depth++
	defer func() { p.depth-- }()

	sub := NewLexer(text, p.operandDialect())
	w := &Word{Start: at, Stop: at}
	// Whatever the lexer stepped over between two tokens is text here, not a
	// separator: `${u:-a b}` is the two words `a b` and not `ab`. A lexer
	// reading a command is right to drop the blank between two words, and an
	// operand is not a command — everything up to the closing brace belongs
	// to it. The gap is put back by offset rather than guessed at, so a run
	// of spaces, a tab, or a `#` the lexer read as a comment all survive as
	// what they were.
	last := 0
	gap := func(upto int) {
		if upto > last && last >= 0 && upto <= len(text) {
			w.Spans = append(w.Spans, Span{Kind: Literal, Value: text[last:upto], Pos: at})
		}
	}
	for {
		t := sub.Next()
		if t.Kind == TokEOF {
			gap(len(text))
			break
		}
		gap(t.Pos.Offset)
		last = t.End.Offset
		if t.Kind == TokWord {
			// Through newWord, so a nested ${ } in an operand is parsed too.
			nested := p.newWord(t.Spans, at, at)
			w.Spans = append(w.Spans, nested.Spans...)
			continue
		}
		// An operator inside an operand is ordinary text there — `${x:-a>b}`
		// has no redirection in it — so it is kept as a literal span rather
		// than being dropped.
		w.Spans = append(w.Spans, Span{Kind: Literal, Value: t.Text, Pos: at})
	}
	return w
}

// firstRune is the one character a diagnostic names when the operator it
// could not read is longer than one — `${x^^}` is `^` unexpected.
func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

// operandDialect is the grammar an expansion's operand is read under.
//
// It is the dialect itself but for one question: whether `<(cmd)` opens a
// process substitution here. bash says yes and ksh93, zsh and dash say no —
// measured, and the three that say no have process substitution everywhere
// else — so the answer belongs to the position and is applied by handing the
// operand's lexer a grammar that has been told.
func (p *Parser) operandDialect() Dialect {
	if p.dialect.ProcessSubstitutionInParamOperand {
		return p.dialect
	}
	d := p.dialect
	d.ProcessSubstitution = false
	return d
}

// scanNestedExpansion peels one substitution off the front of s, for the
// grammar where an expansion may stand where a parameter name would.
//
// The extent is measured by lexing rather than by counting braces here: the
// lexer already knows that a `}` inside quotes does not close one and that
// `$(` ends at its own parenthesis, and a second counter beside it would be a
// second answer to the same question. What comes back has to be exactly one
// substitution span at the front — anything else is not this shape, and the
// caller then reads the text the ordinary way and fails the ordinary way.
func (p *Parser) scanNestedExpansion(s string, at Pos) (inner *Word, rest string, ok bool) {
	if p.depth >= maxParamDepth {
		p.fail("expansions nested too deeply")
		return nil, "", false
	}
	p.depth++
	defer func() { p.depth-- }()

	// A *braced* parameter expansion, or a substitution written with
	// parentheses. Measured: `${$v}` and `${$v#a}` are a bad substitution in
	// the shell with the grammar, where `${${v}}` and `${${v}#a}` are not, so
	// the braces are part of the shape rather than decoration — and `${$}` is
	// the parameter named `$`, which is not this at all. `${$'x'}` opens with
	// a dollar and is not a substitution either: the quoting carries it, and
	// the span is literal text.
	if !strings.HasPrefix(s, "${") && !strings.HasPrefix(s, "$(") {
		return nil, "", false
	}
	// Which is also the whole of the test. A kind check on the span behind it
	// could not be reached: those two prefixes lex to a substitution and
	// nothing else does, so a guard on the kind was a branch no mutation
	// could tell from its absence.
	t := NewLexer(s, p.dialect).Next()
	if t.Kind != TokWord || len(t.Spans) == 0 {
		return nil, "", false
	}
	// Where the substitution ended: the next span's start, or the token's own
	// end when it was the whole of it. A span records where it began and not
	// where it stopped, and the neighbor's position is the same fact read
	// from the other side.
	end := t.End.Offset
	if len(t.Spans) > 1 {
		end = t.Spans[1].Pos.Offset
	}
	if end > len(s) {
		return nil, "", false
	}
	return p.newWord(t.Spans[:1], at, at), s[end:], true
}
