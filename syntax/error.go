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
