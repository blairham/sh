// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"fmt"
	"strings"
)

// Lexer turns shell source into tokens.
//
// It is written from docs/spec/grammar/tokenization.md. Two properties are
// requirements rather than niceties, because a shell re-parses the current
// line on every keypress to highlight it:
//
//   - It never panics, on any input.
//   - Incomplete input is distinguishable from invalid input, because a half
//     typed line is the normal case at a prompt and is not an error yet.
type Lexer struct {
	src     string
	dialect Dialect

	off  int
	line int
	col  int

	err        error
	incomplete bool
	// remarks are what the parser has to say about input it accepted anyway.
	// See Remark.
	remarks []Remark

	// openWord is what the input was inside when it ran out — a quote, an
	// expansion, a here-document. The first one wins: a quote inside a
	// substitution ends the input once, and it is the quote that is waiting.
	openWord string
	// innerOpen is what a program between parentheses was itself still
	// inside when the input ran out there — the here-document or quote a
	// `$(` holds — which openWord cannot say, because the read of that
	// program is a lexer of its own and its running out never reaches this
	// one. Empty where nothing was open inside. See Lexer.OpenInnermost.
	innerOpen string
	// lastInner is what the most recent failed read of a program between
	// parentheses was inside when it ran out, waiting for scanParens to run
	// out of the same input.
	lastInner string

	// wordStart is where the word being read began, kept for the diagnostic
	// that quotes it back.
	//
	// One dialect names the whole word a construct ran out inside rather
	// than the construct — `v=$(echo hi` and not `$(echo hi` — and the word
	// is not something the text can be searched backwards for. Quoting is
	// what makes it so: `echo "a b"$(echo hi` is one word and names all of
	// it, `echo a\ b$(echo hi` equally, and a scan back to the previous
	// blank would cut both in half. The scanner is the only thing that knows
	// where a word started, so it says (#1022).
	//
	// Zero outside a word, which is a real state rather than an unset one: a
	// here-document body that runs out is inside no word, and that dialect
	// quotes nothing there.
	wordStart Pos

	// continuations is where the line continuations of the arithmetic
	// expression being read stand, while one is being read, and nil
	// otherwise. See Lexer.collectContinuations.
	continuations *[]int

	// inRegex is set while the token being read is the operand of `=~`. Its
	// parentheses belong to the regular expression rather than to the shell,
	// and in two of the three dialects that have `[[ ]]` so does a bare `|`.
	inRegex bool

	// inCondition is set while the parser is inside `[[ ]]`. One dialect
	// reads pattern groups there and nowhere else, and the lexer is what has
	// to know: whether `(` ends the word is decided before any parser sees a
	// token. It also suspends the arithmetic command, whose `((` is two
	// grouping parentheses in a condition.
	inCondition bool

	// inPattern is set while the token being read is the operand of a
	// pattern operator — `==`, `=` or `!=`. Its `(` opens an alternation
	// group rather than anything of the shell's, in the dialect that has
	// bare groups, and that has to be known before the operator table sees
	// the character: `(` is an operator, so a token beginning with one never
	// reaches the word scanner. Exactly the shape inRegex has, and for
	// exactly the same reason.
	inPattern bool

	// inCondOperandGroup is set while the token being read is the **third
	// word of a condition** — the right operand of a binary operator, or the
	// second argument of a named one — in a dialect where a `(` there opens
	// a group belonging to the word.
	//
	// Separate from inPattern, which says the operand is a *pattern* and is
	// set for `==`, `=` and `!=` alone. This one is set whatever the
	// operator is, because the reading is positional rather than the
	// operator's: `[[ 9 -gt ( 1 + 2 ) ]]` is true on zsh 5.9.2 and the group
	// is handed to the arithmetic evaluator, not to the matcher.
	//
	// The position is the whole of the rule and is measured, 2026-09-15,
	// each probe in a script file of its own:
	//
	//	[[ 9 -gt ( 1 ) ]]        the group — 0, and `( 1 + 2 )` is 3
	//	[[ -pfx 1 ( a ) ]]       the group — `unknown condition: -pfx`, so it parsed
	//	[[ -n ( a ) ]]           parse error near `(` — the *second* word
	//	[[ -pfx ( a ) ]]         parse error near `(` — the second word again
	//	[[ -pfx 1 2 ( a ) ]]     parse error near `(` — the *fourth*
	//	[[ ( 1 -gt 0 ) ]]        the condition grouping, which is the first word
	//
	// So a `(` is a condition's grouping paren at the first word, is refused
	// at the second and beyond the third, and belongs to the word at the
	// third. The `!` and the two connectives start a condition over — `[[ !
	// -pfx 1 ( a ) ]]` and `[[ x == y || -pfx 1 ( a ) ]]` both parse — so the
	// count is per primary rather than per `[[ ]]`.
	inCondOperandGroup bool

	// inArgument is set while the token being read stands where an
	// *argument* may, rather than where a command may begin. One dialect
	// reads a `(` there as part of the word — a pattern with a list of glob
	// qualifiers — and command position is the whole of the difference:
	// `( x )` written first is a subshell and written after a word is one
	// argument. The lexer cannot tell which on its own, because `(` is in
	// the operator table and a token beginning with one never reaches the
	// word scanner, so the parser sets this before the token is read.
	// Exactly the shape inPattern has, for exactly the same reason.
	inArgument bool

	// inDeclarationOperand is set while the token being read stands where a
	// *declaration utility's* operand does — behind `typeset`, `local`,
	// `export` or `readonly` written the way the dialect requires, and
	// between the parentheses of the array literal such an operand opens.
	//
	// It is what lets [Lexer.arrayLiteralCouldStandHere] say yes in argument
	// position without saying yes to every argument. `local a=(x y)` reaches
	// the parser as the word `a=` and then a parenthesis, which is exactly
	// what an array literal produces; `print -r -- x=(a|b)c` is one word with
	// a group in it, and nothing in the *word* tells the two apart. The
	// command's name does, and only the parser can see it — the same shape
	// inArgument and inArrayLiteral have, set for the same reason (#3087).
	inDeclarationOperand bool

	// inArrayLiteral is set while the token being read stands where an
	// *element* of a compound array literal does — between the parentheses
	// of `a=( … )` — and it exists for one question: an element that opens
	// with `[` carries its subscript through the blanks inside it, so
	// `a=( [two words]=2 )` is one element keyed `two words` rather than the
	// two fields a blank would otherwise make of it.
	//
	// The parser sets it for the same reason it sets inArgument beside it:
	// the position is the parser's to know and the word boundary is the
	// lexer's, and the two meet nowhere else. See
	// [Dialect.SubscriptSpansSeparators], where the measurement is, and
	// opensArrayElementSubscript for what the flag decides.
	inArrayLiteral bool

	// inCaseSubject is set while the token being read is the word a `case`
	// matches its arms against. It exists for one question the flags beside
	// it cannot answer: whether a `(` at the front of that word opens
	// something of the shell's or is text.
	//
	// It is text in one dialect, because the subject stands where an
	// argument does there. Measured 2026-09-15 on zsh 5.9.2 and on zsh 5.9,
	// each probe in a script file of its own: `case (x) in "(x)") …` runs
	// that arm and `case (x) in x) …` runs none, so the parentheses reach
	// the matcher as two characters of the subject rather than grouping
	// anything. The other six columns — bash 5.3, bash as `sh`, bash 3.2,
	// ksh93u+, dash and BusyBox ash — all refuse `case (x) in` outright,
	// which is why the reading is the dialect's and the flag is read only
	// where [Dialect.GlobQualifiers] already says a leading `(` may be a
	// word's.
	//
	// Separate from noAssignment, which is true here too: that flag answers
	// what an `=` means and this one answers what a `(` does, and a shell
	// could have either without the other.
	inCaseSubject bool

	// noAssignment is set while the token being read stands where no
	// *assignment* may be written, in the two positions the flags above do
	// not already cover: a `case` subject and a redirection's target. Both
	// are read where a command would otherwise begin, so the zero value —
	// an assignment may stand here — is right everywhere else.
	//
	// One dialect asks: `=(cmd)` opens its temp-file substitution at the
	// front of a word and at the front of an assignment's *value*, and
	// nowhere else. Measured 2026-09-11 on zsh 5.9.2, where `a==(echo hi)`
	// assigns a path while `case a==(echo hi) in *)` matches with no
	// command run and no file made, and `> a==(echo hi) cat` is `missing
	// end of string`. See startsProcSubstFile.
	//
	// Both examples carry **two** `=`, and that is the whole reason the flag
	// is needed: `case a=(b) in` and `> a=(b) cat` are decided by the array
	// literal guard in opensPatternGroup long before this, so asking them
	// pins nothing. Found by mutation — a test written with one `=` stayed
	// green with this flag removed.
	noAssignment bool

	// inRedirectTarget is set while the token being read is a redirection's
	// *target*, and it exists for one question the two flags above cannot
	// answer between them: a subscript in that position runs to its matching
	// `]` in one dialect. noAssignment is already true there and is true for
	// a `case` subject too, and the subject is measured **not** to span — so
	// reusing it would have spanned in a position no shell spans in. See
	// [Dialect.SubscriptSpansSeparatorsInRedirect], where the rows are.
	//
	// The flag says only *that* the token is a target. Whether the target
	// stands where a command may begin is inArgument's to say, and the two
	// are read together: a redirection written in front of a command word,
	// or after a compound one, has inArgument false, while a redirection
	// following a simple command's word has it true — which is exactly the
	// split ksh93 makes.
	inRedirectTarget bool

	// inHeredocDelimiter is set while the token being read is a
	// here-document's *delimiter*, which is the one word position where no
	// expansion happens at all. A delimiter is subject to quote removal and
	// to nothing else, so `<<$d` waits for a line reading `$d` — measured
	// the same in bash 5.3.15, ksh93u+, zsh 5.9.2 and dash — and `${d}`,
	// `$(cmd)`, `$((1+1))` and a backquoted run are all text in one.
	//
	// The construct is still *scanned*, because the delimiter ends where it
	// ends: `cat <<$(echo X)` is one word in bash and zsh, and a scanner
	// that read the `(` as an ordinary character would stop the word at it
	// and refuse the line. What changes is only what is kept — the source
	// the construct occupied, rather than anything it would produce. See
	// scanDelimiterSubstitution.
	//
	// Quoting is not expansion and stays on: `<<$'a'` is delimited by `a`
	// and `<<E$"x"F` by `ExF`, because `$'` and `$"` open quoted runs. So
	// the flag is read below those two and above everything else a `$` or a
	// backquote starts.
	//
	// Without it the delimiter was the word's spans *joined*, which is the
	// text with each construct's own delimiters already stripped: `$d`
	// arrived as `d`, the delimiter line never matched, and the rest of the
	// file was swallowed as the body (#2745).
	inHeredocDelimiter bool

	// inRawBody is set while the text being read is a *body* rather than a
	// word: a here-document's, or a value being read again by the flag that
	// re-evaluates one. Both go through heredocSpans, which marks every span
	// double-quoted so that nothing it produces is field-split — and that
	// mark is what an unmatched-construct diagnostic would otherwise blame,
	// reporting `unmatched "` about a text with no quote character in it.
	//
	// A body's quoting is a fact about splitting and not about a character
	// somebody wrote, which is why this is a second bool rather than a third
	// Quoting value: every other reader of the mark wants it to keep saying
	// double-quoted.
	inRawBody bool

	// inWordTail is set while the text being read is the tail of one word
	// that has already been read once — a [ParamExpr.RawTail], divided again
	// because a run in POSIX mode reads the closing brace of a `${ … }`
	// somewhere else. See [RereadWordTail].
	//
	// What it says is that the input running out is the *word* running out.
	// The word was cut by a parse that read a closing quote wherever one was
	// written, so a quote left open at the end of this text is the one that
	// closed the word — the division being redone is inside it, not around
	// it. Refusing the tail for an unmatched quote would be refusing the word
	// the parse already accepted.
	inWordTail bool

	// recorded, when non-nil, collects every token Next hands back. It is
	// how [ShellWords] gets the *parser's* reading of a text without a tree:
	// the parser is the only thing that knows a token stands where an
	// argument may, and the token it produced there is already the answer.
	recorded *[]Token

	// noHeredocBodies stops a `<<` from claiming the lines after it.
	//
	// One caller, and it is the same one: [ShellWords] reads a **value**
	// rather than a program, so a `<<` in it is an operator with a word
	// behind it and the lines that follow are more of the same text. That is
	// measured on the shell whose flag it serves — `a <<EOF`, a body line and
	// the delimiter come back as six separate words — and it is the one place
	// where driving the parser would otherwise read input a splitter must
	// leave alone.
	noHeredocBodies bool

	// inOperand is set while the tokens being read are an expansion's
	// operand — the pattern of `${v#pat}`, the word of `${v:-word}`, the
	// replacement of `${v/pat/repl}` — rather than a command.
	//
	// It suspends the arithmetic command for the reason inCondition and
	// inCaseArm do: no command may begin there, so `((` is not one, and
	// scanArithCommand keeps the *expression* as its token text rather than
	// the source it read. That is not a lossless reading to fall back on —
	// it strips the two opening parens and up to two closing ones — so an
	// operand read as one comes back shorter than it was written. Measured
	// 2026-09-07, and the panel is unanimous, which is what makes this the
	// core's answer rather than a dialect's:
	//
	//	u=; x=${u:-((a))}; echo "[$x]"      [((a))] in all six
	//
	// where this lexer answered `[a]` — the arithmetic expression `a`, read
	// as a variable name and found unset. `"${u:-((a))}"` was unaffected and
	// is not the narrower case: a double-quoted *word* operand is read by
	// quotedWordFrom, which scans double-quoted content and never a command,
	// so only the unquoted routes reach here. Every pattern operand is one
	// of those whatever it is written inside, which is how the same fault
	// reached `${v#((#s)a)}` as `bad pattern: #s)a` (#1408).
	//
	// It suspends the **comment** for exactly the same reason, and that is
	// the same fault a second time: a comment is a thing a command line has,
	// and an operand is not one. Measured 2026-09-11 on zsh 5.9.2, bash
	// 5.3.15, bash 3.2.57 and dash, from a script file under `env -i`:
	//
	//	u=; printf '[%s]' "${u:-a#b}"    [a#b] in all four
	//	u=; printf '[%s]' "${u:-#b}"     [#b]  in all four
	//
	// so a `#` where a word could begin inside an operand is a character of
	// that word and never opens a comment. This lexer skipped one — the
	// operand's text from the `#` to its end — and Parser.wordFrom then put
	// the skipped run back as a *raw* literal span, which is the silent half:
	// the text reappeared, and the quoting inside it did not. So
	// `${v/(#b)\X/Q}` reached the matcher as the five characters `(#b)\X`
	// with a live backslash where the pattern is `(#b)X`, and every glob flag
	// group in a substitution — `(#b)`, `(#m)`, `(#i)` — stopped matching as
	// soon as anything behind it was quoted or escaped (#2074).
	//
	// A `$( )`, `<( )` or `${ ;}` *inside* an operand is unaffected, because
	// its body is a command line again: those are raw scans that ask
	// commentsExist rather than coming through here, and the panel is
	// unanimous the other way on them —
	// `printf '[%s]' "${u:-$(echo hi # there)}"` is a parse failure in all
	// four, the `#` having swallowed the closing paren.
	inOperand bool

	// inCaseParenList is set while the token being read stands inside a
	// `case` arm's parenthesized pattern list — between the paren the arm
	// carries and the one that closes it. One dialect reads a newline there
	// as text; see newlineIsText.
	inCaseParenList bool

	// inCaseArm is set while the token that *begins* a `case` arm is read.
	//
	// Two things are different there, and both are about a `(` the arm may
	// carry. `((` is not an arithmetic command — `case x in ((a|b))` is an
	// arm's paren in front of a group in the dialect that takes it, where
	// this lexer read the whole of `((a|b))` as one expression and the
	// parser then complained about an arithmetic command that the script
	// never wrote. And a leading `(` belongs to the *pattern* rather than
	// being the arm's own paren exactly when it opens a glob flag: `(#i)a)`
	// is the pattern `(#i)a` with the arm's `)` behind it, while `((#i)a)` is
	// the arm's paren in front of that same pattern. Which of the two it is
	// cannot be seen from the character alone, and `case x in (a)` — one
	// paren, an ordinary pattern — is the row that says so.
	//
	// Exactly the shape inCondition has: a position only the parser knows
	// about, told to the lexer before the token is read.
	inCaseArm bool

	// pending holds here-documents whose bodies have not been read yet.
	//
	// A body starts after the *next newline*, not after the operator — the
	// rest of the line is ordinary input — so the parser registers the
	// redirection when it sees the delimiter and the lexer fills the body in
	// when it reaches the newline. Several on one line are collected in
	// operator order.
	pending       []*Redirect
	pendingQuoted []bool

	// comments says what an unquoted `#` where a word could begin means.
	// The zero value is the shell's ordinary rule and is what every parse
	// uses; the other two exist for ShellWords. See CommentMode.
	comments CommentMode

	// heredocEnd is where the last here-document body read here finished:
	// the line that closed it, which is the delimiter's own line, or the
	// last line there was where the input ended before the delimiter did.
	//
	// It exists because a here-document's body and its delimiter are
	// physical lines of the *command* that owns them, and nothing else in a
	// token says so. The newline token that triggers the read is positioned
	// where the command's first line ended, and the body is consumed behind
	// it — so a caller counting the lines a command occupied sees one line
	// where a script shows three. `set -v` is the caller that minds: it
	// writes the input back as it is read, and it echoed the delimiter after
	// running the command rather than with it.
	heredocEnd Pos

	// subscriptDepth counts the brackets open in the subscript the word
	// being read opened at command position, and is zero everywhere else.
	// While it is positive a separator is a character of the subscript
	// rather than the end of the word; see
	// [Dialect.SubscriptSpansSeparators] and endsWord.
	//
	// Only literal text written unquoted counts, the way the brace counter
	// in scanWord does: a `]` inside quotes, behind a backslash or produced
	// by an expansion closes nothing, which is measured.
	subscriptDepth int

	// pidBraces counts the braces still open in a `{ … }` run the word being
	// read opened immediately after `$$`, and is zero everywhere else. While
	// it is positive a blank, a newline and every operator alike are
	// characters of the run rather than the end of the word — the same job
	// subscriptDepth does one construct along. See
	// [Dialect.PidBraceGroupIsText] and endsWord.
	pidBraces int

	// pidBraceOpen is where that run's `{` stands, kept for the refusal an
	// unmatched one raises at the end of the input.
	pidBraceOpen Pos

	// subscriptCloses caches whether the subscript now open has a matching
	// `]` ahead of it, asked at most once per word: 0 not asked, 1 yes,
	// -1 no. Reset with subscriptDepth at the start of every word.
	subscriptCloses int8

	// assumeSubscriptCloses makes the answer yes without asking. It is set
	// on the *probe* lexer subscriptHasMatchingClose builds, which is the
	// same scanner reading the same bytes to find out where the subscript
	// ends — so this is what stops that question asking itself.
	assumeSubscriptCloses bool

	// inProgramParens is set on the lexer that reads the program inside a
	// pair of parentheses holding one — `$( )`, `<( )`, `>( )` — from
	// [Lexer.parseToClose]. It is what lets a here-document's body end on a
	// line that carries the closing parenthesis after the delimiter, in the
	// dialects that read a body from between the parentheses. See
	// [Lexer.delimiterClosesParens].
	inProgramParens bool
}

// queueHeredoc registers a redirection whose body is still to be read. The
// parser calls it; the lexer fills r.Heredoc at the next newline.
//
// quoted comes from the parser because it is a property of how the delimiter
// was *written*, which only the raw token still knows: `\EOF` produces the
// same spans as `EOF`, and both make the body literal.
func (l *Lexer) queueHeredoc(r *Redirect, quoted bool) {
	if l.noHeredocBodies {
		return
	}
	l.pending = append(l.pending, r)
	l.pendingQuoted = append(l.pendingQuoted, quoted)
}

// NewLexer returns a Lexer over src.
func NewLexer(src string, d Dialect) *Lexer {
	return &Lexer{src: src, dialect: d, comments: d.Comments, line: 1, col: 1}
}

// Err reports why lexing stopped early, or nil.
func (l *Lexer) Err() error { return l.err }

// forgetErr drops an error the lexer recorded, for the one caller that reads
// on past one: Parser.giveUpOnTheArray, where the input running out between an
// array literal's parentheses ends the line rather than the file. Without it
// the parser adopts the lexer's copy again on its next token — see
// Parser.next — and the refusal becomes the file's after all.
//
// Only ever called at the end of the input, which is what makes it safe: there
// is no more text for a lexer in a state it cannot continue from to be asked
// about.
func (l *Lexer) forgetErr() { l.err = nil }

// Incomplete reports whether the input ended in the middle of something that
// could still be finished — an unclosed quote, a trailing line continuation.
// A prompt should ask for another line; a script should report an error.
func (l *Lexer) Incomplete() bool { return l.incomplete }

// Open is what the lexer was inside when the input ran out, spelled as it is
// written: `'`, `"`, `${`, “ ` “, `<<`. Empty when the input was whole, or
// when what ran out was the parser's rather than the lexer's.
//
// One thing rather than a stack. What the lexer is inside nests through
// recursion — a substitution runs a parser of its own — so the innermost is
// the one this level knows about, and the outer ones are the callers'.
func (l *Lexer) Open() string { return l.openWord }

// OpenInnermost is Open for a reader that needs the **innermost** thing still
// open rather than the outermost: the here-document inside a `$(`, where Open
// says `$(`.
//
// Measured 2026-09-16 on bash 5.3.20 from a script with history expansion on:
// `echo $(cat <<EOF` / `echo !!` / `EOF` / `)` writes `echo !!` — the body
// line is a here-document's and is not expanded, inside a substitution
// exactly as outside one — where reading only the outer construct expanded
// it. See driver's history gate, the one caller.
func (l *Lexer) OpenInnermost() string {
	if l.innerOpen != "" {
		return l.innerOpen
	}
	return l.openWord
}

// ranOut records that the input ended inside something, and what.
//
// The first call wins. Once the input has ended, everything after it is a
// consequence rather than another thing left open.
func (l *Lexer) ranOut(word string) {
	if !l.incomplete {
		l.openWord = word
	}
	l.incomplete = true
}

func (l *Lexer) pos() Pos {
	return Pos{Offset: int32(l.off), Line: int32(l.line), Col: int32(l.col)}
}

// shiftLines moves the line counter on without moving through any input.
//
// One caller: an alias body whose newlines the dialect counts as lines of the
// program. The text substituted for the alias word is not in this source at
// all, so nothing here can advance over it, and everything read afterwards
// still has to be numbered as though it had been.
func (l *Lexer) shiftLines(n int) { l.line += n }

// skipOver moves the lexer n bytes further into the input, counting the lines
// and columns it passes over.
//
// One caller, and it is the one seam in the language where two lexers read the
// same text: an alias body whose last construct was still open when the body
// ended is continued by a lexer over the body's tail joined to the rest of
// this input, because that is the only arrangement in which a quote can span
// the two. See Parser.carryOpenWord. This is how much of the input that
// reading consumed.
func (l *Lexer) skipOver(n int) {
	for range n {
		if l.eof() {
			return
		}
		l.advance()
	}
}

