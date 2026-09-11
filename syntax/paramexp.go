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
	// ParamAssignAlways is `::=`: assign the word and substitute it, with no
	// test at all.
	//
	// It is not ParamAssign with a second colon. ParamAssign *asks* — is the
	// parameter unset, or unset-or-empty when a colon was written — and
	// substitutes the parameter when the answer is no; this operator asks
	// nothing, so [ParamExpr.Colon] has no meaning on it and there is no
	// non-firing side for the parameter's own value to be returned on.
	//
	// One grammar in the panel has it. The rest read the same characters as
	// a substring whose offset is empty and whose length is `=word`, and
	// fail in arithmetic — `${v::=rst}` is `operand expected at \`=rst'` in
	// bash 5.3, bash 3.2 and ksh93, and a bad substitution in dash.
	ParamAssignAlways
	// ParamElementReplace is `:/`: the elements a pattern matches **whole**
	// are replaced by the word after the second slash and the rest are left
	// exactly as they were.
	//
	// The fourth member of ParamExclude's family and the only one that
	// writes rather than only choosing, so it shares that one's
	// disambiguation — the single character after the colon — and not
	// ParamReplace's reading, which substitutes a matching *span* inside a
	// value. `${x:/foo/Z}` on `foobar` is `foobar` where `${x/foo/Z}` is
	// `Zbar`.
	//
	// The operands are ParamReplace's: Arg is the pattern up to the first
	// unquoted slash and Arg2 the replacement, which is everything after it
	// however many slashes that holds. Omitting the slash is the empty
	// replacement rather than a shape of its own.
	ParamElementReplace
)

