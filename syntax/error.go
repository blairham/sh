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
	// EndLine is the line *after* the input's last, which is where one shell
	// considers the end of input to be: `eval "if"` is line 2 there and line
	// 1 in the other three. Input that already ends in a newline puts Pos on
	// that line anyway, so the two agree there and differ only when the text
	// stops mid-line.
	EndLine int
}

func (e *Error) Error() string { return e.Pos.String() + ": " + e.Msg }