// posAt is where an offset of this input falls, counted from the start.
//
// The lexer knows its own line and column as it goes and has no way back to a
// position it has already passed, which is all that is ever wanted — except on
// the one error path that is handed an offset by somebody else. See
// adoptOpenConstruct. Linear in the offset, and on a path that has already
// decided the parse is over.
func (l *Lexer) posAt(off int) Pos {
	off = min(max(off, 0), len(l.src))
	line, col := 1, 1
	for i := range off {
		if l.src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return Pos{Offset: int32(off), Line: int32(line), Col: int32(col)}
}

// adoptOpenConstruct takes the failure of the joined reading described on
// skipOver as this lexer's own, and it is the one report whose opener is not
// in this input at all: an alias body is text the program never contained.
//
// So the opener is named where the alias word stood, which is what the panel
// does — `alias a='echo "x'` used on line 4 of a five-line file is blamed on
// line 4 by bash 5.3, bash 3.2, bash as `sh` and ksh93, and at the end of the
// input by zsh and dash. Both of those are already on the error, and which
// one a dialect prints is [interp.Diagnostics]' to decide; what this fills in
// is the pair, with the end taken from where this input really ran out.
//
// split is how long the borrowed text was, so an opener past it is one the
// input does hold and is named where it really is.
func (l *Lexer) adoptOpenConstruct(join *Lexer, split int, word Pos) {
	l.ranOut(join.Open())
	var se *Error
	if !errors.As(join.Err(), &se) {
		if l.err == nil {
			l.err = join.Err()
		}
		return
	}
	e := *se
	e.Pos = word
	if int(se.Pos.Offset) >= split {
		e.Pos = l.posAt(len(l.src) - (len(join.src) - int(se.Pos.Offset)))
	}
	e.EofLine = l.line
	e.EndLine = l.line
	if len(l.src) > 0 && l.src[len(l.src)-1] != '\n' {
		// The text stopped mid-line, so the end of it is the line after — the
		// same convention failUnmatched uses.
		e.EndLine++
	}
	if l.err == nil {
		l.err = &e
	}
}

func (l *Lexer) eof() bool { return l.off >= len(l.src) }

func (l *Lexer) peek() byte {
	if l.eof() {
		return 0
	}
	return l.src[l.off]
}

func (l *Lexer) peekAt(n int) byte {
	if l.off+n >= len(l.src) {
		return 0
	}
	return l.src[l.off+n]
}

func (l *Lexer) advance() byte {
	c := l.src[l.off]
	l.off++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

// failUnmatched records input that ran out inside a quoted or substituted
// region, carrying everything a dialect might name: the opener, its closer,
// the text from the opener to the end of its line, and the line the input
// ran out on in both conventions.
func (l *Lexer) failUnmatched(open Pos, opener, closer, msg string) {
	if l.err != nil && !l.replacesUnmatched() {
		return
	}
	// From the start of the word rather than from the opener, which is what
	// the dialect that quotes this names: the construct is part of a word and
	// the word is what was being read. An assignment prefix is inside it —
	// `v=$(echo hi` is one word — and so is anything quoted, which is why
	// this comes from the scanner rather than from a search backwards
	// through the text (#1022).
	//
	// No comparison between the two positions: the word *contains* the
	// opener, so its start is never past it, and a guard saying so was a
	// mutant nothing could kill.
	from := open.Offset
	if l.wordStart.IsValid() {
		from = l.wordStart.Offset
	}
	near := l.src[from:]
	if i := strings.IndexByte(near, '\n'); i >= 0 {
		near = near[:i]
	}
	after := l.line
	if len(l.src) > 0 && l.src[len(l.src)-1] != '\n' {
		// The text stopped mid-line, so the end of it is the line after —
		// the same convention the parser's unterminated() uses.
		after++
	}
	l.err = &Error{
		Pos: open, Kind: ErrUnmatched, Msg: msg,
		Token: opener, Expected: closer, LastToken: near,
		EndLine: after, EofLine: l.line,
	}
}

// closesQuotesAtEOF reports whether an unterminated `'`, `"` or backquote
// ends here as if its closing mark were written, instead of the parse being
// refused.
//
// One function for all four places that ask — the two quotes, the backquote
// scanner and the backquote *skipper* — because they are one rule and a rule
// spread over four `if`s is a rule three of them can be left out of. It was:
// the route the answer depends on had to be added in exactly these four
// places, and the skipper is the one nothing would have failed without.
//
// It is a route question and not only a dialect one, which is
// [Dialect.CloseQuotesAtEOF]. The route is the empty set unless a front end
// said otherwise, so anything parsing text without saying where it came from
// gets the answer every shell in the panel agrees on.
func (l *Lexer) closesQuotesAtEOF() bool {
	return l.inWordTail || l.dialect.CloseQuotesAtEOF.Has(l.dialect.ProgramRoute)
}

// replacesUnmatched reports whether a construct noticing that the input ran
// out should take the complaint from one *inside* it that noticed first.
//
// Which construct is blamed when they nest is a dialect question, and it is
// answered by the order the reports arrive in rather than by anything having
// to look around: the scanners recurse, so the innermost one to run out
// returns first and the enclosing ones follow it outwards. Keeping the first
// report blames the innermost, and letting each replace the last blames the
// outermost.
//
// Measured 2026-09-07, `-n` over a script file, on constructs nested both
// ways round:
//
//	echo $( echo "hi          bash, dash, ksh93 blame the `"`
//	echo "${x:-"$( echo hi    the same three blame the `$(`
//	echo $(( 1 + `echo 2      and the backquote
//
// so those three name the innermost in every arrangement. zsh names the
// outermost in the same rows, which is why `echo "$( echo hi` is `unmatched
// "` there and `unexpected EOF while looking for matching )` in bash.
//
// Only an unmatched construct may be replaced. Anything else that has
// already failed is a different diagnosis and the first one stands, which is
// what keeps this from turning a real refusal into a report about a
// delimiter that was merely still open when it happened.
func (l *Lexer) replacesUnmatched() bool {
	if !l.dialect.UnmatchedBlamesTheOutermost {
		return false
	}
	var se *Error
	return errors.As(l.err, &se) && se.Kind == ErrUnmatched
}

func (l *Lexer) fail(p Pos, format string, args ...any) {
	if l.err == nil {
		l.err = fmt.Errorf("%s: %s", p, fmt.Sprintf(format, args...))
	}
}

// isBlank reports whether c separates tokens. TokNewline does not: it is a token.
func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// Next returns the next token. At the end of input it returns TokEOF forever.
func (l *Lexer) Next() Token {
	t := l.next()
	if l.recorded != nil {
		*l.recorded = append(*l.recorded, t)
	}
	return t
}

func (l *Lexer) next() Token {
	l.skipBlanksAndComments()
	start := l.pos()

	if l.eof() {
		if len(l.pending) > 0 {
			// Input that ends without a newline still has a here-document
			// waiting, and what it never got is an *empty* body rather than
			// no body: `sh -c 'cat <<X'` runs cat with nothing on its input
			// in every shell. Reading them here is also what records the
			// remark about the delimiter that never arrived, which was
			// otherwise missing for exactly this shape.
			l.readHeredocs()
		}
		return Token{Kind: TokEOF, Pos: start, End: start}
	}

	if l.comments == CommentsKept && l.peek() == '#' {
		return l.scanCommentWord(start)
	}

	if l.peek() == '\n' && !l.newlineIsText() {
		l.advance()
		// The newline is where any pending here-document bodies begin.
		l.readHeredocs()
		return Token{Kind: TokNewline, Pos: start, End: l.pos(), Text: "\n"}
	}

	// An IO number is digits *immediately* followed by a redirection. The
	// adjacency is the whole rule: `echo 1>b` writes an empty file because the
	// 1 is a descriptor, and `echo 1 >b` writes "1" because it is an argument.
	if tok, ok := l.tryIONumber(); ok {
		return tok
	}

	// `{name}` under the same adjacency rule is a descriptor the shell will
	// pick, with the variable receiving its number. The braces survive into
	// the token so the interpreter can tell the two apart.
	if tok, ok := l.tryFdVariable(); ok {
		return tok
	}

	// A pattern's operand owns a group that *starts* it — `[[ $k == (a|b) ]]`
	// — where the dialect reads bare groups. Only the first character needs
	// saying: mid-word the scanner already takes one, which is why
	// `[[ $k == a(b|c) ]]` worked while this did not (#826).
	//
	// Before the arithmetic command below, not after it: a nested group
	// starts `((`, and `[[ $k == ((a|b)|x) ]]` is a pattern rather than the
	// one place in the grammar where two parentheses are one token.
	if (l.inPattern || l.inCondOperandGroup) && l.peek() == '(' && l.opensPatternGroup() {
		return l.scanWord(start)
	}

	// A `(` where an argument may stand belongs to the word in the dialect
	// that reads glob qualifiers. It has to be seen before the operator
	// table, which would otherwise take it — and before the arithmetic
	// command below, because `echo ((1))` is one word there and not an
	// expression: measured, `no matches found: ((1))`.
	if l.dialect.GlobQualifiers && l.leadingParenBelongsToTheWord() && l.peek() == '(' &&
		l.opensPatternGroup() {
		return l.scanWord(start)
	}

	// `((` is an arithmetic command; `( (` is a subshell containing one. The
	// distinction is textual wherever a command may begin: `((echo nested))`
	// is an arithmetic error in bash, ksh and zsh even with no space, so no
	// knowledge of *which* command position this is is needed there.
	//
	// Inside `[[ ]]` no command may begin at all, so there is no arithmetic
	// command to be had and `((` is two grouping parentheses — unanimous in
	// bash 3.2 and 5.3, bash-as-sh, ksh93 and zsh, which all take
	// `[[ ((1 -eq 1)) ]]` and read it exactly as the spaced `[[ ( (1 -eq 1) ) ]]`.
	// The condition is the only context in the grammar that suspends the
	// rule, which is why the flag rather than a command-position test is
	// what asks (#859).
	// A `case` arm suspends it for the same reason a condition does: no
	// command may begin where a pattern belongs, so there is no arithmetic
	// command to be had and `((` is the arm's paren in front of a group.
	// An expansion's operand suspends it for that same reason and is the
	// third such position; see inOperand for what reading one as arithmetic
	// cost (#1408).
	// Measured on zsh 5.9.2: `case x in ((a|b))`, `case x in ((a))` and
	// `case x in ((1))` are all accepted there and were all `parse error
	// near 'arithmetic command'` here (#1161).
	// `((` is ambiguous even where an arithmetic command may stand: it opens
	// one, or it is a subshell whose first command is itself a subshell.
	// `((cmd); cmd)` is the second and the whole panel runs it, so the
	// reading is *tried* here and abandoned when it does not close — see
	// scanArithCommand, which is what says so, and #3052 for the rows. The
	// lexer is put back where it stood and the `(` falls through to the
	// operator table, which takes one character: the next token is read from
	// the second `(`, so a `((` that opens an arithmetic command one paren
	// in still finds it.
	if l.dialect.ArithCommand && !l.inCondition && !l.inCaseArm && !l.inOperand &&
		l.peek() == '(' && l.peekAt(1) == '(' {
		// A line continuation standing *directly* behind the second
		// parenthesis opens nothing in one dialect: the two parentheses and
		// the pair are consumed, no command is produced, and reading starts
		// again from what follows — which is why the refusals that shape
		// draws there are at the leftovers rather than at the continuation.
		// See Dialect.ContinuationEndsTheArithmeticCommandOpener.
		if l.dialect.ContinuationEndsTheArithmeticCommandOpener &&
			l.peekAt(2) == '\\' && l.peekAt(3) == '\n' {
			l.advance() // (
			l.advance() // (
			l.advance() // the backslash
			l.advance() // the newline
			return l.next()
		}
		saved := *l
		if tok, ok := l.scanArithCommand(start); ok {
			return tok
		}
		*l = saved
	}

	// A regular expression's operand owns its parentheses even at the start
	// of it — `[[ x =~ (b) ]]` — and the operator table would otherwise take
	// the `(` before the word scanner ever saw it.
	if l.inRegex && (l.peek() == '(' || l.peek() == ')') {
		return l.scanWord(start)
	}

	// `<(` and `>(` begin a *word* rather than a redirection, so they have to
	// be seen before the operator table takes the `<`. The adjacency is the
	// whole rule, the same way it is for an IO number: `cat <(echo hi)` is a
	// process substitution and `cat < (echo hi)` is a redirection to a
	// subshell, which is a syntax error in every shell that has either.
	if l.startsProcSubst() {
		return l.scanWord(start)
	}

	// `<->` and its bounded spellings begin a *word* rather than a
	// redirection, and like `<(` they have to be seen before the operator
	// table takes the `<`.
	if l.startsNumericRange() {
		return l.scanWord(start)
	}

	if k, ok := l.matchOperator(); ok {
		s := text[k]
		for range s {
			l.advance()
		}
		l.remarkOnOperatorsRunTogether(start, s)
		return Token{Kind: k, Pos: start, End: l.pos(), Text: s}
	}

	return l.scanWord(start)
}

// startsProcSubst reports whether the cursor is on `<(` or `>(` in a dialect
// that has process substitution.
//
// Adjacency is the grammar and not a convention: `cat < (echo hi)` is a
// syntax error in every shell in the panel, the three that have the
// construct included. Specified in docs/spec/grammar/substitutions.md.
func (l *Lexer) startsProcSubst() bool {
	if !l.dialect.ProcessSubstitution {
		return false
	}
	c := l.peek()
	return (c == '<' || c == '>') && l.peekAt(1) == '('
}

// startsProcSubstFile reports whether the cursor is on the `=(` that opens
// the temp-file form of process substitution, in a dialect that has one.
//
// **Where it stands is most of the rule.** `<(` and `>(` open a substitution
// wherever they appear in an unquoted word; `=(` opens one only at the front
// of a word, because an `=` is an ordinary character everywhere else and a
// `(` behind one already means something — an array literal, or a pattern
// group. Measured 2026-09-11 on zsh 5.9.2: `echo =(echo hi)x` appends the
// `x` to the path it writes, and `echo x=(echo hi)`, `echo a=b=(echo hi)`
// and `echo =(echo hi)=(echo hi)` are all `missing end of string`.
//
// The front of an assignment's *value* is the second place and the only one,
// which is what atAssignValue answers. Specified in
// docs/spec/grammar/substitutions.md.
func (l *Lexer) startsProcSubstFile() bool {
	if !l.dialect.ProcessSubstitutionToFile {
		return false
	}
	if l.peek() != '=' || l.peekAt(1) != '(' {
		return false
	}
	return l.off == int(l.wordStart.Offset) || l.atAssignValue()
}

// atAssignValue reports whether the cursor stands where an assignment's value
// begins: the word so far is a name, an optional subscript and an optional
// `+`, ending at the `=` just consumed, and an assignment may be written
// here at all.
//
// Both halves are measured and neither alone is the rule. `a==(echo hi)`
// assigns a path in zsh 5.9.2 and `echo a==(echo hi)` — the same word one
// position later, where it is an argument rather than an assignment — is
// `missing end of string`; so is `> a==(echo hi) cat`, while
// `case a==(echo hi) in *)` matches with nothing run. The name half is what
// keeps `echo a=b=(echo hi)` a refusal: `a=b` is not a name, so the second
// `=` is not a value's front.
func (l *Lexer) atAssignValue() bool {
	if l.inArgument || l.inCondition || l.inOperand || l.inCaseArm ||
		l.inCaseParenList || l.inRawBody || l.noAssignment {
		return false
	}
	head := l.src[l.wordStart.Offset:l.off]
	head, ok := strings.CutSuffix(head, "=")
	if !ok {
		return false
	}
	head = strings.TrimSuffix(head, "+")
	if i := strings.IndexByte(head, '['); i >= 0 {
		if !strings.HasSuffix(head, "]") {
			return false
		}
		head = head[:i]
	}
	return isNameIn(head, l.dialect.DottedName)
}

// arrayLiteralCouldStandHere reports whether an array literal may be written
// at the cursor, which is what decides that a `(` straight after an `=` is
// not a group.
//
// The `=` is what makes the question narrow: an array literal is `name=( … )`
// and nothing else, so the reading is available only where an assignment is.
// A `case` **pattern** is one of the places it is not — no assignment may be
// written there — so the `=` is an ordinary character and the `(` behind it
// is the group it looks like. Measured 2026-09-15 on zsh 5.9.2, each probe
// in a script file of its own, the subject `a=b`:
//
//	case a=b in ((a)=(b)) echo m;; *) echo n;; esac    `m`
//	case a=b in (a=(b))   echo m;; *) echo n;; esac    `m`
//	a=(x y); print -r -- $#a                           2
//
// The last row is the one this must not lose, and it is the case the guard
// was written for. The first two are what it was too wide for: read as an
// array literal, each ended the word at the `(` and the parenthesis was then
// an operator with nowhere to go. A `case` arm that captures on both sides of
// an `=` is a shape a shipped completion function writes, and this is what it
// stopped at (#3040).
//
// **An *argument* is a fourth position**, and it is the one the answer is not
// uniform in. An array literal is what a declaration utility's operand needs —
// `local a=(x y)` reaches the parser as the word `a=` and then a parenthesis —
// and it is exactly what an ordinary argument must not get: `print -r -- // x=(a|b)c` is one word with a group in it in zsh 5.9.2, and
// `print -r -- x=(a)` is a word with a glob qualifier on it. Nothing in the
// *word* tells the two apart; the command's name does, so the parser says
// which position this is through inDeclarationOperand (#3087).
func (l *Lexer) arrayLiteralCouldStandHere() bool {
	if l.inCondition || l.inCaseArm || l.inCaseParenList {
		return false
	}
	return !l.inArgument || l.inDeclarationOperand
}

// startsNumericRange reports whether the cursor is on a numeric range
// pattern — `<->`, `<1-9>`, `<2->`, `<-9>` — in a dialect that has one.
func (l *Lexer) startsNumericRange() bool {
	_, ok := l.numericRangeAt(0)
	return ok
}

// numericRangeAt measures a numeric range starting n bytes ahead of the
// cursor, reporting how many bytes it spans.
//
// The shape is the entire disambiguation and it is exact: `<`, digits, `-`,
// digits, `>`, with either run of digits allowed to be empty. Anything else
// and the `<` is the redirection it is everywhere else — which is why this
// answers a width rather than a bool for the scanner, and a bool for the two
// places that only need to know a redirection is not starting here.
func (l *Lexer) numericRangeAt(n int) (int, bool) {
	if !l.dialect.NumericRangePattern || l.peekAt(n) != '<' {
		return 0, false
	}
	i := n + 1
	for isDigitByte(l.peekAt(i)) {
		i++
	}
	if l.peekAt(i) != '-' {
		return 0, false
	}
	i++
	for isDigitByte(l.peekAt(i)) {
		i++
	}
	if l.peekAt(i) != '>' {
		return 0, false
	}
	return i + 1 - n, true
}

// isDigitByte is the ASCII digit test the range shape is written in.
func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

// skipBlanksAndComments consumes what separates tokens: blanks, line
// continuations, and comments.
func (l *Lexer) skipBlanksAndComments() {
	for !l.eof() {
		switch c := l.peek(); {
		case isBlank(c):
			l.advance()
		case c == '\\' && l.peekAt(1) == '\n':
			// Removed before tokens are formed, so it can split anything.
			l.advance()
			l.advance()
		case c == '#':
			// Only where a word could begin, which is the case here: mid-word
			// this function is not running. `echo a#b` prints a#b.
			//
			// An operand has no comments at all, whatever the mode says: see
			// inOperand, where the measurement is.
			if l.comments != CommentsSkipped || l.inOperand {
				// The `#` is the caller's to deal with: either it opens no
				// comment at all and starts an ordinary word, or Next
				// hands the comment back as a token. Either way nothing is
				// consumed here.
				return
			}
			l.skipComment()
		default:
			return
		}
	}
}

// skipComment consumes a `#` and the rest of its line, leaving the newline.
//
// The one implementation of the rule, and the reason it is a function rather
// than a loop written out three times is #1397: two of its three callers did
// not exist, so the bodies of `<( )`, `>( )` and `${ cmd;}` were scanned with
// no comment rule at all, and an apostrophe in `# it's fine` opened a quote
// that ran to the end of the file. Writing the loop again in each new place
// is how that comes back — a second helper omitting a fix the first one
// carries has bitten this repo four times — so all three go through here.
//
// Where a `#` is a comment at all is the callers' question, not this one's:
// between tokens skipBlanksAndComments only runs where a word could begin,
// and the two raw scans ask commentCouldStart.
func (l *Lexer) skipComment() {
	for !l.eof() && l.peek() != '\n' {
		l.advance()
	}
}

// CommentMode says what an unquoted `#` standing where a word could begin
// means to this lexer.
//
// The zero value is the shell's own rule and is what a parse that says
// nothing gets. Two callers say something. [ShellWords] re-reads a *value* as
// a command line rather than reading a program, and there the question has
// three answers rather than one — see the note on that function. A front end
// reading a line a person typed has two of the three, through
// [Dialect.Comments], because one shell in the panel reads a `#` at its
// prompt as an ordinary character.
//
// [CommentsKept] is the one that stays [ShellWords]': a token stream carrying
// a comment as a word is not a program, and the parser is never handed one.
type CommentMode uint8

const (
	// CommentsSkipped is the ordinary rule: a `#` opens a comment, the
	// comment runs to the end of the line, and none of it is a token.
	CommentsSkipped CommentMode = iota
	// CommentsOrdinaryText has no comments at all: a `#` is a character of
	// the word it stands in, wherever it stands.
	CommentsOrdinaryText
	// CommentsKept makes a comment one word, `#` included, running to the
	// end of the line and not taking the newline with it.
	CommentsKept
)

// bodyComments is the comment rule to record on a span this lexer is about to
// cut: the rule this lexer read it under for a command substitution, and
// [CommentsSkipped] — the ordinary rule — for everything else.
//
// **A command substitution and a process substitution answer differently**,
// which is measured and is not a distinction anyone would guess. zsh 5.9.2,
// 2026-09-12, `-f -i` on a pipe with `interactivecomments` off, so the typed
// line reads a `#` as a character:
//
//	echo M-$(echo a #b; echo AFTER)      M-a #b AFTER
//	cat <(echo MARK-a #b; echo AFTER)    MARK-a
//	cat =(echo MARK-a #b)                MARK-a
//	echo M-`echo a #b`                   M-a #b
//
// So the rule reaches `$( )` and its backquoted spelling and stops at `<( )`
// and `=( )`, whose bodies are read the way a script is however the line was
// typed. Turning the option on makes all four a comment, and there the *end*
// of the body moves with it too: `cat <(echo a #b) ; echo TAIL` and the `$( )`
// line beside it both become `parse error` because the comment swallows the
// `)`, exactly as they do in a script.
//
// Where the body ends is therefore a separate question from how it is read,
// and it is the one [Lexer.commentsExist] answers for every kind alike.
//
// Arithmetic is out for a third reason: `$(( 16#ff ))` is 255 in every shell
// in the panel and `$(( 1 # c ))` is a syntax error in all of them, so a `#`
// there is neither a comment nor a thing this rule is about.
func (l *Lexer) bodyComments(kind SpanKind) CommentMode {
	if kind != CommandSubst {
		return CommentsSkipped
	}
	return l.comments
}

// commentsExist reports whether a `#` opens a comment for the *raw* scans —
// the bodies of `$( )`, `<( )` and `${ ;}`, which have no word structure to
// consult and only need to find their closing character.
//
// Two of the three modes answer yes, which is not an oversight. Measured on
// zsh 5.9.2 with `v='a $(b # c) d'`: with the comment options off the whole
// `$(b # c)` is one word, and with either of them on the `#` opens a comment
// that swallows the `)` and the rest becomes a single unterminated word. So
// keeping a comment as a token is a rule about the *command line*, and inside
// a substitution a kept comment and a skipped one behave alike.
func (l *Lexer) commentsExist() bool { return l.comments != CommentsOrdinaryText }

// scanCommentWord takes a comment as a single word token, for CommentsKept.
// It stops at the newline rather than consuming it, so the newline is still
// the caller's to see — measured: the shell that has the construct answers
// `a`, `# hi`, `;`, `b` for a two-line value, and the `;` is the newline.
func (l *Lexer) scanCommentWord(start Pos) Token {
	l.skipComment()
	text := l.src[start.Offset:l.off]
	return Token{
		Kind:  TokWord,
		Pos:   start,
		End:   l.pos(),
		Text:  text,
		Spans: []Span{{Kind: Literal, Value: text, Quoting: Unquoted, Pos: start}},
	}
}

// prevByte is the byte in front of the cursor, or 0 at the start of the text.
func (l *Lexer) prevByte() byte {
	if l.off == 0 {
		return 0
	}
	return l.src[l.off-1]
}

// commentCouldStart reports whether a `#` at the cursor opens a comment, given
// the byte in front of it.
//
// Only for the raw scans, which have no word structure to consult; where the
// lexer is forming tokens the question does not arise, because
// skipBlanksAndComments runs only between them.
//
// prev is the byte before the `#`, or 0 at the start of the text. Measured
// across the panel inside `<(…)`: a comment opens at the start of the body
// (`<(#it's tight`), after a blank or newline, and immediately after `|` or
// `;` with no space — and does *not* open mid-word, where `echo a#b` prints
// `a#b` in all five shells that have the construct.
func commentCouldStart(prev byte) bool {
	switch prev {
	case 0, ' ', '\t', '\n', ';', '&', '|', '(', ')', '<', '>':
		return true
	}
	return false
}

// tryIONumber matches digits followed with no gap by a redirection operator.
func (l *Lexer) tryIONumber() (Token, bool) {
	if c := l.peek(); c < '0' || c > '9' {
		return Token{}, false
	}
	n := 0
	for {
		c := l.peekAt(n)
		if c < '0' || c > '9' {
			break
		}
		n++
	}
	// Strict adjacency: anything but a redirection here and these digits are
	// an ordinary word.
	if c := l.peekAt(n); c != '<' && c != '>' {
		return Token{}, false
	}
	// And the `<` has to be one. Where the dialect has numeric ranges the
	// operator these digits would attach to may be a pattern instead, and
	// then the digits belong to the word in front of it: `echo 2<->` is one
	// word rather than a redirection of descriptor 2.
	if _, ok := l.numericRangeAt(n); ok {
		return Token{}, false
	}
	// And width, where the dialect reads only one digit as a number. The
	// digits are then an ordinary word and the operator a redirection with no
	// number of its own, which is what makes `exec 10>f` a command called
	// `10` in three of the five shells.
	if n > 1 && !l.dialect.MultiDigitFdNumber {
		return Token{}, false
	}
	start := l.pos()
	digits := l.src[l.off : l.off+n]
	for range n {
		l.advance()
	}
	return Token{
		Kind:  TokIONumber,
		Pos:   start,
		End:   l.pos(),
		Text:  digits,
		Spans: []Span{{Kind: Literal, Value: digits, Quoting: Unquoted, Pos: start}},
	}, true
}

// tryFdVariable matches `{name}` followed with no gap by a redirection
// operator, in a dialect where the shell picks the descriptor and the name
// receives its number.
//
// The name must be one a variable could have; `{a,b}>f` has a comma and is a
// word, which is what keeps this clear of brace expansion. Where the dialect
// says so it may be a subscripted one — `{a[1]}>&-` names an element — and
// the comma rule survives that, because a subscript ends at its own `]` and
// what follows has to be the closing brace.
func (l *Lexer) tryFdVariable() (Token, bool) {
	if !l.dialect.FdVariableRedirections || l.peek() != '{' {
		return Token{}, false
	}
	n := 1
	for {
		c := l.peekAt(n)
		// A digit leads only where the dialect takes a positional
		// parameter here — `{1}>&-` closes the descriptor `$1` holds.
		// Whether a *mixed* name like `{1a}` is one anybody has is the
		// resolver's question and not this one's; see FdVariablePositional.
		digit := c >= '0' && c <= '9' && (n > 1 || l.dialect.FdVariablePositional)
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || digit {
			n++
			continue
		}
		break
	}
	if n == 1 {
		return Token{}, false
	}
	if l.peekAt(n) == '[' {
		sub, ok := l.fdVariableSubscript(n)
		if !ok {
			return Token{}, false
		}
		n = sub
	}
	if l.peekAt(n) != '}' {
		return Token{}, false
	}
	// The same strict adjacency an IO number has: anything but a
	// redirection after the brace and this is an ordinary word.
	if c := l.peekAt(n + 1); c != '<' && c != '>' {
		return Token{}, false
	}
	// And the same exception: `{a}<->` is a word, not a descriptor the shell
	// would pick.
	if _, ok := l.numericRangeAt(n + 1); ok {
		return Token{}, false
	}
	start := l.pos()
	text := l.src[l.off : l.off+n+1]
	for range n + 1 {
		l.advance()
	}
	return Token{
		Kind:  TokIONumber,
		Pos:   start,
		End:   l.pos(),
		Text:  text,
		Spans: []Span{{Kind: Literal, Value: text, Quoting: Unquoted, Pos: start}},
	}, true
}

// fdVariableSubscript reads the `[...]` of `{a[1]}`, reporting how far the
// token now reaches and whether there was a subscript there at all.
//
// It is deliberately narrow about what may stand between the brackets. The
// whole token becomes one literal span, so nothing in it is ever expanded —
// and a subscript that says `$i` and means the two characters would be worse
// than one that is not a subscript at all. What is left is what needs no
// expansion: a name, a numeral, an expression built from them. `{a[$i]}` is
// therefore an ordinary word here, which is the one spelling bash reads and
// this does not; ksh93 takes the token and then refuses the `$` in the
// arithmetic, so there is no answer that is everyone's.
//
// A subscript is also never empty and never nested: `{a[]}` and `{a[b[1]]}`
// are words, so a bracket that opens one has to close it before the brace.
func (l *Lexer) fdVariableSubscript(open int) (int, bool) {
	if !l.dialect.FdVariableSubscript {
		return 0, false
	}
	n := open + 1
	for {
		c := l.peekAt(n)
		if c == ']' {
			break
		}
		if c == '_' || c == '+' || c == '-' || c == '*' || c == '/' || c == '%' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			n++
			continue
		}
		return 0, false
	}
	if n == open+1 {
		return 0, false
	}
	return n + 1, true
}

// operators, longest first. Longest match wins, so the order is the algorithm
// and not merely tidiness: `>>` must be found before `>`.
var operators = []Kind{
	TokAmpDGreatClobber, TokAmpDGreatBang, // 4 bytes
	TokDSemiAmp, TokTLess, TokAmpDGreat, TokDLessDash,
	TokDGreatClobber, TokDGreatBang, TokAmpGreatClobber, TokAmpGreatBang, // 3 bytes
	TokAndAnd, TokOrOr, TokDSemi, TokSemiAmp, TokDGreat, TokLessAmp, TokGreatAmp,
	TokLessGreat, TokClobber, TokClobberBang, TokDLess, TokAmpGreat,
	TokAmpBang, TokAmpPipe, TokPipeAmp, TokSemiPipe, TokGreatSemi,
	TokLessHash, TokGreatHash, // 2 bytes
	TokAmp, TokPipe, TokSemi, TokLeftParen, TokRightParen, TokLess, TokGreat, // 1 byte
}

// enabled reports whether the dialect has this operator at all.
func (l *Lexer) enabled(k Kind) bool {
	switch k {
	case TokAmpGreat, TokAmpDGreat:
		return l.dialect.AmpersandRedirect
	case TokSemiAmp:
		return l.dialect.CaseFallthrough
	case TokDSemiAmp:
		return l.dialect.CaseContinue
	case TokSemiPipe:
		return l.dialect.CaseContinuePipe
	case TokTLess:
		return l.dialect.Herestring
	case TokAmpBang, TokAmpPipe:
		return l.dialect.BackgroundAndDisown
	case TokPipeAmp:
		if l.inCaseParenList && l.dialect.CasePatternListPipeIsOnlyASeparator {
			// Inside a `case` arm's parentheses one dialect reads the `|` as
			// the alternation separator and stops, so the `&` after it is a
			// token of its own. See
			// Dialect.CasePatternListPipeIsOnlyASeparator.
			return false
		}
		// Either reading lexes the two bytes as one token; which construct
		// they are is the parser's. Where neither is set the operator table
		// falls back to `|` then `&`, which is what bash 3.2 and dash lex.
		return l.dialect.PipeBothStreams || l.dialect.CoprocPipeOperator
	case TokClobberBang, TokDGreatClobber, TokDGreatBang:
		return l.dialect.ClobberOverrideMarker
	case TokGreatSemi:
		return l.dialect.RenameOnSuccessRedirect
	case TokLessHash, TokGreatHash:
		return l.dialect.SeekRedirect
	case TokAmpGreatClobber, TokAmpGreatBang, TokAmpDGreatClobber, TokAmpDGreatBang:
		// The marker on the both-streams operators needs those operators
		// first: where `&>` is not read at all, `&>|` cannot be the marker on
		// it, and the fallback that matters is `&` then `>|` rather than a
		// four-byte operator nobody wrote.
		return l.dialect.ClobberOverrideMarker && l.dialect.AmpersandRedirect
	}
	return true
}

// matchOperator finds the longest operator the dialect has at the cursor.
//
// Falling back to a shorter match when the longest is disabled is not a
// convenience: it is what the shells do. Where `&>` does not exist, `&>b` is
// `&` followed by `>b`, so the text still lexes and means something else.
func (l *Lexer) matchOperator() (Kind, bool) {
	rest := l.src[l.off:]
	for _, k := range operators {
		if !l.enabled(k) {
			continue
		}
		if strings.HasPrefix(rest, text[k]) {
			return k, true
		}
	}
	return 0, false
}

// remarkOnOperatorsRunTogether records two operators written with no blank
// between them, which one shell in the panel remarks on and the rest read
// without a word.
//
// The rule is about the *layout* rather than about the construct: writing the
// same two operators apart removes the remark and leaves whatever the parse
// was going to do alone, so `a |; b` is silent where `a |; b` written as
// `a|;b` is not, and both are refused identically. Half the shapes it fires
// on parse and run — `(:);(:)` and `echo a&;b` are ordinary programs — which
// is why this is a remark and not a fact hung off an error.
//
// Two halves of the condition, and each was measured rather than reasoned:
//
//   - The operator just lexed is exactly one character and is one of `;`,
//     `|` or `&`. Length is the whole of it: `&&(` is silent where `|(`
//     remarks, and `;;`, `;&`, `|&`, `&&` and `||` never start one because
//     the two bytes are one operator. Asking the *operator table* rather than
//     a list of pairs is what keeps that true per dialect — a dialect that
//     lexes `&|` as one token cannot remark on it, which is the answer the
//     shell that has that operator gives.
//   - The byte immediately after it begins another operator: `;`, `|`, `&`,
//     `(`, `<` or `>`. It is the byte and not the next token, which is what
//     `$(` settles — `:;$(:)` is silent where `:;(:)` remarks — and it is a
//     byte rather than a spelling, since `:;>>f` names `>` and not `>>`.
//     `)`, `{`, `!` and `[[` are not in the set, and neither is a word.
//
// Measured on ksh93u+ over a script file with `-n`, 2026-09-12. Recorded for
// every dialect and worded by one; see interp.Diagnostics.OperatorsNotSeparated
// and interp.RemarkOnlyWhenNotRunning, which is what holds it back on the
// routes that are going to run the program.
func (l *Lexer) remarkOnOperatorsRunTogether(start Pos, op string) {
	switch op {
	case ";", "|", "&":
	default:
		return
	}
	switch l.peek() {
	case ';', '|', '&', '(', '<', '>':
	default:
		return
	}
	l.remarks = append(l.remarks, Remark{
		Kind:  RemarkOperatorsNotSeparated,
		Pos:   start,
		At:    start,
		Token: op,
		Next:  string(l.peek()),
	})
}

// isWordEnd reports whether c ends an unquoted word.
func (l *Lexer) isWordEnd(c byte) bool {
	if isBlank(c) || c == '\n' {
		return true
	}
	switch c {
	case '&', '|', ';', '(', ')', '<', '>':
		return true
	}
	return false
}

// endsWord reports whether c closes the word being read.
//
// It is isWordEnd with the one exception that depends on where we are: a `(`
// that opens a pattern group belongs to the word instead of ending it, and the
// switch below cannot see it unless this lets it through.
func (l *Lexer) endsWord(c byte) bool {
	if !l.isWordEnd(c) {
		return false
	}
	// `<(` and `>(` belong to the word rather than ending it, the same way a
	// pattern group's `(` does. Without this the word scanner returns nothing
	// at all where Next has just decided a word starts here, which is not a
	// wrong answer so much as no answer: the cursor never moves.
	if l.startsProcSubst() {
		return false
	}
	// A numeric range does too, which is what keeps `<->\" \"<->` one word:
	// the pattern a real prompt theme compares a terminal size against.
	if l.startsNumericRange() {
		return false
	}
	if l.inRegex {
		// A regular expression owns its parentheses — a group is taken whole
		// by the scanner above — and owns a bare `|` where the dialect says
		// so: `[[ ab =~ a|b ]]` matches in two of the three shells with
		// `[[ ]]` and is a parse error in the third.
		switch c {
		case '(', ')':
			return false
		case '|':
			return !l.dialect.RegexTakesAlternation
		}
	}
	if l.pidBraces > 0 {
		// Inside a `{ … }` run opened immediately after `$$` nothing
		// separates either, until the brace that matches. One test rather
		// than one per character, for the reason the subscript below gives
		// and from the same kind of measurement — see
		// [Dialect.PidBraceGroupIsText], where the rows are.
		return false
	}
	if l.insideOpenSubscript() {
		// Inside a command word's subscript nothing separates: a blank, a
		// newline and every operator alike are characters of the subscript
		// until the matching `]`. One test rather than one per character,
		// because that is how it was measured — see
		// [Dialect.SubscriptSpansSeparators], where the rows are.
		return false
	}
	if c == '\n' && l.newlineIsText() {
		return false
	}
	if isBlank(c) && l.blankIsText() {
		return false
	}
	return c != '(' || (!l.opensPatternGroup() && !l.opensSubscriptFlags() && !l.opensTildeGroup())
}

// insideOpenSubscript reports whether the cursor stands inside a subscript
// the word being read opened at command position and has not closed yet.
//
// The depth counter alone is not the answer, because a bracket that never
// closes is not a subscript: `m[a b; echo done` is a word ending at the
// blank, which is what a grammar without the flag makes of it and what this
// leaves it as. See [Dialect.SubscriptSpansSeparators] for why the fallback
// is that rather than the refusal bash raises there.
func (l *Lexer) insideOpenSubscript() bool {
	if l.subscriptDepth == 0 {
		return false
	}
	return l.subscriptHasMatchingClose()
}

// subscriptHasMatchingClose reports whether the subscript now open is closed
// somewhere ahead, caching the answer for the rest of the word.
//
// It is asked by running **this same scanner** over the word from its start,
// with the question already answered yes, and reading off the depth it ends
// at — the way caseArmParenOpensAGroup asks where a pattern list ends by
// lexing one. A hand-written search for the matching `]` would be a second
// account of which brackets count, and it would have to be told about quotes,
// escapes and expansions all over again; this one cannot drift from the scan
// it is deciding for, because it is that scan.
//
// A probe that fails answers no: input that ran out inside a quote closed no
// bracket either, so the word falls back to the reading it has today and the
// quote is reported by the ordinary route.
func (l *Lexer) subscriptHasMatchingClose() bool {
	if l.assumeSubscriptCloses {
		return true
	}
	if l.subscriptCloses != 0 {
		return l.subscriptCloses > 0
	}
	probe := NewLexer(l.src[l.wordStart.Offset:], l.dialect)
	probe.assumeSubscriptCloses = true
	// The probe is the same scanner over the same bytes, so it has to stand
	// where this one stands: an element's bracket opens a subscript only
	// between an array literal's parentheses, and a probe that did not know
	// that would count no brackets, end at depth zero and answer "closes"
	// for a bracket that never does.
	probe.inArgument, probe.inArrayLiteral = l.inArgument, l.inArrayLiteral
	// And where a redirection's target stands, for the same reason: a target
	// spans in one dialect, so a probe that did not know it was in one would
	// answer for a different position than the scan it is deciding for.
	probe.inRedirectTarget = l.inRedirectTarget
	closes := probe.Next().Kind == TokWord && probe.err == nil &&
		probe.subscriptDepth == 0
	if closes {
		l.subscriptCloses = 1
	} else {
		l.subscriptCloses = -1
	}
	return closes
}

// opensCommandWordSubscript reports whether the `[` at the cursor opens a
// subscript that spans separators, given the word so far.
//
// `name` is the unquoted literal text read so far and `started` whether any
// span has been produced before it — together they are "the word so far is
// one unquoted name and nothing else", which is the whole of the condition
// besides the position. Measured: the bracket after anything that is not a
// name ends the word at the next blank in every shell that has the
// construct.
func (l *Lexer) opensCommandWordSubscript(name string, started bool) bool {
	return l.dialect.SubscriptSpansSeparators && !started && isName(name) &&
		(l.atCommandWord() || l.atSpanningRedirectTarget())
}

// atSpanningRedirectTarget reports whether the cursor stands in a redirection
// target that reads a subscript the way a command word does.
//
// A second position rather than a second rule: the name-in-front condition is
// opensCommandWordSubscript's and is asked once, above. What is asked here is
// only where the word stands — a redirection's target (inRedirectTarget) that
// is not itself an argument (inArgument), which is a prefix or a compound
// command's trailing redirection and not the `> f` after `echo hi`. See
// [Dialect.SubscriptSpansSeparatorsInRedirect] for the measurement, and note
// that atCommandWord is false in this position by construction: a target sets
// noAssignment, which that predicate reads.
func (l *Lexer) atSpanningRedirectTarget() bool {
	return l.dialect.SubscriptSpansSeparatorsInRedirect &&
		l.inRedirectTarget && !l.inArgument
}

// opensArrayElementSubscript reports whether the `[` at the cursor opens the
// subscript of a compound array literal's element, so that the blanks inside
// it are characters rather than the end of the element.
//
// The *front of the element* and nothing else, which is where it parts from
// opensCommandWordSubscript: that one wants a name in front of the bracket
// and this one wants nothing in front of it at all. Measured 2026-09-13 on
// bash 5.3.15 and on that same bash called `sh`, with an associative array so
// the key is visible rather than evaluated:
//
//	m=( [two words]=2 )     one element, keyed `two words`
//	a=( pre[1 2]=x )        two fields — a name in front does not span
//	a=( "[1 2]"=x )         one field, and no subscript: the quote is text
//	a=( x[1 2] )            two fields — a value is not a subscript
//
// ksh93 spans in this position too and reaches further than bash does, taking
// `pre[1 2]=x` as a subscripted element where bash makes two fields of it;
// that extra reach is recorded and not implemented, because the flag says
// where a subscript *may* stand and this is the shape all of the panel that
// spans here agrees on. bash 3.2.57 spans at command position and not here,
// and zsh globs the bracket instead — so the flag stays off for zsh, which is
// where it already was.
func (l *Lexer) opensArrayElementSubscript(name string, started bool) bool {
	return l.dialect.SubscriptSpansSeparators && l.inArrayLiteral &&
		!started && name == ""
}

// newlineIsText reports whether a newline here is an ordinary character of
// the word rather than the end of one.
//
// One position only: inside a `case` arm's parenthesized pattern list, where
// one dialect reads the whole list as a single alternation word. See
// Dialect.CasePatternListSpansNewlines, which is where it is measured — and
// note that a blank beside one is a separate question, answered by
// blankIsText below: `(a | b)` is two alternatives either way.
func (l *Lexer) newlineIsText() bool {
	return l.inCaseParenList && l.dialect.CasePatternListSpansNewlines
}

// blankIsText reports whether a blank here is an ordinary character of the
// word rather than the end of one.
//
// The same position newlineIsText answers for and the same shell, with one
// difference the newline does not have: a blank is text only where the
// pattern *continues* after it. A run of blanks in front of the `|` that
// separates two alternatives, or in front of the `)` that closes the list, is
// still a separator and is dropped, which is why `(a | b)` is two
// alternatives here as it is everywhere. See
// Dialect.CasePatternListSpansBlanks.
//
// The lookahead is over the whole run rather than one character, because what
// decides this is what the run leads *to*: `(a b )` and `(a b |z)` both end
// their pattern at `a b`, and the blanks that do so are two characters away
// from the one inside it.
//
// An operator after the run leaves the blanks a separator too, so `(a >b)`
// stays the parse error it is in that shell rather than becoming a pattern
// with a space on the end. A `(` is the exception, being a pattern group:
// `(a (b) c)` matches `a b c` there.
func (l *Lexer) blankIsText() bool {
	if !l.inCaseParenList || !l.dialect.CasePatternListSpansBlanks {
		return false
	}
	i := l.off
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	if i >= len(l.src) {
		return false
	}
	switch c := l.src[i]; c {
	case '\n':
		return l.newlineIsText()
	case '(':
		return true
	default:
		return !l.isWordEnd(c)
	}
}

// opensPatternGroup reports whether a `(` here belongs to the word.
//
// Two dialects allow it and they allow different things. One takes a group
// only behind a quantifier — `@(`, `?(`, `+(`, `*(`, `!(` — and the other
// takes a bare `(` anywhere inside a word.
//
// Nothing here tests for being mid-word, and it looked as though something
// should: a `(` that *starts* a word opens a subshell. It cannot reach here
// to start one. `(` is in the operator table, so a token beginning with it is
// taken as an operator and scanWord is never entered on one — a guard for it
// would be a line no test could distinguish.
// leadingParenBelongsToTheWord reports whether a `(` at the *front* of a
// token is part of the word rather than an operator.
//
// Where an argument may stand it always is, which is what `inArgument` says,
// and a `case` subject is such a position in the dialect that reads this at
// all — see inCaseSubject, where that measurement is.
// At the start of a `case` arm both readings are available at the same
// character — the arm carries an optional paren of its own, and a pattern
// may be a group — and what separates them is not the character but whether
// the arm is left a `)` to close it. Measured on zsh 5.9.2, 2026-09-07, each
// probe in a script file of its own under `env -i`:
//
//	case x in (a) …          the arm's paren, pattern `a`
//	case x in ((a|b)) …      the arm's paren, pattern `(a|b)`
//	case x in ((#i)a) …      the arm's paren, pattern `(#i)a`
//	case x in (a|b)) …       *no* arm paren, pattern `(a|b)`
//	case x in (a)b) …        *no* arm paren, pattern `(a)b`
//	case x in (#i)a) …       *no* arm paren, pattern `(#i)a`
//	case x in (a|b) ) …      *no* arm paren; a blank before the arm's `)`
//	case x in (a) b) …       refused: the arm's paren, then `b` is not `)`
//
// The `#` looked like the discriminator and is not — rows four, five and
// seven have no `#` in them and are the pattern's paren all the same, and
// the previous note here recorded `case x in (a)b)` as refused where that
// shell answers it. The `#` rows are subsumed rather than special: what
// decides them is the same thing that decides the rest.
//
// A group and a two-element pattern list match identically, so the two
// readings can only be told apart where the group carries text of its own —
// `(a)b)`, `(a|b)x)` — or where the arm reading runs out of parentheses.
// That is why the rule is about what follows the list and not about it.
//
// Read only where the *operator table* would otherwise take the paren. Once
// the token is known to be a word, `inArgument` alone decides how a leading
// group is scanned; see the note at that call.
func (l *Lexer) leadingParenBelongsToTheWord() bool {
	if l.inArgument || l.inCaseSubject {
		return true
	}
	if !l.inCaseArm || l.peek() != '(' {
		return false
	}
	// A glob flag is decided by the `#` alone, because the arm reading is
	// not available behind one at all: `#` where a word may begin opens a
	// *comment*, so `case x in (#i*)` read as the arm's paren swallows the
	// rest of the line. That shape is refused either way — this shell
	// answers `bad pattern: #i*` and reads the pattern — and the `#` is
	// what keeps the refusal pointing at the pattern rather than at the
	// arm's own paren.
	if l.peekAt(1) == '#' {
		return true
	}
	return l.caseArmParenOpensAGroup()
}

// caseArmParenOpensAGroup reports whether the `(` beginning a `case` arm is
// the *pattern's* rather than the arm's own, by reading the pattern list it
// would open and asking whether a `)` is left to close the arm.
//
// The lookahead is a throwaway lexer over the rest of the source, driven as
// an argument, rather than a hand-written scan for the matching `)`. That is
// the point of it: a group is taken whole — nesting, blanks and glob flags
// alike — by the scanner that already knows how, so there is no second
// implementation of "where does this group end" to drift from the first.
// Its diagnostics are discarded, and a probe that fails to read a list at
// all answers no, which leaves the arm's own paren as it was.
//
// It inherits that scanner's blind spot for a quoted `)` (#1241) rather than
// working around it, so the two answer the same shape the same way.
//
// Two of its guards are equivalent mutants, measured rather than assumed,
// and recorded so the next reader does not go looking for the row that would
// kill them. Both survive because the *caller* asks more than this does:
// nothing reaches here unless `opensPatternGroup` also says yes.
//
//	dropping the TokWord test      the leading `(` is folded into a word
//	                               exactly when a group opens there, and
//	                               where it is not — `()`, `((`) — the
//	                               caller has already declined
//	dropping the probe's Err test  the probe only fails where the real scan
//	                               fails on the same bytes, so the reading
//	                               it would have chosen never runs
func (l *Lexer) caseArmParenOpensAGroup() bool {
	probe := NewLexer(l.src[l.off:], l.dialect)
	// An argument is exactly the position a pattern list stands in once the
	// arm's paren is out of the way, which is also what the parser sets for
	// the patterns it reads after taking one.
	probe.inArgument = true
	for {
		if probe.Next().Kind != TokWord || probe.Err() != nil {
			return false
		}
		switch probe.Next().Kind {
		case TokRightParen:
			// The arm has its `)`, so the paren this started at was the
			// pattern's.
			return true
		case TokPipe:
			// So is a `|`, and it settles the question rather than leaving
			// it open. Were the leading `(` the arm's own, everything up to
			// the matching `)` would already be inside its pattern list and
			// the `)` would have closed the arm — so a `|` standing *after*
			// that `)` could only begin a body, and no body begins with one.
			// The group was one alternative of a longer list.
			//
			// Requiring another word after the `|` was wrong on five
			// measured rows, and found by mutation. `case a in (a|b)|)` is
			// the plain one: this dialect writes an alternative as nothing,
			// so there need not be a word there at all, and zsh 5.9.2
			// matches `a`. The other four are refusals whose *position*
			// moved — `(a|b)|c echo`, `(a|b)| echo`, `(a|b)|(c) echo` and
			// `(a|b)|c d)` are all refused there at the token after the
			// list, and falling back to the arm reading blamed the `|`.
			return true
		default:
			return false
		}
	}
}

// opensSubscriptFlags reports whether the `(` at the cursor opens a
// subscript's flag group — which is to say it stands immediately after the
// `[` that opened a subscript, and reads as a group.
//
// The bracket is the whole of the context needed: a group is only ever at the
// *front* of a subscript, so the character before it says which position this
// is without a depth counter. A `(` anywhere else in a word is the pattern
// group question and is answered below.
func (l *Lexer) opensSubscriptFlags() bool {
	if !l.dialect.ArraySubscriptFlags || l.off == 0 || l.src[l.off-1] != '[' {
		return false
	}
	_, _, ok := scanSubscriptFlags(l.src[l.off:])
	return ok
}

// opensTildeGroup reports whether the `(` at the cursor is the one a `~(…)`
// prefix opens.
//
// The tilde in front of it is the whole of the context, the same way the `[`
// is for opensSubscriptFlags: the group is only ever written straight after
// one, so the character before says which position this is without a counter.
// A `~` that has been quoted never reaches here — the scanner routes a quoted
// run elsewhere — which is measured to be right: `[[ abc == "~(E)"a.c ]]`
// does not match on ksh93 where the unquoted spelling does.
func (l *Lexer) opensTildeGroup() bool {
	return l.dialect.TildeGroup && l.off > 0 && l.src[l.off-1] == '~'
}

func (l *Lexer) opensPatternGroup() bool {
	// A `(` straight after `=` opens an array literal, never a group —
	// measured, because it is the same shell: `a=(b|c)` is a parse error
	// there rather than a pattern, so the assignment always wins.
	//
	// Outside a condition. Inside one there is no assignment for it to win
	// against: `[[ ]]` cannot contain one, so an `=` there is an ordinary
	// character of the pattern and the `(` after it is the group it looks
	// like. Measured, `[[ a=b == a=(b) ]]` and `[[ a=b == a=(b|c) ]]` are
	// both 0 in zsh 5.9.2 — the alternation reads too — where bash 5.3.15
	// answers `unexpected token `('` for the pair. Both columns still count
	// `a=(x y)` as two, which is the case this guard was written for and the
	// one it keeps.
	//
	// It is what stopped powerlevel10k parsing, and so what left a real
	// startup with no prompt: its `prompt[$' \t']#=([^$'\n']#)` matches the
	// `=` of a `prompt = value` line and captures what follows (#1585).
	if l.off > 0 && l.src[l.off-1] == '=' && l.arrayLiteralCouldStandHere() {
		return false
	}
	if l.dialect.PatternAlternation {
		// An empty `()` is a function definition and not a *bare* group,
		// which is how `f() { … }` survives the rule: the shell that takes
		// bare groups rejects `a()` as a pattern outright, so nothing is
		// lost by leaving it alone.
		return l.peekAt(1) != ')'
	}
	extended := l.dialect.ExtendedPattern ||
		(l.inCondition && l.dialect.ExtendedPatternInCondition)
	if !extended || l.off == 0 {
		return false
	}
	switch l.src[l.off-1] {
	case '@', '?', '+', '*', '!':
		// A *quantified* group may be empty, and an empty one is not the
		// function definition the bare spelling would be: the quantifier is
		// what opens it, so there is no name in front of the `(` for a
		// definition to have. Measured 2026-09-12 on bash 5.3.15 with
		// `extglob` on and on ksh93u+, which are unanimous — `echo +()z`
		// prints `+()z` with no such file, `case z in +()z)` matches
		// because a group standing for nothing matches nothing, and
		// `a+() { echo fn; }` is `syntax error near unexpected token `}''
		// in both, because the `+(` is read as the group and the name `a`
		// is left standing on its own. Refusing the `(` here forfeited two
		// whole files of a third-party corpus sweep (#2296).
		return true
	}
	return false
}

// scanPatternGroup consumes `( … )` and returns it as written, nesting and
// all. Nothing is interpreted here: the group is text until the matcher reads
// it, exactly as a bracket expression is.
func (l *Lexer) scanPatternGroup() string {
	start := l.off
	depth := 0
	for !l.eof() {
		c := l.advance()
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return l.src[start:l.off]
			}
		}
	}
	// Unterminated: the caller reports the word as unfinished, the same as an
	// unclosed quote.
	l.ranOut("pattern")
	l.fail(l.pos(), "unterminated pattern group")
	return l.src[start:l.off]
}