func (o ParamOp) String() string {
	switch o {
	case ParamDefault:
		return "-"
	case ParamAssign:
		return "="
	case ParamAssignAlways:
		return "::="
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
	case ParamElementReplace:
		return ":/"
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
	// BareIndexText is that same subscript read as *text*: the `[`, whatever
	// stands between the brackets, and the `]`, lexed as a word in the
	// expansion's own quoting. Nil unless the expansion was written without
	// braces and carried a subscript at all.
	//
	// Two readings of one span, kept side by side because nothing here can
	// choose between them. A grammar with BareSubscript reads `$a[1]` as an
	// element; the same grammar can be told at run time that an unbraced
	// name's brackets are ordinary characters, and then the same text is the
	// parameter followed by three of them. Measured on the shell with the
	// construct: the answer is the one in force when the word is *expanded*
	// and not when it was read — a function body written under one answer
	// and called under the other takes the caller's.
	//
	// So the parser records both and the run picks, which is also why this
	// is a word rather than a string: the brackets are text, and text in a
	// word is expanded, split and matched like any other.
	BareIndexText *Word
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
	// Arg2Enclosed is the *replacement* operand read a second way: as
	// content of the quoting around the expansion, rather than as a word of
	// its own. Nil unless the two readings could differ, which is what makes
	// it the question rather than an alternative to Arg2.
	//
	// Both readings are kept for the reason ArithIndex.Sub keeps the raw text
	// beside the parsed expression: which of the two applies is not a
	// property of the text. The panel divides on it — bash 5.3, that build as
	// `sh` and ksh93 read the operand as a word, so `"${s/a/'$v'}"` is
	// `x$vy`, while bash 3.2 and zsh read it as double-quoted content, where
	// the quotes are two characters of the result and the `$v` between them
	// is still substituted, giving `x'VAL'y`. Unquoted, all five agree with
	// the first reading, which is what says the disagreement belongs to the
	// enclosing context. Answered by
	// interp.Semantics.ReplacementOperandTakesTheEnclosingQuoting (#1209).
	//
	// The pattern operand never takes it: its quotes quote in every column,
	// unanimously, which is why only Arg2 has a second reading.
	Arg2Enclosed *Word
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
	//
	// The `+` or `-` a `q` ate is stripped too, into QuoteModifier, so that
	// every `-` left in here is the sort flag and nothing has to ask which
	// reading a given one had.
	Flags string
	// QuoteModifier is the `+` or `-` the group's `q` ate — `q-` for minimal
	// quoting and `q+` for the extended form — and 0 when the group has
	// neither. It is not in Flags, because the same two characters name a
	// flag apiece there: `-` is the signed-numeric sort, and `+` is nothing
	// at all.
	//
	// Which of the two readings a written `-` had is settled here, once, by
	// scanParamFlags. Measured on zsh 5.9.2, 2026-09-08, with
	// `b=(-1 -10 -3 2 10)`:
	//
	//	${(o-)b}     -10 -3 -1 2 10   signed, so a lone `-` is a sort flag
	//	${(oq-)b}    -1 -10 -3 10 2   lexical: the `q` ate the `-`
	//	${(oq--)b}   -10 -3 -1 2 10   signed: the *second* `-` is the flag
	//	${(oq+-)b}   -10 -3 -1 2 10   signed: a `q+` had already taken it
	//	${(qU-)v}    A\ B             adjacency is literal, not associative
	//
	// The character is always the one directly behind the group's first `q`;
	// the scan refuses it anywhere else. So a reading that needs its
	// position has it as one past that `q` rather than as a second copy of
	// this rule.
	QuoteModifier byte
	// SplitSep and JoinSep are the arguments of the `s` and `j` flags. An
	// empty SplitSep with an `s` in Flags is meaningful — it splits into
	// characters — which HasFlags plus the letter already distinguish from
	// no `s` at all.
	SplitSep string
	JoinSep  string
	// PadLeft and PadRight are the arguments of the `l` and `r` flags, which
	// pad a word out to a width. Whether either flag was written at all is
	// Flags' question, exactly as it is for `s`: a group may name a width of
	// zero, and a zero width is not the same as no padding — see ParamPad.
	PadLeft  ParamPad
	PadRight ParamPad
	// ShellSplitOpts is the option letters the shell-word split runs with:
	// what the `Z` flag's delimited arguments came to, `${(Z+Cn+)v}`
	// carrying "Cn".
	//
	// It is the *accumulated* set rather than one argument's text, because
	// the group may write the flag more than once and may write its
	// argumentless spelling, `z`, in among them. Measured on zsh 5.9.2, the
	// one shell with the flag, on `$'a # h\nb'`:
	//
	//	${(Z+C+Z+n+)v}   a  b          two `Z` arguments union
	//	${(Z+n+Z+C+)v}   a  b          in either order
	//	${(zZ+n+)v}      a # h  b      a `Z` behind a `z` still counts
	//	${(Z+n+z)v}      a # h ; b     but a `z` behind it clears the set
	//	${(Z+C+zZ+n+)v}  a # h  b      clearing what stands before it only
	//	${(Z+n+Z::)v}    a # h  b      and an empty argument adds nothing
	//
	// So `z` is not "the `Z` flag with no letters" written once and for all:
	// it is a reset, and the reset happens where it is written. Reading the
	// last argument alone — which is what one assignment per `Z` came to —
	// answers the first two rows with `n` only and loses the dropped
	// comment.
	//
	// Empty is meaningful and is not the same as no split at all. `${(Z::)v}`
	// on `a  b` is the value unchanged, double space and all, where
	// `${(z)v}` is `a b` — so an empty option list leaves the flag doing
	// nothing, while `z` splits with no options set. Which of the two a
	// group means is Flags' question: a `z` in it splits, a `Z` in it splits
	// when this is non-empty.
	ShellSplitOpts string
	// EscapeOpts is the option letters the `g` flag's escape reading runs
	// with: `o` for octal escapes that need no leading zero, `e` for the
	// `\M-x` family, and `c` for `^X`. `${(g:oe:)v}` carries "oe" and the
	// bare `${(g::)v}` carries nothing at all.
	//
	// Accumulated rather than assigned, for the reason ShellSplitOpts is:
	// a group may write the flag twice and the sets union. Measured on zsh
	// 5.9.2, the one shell with the flag, on `X\EY` and `X\101Y`:
	//
	//	${(g::g:e:)v}   the escape character   an empty argument adds none
	//	${(g:e:g::)v}   the escape character   and takes none away
	//	${(g:o:g::)w}   A                      so `o` survives the second
	//
	// Empty is meaningful and is not the same as no `g` at all: `${(g::)v}`
	// reads escapes with no option set, where a group with no `g` reads none.
	// Which of the two it is, is Flags' question.
	EscapeOpts string
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

	// Leading holds the subscripts written *before* Index, in written order,
	// where the grammar lets several be chained: `${m[k][2]}` carries `k`
	// here and `2` in Index.
	//
	// Index is the last one and not the first, which looks backwards and is
	// the whole point. Every question anything asks about a subscripted
	// expansion — whether it is the whole array, whether it is a range, how
	// many fields it makes, whether `${#…}` is a count or a width — is a
	// question about the *final* subscript, because that is the one whose
	// result the expansion is. The leading ones only say what it is read
	// against. Keeping the final one where a single subscript has always
	// lived means every one of those readings answers a chain correctly
	// without being told chains exist; putting the first one there would
	// have needed each of them found and changed, and the one that was
	// missed would have answered a plausible value.
	Leading []LeadingIndex

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

	// RcExpandFlags is how many `^` characters were written in the same
	// slot — `${^x}` carries 1 and `${^^x}` carries 2. Zero is the ordinary
	// expansion.
	//
	// A count for the reason TildeFlags and SplitFlags are counts: parity is
	// the meaning. An odd number distributes the word the expansion stands
	// in over the elements the expansion came to — `a=(1 2); x${^a}y` is
	// `x1y x2y` where `x${a}y` is `x1 2y` — and an even one refuses to, both
	// regardless of the `RC_EXPAND_PARAM` option that would otherwise
	// decide, which is measured.
	//
	// Its own field rather than a third `~`/`=` counter for the reason those
	// two are separate from each other: the three decide three different
	// questions and a script writes more than one of them, `${=^a}` and
	// `${^=a}` being the same expansion, so each has to be counted on its
	// own for parity to mean anything.
	RcExpandFlags int

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

// ParamPad is one padding flag's arguments: the width and the two optional
// fill strings of `${(l:expr::string1::string2:)x}` and of its `r` mirror.
//
// The two Set fields are the reason this is a struct rather than three
// strings. Whether a fill was *written* is a distinction the shell keeps,
// measured on zsh 5.9.2 with `v=ab` and `IFS=.`:
//
//	${(l:5:)v}         `   ab`   no string1: the padding is spaces
//	${(l:5:::)v}       `...ab`   string1 written empty: IFS's first character
//	${(l:5::x:::)v}    `xx.ab`   string2 written empty: the same character,
//	                             inserted once against the value
//	${(l:5::x:)v}      `xxxab`   no string2: nothing is inserted
//
// So an absent fill and an empty one are different answers, and only under a
// default IFS do they agree.
//
// The slots survive a flag written twice, which is also measured: with
// `v=ab`, `${(l:3::x::y:l:5:)v}` is `xxyab` and `${(l:3::x::y:l:5::z:)v}` is
// `zzyab`. So a later `l` sets the width always and each fill only where it
// wrote one — the fields are three slots the group fills in, rather than one
// argument list the last flag replaces.
type ParamPad struct {
	// Width is the field width as written. It is an arithmetic expression
	// rather than a number — `${(l:COLUMNS-1:)x}` and `${(l:$n:)x}` are both
	// in reach on this machine — so the text is kept and evaluated when the
	// expansion runs.
	Width string
	// Fill is `string1`, repeated as often as needed to fill the space.
	Fill string
	// Insert is `string2`, laid once directly against the word before Fill
	// produces the rest.
	Insert string
	// FillSet and InsertSet report whether the group wrote that argument at
	// all, empty or not.
	FillSet   bool
	InsertSet bool
}

// specialParams are the one-character parameters that are not names.
const specialParams = "@*#?-$!0123456789"

// parseParamExp parses the text between `${` and `}`.
//
// The lexer already found the matching brace, tracking quoting so a `}` inside
// quotes did not end it early, so src here is exactly the inside.
//
// bare says the expansion was written without braces, which is Span.Bare and
// is read for one thing only: the subscript such an expansion carries is
// recorded as text as well as as a subscript, because a run-time answer
// decides which of the two it is. See ParamExpr.BareIndexText.
func (p *Parser) parseParamExp(src string, start Pos, q Quoting, bare bool) *ParamExpr {
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

	// A run of `~`, `=` and `^` stands between the flag group and everything
	// else, which is where the shell that has them puts them: `${(U)~g}`
	// reads, `${~(U)g}` is a bad substitution, and `${(U)~#g}` is a length —
	// so after the group and in front of the `#` below. The three characters
	// share the slot and are interchangeable within it, measured: `${=~g}`
	// and `${~=g}` are the same expansion, and so are `${=^a}` and `${^=a}`.
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
		case p.dialect.ParamRcExpandFlag && s[0] == '^':
			e.RcExpandFlags++
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
	case strings.HasPrefix(s, "#") && len(s) > 1 &&
		!hashIsTheParameter(s[1:], p.dialect.NamelessParamExpansion):
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
		e.RcExpandFlags == 0 && e.Inner == nil && !p.dialect.NamelessParamExpansion {
		if !p.dialect.BadSubstitutionAtParseTime {
			// The majority defers an unreadable expansion to the run, the
			// same way an unknown operator is deferred: a `${%x}` in a
			// branch never taken is not an error at all.
			e.Bad, e.Src = true, src
			return e
		}
		p.failKind(ErrBadSubstitution, "expected a parameter name in ${%s}", src)
		if pe, isErr := p.err.(*Error); isErr {
			// The character standing where the name belonged, for the one
			// dialect that words this as a syntax error naming the token —
			// left empty, it printed `' unexpected.
			//
			// With nothing at all between the braces the character standing
			// there is the brace that closed them: measured, `${}` in ksh93
			// is `syntax error at line 1: `}' unexpected`, naming the `}`
			// rather than the nothing in front of it.
			pe.Token = "}"
			if s != "" {
				pe.Token = firstRune(s)
			}
		}
		return e
	}
	// With a flag group the name may be empty — `${(U)}` is an empty string
	// and `${(%):-%x}` is all operator — so an operator may still follow.
	// A tilde run relaxes it the same way, and so do an `=` run and a `^`
	// run: `${~}`, `${=}` and `${^}` are the empty string too.
	//
	// NamelessParamExpansion relaxes it with none of those in front of it,
	// which is what the group was standing in for: `${(%):-%x}` read here
	// only because the group happened to be present, and `${:-%x}` did not,
	// so the group was gating a reading it has nothing to do with. With the
	// flag the name may simply be absent, and the group is back to deciding
	// only how the result is rendered — measured, `${(U):-abc}` is `ABC` and
	// `${:-abc}` is `abc`, the same reading with and without it.

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
		// A loop rather than one read, because a grammar with
		// ChainedSubscript writes several: `${m[k][2]}` is the value under
		// `k` and then the second character of it. Without the flag the loop
		// runs once and the second bracket is left unconsumed, which is what
		// makes the whole expansion a bad substitution in the four grammars
		// that call it one.
		for strings.HasPrefix(s, "[") {
			i := closingBracket(s)
			if i <= 0 {
				break
			}
			inner := s[1:i]
			idx := p.wordFrom(inner, start, Unquoted)
			var g *SubscriptFlags
			if p.dialect.ArraySubscriptFlags {
				if group, rest, isGroup := scanSubscriptFlags(inner); isGroup {
					// The operand is lexed as a word of its own, so a
					// substitution inside it is performed exactly as one in
					// the subscript would be: `${path[(re)${ZPFX}/bin]}` is
					// the whole of why this construct is worth having.
					group.Arg = p.wordFrom(rest, start, Unquoted)
					g = group
				}
			}
			if e.Index != nil {
				// The one already read is not the last after all — see
				// Leading, which is where every subscript but the final one
				// goes.
				e.Leading = append(e.Leading, LeadingIndex{Index: e.Index, Flags: e.IndexFlags})
			}
			e.Index, e.IndexFlags = idx, g
			if bare {
				// The same brackets read the other way: as the text they
				// would be if nothing here had taken them for a subscript.
				// Both readings are kept because the choice between them is
				// not the grammar's — see the field.
				//
				// Lexed in the expansion's own quoting, so what is inside is
				// performed exactly as the surrounding word would perform
				// it: `$a[$b]` where the brackets are text still expands the
				// `$b` between them, measured.
				e.BareIndexText = p.wordFrom(s[:i+1], start, q)
			}
			s = s[i+1:]
			if !p.dialect.ChainedSubscript {
				break
			}
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
// the separators of `s` and `j`, the width and two fills of the padding
// pair, and the argument groups of the grouping flags — scanned so the
// group's closing parenthesis is still found even for the letters the
// interpreter refuses.
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
	// The state the `q` modifier's grammar needs, carried as the group is
	// read: the last flag character and where it stood, how many `q` have
	// been seen, and whether a `q-` has been taken.
	//
	// Measured on zsh 5.9.2, 2026-09-08, every row of it — the accepted
	// spellings and the refused ones alike, since a grammar is only pinned
	// by what it turns away:
	//
	//	${(q-)v}    minimal      the modifier is the character behind the `q`
	//	${(q+)v}    extended
	//	${(oq-)b}   minimal      and the `q` need not open the group
	//	${(-q-)v}   minimal      a `-` in front of it is the sort flag
	//	${(qU-)v}   plain `q`    adjacency is literal, not associative
	//	${(q--)v}   minimal      the second `-` is the sort flag
	//	${(q+-)w}   extended     and so is the one behind a `q+`
	//	${(+)v}     error at 4   a `+` on its own is no flag at all
	//	${(U+)v}    error at 5   nor is one behind any other letter
	//	${(q-+)v}   error at 6   nor one behind the modifier
	//	${(qq-)v}   error at 5   only the group's *first* `q` takes one,
	//	${(qq+)v}   error at 5   and the position reported is that `q`'s
	//	${(qoq-)v}  error at 6   wherever in the group it stands
	//	${(q-q)v}   error at 6   a `q-` group takes no further `q`,
	//	${(q-Uq)v}  error at 7   adjacent or not
	//	${(q+q)v}   read         where a `q+` group does — which is the one
	//	${(q+Uq)v}  read         asymmetry, and it is measured, not derived
	//
	// The two `q+` rows the scan lets through are answered by that shell
	// with a spelling that does not read back — `${(q+q)u}` on `has'quote`
	// is `'has'quote'` — so the interpreter refuses them by name rather
	// than reproducing them. This scan's job is the grammar; what a legal
	// group means is not its question.
	prev, prevAt, qSeen, minusTaken, bSeen := byte(0), 0, 0, false, false
	for i < len(src) {
		c := src[i]
		if c == ')' {
			return src[i+1:]
		}
		if strings.IndexByte(paramFlagChars, c) < 0 {
			e.FlagsErrPos = i + 3
			return ""
		}
		switch c {
		case '+', '-':
			switch {
			case prev == 'q' && qSeen == 1:
				// The modifier. It is kept off Flags so that every `-` left
				// there is the sort flag and no later reading has to ask.
				e.QuoteModifier = c
				minusTaken = minusTaken || c == '-'
				prev, prevAt = c, i
				i++
				continue
			case prev == 'q':
				e.FlagsErrPos = prevAt + 3
				return ""
			case c == '+':
				e.FlagsErrPos = i + 3
				return ""
			}
			// Anywhere else a `-` is the signed-numeric sort flag.
		case 'q':
			if minusTaken || bSeen {
				e.FlagsErrPos = i + 3
				return ""
			}
			qSeen++
		case 'b':
			// `b` and `q` are one family and a group may write one member of
			// it once. Measured on zsh 5.9.2, 2026-09-10, and the position
			// reported is always the *second* member's own:
			//
			//	${(bq)v}    error at 5   the `q` behind a `b`
			//	${(qb)v}    error at 5   the `b` behind a `q`
			//	${(bb)v}    error at 5   a second `b`
			//	${(bUq)v}   error at 6   adjacency has nothing to do with it
			//	${(qUb)v}   error at 6
			//	${(bUb)v}   error at 6
			//	${(q-b)v}   error at 6   a modifier does not spend the `q`
			//	${(q+b)v}   error at 6
			//	${(qqqb)v}  error at 7   nor does a repeat
			//	${(j:x:bq)a} error at 9  the count is of source characters
			//	${(bQ)v}    read         `Q` is not in the family
			//	${(b-)v}    read         and the `-` behind a `b` is the sort
			//	                         flag, exactly as it is anywhere else
			//
			// Reported here rather than by the interpreter because it is the
			// grammar that turns it away: the group means nothing, which is
			// this scan's kind of failure and not a refusal by name.
			if bSeen || qSeen > 0 {
				e.FlagsErrPos = i + 3
				return ""
			}
			bSeen = true
		}
		e.Flags += string(c)
		prev, prevAt = c, i
		if c == 'z' {
			// The argumentless spelling of the shell-word split *clears* the
			// option letters written in front of it, which is measured and is
			// the whole reason this stands in the letter loop rather than
			// being read off Flags later: only here is the written order of a
			// `z` and a `Z` argument still known. See ShellSplitOpts.
			e.ShellSplitOpts = ""
		}
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
			case 'l', 'r':
				// Three slots filled in by position, and only where the
				// group wrote one. See ParamPad for why a fill written
				// empty and a fill not written at all are kept apart, and
				// for the measurement that says a second `l` replaces the
				// width without clearing the fills.
				pad := &e.PadLeft
				if c == 'r' {
					pad = &e.PadRight
				}
				switch n {
				case 0:
					pad.Width = arg
				case 1:
					pad.Fill, pad.FillSet = arg, true
				default:
					pad.Insert, pad.InsertSet = arg, true
				}
			case 'g':
				if k := strings.IndexFunc(arg, notAnEscapeOpt); k >= 0 {
					// The same shape the `Z` argument's letters have, and
					// measured the same way: `${(g:x:)v}` is `error in flags
					// near position 6`, the position of the `x`, on the one
					// shell that has the flag.
					e.FlagsErrPos = i + 1 + k + 3
					return ""
				}
				// Unioned, as `Z`'s letters are. See EscapeOpts.
				e.EscapeOpts += arg
			case 'Z':
				if k := strings.IndexFunc(arg, notAShellSplitOpt); k >= 0 {
					// An option letter the flag does not have is an error
					// *in the flags*, at the letter, rather than a refusal
					// when the expansion is reached — which is the opposite
					// of how an unknown flag letter is handled and is
					// measured that way: `${(Z:x:)v}` is `error in flags
					// near position 6`, the position of the `x`, on the one
					// shell that has the flag.
					e.FlagsErrPos = i + 1 + k + 3
					return ""
				}
				// Unioned rather than assigned: a second `Z` adds its
				// letters to the first one's. See ShellSplitOpts.
				e.ShellSplitOpts += arg
			}
			i += j + 2
		}
	}
	e.FlagsErrPos = len(src) + 2
	return ""
}

// paramShellSplitOpts are the option letters the `Z` flag's argument may
// carry, established by trying the whole alphabet one letter at a time
// against the shell that has the flag: `c`, `C` and `n`, and nothing else —
// every other letter, digit and punctuation mark is an error in the flags at
// its own position.
const paramShellSplitOpts = "cCn"

func notAShellSplitOpt(r rune) bool {
	return !strings.ContainsRune(paramShellSplitOpts, r)
}

// paramEscapeOpts are the option letters the `g` flag's argument may carry,
// established the way paramShellSplitOpts was — the whole alphabet, one
// letter at a time, against the shell that has the flag: `o`, `e` and `c`,
// and nothing else, every other letter being an error in the flags at its own
// position.
const paramEscapeOpts = "oec"

func notAnEscapeOpt(r rune) bool {
	return !strings.ContainsRune(paramEscapeOpts, r)
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
// again — the nameless expansion, which is `nameless`'s whole job here. The
// operators that read `#` as the parameter and are not in this set are filed
// rather than carried: `${##2}`, `${#%2}`, `${#/2/X}` and `${#:0:1}` are all
// `$#` there, and `${##}` is the *length* of `$#`, which is the same two
// characters resolved the other way — a backtracking parse rather than a
// lookahead, and not a set this can express.
//
// nameless is NamelessParamExpansion, and `:-` is the only place it changes
// the answer. Measured 2026-09-08 with `set -- p q`: `${#:-w}` is `2` in the
// five shells without the nameless form and `1` in the one with it, while
// `${#:+w}` is `w`, `${#:=w}` is `2` and `${#:?w}` is `2` in all six — so
// the divergence is `:-` alone and not the colon.
func hashIsTheParameter(s string, nameless bool) bool {
	switch {
	case s == "":
		return false
	case s[0] == '=' || s[0] == '+':
		// Neither can begin a name, so there is no length reading to
		// compete: `${#=w}` and `${#+w}` are `$#` in all six shells.
		return true
	case s[0] == ':':
		// Every `:` operator, and `:-` as well in a grammar with no nameless
		// expansion to be a length over: there `${#:-w}` is `$#` with a
		// default that never fires, and the five answer `2`.
		//
		// Where the nameless form exists, `:-` is the exception rather than
		// a rule about the colon: `${#:-w}` is the length of `${:-w}`, which
		// is `1`. Nothing else in the colon family moves with it — `${#:+w}`
		// is `w` and `${#:=w}` and `${#:?w}` are `2` in all six — so the
		// exception is exactly one operator wide.
		return len(s) > 1 && (!nameless || s[1] != '-')
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
	// `::=` is the always-assign operator, and it has to be read before the
	// substring below: there, `${v::=w}` is an offset of nothing and a
	// length of `=w`, which is the arithmetic error `operand expected at
	// \`=w'` — the whole of #1369, eighteen lines of one real startup. It
	// cannot collide with the colon-prefixed conditionals ahead of the
	// switch: their second character is one of `-=?+` and this one's is
	// another colon.
	//
	// Only `=` gets the reading. Measured 2026-09-07 on zsh 5.9.2:
	// `${v::-new}` and `${v::+new}` are substrings and answer empty, and
	// `${v::?new}` is `bad math expression: operator expected at \`new'` —
	// so the second colon opens an operator for exactly one character and a
	// substring for the rest.
	case p.dialect.ParamAssignAlways && strings.HasPrefix(s, "::="):
		return ParamAssignAlways, s[3:], true
	// A `:` followed by one of these three is an operator rather than the
	// start of an offset, and that one character is the whole
	// disambiguation. Before the substring case, because everything here
	// would otherwise be read as arithmetic — which is exactly what a
	// grammar without the flag does, and what the error `operand expected
	// at \`#fig_precmd\'` was.
	case p.dialect.ParamElementSelection && len(s) >= 2 && s[0] == ':' &&
		strings.IndexByte("#|*", s[1]) >= 0:
		return elementSelectOp(s[1]), s[2:], true
	// `:/` is the same rule again, one flag further along: the whole-element
	// replacement, whose operands are the replacement's and whose reading is
	// the element family's. It cannot take input from the substring below,
	// because no arithmetic expression begins with a division — `${x:3/2}`
	// is an offset of `3/2` and `${x: /2}` is the error that says so.
	case p.dialect.ParamWholeElementReplace && len(s) >= 2 && s[0] == ':' &&
		s[1] == '/':
		return ParamElementReplace, s[2:], true
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
	case ParamReplace, ParamElementReplace:
		// The separator is an unquoted slash, so a slash inside quotes or
		// after a backslash belongs to the pattern.
		if i := indexUnquoted(rest, '/'); i >= 0 {
			e.Arg = p.wordFrom(rest[:i], start, Unquoted)
			e.Arg2 = p.wordFrom(rest[i+1:], start, Unquoted)
			// And the same text read as content of the quoting around the
			// expansion, where that could come to something else. See
			// ParamExpr.Arg2Enclosed.
			if q == DoubleQuoted && replacementReadingsCanDiffer(rest[i+1:]) {
				e.Arg2Enclosed = p.wordFrom(rest[i+1:], start, q)
			}
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

// replacementReadingsCanDiffer reports whether reading a replacement operand
// as a word of its own and reading it as double-quoted content could produce
// different text — which is what decides whether the axis is asked at all.
//
// Three characters part them, and the list is measured rather than reasoned
// out. With `s=xay` inside double quotes, the word reading (bash 5.3, that
// build as `sh`, ksh93) against the enclosing one (zsh):
//
//	'q'      q          against  'q'      a single quote quotes, or is a character
//	\q       q          against  \q       a backslash escapes, or stands before one
//	~        the home   against  ~        a leading tilde expands, or does not
//	"q"      q          against  q        the same either way
//	*        *          against  *        the same
//	{p,q}    {p,q       against  {p,q     the same
//	p~q      p~q        against  p~q      the same — only a leading tilde expands
//
// So a `"`, a glob, a brace group and a tilde that is not at the front are
// answered alike by both readings and must not reach the axis: a core that
// refused `"${s/a/b}"` would be refusing where the whole panel agrees.
//
// bash 3.2 answers the first three the way zsh does and keeps the `"` as a
// character besides, which is a further difference inside the keeping group.
// No dialect here targets that build, so it is recorded in the corpus rather
// than modeled.
func replacementReadingsCanDiffer(text string) bool {
	return strings.HasPrefix(text, "~") || strings.ContainsAny(text, `'\`)
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
	// An operand is not a command, so no arithmetic command begins in one.
	// See Lexer.inOperand: the `((` reading is also a *lossy* one, so an
	// operand that reached it came back with its parentheses missing
	// (#1408).
	sub.inOperand = true
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
