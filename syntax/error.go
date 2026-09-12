// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// ErrorKind classifies a parse failure.
//
// It exists so a dialect can word the failure its own way without matching on
// message text. The set is small on purpose: it distinguishes only the cases
// the panel *words differently*, not every way a parse can fail. dash says
// "Bad substitution" for anything wrong inside `${ }` and "Syntax error: …"
// for everything else, so those are the two.
type ErrorKind int

const (
	// ErrSyntax is a parse failure with no more specific classification.
	ErrSyntax ErrorKind = iota
	// ErrBadSubstitution is a parse failure inside `${ }`. Every shell in
	// the panel has a distinct message for it, and none of them mentions
	// which operator was wrong.
	ErrBadSubstitution
	// ErrUnterminated is input that ran out with a construct still open.
	//
	// It is its own kind because the panel does not merely word it
	// differently — each shell names a *different part* of the same failure.
	// For `if true; then echo x`:
	//
	//	bash   unexpected end of file from `if' command on line 1
	//	dash   end of file unexpected (expecting "fi")
	//	ksh93  `then' unmatched
	//	zsh    parse error near `x'
	//
	// Four views of one state rather than four states, which is why Error
	// carries the construct, the innermost unclosed keyword, what was
	// expected and the last token seen: each dialect names the one it names.
	ErrUnterminated
	// ErrArithOperand is an arithmetic expression that wanted a value and
	// found something that could not be one: `$((%))`, `$((1+&2))`. Every
	// shell in the panel words this as an *arithmetic* failure rather than as
	// a syntax error, which is why it is its own kind — dash calls it
	// "expecting primary" and bash "operand expected", both inside the shape
	// they use for a division by zero.
	//
	// Token is the text from the refused byte to the end of the expression,
	// which is what the two shells that name anything here name.
	ErrArithOperand
	// ErrArithOperandEnd is an arithmetic expression that wanted a value and
	// ran out of text instead: `$((1+))`, `$((~))`.
	//
	// A separate kind because two of the panel word the two apart, and they
	// are the two that say the least otherwise: ksh93 has "more tokens
	// expected" against "arithmetic syntax error", and zsh "operand expected
	// at end of string" against "operand expected at `%'". bash words both
	// identically, which is why nothing had noticed — it is the column a
	// conformance number is usually read against.
	//
	// Token is the operator that was left wanting, which is what bash names.
	ErrArithOperandEnd
	// ErrArithOperator is an expression with something left over that could
	// have been an operand: `$((1 2))`, and in dash also `$((1,2))`, whose
	// comma it does not have. dash calls it "expecting EOF" and zsh
	// "operator expected".
	ErrArithOperator
	// ErrArithBadOperator is text where an operator belonged that could not
	// be one at all — `1 @`, or `1.5` in a dialect without floats, where the
	// `.5` is neither an operator nor part of the number.
	//
	// A separate kind because one shell words the two differently: bash says
	// "arithmetic syntax error in expression" when an operand stands where an
	// operator belonged and "invalid arithmetic operator" when the text could
	// not be either. The other three have one wording for both.
	ErrArithBadOperator
	// ErrArithCharacterMissing is the character-code operator with nothing
	// after it to take the code of: `$((##))`, or a backslash at the end of
	// the expression. Its own kind because the one dialect that has the
	// operator words it as neither an operand nor an operator failure —
	// `character missing after ##` — and a dialect without the operator never
	// reaches it at all.
	ErrArithCharacterMissing
	// ErrArithIllegalByte is a byte the dialect's arithmetic reader refuses as
	// part of no token at all, reported *at the byte* rather than as a
	// missing operand.
	//
	// Its own kind because one shell in the panel has a third sentence for it
	// — `illegal character: @` — and gives that sentence only where the token
	// stream could legally have ended: at the start of the expression, or
	// where an operator belonged. Where an operator has just been consumed
	// and an operand is wanted, the same byte gets the operand sentence
	// instead, so the byte alone does not decide. See Dialect's
	// ArithBytesRefusedOutright, which is the table; the position is the
	// parser's to know.
	//
	// Token is the single refused byte, which is what that shell names — not
	// the text from it to the end of the expression, which is what the
	// operand failures name.
	ErrArithIllegalByte
	// ErrArithBadOutputFormat is an output-format specifier the dialect that
	// has the construct could not read: `[#]`, `[##]`, `[foo]`, `[# 16]`,
	// `[#16 ]`. See Dialect's ArithOutputFormat for what the construct is.
	//
	// Its own kind because the sentence is about the *specifier* and not
	// about an operand or an operator — `bad output format specification` —
	// and it is raised where a value has not been wanted yet, so neither of
	// the operand kinds could carry it. A dialect without the construct never
	// reaches it: there, the `[` is an operand failure as it always was.
	//
	// Token is the specifier as written, brackets included.
	ErrArithBadOutputFormat
	// ErrArithBadBaseSyntax is a bracketed group holding nothing but digits —
	// `$(( [16] 255 ))` — which the one dialect with output formats words
	// differently again, as `bad base syntax`.
	//
	// Its own kind rather than a shape of ErrArithBadOutputFormat because the
	// two are measurably apart in that dialect and are told apart by one
	// character: `[16]` is this and `[#16]` is a format. A reading that
	// folded them would word half of the pair wrongly and nothing else would
	// notice, since both are failures either way.
	//
	// Token is the group as written, brackets included.
	ErrArithBadBaseSyntax
	// ErrUnexpected is a token where the grammar wanted something else. The
	// panel names the token three ways and one of them names its *class*
	// instead — dash says "word unexpected" for an ordinary word and quotes
	// a reserved word or an operator — so the class travels with it.
	ErrUnexpected
	// ErrUnmatched is input that ran out inside a quote, a command
	// substitution or a `${`. Its own kind because the panel splits three
	// ways over the same EOF: two dialects name the delimiter (one the
	// opener, one the closer), one echoes the text near it, and one closes
	// a quote quietly and runs what it got — which is a grammar flag, so
	// the error never exists there. Token is the opener as written, `'`,
	// `"`, a backquote, `$(` or `${`; Expected is what would have closed
	// it; LastToken is the text from the opener to the end of its line.
	ErrUnmatched
	// ErrCondOperand is an operand a conditional operator cannot take: a
	// token where `[[ ]]` wanted a word, so `[[ $k == (a|b) ]]` in a dialect
	// with no bare pattern groups, or `[[ -n ]]` with nothing after the
	// operator at all.
	//
	// Its own kind because one dialect words it as a statement about the
	// *operator* — "unexpected argument `(' to conditional binary operator" —
	// rather than as a token the grammar did not want, and the others use
	// the wording they use for any such token. Token is the offending one
	// and Expected is `unary` or `binary`, which is the only part of the
	// sentence that varies within the dialect that has one.
	ErrCondOperand
	// ErrForName is a `for` whose variable is not a name. Its own kind
	// because the panel does not word it as an unexpected token: three of the
	// four say something about the *name* and only the fourth blames the
	// word it found.
	ErrForName
)