// bracketExprAt reports the width of the bracket expression at the start of
// src, and false where src does not begin one or the `]` never arrives on
// that line.
//
// The three rules that make this more than "find the next `]`" are the
// bracket expression's own, from POSIX XCU 2.13.1: a `]` first — after the
// negation, where there is one — is the character rather than the closer,
// and `[:class:]`, `[.collating.]` and `[=equivalence=]` hold a `]` that
// closes only themselves.
//
// A run that reaches a newline is not one. That is the bound: a `[` with no
// partner would otherwise take the rest of the input into the group, which
// turns a typo on one line into a refusal pages away.
func bracketExprAt(src string) (int, bool) {
	if len(src) == 0 || src[0] != '[' {
		return 0, false
	}
	i := 1
	if i < len(src) && (src[i] == '!' || src[i] == '^') {
		i++
	}
	if i < len(src) && src[i] == ']' {
		i++
	}
	for i < len(src) && src[i] != '\n' {
		switch {
		case src[i] == ']':
			return i + 1, true
		case src[i] == '[' && i+1 < len(src) && strings.IndexByte(":.=", src[i+1]) >= 0:
			end := strings.Index(src[i+2:], string(src[i+1])+"]")
			if end < 0 {
				return 0, false
			}
			i += 2 + end + 2
		default:
			i++
		}
	}
	return 0, false
}

// scanGroupSpans reads a `( … )` that belongs to a word and returns it as
// *spans* rather than as text.
//
// Text was the bug. A group used to be copied out of the source and appended
// to the word's unquoted literal run, which loses the two things quoting is
// for, in both directions:
//
//   - The scan ran over raw bytes, so a `\<` or a `"<"` inside a group ended
//     the word at its `<` and left the `)` to the parser — `[[ "<x" == (\<)* ]]`
//     was `parse error near `<”. The same for `\>`, `\;`, `\&`, `\)` and `\(`,
//     and for each of them written inside quotes (#1248).
//   - What did scan through arrived at the matcher with its quotes still in
//     it, so `[[ b == ("b") ]]` asked whether `b` is the three characters
//     `"b"` and answered no. That half is the silent one.
//
// Spans fix both at once, because a span carries its quoting and the pattern
// builder already knows what to do with it: a quoted span's metacharacters
// are escaped and an unquoted one's are live, which is exactly the rule the
// rest of a word gets. Nothing here decides what a character *means* — `(`,
// `|` and `)` stay in the literal text for the matcher to read, and a
// numeric range is stepped over whole for the same reason it is elsewhere.
//
// **It is the scanner for every group that belongs to a word**, and `;`, `<`,
// `>` and `&` end the word where they stand in one rather than being taken
// into it: `echo ( a <b )` leaves the `)` to the parser. A `|` does not,
// because a group may hold an alternation — `echo ( a|b )` is one word,
// which the shell then reports as matching nothing. Measured on zsh 5.9.2,
// 2026-09-07, from a script file under `env -i`, each probe in a file of its
// own so the first refusal does not hide the rest:
//
//	[[ $k == (a<b) ]]     parse error near `<'
//	[[ $k == (a>b) ]]     parse error near `>'
//	[[ $k == (a;b) ]]     parse error near `;'
//	[[ $k == (a&b) ]]     parse error near `&'
//	[[ $k == (a|b) ]]     matches — the `|` is the group's
//	[[ $k == a(b<c) ]]    parse error near `<'
//	[[ $k == a(b;c) ]]    parse error near `;'
//	[[ $k == a(b|c) ]]    matches
//
// **Position does not decide it either.** #1175 moved this from a route
// question to a "does the group start the word" one, and the last three rows
// say it is neither: a group in the middle of a word ends it at the same four
// characters, and reading them into it matched `ab<c` against `a(b<c)` where
// the shell will not read the line at all. Route and position were each the
// shape of where the group happened to be measured.
//
// They end the word only where they are *unquoted*, which is the fix: the
// four are operators for the same reason a `<` outside a group is one, and
// quoting is what says a character is not an operator anywhere else in a
// word.
//
// A *regular expression's* operand is the other answer and never comes
// through here: scanWord takes a regex group whole at its own case, and the
// four shells with `=~` agree it should — `[[ 'a<b' =~ (a<b) ]]` and
// `[[ 'a;b' =~ (a;b) ]]` both match in bash 5.3, bash 3.2, bash-as-`sh` and
// ksh93, where zsh refuses the `<` while parsing. So a regex operand owns its
// operators and a pattern operand does not, which is a difference between two
// constructs rather than the accident it looked like (#1175).
//
// Expansions inside a group *are* read here, through the same
// substitutionSpans the rest of a word goes through. They were literal text
// until #1331 and that was the second half of the same omission this scanner
// began as: quoting was shared with scanWord and expansion was not, so
// `[[ b == ("b") ]]` worked and `L=wait; [[ wait == ($L) ]]` matched the two
// characters `$L`. What an expansion's value is *worth* once it is here — a
// pattern or ordinary text — is the interpreter's GlobExpansionResults axis,
// answered where every other expansion's is.
func (l *Lexer) scanGroupSpans() []Span {
	// Whether this is a *quantified* group, which decides one rule below.
	// The quantifier is the character in front of the parenthesis, and
	// opensPatternGroup has already read it: a bare group is opened by the
	// parenthesis alone, in the one dialect that takes those.
	quantified := !l.dialect.PatternAlternation && l.off > 0 &&
		strings.IndexByte("@?+*!", l.src[l.off-1]) >= 0
	var spans []Span
	var lit strings.Builder
	litPos := l.pos()
	flush := func() {
		if lit.Len() > 0 {
			// PatternGroup marks the run as the group's own text. This scan
			// is the only place that knows: by the time a word is a list of
			// spans an unquoted `(` is a byte like any other, and a printer
			// reading the byte has to guess — escaping a group into three
			// literal characters, or writing a backslashed parenthesis back
			// live. It escaped, so `echo (v5|v6)` printed as
			// `echo \(v5\|v6\)`, which parses, runs, exits 0 and echoes
			// its own pattern (#1221).
			spans = append(spans, Span{
				Kind:         Literal,
				Value:        lit.String(),
				Quoting:      Unquoted,
				PatternGroup: true,
				Pos:          litPos,
			})
			lit.Reset()
		}
	}
	keep := func(c byte) {
		if lit.Len() == 0 {
			litPos = l.pos()
		}
		lit.WriteByte(c)
		l.advance()
	}
	depth := 0
	for !l.eof() {
		c := l.peek()
		// A numeric range's `<` is pattern text rather than the operator
		// that ends the word, so it is taken whole before the four
		// characters below get to see it — the precedence `next` and
		// `endsWord` already give it outside a group. Without this the
		// group ended at the `<` and left its `)` to the parser, which is
		// why every powerlevel10k config died on `(5.<1->*|<6->.*)` (#1217).
		//
		// `numericRangeAt` is the whole disambiguation and it is exact, so
		// the plain operator keeps every byte that is not a range: measured
		// on zsh 5.9.2, `(a<b)`, `(<a-b>)`, `(<1-2-3>)`, `(<-->)`, `(<>)`
		// and `(<1)` are all still parse errors there.
		//
		// Adding `&& depth > 0` here survives the suite, for the same
		// reason `depth >= 0` does below and recorded for the same reason:
		// the only pass with depth zero is the first one and its byte is
		// the `(`, which is not a `<`. It is an equivalent mutant rather
		// than a gap.
		if width, ok := l.numericRangeAt(0); ok {
			for range width {
				keep(l.peek())
			}
			continue
		}
		// Unquoted only. That is the whole of #1248: a `<` that a backslash
		// or a quote has protected is pattern text, and the cases below take
		// it before this one is reached.
		//
		// `depth > 0` cannot be false at one of those four bytes and is
		// kept for what it says rather than for what it decides: this is
		// entered on a `(`, so the only pass with depth zero is the first
		// one and its byte is that `(`. Mutating it to `depth >= 0`
		// survives the suite, and that is an equivalent mutant rather than
		// a gap — recorded here so the next reader does not go looking for
		// the row that would kill it.

		// A bracket expression inside a *quantified* group is the group's,
		// operators and all, so it is taken whole before the four
		// characters below get to see it — the same precedence a numeric
		// range has just above, and for the same reason.
		//
		// Only the quantified group, and that is measured rather than
		// tidied: bash 5.3.15 with `extglob` on and ksh93u+ both match
		// `x;y` against `x@([;])y` and both refuse the identical brackets
		// without the group — `case "x;y" in x[;]y)` is a syntax error in
		// every column, so this is the group protecting them and not the
		// brackets. zsh 5.9.2, whose groups are the bare ones, refuses
		// `x([;])y` in a condition and in a `case` alike, with and without
		// `extendedglob`, so reading the brackets there would accept what
		// that shell will not parse.
		if quantified && c == '[' {
			if width, ok := bracketExprAt(l.src[l.off:]); ok {
				for range width {
					keep(l.peek())
				}
				continue
			}
		}
		if depth > 0 && strings.IndexByte(";<>&", c) >= 0 {
			flush()
			return spans
		}
		switch {
		case c == '\\' && l.peekAt(1) == '\n':
			// A line continuation is removed before tokens are formed, here
			// as everywhere else, so it can split a group anywhere.
			l.advance()
			l.advance()

		case c == '\\':
			escPos := l.pos()
			atStart := lit.Len() == 0 && len(spans) == 0
			l.advance()
			if l.eof() {
				// The same word rule scanWord reads, from the same place.
				// The group is unterminated too, and that is the refusal
				// the panel gives here — reporting the backslash instead
				// named the wrong thing and got in front of it.
				flush()
				spans = append(spans, l.endOfInputBackslashSpan(escPos, atStart))
				continue
			}
			// Its own span, for the reason the same case in scanWord gives:
			// the protection has to outlive the lexer, because a later stage
			// decides whether the character is a metacharacter.
			flush()
			spans = append(spans, Span{
				Kind:    Literal,
				Value:   string(l.advance()),
				Quoting: BackslashQuoted,
				Pos:     escPos,
			})

		default:
			// The quoting and substitution forms, from the same list scanWord
			// reads. A group used to keep its own copy of it, holding the
			// quoting half and none of the expansions; see substitutionSpans
			// for what that cost (#1331).
			//
			// A substitution's own parentheses never reach `depth` below,
			// because the scanner that takes it balances them itself — which
			// is why `(a$(echo ')')b)` closes at the last `)` and not at the
			// one inside the command.
			if ss, ok := l.substitutionSpans(flush); ok {
				spans = append(spans, ss...)
				break
			}
			keep(c)
			switch c {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					flush()
					return spans
				}
			}
		}
		if l.err != nil {
			break
		}
	}
	// Unterminated: the caller reports the word as unfinished, the same as an
	// unclosed quote.
	flush()
	if l.err == nil {
		l.ranOut("pattern")
		l.fail(l.pos(), "unterminated pattern group")
	}
	return spans
}

