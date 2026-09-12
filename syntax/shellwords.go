// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// ShellSplit says how ShellWords reads the text it is given.
type ShellSplit struct {
	// Comments is what a `#` where a word could begin means. See CommentMode.
	//
	// The zero value is the shell's ordinary rule, CommentsSkipped, because
	// this shares the lexer's type and the lexer's default has to be the
	// ordinary one. A caller wanting no comment rule at all — which is what
	// the flag this serves does with neither of its comment letters written
	// — has to say CommentsOrdinaryText.
	Comments CommentMode

	// NewlineIsBlank makes an unquoted newline ordinary whitespace. Without
	// it a newline is a word of its own, spelled `;` — which is what the
	// lexer already calls it, a command terminator, written the way the
	// terminator is written rather than the way this text happened to spell
	// it.
	NewlineIsBlank bool
}

// ShellWords reads src as a command line and returns the words it is made of,
// each one exactly as it was written — quotes, backslashes and all.
//
// It is [Lexer.Next] and nothing else. The question "how would the shell
// split this" already has an answer in this package, and the answer is the
// lexer: a second scanner beside it would agree on `a b` and then disagree
// about `a"b c"d`, `$(f x)`, `a#b`, `2>&1` and every other place where a word
// boundary is not a blank. So this walks the token stream and keeps the
// source each token covers.
//
// **What it reads is a value, not a program.** That difference is real and it
// is why this is a function rather than a parse:
//
//   - Nothing is expanded, run or resolved. `$(f x)` is three tokens' worth
//     of text kept as the one word it was written as, because the caller
//     wants the word and not what it would produce.
//   - No here-document body is ever read. A `<<` here is an operator with a
//     word behind it; the lines that follow are more of the same text and
//     not a body, which is measured on the shell whose flag this serves —
//     `a <<EOF`, a body line and the delimiter come back as six separate
//     words.
//   - Input that ends inside a quote or a substitution is not an error. The
//     rest of the text is the last word, unterminated as written.
//
// The token stream this walks is not a program either, and must not be handed
// to a parser: under CommentsKept a comment arrives as a word.
//
// **Argument position is the parser's to know, so the parser is what reads
// this.** A `(` that *starts* a token belongs to the word where an argument
// may stand — `a (b c) d` is three words and not six — and command position
// is the whole of the difference: `(b c) d` and `a; (b c) d` split the
// parenthesis off. The lexer has the flag, `inArgument`, and it is set from
// `parseSimple`, because deciding it needs to know that `a="x"` is an
// assignment and that `then` is a keyword — neither of which a token stream
// says.
//
// So this drives a Parser and keeps the tokens it caused, rather than running
// the lexer alone and answering the question a second time. A state machine
// here would be that second answer: it gets `a="x" (b c)` and `if a; then (b
// c)` right only by restating the assignment and keyword rules, and then
// still misses `repeat 2 (b c)`, `coproc (b c)` and `foreach x (a b)`, which
// are three more of the parser's own (measured on the shell with the flag —
// all three split the parenthesis off, and `(a) (b)` does not). That is the
// shape #1331 and #1397 were both undoing (#1514).
//
// The **tree is thrown away and so is any error**, which is what keeps this a
// split of a value rather than a parse of a program: input that ends inside a
// quote or a substitution is not a failure here, and text that is not a
// program at all still has to come back as the words it was written as. Where
// the parser stops early, the rest is lexed plainly and appended — there is
// no position to know past the point the grammar lost track, and the shell
// with the flag is no more definite there.
//
// `noHeredocBodies` is the one thing the parser must be stopped from doing.
// It would otherwise queue a `<<` and let the lexer claim the lines after it
// as a body, which is input a splitter may not consume; see that field.
func ShellWords(src string, d Dialect, opt ShellSplit) []string {
	var toks []Token
	lex := NewLexer(src, d)
	lex.comments = opt.Comments
	lex.noHeredocBodies = true
	lex.recorded = &toks
	p := newParserOn(lex, d)
	p.Parse()
	// The parser holds one token of lookahead, so what it read is exactly
	// what was recorded — including the token it stopped on. Anything after
	// that point is text the grammar never reached.
	rest := lex.off
	lex.recorded = nil

	if rest < len(src) {
		tail := NewLexer(src[rest:], d)
		tail.comments = opt.Comments
		tail.noHeredocBodies = true
		for {
			t := tail.Next()
			if t.Kind == TokEOF || t.End.Offset <= t.Pos.Offset && t.Kind != TokNewline {
				break
			}
			t.Pos.Offset += int32(rest)
			t.End.Offset += int32(rest)
			toks = append(toks, t)
		}
	}

	var out []string
	lastWasOneDigitFd := false
	for _, t := range toks {
		if t.Kind == TokEOF {
			continue
		}
		if t.Kind == TokNewline {
			lastWasOneDigitFd = false
			if opt.NewlineIsBlank {
				continue
			}
			out = append(out, ";")
			continue
		}
		// The source the token covers rather than t.Text, which is the same
		// string for a word and for every operator and is *not* for an
		// arithmetic command: TokArithCmd carries the expression with its
		// parentheses already stripped, so `((1+2))` would come back as
		// `1+2` — a word nobody wrote, and one that means something else.
		w := src[t.Pos.Offset:t.End.Offset]

		// A one-digit file descriptor is spelled back joined to the operator
		// it was written against. This lexer keeps them apart because the
		// grammar does — IO_NUMBER is its own token, and `echo 1>b` means
		// something `echo 1 >b` does not — but they were one word on the
		// line, and a splitter hands words back.
		//
		// One digit and no more, which is measured rather than assumed. On
		// zsh 5.9.2, the one shell whose grammar has the flag this serves:
		//
		//	a 2>&1      a  2>&  1     joined
		//	a 2<&1      a  2<&  1     whichever direction
		//	a 2>>f      a  2>>  f     and whichever operator
		//	a 22>&1     a  22  >&  1  two digits are not
		//	a {v}> f    a  {v}  >  f  and neither is a named descriptor
		//	a 2 > f     a  2   >   f  nor a digit written apart from it
		//
		// Two of those rows are already answered before this: an IO number is
		// only an IO number when it is *adjacent*, so a spaced digit never
		// reaches here at all, and a dialect that does not take a multi-digit
		// descriptor reads `22` as an ordinary word rather than as a number.
		// So the width test is carrying the named descriptor, `{v}>`, and a
		// multi-digit one wherever a dialect has them — which is why it is a
		// length and not a `!= "{"`.
		if n := len(out); n > 0 && t.Kind.IsRedirect() && lastWasOneDigitFd {
			out[n-1] += w
			lastWasOneDigitFd = false
			continue
		}
		lastWasOneDigitFd = t.Kind == TokIONumber && len(w) == 1

		out = append(out, w)
	}
	return out
}