// TokenClass is what sort of thing a token is, for the dialect that words an
// ordinary word differently from a reserved one.
type TokenClass int

const (
	// ClassOperator is punctuation: `&`, `)`, `}`.
	ClassOperator TokenClass = iota
	// ClassWord is an ordinary word, the one class dash does not quote.
	ClassWord
	// ClassReserved is a word the grammar reserves: `fi`, `do`, `esac`.
	ClassReserved
	// ClassNewline is the newline, which is a token here and is not
	// punctuation: the dialect that leaves a word unquoted leaves this one
	// unquoted too — `newline unexpected` beside `";;" unexpected` — and one
	// other spells it `\n` rather than by name.
	//
	// Its own class rather than an operator because both of those facts are
	// about the newline alone: every other member of ClassOperator is quoted
	// by that dialect and named by the characters it was written with.
	ClassNewline
)

// Error is a parse failure with its position, kind, and enough of the state
// it failed in that a dialect can word it its own way.
//
// The context fields are set where the parser has them and are empty
// otherwise. A caller that only wants a sentence still has Msg.
type Error struct {
	Pos  Pos
	Kind ErrorKind
	Msg  string

	// Construct is the compound command left open — `if`, `for`, `case`,
	// `{`, `(` — and ConstructLine is the line it began on. One shell names
	// both.
	Construct     string
	ConstructLine int
	// Innermost is the unclosed keyword nearest the failure, which is the
	// construct itself until a clause of it has been entered: `if` alone is
	// `if`, and `if cond; then` is `then`. Another shell names this instead.
	Innermost string
	// Expected is the word that would have closed it — `fi`, `done`, `esac`.
	Expected string
	// LastToken is the last token consumed before the input ran out, which
	// is what the remaining shell names.
	LastToken string
	// Class is what sort of token Token is, when the kind is ErrUnexpected.
	Class TokenClass
	// TokenOpener is the operator the unexpected token *began* with, where
	// the token is a construct whose text is not its own spelling. Empty for
	// everything else, which is nearly everything.
	//
	// One construct needs it. An arithmetic command standing where the
	// grammar has no command is named two ways in the panel — measured
	// 2026-09-11 on a script holding `x=1` and `(( 1 )) (( 2 ))`:
	//
	//	bash 5.3   syntax error near unexpected token ` 2 '
	//	zsh 5.9.2  parse error near ` 2 '
	//	ksh93u+    syntax error at line 2: `((' unexpected
	//	bash 3.2   syntax error near unexpected token `('
	//
	// So two of them quote the expression the construct held, blanks and
	// all, and one quotes the operator that opened it. Token cannot say
	// both, and which is written is the dialect's answer rather than the
	// parser's — see Diagnostics.SyntaxUnexpectedNamesTheOpener.
	TokenOpener string
	// TokenSource is the unexpected token as it was **written** — quotes,
	// backslashes and the text of an expansion included — where Token is
	// what the word comes to once the quoting is off.
	//
	// Two spellings because the panel splits three ways over the same word,
	// measured 2026-09-12 on a script holding `if true; then echo t; fi W`:
	//
	//	W          bash 5.3 / bash32 / bash-as-sh / zsh   ksh93
	//	"zzz"      `"zzz"`                                `zzz`
	//	'a b'      `'a b'`                                `a b`
	//	a\"b       `a\"b`                                 `a"b`
	//	$x         `$x`                                   `$x`
	//	"$x"       `"$x"`                                 `"$x"`
	//
	// so bash and zsh echo the source always, ksh93 echoes it only where the
	// word holds an expansion, and dash names no word at all. Which of the
	// two is written is the dialect's answer rather than the parser's — see
	// Diagnostics.UnexpectedWordNaming — so both travel, the way
	// TokenOpener does.
	//
	// Set for a word token and empty for everything else, whose source and
	// whose spelling are the same characters.
	TokenSource string
	// TokenHoldsExpansion says the unexpected word carried an expansion —
	// `$x`, `${x}`, `$(…)`, `` `…` `` or `$((…))`. One dialect reads it as
	// the question of which of the two spellings above to write.
	TokenHoldsExpansion bool

	// Redirect says the unexpected token was itself a redirection operator.
	//
	// One dialect words that case separately — dash says `redirection
	// unexpected` where the other three name the token — so the fact has to
	// travel with the error rather than being worked out from the text.
	Redirect bool
	// FuncBody says the grammar was waiting for a function's body, and that
	// the body never began — `f() ;` and `f()` rather than `f() {` with the
	// input running out inside the braces.
	//
	// One dialect reports this one failure without a line: zsh answers `f()
	// ;` with ``zsh: parse error near `;' `` where it answers `if true` with
	// ``zsh:1: parse error near `true' ``, and both are an input that ran out.
	// Whatever decides that is not the kind of failure, so the fact travels
	// with the error the way Redirect does rather than being worked out from
	// the text.
	//
	// Not set once a newline has come between the parens and the failure:
	// measured, the line comes back there — `f()` and a newline is
	// ``zsh:1: parse error near `\n' ``. Which line it then names is a
	// further divergence and is not modeled: zsh says 1 where the offending
	// token is on line 2.
	FuncBody bool
	// Expr is the whole arithmetic expression a failure was inside, and
	// Token the part of it the failure is attributed to. Every shell quotes
	// the first; only one names the second.
	Expr  string
	Token string
	// EndLine is the line *after* the input's last, which is where one shell
	// considers the end of input to be: `eval "if"` is line 2 there and line
	// 1 in the other three. Input that already ends in a newline puts Pos on
	// that line anyway, so the two agree there and differ only when the text
	// stops mid-line.
	EndLine int
	// EofLine is the line the input actually ran out on, in the lexer's
	// own count — the same point EndLine names in the next-line
	// convention. Two dialects report this one for an unmatched quote.
	EofLine int
}

func (e *Error) Error() string { return e.Pos.String() + ": " + e.Msg }