// atCommandWord reports whether the token being read stands where a *command*
// may begin, rather than where an argument, a pattern, an operand, a
// redirection's target or a body stands.
//
// The reserved words are what need it. A `{` is one only at the front of a
// command, so `echo hi > {a}` writes a file with braces in its name and
// `for i in {a,b}` expands to two words, in the shell where `{a,b}` written
// first runs a command called `a,b`.
//
// It is the flag list [Lexer.atAssignValue] reads plus the two positions an
// assignment does not care about — a pattern operand and a regular
// expression's — since a word may not begin a command in either.
func (l *Lexer) atCommandWord() bool {
	return !l.inArgument && !l.inCondition && !l.inOperand && !l.inCaseArm &&
		!l.inCaseParenList && !l.inRawBody && !l.noAssignment &&
		!l.inPattern && !l.inRegex
}

// openBraceIsAWordOfItsOwn reports whether the `{` at the cursor is the
// reserved word by itself, whatever follows it.
//
// See [Dialect.OpenBraceNeedsNoBlank]. Bare and at command position, which is
// the whole of the rule: `'{'print` and `\{print` are ordinary words there,
// and `echo {print A}` keeps the brace because an argument may not begin a
// command.
func (l *Lexer) openBraceIsAWordOfItsOwn() bool {
	return l.dialect.OpenBraceNeedsNoBlank && l.peek() == '{' && l.atCommandWord()
}

// closeBraceIsAWordOfItsOwn reports whether the `}` at the cursor ends the
// word being read, leaving the brace to be read as the reserved word it is.
//
// The caller owns the two questions this cannot see — whether the brace pairs
// with a `{` already in the word, and whether anything has been read yet — and
// this owns the two it can: whether the brace ends the word at all, and the
// one position where it is text to the end.
//
// **An assignment's value is that position.** `x=a}` assigns `a}` in zsh
// 5.9.2 where `echo x=a}` is a parse error, so the carve-out is the
// assignment rather than the characters. Measured 2026-09-12; see
// [Dialect.CloseBraceAlwaysReserved].
func (l *Lexer) closeBraceIsAWordOfItsOwn() bool {
	if !l.dialect.CloseBraceAlwaysReserved {
		return false
	}
	if l.off+1 < len(l.src) && !l.isWordEnd(l.src[l.off+1]) {
		return false
	}
	return !l.inAssignmentValue()
}

// inAssignmentValue reports whether the word being read is an assignment and
// the cursor stands somewhere in its value.
//
// [Lexer.atAssignValue] answers the same question at the `=` itself and this
// answers it for the rest of the word, so the two share their shape: a name,
// an optional subscript and an optional `+`, in a position where an
// assignment may be written at all.
func (l *Lexer) inAssignmentValue() bool {
	if l.inArgument || l.inCondition || l.inOperand || l.inCaseArm ||
		l.inCaseParenList || l.inRawBody || l.noAssignment {
		return false
	}
	head, _, ok := strings.Cut(l.src[l.wordStart.Offset:l.off], "=")
	if !ok {
		return false
	}
	head = strings.TrimSuffix(head, "+")
	if i := strings.IndexByte(head, '['); i >= 0 {
		if !strings.HasSuffix(head, "]") {
			return false
		}
		head = head[:i]
	}
	return isNameIn(head, l.dialect.DottedName)
}

// endOfInputBackslashSpan is what an unquoted backslash the input ends
// immediately after leaves in the word being read.
//
// One home for the rule, called from both scans that can reach it — a word's
// and a pattern group's — because a decision written twice is a decision that
// can be changed once. It always returns a span, including for the reading
// that drops the backslash: a word that was nothing but the backslash is
// still a word there, and zsh prints the empty field to prove it.
//
// alone says the backslash is the first thing in the word, which only
// [EndOfInputBackslashIsLiteralOnlyAtAWordStart] asks about.
func (l *Lexer) endOfInputBackslashSpan(escPos Pos, alone bool) Span {
	value := "\\"
	switch l.dialect.BackslashAtEndOfInput {
	case EndOfInputBackslashIsDropped:
		value = ""
	case EndOfInputBackslashIsLiteralOnlyAtAWordStart:
		if !alone {
			value = ""
		}
	}
	// BackslashQuoted rather than plain literal text, for the reason the
	// ordinary escape below gives: the character is protected, and a field
	// that forgot that would take the backslash back as an escape the next
	// time something read it.
	return Span{Kind: Literal, Value: value, Quoting: BackslashQuoted, Pos: escPos}
}

// spansAreThePid reports whether what was just read is the bare `$$` and
// nothing else.
//
// The two characters rather than the parameter: `$!{a,b}`, `$?{a,b}`,
// `$-{a,b}` and `$#{a,b}` are all brace expansions in the dialect that has
// this, and so is `${$$}{a,b}`, so it is this spelling of this name. Measured
// with the rows on [Dialect.PidBraceGroupIsText].
func spansAreThePid(ss []Span) bool {
	return len(ss) == 1 && ss[0].Kind == ParamExp && ss[0].Bare &&
		ss[0].Quoting == Unquoted && ss[0].Value == "$"
}

// pidBraceSpan takes the `{` or the `}` of a run written immediately after
// `$$`, where the dialect makes that pair characters rather than syntax.
//
// It answers for the outer pair alone. A brace *inside* the run is counted
// and handed back, so it stays ordinary literal text and the brace expansion
// it may open still runs: `$${a{b,c}d}` is two words in the shell this is
// measured from, and `$${a,b}` is one.
func (l *Lexer) pidBraceSpan(armed bool, c byte) (Span, bool) {
	switch {
	case armed && c == '{':
		l.pidBraceOpen = l.pos()
		l.pidBraces = 1
	case l.pidBraces == 0:
		return Span{}, false
	case c == '{':
		l.pidBraces++
		return Span{}, false
	case c == '}':
		l.pidBraces--
		if l.pidBraces > 0 {
			return Span{}, false
		}
	default:
		return Span{}, false
	}
	pos := l.pos()
	l.advance()
	return Span{
		Kind: Literal, Value: string(c), Quoting: Unquoted,
		Pos: pos, PidBrace: true,
	}, true
}

// scanWord reads a word as a sequence of spans, one per run of uniform
// quoting. The spans are the point: a"b c"d is one word of three spans, and
// only the unquoted ones are subject to splitting and globbing later.
func (l *Lexer) scanWord(start Pos) Token {
	// Cleared on the way out rather than left behind: a failure raised after
	// the word is read belongs to no word, and a stale start would quote one
	// that had already finished.
	l.wordStart = start
	defer func() { l.wordStart = Pos{} }()

	var spans []Span
	var lit strings.Builder
	litPos := start

	flush := func() {
		if lit.Len() > 0 {
			spans = append(spans, Span{Kind: Literal, Value: lit.String(), Quoting: Unquoted, Pos: litPos})
			lit.Reset()
		}
	}

	// braces counts the bare `{` this word has open, so that the `}` closing
	// one is told from the `}` that closes nothing. Only literal text written
	// by the default case below counts: a quoted or escaped brace is a
	// character, and an expansion's braces are its own. See
	// [Dialect.CloseBraceAlwaysReserved], where the pairing is measured.
	braces := 0

	// The subscript counter is the lexer's rather than a local, because
	// endsWord is what consults it. Cleared here so that a bracket left open
	// by the word before cannot decide this one; the value is deliberately
	// *not* cleared on the way out, since the probe in
	// subscriptHasMatchingClose reads it off a finished scan.
	l.subscriptDepth, l.subscriptCloses = 0, 0
	l.pidBraces, l.pidBraceOpen = 0, Pos{}

	// pidArmed says the `$$` just read is touching a `{`, so that brace opens
	// a run of text. A local rather than a field: the arming and the brace it
	// arms for are one turn of this loop apart, and nothing outside sees it.
	pidArmed := false

	if l.openBraceIsAWordOfItsOwn() {
		lit.WriteByte(l.advance())
		flush()
		return Token{
			Kind: TokWord, Pos: start, End: l.pos(),
			Text: l.src[start.Offset:l.off], Spans: spans,
		}
	}

	for !l.eof() {
		c := l.peek()
		if l.endsWord(c) {
			break
		}
		if c == '}' && braces == 0 && l.pidBraces == 0 &&
			(lit.Len() > 0 || len(spans) > 0) &&
			l.closeBraceIsAWordOfItsOwn() {
			// The reserved `}` reaching into the word: it ends this one and
			// is read as the token it always is. Only where something has
			// been read already — a word that *starts* with `}` is that token
			// by the ordinary route, and stopping here would consume nothing
			// and never move.
			break
		}
		switch {
		case c == '\\' && l.peekAt(1) == '\n':
			l.advance()
			l.advance()

		case c == '\\':
			escPos := l.pos()
			atStart := lit.Len() == 0 && len(spans) == 0
			l.advance()
			if l.eof() {
				// A trailing backslash is neither unfinished nor wrong: it
				// is a word that ends in one, and what it leaves behind is
				// the dialect's. See [Dialect.BackslashAtEndOfInput].
				flush()
				spans = append(spans, l.endOfInputBackslashSpan(escPos, atStart))
				break
			}
			// Its own span: the protection must outlive the lexer, because a
			// later stage decides whether the character is a metacharacter.
			flush()
			spans = append(spans, Span{
				Kind:    Literal,
				Value:   string(l.advance()),
				Quoting: BackslashQuoted,
				Pos:     escPos,
			})

		case c == '(' && l.inRegex:
			// A regular expression's group is taken whole, balanced, with
			// whatever is inside it — an alternation in there belongs to the
			// group in all three shells that have `[[ ]]`, so it needs no
			// dialect. Only a *bare* `|` outside one does.
			if lit.Len() == 0 {
				litPos = l.pos()
			}
			lit.WriteString(l.scanPatternGroup())

		case c == '<' && l.startsNumericRange():
			// Taken whole, because the `>` that ends it would otherwise end
			// the word: the range is literal pattern text and the matcher
			// reads it later.
			if lit.Len() == 0 {
				litPos = l.pos()
			}
			width, _ := l.numericRangeAt(0)
			for range width {
				lit.WriteByte(l.advance())
			}

		case c == '(' && l.opensSubscriptFlags():
			// A flag group at the front of a subscript belongs to the word,
			// whatever the dialect says about pattern groups: `b[(r)y]=Q` is
			// one word and the group is scanned off its text later.
			//
			// It is the *assignment* that needs saying here. The brace-less
			// read `$a[(r)b]` is stepped over by bareSubscript and the braced
			// one never reaches this scanner at all, so this is the third
			// spelling and the one with no reader of its own — and it worked
			// only by accident where the dialect also had bare pattern
			// groups, which is a different flag answering a question that is
			// not its.
			lit.WriteString(l.scanPatternGroup())

		case c == '(' && l.opensTildeGroup():
			// A `(` straight after a `~` belongs to the word in the one
			// grammar that has `~(…)`, whatever that grammar says about
			// pattern groups — the prefix is written where no group would
			// be read, in an ordinary argument and in an assignment's
			// value, and it has to survive the lexer there too because the
			// shell keeps it as literal text. See
			// [Dialect.TildeGroup] for the rows.
			lit.WriteString(l.scanPatternGroup())

		case c == '(' && l.opensPatternGroup():
			// A parenthesised group belongs to the word rather than ending
			// it. Mid-word everywhere, and at the *start* of one only where
			// the parser has said a word may begin with one: elsewhere a
			// leading `(` opens a subshell, or is the paren a `case` arm may
			// carry, and neither is a pattern.
			if lit.Len() == 0 {
				litPos = l.pos()
			}
			// **Neither route nor position.** A group ends the word at a
			// shell operator wherever it stands, which is measured rather
			// than assumed — see scanGroupSpans, where the eight probes
			// are. An `l.inArgument` guard used to stand in this condition
			// so only the argument route stopped at one (#1175), and a
			// `lit.Len() == 0` test replaced it so only a group *opening*
			// the word did; both were the shape of the measurement rather
			// than of the shell. What #1161 measured still holds — a `case`
			// arm cannot tell the readings apart, `case x in (#i;a)b)` and
			// `case x in (#i<a)b)` being refused under all of them.
			flush()
			spans = append(spans, l.scanGroupSpans()...)

		case l.startsProcSubstFile():
			// The temp-file spelling, which is one dialect's. It is here
			// rather than beside the other two in `Next` because an `=` is
			// not an operator: a word may already have been started by the
			// scanner below and reach its `=` mid-word, and the position is
			// the whole question. See startsProcSubstFile.
			//
			// Unquoted only, for the reason the case below gives: what it
			// produces is a path, and a quoted path is still a path.
			flush()
			spans = append(spans, l.scanParens(ProcSubstFile, Unquoted))

		case l.startsProcSubst():
			// Unquoted only, and that is not an omission: `"<(echo hi)"` is
			// its own ten characters of text in every shell in the panel,
			// dash included, because what it produces is a *path* and a
			// quoted path is still a path — there would be nothing for the
			// quoting to change. Measured, and pinned by the corpus case
			// procsub/quoted-is-not-a-substitution; see
			// docs/spec/grammar/substitutions.md.
			flush()
			spans = append(spans, l.scanParens(procSubstKind(c), Unquoted))

		default:
			// Every other quoting and substitution form, from the list
			// scanGroupSpans reads from too. The cases above are the ones a
			// group does not get and cannot share — a regular expression's
			// parentheses, a subscript's flag group, the group itself, and
			// the `<(` whose first byte a group reads as an operator.
			if ss, ok := l.substitutionSpans(flush); ok {
				spans = append(spans, ss...)
				// Not inside a run already: a second `$$` in there opens
				// nothing, which is measured. `$${a$${b,c}d}` is two words
				// on the shell this comes from — the inner `{b,c}` expanded
				// as an ordinary list — where a nested run would have made
				// it one. The inner braces are counted below as the text
				// they are.
				pidArmed = l.dialect.PidBraceGroupIsText && l.pidBraces == 0 &&
					l.peek() == '{' && spansAreThePid(ss)
				break
			}
			armed := pidArmed
			pidArmed = false
			if sp, ok := l.pidBraceSpan(armed, c); ok {
				flush()
				spans = append(spans, sp)
				break
			}
			if lit.Len() == 0 {
				litPos = l.pos()
			}
			switch c {
			case '{':
				braces++
			case '}':
				if braces > 0 {
					braces--
				}
			case '[':
				// Counted only here, in the unquoted literal case, which is
				// what makes `m['a]b']=v` and `m[a\]b]=v` the one key they
				// are measured to be: a bracket a quote or a backslash
				// protects is a character, and one an expansion produces was
				// never written at all.
				if l.subscriptDepth > 0 ||
					l.opensCommandWordSubscript(lit.String(), len(spans) > 0) ||
					l.opensArrayElementSubscript(lit.String(), len(spans) > 0) {
					l.subscriptDepth++
				}
			case ']':
				if l.subscriptDepth > 0 {
					l.subscriptDepth--
				}
			}
			lit.WriteByte(l.advance())
		}
		if l.err != nil {
			break
		}
	}
	if l.pidBraces > 0 {
		// The run swallowed the rest of the input looking for its match, so
		// the refusal lands at the end of it — which is where the dialect
		// that has the construct puts it too.
		l.failUnmatched(l.pidBraceOpen, "{", "}", "unterminated brace run after `$$`")
	}
	flush()

	return Token{
		Kind:  TokWord,
		Pos:   start,
		End:   l.pos(),
		Text:  l.src[start.Offset:l.off],
		Spans: spans,
	}
}

// substitutionSpans scans the quoting or substitution construct standing at
// the cursor and returns its spans, reporting false and consuming nothing
// where none stands there. `flush` runs before the scan wherever one does, so
// the run of literal text in front of it keeps its place in the word.
//
// **It is the one list, and being one is the whole point of it.** scanWord
// and scanGroupSpans each held a copy, and the group's copy had the *quoting*
// forms and none of the expansions: `[[ b == ("b") ]]` was read, and
// `L=wait; [[ wait == ($L) ]]` asked whether `wait` is the two characters
// `$L`, answered no, and said nothing anywhere. The other half of the same
// omission is louder — `L=ice; print -r -- (${L}).zsh` reported
// `no matches found: (${L}).zsh`, naming a pattern nobody wrote (#1331).
//
// A group is not a second language. An expansion inside one is the same
// expansion it is anywhere else in a word, and its *value* is the pattern
// text: measured 2026-09-08, `[[ '$L' = ($L) ]]` does not match in zsh 5.9.2
// and `[[ '$L' = @($L) ]]` does not match in bash 5.3, bash 3.2 or ksh93 —
// the same answer read from both dialects that have the construct.
// Whether the metacharacters in that value are then live is the question the
// interpreter's GlobExpansionResults axis already answers, and it is answered
// where every other expansion's is rather than a second time here.
//
// Process substitution is deliberately *not* in the list. `<(` and `>(` are
// the only forms in it whose first byte is also one of the four operators
// that end a word inside a group, and there the operator wins. Measured on
// zsh 5.9.2, the only dialect with bare groups:
//
//	[[ x = (a<(echo x)b) ]]     process substitution … cannot be used here
//	print -r -- (a<(echo x)b)   number expected
//
// Both are refusals, so reading the `<(` as a substitution inside a group
// would make two constructs work that the shell does not have. It stays a
// case of scanWord's own.
// startsDelimiterSubstitution reports whether an *expansion* starts at the
// scanner's position — as opposed to a quoting form, which `$'` and `$"` are.
// Only read while [Lexer.inHeredocDelimiter] is set, where the two have to be
// told apart: quote removal applies to a delimiter and expansion does not.
func (l *Lexer) startsDelimiterSubstitution() bool {
	c := l.peek()
	if c == '`' {
		return true
	}
	if c != '$' {
		return false
	}
	switch l.peekAt(1) {
	case '(', '{':
		return true
	case '[':
		return l.dialect.DollarBracketArith
	}
	return l.startsBareParam()
}

// scanDelimiterSubstitution reads a substitution standing in a here-document's
// delimiter and hands back the source it occupied, as literal text.
//
// Scanned with the ordinary scanners because the *extent* is a real question —
// `<<$(echo X)` is one word, and a `)` inside a nested quote belongs to the
// substitution — and then discarded, because a delimiter takes quote removal
// and nothing else. The value is the raw source, so the word this span belongs
// to reads back identical to how it was written, which is what makes
// [Token.Literal] the delimiter again.
func (l *Lexer) scanDelimiterSubstitution(q Quoting) Span {
	start, startPos := l.off, l.pos()
	switch {
	case l.peek() == '`':
		l.scanBackticks(q)
	case l.peekAt(1) == '(' && l.peekAt(2) == '(':
		l.scanParens(l.doubleParenKind(0), q)
	case l.peekAt(1) == '[':
		l.scanBracket(q)
	case l.peekAt(1) == '(':
		l.scanParens(CommandSubst, q)
	case l.peekAt(1) == '{':
		l.scanBraces(q)
	default:
		l.scanBareParam(q)
	}
	return Span{Kind: Literal, Value: l.src[start:l.off], Quoting: q, Pos: startPos}
}

func (l *Lexer) substitutionSpans(flush func()) ([]Span, bool) {
	c := l.peek()
	// How far the `$` reaches over a line continuation written directly
	// behind it, and the form it was stopped at where it does not. Zero in
	// every dialect for text with no such pair in it, so every case below
	// reads as it did before the question existed.
	skip, stopped := l.continuationAfterADollar(l.off+1, Unquoted)
	switch {
	case c == '\'':
		flush()
		if s, ok := l.scanSingle(); ok {
			return []Span{s}, true
		}
		return nil, true

	case c == '"':
		flush()
		return l.scanDouble(), true

	case c == '$' && l.peekAt(1+skip) == '\'' && l.dialect.DollarSingleQuote:
		flush()
		if s, ok := l.scanDollarSingle(); ok {
			return []Span{s}, true
		}
		return nil, true

	case c == '$' && l.peekAt(1+skip) == '"' && l.dialect.DollarDoubleQuote:
		// `$"..."` marks the string for locale translation. With no message
		// catalog every shell that has the form reads it as a plain
		// double-quoted string — same escapes, same expansions — so the `$`
		// contributes nothing and the spans are exactly what a bare `"`
		// produces. The printer therefore writes them back as plain quotes:
		// the two spellings parse to identical trees, and recording the `$`
		// would be keeping a byte the tree has no question for. Where the
		// flag is off, the `$` falls through to the literal path, which is
		// what dash and zsh do with it.
		flush()
		l.advance() // $
		l.takeContinuationAfterADollar(Unquoted)
		return l.scanDouble(), true

	case l.inHeredocDelimiter && l.startsDelimiterSubstitution():
		// Below `$'` and `$"`, which are quoting rather than expansion and
		// so still apply. See inHeredocDelimiter.
		flush()
		return []Span{l.scanDelimiterSubstitution(Unquoted)}, true

	case l.opensDoubleParen(skip):
		// `$((` is arithmetic, unless the parentheses say otherwise: a
		// command substitution whose first command is a subshell may be
		// written with the two touching, and doubleParenKind is where the two
		// readings are told apart. Decided here because by the time the
		// parser sees tokens the choice has been made.
		flush()
		return []Span{l.scanParens(l.doubleParenKind(skip), Unquoted)}, true

	case c == '$' && l.peekAt(1+skip) == '[' && l.dialect.DollarBracketArith:
		// The older spelling of the case above. Where the flag is off this
		// falls through to the literal path, which leaves a `$` and a bracket
		// expression — what ksh93 and dash do with it.
		flush()
		return []Span{l.scanBracket(Unquoted)}, true

	case c == '$' && l.peekAt(1+skip) == '(':
		flush()
		return []Span{l.scanParens(CommandSubst, Unquoted)}, true

	case c == '$' && l.peekAt(1+skip) == '{':
		flush()
		return []Span{l.scanBraces(Unquoted)}, true

	case c == '$' && stopped == DollarBraces && l.dialect.DollarGoesWhenAContinuationStopsItAtABrace:
		// Stopped at a `${`, and this dialect drops the `$` rather than
		// leaving it as text. Nothing is flushed and no span is made: the
		// character is simply not part of the word, and the pair behind it is
		// removed by whatever removes one.
		l.advance() // $
		return nil, true

	case c == '$' && l.startsBareParamAt(1+skip):
		flush()
		return l.scanBareParam(Unquoted), true

	case c == '`':
		flush()
		return []Span{l.scanBackticks(Unquoted)}, true
	}
	return nil, false
}

// scanSingle reads '...'. Single quotes protect everything, and no escape
// exists inside them — a backslash is an ordinary character.
func (l *Lexer) scanSingle() (Span, bool) {
	open := l.pos()
	l.advance() // '
	var b strings.Builder
	for {
		if l.eof() {
			l.ranOut("'")
			if l.closesQuotesAtEOF() {
				// The end of input is as good as the closing mark here;
				// what was read is the string. ranOut still marks the
				// input incomplete, so a prompt continues the line.
				return Span{Kind: Literal, Value: b.String(), Quoting: SingleQuoted, Pos: open}, true
			}
			l.failUnmatched(open, "'", "'", "unterminated single quote")
			return Span{Kind: Literal, Value: b.String(), Quoting: SingleQuoted, Pos: open}, true
		}
		if l.peek() == '\'' {
			l.advance()
			return Span{Kind: Literal, Value: b.String(), Quoting: SingleQuoted, Pos: open}, true
		}
		b.WriteByte(l.advance())
	}
}

// dquoteEscapes are the only characters a backslash escapes inside double
// quotes. Before anything else the backslash is literal, so "a\nb" is
// backslash-then-n and not a newline — the rule C intuition gets wrong.
const dquoteEscapes = "$`\"\\"

// heredocEscapes are the only characters a backslash escapes in an unquoted
// here-document body. It is the double-quote set without the quote, because a
// quote there is an ordinary character with nothing to escape.
const heredocEscapes = "$`\\"

// operandEscapes is dquoteEscapes plus the closing brace, and it is the set
// that applies inside the operand of a `${ }` that stands in double quotes.
//
// The brace is the whole of the difference, and it is the brace and not
// braces: a backslash before `}` escapes it and is removed, while one before
// `{` is two characters of the result, and so is one before an ordinary
// character. Measured 2026-09-10 with `u` unset:
//
//	"${u-A\}B}"     A}B     the backslash is consumed
//	"${u-A\{B}"     A\{B    an opening brace keeps it
//	"${u-A\qB}"     A\qB    so does an ordinary character
//	"A\}B"          A\}B    and so does the same text outside an expansion
//
// The first row is dash, bash 5.3, that build as `sh`, ksh93u+ and zsh 5.9.2
// alike — only bash 3.2 answers `A\}B`, which is recorded in the corpus and
// not modeled, since no dialect here targets that build. The other three are
// unanimous across all six.
//
// It is the *enclosing* quotes that make this reading apply at all: written
// without them the operand is an ordinary word, where a backslash already
// escapes anything. So this set is what closes the gap between the two
// readings of `${u-A\}B}` rather than a rule of its own.
const operandEscapes = dquoteEscapes + "}"

// HeredocSpans splits an unquoted here-document body into spans.
//
// A body is not a word and not a double-quoted string, though it is much
// closer to the second: `$name`, `${ }`, `$( )`, `$(( ))` and backticks
// expand, a backslash escapes only those and itself and a newline, and
// everything else is literal — **quotes included**.
//
// Running the word lexer over it instead is what this replaces, and the
// difference is not subtle: quoting rules applied, so `don't` came out as
// `dont` and `\n` as `n`. It printed no error and exited 0, which is the
// worst way to be wrong.
//
// A quoted delimiter is not this. That body is literal throughout and never
// reaches here.
//
// The error is the text running out *inside* one of those constructs — an
// unterminated `${`, `$(` or backquote — and it is returned rather than
// swallowed because the spans alone cannot say it happened: a `${` with no
// closing brace hands back a parameter-expansion span whose value is whatever
// followed, which reads as an ordinary expansion nobody wrote. A caller that
// drops it answers `x` for `x${` where every shell in the panel refuses the
// text (#1653).
//
// Spans are still returned alongside it, as much of the body as was read, so
// a caller that has a use for a partial reading is not forced to re-lex.
func HeredocSpans(body string, d Dialect) ([]Span, error) {
	l := NewLexer(body, d)
	spans := l.heredocSpans()
	return spans, l.Err()
}

