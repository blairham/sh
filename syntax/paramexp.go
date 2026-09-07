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

	// IndexFlags is the parenthesized flag group the subscript opened with,
	// nil when there was none. Index keeps the subscript as written, and the
	// operand behind the group is IndexFlags.Arg — see Subscript, which is
	// what a reading should use.
	IndexFlags *SubscriptFlags

	// TildeFlags is how many `~` characters were written between the `${`
	// and the rest of the expansion — `${~x}` carries 1 and `${~~x}` carries
	// 2. Zero is the ordinary expansion.
	//
	// A count and not a bool because parity is the meaning: an odd number
	// makes the result eligible for tilde expansion and filename generation
	// and an even one refuses it, both regardless of the option that would
	// otherwise decide, which is measured. Keeping the count also lets the
	// printer write the span back exactly as written.
	TildeFlags int

	// SplitFlags is how many `=` characters were written in the same slot —
	// `${=x}` carries 1 and `${==x}` carries 2. Zero is the ordinary
	// expansion.
	//
	// A count for the same reason TildeFlags is one: parity is the meaning.
	// An odd number splits the substituted value into words on `IFS` and an
	// even one refuses to, both regardless of the option that would
	// otherwise decide, which is measured.
	//
	// Separate from TildeFlags rather than one run of "flag characters",
	// because the two decide different questions and a script writes both:
	// `${=~g}` and `${~=g}` are the same expansion, and each half has to be
	// counted on its own for parity to mean anything.
	SplitFlags int

	// SetTest marks a `+` written between the `${` and the parameter —
	// `${+name}` — which asks whether the parameter is set and substitutes
	// `1` or `0` rather than its value.
	//
	// A bool and not a count, where TildeFlags is a count: `${++x}` is a bad
	// substitution, measured, so there is no doubled spelling for a parity
	// to be about.
	//
	// It is only the answer when the expansion carries no operator.
	// Measured: `${+v#a}` on `abc` is `bc`, `${+v:-x}` on an unset `v` is
	// `x`, and `${+v=w}` assigns — with an operator written the `+` has no
	// effect at all rather than being refused.
	SetTest bool

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
func (p *Parser) parseParamExp(src string, start Pos, q Quoting) *ParamExpr {
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

	// A run of `~` and `=` stands between the flag group and everything
	// else, which is where the shell that has them puts them: `${(U)~g}`
	// reads, `${~(U)g}` is a bad substitution, and `${(U)~#g}` is a length —
	// so after the group and in front of the `#` below. The two characters
	// share the slot and are interchangeable within it, measured: `${=~g}`
	// and `${~=g}` are the same expansion.
	//
	// In front of the `#` is what keeps `${#=word}` the assignment it is in
	// that shell — `$#` with a default assigned to it — rather than a
	// length with a flag inside it.
scan:
	for s != "" {
		switch {
		case p.dialect.ParamTildeFlag && s[0] == '~':
			e.TildeFlags++
		case p.dialect.ParamSplitFlag && s[0] == '=':
			e.SplitFlags++
		default:
			break scan
		}
		s = s[1:]
	}

	// A `+` stands after that run and in front of everything else: measured,
	// `${~+x}` reads and `${+~x}` does not, `${(t)+x}` reads and `${+#x}`
	// does not — and `${#+x}` is not this construct at all but the `$#`
	// parameter with an alternate word.
	//
	// What may follow it is a name or a positional and nothing else, which
	// is checked here rather than after the name scan: `${+#v}` would
	// otherwise be read as a length over a set test, and the shell that has
	// the construct calls it a bad substitution. Deferred to the run like
	// every other unreadable expansion in this grammar — measured, `${+?}`
	// inside a branch never taken is no error at all.
	if p.dialect.ParamSetTestFlag && strings.HasPrefix(s, "+") {
		e.SetTest = true
		s = s[1:]
		if !setTestNameStarts(s) {
			e.Bad, e.Src = true, src
			return e
		}
	}

	switch {
	case strings.HasPrefix(s, "#") && len(s) > 1 && !hashIsTheParameter(s[1:]):
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

	// What may stand here is decided in one place, and it is not this one:
	// scanNestedExpansion returns false for anything that is not a
	// substitution. A prefix test here as well was a second copy of the same
	// rule, and mutants that widened *this* one were unobservable because
	// the other still refused — which is how the duplication was found.
	if p.dialect.NestedParamExpansion {
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
	if e.Name == "" && !e.HasFlags && e.TildeFlags == 0 && e.SplitFlags == 0 &&
		e.Inner == nil {
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
	// A tilde run relaxes it the same way, and so does an `=` run: `${~}`
	// and `${=}` are the empty string too.

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
			inner := s[1:i]
			e.Index = p.wordFrom(inner, start, Unquoted)
			if p.dialect.ArraySubscriptFlags {
				if g, rest, isGroup := scanSubscriptFlags(inner); isGroup {
					// The operand is lexed as a word of its own, so a
					// substitution inside it is performed exactly as one in
					// the subscript would be: `${path[(re)${ZPFX}/bin]}` is
					// the whole of why this construct is worth having.
					g.Arg = p.wordFrom(rest, start, Unquoted)
					e.IndexFlags = g
				}
			}
			s = s[i+1:]
		}
	}
	if s == "" {
		return e
	}

	op, rest, ok := p.scanParamOp(s, e)
	if ok && e.Length && !p.dialect.ParamLengthTakesAnOperator {
		// `${#v#a}` is a bad substitution in four of the five: the length
		// takes no operator there. Marked and deferred rather than failed,
		// in every dialect — measured, none of the four decides this while
		// reading, so `${#v#a}` in a branch never taken is not an error at
		// all. That includes the one grammar that *does* refuse an unknown
		// operator early, which is why BadSubstitutionAtParseTime is not
		// consulted here; the `@` family above is deferred by it for the
		// same measured reason.
		//
		// After the operator scan rather than before it, so a `#` that is
		// not an operator at all cannot be mistaken for one: `${#v}` never
		// reaches this, and neither does `${#a[@]}`.
		e.Bad, e.Src = true, src
		return e
	}
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
	p.fillParamArgs(e, rest, start, q)
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
// setTestNameStarts reports whether s begins the only thing `${+` may be
// followed by: a name, or a positional parameter's digits.
//
// Every other parameter is refused, which is measured and is not what the
// name "is it set" suggests — `${+@}`, `${+*}`, `${+#}`, `${+?}`, `${+$}`,
// `${+!}` and `${+-}` are bad substitutions in the shell that has the
// construct, and so is `${+}` with nothing after it at all.
// hashIsTheParameter reports whether the `#` at the front of an expansion is
// the parameter `$#` rather than the length prefix — so `${#=w}` is `$#` with
// a default assignment on it and not the length of anything.
//
// The same shape the `!` above uses: a prefix that could begin a name is the
// prefix, and one that cannot is the parameter itself. What decides it is the
// *operator*, and the set is measured rather than reasoned, because the two
// readings are not separated by anything as tidy as "an operator follows".
// Measured 2026-09-07 on zsh 5.9.2 with `set -- p q`, so `$#` is 2 and `$-`
// is five characters:
//
//	${#=w}    2       the parameter, with `=w` never firing since `$#` is set
//	${#:=w}   2       and the same
//	${#+w}    w       the parameter, and its alternate does fire
//	${#:+w}   w
//	${#:?w}   2
//	${#-w}    bad substitution  — the *length* of `$-`, then a stray `w`
//	${#?w}    bad substitution  — the length of `$?`, then a stray `w`
//	${#:-w}   1       the length of the nameless `${:-w}`, which is `w`
//
// So `-` and `?` are names and stay lengths, and `:-` is a third reading
// again — the nameless expansion this grammar does not have. The operators
// that read `#` as the parameter and are not in this set are filed rather
// than carried: `${##2}`, `${#%2}`, `${#/2/X}` and `${#:0:1}` are all `$#`
// there, and `${##}` is the *length* of `$#`, which is the same two
// characters resolved the other way — a backtracking parse rather than a
// lookahead, and not a set this can express.
func hashIsTheParameter(s string) bool {
	switch {
	case s == "":
		return false
	case s[0] == '=' || s[0] == '+':
		// Neither can begin a name, so there is no length reading to
		// compete: `${#=w}` and `${#+w}` are `$#` in all six shells.
		return true
	case s[0] == ':':
		// Every `:` operator but `:-`. That one is the exception rather than
		// a rule about the colon, and it is measured: `${#:-w}` is `$#` in
		// five shells and 1 in zsh, where it is the length of the nameless
		// `${:-w}`. Reading it as the parameter here would answer five
		// shells and give the sixth a plausible number instead of the loud
		// refusal it has now, which is the wrong trade — see the follow-up
		// the spec entry names.
		return len(s) > 1 && s[1] != '-'
	case s[0] == '%' || s[0] == '/':
		// With an operand. `${#%2}` and `${#/2/X}` are `$#` trimmed and
		// replaced in every shell that has the operator, while a bare
		// `${#%}` is a bad substitution in five of the six.
		return len(s) > 1
	case s[0] == '#':
		// The same, and the reason the operand is required: `${##2}` is `$#`
		// with a `2` stripped off its front, and `${##}` is the *length* of
		// `$#` — the same two characters resolved the other way, unanimously.
		return len(s) > 1
	}
	// Anything else begins a name, including the special ones: `${#-}` and
	// `${#?}` are lengths, so `${#-w}` and `${#?w}` are a length with a
	// stray word after it.
	return false
}

func setTestNameStarts(s string) bool {
	return s != "" && (isNameStart(s[0]) || (s[0] >= '0' && s[0] <= '9'))
}

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
	// The colon before a trim is ignored in one shell, so the operator is
	// the trim it would have been without it. After the element-selection
	// case above, which is the other reading of `:#`; no shell sets both,
	// and the order makes the pair defined rather than accidental.
	//
	// Read by dropping the colon and scanning again rather than by naming
	// the four operators here, so `##` against `#` and `%%` against `%` stay
	// decided in one place. The recursion cannot run away: it is entered
	// only when the next character is `#` or `%`, and both of those return
	// on the following pass.
	case p.dialect.ParamColonBeforeTrimIsIgnored && len(s) >= 2 &&
		s[0] == ':' && (s[1] == '#' || s[1] == '%'):
		return p.scanParamOp(s[1:], e)
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
func (p *Parser) fillParamArgs(e *ParamExpr, rest string, start Pos, q Quoting) {
	// Only a *word* operand takes the enclosing quoting; a pattern does not.
	// Measured 2026-09-07, unanimous across the panel with `s=xay`:
	// `"${s#'x'}"` is `ay`, so the quotes in a pattern quote and are removed,
	// while `"${u:-'$v'}"` is `'VAL'`, where they are two characters of the
	// result and the `$v` between them is still substituted. The two readings
	// are the same characters and different rules, and asking which operand
	// this is is the only way to tell them apart.
	word := q
	switch e.Op {
	case ParamReplace:
		// The separator is an unquoted slash, so a slash inside quotes or
		// after a backslash belongs to the pattern.
		if i := indexUnquoted(rest, '/'); i >= 0 {
			e.Arg = p.wordFrom(rest[:i], start, Unquoted)
			e.Arg2 = p.wordFrom(rest[i+1:], start, Unquoted)
		} else {
			// Omitting the replacement deletes the match.
			e.Arg = p.wordFrom(rest, start, Unquoted)
		}
	case ParamSubstring:
		if i := indexUnquoted(rest, ':'); i >= 0 {
			e.Arg = p.wordFrom(rest[:i], start, Unquoted)
			e.Arg2 = p.wordFrom(rest[i+1:], start, Unquoted)
		} else {
			e.Arg = p.wordFrom(rest, start, Unquoted)
		}
	case ParamUpper, ParamLower, ParamToggle,
		ParamUpperFirst, ParamLowerFirst, ParamToggleFirst:
		// What follows is the pattern saying which characters to convert.
		// Empty means every one of them, which is what `?` would say.
		if rest != "" {
			e.Arg = p.wordFrom(rest, start, Unquoted)
		}
	case ParamTrimPrefix, ParamTrimPrefixLong, ParamTrimSuffix, ParamTrimSuffixLong,
		ParamExclude, ParamSetDifference, ParamSetIntersection:
		if rest != "" {
			e.Arg = p.wordFrom(rest, start, Unquoted)
		}
	default:
		if rest != "" {
			e.Arg = p.wordFrom(rest, start, word)
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
func (p *Parser) wordFrom(text string, at Pos, q Quoting) *Word {
	if text == "" {
		return &Word{Start: at, Stop: at}
	}
	if p.depth >= maxParamDepth {
		p.fail("expansions nested too deeply")
		return &Word{Start: at, Stop: at}
	}
	p.depth++
	defer func() { p.depth-- }()

	if q == DoubleQuoted {
		return p.quotedWordFrom(text, at)
	}

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

// quotedWordFrom is wordFrom for an operand that stands inside double quotes.
//
// Its text is read as double-quoted *content* rather than as a word, because
// that is what the enclosing quotes made it. The difference shows on a single
// quote: there it is an ordinary character rather than a quote, and what is
// written between two of them is still substituted. Measured 2026-09-07,
// unanimous across bash 5.3.15, bash 3.2.57, that build invoked as `sh`, dash,
// ksh93u+ and zsh 5.9.2:
//
//	v=VAL; printf '[%s]' "${u:-'$v'}"          ['VAL']
//	printf '[%s]' "${u:-'$(echo hi)'}"         ['hi']
//	printf '[%s]' "${u:-''}"                   ['']
//	printf '[%s]' "${u:-'a$(b'}"               a substitution that never closes
//
// So the substitution inside the quotes is *recognized and performed*, not
// merely scanned past for a delimiter — the first two rows are what separate
// those two readings, and the last is the same fact reaching the parse: with
// the `'` an ordinary character there is nothing to end the `$(` and every
// shell in the panel refuses the line.
//
// There is no closing quote to find, because the one that opened the context
// is outside this text: running out of input is the end of the operand rather
// than an unterminated quote.
func (p *Parser) quotedWordFrom(text string, at Pos) *Word {
	sub := NewLexer(text, p.operandDialect())
	spans := sub.scanDoubleBody(at, false)
	if err := sub.Err(); err != nil && p.err == nil {
		p.err = err
	}
	// Through newWord, so a nested ${ } in the operand is parsed too — and
	// parsed knowing it is in this same quoting, since the spans carry it.
	return p.newWord(spans, at, at)
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
	// Or the same substitution in double quotes, which is what `${(@f)"$(cmd)"}`
	// is written with and is a *different program* from the unquoted one:
	// quoted, the inner comes to one field and the outer flags split that;
	// unquoted, it is split on IFS first and the flags then see several.
	// Measured on zsh 5.9.2 — `${(@f)"$(printf "a b\nc")"}` is two fields
	// and `${(@f)$(printf "a b\nc")}` is three.
	//
	// Only double quotes, and only around a substitution: `${"abc"}`,
	// `${'$(cmd)'}` and `${"$v"}` are all a bad substitution there, so what
	// the quotes may hold is exactly what they may hold without them.
	quoted := strings.HasPrefix(s, `"`)
	body := s
	if quoted {
		body = s[1:]
	}
	if !strings.HasPrefix(body, "${") && !strings.HasPrefix(body, "$(") {
		return nil, "", false
	}
	// Which is also the whole of the test, for the quoted spelling as much as
	// the bare one. A kind check on the span behind it could not be reached:
	// those two prefixes lex to a substitution and nothing else does, so a
	// guard on the kind was a branch no mutation could tell from its absence
	// — and a check that the span came back *double*-quoted is unreachable
	// for the same reason, because the only quote stripped above is a `"`.
	// The single-quoted spelling is refused by that strip and by nothing
	// else, which is where to look if `${(@f)'$(cmd)'}` ever starts parsing.
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
