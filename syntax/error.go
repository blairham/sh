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
	// ErrArithConditionalThen is `? ` with nothing after it to be the value
	// the condition chooses when it is true: `$(( 1 ? ))`.
	//
	// Its own kind because two of the four shells word it apart from an
	// ordinary missing operand — bash "expression expected" and ksh93
	// "':' expected for '?' operator", against their own "operand expected"
	// and "more tokens expected" — while zsh and dash reuse theirs. Token is
	// the `?` and everything after it, which is what bash names.
	ErrArithConditionalThen
	// ErrArithConditionalColon is a conditional whose two values are not
	// parted by a `:`: `$(( 1 ? 2 ))`. All four word it, and each its own
	// way. Token is the value that was read and everything after it.
	ErrArithConditionalColon
	// ErrArithConditionalElse is `:` with nothing after it to be the value
	// the condition chooses when it is false: `$(( 1 ? 2 : ))`.
	//
	// Apart from ErrArithConditionalThen because ksh93 parts the two — its
	// then is the colon complaint and its else the ordinary end of input —
	// and apart from ErrArithOperandEnd because bash parts *those*. Token is
	// the `:` and everything after it.
	ErrArithConditionalElse
	// ErrArithColonWithoutQuestion is a `:` standing where no `?` opened a
	// conditional: `$(( 1 : 2 ))`, and `$(( 1 ? 2 : 3 : 4 ))` after a
	// complete one.
	//
	// Two readers arrive here. Under Dialect.ArithColonIsAToken the colon is
	// read as a math token and the failure comes after the second value has
	// been found; everywhere else it is text left over, which is where
	// leftoverKind sends it. The kinds are one because the dialects that
	// word it apart word it the same in both readings — and because one of
	// them, ksh93, gives a stray colon a shape no other leftover gets, which
	// only a kind of its own can carry (#2224).
	//
	// Token is the text from the colon to the end of the expression. A
	// dialect with no sentence for it falls back to the leftover-text one,
	// which is what it said before this kind reached it.
	ErrArithColonWithoutQuestion
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
	// ErrForArithHeader is a C-style `for` header that does not hold two
	// separators: `for (())`, `for ((;))`, `for ((i=0))`, `for ((1;2))`.
	//
	// Its own kind because every shell in the panel refuses it and none of
	// them words it as a token the grammar did not want. Measured 2026-09-12
	// on `for ((i=0)); do :; done`:
	//
	//	bash 5.3.15   syntax error: arithmetic expression required
	//	bash 3.2.57   the same sentence
	//	bash-as-sh    the same sentence
	//	ksh93u+       `))' unexpected
	//	zsh 5.9.2     parse error near `i=0'
	//
	// so one of the three sentences is about the expression that was not
	// there, one blames the closer, and one names the text of the last part
	// — and names nothing where that part is empty, which is why Token and
	// LastToken are both carried.
	//
	// Token is the header as written, `((` and `))` included and the blanks
	// and newlines inside it kept, which is what one dialect echoes back on
	// a second line. LastToken is the last part trimmed, which is what
	// another names.
	ErrForArithHeader
	// ErrForArithSeparator is a C-style `for` header with *more* than two
	// separators: `for ((;;;))`, `for ((1;2;3;4))`.
	//
	// Apart from ErrForArithHeader because the panel splits twice over it.
	// It splits on whether the header is refused at all — bash refuses,
	// where ksh93 and zsh take the header and fold everything past the
	// second `;` into the third expression, so the refusal arrives at run
	// time and only if that expression is ever evaluated. That half is
	// Dialect.ForArithExtraSeparators. And in the dialect that does refuse
	// it, the sentence is not the one above: bash says `` `;' unexpected ``
	// where it says `arithmetic expression required` for too few. Two
	// refusals rather than one, so two kinds (#2225).
	//
	// Token and LastToken carry what ErrForArithHeader's do.
	ErrForArithSeparator
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
	// HoldsProgram says the construct the input ran out inside holds a
	// *program* rather than a parameter or an expression. Only one opener is
	// written the same way for both — `${x}` and `${ cmd;}` — so it is the
	// only construct that needs the fact carried rather than read off Token.
	//
	// One dialect blames the two forms at different lines. Measured
	// 2026-09-12 on bash 5.3.15 over a two-line file:
	//
	//	echo ${ echo hi   line 3 — the line after the input's last
	//	echo ${x          line 1 — the line the `${` is on
	//
	// which is the same split it draws between `$( )` and `$(( ))`, and the
	// reason ParseFailureLine already keys the convention on the opener. The
	// command form is a fifth member of the set that holds a program, and
	// the brace is what hides it (#1425).
	HoldsProgram bool
	// BraceNameStop is the token that stood where an unterminated `${…}` in
	// the *parameter* form could read no further — `newline` for a newline,
	// and the character itself for a space or a tab, spelled the way
	// [Kind.String] spells a token. Empty where the input simply ended, where
	// an operator had already been read and the rest of the braces is a word,
	// or where the braces held a program.
	//
	// Two dialects answer an unterminated `${x` differently from an
	// unterminated `${x:-a}`, and neither difference can be read off the
	// opener: both are `${`. Measured 2026-09-12 over a two-line file,
	// `-n`, `env -i` with a scratch HOME:
	//
	//	echo ${x        ksh93 ``syntax error at line 1: `newline' unexpected``
	//	echo ${x        dash  `2: Syntax error: Missing '}'` — a line early
	//	echo ${x:-a     ksh93 ``syntax error at line 1: `{' unmatched``
	//	echo ${x:-a     dash  `3: Syntax error: Missing '}'`
	//	echo ${x        ksh93 ``syntax error at line 1: `' unexpected`` — the
	//	                space, for a name a space stopped
	//
	// A `${` whose name never began takes it too: `echo ${` and a newline is
	// dash's line-early answer as well, so the stop is about what stood
	// where the expansion stopped rather than about a name being there.
	BraceNameStop string
	// BraceNameStopFollowsAPrefix says that stop came after a `#`, `##` or
	// `!` written in front of the name, or after an `@` the expansion was
	// still reading an operator letter for, rather than after a bare
	// parameter name.
	//
	// The two dialects that read BraceNameStop want different sets, which is
	// why the fact is carried apart from the stop rather than folded into it.
	// Measured beside the rows above: `echo ${#x` is dash's *ordinary* line
	// — no line early — where `echo ${#` is a line early, because a bare `#`
	// is the parameter and a `#` in front of a name is the length operator.
	// ksh93 says ``newline' unexpected`` for both.
	BraceNameStopFollowsAPrefix bool
	// FlagGroupWordTail is the rest of the *word* a refused expansion flag
	// group stands in: the source from the character after the group's
	// closing `)` to the end of the word, the expansion's own `}` and any
	// text written after it included.
	//
	// One dialect quotes that text back instead of the `(` it refused.
	// Measured on ksh93u+ 2012-08-01, 2026-09-12, `env -i` with a scratch
	// HOME, `-c`:
	//
	//	echo ${(U)x}                  x}                 the brace as well
	//	echo ${(U)x} after            x}                 and no further
	//	echo a${(U)x}b c              x}b                through the literal
	//	echo ${(@f)"$(printf ab)"}    "$(printf ab)"}    quotes and all
	//
	// Row two is the discriminating one: to the end of the *line* would have
	// carried ` after` with it, and the expansion alone would have carried
	// nothing in rows three and four.
	//
	// It is the flag group alone and not every `${…}` that dialect refuses:
	// `echo ${~x} after`, `${=x}`, `${^x}` and `${+x}` each name the one
	// character, measured in the same run.
	//
	// Two neighbors are measured and *not* modeled. Quotes are dropped from
	// the tail — `${(U)"x"}` gives `x}` and `"[${(U)x}]"` gives `x}]` —
	// which is what flagGroupTail does, but only where the tail holds no
	// expansion: with one in it the shell keeps the quotes it was written
	// with and then writes a second `"` where the word's closing quote
	// stood, so `"${(U)x}$w"` gives `x}$w""` where this writes `x}$w"`. And
	// a flag group holding a nested `(` — `${(l(3))x}` — goes back to
	// naming the `(`, which is where the `)` search stops.
	//
	// Empty where the word could not be recovered, for the reasons
	// ParamExpr.FlagsErrTail is: the report then names the `(` as it always
	// did.
	FlagGroupWordTail string
	// EofLine is the line the input actually ran out on, in the lexer's
	// own count — the same point EndLine names in the next-line
	// convention. Two dialects report this one for an unmatched quote.
	EofLine int
}

func (e *Error) Error() string { return e.Pos.String() + ": " + e.Msg }