func (l *Lexer) heredocSpans() []Span {
	l.inRawBody = true
	var out []Span
	var b strings.Builder
	litPos := l.pos()
	flush := func() {
		if b.Len() > 0 {
			out = append(out, Span{Kind: Literal, Value: b.String(), Quoting: DoubleQuoted, Pos: litPos})
			b.Reset()
		}
	}
	for !l.eof() {
		c := l.peek()
		// An unquoted body reaches across a line continuation behind a `$` in
		// every dialect — see [Lexer.continuationAfterADollar], which reads
		// inRawBody and stops at nothing here.
		skip, _ := l.continuationAfterADollar(l.off+1, DoubleQuoted)
		switch {
		// The substitutions, which are the whole reason an unquoted body is
		// treated differently from a quoted one. Marked as double-quoted
		// because that is what stops the result being split: a body is one
		// blob of input, not a list of fields.
		case l.opensDoubleParen(skip):
			flush()
			out = append(out, l.scanParens(l.doubleParenKind(skip), DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.peekAt(1+skip) == '[' && l.dialect.DollarBracketArith:
			flush()
			out = append(out, l.scanBracket(DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.peekAt(1+skip) == '(':
			flush()
			out = append(out, l.scanParens(CommandSubst, DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.peekAt(1+skip) == '{':
			flush()
			out = append(out, l.scanBraces(DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.startsBareParamAt(1+skip):
			flush()
			out = append(out, l.scanBareParam(DoubleQuoted)...)
			litPos = l.pos()
		case c == '`':
			flush()
			out = append(out, l.scanBackticks(DoubleQuoted))
			litPos = l.pos()

		case c == '\\' && l.peekAt(1) == '\n':
			// A continued line, joined with no newline between.
			l.advance()
			l.advance()
		case c == '\\' && strings.IndexByte(heredocEscapes, l.peekAt(1)) >= 0:
			l.advance()
			if b.Len() == 0 {
				litPos = l.pos()
			}
			b.WriteByte(l.advance())
		default:
			// Everything else, and there is a lot of it: a backslash before
			// an ordinary character stays, both of it, and so does a quote.
			if b.Len() == 0 {
				litPos = l.pos()
			}
			b.WriteByte(l.advance())
		}
	}
	flush()
	return out
}

func (l *Lexer) scanDouble() []Span {
	open := l.pos()
	l.advance() // "
	return l.scanDoubleBody(open, true)
}

// scanDoubleBody reads double-quoted content from the cursor.
//
// `closing` says a `"` ends it, which is the ordinary run scanDouble opens.
// Without one the content runs to the end of the input and a `"` in it is an
// ordinary character — which is what the operand of a `${ }` written inside
// double quotes is. The quote that put it in this context is outside the text,
// so there is none to find and running out is not a failure.
func (l *Lexer) scanDoubleBody(open Pos, closing bool) []Span {
	// Without a closing quote to find, this text is a `${ }` operand — the one
	// place a backslash also escapes the brace that would end the expansion.
	// See operandEscapes.
	escapes := dquoteEscapes
	if !closing {
		escapes = operandEscapes
	}
	return l.scanDoubleEscaping(open, closing, escapes)
}

// scanNestedDoubleInOperand is a `"` run written *inside* a `${ }` operand,
// which is where the panel splits: five shells say the whole body of the
// expansion is the escaping context, so the brace is still escapable in the
// nested run, and one says the context resets at the quote. See
// Dialect.NestedQuoteResetsOperandEscapes.
//
// A function rather than a flag threaded through scanDoubleBody, because the
// nested run is the *only* caller that can be in this position: every other
// `"` either opens a word's own run or stands inside one, and neither has an
// operand around it to inherit an escape set from.
func (l *Lexer) scanNestedDoubleInOperand() []Span {
	open := l.pos()
	l.advance() // "
	escapes := operandEscapes
	if l.dialect.NestedQuoteResetsOperandEscapes {
		escapes = dquoteEscapes
	}
	return l.scanDoubleEscaping(open, true, escapes)
}

// scanDoubleEscaping is the body of both, with the escape set handed in.
//
// The set reaches the backslash and nothing else: a `$( )` or a backtick
// inside this run is scanned by its own reader in its own context, so an
// operand's widened set never leaks into a substitution written in one.
// Measured 2026-09-12, `"${u-$(printf %s "A\}B")}"` is `A\}B` in bash 5.3.15
// and zsh 5.9.2 alike — the two columns that disagree about the plain nested
// run agree here, which is what says the substitution starts over.
func (l *Lexer) scanDoubleEscaping(open Pos, closing bool, escapes string) []Span {
	var out []Span
	var b strings.Builder
	litPos := l.pos()
	flush := func() {
		if b.Len() > 0 {
			out = append(out, Span{Kind: Literal, Value: b.String(), Quoting: DoubleQuoted, Pos: litPos})
			b.Reset()
		}
	}

	for {
		if l.eof() {
			if !closing {
				flush()
				return out
			}
			l.ranOut("\"")
			if !l.closesQuotesAtEOF() {
				l.failUnmatched(open, "\"", "\"", "unterminated double quote")
			}
			flush()
			return out
		}
		c := l.peek()
		// The same lookahead substitutionSpans does, asked of the quoting
		// this run is: the panel splits widest here, and a `$` that reaches
		// nothing across the pair is read as text exactly as it was before.
		skip, stopped := l.continuationAfterADollar(l.off+1, DoubleQuoted)
		switch {
		// A `"` written inside an operand opens a run of its own rather than
		// standing for a character, which is the half of this that is *not*
		// like a single quote: `"${u:-"a b"}"` is `a b` in every shell in the
		// panel, quotes removed, where `"${u:-'a b'}"` keeps them. So the two
		// quote characters part company here and each keeps its own rule.
		case c == '"' && !closing:
			flush()
			out = append(out, l.scanNestedDoubleInOperand()...)
			litPos = l.pos()
		case c == '"':
			l.advance()
			flush()
			if len(out) == 0 {
				// An empty "" still produced a span: it is an empty field,
				// not the absence of one.
				out = append(out, Span{Kind: Literal, Quoting: DoubleQuoted, Pos: open})
			}
			return out

		// A double-quoted run inside a here-document's delimiter is still
		// part of the delimiter, and nothing in a delimiter expands:
		// `<<"$d"` is delimited by `$d` in bash, zsh and dash.
		case l.inHeredocDelimiter && l.startsDelimiterSubstitution():
			flush()
			out = append(out, l.scanDelimiterSubstitution(DoubleQuoted))
			litPos = l.pos()

		// Substitutions happen inside double quotes — "$(cmd)" is how most
		// scripts spell a substitution — so they are spans of their own here
		// too. The quoting is carried on them because it decides whether the
		// result is split afterwards, which is the only thing it changes.
		case l.opensDoubleParen(skip):
			flush()
			out = append(out, l.scanParens(l.doubleParenKind(skip), DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.peekAt(1+skip) == '[' && l.dialect.DollarBracketArith:
			flush()
			out = append(out, l.scanBracket(DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.peekAt(1+skip) == '(':
			flush()
			out = append(out, l.scanParens(CommandSubst, DoubleQuoted))
			litPos = l.pos()
		case c == '$' && l.peekAt(1+skip) == '{':
			flush()
			out = append(out, l.scanBraces(DoubleQuoted))
			litPos = l.pos()
		case c == '$' && stopped == DollarBraces && l.dialect.DollarGoesWhenAContinuationStopsItAtABrace:
			// Stopped at a `${`, and this dialect drops the `$` rather than
			// leaving it as text. Nothing is flushed and no span is made: the
			// character is not part of the run, and the pair behind it is
			// taken out by the continuation case below.
			l.advance() // $
		case c == '$' && l.startsBareParamAt(1+skip):
			flush()
			out = append(out, l.scanBareParam(DoubleQuoted)...)
			litPos = l.pos()
		case c == '`':
			flush()
			out = append(out, l.scanBackticks(DoubleQuoted))
			litPos = l.pos()

		case c == '\\' && l.peekAt(1) == '\n':
			l.advance()
			l.advance()
		case c == '\\' && strings.IndexByte(escapes, l.peekAt(1)) >= 0:
			l.advance()
			if b.Len() == 0 {
				litPos = l.pos()
			}
			b.WriteByte(l.advance())
		default:
			if b.Len() == 0 {
				litPos = l.pos()
			}
			b.WriteByte(l.advance())
		}
	}
}

// scanDollarSingle reads $'...'.
//
// The escape sequences inside are *not* resolved here, and that is a division
// of labor rather than a gap: the table is written down — see the `$'...'`
// section of docs/spec/grammar/tokenization.md, which measures it escape by
// escape — and interp's expandDollarSingle applies it. What this stage owes
// the later ones is the span's *quoting*, since a decoded tab must not be
// split on, and the raw text, so that decoding it later loses nothing.
func (l *Lexer) scanDollarSingle() (Span, bool) {
	open := l.pos()
	l.advance() // $
	l.takeContinuationAfterADollar(Unquoted)
	l.advance() // '
	var b strings.Builder
	for {
		if l.eof() {
			l.ranOut("$'")
			if l.closesQuotesAtEOF() {
				// The sixth caller of the rule the other quotes ask. ksh93
				// closes an unterminated `$'…'` at the end of a command
				// string exactly as it closes a plain one — measured
				// 2026-09-12, `-c` over `echo one` and `x=$'never closed`
				// prints `one` at status 0 there — and it was left out of
				// #1424 on purpose, being a loosening (#1468).
				return Span{Kind: Literal, Value: b.String(), Quoting: DollarSingleQuoted, Pos: open}, true
			}
			// Every shell in the panel that has the construct calls this a
			// plain `'`: the `$` opens it and the quote is what never
			// closed. So it reports the way scanSingle does and the sentence
			// falls out of the dialect, rather than the lexer writing one
			// nobody in the panel says at a line nobody names.
			l.failUnmatched(open, "'", "'", "unterminated single quote")
			return Span{Kind: Literal, Value: b.String(), Quoting: DollarSingleQuoted, Pos: open}, true
		}
		c := l.peek()
		switch {
		case c == '\'':
			l.advance()
			return Span{Kind: Literal, Value: b.String(), Quoting: DollarSingleQuoted, Pos: open}, true
		case c == '\\' && !l.eofAt(1):
			// A backslash consumes the next character whatever it is, so an
			// escaped quote does not end the span. Both bytes are kept.
			b.WriteByte(l.advance())
			b.WriteByte(l.advance())
		default:
			b.WriteByte(l.advance())
		}
	}
}

func (l *Lexer) eofAt(n int) bool { return l.off+n >= len(l.src) }

// Tokens reads the whole input. It is a convenience for tests and for callers
// that are not streaming; it stops at TokEOF or at the first error.
func (l *Lexer) Tokens() []Token {
	var out []Token
	var heredoc Kind
	for {
		// The delimiter is the one word position where nothing expands, and
		// the parser sets the same flag for it. See inHeredocDelimiter.
		l.inHeredocDelimiter = heredoc != 0
		t := l.Next()
		l.inHeredocDelimiter = false
		out = append(out, t)
		if t.Kind == TokEOF || l.err != nil {
			return out
		}
		// A here-document body is not lexical: where it ends is decided by a
		// delimiter the *parser* normally registers, and without that the
		// body is read as ordinary words. That is wrong in a way this helper
		// used to hide — a body with an apostrophe in it reported an
		// unterminated quote — so the queueing the parser would do is done
		// here too, from the same two tokens it uses.
		switch {
		case t.Kind.IsHeredoc():
			heredoc = t.Kind
		case heredoc != 0 && t.Kind == TokWord:
			// Only the delimiter's text is needed to find the body's end,
			// so the word is the token's spans as they stand: nothing here
			// expands, and a delimiter never does.
			l.queueHeredoc(&Redirect{
				Op:   heredoc,
				Word: &Word{Spans: t.Spans, Start: t.Pos, Stop: t.End},
			}, t.Text != t.Literal())
			heredoc = 0
		default:
			heredoc = 0
		}
	}
}

// doubleParenKind decides what a `$((` opens.
//
// Two constructs are spelled with the same three bytes. `$(( … ))` is an
// arithmetic expansion, and `$( ( … ) )` — a command substitution whose first
// command is a subshell — may be written with the two parentheses touching.
// POSIX tells the *author* to separate them and says nothing about what a
// shell does when they are not, so the answer is measured rather than
// reasoned from.
//
// Measured 2026-09-12. bash 5.3.15, the 3.2.57 macOS ships, that build
// invoked as `sh`, ksh93u+ and zsh 5.9.2 all run `echo $((echo ab cde) )` and
// print `ab cde`; dash 0.5.12 and BusyBox ash 1.37.0 refuse it, saying the
// `))` is missing. The same head count that put process substitution in the
// core, so [Dialect.ArithSubstFallsBackToCommandSubst] is on in [Core] and
// off in [POSIX].
//
// The rule the five agree on is positional and it is not "does the expression
// parse". Counting from one after the `$((`, find the `)` that brings the
// count back to zero: it is arithmetic when the very next byte is another
// `)`, and a command substitution otherwise. That is why `$(( 1 ) + (2 ))` is
// a command substitution in all five even though `(1) + (2)` is perfectly
// good arithmetic — the first `)` closes the count, and a `+` follows it —
// and why `$(( (1+2) ))` is 3 while `$(( (1+2)) )` runs `1+2` as a command.
// Reading it as "try arithmetic, fall back on a parse failure" gets both of
// those wrong.
//
// Quoting counts, so the scan is the same one scanParens does: `$(( "0)" ))`
// is arithmetic in bash — it reports an arithmetic error rather than running
// anything — because the `)` is inside a quoted string and closes nothing.
//
// Input that runs out is left to arithmetic, which is what the five say: an
// unfinished `$((` is `unexpected EOF while looking for matching `)'` there,
// and the command-substitution reading would blame a different construct for
// text that never closed either one.
// skip is how far the `$` at the cursor reached across a line continuation
// written behind it, which [Lexer.continuationAfterADollar] already answered
// for the dispatcher that routed here.
func (l *Lexer) doubleParenKind(skip int) SpanKind {
	if !l.dialect.ArithSubstFallsBackToCommandSubst {
		return ArithSubst
	}
	// The expression begins behind `$`, the pair the `$` reached across, `(`,
	// the pair *that* reached across, and `(`.
	from := l.off + 3 + skip + l.arithOpenerContinuationAt(l.off+2+skip)
	if doubleParenIsArith(l.src, from, l.dialect.ContinuationPartsTheArithmeticCloser) {
		return ArithSubst
	}
	return CommandSubst
}

// continuationWidthAt is how many bytes of line continuation stand at i in
// src, zero where none does.
//
// A plain count over the text rather than the lexer's own reader, for the
// reason skipQuotedFrom is one: every caller asks about a delimiter it has
// not reached yet, and may not move the cursor to find out.
func continuationWidthAt(src string, i int) int {
	n := 0
	for i+n+1 < len(src) && src[i+n] == '\\' && src[i+n+1] == '\n' {
		n += 2
	}
	return n
}

// arithOpenerContinuationAt is the width of a line continuation standing at
// i, between the two parentheses of an opening `$((` — zero where none stands
// there, and zero where the dialect parts the two. See
// [Dialect.ContinuationPartsTheArithmeticOpener].
func (l *Lexer) arithOpenerContinuationAt(i int) int {
	if l.dialect.ContinuationPartsTheArithmeticOpener {
		return 0
	}
	return continuationWidthAt(l.src, i)
}

// opensDoubleParen reports whether a `$((` stands at the cursor, reaching over
// a line continuation between the two parentheses where the dialect joins
// them — and over the one the `$` itself reached across, whose width the
// dispatcher has already worked out and hands in as skip.
func (l *Lexer) opensDoubleParen(skip int) bool {
	return l.peek() == '$' && l.peekAt(1+skip) == '(' &&
		l.peekAt(2+skip+l.arithOpenerContinuationAt(l.off+2+skip)) == '('
}

// takeArithOpenerContinuation steps a scanner's cursor over that pair, from
// between the two parentheses it has just read the first of. Asked again from
// there rather than handed in, so the scanner and the dispatcher that routed
// to it cannot disagree about what was skipped.
func (l *Lexer) takeArithOpenerContinuation() {
	for range l.arithOpenerContinuationAt(l.off) {
		l.advance()
	}
}

// doubleParenIsArith applies that rule to the `$((` at off in src.
//
// Taken apart from the method because the printer asks the same question of
// text it is about to write: a command substitution whose body opens with a
// parenthesis is written back without a space only where reading it again
// gives the construct back. One rule, asked from both ends.
// from is the offset the expression begins at — behind the whole opener,
// however many line continuations it was written with.
func doubleParenIsArith(src string, from int, partsCloser bool) bool {
	depth := 1
	for i := from; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '\'', '"', '`':
			if j := skipQuotedFrom(src, i); j > i {
				i = j
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				j := i + 1
				if !partsCloser {
					j += continuationWidthAt(src, j)
				}
				return j < len(src) && src[j] == ')'
			}
		}
	}
	return true
}

// skipQuotedFrom returns the offset of the byte closing the quote that opens
// at i, or i where nothing closes it.
//
// A plain scan rather than the lexer's own, because this one runs *ahead* of
// the cursor to answer a question about text nobody has read yet: it may not
// move the cursor, raise a remark or record a failure, and every one of those
// is what the lexer's quote skippers exist to do. Single quotes hold
// everything; double quotes and backticks let a backslash escape the next
// byte, which is the whole of what either needs here — the question is only
// where the quote ends, never what is inside it.
func skipQuotedFrom(src string, i int) int {
	quote := src[i]
	escapes := quote != '\''
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			if escapes {
				j++
			}
		case quote:
			return j
		}
	}
	return i
}

// scanParens reads $( … ) or $(( … )).
//
// The closing delimiter is not found by counting parens. A `)` inside quotes
// does not close the substitution — `$(echo ")" )` yields `)` in every shell
// in the panel — so the scan tracks quoting as it goes, using the same rules
// as the rest of the lexer. Counting alone truncates the substitution and
// silently changes the program.
func (l *Lexer) scanParens(kind SpanKind, q Quoting) Span {
	open := l.pos()
	l.advance() // $
	l.takeContinuationAfterADollar(q)
	l.advance() // (
	depth := 1
	if kind == ArithSubst {
		l.takeArithOpenerContinuation()
		l.advance() // the second (
		depth = 2
	}

	start := l.off
	if holdsCommands(kind) {
		// Where the contents end is a question about the grammar, not about
		// how many parentheses have been seen: a `case` arm's `)` closes
		// nothing, so counting stops early and takes half an arm with it.
		//
		//	x=$(case a in a) echo yes;; esac)
		//
		// Every shell in the panel runs that. Counting made it a syntax
		// error here — and worse, made it *parse into the wrong tree* where
		// the leftovers happened to be a command, which is how two scripts
		// on this machine parsed and could not be printed back.
		//
		// The contents of `$( )` are the contents of a subshell, so the
		// answer is the one the parser already knows: read a list, and stop
		// where it stops.
		//
		// And the contents of `<( )` and `>( )` are a subshell too, which is
		// what holdsCommands has said all along — but this route asked for
		// CommandSubst by name, so the two process substitutions never took
		// it and were left to the counting loop below. That loop knows
		// quotes and backslashes and nothing about comments, so `<(` + `#
		// it's fine` opened a single quote that ran to the end of the file
		// and the refusal landed 214 lines from the cause (#1397). Reading
		// them here is the fold: one scanner, one comment rule, one answer
		// to where a body ends, for all three kinds that hold a program.
		l.lastInner = ""
		if end, remarks, ok := l.parseToClose(start); ok {
			// What that read had to say comes back with it. A parse inside a
			// parse otherwise says nothing — the reason takeRemarks exists
			// below — and this is the *other* route into a substitution's
			// contents, reached when the grammar does find the `)`.
			//
			// Which is what a nested substitution looks like: the inner one
			// swallows the here-document and leaves a `)` for the outer one,
			// so the outer read succeeds where the un-nested shape's fails,
			// and the remark the innermost lexer raised was dropped with the
			// parser that noticed it (#1024). Only the dialects that read a
			// body as ending at the closing parenthesis get here at all: in
			// the others the inner construct is refused, this read fails
			// with it, and the refusal is the whole answer.
			l.remarks = append(l.remarks, remarks...)
			for l.off < end {
				l.advance()
			}
			value := l.src[start:l.off]
			l.advance() // the )
			return Span{Kind: kind, Value: value, Quoting: q, Pos: open, Comments: l.bodyComments(kind)}
		}
		// Not something the parser could read — half a line at a prompt,
		// most often. Counting is the older answer and is kept for it: it
		// gets the common shapes right and reports the rest as unterminated,
		// which is what an unfinished substitution is.
	}
	inner := l.lastInner
	l.lastInner = ""
	joined := l.collectContinuations()
	for depth > 0 {
		if l.eof() {
			if !l.incomplete && inner != "" && inner != openingOf(kind) {
				l.innerOpen = inner
			}
			l.ranOut(openingOf(kind))
			l.failedToClose(open, kind)
			break
		}
		switch c := l.peek(); c {
		case '\'':
			l.skipQuoted('\'', false)
		case '"':
			l.skipQuoted('"', true)
		case '`':
			l.skipBackticks()
		case '\\':
			l.noteContinuation()
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '#':
			// The same rule skipBlanksAndComments applies, through the same
			// helper, because this loop is the other way into the body of a
			// construct that holds a program and it has to agree with the
			// parser above about what a comment is. Without it the fallback
			// silently *accepts* what the fold correctly refuses: in
			// `<(echo hi # cmt )` the `)` is inside the comment, every shell
			// with the construct refuses the file, and counting found that
			// `)` and called the substitution closed.
			//
			// Gated on holdsCommands because `#` is not a comment in
			// arithmetic — `$(( 16#ff ))` is 255 in bash, bash 3.2, ksh93
			// and zsh, and all four call `$(( 1 # c ))` an arithmetic syntax
			// error rather than reading a comment.
			//
			// And gated on the byte in front, because a raw scan has no word
			// structure: `<(echo a#b)` prints `a#b` everywhere.
			if holdsCommands(kind) && l.commentsExist() && commentCouldStart(l.prevByte()) {
				l.skipComment()
				continue
			}
			l.advance()
		case '(':
			depth++
			l.advance()
		case ')':
			depth--
			l.advance()
		default:
			l.advance()
		}
	}

	// Where the loop stopped, kept before anything below moves the cursor.
	//
	// Only the refusal moves it, and a refused span is nobody's to read
	// today — a mutant taking the span from the moved cursor is
	// indistinguishable end to end. The span is still cut here, because the
	// alternative is a Value that silently becomes the rest of the file for
	// the first caller that reads a tree after a parse failure.
	stop := l.off

	if holdsCommands(kind) {
		// Counting found the end, which for a command substitution means the
		// read above could not — and the commonest reason is the shape #785
		// is about: a here-document
		// whose delimiter is only there because the `)` follows it, so the
		// body ran past the substitution and took the closing parenthesis
		// with it. The document *did* end at end of input, and the one shell
		// that says so says it here.
		//
		// Read from the substitution's own text, closing parenthesis
		// included, because that is what the here-document's input is: the
		// delimiter line is `EOF)` and matches nothing, and the last line of
		// the substitution is the last line the body could have. The trimmed
		// text would say the opposite — `EOF` alone is the delimiter, so
		// there would be nothing to remark on, which is also why the
		// substitution itself runs and yields `a`.
		remarks, bodyRanOut := l.takeRemarks(l.src[start:l.off], int(open.Line))
		// And whether that read is an answer at all is a dialect question,
		// which nothing here used to ask. Counting the parentheses finds the
		// `)` whatever shell this is, so all four dialects accepted a program
		// dash and zsh refuse.
		//
		// Reading the body from between the parentheses is one answer to
		// where it ends and reading it from the whole input is the other.
		// Under the second, the `)` counting just found is a line of the
		// body, nothing ever closed the construct, and the right complaint
		// is the one an unterminated `(` already gets — which is exactly
		// what dash and zsh say here, word for word.
		//
		// `depth == 0` because the loop above may have run out of input
		// instead of finding the `)`, which is already reported and is not
		// this. Nothing can observe the difference — ranOut keeps the first
		// call and the fail helpers keep the first error, so the second pass
		// would change nothing — and a mutant without it is byte-identical
		// across every unterminated shape in all four dialects. It stays
		// because "already reported" is the reason, not the idempotence.
		//
		// The remark is kept either way, and deliberately. It is what says a
		// body reached the end of this text, so the nesting depends on it:
		// `$(echo $(cat <<E` … `E))` is refused only because the inner
		// construct's remark reaches the outer one's read. It cannot be seen
		// where it is refused — a dialect that reads a body from the whole
		// input has no wording for a here-document at end of file, so there
		// is nothing to print beside the complaint — and suppressing it here
		// silently made that nested shape parse again.
		l.remarks = append(l.remarks, remarks...)
		if bodyRanOut && depth == 0 && !l.dialect.HeredocEndsAtClosingParen {
			// The body took the `)` and everything after it, so that is
			// where the cursor belongs: the input ran out inside this
			// construct and there is nothing left for anyone to read.
			//
			// Not bookkeeping. The line a refusal is located on is the line
			// the input ended on, and both shells that refuse this name it —
			// leaving the cursor at the `)` blamed the delimiter's line and
			// every one of the six corpus rows said so.
			for !l.eof() {
				l.advance()
			}
			l.ranOut(openingOf(kind))
			l.failedToClose(open, kind)
		}
	}
	// Trim the closing delimiters the loop consumed, reaching back over a
	// line continuation between them where the dialect joins the two: the
	// pair is not part of the expression, and leaving it in handed a `)` to
	// the evaluator. See Dialect.ContinuationPartsTheArithmeticCloser.
	end := stop
	for n := 1; n <= closers(kind) && end > start && l.src[end-1] == ')'; n++ {
		end--
		if n < closers(kind) && !l.dialect.ContinuationPartsTheArithmeticCloser {
			for end-2 >= start && l.src[end-2] == '\\' && l.src[end-1] == '\n' {
				end -= 2
			}
		}
	}
	value := joined(start, end)
	if kind != ArithSubst {
		// A program's text is read again by a parser of its own, which
		// removes its continuations where its own quoting says they are
		// continuations — and in a single-quoted part they are not.
		value = l.src[start:end]
	}
	return Span{Kind: kind, Value: value, Quoting: q, Pos: open, Comments: l.bodyComments(kind)}
}

// failedToClose records a parenthesised construct the input ran out inside.
//
// The two kinds part here, and on the line holdsCommands already draws.
// Everything that holds a *program* goes through failUnmatched, which carries
// what a dialect words from: the opener as written, the closer that never
// came, the text from the opener, and the line the input ran out on in both
// conventions. Before this, only `$(` did — the two process-substitution
// spellings took a plain formatted error instead, so all four dialects said
// the lexer's own sentence, none of them said what its shell says, and
// `ParseFailureLine` answered 0 because there was no *syntax.Error to read a
// line from (#1023).
//
// The one that holds an *expression* goes through it too, since #1086, and
// its line is why that took a second measurement rather than following on
// from the first. A dialect that reports an unmatched `$(` at the line after
// the input's last reports `$((` at the **opener's**: `echo $((1+2` with a
// trailing newline answers `line 1` where `v=$(echo hi` answers `line 2`.
//
// What settles it is that the line convention is already keyed on the opener
// rather than on the kind. ParseFailureLine reads CmdSubstUnmatchedAtEnd for
// `$(`, `<(` and `>(` and UnmatchedReportedAtOpener for everything else, and
// `$((` wants the second — which is the same answer a quote gets, in the one
// dialect that parts them, and is what that dialect prints. So the routing
// needed no new switch at all, and adding `$((` to the first set is the
// mistake that would have moved the line: measured per dialect afterwards,
// all four agree with the panel unchanged.
func (l *Lexer) failedToClose(open Pos, kind SpanKind) {
	l.failUnmatched(open, openingOf(kind), closingOf(kind), "unterminated "+kind.String())
}

// collectContinuations starts recording the line continuations of an
// arithmetic expression, and returns what ends the recording: the text between
// two offsets with every recorded continuation taken out of it.
//
// A backslash-newline is removed before tokens are formed, here as everywhere
// else, so it can stand anywhere in an expression — inside a number, between
// the two characters of `<<`, inside a name. Measured 2026-09-16 from script
// files, and unanimous in bash 5.3, bash 3.2, zsh 5.9.2, ksh93u+ and dash for
// every shape each of them has: `$(( 1\⏎2 + 1 ))` is 13, `$(( 1 <\⏎< 3 ))`
// is 8, `ab\⏎c` is the name abc, in `$(( ))`, in `(( ))`, in a C-style `for`
// header, in `$[ ]` and in a here-document body. The scanners that find where
// an expression ends step over the pair and used to keep it, so the
// evaluator was handed a backslash and a newline no arithmetic grammar has a
// rule for, and the script stopped there (#3442).
//
// Recorded where the scanner meets them rather than found again afterwards,
// because the scanner is what knows which backslashes are the expression's:
// one in a single-quoted part is a literal, and one inside a `$( )` is that
// program's to read by its own rules. What a scanner skips over through
// skipQuoted is hidden from the recording unless it is a double-quoted part
// of the expression itself, where a continuation is removed as it is in any
// double-quoted string.
func (l *Lexer) collectContinuations() func(from, to int) string {
	var cuts []int
	outer := l.continuations
	l.continuations = &cuts
	return func(from, to int) string {
		l.continuations = outer
		if len(cuts) == 0 {
			return l.src[from:to]
		}
		var b strings.Builder
		at := from
		for _, c := range cuts {
			if c < at || c+2 > to {
				continue
			}
			b.WriteString(l.src[at:c])
			at = c + 2
		}
		b.WriteString(l.src[at:to])
		return b.String()
	}
}

// byteAt is peekAt by absolute offset, for the lookahead that has to reach
// past the cursor's own idea of where it is.
func (l *Lexer) byteAt(i int) byte {
	if i < 0 || i >= len(l.src) {
		return 0
	}
	return l.src[i]
}

// dollarFormAt classifies the form a `$` introduces, read at i — the offset of
// the character behind the `$`. The empty set is "no form at all": a blank, a
// letter of no construct, the end of the word.
func (l *Lexer) dollarFormAt(i int) DollarForms {
	switch l.byteAt(i) {
	case '{':
		return DollarBraces
	case '(':
		return DollarParens
	case '[':
		if l.dialect.DollarBracketArith {
			return DollarBrackets
		}
	case '\'':
		if l.dialect.DollarSingleQuote {
			return DollarQuotes
		}
	case '"':
		if l.dialect.DollarDoubleQuote {
			return DollarQuotes
		}
	}
	if isBareParam(l.byteAt(i)) ||
		(l.dialect.BareParamFlags && bareFlagApplies(l.byteAt(i), l.byteAt(i+1))) {
		return DollarBareParameter
	}
	return NoDollarForm
}

// continuationAfterADollar reports how many bytes of line continuation a `$`
// reaches across, read at i — the offset of the character behind the `$` — and
// which form it reached. Zero where none is written, and zero where
// [Dialect.ContinuationStopsADollarAt] stops the `$` at the form behind the
// pair, in which case the `$` is read exactly as it was before this question
// existed: as text, with the pair removed afterwards by whatever removes one.
//
// A here-document's delimiter is left out. A continuation written in one is
// that document's question — see [Dialect.HeredocDelimiterAcrossAContinuation]
// — and nothing in a delimiter expands, so the form behind the pair is not a
// construct there at all.
func (l *Lexer) continuationAfterADollar(i int, q Quoting) (skip int, stoppedAt DollarForms) {
	if l.inHeredocDelimiter {
		return 0, NoDollarForm
	}
	n := 0
	for l.byteAt(i+n) == '\\' && l.byteAt(i+n+1) == '\n' {
		n += 2
	}
	if n == 0 {
		return 0, NoDollarForm
	}
	stops := l.dialect.ContinuationStopsADollarAt
	if q == DoubleQuoted {
		stops = l.dialect.ContinuationStopsADollarAtInDoubleQuotes
	}
	if l.inRawBody {
		// An unquoted here-document body stops nothing, in any dialect. Its
		// continuations are gone before an expansion in it is scanned —
		// measured 2026-09-16, `[$\⏎x][$\⏎{x}]` in a body delimited by an
		// unquoted `E` is `[5][5]` in bash 5.3, zsh 5.9.2, ksh93u+ and dash —
		// which is the same exemption [Lexer.continuationWaitsForAName]
		// records for that shell's refusal inside `${ }`.
		stops = NoDollarForm
	}
	if form := l.dollarFormAt(i + n); stops.Has(form) {
		return 0, form
	}
	return n, NoDollarForm
}

// takeContinuationAfterADollar steps a scanner's cursor over the line
// continuations its `$` reached across.
//
// Asked again from one byte on rather than handed in, so the scanner and the
// dispatcher that routed to it cannot disagree about what was skipped. It
// moves nothing unless a `$` is what was just read, which is what keeps it off
// `<( )` and `>( )` — [Lexer.scanParens] reads those through the same door.
func (l *Lexer) takeContinuationAfterADollar(q Quoting) {
	if l.byteAt(l.off-1) != '$' {
		return
	}
	skip, _ := l.continuationAfterADollar(l.off, q)
	for range skip {
		l.advance()
	}
}

// noteContinuation records the backslash at the cursor if it begins a line
// continuation and an expression is collecting them.
func (l *Lexer) noteContinuation() {
	if l.continuations != nil && l.peek() == '\\' && l.peekAt(1) == '\n' {
		*l.continuations = append(*l.continuations, l.off)
	}
}

// scanBracket reads `$[ … ]`, the older spelling of `$(( … ))`.
//
// It produces an ArithSubst span, because that is what it is: everything
// downstream — the expression parser, evaluation, every diagnostic — is the
// same, and only the delimiters differ. Span.Bracketed carries the spelling
// for the printer.
//
// The closing `]` is found the way scanParens finds its `)`: by tracking
// quoting and nesting rather than by taking the first one. Nesting is not
// theoretical here — a subscript is arithmetic too, and `$[a[1]+1]` answers
// in both shells that have the construct, so the inner `]` has to be counted
// past.
func (l *Lexer) scanBracket(q Quoting) Span {
	open := l.pos()
	l.advance() // $
	l.takeContinuationAfterADollar(q)
	l.advance() // [
	depth := 1
	start := l.off
	joined := l.collectContinuations()

	for depth > 0 {
		if l.eof() {
			l.ranOut("$[")
			// The older spelling names its own delimiters rather than
			// borrowing ArithSubst's: the dialect that echoes the closer
			// back says `]` here and `)` for `$((`, measured, and the kind
			// is the same for both spellings because everything downstream
			// of the delimiters is.
			l.failUnmatched(open, "$[", "]", "unterminated "+ArithSubst.String())
			break
		}
		switch l.peek() {
		case '\'':
			l.skipQuoted('\'', false)
		case '"':
			l.skipQuoted('"', true)
		case '\\':
			l.noteContinuation()
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '[':
			depth++
			l.advance()
		case ']':
			depth--
			l.advance()
		default:
			l.advance()
		}
	}

	end := l.off
	if end > start && l.src[end-1] == ']' {
		end--
	}
	return Span{Kind: ArithSubst, Value: joined(start, end), Quoting: q, Pos: open, Bracketed: true}
}

// parseToClose reads the contents of a command substitution and returns the
// offset of the `)` that ends them.
//
// A parser of its own over the rest of the input, which is what makes this
// answer the grammar's question rather than a counting one. It reports
// failure rather than a guess: an unfinished substitution has no closing
// parenthesis to find, and the caller has an older answer for that.
// takeRemarks reads text as a program of its own and keeps what it had to say
// about input it accepted anyway, with the positions moved into this source.
//
// A parse inside a parse otherwise says nothing: a *failure* there is reported
// as this parse failing, and a remark — the one thing a parser accepts and
// remarks on — was dropped with the parser that noticed it. `v=$(cat <<EOF` …
// `EOF)` runs, `v` is `a`, and the warning about the document ending at end of
// file went nowhere (#785).
//
// from is the line text begins on, so a remark names a line of the program
// rather than of the substitution. The offsets are relative to text and are
// left that way: nothing reads a remark's offset, and moving it would claim a
// correspondence this text does not have — it is a slice of the source here
// and is not, for the routes that hand this package a fragment.
//
// Only remarks are taken. Whatever else the read found — an error, a tree — is
// the caller's own business and it has already decided what to do about it.
//
// The remarks are returned rather than kept, because whether they are the
// caller's to keep is now a question: a here-document that ran to the end of
// this text is a body that would have run straight past the parentheses had
// it been read from the whole input, and in a dialect that reads it that way
// the construct is refused instead of remarked on. See
// Dialect.HeredocEndsAtClosingParen. The second return says which of those it
// was.
func (l *Lexer) takeRemarks(text string, from int) ([]Remark, bool) {
	sub := NewParserAt(text, l.dialect, from)
	sub.parseList()
	for _, r := range sub.lex.remarks {
		if r.Kind == RemarkHeredocAtEOF {
			return sub.lex.remarks, true
		}
	}
	return sub.lex.remarks, false
}

// It also returns what the read had to say about input it accepted, for the
// same reason takeRemarks does: a remark is the one thing a parser produces
// that its caller cannot recover from the tree or the error.
//
// The sub-parse is told which line it starts on, so those remarks name a line
// of the program rather than of the substitution. `$(` holds no newline, so
// the lexer's current line is the opener's, and the offsets the caller uses
// are untouched by it.
func (l *Lexer) parseToClose(from int) (int, []Remark, bool) {
	lex := NewLexer(l.src[from:], l.dialect)
	lex.line = l.line
	lex.inProgramParens = true
	sub := newParserOn(lex, l.dialect)
	sub.parseList()
	if sub.err != nil || !sub.at(TokRightParen) {
		// Kept for the one caller that goes on to run out of input itself,
		// which is where it becomes true of this lexer too. See innerOpen.
		l.lastInner = ""
		if sub.lex.incomplete {
			l.lastInner = sub.lex.OpenInnermost()
		}
		return 0, nil, false
	}
	return from + int(sub.tok.Pos.Offset), sub.lex.remarks, true
}

// parseToCloseBrace reads the body of `${ cmd;}` and returns the offset of
// the `}` that ends it, for the reading that ends it where a list ends.
//
// It is parseToClose's shape with the other closer, and for the same reason:
// what ends a command list is a question about the grammar rather than about
// how many braces have been seen. `}` is a reserved word, so it stands only
// where a command may begin — which is exactly what [Parser.parseList] stops
// on, and exactly why `${ echo } ;}` hands `echo` a literal brace and
// `${ echo hi}` never closes at all.
//
// A body the parser could not read comes back as not-ok and the caller
// reports the construct unterminated. There is deliberately no fall-back to
// counting braces here, which is where scanParens differs: counting would
// close `${ echo hi}` at the brace that ends the word, which is the reading
// this exists to remove.
//
// It is written as a candidate scan rather than as one read of the whole
// remaining text, because the closing brace need not be a token of its own:
// `echo ${ echo a;}Y` is `aY` in the column that has this rule, so the `}`
// ends the body and `Y` goes on with the word around it — where the same
// characters written as a group command, `{ echo a;}Y`, are refused. So each
// `}` that *could* be the reserved word is offered the text in front of it,
// and the first one a list actually ends at is the answer. Truncating at the
// candidate is what makes the offer honest: a `}` inside quotes or inside a
// substitution leaves that construct unterminated in the prefix, the read
// fails, and the scan moves on.
func (l *Lexer) parseToCloseBrace(from int) (int, []Remark, bool) {
	refused := -1
	for i := from; i < len(l.src); i++ {
		if l.src[i] != '}' || !braceCouldBeReserved(l.src, from, i) {
			continue
		}
		sub := NewParserAt(l.src[from:i+1], l.dialect, l.line)
		sub.parseList()
		if sub.err == nil && sub.atWord("}") {
			return i, sub.lex.remarks, true
		}
		if !sub.at(TokEOF) && refused < 0 {
			// The read stopped inside the prefix rather than running out of
			// it, so what is in front of this brace is text the grammar will
			// not take — with a complaint recorded or, for a pipeline with
			// nothing on its left, without one.
			refused = i
		}
	}
	if refused >= 0 {
		// A brace *does* stand where the reserved word can, and what is in
		// front of it is not a list the parser will take: `${ | REPLY=hi;}`
		// is a pipeline with nothing on its left. That is a complaint about
		// the body and not about the brace, and the panel words it that way
		// — `|' unexpected, looking for `}'` — so the expansion is closed
		// here and the body is handed on to be refused by whoever reads it.
		//
		// The distinction this keeps is the whole of #2711: a body with *no*
		// brace in command position at all is a different failure, and it is
		// the one that has to stay unterminated rather than closing at the
		// first `}` in sight.
		return refused, nil, true
	}
	return 0, nil, false
}

// braceCouldBeReserved is the cheap half of that scan: whether a `}` at i
// stands anywhere a reserved word could, given only the byte in front of it.
//
// Every brace the reserved word could be passes it and most of the braces in
// a body do not, which is the point — the expensive half is a parse, and
// `${x}` written in a body would otherwise buy one for every parameter the
// body reads.
func braceCouldBeReserved(src string, from, i int) bool {
	if i <= from {
		return true
	}
	switch src[i-1] {
	case ' ', '\t', '\n', ';', '&', '|', '(', ')':
		return true
	}
	return false
}

// procSubstKind says which end of the pipe the word will name.
func procSubstKind(c byte) SpanKind {
	if c == '>' {
		return ProcSubstOut
	}
	return ProcSubstIn
}

// holdsCommands reports whether what is between the parentheses is a program
// rather than an expression.
//
// It decides which spans are read again for what that read has to say about
// them, and the line it draws is not decoration: `$(( a << b ))` is a left
// shift, and reading it as a program makes `<<` a here-document whose
// delimiter `b` never arrives — a warning about a script that has none. The
// two process-substitution kinds are on this side of it, which is measured
// rather than assumed: bash remarks on `<(cat <<EOF` … `EOF)` exactly as it
// does on `$(cat <<EOF` … `EOF)`.
func holdsCommands(k SpanKind) bool {
	return k == CommandSubst || k == ProcSubstIn || k == ProcSubstOut ||
		k == ProcSubstFile
}

func closers(k SpanKind) int {
	if k == ArithSubst {
		return 2
	}
	return 1
}

// scanBracedArithmetic reads `${((expr))}`, one shell's second spelling for
// an arithmetic expansion. See Dialect.BracedArithmeticExpansion for the rows
// and for the space that tells it from the subshell spelling.
//
// It is asked before that one because the two adjacent parens are the whole
// of the discriminator, and it commits rather than backtracking: the shell
// that has this construct commits too, so `${((1+2)) }` is refused there
// rather than falling back to anything.
func (l *Lexer) scanBracedArithmetic(open Pos, start int, q Quoting) (Span, bool) {
	if !l.dialect.BracedArithmeticExpansion || start+1 >= len(l.src) ||
		l.src[start] != '(' || l.src[start+1] != '(' {
		return Span{}, false
	}
	l.advance() // the first (
	l.advance() // the second (
	from := l.off
	joined := l.collectContinuations()
	// Two parentheses to close, counted the way `$(( ))` counts them, so
	// `${(((1+2)))}` ends at the third `)` and not at the first pair.
	if !l.skipToDepth(2) {
		value := joined(from, l.off)
		l.ranOut("${")
		l.failUnmatched(open, "${", "}", "unterminated arithmetic expansion")
		return Span{Kind: ArithSubst, Braced: true, Value: value, Quoting: q, Pos: open}, true
	}
	end := l.off - closers(ArithSubst)
	value := joined(from, end)
	if l.eof() || l.peek() != '}' {
		// The brace has to sit directly behind the `))` with nothing between
		// them, not even a blank — the same shape the subshell spelling has,
		// and measured the same way.
		l.failUnmatched(open, "${", "}", "unterminated arithmetic expansion")
		return Span{Kind: ArithSubst, Braced: true, Value: value, Quoting: q, Pos: open}, true
	}
	l.advance() // the }
	return Span{Kind: ArithSubst, Braced: true, Value: value, Quoting: q, Pos: open}, true
}

// scanSubshellSubstitution reads `${(list)}`, the spelling whose body is a
// parenthesized subshell. See Dialect.SubshellSubstitution for the panel and
// for why the extent is the parenthesis rather than a command list.
//
// It is asked before the blank form's opener and answered with the parser
// rather than by counting, through the same parseToClose the two parenthesis
// scanners use: what closes a subshell is where a list stops, and a `)`
// written in a `case` arm or inside quotes closes nothing.
//
// A word the rule does not fit is left alone rather than refused — the `${`
// goes on to be read as it was before this flag, which for the `${(flags)x}`
// an expansion written for another shell reaches is the parameter form and
// the diagnostic Error.FlagGroupWordTail spells. That is ksh93's own order
// and not a fallback invented here: `ksh -n` refuses `${(echo a);}` while
// reading and passes `${(echo a)b}` to the run, so the paren-and-brace shape
// is settled first and everything else is somebody else's word.
func (l *Lexer) scanSubshellSubstitution(open Pos, start int, q Quoting) (Span, bool) {
	if !l.dialect.SubshellSubstitution || start >= len(l.src) || l.src[start] != '(' {
		return Span{}, false
	}
	if start+1 < len(l.src) && l.src[start+1] == '(' {
		// `${((expr))}` is the braced arithmetic expansion and not a
		// subshell — `${((echo hi))}` is an arithmetic syntax error in the
		// shell that has both. Two adjacent parens are the whole of the
		// discriminator, so `${( (1+2) )}` is a subshell running a command
		// called `1+2`. scanBracedArithmetic is asked first and has already
		// taken it where the dialect has that spelling; declining here is
		// what leaves the text the refusal it was for every dialect that
		// does not (#2725).
		return Span{}, false
	}
	end, remarks, ok := l.parseToClose(start + 1)
	if !ok || end+1 >= len(l.src) || l.src[end+1] != '}' {
		return Span{}, false
	}
	l.remarks = append(l.remarks, remarks...)
	for l.off <= end {
		l.advance() // through the )
	}
	value := l.src[start:l.off]
	l.advance() // the }
	return Span{Kind: CommandSubst, CurrentShell: true, Value: value, Quoting: q, Pos: open, Comments: l.bodyComments(CommandSubst)}, true
}

// scanBraces reads ${ … }. Same rule as scanParens: a `}` inside quotes does
// not close it. What the operators inside mean is a separate specification;
// this only finds the end.
func (l *Lexer) scanBraces(q Quoting) Span {
	open := l.pos()
	l.advance() // $
	l.takeContinuationAfterADollar(q)
	l.advance() // {
	start := l.off
	// Whether this is `${ cmd;}` or `${x}` is settled by the character after
	// the brace, so it can be settled here — before the loop that needs the
	// answer, rather than only at the bottom where the span kind is chosen.
	//
	// The loop needs it because a `#` is a comment in one form and an
	// operator in the other: `${ echo hi # it's fine\n}` yields `hi` in bash
	// 5.3 and ksh93, the two panel members that have the construct, while
	// `${x#a}` strips a prefix in all six. Same hole as #1397's, one scanner
	// over, and the same fix.
	//
	// `${|cmd;}` reaches the same answer by its own character, and the two
	// rules are deliberately asked separately: a blank opens the body of one
	// form and the `|` *is* the marker of the other, so `${ | cmd;}` is a
	// syntax error in the shell that has both. See Dialect.ReplySubstitution
	// for the five rows.
	reply := l.dialect.ReplySubstitution && start < len(l.src) && l.src[start] == '|'
	if span, ok := l.scanBracedArithmetic(open, start, q); ok {
		return span
	}
	if span, ok := l.scanSubshellSubstitution(open, start, q); ok {
		return span
	}
	brace := reply ||
		(l.dialect.CurrentShellSubstitution && start < len(l.src) && isBraceCommandStart(l.src[start]))
	// The line continuations of a parameter expansion are removed from its
	// text before it is read, as an arithmetic expression's are and by the
	// same recording, because a backslash-newline is removed before tokens
	// are formed and can stand anywhere in `${ }` — inside the name, in
	// front of the brace, between an operator's characters. Measured
	// 2026-09-16 from script files: `xy=5; echo "[${x\⏎y}]"` is `[5]`, and so
	// are `${x\⏎}`, `${x:\⏎-d}`, `${x%\⏎%p}` and `${#\⏎x}` in the shapes
	// each shell has, in bash 5.3, bash 3.2, zsh 5.9.2, ksh93u+ and dash —
	// all of which were a bad substitution in every dialect here (#3452).
	//
	// A command form's body is a program and its parser removes its own, so
	// only the parameter form takes the joined text.
	joined := l.collectContinuations()
	depth := 1
	// Where the body of each open nesting level began. Only the innermost
	// level's operand decides what a quote written there does — measured, and
	// not assumed: `w=Wq}r; echo "[${v-${w#'W}'}}]"` is `[Wq}r]` in six of the
	// seven columns, so the inner `#` governs a quote the outer `-` encloses.
	// A level opened by a bare `{` repeats its enclosing level's start,
	// because a brace opens no operand of its own.
	bodies := []int{start}
	// Where the command body itself begins, which is past the `|` of the
	// reply form. The two readings below both need it: one hands it to the
	// parser and the other measures token starts from it.
	body := start
	if reply {
		body++
	}
	// Which `}` ends a command body. See [Dialect.BraceProgramBodyEnd] for
	// the two rules and the rows that separate them.
	//
	// The stack is per level rather than per construct because a `${x}`
	// written inside a body is a parameter expansion again and ends at its
	// own brace wherever that brace stands: `${ echo ${x};}` yields the
	// value in ksh93, which the token-start rule applied to the inner level
	// would refuse. A level a token-start `{` opens keeps the rule, which is
	// what makes `${ echo {a,b};}` run off the end there.
	tokenStart := []bool{brace && l.dialect.BraceProgramBodyEnd == BraceProgramBodyEndsAtATokenStart}
	// Where the parser said the body ends, for the reading that ends it
	// where a list ends. -1 says nothing ends it, and the loop then reads to
	// the end of the input and reports the `${` unterminated — which is the
	// answer both shells give to `${ echo hi}` and is the whole of #2711.
	closeAt := -1
	listBody := brace && !tokenStart[0]
	if listBody {
		if end, remarks, ok := l.parseToCloseBrace(body); ok {
			// What that read had to say comes back with it, exactly as it
			// does for the parenthesis a substitution closes at.
			l.remarks = append(l.remarks, remarks...)
			closeAt = end
		}
	}
	// The offset of the most recent `)` that closed a `$( )`, which leaves
	// the cursor inside a word rather than at the end of a token. Only the
	// token-start reading consults it.
	wordParen := -1
	dollarParens := 0
	for depth > 0 {
		if l.eof() {
			l.ranOut("${")
			if q == DoubleQuoted && !l.inRawBody {
				// The `${` began inside a double quote, and three of the
				// panel blame the quote for the whole thing — the fourth
				// blames the quote character itself, which its wording of
				// this same failure carries.
				//
				// A body is the exception and it is not a quoting question:
				// heredocSpans marks its spans double-quoted so that nothing
				// it produces is split, and there is no quote character in
				// the text to blame. The panel words this one against the
				// brace — bash `unexpected EOF while looking for matching
				// }`, zsh `bad substitution` — rather than against a quote
				// nobody wrote (#1653).
				l.failUnmatched(open, "\"", "\"", "unterminated parameter expansion")
			} else {
				l.failUnmatched(open, "${", "}", "unterminated parameter expansion")
			}
			if se, ok := l.err.(*Error); ok && brace {
				// Which form the braces held, for the dialect that blames
				// the two at different lines. See Error.HoldsProgram. Read
				// back off the recorded error rather than passed in, because
				// failUnmatched may keep a report an inner construct already
				// made — and that one is not this construct's form.
				se.HoldsProgram = true
			}
			if se, ok := l.err.(*Error); ok && !brace && se.Pos == open {
				// What stood where the parameter form stopped, for the two
				// dialects that answer `${x` and `${x:-a}` differently. See
				// Error.BraceNameStop. The position guard is what keeps an
				// enclosing `${` from overwriting the answer an inner one
				// already recorded: failUnmatched keeps the innermost
				// report, and the stop belongs to whichever construct that
				// report is about.
				se.BraceNameStop, se.BraceNameStopFollowsAPrefix = braceNameStop(l.src[start:])
			}
			break
		}
		if listBody {
			// The end was settled before the loop, by the parser, so there
			// is nothing here to find: walk to it. Quoting, comments and
			// nested constructs were all the sub-parse's question and it has
			// already answered them, and closeAt of -1 walks to the end of
			// the input and is reported above.
			if l.off == closeAt {
				depth--
			}
			l.advance()
			continue
		}
		switch c := l.peek(); c {
		case '\'':
			// Whether this quotes, or is a character the `}` after it closes
			// the expansion behind. See Dialect.QuoteProtectsTheClosingBrace:
			// the panel splits on the kind of operand it is written in, not
			// on the shell, and the command form and the unquoted reading are
			// each unanimous.
			if brace || q != DoubleQuoted || l.quoteProtectsTheBrace(l.src[bodies[len(bodies)-1]:l.off]) {
				l.skipQuoted('\'', false)
			} else {
				l.advance()
			}
		case '"':
			// Unanimous in both operand kinds and in every column:
			// `v=SET; echo "[${v-"a}b"}]"` is `[SET]` in all seven, so no
			// flag is consulted here.
			l.skipQuoted('"', true)
		case '\\':
			if !l.continuationWaitsForAName(bodies[len(bodies)-1]) {
				l.noteContinuation()
			}
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '#':
			// The command form's body is a program, so it gets the program's
			// comment rule — through the same two helpers the other two
			// scanners use, because writing the loop a third time is how the
			// hole reopens. Measured in bash 5.3 and ksh93: mid-word is not
			// a comment (`${ echo x#y; }` yields `x#y`), and a comment runs
			// to the newline, so `${ echo hi # cmt }` is `unexpected EOF
			// while looking for matching }` rather than a closed expansion.
			if brace && l.commentsExist() && commentCouldStart(l.prevByte()) {
				l.skipComment()
				continue
			}
			l.advance()
		case '$':
			// `${` nests and a bare `{` does not, so the nesting is counted
			// on the *pair* rather than on the brace. Counting the brace
			// alone made an escaped dollar open a level nothing could close:
			// `\$` is consumed as an escape pair just above, which left its
			// `{` to be read as a nested expansion, and the `"` around the
			// whole thing was then eaten looking for the `}` that would
			// balance it. Measured on the pair, unanimous in both columns
			// that have the construct — `a=x; echo "${a/x/\${y}"` is `${y}`
			// in zsh 5.9.2 and bash 5.3.15 where this shell answered
			// `unmatched "` (#1586).
			//
			// The substitution check stays in front of it: `$(` is a
			// construct of its own and is stepped over whole, exactly as the
			// default branch does it for a backquote.
			//
			// Except under the token-start reading, which does not let a
			// `$( )` shield a brace — measured, and the one place the two
			// spellings of a substitution part company there:
			// `${ echo $(echo }x);}` ends the body at the brace inside the
			// parentheses and is refused, while the backquoted spelling of
			// the same body keeps it and yields `}x`. So the parentheses are
			// counted rather than stepped over, and what the count is for is
			// the `)` at the far end: it leaves the cursor inside a word, so
			// a `}` behind it begins no token. A subshell's `)` does —
			// `${ (echo q)}` is `q` there.
			if tokenStart[len(tokenStart)-1] && l.peekAt(1) == '(' {
				dollarParens++
				l.advance() // $
				l.advance() // (
				continue
			}
			if l.skipSubstitution() {
				continue
			}
			l.advance() // $
			if !l.eof() && l.peek() == '{' {
				depth++
				l.advance()
				bodies = append(bodies, l.off)
				// A `${x}` written in a body is a parameter expansion and
				// ends at its own brace wherever that brace stands.
				tokenStart = append(tokenStart, false)
			}
		case '{':
			// A bare `{` inside a *double-quoted* expansion is an ordinary
			// character and closes nothing in any of the six, so the brace
			// that opens a level there is only ever the one a `$` brought
			// with it.
			//
			// Quoting is half of the difference and it is measured.
			// `printf "[%s]" ${u:-{a,q}.z}` is `[a.z][q.z]` in zsh 5.9.2 —
			// two fields, so the operand ran to `.z` and the group was
			// expanded — and the same line in quotes is the single field
			// `[{a,q.z}]`, which is the operand stopping at the first `}`.
			// `"${a/x/{y}"` is `{y` for the same reason.
			//
			// `${a/x/{y}}` is deliberately not the case this rests on: it
			// is `{y}` whether the brace nests or not — closing early leaves
			// the second `}` as literal text and closing late puts it inside
			// the replacement — so it cannot tell the two readings apart.
			//
			// Unquoted the panel splits, so the second half is a dialect
			// question rather than a rule: zsh and ksh93 balance the group
			// and bash, bash 3.2 and dash stop at the first `}` there too,
			// which is BareBraceNestsInExpansion. Off in the core, because
			// the wider reading swallows text the other three leave in the
			// word.
			//
			// The command form keeps its own rule. Its body is a program and
			// its braces are that program's, so a `{ … }` block written in
			// one has to balance for the same reason its quotes do — which
			// is why the flag is not consulted for it.
			//
			// The token-start reading narrows that to a `{` which begins a
			// token, and the level it opens keeps the rule: that is what
			// makes `${ echo {a,b};}` run off the end of the input in the
			// column that has it, since the `}` of the group is mid-word and
			// closes nothing.
			nests := brace || (q != DoubleQuoted && l.dialect.BareBraceNestsInExpansion)
			if tokenStart[len(tokenStart)-1] {
				nests = l.atBraceTokenStart(body, wordParen)
			}
			if nests {
				depth++
				bodies = append(bodies, bodies[len(bodies)-1])
				tokenStart = append(tokenStart, tokenStart[len(tokenStart)-1])
			}
			l.advance()
		case ')':
			// Which kind of `)` this was, for the token-start reading above.
			// Nothing else consults it, and for every other reading this is
			// the default branch spelled out.
			if dollarParens > 0 {
				dollarParens--
				wordParen = l.off
			}
			l.advance()
		case '(':
			if dollarParens > 0 {
				// Inside a `$( )` already, so this parenthesis is one the
				// matching `)` at the far end has to get past — `$(( ))`
				// most of all, whose two closers would otherwise leave the
				// second one looking like a subshell's.
				dollarParens++
			}
			l.advance()
		case '}':
			if tokenStart[len(tokenStart)-1] && !l.atBraceTokenStart(body, wordParen) {
				// A `}` in the middle of a word is an ordinary character
				// there: `${ echo a}b;}` yields `a}b`.
				l.advance()
				continue
			}
			depth--
			if len(bodies) > 1 {
				bodies = bodies[:len(bodies)-1]
			}
			if len(tokenStart) > 1 {
				tokenStart = tokenStart[:len(tokenStart)-1]
			}
			l.advance()
		default:
			// A substitution written in the body is stepped over whole, so
			// a `}` inside it does not close this expansion and a `)` or a
			// backquote it never closes is blamed on the substitution
			// rather than on the brace. `${x:-$(echo hi` is `unexpected EOF
			// while looking for matching )` in bash, dash and ksh93, and the
			// backquoted spelling the same — both were `${` here, which is
			// the same cause as the nested case #1151 is about, one scanner
			// over.
			//
			// This is the *body* of the expansion holding a program, which
			// is a different question from what skipSubstitution's own note
			// declines to answer: that one is about a `${ }` written inside
			// a quote being skipped as text, where the panel disagrees over
			// what quoting means. What a `$( )` contains is not in dispute.
			if l.skipSubstitution() {
				continue
			}
			l.advance()
		}
	}
	end := l.off
	if end > start && l.src[end-1] == '}' {
		end--
	}
	value := joined(start, end)
	if brace {
		// `${ cmd;}` is a command and `${x}` is a parameter, and the space
		// is the whole of the difference — which is why it is decided here,
		// on the character after the brace, rather than by trying to read
		// what follows as a name and failing.
		//
		// The body is taken exactly as the parameter form takes it: a `}`
		// inside quotes does not close either, and both nest. Where it
		// *ends* is the one place they differ and is settled above; `body`
		// is past the `|` of the reply form, because the blank form's
		// opening blank is body text and this one's pipe is its marker.
		return Span{Kind: CommandSubst, CurrentShell: true, ReplyValue: reply, Value: l.src[body:end], Quoting: q, Pos: open, Comments: l.bodyComments(CommandSubst)}
	}
	return Span{Kind: ParamExp, Value: value, Quoting: q, Pos: open}
}

// continuationWaitsForAName reports whether a line continuation at the cursor,
// inside a `${ }` level whose text began at from, is kept rather than removed,
// under [Dialect.ParamContinuationNeedsAName]: whether no name has begun yet.
func (l *Lexer) continuationWaitsForAName(from int) bool {
	if !l.dialect.ParamContinuationNeedsAName || l.inRawBody {
		return false
	}
	// What stands in front of it, as written: a continuation already removed
	// in front of this one was behind a name or an operator, and nothing
	// after either of those can be the start of the expansion again.
	s := l.src[from:l.off]
	if s != "" && (s[0] == '#' || s[0] == '!') {
		s = s[1:]
	}
	switch {
	case s == ".":
		// A `.` alone has not begun one either: it opens a compound name,
		// and `${.sh\⏎.version}` reads where `${.\⏎sh.version}` is refused.
		return true
	case len(s) == 1 && strings.IndexByte("@*#?$!-", s[0]) >= 0:
		return true
	}
	// Nothing at all yet, or a positional parameter's digits.
	return strings.Trim(s, "0123456789") == ""
}

// isBraceCommandStart reports whether what follows `${` makes it a command
// rather than a parameter. A space, a tab and a newline do.
//
// This used to add "and nothing else can — a parameter name may not begin
// with any of them", which had the right premise and the wrong conclusion: a
// `(` cannot begin a name either, and ksh93 takes it, so `echo ${(echo hi)}`
// prints `hi` there and is a bad substitution in every other column.
//
// That is #2615 and it **is** implemented — one scanner over, in
// scanSubshellSubstitution, rather than by widening this test. Which is the
// measurement that settled the shape: `${(list)}` is not this construct with
// a `(` for an opener, because its extent is the parenthesis and not a list.
// `${(echo a); echo b;}` is refused there, blaming the `}`, and the blank
// form's `${ (echo a); echo b;}` runs both, so a `(` admitted here would
// accept a body ksh93 refuses while reading it. See
// Dialect.SubshellSubstitution for the six rows.
// atBraceTokenStart reports whether a brace at the cursor begins a token, for
// the reading that ends a `${ cmd;}` body at one. See
// [BraceProgramBodyEndsAtATokenStart].
//
// from is where the body began and wordParen the offset of the most recent
// `)` that closed a `$( )`. That second argument is the whole of the
// difference between `${ (echo q)}`, which is `q` in the column that has this
// rule, and `${ echo $(echo x)}`, which is refused there: both have a `)` in
// front of the brace and only one of them has ended a token with it.
func (l *Lexer) atBraceTokenStart(from, wordParen int) bool {
	if l.off <= from {
		return true
	}
	switch l.src[l.off-1] {
	case ' ', '\t', '\n', ';', '&', '|', '(':
		return true
	case ')':
		return l.off-1 != wordParen
	}
	return false
}

func isBraceCommandStart(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n'
}

// scanBackticks reads ` … `, the older command substitution. It nests only
// with backslash escaping, which is why $( ) exists and why this form is
// supported but never recommended.
func (l *Lexer) scanBackticks(q Quoting) Span {
	open := l.pos()
	// Said out loud by one shell and passed over by four, so it is recorded
	// here and worded — or not — by the front end. It is recorded before the
	// body is read rather than after, because a substitution that never
	// closes draws the remark as well as the refusal, and warning first is
	// the order the one shell that says both writes them in.
	l.remarks = append(l.remarks, Remark{Kind: RemarkBackquoteSubstitution, Pos: open, At: open})
	l.advance() // `
	start := l.off
	for {
		if l.eof() {
			l.ranOut("`")
			if !l.closesQuotesAtEOF() {
				l.failUnmatched(open, "`", "`", "unterminated backquote substitution")
			}
			return Span{Kind: CommandSubst, Backquoted: true, Value: unescapeBackquoted(l.src[start:l.off], q), Quoting: q, Pos: open, Comments: l.bodyComments(CommandSubst)}
		}
		switch l.peek() {
		case '\\':
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '`':
			end := l.off
			l.advance()
			return Span{Kind: CommandSubst, Backquoted: true, Value: unescapeBackquoted(l.src[start:end], q), Quoting: q, Pos: open, Comments: l.bodyComments(CommandSubst)}
		default:
			l.advance()
		}
	}
}

// unescapeBackquoted removes the one layer of backslashes the older
// substitution form requires, so the value handed on is the command text.
//
// This belongs to the lexer rather than to whoever evaluates the span,
// because it is part of *recognizing* the construct: POSIX gives the
// backslash its literal meaning inside backquotes except before `$`, a
// backquote, or another backslash. Doing it here is also what makes nesting
// work at all — the inner `\“ becomes a plain backquote, and re-lexing the
// result finds the nested substitution that `$( )` would have made obvious.
//
// q is the quoting the substitution was written in, and it adds a fourth
// character to that set. Inside double quotes a `\"` is unescaped as well, so
// `"`echo \"a b\"`"` runs `echo "a b"` and yields one field — measured
// 2026-09-13 across bash 5.3, bash 3.2, bash as sh, dash, ksh93, zsh and zsh
// as sh, every one of which prints `a b`. It is the backquoted form alone:
// the same escape inside `"$(echo \"x\")"` stays a backslash in all seven,
// and so does a backquote written outside double quotes. The rule is not the
// backslash meeting a quote, it is the two quotings meeting — which is why
// this takes the position as a parameter rather than growing a case.
//
// A backslash-newline is the one pair that is taken *out* rather than
// unescaped, and it is taken out whatever quoting it stands in inside the
// text. That is the whole difference between this form and `$( )`: the pair
// is removed here before the command text is parsed, so a single-quoted
// `'a\⏎b'` inside backquotes is the two-character string `ab`, where the
// same text inside `$( )` keeps the backslash and the newline. Measured
// 2026-09-16 from script files and unanimous in bash 5.3, bash 3.2,
// zsh 5.9.2, ksh93u+ and dash: a backquoted `printf %s 'a\⏎b'` is `ab`, and
// so are the double-quoted and the bare spelling of it, and so is a body read from a
// *quoted* here-document inside the substitution — which is the shape that
// says this is not a continuation the inner parse removes for itself, because
// nothing inside a quoted here-document removes one. `'a\\⏎b'` is `a\⏎b` in
// all five and falls out of the same pass: the `\\` is unescaped first and
// the newline it leaves behind is an ordinary character (#3453).
func unescapeBackquoted(s string, q Quoting) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '$', '`', '\\':
				i++
			case '\n':
				// Removed, not unescaped: neither character reaches the
				// command text.
				i++
				continue
			case '"':
				if q == DoubleQuoted {
					i++
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// skipQuoted consumes a quoted run while scanning inside a substitution. It
// does not build a span: the inner text is kept verbatim and re-lexed later by
// whoever parses the substitution.
//
// A double-quoted run is not a run of text. A substitution written inside one
// holds a *program*, and a program brings its own quoting: the single quotes
// in `"$(grep '"')"` quote that `"`, so it is not the one that ends the run.
// Skipping the run character by character reads it as the closing quote,
// leaves the cursor on the second `'`, and takes the rest of the file as a
// single-quoted string — which is why `${x:-"$(grep '"')"}` was refused with
// the brace never found, in all four dialects at once (#1140). So the skip
// steps over a nested substitution whole, exactly as scanDouble does when it
// is building spans rather than skipping them.
//
// A `${ }` written inside the run is the same sentence, and it was left out
// of it for a year: its operand is a word rather than a program, but the word
// brings its own quoting just as the program does, so the `"` in
// `"${y:-"Z"}"` is that expansion's and not this run's closing one (#2092).
// The loop below has it; skipSubstitution deliberately does not, and the note
// on it says why.
//
// Single quotes need none of this: nothing inside them is special, which is
// the whole of their specification, and `escapes` is the flag that already
// separates the two.
func (l *Lexer) skipQuoted(quote byte, escapes bool) {
	// A continuation inside a double-quoted part of an expression is the
	// expression's to remove, and one inside anything this run steps over on
	// the way — a `$( )`, a `${ }` — belongs to that construct instead, so the
	// collection is taken for this run's own backslashes and hidden from the
	// rest.
	cuts := l.continuations
	l.continuations = nil
	defer func() { l.continuations = cuts }()
	open := l.pos()
	l.advance() // opening quote
	for !l.eof() {
		c := l.peek()
		if c == quote {
			l.advance()
			return
		}
		if escapes && c == '\\' {
			if cuts != nil && l.peekAt(1) == '\n' {
				*cuts = append(*cuts, l.off)
			}
			l.advance()
			if !l.eof() {
				l.advance()
			}
			continue
		}
		if escapes && l.skipSubstitution() {
			continue
		}
		if escapes && c == '$' && l.peekAt(1) == '{' {
			// A `${…}` written inside this run brings its own quoting too,
			// so the `"` in `"${y:-"Z"}"` belongs to that expansion and is
			// not the one that ends this run. Skipping the run character by
			// character reads it as the closing quote, which leaves the
			// braces on either side of it counted by the *wrong* scanner:
			// the nested `${` is consumed inside the quote and uncounted,
			// and its `}` then lands outside and closes the enclosing
			// expansion. Measured 2026-09-11, unanimous across the six-shell
			// panel — `unset u w; printf '[%s]' "${u:-"${w:-"X{039}"}x"}"`
			// is `[X{039}x]` in zsh 5.9.2, bash 5.3.15, bash-as-sh, bash
			// 3.2.57, dash and ksh93, where this shell ended the expansion
			// at the `}` of `X{039}` and left `"}x"}` as literal text.
			//
			// This is the same sentence as the `$( )` case just above, one
			// construct over, and it was left out of it: powerlevel10k's
			// directory segment is `${P9K_CONTENT::="…${:-"%B%F{039}"}…"}`
			// and the prompt drew the `"}`, `}+}` and `}}}+}` tails of the
			// expansions this closed early (#2092).
			//
			// scanBraces rather than a brace counter here, because it is
			// already the answer to "where does a `${` end" — it knows that
			// a `}` inside quotes closes nothing, that `$(` ends at its own
			// parenthesis, and which dialects let a bare `{` nest. A second
			// counter beside it would be a second answer to one question.
			// DoubleQuoted because that is what this run is, and it is the
			// half that is measured rather than assumed: a bare `{` inside
			// one opens no level, so in a here-document body
			// `[${u:-"${w:-a{b}c"}z"}]` is `[a{bcz"}]` in zsh 5.9.2, bash
			// 5.3.15, that build as `sh`, bash 3.2.57 and dash alike. Passing
			// the run's quoting on is what keeps that true — read as if the
			// nested expansion stood bare, the `{` opens a level, the
			// expansion runs past the `}` that closes it and the line is
			// refused. With BareBraceNestsInExpansion off, as it is in the
			// core, the two readings agree, which is why the row that grades
			// this one carries the flag.
			l.scanBraces(DoubleQuoted)
			continue
		}
		l.advance()
	}
	// The input ran out inside this quote, and saying so is the whole of
	// what the skippers were missing: a delimiter scan used to return
	// quietly here, so the construct *enclosing* it reached end of input and
	// named its own opener — which meant the outermost was blamed in every
	// dialect, including the three that name the innermost (#1151).
	//
	// ranOut is deliberately not called here, and the two questions part on
	// exactly this point. What a *diagnostic* blames is the innermost
	// construct in three of the four dialects; what a **prompt** is still
	// waiting on is the outermost, which is what Lexer.openWord answers and
	// what Parser.Open reports — `echo $( echo 'x` is a substitution that is
	// still open, and the quote inside it is not the thing a continuation
	// prompt is for. The enclosing scanner records it on its way past.
	//
	// Unless the route ends a quote here, in which case there is nothing
	// unfinished to report and the enclosing construct is the only thing
	// still open. The fourth caller of the same rule, and the one that was
	// left out: measured 2026-09-07, `ksh -c 'echo $( echo "hi'` and
	// `ksh -c "echo \$(echo 'abc)"` are both ``syntax error at line 1: `('
	// unmatched`` — the quote is closed and the substitution is what ran
	// out — where we named the quote (#1424).
	if !l.closesQuotesAtEOF() {
		l.failUnmatched(open, string(quote), string(quote), unterminatedQuoteMsg(quote))
	}
}

// unterminatedQuoteMsg is the substrate's own sentence for a quote the input
// ran out inside, matching what scanWord says for the same failure so the two
// routes into it cannot drift apart.
func unterminatedQuoteMsg(quote byte) string {
	if quote == '\'' {
		return "unterminated single quote"
	}
	return "unterminated double quote"
}

// skipSubstitution steps over a substitution beginning at the cursor and
// reports whether one did. It is the skipping counterpart of the spans
// scanWord builds, and it exists for the scanners that only need to find a
// delimiter: `$( )`, `$(( ))` and the backquoted form all hold input whose
// quoting is its own.
//
// `${ }` is deliberately not one of them. Its body is a word rather than a
// program, and what quoting means inside it is exactly where the panel stops
// agreeing — `${x:-"${y:-'"'}"}` is accepted by bash alone and refused by the
// other five — so it is a dialect question rather than a delimiter one, and
// nothing here should answer it.
//
// Where a `${ }` *does* have to be stepped over is skipQuoted, and the
// difference is which question is being asked: there the run has already been
// entered and the only thing wanted is where the nested expansion ends, which
// scanBraces answers. Here the caller is still choosing what it is looking
// at, and the two callers want opposite things — scanBraces counts a nested
// `${` on its own loop, with its own `brace` and its own dialect flags, and a
// step-over here would take that decision away from it.
func (l *Lexer) skipSubstitution() bool {
	// A continuation inside the program is that program's, read by its own
	// rules when it runs — and a quoted here-document in it keeps one — so
	// whatever construct is collecting its own does not see these.
	cuts := l.continuations
	l.continuations = nil
	defer func() { l.continuations = cuts }()
	if l.peek() == '`' {
		l.skipBackticks()
		return true
	}
	if l.peek() != '$' || l.peekAt(1) != '(' {
		return false
	}
	open := l.pos()
	l.advance() // $
	l.advance() // (
	// The arithmetic spelling needs no case of its own. Its second `(` is
	// the next character skipToDepth reads, and counting it there is what
	// makes both `)` at the other end belong to the construct — so one loop
	// serves `$( )` and `$(( ))` alike.
	if !l.skipToDepth(1) {
		// It ran out rather than finding the `)`, and it is the construct
		// three of the four dialects blame — `echo "${x:-"$( echo hi` is
		// `unexpected EOF while looking for matching )` in bash, exactly as
		// the un-nested shape is. Before this the skip returned quietly and
		// the `${` around it was blamed instead (#1151).
		//
		// Named as the plain substitution and not the arithmetic one: this
		// skip does not know which it stepped over, and the two spellings
		// are told apart by their closers, which it never reached. The
		// scanners that *do* know still report their own.
		l.failUnmatched(open, "$(", ")", "unterminated "+CommandSubst.String())
	}
	return true
}

// skipToDepth consumes input until the given number of parentheses have been
// closed, tracking quoting and further nesting on the way. It is the scanning
// rule scanParens uses to find its own `)`, factored out for the skippers,
// and it stops at end of input rather than complaining: whoever called it is
// inside a construct of its own and already has the unterminated one to
// report.
//
// It does not call skipSubstitution on the way, and that is not an omission:
// a nested `$( )` reached from here is already counted correctly — the `$` is
// an ordinary byte and the parentheses balance — and `$(( ))` for the same
// reason, its two opening parens counted the same as its two closing ones.
// Recurring was tried and every mutant of it read the same, in all four
// dialects and across the corpus, because it can only arrive at the state
// counting arrives at.
func (l *Lexer) skipToDepth(depth int) bool {
	for depth > 0 && !l.eof() {
		switch c := l.peek(); c {
		case '\'':
			l.skipQuoted('\'', false)
		case '"':
			l.skipQuoted('"', true)
		case '`':
			l.skipBackticks()
		case '\\':
			l.noteContinuation()
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '(':
			depth++
			l.advance()
		case ')':
			depth--
			l.advance()
		default:
			l.advance()
		}
	}
	return depth == 0
}

func (l *Lexer) skipBackticks() {
	open := l.pos()
	l.advance()
	for !l.eof() {
		switch l.peek() {
		case '\\':
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '`':
			l.advance()
			return
		default:
			l.advance()
		}
	}
	// Ran out rather than finding the closing mark. The dialect that ends an
	// unterminated quote at end of input and runs is asked here for the same
	// reason scanBackticks asks it: there is nothing unfinished there.
	if !l.closesQuotesAtEOF() {
		l.failUnmatched(open, "`", "`", "unterminated backquote substitution")
	}
}

// peekIsArithCommand reports whether an arithmetic command begins here.
func (l *Lexer) peekIsArithCommand() bool {
	i := l.off
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	return l.dialect.ArithCommand && i+1 < len(l.src) && l.src[i] == '(' && l.src[i+1] == '('
}

// scanArithCommand reads `(( expr ))` used as a command.
//
// The expression is kept as raw text rather than tokenized, because what is
// inside is an arithmetic expression and not a command list: `(( 2 > 1 ))`
// compares, and lexing that `>` as a redirection would lose the program. The
// operator set inside is a separate specification, so nothing here interprets
// it.
//
// The closing `))` is found the same way substitutions find theirs — tracking
// quoting and nesting rather than counting — so `(( (1+2)*3 ))` works.
//
// It reports whether the arithmetic reading held. `((` is ambiguous at the
// start of a command: it opens an arithmetic command, or it is a subshell
// whose first command is itself a subshell, and nothing but the text ahead
// says which. The two closing parentheses have to be **adjacent** and stand
// where the expression's own nesting is closed; a `)` that arrives anywhere
// else is a grouping paren and not this construct's. Measured 2026-09-15 and
// unanimous in bash 5.3.20, bash 3.2.57, bash-as-sh, zsh 5.9.2 and ksh93u+:
//
//	((echo a); echo b)      a b        — the `)` is followed by `;`
//	((echo a) )             a          — the two closers are not adjacent
//	(( (echo a) ))          arithmetic — they are, so the expression is read
//	((echo a))              arithmetic — and a bad expression is a *run-time*
//	                                     error, which `echo B; ((echo a)); echo A`
//	                                     shows by printing B and A around it
//
// A failed reading says so rather than reporting: the caller puts the lexer
// back and the `(` is read as a grouping paren instead. Running out of input
// is still a refusal — every shell on the panel refuses `((1+1` and
// `((echo a` alike — so the diagnostic below stays where it is and the
// prompt still asks for the rest of an unfinished expression (#3052).
func (l *Lexer) scanArithCommand(start Pos) (Token, bool) {
	l.advance() // (
	l.advance() // (
	// The depth the *expression* is nested to, so zero is where its own
	// closing parenthesis stands. The two opening parens are not counted
	// because they are not the expression's.
	depth := 0
	exprStart := l.off
	exprEnd := -1
	joined := l.collectContinuations()

	for exprEnd < 0 {
		if l.eof() {
			l.ranOut("$((")
			l.fail(start, "unterminated arithmetic command")
			exprEnd = l.off
			break
		}
		switch l.peek() {
		case '\'', '"':
			// A quoted run is stepped over whole, so a `)` inside it closes
			// nothing — unless the dialect's scan is blind to quoting, where
			// the quote is an ordinary character and the `)` behind it is
			// counted like any other. See
			// Dialect.ArithCommandScanIgnoresQuoting for the rows.
			if l.dialect.ArithCommandScanIgnoresQuoting {
				l.advance()
				continue
			}
			l.skipQuoted(l.peek(), l.peek() == '"')
		case '\\':
			l.noteContinuation()
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case '$':
			// A command substitution holds a *program*, so where it ends is
			// a question about the grammar and not about how many
			// parentheses have been seen — the same rule, and the same
			// reasoning, as scanParens above. A `case` arm's pattern ends in
			// a `)` that closes nothing, so counting stopped an arm early
			// and left the rest of the arithmetic looking like a stray
			// closer: `(( $(case q in q) echo 7;; esac) ))` is refused here
			// and runs in bash 5.3.15, ksh93u+ and zsh 5.9.2. Where the
			// parser cannot read it — half a line at a prompt — the counting
			// below is still the answer, which is why this falls through
			// rather than reporting.
			//
			// `$((` is left to the counting deliberately: it is an
			// expression rather than a program, and its two parentheses
			// balance against its two closers on their own.
			if l.peekAt(1) == '(' && l.peekAt(2) != '(' {
				if end, remarks, ok := l.parseToClose(l.off + 2); ok {
					l.remarks = append(l.remarks, remarks...)
					for l.off <= end {
						l.advance()
					}
					continue
				}
			}
			if l.peekAt(1) == '{' {
				// A `${ … }` ends at *its* brace and holds no parenthesis
				// this loop has any business counting — which matters most
				// for the form whose body is a program, where a `case` arm's
				// `)` would otherwise close a level nothing opened:
				// `for (( ${ case q in q) true;; esac; };; ))` is refused
				// here and runs in bash 5.3.15. scanBraces is the scanner
				// that knows
				// where one ends, comments and nesting and all, and its
				// span is discarded because this token keeps its text
				// whole.
				l.scanBraces(Unquoted)
				continue
			}
			l.advance()
		case '(':
			depth++
			l.advance()
		case ')':
			if depth > 0 {
				depth--
				l.advance()
				continue
			}
			// The expression's own nesting is closed, so this `)` is where
			// the construct ends — if the next character closes it too.
			// Anything else and there was never an arithmetic command here:
			// `((echo a); echo b)` is two groupings, and every shell on the
			// panel runs it as one.
			if l.peekAt(1) != ')' {
				joined(exprStart, exprStart)
				return Token{}, false
			}
			exprEnd = l.off
			l.advance()
			l.advance()
		default:
			l.advance()
		}
	}

	expr := joined(exprStart, exprEnd)
	return Token{
		Kind:  TokArithCmd,
		Pos:   start,
		End:   l.pos(),
		Text:  expr,
		Spans: []Span{{Kind: ArithSubst, Value: expr, Quoting: Unquoted, Pos: start}},
	}, true
}

// peekIsFuncParens reports whether `()` follows the current position with only
// blanks between, which is how a POSIX function definition announces itself.
//
// The parser needs this because `name` and `name()` are indistinguishable
// until the paren: a word at command position is a command name right up to
// the point where it is a function being defined.
// peekIsLeftParen reports whether the next thing is a `(`, whatever follows
// it. Two dialects commit to a function definition there and complain about
// what they find next; the other two never get that far.
func (l *Lexer) peekIsLeftParen() bool {
	i := l.off
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	return i < len(l.src) && l.src[i] == '('
}

// peekIsRightParen reports whether the next thing in the input is a `)`,
// blanks aside. Used where the `(` is already the current token, which is
// what tells an anonymous function's empty parameter list from a subshell.
func (l *Lexer) peekIsRightParen() bool {
	i := l.off
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	return i < len(l.src) && l.src[i] == ')'
}

func (l *Lexer) peekIsFuncParens() bool {
	i := l.off
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	if i >= len(l.src) || l.src[i] != '(' {
		return false
	}
	i++
	for i < len(l.src) && isBlank(l.src[i]) {
		i++
	}
	return i < len(l.src) && l.src[i] == ')'
}

// readHeredocs consumes the bodies of every here-document queued on the line
// just ended, in the order their operators appeared.
func (l *Lexer) readHeredocs() {
	queued, quoted := l.pending, l.pendingQuoted
	l.pending, l.pendingQuoted = nil, nil
	for i, r := range queued {
		l.readOneHeredoc(r, quoted[i])
	}
}

func (l *Lexer) readOneHeredoc(r *Redirect, quoted bool) {
	strip := r.Op == TokDLessDash
	delim := r.Word.Literal()
	start := l.pos()
	// Where the last line of the body began, which is where the input ran
	// out as far as the one shell that remarks on this is concerned. Not
	// l.pos() at the end: a body whose last line ends in a newline leaves
	// the lexer at the start of the line *after* it, and the warning names
	// the last line that had something on it. A body with no lines at all
	// names the here-document's own line, which is measured.
	lastLine := r.OpPos
	var body strings.Builder
	for {
		if l.eof() {
			// Reaching the end without the delimiter is unfinished input and
			// not a syntax error: every shell in the panel takes the body as
			// everything to the end and runs the command, one of them with a
			// warning and three in silence.
			//
			// Marked incomplete all the same, because *where* the input ended
			// is a different question at a prompt: a here-document still open
			// when the line ends should ask for another line rather than run
			// with what it has. The parser reports both, and each front end
			// reads the one it needs.
			l.ranOut("<<")
			// Said out loud by one shell and passed over by three, so it is
			// recorded here and worded — or not — by the front end.
			l.remarks = append(l.remarks, Remark{
				Kind:  RemarkHeredocAtEOF,
				Pos:   lastLine,
				At:    r.OpPos,
				Token: delim,
			})
			// The body ran to the end of the input, so the last line there
			// was is the last line this command occupied.
			l.markHeredocEnd(lastLine)
			// And the body may not end in a newline, which no terminated
			// here-document's can — recorded so the interpreter can ask the
			// one question that shape raises. See Redirect.HeredocAtEOF.
			r.HeredocAtEOF = true
			break
		}
		linePos := l.pos()
		if l.delimiterClosesParens(delim, strip) {
			// The delimiter, then the `)` of the parentheses this body sits
			// in. The body ends here and the rest of the line — the `)`
			// first — is program text again, so the cursor stops in front of
			// it rather than at the next line.
			//
			// The one shell that remarks on this says the document was
			// delimited by end of file, and names this line: from the
			// document's own point of view the line matched nothing and the
			// parentheses' text ran out under it.
			if strip {
				for !l.eof() && l.peek() == '\t' {
					l.advance()
				}
			}
			for range len(delim) {
				l.advance()
			}
			l.remarks = append(l.remarks, Remark{
				Kind:  RemarkHeredocAtEOF,
				Pos:   linePos,
				At:    r.OpPos,
				Token: delim,
			})
			l.markHeredocEnd(linePos)
			break
		}
		line, done, join := l.heredocLine(strip, !quoted)
		if l.delimiterMatches(line, done, delim, strip, join) {
			// The delimiter's own line is the command's last, and it is not
			// part of the body.
			l.markHeredocEnd(linePos)
			break
		}
		lastLine = linePos
		body.WriteString(line)
	}

	q := Unquoted
	if quoted {
		q = SingleQuoted
	}
	// The body is kept raw. An unquoted body is subject to expansion, which
	// re-reads it later — the same treatment $( ) and ${ } get, and for the
	// same reason: what is inside is not this stage's to interpret.
	r.Heredoc = &Word{
		Spans: []Span{{Kind: Literal, Value: body.String(), Quoting: q, Pos: start}},
		Start: start,
		Stop:  l.pos(),
	}
}

// delimiterMatches reports whether a body line just read ends the document.
//
// line is the line as written — tabs already stripped from it where the
// operator strips them — and done is what the delimiter is compared against.
// Where the operator is `<<-` the panel parts three ways over tabs the
// *delimiter* was written with; see [Dialect.StrippedHeredocDelimiter].
func (l *Lexer) delimiterMatches(line, done, delim string, strip bool, join heredocJoin) bool {
	if !l.delimiterIsReachable(join) {
		return false
	}
	if done == delim {
		return true
	}
	if !strip || !strings.HasPrefix(delim, "\t") {
		return false
	}
	switch l.dialect.StrippedHeredocDelimiter {
	case HeredocDelimiterTabsAreStrippedToo:
		return done == strings.TrimLeft(delim, "\t")
	case HeredocLineAsWrittenMeetsTheDelimiter:
		return join == heredocOnePhysicalLine && l.lineAsWritten(line) == delim
	}
	return false
}

// lineAsWritten is the body line just read, before its tabs were stripped
// and without its newline: the text between the start of the line the
// cursor has just left and that line's end.
func (l *Lexer) lineAsWritten(stripped string) string {
	end := l.off
	if end > 0 && l.src[end-1] == '\n' {
		end--
	}
	begin := end - len(strings.TrimSuffix(stripped, "\n"))
	for begin > 0 && l.src[begin-1] == '\t' {
		begin--
	}
	return l.src[begin:end]
}

// delimiterClosesParens reports whether the line at the cursor is the
// delimiter with the closing parenthesis of the construct around it written
// straight after it — `EOF)` — in a dialect that ends a body there.
//
// Asked before the line is read, because the answer decides where reading
// stops: in front of the `)`, which is the construct's and not the body's.
// That is the difference from the reading [Lexer.takeRemarks] makes after the
// fact. There the parentheses were found by counting, which only happens when
// nothing further down could end the body; here the body ends at this line
// even when a line reading exactly `EOF` comes later, which is what bash 5.3
// and ksh93 do and what reading the body from the whole input got wrong.
func (l *Lexer) delimiterClosesParens(delim string, strip bool) bool {
	if !l.inProgramParens || !l.dialect.HeredocEndsAtClosingParen || delim == "" {
		return false
	}
	i := l.off
	if strip {
		for i < len(l.src) && l.src[i] == '\t' {
			i++
		}
	}
	rest := l.src[i:]
	return strings.HasPrefix(rest, delim) && strings.HasPrefix(rest[len(delim):], ")")
}

// markHeredocEnd records how far a here-document reached, keeping the furthest
// of several on one line: `cat <<A <<B` is closed by B's delimiter and not by
// A's, and they are read in the order their operators appeared.
func (l *Lexer) markHeredocEnd(at Pos) {
	if at.Offset > l.heredocEnd.Offset {
		l.heredocEnd = at
	}
}

// heredocJoin says what a body line's backslash-newlines did to it, which is
// the question [Dialect.HeredocDelimiterAcrossAContinuation] is asked about.
type heredocJoin uint8

const (
	// heredocOnePhysicalLine is a line that took no continuation at all.
	// Every dialect compares it against the delimiter.
	heredocOnePhysicalLine heredocJoin = iota
	// heredocJoinedFromTheStart is a line whose every continuation stood
	// before any text of it, so what the delimiter would be compared against
	// began at the start of a physical line all the same.
	heredocJoinedFromTheStart
	// heredocJoinedAfterText is a line that continued after text, so the
	// delimiter would have to be recognized in the middle of what was
	// written as a line.
	heredocJoinedAfterText
)

// delimiterIsReachable reports whether a line joined this way is one this
// dialect will compare against the delimiter at all.
func (l *Lexer) delimiterIsReachable(j heredocJoin) bool {
	switch j {
	case heredocOnePhysicalLine:
		return true
	case heredocJoinedFromTheStart:
		return l.dialect.HeredocDelimiterAcrossAContinuation >= HeredocDelimiterAfterALeadingContinuation
	default:
		return l.dialect.HeredocDelimiterAcrossAContinuation == HeredocDelimiterOnTheJoinedLine
	}
}

// heredocLine reads one line of a here-document body. It returns the line as
// written, including its newline and any continuations inside it; the line's
// content with those continuations removed and tabs stripped when the
// operator asked for it, so the caller can compare that against the
// delimiter; and what the continuations did to it.
//
// `join` is off for a quoted delimiter, whose body is literal throughout —
// there a backslash before a newline is two ordinary characters, which is
// what makes `<<'EOF'` and `<<\EOF` the control for all of this.
//
// The line is returned **raw**, with the backslash and the newline still in
// it, because the body is kept raw: an unquoted body is re-read at expansion
// time and [Lexer.heredocSpans] already removes a continuation there, the
// same treatment the escapes beside it get. Removing it here as well would
// remove it twice.
//
// Tabs are stripped from the start of the line *as written*, which is its
// first physical line, and not from what a continuation brings into it —
// `<<-EOF` over `→A\` and `→B` is `A→B` in every column of the panel, so the
// tab the second line opens with survives the join.
func (l *Lexer) heredocLine(strip, join bool) (line, content string, j heredocJoin) {
	begin := l.off
	var joined strings.Builder
	for {
		from := l.off
		for !l.eof() && l.peek() != '\n' {
			l.advance()
		}
		text := l.src[from:l.off]
		if strip && from == begin {
			// Tabs only. Spaces are not stripped, which is why a delimiter
			// indented with spaces never matches.
			text = strings.TrimLeft(text, "\t")
		}
		if !join || l.eof() || !endsInAnOddBackslashRun(text) {
			joined.WriteString(text)
			if !l.eof() {
				l.advance() // the newline
			}
			break
		}
		if j != heredocJoinedAfterText {
			if joined.Len() == 0 && len(text) == 1 {
				j = heredocJoinedFromTheStart
			} else {
				j = heredocJoinedAfterText
			}
		}
		// The backslash escapes the newline, so neither is content and the
		// line goes on into the one below it.
		joined.WriteString(text[:len(text)-1])
		l.advance() // the newline
	}
	line = l.src[begin:l.off]
	if strip {
		line = strings.TrimLeft(line, "\t")
	}
	return line, joined.String(), j
}

// endsInAnOddBackslashRun reports whether the backslashes ending s leave one
// of them free to escape whatever comes next.
//
// Parity rather than "the last character is a backslash", because a backslash
// escapes a backslash: `A\\` ends a line in every column of the panel and
// `A\\\` continues it.
func endsInAnOddBackslashRun(s string) bool {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// bareParamSpecials are the one-character parameters that may follow a `$`
// without braces.
const bareParamSpecials = "@*#?-$!"

// isBareParam reports whether c can begin a `$name` expansion written without
// braces. `$x` and `${x}` mean the same thing, and the short form is by far
// the commoner one, so the lexer has to produce the same span for both.
func isBareParam(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || strings.IndexByte(bareParamSpecials, c) >= 0
}

// scanBareParam reads `$name`, `$1` or `$?` and friends.
//
// A digit is a *single* positional parameter here — `$12` is `$1` followed by
// the character 2, which is why the multi-digit form needs braces — unless the
// dialect has MultiDigitPositional, where the whole run is one parameter and
// `$12` is the twelfth. A special character is exactly one either way.
func (l *Lexer) scanBareParam(q Quoting) []Span {
	open := l.pos()
	l.advance() // $
	l.takeContinuationAfterADollar(q)
	begin := l.off
	if l.dialect.BareSubscript && l.peek() == '#' && isBareLengthTarget(l.peekAt(1)) {
		// `$#name` is that parameter's length, so the `#` is an operator here
		// and the parameter is what follows it. Without the flag — and with
		// it, where nothing a length can be taken of follows — the `#` is
		// itself the parameter, and the rest of the word is literal text.
		l.advance()
	}
	if l.dialect.BareParamFlags && bareFlagApplies(l.peek(), l.peekAt(1)) {
		// A flag character between the `$` and the name, which is the same
		// flag the braced spelling writes inside the group: `$=v` is `${=v}`
		// and `$+v` is `${+v}`. The sigil is consumed here so that the
		// parameter is what follows it, exactly as the `#` above is.
		l.advance()
	}
	switch c := l.peek(); {
	case c >= '0' && c <= '9':
		l.advance()
		if l.dialect.MultiDigitPositional {
			// The rest of the run, where the dialect reads it as one
			// parameter. The digits are kept exactly as written and the
			// number is read by whoever resolves the name, which is what
			// makes `$00` the shell's own name and `$010` the tenth
			// parameter without a rule here about leading zeros.
			for !l.eof() && l.peek() >= '0' && l.peek() <= '9' {
				l.advance()
			}
		}
	case strings.IndexByte(bareParamSpecials, c) >= 0:
		l.advance()
	default:
		for !l.eof() {
			c := l.peek()
			ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9' && l.off > begin)
			if !ok {
				break
			}
			l.advance()
		}
	}
	// Where the name ends, kept because the scan below advances the cursor
	// in both of its outcomes and the expansion's Value is only the first
	// of them: a subscript the scan *took* belongs to the expansion, and a
	// bracket run it gave back and kept as text does not.
	name := l.src[begin:l.off]
	kept, hasKept := l.bareSubscript(name, q)
	if hasKept {
		// The brackets were given back as text, so anything after them is
		// text too: a modifier list belongs to an expansion, and what stands
		// in front of these characters is a name followed by a `[`.
		return []Span{
			{Kind: ParamExp, Value: name, Quoting: q, Pos: open, Bare: true},
			kept,
		}
	}
	l.bareModifiers()
	return []Span{{Kind: ParamExp, Value: l.src[begin:l.off], Quoting: q, Pos: open, Bare: true}}
}

// bareModifiers takes a `:h:t:s/a/b/` run written straight after an unbraced
// expansion into that expansion, and takes nothing where the colon does not
// begin one.
//
// Into the span's text rather than onto a field of its own, because the
// braced spelling of the same list is already a ParamSubstring whose operand
// is that text — `${p:t}` and `$p:t` are one expansion written two ways, and
// the parser reads the second by being handed `p:t`. See
// Dialect.BareParamModifiers for what the two spellings do differ about.
//
// **The letter and nothing after it**, which is the difference: `$p:h2` is
// the head with a literal `2` after it where `${p:h2}` is the head twice. So
// a segment here is a colon and one letter, except `s`, which is a delimited
// substitution and is taken whole.
//
// A colon that begins nothing is left where it is, with no complaint —
// `$s:zz` is the value and four characters, measured — which is why this
// scans on a copy of the cursor and commits a segment at a time.
func (l *Lexer) bareModifiers() {
	if !l.dialect.BareParamModifiers {
		return
	}
	for {
		next, ok := bareModifierSegment(l.src[l.off:])
		if !ok {
			return
		}
		for range next {
			l.advance()
		}
	}
}

// bareModifierSegment measures one `:` segment at the front of src, and
// reports how many bytes it takes.
func bareModifierSegment(src string) (int, bool) {
	if len(src) < 2 || src[0] != ':' {
		return 0, false
	}
	letter := src[1]
	if _, known := ModifierLetters[letter]; !known {
		return 0, false
	}
	if letter != 's' {
		return 2, true
	}
	// `:s` carries a delimited pattern and replacement, whose delimiter is
	// whatever byte follows the letter. Measured: the closing delimiter may
	// be left off at the end of the word — `$p:s/a/Z` substitutes — so the
	// scan takes what is there rather than requiring three.
	if len(src) < 3 {
		return 0, false
	}
	delim := src[2]
	seen := 0
	for i := 3; i < len(src); i++ {
		switch {
		case src[i] == '\\' && i+1 < len(src):
			// A backslash protects the delimiter, which is what keeps
			// `$p:s/\//:/` one segment. What the escape then *means* is the
			// modifier's — see interp/modifier.go.
			i++
		case src[i] == delim:
			seen++
			if seen == 2 {
				return i + 1, true
			}
		case bareModifierEnds(src[i]):
			// The word ends here whatever this scan wanted, so the
			// substitution is however much of it was written.
			return i, seen > 0
		}
	}
	return len(src), seen > 0
}

// bareModifierEnds reports whether a byte ends the word an unbraced modifier
// list is being read inside.
//
// The blanks and the operators, which is the set that would have ended the
// word anyway: a `:s` whose second delimiter is missing takes the rest of the
// word and not the rest of the line, so `echo $p:s/a/Z; echo done` is two
// commands.
func bareModifierEnds(c byte) bool {
	switch c {
	case ' ', '\t', '\n', ';', '&', '|', '(', ')', '<', '>':
		return true
	}
	return false
}

// startsBareParam reports whether the `$` under the cursor begins a bare
// expansion.
//
// isBareParam alone is not the whole question once a dialect has the flag
// sigils: `#` gets through it by being a special parameter in its own right,
// which is how `$#name` is reached, and none of `+`, `=`, `~` or `^` is one.
// So the gate has to ask the dialect as well, and it has to ask *here* rather
// than inside scanBareParam — a `$` that is not the start of an expansion
// never reaches that function at all.
func (l *Lexer) startsBareParam() bool { return l.startsBareParamAt(1) }

// startsBareParamAt is [Lexer.startsBareParam] with the character the `$`
// introduces read n bytes on, for the caller that has to look past a line
// continuation standing between the two.
func (l *Lexer) startsBareParamAt(n int) bool {
	if isBareParam(l.peekAt(n)) {
		return true
	}
	return l.dialect.BareParamFlags && bareFlagApplies(l.peekAt(n), l.peekAt(n+1))
}

// bareFlagApplies reports whether a `$` is followed by one of zsh's unbraced
// flag sigils and something that sigil can be a flag *on*.
//
// The four are `+`, `=`, `~` and `^`, and they split two ways, measured on
// zsh 5.9.2 with `v=hi; set -- a b`:
//
//	           v      1      @        *        ?      nope
//	`$+`       1      1      `$+@`    `$+*`    `$+?`  0
//	`$=`       hi     a      a b      a b      0      (empty)
//	`$~`       hi     a      a b      a b      0      (empty)
//	`$^`       hi     a      a b      a b      0      (empty)
//
// So `$+` takes a name or a digit and nothing else — the specials leave the
// whole thing as literal text, which is what `$+@` printing itself says —
// while the other three are flags on any parameter at all and take every
// target a bare `$` does. `$++v` is literal too: the sigil does not repeat.
//
// The other three are consumed with no target at all, because zsh expands a
// bare `$=` to *nothing* rather than printing it, which is the empty name
// being flagged. `$+` alone prints `$+`, and that is the difference the
// second half of this function is for.
func bareFlagApplies(flag, next byte) bool {
	switch flag {
	case '=', '~', '^':
		return true
	case '+':
		return isNameStart(next) || (next >= '0' && next <= '9')
	}
	return false
}

// isBareLengthTarget reports whether c may follow the `#` of `$#name`.
//
// A name, a digit, `@` or `*`. Measured on zsh 5.9.2: `$#a` is an array's
// count, `$#0` and `$#1` are the lengths of those parameters, and `$#@` and
// `$#*` are the number of positional parameters — while `$##` prints the
// count and then a `#`, and `$#!` the count and then a `!`, so the two
// specials that would make the inner text ambiguous are also the two the
// shell itself leaves out.
func isBareLengthTarget(c byte) bool {
	return c == '@' || c == '*' || (c >= '0' && c <= '9') || isNameStart(c)
}

// takesBareSubscript reports whether the parameter a bare `$` just named may
// carry a `[ … ]` after it.
//
// name is the inner text scanned so far, which is the parameter with the `#`
// of a length still on the front of it: `$#a[2]` is the length of the second
// element, so the subscript belongs to `a` and is read here.
//
// A name, `@` or `*`. The positional digits are excluded because the shell
// excludes them — `set -- abcd; echo $1[2]` prints `abcd[2]` — and the other
// specials because the span could not record the result: `#` and `!` already
// mean an operator at the front of a `${ … }`, and the rest are single-valued
// parameters no script subscripts.
func takesBareSubscript(name string) bool {
	// The sigil the span may open with is not part of the name. `#` is the
	// length and the other four are zsh's unbraced flags, and every one of
	// them leaves the parameter underneath free to carry a subscript:
	// measured, `a=(x y z); $#a[1]` is 1 — the length of the first element —
	// and `$+a[1]` is 1, where without this the brackets fell out of the
	// expansion and `$+a[1]` printed `1[1]`.
	//
	// Unconditional rather than asked of the dialect, because no name starts
	// with one of these: a dialect without the flags never produces a span
	// that this could trim.
	name = strings.TrimLeft(name, "#+=~^")
	if name == "@" || name == "*" {
		return true
	}
	if name == "" || !isNameStart(name[0]) {
		return false
	}
	return true
}

// bareSubscript reads the `[ … ]` a bare parameter carries where the dialect
// has them, leaving the cursor after the closing bracket.
//
// The brackets are balanced, because a subscript may hold another — `$a[$b[1]]`
// — and the scan gives up where the word would end: an unquoted `[` that never
// closes before the word does is not a subscript at all, and its characters
// stay literal. That is one step short of the shell, which commits to the
// subscript and reports an invalid one at run time; the shape still fails
// either way, and the difference is the wording of a diagnostic for input
// nobody writes.
//
// Where the word ends is the *quoting's* question and not a fixed set of
// characters, which is measured on both sides: inside double quotes a blank
// and a **newline** are ordinary text and `"$a[1\n]"` is the first element,
// while unquoted either one ends the word and the brackets are literal. A
// scan that stopped at every newline would have been wrong for the quoted
// half and was — nothing caught it until the line was mutated away and the
// shell was asked.
func (l *Lexer) bareSubscript(name string, q Quoting) (Span, bool) {
	if !l.dialect.BareSubscript || l.peek() != '[' || !takesBareSubscript(name) {
		return Span{}, false
	}
	depth := 0
	// A subscript may open with a flag group of its own, and the `(` that
	// opens one would otherwise end the word: `$a[(r)b]` was a syntax error
	// naming the parenthesis, which is worse than a wrong answer because it
	// takes the whole file with it. The group is stepped over as a unit, so
	// only a group the grammar can actually read is protected — anything
	// else still ends the word exactly where it did.
	past := -1
	if l.dialect.ArraySubscriptFlags {
		if _, rest, isGroup := scanSubscriptFlags(l.src[l.off+1:]); isGroup {
			past = len(l.src) - len(rest)
		}
	}
	// subEnd is where a substitution the scan is standing inside ends. Its
	// characters do not end the *word*: the `(` of `$a[$((n))]` and of
	// `$a[$(echo 3)]` is a word end everywhere else, so meeting it here gave
	// the subscript up and left `[$((n))]` as literal text beside the whole
	// array. Measured on zsh 5.9.2, `ar=(one two three); n=3`:
	// `x=$ar[$((n))]` is `three`, where this shell answered
	// `one two three[3]`. The quoted spelling was unaffected, which is why
	// it went unseen — and powerlevel10k reads its saved prompt out of an
	// associative array with exactly this shape (#2048).
	//
	// A substitution suspends the word-end test and **not** the bracket
	// count, which is measured rather than assumed and is the half a
	// wholesale skip would get wrong: `x=$a[$(echo 2; : [)]` leaves the
	// brackets as text in zsh 5.9.2 where `x=$a[$(echo 2; : [ ])]` is the
	// second element, so a `[` written inside still has to be closed.
	//
	// Where it ends is asked of the lexer's own skip rather than of a
	// counter beside it. A sub-lexer is what carries the cursor, since this
	// scan has not committed to the subscript yet and must be able to give
	// the characters back.
	subEnd := -1
	sawSub := false
	for i := l.off; i < len(l.src); i++ {
		if i > l.off && i < past {
			continue
		}
		c := l.src[i]
		if i >= subEnd && (c == '`' || (c == '$' && i+1 < len(l.src) && l.src[i+1] == '(')) {
			sub := NewLexer(l.src[i:], l.dialect)
			if sub.skipSubstitution() && sub.Err() == nil {
				subEnd = i + sub.off
				sawSub = true
			}
		}
		switch {
		case c == '[':
			depth++
		case c == ']':
			if depth--; depth == 0 {
				if i < subEnd {
					// The bracket that balanced it was written *inside* the
					// substitution, so what stands between the brackets is
					// half a substitution and no subscript at all. zsh gives
					// the characters back here too: `x=$a[$(: ]; echo 2)]`
					// leaves them as text.
					return l.keptBareSubscript(q, sawSub)
				}
				for l.off <= i {
					l.advance()
				}
				return Span{}, false
			}
		case i < subEnd:
		case q == DoubleQuoted && c == '"':
			return l.keptBareSubscript(q, sawSub)
		case q != DoubleQuoted && l.isWordEnd(c):
			return l.keptBareSubscript(q, sawSub)
		}
	}
	return l.keptBareSubscript(q, sawSub)
}

// keptBareSubscript is what the brackets are once the scan above has given
// them back, and it is not "the rest of the word".
//
// Measured on zsh 5.9.2, 2026-09-14, `setopt noglob` so a pattern matching
// nothing cannot be what is reported, `a=(xx yy zz)`:
//
//	$a[$(: ]; echo 2)]             xx yy zz[$(: ]; echo 2)]
//	$a[$(echo 2; : [)]             xx yy zz[$(echo 2; : [)]
//	$a[$(: ]; echo 2)]$(echo Q)    xx yy zz[$(: ]; echo 2)]Q
//	$a[$(: ]; echo *)]             xx yy zz[$(: ]; echo *)]
//	$a[$(: ]; echo 2)]*            no matches found: …]*
//	$a[$(: ]; echo 2)              invalid subscript
//	$a[$(: ]; echo 2)x             invalid subscript
//
// So the characters the subscript scan gave back are **kept as written**: the
// substitution between them is never run, the `*` between them is not a
// pattern, and the spaces between them do not split a field. What stands
// *behind* the closing bracket is a word like any other — the third row runs
// its substitution and the fifth matches with its `*` — so what is quoted is
// the bracket run and nothing past it (#2786).
//
// The extent is the `]` that closes the `[` with every substitution stepped
// over **as a unit**, which is a second scan and not the one above: the scan
// above counts a bracket written inside a substitution, which is what decides
// whether there is a subscript at all, and this one does not, which is what
// decides how far the text runs. The last two rows are the other end of it —
// no such bracket, and the word is refused rather than kept, which the same
// scan answers for the parser — see bareSubscriptClose.
//
// Only where a substitution was in the way, and only unquoted. A `[` a word
// simply never closed is the refusal #1757 records, and the double-quoted
// spelling is a different question this shell already answers differently.
func (l *Lexer) keptBareSubscript(q Quoting, sawSub bool) (Span, bool) {
	if !sawSub || q != Unquoted {
		return Span{}, false
	}
	at := bareSubscriptClose(l.src[l.off:], l.dialect, l.isWordEnd)
	if at < 0 {
		return Span{}, false
	}
	open, start := l.pos(), l.off
	for l.off <= start+at {
		l.advance()
	}
	// SingleQuoted, which is what the measurements say: the `*` between the
	// brackets is a character and the blanks there do not split.
	return Span{
		Kind: Literal, Quoting: SingleQuoted,
		Value: l.src[start : start+at+1], Pos: open,
	}, true
}

// bareSubscriptClose finds the `]` that closes the `[` s opens with, with
// each `$( )` stepped over as a unit, or -1. ends reports a byte that ends
// the word, for the caller whose text runs past the word's end.
//
// One function for two callers, which is the point rather than an economy:
// the lexer asks it how far the kept run reaches and the parser asks it
// whether the subscript is closed at all, and a rule answered in two places
// is a rule that drifts. A plain search for a `]` answers the parser's
// question for every shape but one — `$a[$(: ]; echo 2)` holds a `]` that
// closes nothing, and zsh refuses that word where it keeps
// `$a[$(: ]; echo 2)]` as text.
//
// A backtick ends it rather than being stepped over, and the measurement is
// what says so rather than the shape of the code: on zsh 5.9.2, 2026-09-14,
// a subscript whose only `]` is inside backticks is `invalid subscript`
// where the `$( )` spelling of the same word is kept. A backtick written
// *inside* a `$( )` is a different matter and never reaches here, since the
// group around it is stepped over whole.
func bareSubscriptClose(s string, d Dialect, ends func(byte) bool) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '`' {
			return -1
		}
		if c == '$' && i+1 < len(s) && s[i+1] == '(' {
			sub := NewLexer(s[i:], d)
			if sub.skipSubstitution() && sub.Err() == nil {
				i += sub.off - 1
				continue
			}
		}
		switch {
		case c == '[':
			depth++
		case c == ']':
			if depth--; depth == 0 {
				return i
			}
		case ends != nil && ends(c):
			return -1
		}
	}
	return -1
}

// openingOf is how a span's kind is written, for saying what is unfinished.
//
// A kind describes itself in words for a diagnostic — "command substitution"
// — and a caller drawing what is still open wants the characters that opened
// it instead.
//
// Only the kinds that reach here have a case. A parameter expansion runs out
// in a scanner of its own and names itself there, and a branch for it here
// was dead: a mutation of it changed nothing, which is how it was found.
func openingOf(kind SpanKind) string {
	switch kind {
	case ArithSubst:
		return "$(("
	case CommandSubst:
		return "$("
	case ProcSubstIn:
		return "<("
	case ProcSubstOut:
		return ">("
	case ProcSubstFile:
		return "=("
	}
	return kind.String()
}

// closingOf is the delimiter that never came, for the dialect whose sentence
// names it rather than the opener.
//
// One parenthesis for every kind that reaches here, the arithmetic one
// included, and that is measured rather than a simplification: the dialect
// that words this as "looking for matching `)'" says `)` for `$((1+2` and not
// `))`, so echoing the two characters the construct was opened with would be
// wrong. The bracketed spelling is the exception and does not come through
// here — scanBracket names its own `]`, which is what that dialect prints
// for it.
func closingOf(kind SpanKind) string {
	switch kind {
	case ArithSubst, CommandSubst, ProcSubstIn, ProcSubstOut, ProcSubstFile:
		return ")"
	}
	return kind.String()
}

// braceNameStop reads the body of an unterminated `${…}` — everything after
// the brace, which is everything left of the input — and reports the token
// that stood where the parameter form could read no further, together with
// whether a prefix stood in front of the name.
//
// Empty where the body ran out mid-name at the true end of the input, and
// empty once an operator has been read, because what follows an operator is a
// word and a word may hold anything. See Error.BraceNameStop for the panel
// this answers for.
func braceNameStop(body string) (stop string, prefixed bool) {
	i := 0
	// `#`, `##` and `!` in front of a name are operators on it; the same
	// characters standing alone are parameters in their own right — `${#}`
	// is the count of the positional parameters — so the run is counted
	// first and its last character handed back to the name when the run has
	// nothing to operate on.
	for i < len(body) && (body[i] == '#' || body[i] == '!') {
		i++
	}
	if i > 0 && braceName(body[i:]) == "" {
		i--
	}
	prefixed = i > 0
	i += len(braceName(body[i:]))
	if i < len(body) && body[i] == '@' {
		// An operator letter this expansion has not read yet: `${x@` is
		// still reading, where `${x:-` has read its operator and is reading
		// a word.
		i, prefixed = i+1, true
	}
	if i >= len(body) {
		return "", prefixed
	}
	switch body[i] {
	case '\n':
		return "newline", prefixed
	case ' ', '\t':
		return string(body[i]), prefixed
	}
	return "", prefixed
}

// quoteProtectsTheBrace reports whether a single quote standing at the end of
// body — the text an expansion has read since its own `${` — quotes what
// follows it, so that a `}` inside the quotes does not close the expansion.
//
// The caller has already settled the two readings that are unanimous: a quote
// written outside double quotes protects in every column, and the `${ cmd;}`
// command form holds a program whose quotes are the program's.
func (l *Lexer) quoteProtectsTheBrace(body string) bool {
	switch l.dialect.QuoteProtectsTheClosingBrace {
	case BraceQuoteProtectsEveryOperand:
		return true
	case BraceQuoteProtectsNothing:
		return false
	}
	return l.braceOperandIsAPattern(body)
}

// braceOperandIsAPattern reports whether what an expansion has read of its own
// body so far leaves the scan inside a *pattern* operand rather than a word
// one — which is to say, whether the operator after the parameter name is one
// of `#`, `##`, `%`, `%%` or `/`.
//
// The distinction is the panel's, not an invention: a word operand is read in
// the quoting that encloses the whole expansion, where a single quote stands
// for itself, and a pattern is read on its own terms. `${x:1:2}` is on the
// word side of it, measured in the one column that answers rather than
// refusing the arithmetic.
func (l *Lexer) braceOperandIsAPattern(body string) bool {
	i := 0
	// The same prefix run braceNameStop counts, and for the same reason:
	// `${#x}` operates on a name and `${#}` is a parameter in its own right,
	// so the last character of the run goes back to the name when the run has
	// nothing to operate on.
	for i < len(body) && (body[i] == '#' || body[i] == '!') {
		i++
	}
	if i > 0 && braceName(body[i:]) == "" {
		i--
	}
	i += len(braceName(body[i:]))
	// A subscript belongs to the name: `${a[1]#p}` operates on one element,
	// and the operator is what follows the `]`.
	if i < len(body) && body[i] == '[' {
		if j := strings.IndexByte(body[i:], ']'); j >= 0 {
			i += j + 1
		}
	}
	if i >= len(body) {
		return false
	}
	switch body[i] {
	case '#', '%':
		return true
	case '/':
		// A dialect without the form has no `/` operator, so nothing there
		// introduces a pattern. dash is the panel member that shows it.
		return l.dialect.ParamSubstitution
	}
	return false
}

// braceName is the parameter name at the front of s: a run of name characters
// or one of the single characters that names a parameter on its own.
func braceName(s string) string {
	if s == "" {
		return ""
	}
	if isNameStart(s[0]) || (s[0] >= '0' && s[0] <= '9') {
		i := 0
		for i < len(s) && (isNameStart(s[i]) || (s[i] >= '0' && s[i] <= '9')) {
			i++
		}
		return s[:i]
	}
	switch s[0] {
	case '@', '*', '?', '$', '!', '-', '#':
		return s[:1]
	}
	return ""
}
