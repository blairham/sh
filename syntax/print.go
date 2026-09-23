// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// Printing a tree back as source.
//
// Two features want this and neither can be built without it: `type f` shows
// a function's body, and `export -f` carries one through the environment. A
// shell that has read a function has to be able to say it again.
//
// The promise here is the tree and not the text. What comes back parses to
// the same tree as what went in — it is not the original spelling, because
// the tree does not hold one: `$x` and `${x}` are one node, and so are
// `if a; then b; fi` and the same thing over four lines. Anything wanting a
// particular *layout* asks for it; anything wanting the original text should
// keep the text, as a background statement and a redirection target already
// do.
//
// Most of the work is already done by the parser, which keeps the inside of
// every expansion as it was written: a `${…}` span holds `u:-a b`, a `$(…)`
// span holds its script, and an arithmetic command holds its expression. So
// this is mostly re-emission with the punctuation put back, and the only real
// decisions are about quoting and about where a statement ends.

// Print renders a parsed file as source that parses to the same tree.
func Print(f *File) string { return PrintFileWith(f, Layout{}) }

// PrintFileWith renders a parsed file with a chosen arrangement, which is what
// [PrintWith] is to [PrintCommand].
//
// A whole file rather than one command, because the thing a prompt loop has in
// hand is a *line*, and a line can hold several statements. `true;false` typed
// at a zsh prompt reaches `preexec` as `true; false` in its second argument
// and as two lines in its third, from one input — so both forms are a file
// printed two ways, and neither is a command printed at all.
func PrintFileWith(f *File, l Layout) string {
	if f == nil {
		return ""
	}
	p := printer{layout: l, carried: f.CarriedHeredocs}
	if l.FileFollowsTheSourceUnits {
		p.units(f.Stmts)
	} else {
		p.lines(f.Stmts)
	}
	if l.TrailingBlankLine {
		p.str("\n")
	}
	return p.b.String()
}

// TranslatedString is one `$"…"` the program was written with: the text
// between its quotes, and the line it stands on.
//
// The text is what the printer would write back inside the quotes, which is
// the same thing a reader of the script sees — escapes as written, expansions
// as written, nothing performed. Nothing here translates it and no message
// catalog is consulted; see [Span.Translated].
type TranslatedString struct {
	Text string
	Line int32
}

// TranslatedStrings is every `$"…"` in a file, in the order it was written.
//
// It is the printer walking the tree rather than a walk of its own, and that
// is the whole reason it is here instead of in the caller. A second traversal
// would have to know every node kind, and the one that is missed is the one
// nobody notices — where a printer that failed to reach a node would be
// writing a program back with a piece of it missing, which nothing in this
// tree could keep quiet about.
//
// What it does *not* reach is a substitution's interior: a `$(…)` span holds
// its script as unparsed text, so a `$"…"` written inside one is text here
// rather than a run. That is the same recursion a listing of the span would
// need; see the printer's own note about it.
func TranslatedStrings(f *File) []TranslatedString {
	if f == nil {
		return nil
	}
	var found []TranslatedString
	p := printer{translated: &found}
	p.lines(f.Stmts)
	return found
}

// PrintCommand renders one command, which is what a function's body is.
func PrintCommand(c Command) string { return PrintWith(c, Layout{}) }

// PrintWord renders one word as source that parses to the same word, quotes
// and all.
//
// A diagnostic wants this: a grammar that names the word an unreadable
// expansion sits in has to be able to say the word, and by the time anything
// has failed the word has been taken apart into spans.
func PrintWord(w *Word) string {
	if w == nil {
		return ""
	}
	var p printer
	p.word(w)
	return p.b.String()
}

// PrintWordWith is PrintWord with an arrangement, for the one caller that
// needs a word written some way other than as it stands.
//
// Only the fields a *word* can reach mean anything here, which today is
// [Layout.AnsiCQuotedWordIsItsValue]: a shell's own `set -x` writes a `$'…'`
// element of an array literal as the text it stands for, and the decoder for
// that is the dialect's — see the field.
func PrintWordWith(w *Word, l Layout) string {
	if w == nil {
		return ""
	}
	p := printer{layout: l}
	p.word(w)
	return p.b.String()
}

// PrintArrayElem renders one element of an array literal as it was written —
// an ordinary word, or a nested literal with its parentheses.
func PrintArrayElem(e *ArrayElem) string { return PrintArrayElemWith(e, Layout{}) }

// PrintArrayElemWith is PrintArrayElem with an arrangement, for the reason
// PrintWordWith has one.
func PrintArrayElemWith(e *ArrayElem, l Layout) string {
	if e == nil {
		return ""
	}
	p := printer{layout: l}
	p.arrayElem(e)
	return p.b.String()
}

// PrintWordQuotingRun renders the run of spans around index i that share its
// quoting, *without* the quote characters that surrounded the run.
//
// The run rather than the word, because that is the unit one grammar's
// diagnostics name: `echo 'lit'"${x@QQ}"` blames `${x@QQ}` alone while `echo
// "pre${x@QQ}post"` blames all of `pre${x@QQ}post`. Both are one word, and
// what separates them is that the first changes quoting in the middle.
//
// Literal text goes in as it stands. There are no quotes around the result to
// protect anything from, so escaping it would add characters that were never
// written — `"\"${x@QQ}\""` names `"${x@QQ}"`, measured.
func PrintWordQuotingRun(w *Word, i int) string {
	if w == nil || i < 0 || i >= len(w.Spans) {
		return ""
	}
	q := w.Spans[i].Quoting
	lo, hi := i, i+1
	for lo > 0 && w.Spans[lo-1].Quoting == q {
		lo--
	}
	for hi < len(w.Spans) && w.Spans[hi].Quoting == q {
		hi++
	}
	var p printer
	p.raw = true
	for j := lo; j < hi; j++ {
		if w.Spans[j].Kind == Literal {
			p.str(w.Spans[j].Value)
			continue
		}
		p.withNext(w.Spans, j, func() { p.span(w.Spans[j]) })
	}
	return p.b.String()
}

// Layout is how a printed block is arranged.
//
// Every field is a junction where the shells differ, and the type exists so
// that this package decides none of them. The zero value keeps the source's
// own line structure and adds nothing, which is what a round trip wants; a
// caller that needs a particular shape fills the fields in.
//
// That is the whole point of it being data. An arrangement measured from one
// shell and written into this package would be that shell's taste living
// where nothing is supposed to know a shell — and the differences are not
// small: one puts `then` on the line of its `if` and another on a line of its
// own, one terminates statements with `;` and another with nothing at all.
type Layout struct {
	// Indent is one level of indentation, and Nested repeats it once per
	// enclosing block. Without Nested every block is indented the same,
	// however deep.
	Indent string
	Nested bool
	// Lines puts each statement of a block on a line of its own, whatever
	// the source did. Nothing else here applies without it.
	Lines bool

	// Separator goes after every statement of a block but the last.
	Separator string
	// KeywordTerminator goes after the *last* statement of a body closed by
	// a keyword — `fi`, `done`, `else`. A body closed by a bracket takes
	// nothing, in every shell measured.
	KeywordTerminator string

	// ThenOnItsOwnLine, DoAfterWordsOnItsOwnLine and DoAfterCommandOnItsOwnLine
	// say whether the keyword that opens a body starts a line of its own.
	//
	// Three fields and not one, because a shell may answer them differently:
	// one keeps `then` with its `if` and moves the `do` of a loop over words
	// while keeping the `do` of a loop over a command.
	ThenOnItsOwnLine           bool
	DoAfterWordsOnItsOwnLine   bool
	DoAfterCommandOnItsOwnLine bool

	// BraceOpenSuffix follows the `{` that opens a block — a space, or
	// nothing.
	BraceOpenSuffix string
	// OutermostBraceOpensALine puts the first statement of the outermost
	// block on a line of its own. A block inside one always does.
	OutermostBraceOpensALine bool

	// CaseHeaderSuffix follows the `in` of a `case`.
	CaseHeaderSuffix string
	// CaseArmsOnOneLine keeps a `case` arm's pattern, body and terminator
	// together instead of giving each a line.
	CaseArmsOnOneLine bool
	// CasePatternsParenthesised writes an arm's pattern with the opening
	// parenthesis that the grammar allows and most shells leave out.
	CasePatternsParenthesised bool

	// SubshellBodyOnItsOwnLines gives a `( … )` the shape a brace group
	// gets: the parenthesis ends its line, the statements are indented one
	// level further, and the closing parenthesis starts a line of its own.
	//
	// One engine writes `( exit 1 )` and the other writes the three lines,
	// in the same listing and from the same tree, which is what makes this
	// a field.
	SubshellBodyOnItsOwnLines bool

	// BlankLineAfterAHereDocumentBody writes the newline that opens the next
	// line even where a here-document body has already ended the line,
	// leaving a blank line the source never had.
	//
	// Measured, and not a defect in either engine: one leaves the blank line
	// between the body and whatever follows it — a statement, a `fi`, the
	// `}` that closes the function — and the other writes the next line
	// straight onto the one the delimiter ended.
	BlankLineAfterAHereDocumentBody bool

	// BackgroundKeepsTheLine lets the statement after a `&` follow it on the
	// same line rather than starting one of its own.
	//
	// A `&` is a terminator, so `a & b` is already two statements and an
	// arrangement that puts each on a line of its own is free to break there.
	// One engine does and another does not, which is the whole of why this is
	// a field: `f() { echo bg & ( exit 1 ); echo $?; }` comes back with the
	// subshell on the `&`'s line in one listing and on the next line in the
	// other.
	BackgroundKeepsTheLine bool

	// ElifWrittenAsANestedIf writes an `elif` out as an `else` whose body is
	// an `if` of its own.
	//
	// The two are the same program and are not the same tree, so this is the
	// one field here that a round trip cannot survive — which is what makes
	// it a field rather than the printer's default. Measured: one engine
	// expands every `elif` of a listed body and the other keeps the word, and
	// neither is a taste this package may hold.
	ElifWrittenAsANestedIf bool

	// ParameterBracesAsWritten keeps the braces of a `${x}` that was written
	// with them, instead of dropping the pair a plain name does not need.
	//
	// Off by default because the *word* printer is a diagnostic's, and a
	// diagnostic quotes a redirect target the way every shell does: `$e:
	// ambiguous redirect`, never `${e}`. A listing is the other caller and
	// wants the spelling back — `echo "${x}"` reprinted as `echo "$x"` is a
	// different program the moment the next character continues a name, and
	// a person reading the listing cannot see which of the two they have.
	//
	// [Span.Bare] is what records the spelling; this decides whether the
	// printer reads it.
	ParameterBracesAsWritten bool

	// PipeBothWrittenOut writes the `2>&1` that a `|&` stands for, and a
	// plain `|` after it, rather than the operator the script used.
	//
	// The redirection is in the tree either way — see [Redirect.PipeBoth] —
	// so this decides which of the two the reader is shown. Both engines that
	// say a body back write it out, and one of them cannot read the operator
	// at all: `|&` arrived in bash 4, so a listing that kept it would hand
	// bash 3.2 a syntax error for a function it can run.
	PipeBothWrittenOut bool

	// BodyIsAlwaysBraced writes the command handed to [PrintWith] as a brace
	// group where it is not one already.
	//
	// A function's body is the caller: `f() ( … )` and `f() if …; fi` are
	// declarations both engines that list a body write back with `{ … }`
	// around them, so what a person reads is the shape a listing always has
	// rather than the shape the author wrote. It is a normalization and not a
	// round trip — the braces are a node the source did not have — which is
	// why it is asked for rather than assumed.
	BodyIsAlwaysBraced bool

	// DoAfterArithmeticOnItsOwnLine is the third of the three `do` questions
	// above, for the arithmetic `for`.
	//
	// A third field and not a reuse of either, because one engine answers it
	// differently from both: it keeps `do` on the line of a `while` and of a
	// `for x in …` and gives the arithmetic form's `do` a line of its own.
	// Measured 2026-09-16 on bash 5.3.20, `declare -f` over `f() { for
	// ((i=0;i<2;i++)); do echo $i; done; }` — `for ((i=0; i<2; i++))` then
	// `do` on the next line, where a `for i in a b` in the same listing is
	// `for i in a b` then `do` and a `while` is `while …; do`.
	DoAfterArithmeticOnItsOwnLine bool

	// EmptyArithmeticForExpressionIsOne writes the `1` that a missing
	// expression of an arithmetic `for` stands for, instead of the nothing
	// the source wrote.
	//
	// All three places, and the value is the same in each: `for ((;;))`
	// comes back `for ((1; 1; 1))` and `for ((i=0;;i++))` comes back
	// `for ((i=0; 1; i++))`. Measured 2026-09-16 on bash 5.3.20 through
	// `declare -f`; the engine that lists a body as source text writes the
	// text back and the third writes the empty expressions as they stood.
	//
	// A normalization rather than a round trip — the `1` is not in the tree
	// — which is why it is asked for and not assumed. It is also the
	// program: an omitted condition is true, and `1` is how that is spelled.
	EmptyArithmeticForExpressionIsOne bool

	// ArithmeticForExpressionsAsWritten writes each of the three expressions
	// of an arithmetic `for` with the blanks the script put *after* it,
	// instead of the trimmed text the fields hold.
	//
	// Both engines that list a body keep that half and drop the other:
	// measured 2026-09-16 on bash 5.3.20 through `declare -f` and on zsh
	// 5.9.2 through `functions`, `for (( i=0 ; i < 3 ; i++ ))` comes back
	// `for ((i=0 ; i < 3 ; i++ ))` and `for ((   i=0;i<3;i++   ))` comes back
	// `for ((i=0; i<3; i++   ))`. So the leading blanks go — the `; ` this
	// writes supplies one — and the trailing blanks stay, which is what makes
	// `i++ ))` rather than `i++))`.
	//
	// The parts come from [ForArithClause.PartsAsWritten], which re-splits
	// the header; a tree built by hand has no header and prints as it always
	// did. Off by default, since a formatter wants the blanks normalized and
	// a round trip wants them not doubled.
	ArithmeticForExpressionsAsWritten bool

	// HereDocumentWordSingleQuoted writes a delimiter that carries any
	// quoting as its literal text inside one pair of single quotes,
	// whichever quoting the source used.
	//
	// The quoting of a delimiter means one thing — the body is literal — so
	// a listing that normalizes it loses nothing a reader needs, and the one
	// engine that does it normalizes every spelling: `<<"EOT"`, `<<\EOT`
	// and `<<E"O"T` all come back `<<'EOT'`, and `<<"E T"` comes back
	// `<<'E T'`. An unquoted delimiter stays bare. Measured 2026-09-16 on
	// bash 5.3.20 through `declare -f`.
	HereDocumentWordSingleQuoted bool

	// CoprocessDefaultName is written where a coprocess over a **compound**
	// command was given no name of its own, and is empty where the name is
	// left out.
	//
	// A name may only be written before a compound command — with a simple
	// one the first word is the command — so the two shapes are two
	// constructs and the engine that lists a body writes them apart.
	// Measured 2026-09-22 on bash 5.3.20 through `type`:
	//
	//	coproc ( : )              coproc COPROC ( : )
	//	coproc { :; }             coproc COPROC { … }
	//	coproc while …; done      coproc COPROC while …; done
	//	coproc cat                coproc cat
	//	coproc NM ( : )           coproc NM ( : )
	//
	// The name is not in the tree for the first three: nothing was written
	// there, and what the coprocess ends up called is the runner's answer.
	// So this is a normalization the caller asks for, in the caller's own
	// spelling — a shell whose default is called something else says so
	// here rather than being hard-coded into the printer.
	CoprocessDefaultName string

	// BlankBeforeAWordlessRedirection writes the blank that separates a
	// command from its redirections even where there is no command in front
	// of it — `> out` written back as ` > out`.
	//
	// A command may be nothing but redirections, and then the blank has
	// nothing to separate. The two engines answer it differently and both
	// answers are visible in a listed body, measured 2026-09-22 over
	// `f() { > out; }`: bash writes `> out` and zsh writes ` > out`. It
	// reaches the inside of a substitution too, which is where it costs
	// something a reader notices — `echo $(< x1)` comes back `echo $( < x1)`
	// with the blank and `echo $(< x1)` without it.
	BlankBeforeAWordlessRedirection bool

	// AnsiCQuotedWordIsItsValue writes a `$'…'` as an ordinary single-quoted
	// string holding the characters it stands for, decoded by this function.
	//
	// A function and not a flag, because **what an escape comes to is the
	// dialect's answer** and nothing under syntax may hold one: `\e`, `\E`,
	// `\cX`, `\u` and an unknown escape are each a semantics axis, so the
	// only correct decoder is the caller's. Nil leaves the word spelled as
	// it was written, which is what every caller but one wants — a formatter
	// that rewrote `$'\t'` into a literal tab would be changing the
	// program's text.
	//
	// Measured 2026-09-16 on bash 5.3.20 through `declare -f`: `echo
	// $'a\tb'` comes back as `echo` and the two characters in single
	// quotes, `x=$'\t'` the same, and so do `echo ${x-$'\t'}`, a `case`
	// subject and a `[[ ]]` operand. A quote in the value comes back as the
	// closed-escaped-reopened spelling every shell writes. ksh93 and zsh
	// write the word back as it was written and leave this nil.
	AnsiCQuotedWordIsItsValue func(string) string

	// KeywordBodiesTakeTheirOwnLines lays out every construct a reserved word
	// closes — `fi`, `done`, `esac` — as Lines would, while a list's own
	// statements follow the source's line breaks and a block the brackets
	// close stays where it was written.
	//
	// It is the arrangement one engine reprints the **inside of a command
	// substitution** with, and it is a third answer rather than either of the
	// two above. Measured 2026-09-19 on bash 5.3.20 through `declare -f`:
	// `$(a >/dev/null;b)` comes back `$(a > /dev/null; b)` on one line and
	// `$(echo one` + newline + `echo two)` keeps its newline, which is what
	// Lines cannot do; `$(if true; then echo x; fi)` comes back over three
	// lines with `echo x` indented and `fi` at the margin, which is what
	// leaving Lines off cannot do. A brace group and a subshell stay on the
	// line either way — `$({ a; b; })` and `$( (a; b) )` come back
	// `$({ a; b; })` and `$( ( a; b ))` — so the split is the closing token
	// and not the construct's depth.
	//
	// Meaningless with Lines on, which already breaks everything.
	KeywordBodiesTakeTheirOwnLines bool

	// CommandSubstitutionIsReprinted writes the inside of a `$( … )` back
	// through this function rather than as the source text the span holds.
	//
	// A [Span] of kind [CommandSubst] keeps its body unparsed, so a printer
	// has nothing to lay out and writes the characters back; one engine
	// re-reads the body and prints it through the same listing, so
	// `$(a >/dev/null;b)` comes back `$(a > /dev/null; b)`. Doing that here
	// would mean the printer re-entering the parser with a dialect it does
	// not hold, so the caller supplies the whole step: parse with its own
	// grammar, print with its own arrangement, hand back the text.
	//
	// `false` means the body was not re-read — it does not parse, which is
	// reachable because a body is not read until the substitution runs — and
	// the span is written back exactly as it stands. That is best effort by
	// design: a listing that refused to print a program it can otherwise run
	// would be a worse answer than a body written out as the author typed it.
	//
	// Asked of `$( … )` alone. A backquoted substitution is written back as
	// written — measured on bash 5.3.20, `` `a >/dev/null;b` `` comes back
	// unchanged where the `$( )` spelling beside it is reprinted — and so is
	// a body whose first character is `(`, because the parentheses that come
	// back decide whether the text re-reads as arithmetic: `$((a; b))` is
	// written back as it stands there and `$( (a; b) )` becomes
	// `$( ( a; b ))`.
	//
	// ksh93 and zsh write every substitution back as it was written and leave
	// this nil, as does a formatter, which must not edit what it lays out.
	CommandSubstitutionIsReprinted func(string) (string, bool)

	// ParameterExpansionIsReprinted writes a `${ … }`'s own text back through
	// this function before it is written out.
	//
	// The same shape as CommandSubstitutionIsReprinted and for the same
	// reason one construct over: a [Span] of kind [ParamExp] holds its
	// operand as **text**, so a substitution written inside one — the word a
	// `-` substitutes, the replacement of a `/`, and every other operand
	// whose operand can hold one — is not a tree this printer can reach. The
	// one engine that lays a substitution's body out reaches inside a
	// `${ … }` too: measured 2026-09-20 on bash 5.3.20 through `declare -f`,
	// `echo ${v-$(a;b)}` comes back `echo ${v-$(a; b)}` and
	// `echo ${v-$(a >/dev/null;b)}` comes back
	// `echo ${v-$(a > /dev/null; b)}`, the redirection respaced with the
	// separator.
	//
	// Handed the span's text and not its parts, because what the caller has
	// to do is find the substitutions in it — which needs its own grammar —
	// and reprint them without touching a byte of anything else. `false`
	// means nothing was reached and the text is written as it stands, which
	// is the same best-effort answer the field above gives.
	//
	// Asked of the braced spelling alone: a bare `$x` has no operand to hold
	// anything. Left nil by every arrangement that does not reprint a
	// substitution's body either.
	ParameterExpansionIsReprinted func(string) (string, bool)

	// FunctionHeader is how a function declaration's header is spelled: the
	// `function` keyword, the `()`, or both.
	FunctionHeader FunctionHeader

	// BraceAfterAFunctionHeaderOnItsOwnLine gives the `{` that opens a
	// declared body a line of its own, the way the first statement of a
	// block does under OutermostBraceOpensALine.
	//
	// Only where the header was written with `()`; the keyword form already
	// decides this for itself, since a name list is greedy and only a
	// newline ends it.
	BraceAfterAFunctionHeaderOnItsOwnLine bool

	// StatementsShareALineOutsideADeclaration joins the statements of a block
	// with Separator and a blank rather than giving each a line, and keeps a
	// brace group or a subshell on the line it started — everywhere but inside
	// a function declaration's body, where Lines applies as it stands.
	//
	// One field with a boundary in its name, because the boundary is what was
	// measured. A canonicalizer that writes a whole script back uses one
	// arrangement for a declaration's body and another for everything outside
	// one, and the two differ in exactly this: `{ echo a; echo b; }` written at
	// file scope comes back on one line, and the same group inside `f() { … }`
	// comes back over four. It is not a depth rule and not a rule about the
	// outermost command — `if true; then { echo a; echo b; }; fi` at file scope
	// expands the `if` and leaves the group alone, and `if true; then f() {
	// echo a; echo b; }; fi` expands the declaration's group while joining the
	// `if`'s own body. The declaration is the whole of the boundary.
	//
	// What a keyword opens still takes lines of its own: `if a; then` ends its
	// line, the body is indented, and `fi` starts one. So this is narrower than
	// turning Lines off, which flattens the construct as well.
	StatementsShareALineOutsideADeclaration bool

	// FileFollowsTheSourceUnits writes the statements of a *file* the way the
	// input laid them out: a statement that begins on a later line than the one
	// before it ended begins a line of its own, and a gap of any size between
	// two of them becomes exactly one blank line.
	//
	// The top level only, which is what makes it a separate answer from Lines.
	// Inside a block the same arrangement ignores the source's lines entirely —
	// `if true; then` over three input lines comes back with its body joined on
	// one — so a file's own line structure is preserved at the level a reader
	// has by the line and nowhere below it.
	//
	// A gap before the *first* statement counts, which is how a shebang and a
	// leading comment block leave a blank line behind in an engine whose tree
	// holds no comments.
	FileFollowsTheSourceUnits bool

	// TrailingBlankLine ends the output with a blank line.
	//
	// Its own field rather than part of the one above, because the two come
	// apart on a route that exists: an engine that writes a file back unit by
	// unit and then meets a parse failure writes every unit it read and no
	// blank line at the end, where the same engine reaching the end of the
	// input writes one.
	TrailingBlankLine bool

	// TranslatedWordWrittenPlain writes a `$"…"` back as an ordinary
	// double-quoted string, dropping the mark — see [Span.Translated].
	//
	// Off by default, because the mark is in the tree and a round trip has
	// to give it back: `$"x"` reprinted as `"x"` is a different tree, and
	// the one option that lists these strings would not see it.
	//
	// On for a *listing*, which is measured and was not obvious: the engine
	// that says a function body back drops the mark in all three of the
	// places it says one — shown to a person, written into the environment
	// for a child, and writing a whole script back — so a body reprinted out
	// of a running shell holds plain quotes. The mark is an invocation-time
	// fact there, and it has already been used by the time a body can be
	// listed.
	TranslatedWordWrittenPlain bool
}

// FunctionHeader is how a function declaration's header is spelled.
//
// The three values are the three answers measured, and the spelling matters
// beyond taste in exactly one dialect: `typeset` in a `function f { … }` body
// declares a local there and assigns the global in an `f() { … }` body, which
// is the axis Semantics.TypesetLocalNeedsKeywordFunction records. So the
// default keeps what was written, and a listing that respells says so.
type FunctionHeader uint8

const (
	// FunctionHeaderAsWritten writes the keyword where the declaration had
	// one and `()` where it did not, which is the spelling that reads back
	// to the same tree.
	FunctionHeaderAsWritten FunctionHeader = iota
	// FunctionHeaderKeywordAndParens writes both, whichever was used:
	// `function f () `. The hybrid spelling, which parses to the keyword
	// declaration in every grammar that takes it.
	FunctionHeaderKeywordAndParens
	// FunctionHeaderParens writes `f () ` and never the keyword. Correct
	// only where the two bodies run alike, which is every dialect but one.
	FunctionHeaderParens
)

func (h FunctionHeader) String() string {
	switch h {
	case FunctionHeaderKeywordAndParens:
		return "keyword and parens"
	case FunctionHeaderParens:
		return "parens"
	}
	return "as written"
}

// PrintWith renders one command with a chosen arrangement.
func PrintWith(c Command, l Layout) string {
	if c == nil {
		return ""
	}
	p := printer{layout: l}
	p.printed(c)
	return p.b.String()
}

// printed writes the command a caller handed in, put in a brace group first
// where the arrangement asks for a body that is always one.
//
// The wrapping is here rather than in command because it is about the
// outermost command alone: a subshell *inside* a body is written as the
// subshell it is, and it is only the body itself that a listing spells with
// braces however it was declared.
func (p *printer) printed(c Command) {
	if _, braced := c.(*Group); p.layout.BodyIsAlwaysBraced && !braced {
		p.braceGroup([]*Stmt{{Expr: &Pipeline{Cmds: []Command{c}}}})
		return
	}
	p.command(c)
}

type printer struct {
	b strings.Builder
	// after is the text that will follow the span being written, which is
	// what decides whether a parameter can drop its braces.
	after string
	// raw suppresses escaping of an unquoted literal, for the places where
	// the punctuation belongs to a pattern rather than to the shell.
	raw bool
	// caseBlanks keeps a bare blank in an unquoted literal bare, for the one
	// place a word may hold one: a `case` arm's parenthesized pattern list,
	// where the blank is a character of the pattern. Escaping it there
	// re-parses to the same *text* under a different quoting, which is not
	// the same tree — and the shell being modeled writes it bare too, its
	// own `functions` listing printing `(a b)` back.
	//
	// Set only where the arm's `(` is written with it, which is what makes
	// the bare form read back: see patternsNeedTheParen.
	caseBlanks bool
	// layout is how a block is arranged, when the caller asked for one, and
	// depth how many blocks deep the writing has reached.
	layout Layout
	depth  int
	// inDeclaration is whether the writing has descended into a function
	// declaration's body, which is the one boundary an arrangement may change
	// at — see Layout.StatementsShareALineOutsideADeclaration.
	inDeclaration bool
	// heredocs are the bodies owed by the statement being written, which go
	// after it rather than where the operator is.
	heredocs []*Redirect
	// carried is the file's here-document bodies that belong to a
	// substitution rather than to a redirection of the command — see
	// [File.CarriedHeredocs]. Empty for every dialect but the two that read a
	// body from after the enclosing command.
	carried []CarriedHeredoc
	// consumed is the last source line a written here-document body reached,
	// which is the one thing a statement's own End cannot say — see units,
	// its only reader.
	consumed int32
	// oweASkippedSeparator is set when a here-document body has been written
	// and the *next* statement separator has still to be dropped.
	//
	// The body ends the line it was on, so the separator of the statement
	// that carried it is already gone — atLineStart is what says so. The
	// separator after **that** goes too, and it is a separate piece of state
	// because by then something has been written and the line has started
	// again. Measured 2026-09-22 on bash 5.3.20 through `type`, over four
	// shapes that pin exactly which one is dropped:
	//
	//	{ cat <<E …; echo a; echo b; echo c; }
	//	                  cat and `echo a` lose theirs, `echo b` keeps its
	//	{ echo z; cat <<E …; echo a; echo b; }
	//	                  `echo z` keeps its, cat and `echo a` lose theirs
	//	for i in 1; do cat <<E … done; echo a; echo b
	//	                  cat and `done` lose theirs, `echo a` keeps its
	//	for i in 1; do cat <<E …; echo q; done; echo a; echo b
	//	                  cat and `done` lose theirs, `echo q` keeps its
	//
	// The last row is what makes this one flag rather than a count of two:
	// the `;` that closes a keyword body is a **terminator** and neither
	// takes the skip nor consumes it, so the skipped pair can have a
	// statement between them.
	//
	// And a body closed by a **bracket** cancels it, where a body closed by
	// a keyword carries it out: measured the same day, `coproc ( cat <<E … )`
	// and `coproc { cat <<E …; }` are both followed by their `;`, where the
	// `done` of the loop above is not. That is the same line Layout draws
	// between a keyword terminator and a bracket, read from the other end.
	oweASkippedSeparator bool
	// translated collects the `$"…"` runs as they are written, for the one
	// caller that wants them rather than the text — see TranslatedStrings.
	// Nil for every ordinary print.
	translated *[]TranslatedString
}

func (p *printer) str(s string) { p.b.WriteString(s) }

// atLineStart reports whether what has been written so far ends a line.
//
// The printer owns every newline it writes but one: a here-document's body
// carries the newlines the input gave it, and the last of them is the reason
// the delimiter that follows begins a line. So after a body the writing may
// already be at the start of a line — or, where the input ran out mid-line,
// may not be at the start of one the printer would otherwise have assumed.
//
// Everything that ends a statement asks here first. A separator, a keyword
// terminator or a body's opening newline written without asking is a
// separator alone on a line of its own, which is not a statement terminator
// in any shell — bash, dash and ksh93 all refuse it — and a delimiter written
// without asking joins the body's last line and becomes part of the body.
func (p *printer) atLineStart() bool { return strings.HasSuffix(p.b.String(), "\n") }

// newLine starts the next line of a chosen arrangement, indent and all.
//
// A newline and the pad, except where the line has already been ended — which
// only a here-document body does, since the printer owns every other newline
// it writes. Whether a second one goes in there, leaving a blank line the
// source never had, is Layout.BlankLineAfterAHereDocumentBody: the two
// engines that list a body answer it differently, at the closing brace as
// much as between two statements.
// laidOut reports whether this arrangement lays a construct out rather than
// writing it on the line it was written on.
//
// Lines says so for everything. KeywordBodiesTakeTheirOwnLines says so for
// what a reserved word closes, and the three callers it does *not* reach —
// how two statements of a list are separated, a brace group, and a subshell —
// ask [Layout.Lines] directly for exactly that reason.
func (p *printer) laidOut() bool {
	return p.layout.Lines || p.layout.KeywordBodiesTakeTheirOwnLines
}

// laysOutStatements reports whether each statement of a list takes a line of
// its own.
//
// Lines says so everywhere. Under KeywordBodiesTakeTheirOwnLines a list keeps
// the source's own line breaks — that is the half of the arrangement Lines
// cannot express — **except inside a declared body**, which comes back laid
// out wherever it was written: measured, a function declared inside a command
// substitution is listed with its statements one to a line while the
// substitution's own list is not.
func (p *printer) laysOutStatements() bool {
	return p.layout.Lines || (p.laidOut() && p.inDeclaration)
}

func (p *printer) newLine() {
	if p.atLineStart() && !p.layout.BlankLineAfterAHereDocumentBody {
		p.str(p.pad())
		return
	}
	p.str("\n" + p.pad())
}

// stmts writes a list, separated the way a shell separates them on one line.
//
// `;` between and none after, which is what a group needs — `{ a; b; }` has
// the last one terminated by the brace's own rule and this adds it there.
// lines and stmts both write a list; they differ in nothing and are kept
// apart only because the two callers read better for it.
func (p *printer) lines(list []*Stmt) { p.stmts(list) }

// units writes the statements of a file the way the input laid them out.
//
// A unit is what one input line's worth of grammar produced: statements that
// begin where the one before them ended share a line, and one that begins on
// a later line begins a line. A gap of any size between two of them — blank
// lines, comment lines, both — becomes exactly one blank line, and a gap
// before the first statement counts, which is what a shebang leaves behind in
// a tree that holds no comments.
//
// Only the top level asks this. Inside a block the arrangement decides where
// the lines go and the source does not, which is why this is a loop of its own
// and not a value of Layout.Lines.
func (p *printer) units(list []*Stmt) {
	var ended int32
	for i, st := range list {
		switch {
		case i > 0 && st.Pos().Line == ended:
			// The same unit: what the source wrote on one line stays on one.
			p.separate(list[i-1], st)
		default:
			if i > 0 {
				p.str("\n")
			}
			if st.Pos().Line > ended+1 {
				p.str("\n")
			}
		}
		p.stmt(st)
		ended = st.End().Line
		if p.consumed > ended {
			// A here-document body went on past the statement that owed it,
			// so the unit ended where the body did. Without this the lines
			// it read look like a gap, and a blank line goes in that the
			// input never had.
			ended = p.consumed
		}
	}
	if len(list) > 0 {
		p.str("\n")
	}
}

// stmts writes a list, separating each from the one before it the way the
// source did.
//
// Which is not a matter of taste. A shell says which line something happened
// on, and `$LINENO` says where it is, so a printer that decides where the
// line breaks go decides what the script says about itself: two statements
// joined onto one line report line 1 twice, and one line split into two
// reports 1 and 2. Both were wrong here before the behavioral round trip
// said so, in opposite directions.
//
// The tree knows, because a position is a line.
func (p *printer) stmts(list []*Stmt) {
	for i, st := range list {
		if i > 0 {
			p.separate(list[i-1], st)
		}
		p.stmt(st)
	}
}

// shareALine reports whether the statements being written join a line rather
// than taking one each — see Layout.StatementsShareALineOutsideADeclaration,
// whose boundary this is the whole of.
func (p *printer) shareALine() bool {
	return p.layout.StatementsShareALineOutsideADeclaration && !p.inDeclaration
}

// separate writes what goes between two statements.
func (p *printer) separate(prev, next *Stmt) {
	if p.laysOutStatements() && p.shareALine() {
		// Joined, whatever the source did, which is the same disregard for
		// the source's lines the branch below has — it differs only in
		// writing the separator and a blank where that writes a newline.
		switch {
		case prev.Background:
			// Already terminated: `&` is the separator, and `&;` parses
			// nowhere. The blank is all that keeps the two apart.
			p.str(" ")
		case p.atLineStart():
			// A here-document body ended the line, so there is nothing on
			// this line to terminate and the next statement starts here.
		default:
			p.str(p.layout.Separator + " ")
		}
		return
	}
	if p.laysOutStatements() {
		if prev.Background && p.layout.BackgroundKeepsTheLine {
			// The `&` is the terminator, so there is nothing to write
			// between them but the blank that keeps them apart — and the
			// line goes on rather than ending here.
			p.str(" ")
			return
		}
		// A shape the caller asked for, so the source's own lines do not
		// come into it. A backgrounded statement is already terminated
		// whatever the arrangement says, and so is one whose here-document
		// body ended the line — bash leaves the blank line that falls out
		// of this, and writing the separator there instead is a `;` with
		// nothing before it.
		if !prev.Background && !p.atLineStart() {
			if p.oweASkippedSeparator {
				// The separator a here-document body still owes — see
				// printer.oweASkippedSeparator. Consumed here whether or
				// not the arrangement writes anything, because what is
				// being counted is the separator and not the text.
				p.oweASkippedSeparator = false
			} else {
				p.str(p.layout.Separator)
			}
		}
		p.newLine()
		return
	}
	// A here-document body has already ended the line. There is no way back
	// onto the line the statement was written on, so the rest of it follows
	// the body — which is what a shell does with `cat <<E; echo after` too.
	if p.atLineStart() {
		return
	}
	// A backgrounded statement is already terminated: `a & b` is two
	// statements and `a &; b` is a syntax error. The `&` is the separator,
	// and it is one wherever the next statement was written — measured, `$(a
	// &` newline `b)` comes back `$(a & b)`.
	if prev.Background {
		p.str(" ")
		return
	}
	// What the source wrote, which is the *token* and not the line it left
	// the statement on. The two come apart wherever a `;` and a line break
	// are both there — `$(a;` newline `b)` and a `;` with a comment after it
	// — and the token is the one measured: bash writes `$(a; b)` for both,
	// and keeps two lines only where a newline alone terminated (#3830).
	switch prev.Term {
	case TerminatedBySemicolon:
		p.str("; ")
	case TerminatedByNewline:
		p.str("\n")
	default:
		// Nothing terminated it and nothing above claimed it, which leaves
		// the source's own lines as the only thing left to follow.
		if prev.End().Line != next.Pos().Line {
			p.str("\n")
		} else {
			p.str("; ")
		}
	}
}

func (p *printer) stmt(st *Stmt) {
	if st == nil {
		return
	}
	p.expr(st.Expr)
	if st.Background {
		// `&!` rather than `&` where the job was let go of. One spelling of
		// the two goes back, which is the printer's usual bargain: `&!` and
		// `&|` parse to the same tree, so there is nothing to choose
		// between them and nothing a test could tell apart.
		switch {
		case st.Coprocess:
			// The coprocess operator, which is a background terminator
			// carrying two pipes rather than a pipe joining two commands.
			p.str(" |&")
		case st.Disown:
			p.str(" &!")
		default:
			p.str(" &")
		}
	}
	p.flushHeredocs()
}

func (p *printer) expr(e Expr) {
	switch x := e.(type) {
	case *BinaryExpr:
		p.expr(x.X)
		p.str(" " + x.Op.String() + " ")
		p.expr(x.Y)
	case *Pipeline:
		switch {
		case x.Negated:
			p.str("!")
			if len(x.Cmds) > 0 {
				// A negation with no pipeline after it is the whole
				// command, so there is nothing for a blank to separate it
				// from and a trailing one would not reparse the same.
				p.str(" ")
			}
		case len(x.Cmds) == 0:
			// An *even* run of them, which negates nothing and is still a
			// command: it exits 0 where a single `!` exits 1, and writing
			// nothing at all left the line out of the script entirely. The
			// count is not kept — one flag on the tree is the whole of the
			// rule — and it does not need to be, since every even run
			// behaves alike and two is the shortest.
			//
			// Only [Dialect.RepeatedNegationToggles] can produce this node,
			// and that is the flag that reads the pair back.
			p.str("! !")
		}
		for i, c := range x.Cmds {
			if i > 0 {
				if mergesStderr(x.Cmds[i-1]) && !p.layout.PipeBothWrittenOut {
					p.str(" |& ")
				} else {
					// The redirection the operator stands for has already
					// been written by the command before this one, which is
					// what leaves a plain pipe to write here.
					p.str(" | ")
				}
			}
			p.command(c)
		}
	case *TimeClause:
		if x.Negated {
			p.str("! ")
		}
		p.str("time")
		if x.Posix {
			p.str(" -p")
		}
		if x.Pipeline != nil {
			p.str(" ")
			p.expr(x.Pipeline)
		}
	}
}

// braceGroup writes `{ … }` around a list, in whichever arrangement the layout
// asks for.
//
// Factored out because two constructs are spelled with one: a brace group and
// both halves of a [TryClause]. The space after `{` and the `;` before `}` are
// both required — they are what make it a reserved word rather than the start
// of a name.
func (p *printer) braceGroup(list []*Stmt) {
	// A brace group stays on the line it was written on where only what a
	// keyword closes is laid out — measured, `$({ a; b; })` comes back as it
	// was written where `$(if true; then echo x; fi)` is spread over three
	// lines — and a *declared* body does not, since a declaration written
	// inside a substitution comes back laid out there as well.
	if p.laysOutStatements() && !p.shareALine() {
		p.str("{" + p.layout.BraceOpenSuffix)
		// The outermost brace — a function's own — is the one that differs; a
		// brace inside one opens a line in every arrangement measured.
		p.bodyAt(list, false, p.layout.OutermostBraceOpensALine || p.depth > 0)
		p.newLine()
		p.str("}")
		p.oweASkippedSeparator = false
		return
	}
	if len(list) == 0 {
		// An empty body has no statement to terminate, and terminating
		// nothing wrote `{ ; }` — which does not parse in any dialect. The
		// grammars that reach here are the one that takes an empty brace
		// group and the one whose bodyless `function a b` declares an empty
		// body; both read `{ }` back as what they printed.
		p.str("{ }")
		return
	}
	p.str("{ ")
	p.stmts(list)
	p.terminate()
	p.str("}")
}

func (p *printer) command(c Command) {
	switch x := c.(type) {
	case *SimpleCmd:
		p.simple(x)
	case *Subshell:
		if p.layout.Lines && p.layout.SubshellBodyOnItsOwnLines && !p.shareALine() {
			// The shape a brace group gets, which one engine's listing gives
			// a subshell too. No separation to arrange for here: the
			// parenthesis ends its line, so nothing can run into it.
			p.str("(")
			p.bodyAt(x.List, false, true)
			p.newLine()
			p.str(")")
			p.oweASkippedSeparator = false
			p.redirs(x.Redirs)
			return
		}
		// Spaced, which is not decoration: a subshell whose first command is
		// itself a subshell needs the separation, `((` being arithmetic.
		p.str("( ")
		p.stmts(x.List)
		p.str(" )")
		p.oweASkippedSeparator = false
		p.redirs(x.Redirs)
	case *Group:
		p.braceGroup(x.List)
		p.redirs(x.Redirs)
	case *NamespaceClause:
		// The name is written back as it stood, which is what keeps a word
		// the clause is going to refuse readable in the file it came from —
		// the same rule a refused loop variable is printed under.
		p.str("namespace " + x.Name + " ")
		p.braceGroup(x.List)
		p.redirs(x.Redirs)
	case *TryClause:
		// The redirections go after the *second* half, because they belong to
		// the whole construct: writing one after the try half is what ends it
		// and makes the `always` a syntax error.
		p.braceGroup(x.Try)
		p.str(" always ")
		p.braceGroup(x.Always)
		p.redirs(x.Redirs)
	case *IfClause:
		p.ifClause(x)
	case *LoopClause:
		word := "while"
		if x.Until {
			word = "until"
		}
		p.str(word + " ")
		p.stmts(x.Cond)
		if len(x.Body) > 0 {
			p.opener("do", p.layout.DoAfterCommandOnItsOwnLine)
			p.body(x.Body, true)
			p.keyword("done")
		}
		p.redirs(x.Redirs)
	case *ForClause:
		// Every name, and there is normally one. A loop with more than one
		// is zsh's, and no other dialect has a spelling for it — so the
		// printer writes what the construct is rather than something
		// portable it is not.
		// A refused name is written back as it stood, which is what keeps
		// the printer's promise on a clause that parsed and will fail when
		// it runs: printed and re-read, it fails the same way. There is no
		// name to join, so it stands in for the list rather than beside it.
		if x.RefusedName != "" {
			p.str("for " + x.RefusedName)
		} else {
			p.str("for " + strings.Join(x.Names, " "))
		}
		if len(x.Body) == 0 {
			p.parenItems(x.HasItems, x.Items)
			p.redirs(x.Redirs)
			return
		}
		p.items(x.HasItems, x.Items)
		p.doKeyword()
		p.body(x.Body, true)
		p.keyword("done")
		p.redirs(x.Redirs)
	case *AnonFunc:
		// Printed as it was written: there is no long spelling, because a
		// function with no name cannot be defined in one place and called in
		// another.
		//
		// The keyword standing alone is the whole of the command, and the
		// empty group standing in for the body it was not given is not
		// printed — `function { }` is a different command from `function`,
		// and one that answers differently. See [AnonFunc.Bare].
		if x.Bare {
			p.str("function")
			p.redirs(x.Redirs)
			break
		}
		if x.Keyword {
			p.str("function ")
		} else {
			p.str("() ")
		}
		p.command(x.Body)
		for _, a := range x.Args {
			p.str(" ")
			p.word(a)
		}
		p.redirs(x.Redirs)
	case *RepeatClause:
		// Printed with `do … done`, which parses to the same tree under any
		// dialect that has the construct at all — the same choice a short
		// loop body already makes.
		p.str("repeat ")
		p.word(x.Count)
		p.doKeyword()
		p.body(x.Body, true)
		p.keyword("done")
		p.redirs(x.Redirs)
	case *SelectClause:
		if x.RefusedName != "" {
			p.str("select " + x.RefusedName)
		} else {
			p.str("select " + x.Name)
		}
		if len(x.Body) == 0 {
			p.parenItems(x.HasItems, x.Items)
			p.redirs(x.Redirs)
			return
		}
		p.items(x.HasItems, x.Items)
		p.doKeyword()
		p.body(x.Body, true)
		p.keyword("done")
		p.redirs(x.Redirs)
	case *CaseClause:
		p.caseClause(x)
	case *ForArithClause:
		init, cond, post := x.InitText, x.CondText, x.PostText
		if p.layout.ArithmeticForExpressionsAsWritten {
			init, cond, post = x.PartsAsWritten()
			init = strings.TrimLeft(init, " \t\n")
			cond = strings.TrimLeft(cond, " \t\n")
			post = strings.TrimLeft(post, " \t\n")
		}
		if p.layout.EmptyArithmeticForExpressionIsOne {
			init, cond, post = arithForOne(init), arithForOne(cond), arithForOne(post)
		}
		p.str("for ((" + init + "; " + cond + "; " + post + "))")
		if len(x.Body) > 0 {
			p.arithDoKeyword()
			p.body(x.Body, true)
			p.keyword("done")
		}
		p.redirs(x.Redirs)
	case *TestClause:
		p.str("[[ ")
		p.cond(x.Expr)
		p.str(" ]]")
		p.redirs(x.Redirs)
	case *ArithCmdClause:
		p.str("((" + x.Expr + "))")
		p.redirs(x.Redirs)
	case *FuncDecl:
		// With the `function` keyword where it was written with one. The
		// note that used to stand here said the two spellings are the same
		// declaration to every shell that has both, and that is measurably
		// wrong of the one shell for which the word carries meaning: in
		// ksh93 `typeset` in a `function f { …; }` body declares a local
		// and in an `f() { …; }` body assigns the global. That is the axis
		// Semantics.TypesetLocalNeedsKeywordFunction records and the reason
		// FuncDecl.Keyword is in the tree at all, so a print that dropped
		// the word handed back a program whose locals leak — silently, at
		// exit 0, which is what a formatter built on this printer would
		// have done to every keyword function it touched (#1406).
		//
		// ksh93 says the same thing about itself: its own `typeset -f`
		// writes `function f { …; }` back for the one and `f() { …; }` for
		// the other, and `eval "$(typeset -f f)"` keeps the locality only
		// because it does.
		//
		// The *default* rather than unconditional, and no dialect is asked
		// for it: a tree can only carry the keyword if the grammar that read
		// it has the keyword, so writing it back is writing what that
		// grammar reads. The same reasoning AnonFunc above already prints
		// on, from the same field. An arrangement that asked to respell the
		// header says so through Layout.FunctionHeader, which is how a
		// listing reaches the two spellings the engines actually write —
		// and neither of those is this one.
		//
		// The hybrid `function f() { …; }` comes back in the keyword form
		// without its parentheses, which is the tree it parsed to: the
		// parser consumes them and records nothing, because the only shell
		// that parts the two spellings refuses the hybrid outright and the
		// two that take it treat all three alike.
		keyword, parens := x.Keyword, !x.Keyword
		switch p.layout.FunctionHeader {
		case FunctionHeaderKeywordAndParens:
			keyword, parens = true, true
		case FunctionHeaderParens:
			keyword, parens = false, true
		}
		if keyword {
			p.str("function ")
		}
		named := p.b.Len()
		// The name as *written* where it was written with an expansion:
		// printing its literal text would name a different function, which
		// is the same loss the parser used to take (see FuncDecl.NameWord).
		// A name the dialect will refuse is source text already and goes
		// back as it came — quoting it would hand back a program the shell
		// refuses for a different reason, or accepts.
		if x.RefusedName != "" {
			p.str(x.RefusedName)
		} else if x.NameWord != nil {
			p.word(x.NameWord)
		} else {
			p.str(printedFuncName(x.Name))
		}
		// And the names after it, where one body was given several. Each is
		// written the way the first is, which is what makes the print a
		// program that reads back to this tree: dropping them would hand
		// back a definition of the first name alone, at status 0, with the
		// rest of the script calling functions nobody defined.
		for _, n := range x.AlsoNamed {
			p.str(" ")
			if n.Word != nil {
				p.word(n.Word)
			} else {
				p.str(printedFuncName(n.Name))
			}
		}
		switch {
		case parens:
			// The `()` ends the name list, so the greedy reading below
			// cannot arise and the body may open on this line.
			//
			// A blank between the name and the parentheses where the header
			// was respelled, and none where it was kept: `f()` is how a
			// declaration is written and `f () ` is how both engines that
			// list one write it back, and the tree records neither — so the
			// spelling travels with the field that asked for it. None at all
			// before an anonymous function's `()`, whose name is empty and
			// where a leading space would be a word of its own.
			if p.layout.FunctionHeader != FunctionHeaderAsWritten && p.b.Len() > named {
				p.str(" ")
			}
			p.str("()")
			// A body that is not a brace group still opens one where the
			// arrangement braces every body, so the question is whether a `{`
			// is about to be written and not whether the source had one.
			_, braced := x.Body.(*Group)
			if (braced || p.layout.BodyIsAlwaysBraced) && p.layout.BraceAfterAFunctionHeaderOnItsOwnLine {
				p.str(" \n" + p.pad())
			} else {
				p.str(" ")
			}
		case keyword:
			// A newline where the body does not open with a `{`, because the
			// name list is greedy — see
			// [FunctionKeywordBodyNeedsItsOwnLine], which is where that rule
			// and its measurement live, and which the formatter asks too.
			if FunctionKeywordBodyNeedsItsOwnLine(x.Body) {
				p.str("\n" + p.pad())
			} else {
				p.str(" ")
			}
		}
		// A declaration's body is the one boundary an arrangement may change
		// at, and the change reaches everything inside it — a group nested
		// three deep in a declared body is written the declared way.
		//
		// printed rather than command, so that an arrangement asking for a
		// body that is always braced gets one here too: a nested `inner() ( …
		// )` comes back with `{ … }` around the subshell in the engine this
		// models, and writing the subshell bare was a second place the same
		// normalization was owed.
		saved := p.inDeclaration
		p.inDeclaration = true
		p.printed(x.Body)
		p.inDeclaration = saved
	case *CoprocClause:
		p.str("coproc ")
		// The word as written where it was not a bare name: printing what it
		// expands to would name a different coprocess, and printing the text
		// of a word the shell refuses would hand back a program refused for
		// a different reason. Same care as a function name — see
		// FuncDecl.NameWord above.
		switch {
		case x.NameWord != nil:
			p.word(x.NameWord)
			p.str(" ")
		case x.Name != "":
			p.str(x.Name + " ")
		case p.layout.CoprocessDefaultName != "":
			// Nothing was written, and the arrangement asks for the name
			// the shell gives one anyway. Only over a compound command:
			// before a simple one there is no place a name could have
			// stood, so writing one there would turn the command's first
			// word into a name and the rest into the command. See
			// Layout.CoprocessDefaultName.
			if _, simple := x.Cmd.(*SimpleCmd); !simple {
				p.str(p.layout.CoprocessDefaultName + " ")
			}
		}
		p.command(x.Cmd)
	}
}

// printedFuncName writes a name so that reading it back names the same
// function.
//
// Most names go bare, and the ones that cannot are the ones one grammar reads
// and the printer would otherwise change the meaning of — see
// [Dialect.FunctionKeywordNameIsAnyWord], under which the word after the
// keyword is the name whatever is in it. Written bare:
//
//	the empty name    `function  { … }` is the *anonymous* function, which
//	                  runs its body where the definition defines something
//	`a b`             two name words, which is a different construct again
//	`a;b`, `a|b`      the name ends at the operator and the rest is a command
//	`a$b`             an expansion, so the name is whatever it comes to
//	`a*b`             matched against the filesystem
//
// Every one of those is a program that *parses*, which is what makes this
// worth a line rather than an assertion: a printer that dropped the quoting
// handed back a different program at status 0.
//
// The test is isFuncName with punctuation allowed, which is the widest reading
// any dialect gives a bare name — so a name goes bare exactly when some
// grammar here reads it bare, and the printer and the parser cannot come
// apart. That is a narrower bare set than the shell's own listing uses, and
// deliberately: `${(q)…}`-shaped answers are the interpreter's, and this is
// about what re-parses. See interp.listedFunctionName.
func printedFuncName(name string) string {
	if isFuncName(name, true) {
		return name
	}
	return "'" + strings.ReplaceAll(name, "'", `'\''`) + "'"
}

// cond writes the inside of a `[[ … ]]`.
//
// The operands are words rather than strings because what they mean depends
// on how they were written — unquoted, the right operand of `==` is a pattern
// and of `=~` a regular expression — so they go through the word printer like
// any other and keep their quoting with them.
func (p *printer) cond(e CondExpr) {
	switch x := e.(type) {
	case *CondUnary:
		p.str(x.Op + " ")
		p.rawWord(x.X)
	case *CondArity:
		// An operator the grammar accepted with the wrong number of operands
		// — see CondArity. Written back as it was read: the operator and
		// every word that stood with it, in order, because the line is
		// still a line somebody wrote and a formatter may not decide which
		// of its words was the surplus one.
		p.str(x.Op)
		for _, w := range x.Words {
			p.str(" ")
			p.rawWord(w)
		}
	case *CondCompletion:
		// The operator stands in front of its operands — see CondCompletion
		// — and each of them is a pattern, so they go back unquoted exactly
		// as they were read, the way CondBinary's right operand does.
		p.str(x.Op)
		for _, w := range x.Words {
			p.str(" ")
			p.rawWord(w)
		}
	case *CondBinary:
		p.word(x.X)
		p.str(" " + x.Op + " ")
		// The right operand unquoted is a pattern for `==` and a regular
		// expression for `=~`, so its punctuation is the pattern's: escaping
		// `(`, `|` or `$` leaves the tree identical and turns `^(a|x)bc$`
		// into a search for four literal characters. Whatever the parser
		// accepted unquoted here goes back unquoted.
		p.rawWord(x.Y)
	case *CondLogic:
		p.cond(x.X)
		p.str(" " + x.Op + " ")
		p.cond(x.Y)
	case *CondNot:
		p.str("! ")
		p.cond(x.X)
	case *CondGroup:
		// The parentheses are kept rather than dropped where they happen to
		// be redundant: they are in the tree, and a printer that decides
		// they are unnecessary is deciding about precedence.
		p.str("( ")
		p.cond(x.X)
		p.str(" )")
	}
}

// doKeyword writes the `do` that opens a `for` or `select` body.
//
// On a line of its own, which a `while` body's is not — the header of one is
// a word list and of the other a command, and the arrangement follows that
// rather than being uniform.
func (p *printer) doKeyword() { p.opener("do", p.layout.DoAfterWordsOnItsOwnLine) }

// arithDoKeyword is the `do` of an arithmetic `for`, which one arrangement
// spells unlike every other loop: on a line of its own **and** with nothing
// ending the header. `for i in a b;` keeps its semicolon there and
// `for ((i=0; i<2; i++))` has none, so this cannot go through opener.
func (p *printer) arithDoKeyword() {
	if !p.laidOut() || !p.layout.DoAfterArithmeticOnItsOwnLine {
		p.opener("do", false)
		return
	}
	if p.atLineStart() {
		p.str(p.pad() + "do")
		return
	}
	p.str("\n" + p.pad() + "do")
}

// opener writes the keyword that introduces a body — `then`, `do` — either on
// the header's line or on one of its own, which the arrangement decides.
func (p *printer) opener(word string, ownLine bool) {
	if !p.laidOut() {
		p.str("; " + word)
		return
	}
	if p.atLineStart() {
		// The header owed a here-document, whose body ended the line. There
		// is nothing left on it to separate from, so the keyword opens the
		// line it is already at the start of — which is where bash puts it,
		// whether or not the arrangement would have kept it inline.
		p.str(p.pad() + word)
		return
	}
	if ownLine {
		p.str(p.layout.Separator + "\n" + p.pad() + word)
		return
	}
	p.str(p.layout.Separator + " " + word)
}

// items writes a `for` or `select` header's word list.
//
// `in` with nothing after it is not the same as no `in` at all — the first
// loops over nothing and the second over the positional parameters — so the
// flag decides rather than the length.
func (p *printer) items(has bool, items []*Word) {
	if !has {
		return
	}
	p.str(" in")
	for _, w := range items {
		p.str(" ")
		p.word(w)
	}
}

// parenItems writes the same list in the short loop's parentheses.
//
// Only reached for a loop whose body is empty, and that is the whole reason
// the spelling exists here. `do … done` cannot hold no commands — every shell
// in the panel refuses `do done` — so a tree with an empty body has no long
// form to be printed as, and `for i in a b` with nothing after it is a syntax
// error in the shell that produced it. The parentheses end the header, which
// is what lets the body be absent.
func (p *printer) parenItems(has bool, items []*Word) {
	if !has {
		return
	}
	p.str(" (")
	for i, w := range items {
		if i > 0 {
			p.str(" ")
		}
		p.word(w)
	}
	p.str(")")
}

func (p *printer) ifClause(x *IfClause) {
	if len(x.Elifs) > 0 && p.layout.ElifWrittenAsANestedIf {
		// The first `elif` becomes an `else` whose one statement is an `if`
		// carrying everything after it, and the rest arrive back here
		// through that `if`. So the recursion is the construct's own rather
		// than a second walk written beside it, and the `else`, the
		// indentation and the terminator before `fi` are the ones every
		// other body gets.
		p.str("if ")
		p.stmts(x.Cond)
		p.opener("then", p.layout.ThenOnItsOwnLine)
		p.body(x.Then, true)
		p.keyword("else")
		rest := &IfClause{
			Cond: x.Elifs[0].Cond, Then: x.Elifs[0].Then,
			Elifs: x.Elifs[1:], Else: x.Else, HasElse: x.HasElse,
		}
		p.body([]*Stmt{{Expr: &Pipeline{Cmds: []Command{rest}}}}, true)
		p.keyword("fi")
		// On the outermost clause alone: the nested ones carry none, since
		// the construct they were split out of had one set of them.
		p.redirs(x.Redirs)
		return
	}
	p.str("if ")
	p.stmts(x.Cond)
	p.opener("then", p.layout.ThenOnItsOwnLine)
	p.body(x.Then, true)
	for _, e := range x.Elifs {
		p.keyword("elif ")
		p.stmts(e.Cond)
		p.opener("then", p.layout.ThenOnItsOwnLine)
		p.body(e.Then, true)
	}
	if x.HasElse {
		p.keyword("else")
		p.body(x.Else, true)
	}
	p.keyword("fi")
	p.redirs(x.Redirs)
}

// body writes the statements between two keywords.
//
// On one line where the caller asked for nothing, and one to a line where a
// layout was chosen — the arrangement reaches inside a construct rather than
// stopping at the outermost block, because a caller laying a function out
// this way lays all of it out.
//
// closedByKeyword says what follows: a word such as `fi` or `done`, or a
// bracket. It decides the last statement's terminator, which is the one rule
// here that is not about where the line breaks go — a keyword takes a `;`
// before it and `}` and `;;` do not.
func (p *printer) body(list []*Stmt, closedByKeyword bool) {
	p.bodyAt(list, closedByKeyword, true)
}

// bodyAt is body, with whether the first statement starts on a line of its
// own. It does everywhere but at a brace in the flatter of the two
// arrangements, where the body opens on the brace's line.
func (p *printer) bodyAt(list []*Stmt, closedByKeyword, ownLine bool) {
	if !p.laidOut() {
		p.str(" ")
		p.stmts(list)
		return
	}
	p.depth++
	if ownLine {
		p.str("\n")
	}
	p.str(p.pad())
	p.stmts(list)
	if closedByKeyword && !p.atLineStart() {
		p.str(p.layout.KeywordTerminator)
	}
	p.depth--
}

// pad is the indent for the depth being written.
func (p *printer) pad() string {
	if p.depth == 0 {
		// Nothing encloses this, so nothing indents it: the brace that
		// closes a function's body sits where the function does.
		return ""
	}
	if !p.layout.Nested {
		return p.layout.Indent
	}
	return strings.Repeat(p.layout.Indent, p.depth)
}

// keyword writes the word that closes or continues a construct.
func (p *printer) keyword(word string) {
	if p.laidOut() {
		p.newLine()
		p.str(word)
		return
	}
	p.terminate()
	p.str(word)
}

// terminate ends the statement before a closing word.
//
// A `;` unless the line has already ended, which a here-document body does:
// its lines go after the statement, so what follows is at the start of a line
// and `; fi` there is a `;` with nothing before it. Found by printing every
// shell script on this machine — four of them close a block right after a
// here-document, and the corpus has none that do.
func (p *printer) terminate() {
	if p.atLineStart() {
		return
	}
	// A backgrounded statement is already terminated — the rule separate
	// applies between statements holds at the closing word too: `&` before
	// `}` or `done` takes only a space, and `&;` parses nowhere. The suffix
	// is unambiguous, because ` &` is written by the statement printer alone:
	// a literal ampersand in a word arrives escaped or quoted.
	if strings.HasSuffix(p.b.String(), " &") {
		p.str(" ")
		return
	}
	p.str("; ")
}

func (p *printer) caseClause(x *CaseClause) {
	p.str("case ")
	p.word(x.Word)
	if p.laidOut() {
		p.str(" in" + p.layout.CaseHeaderSuffix)
		p.caseArms(x)
		return
	}
	p.str(" in ")
	for _, it := range x.Items {
		saved := p.caseBlanks
		p.caseBlanks = patternsNeedTheParen(it)
		if p.caseBlanks {
			p.str("(")
		}
		for i, pat := range it.Patterns {
			if i > 0 {
				p.str("|")
			}
			p.word(pat)
		}
		p.caseBlanks = saved
		p.str(") ")
		p.stmts(it.Body)
		term := it.Term.String()
		if it.Term == 0 {
			// The last arm may leave its terminator out, and putting one in
			// is what lets `esac` follow without changing the tree.
			term = ";;"
		}
		p.str(" " + term + " ")
	}
	p.str("esac")
	p.redirs(x.Redirs)
}

// caseArms writes a `case`'s arms one to a line.
//
// The pattern, the body one further in, and the terminator back at the
// pattern's depth — which is a block closed by `;;` rather than by a keyword,
// so its last statement takes no `;`.
func (p *printer) caseArms(x *CaseClause) {
	p.depth++
	for i, it := range x.Items {
		// The first arm opens a line of its own where every statement does,
		// and follows the `in` where only what a keyword closes is laid out:
		// measured, `$(case x in a) b;; esac)` comes back `case x in a)` on
		// one line with the arms after it each on their own.
		if i > 0 || p.layout.Lines {
			p.str("\n" + p.pad())
		}
		saved := p.caseBlanks
		p.caseBlanks = patternsNeedTheParen(it)
		if p.layout.CasePatternsParenthesised || p.caseBlanks {
			p.str("(")
		}
		for i, pat := range it.Patterns {
			if i > 0 {
				p.str("|")
			}
			p.word(pat)
		}
		p.caseBlanks = saved
		p.str(")")
		term := it.Term.String()
		if it.Term == 0 {
			// The last arm may leave its terminator out, and putting one in
			// is what lets what follows follow.
			term = ";;"
		}
		if p.layout.CaseArmsOnOneLine {
			p.str(" ")
			p.stmts(it.Body)
			p.str(" " + term)
			continue
		}
		p.bodyAt(it.Body, false, true)
		p.str("\n" + p.pad() + term)
	}
	p.depth--
	p.str("\n" + p.pad() + "esac")
	p.redirs(x.Redirs)
}

// patternsNeedTheParen reports whether an arm's opening `(` is load-bearing —
// whether leaving it out would hand back a different program.
//
// The arm's paren is ordinarily optional and the printer drops it, which is
// the normalization the round trip is written to allow. It stops being
// optional where a pattern holds a bare blank: that is a grammar one dialect
// has only *inside* the parentheses — see
// [Dialect.CasePatternListSpansBlanks] — so `(a b)` printed as `a b)` is a
// parse error and `((x) y)` printed as `(x) y)` is worse, being a program
// that parses to something else.
//
// A bare newline counts for the same reason and used not to.
// [Dialect.CasePatternListSpansNewlines] is the same grammar, but the word
// printer wrote a newline back **quoted**, so the paren it asked for was one
// the printed text did not need — the paren went in on the first pass and the
// quoting took it back out on the second, which made the print unsettled
// rather than wrong. That circularity is gone: inside the paren the newline
// goes back bare, exactly as the blank does, so asking for the paren and
// writing the character it is for are now one decision. It is also what the
// shell being modeled writes — its `functions` listing prints an arm of
// `(a|` ⏎ `b)` back with the newline bare (#1254).
//
// Read off the spans rather than off the source, because it is the *printed*
// text this is about. A blank or a newline in an *unquoted* literal span is
// itself the evidence that the dialect had the rule: no other grammar here
// puts either character in one, and the printer is handed a Layout rather
// than a Dialect, so the shape is the only thing that can say so.
func patternsNeedTheParen(it *CaseItem) bool {
	for _, w := range it.Patterns {
		for _, sp := range w.Spans {
			if sp.Kind != Literal || sp.Quoting != Unquoted {
				continue
			}
			if strings.ContainsAny(sp.Value, " \t\n") {
				return true
			}
		}
	}
	return false
}

func (p *printer) simple(c *SimpleCmd) {
	first := true
	sep := func() {
		if !first {
			p.str(" ")
		}
		first = false
	}
	// The words the grammar took away come back first, because they were
	// written first and printing a tree prints the program that was read.
	for _, w := range c.Precommands {
		sep()
		p.word(w)
	}
	// Prefixes first, then the command, then the assignments written as its
	// operands. Printing them all in front turned `typeset a=(x y)` into
	// `a=(x y) typeset`, which is a different command: the array became the
	// environment of a `typeset` with nothing to declare.
	for _, a := range c.Assigns {
		if a.Operand {
			continue
		}
		sep()
		p.assign(a)
	}
	for _, w := range c.Args {
		sep()
		p.word(w)
	}
	for _, a := range c.Assigns {
		if !a.Operand {
			continue
		}
		sep()
		p.assign(a)
	}
	// A command that is nothing but redirections has no word for the blank
	// to separate them from — see Layout.BlankBeforeAWordlessRedirection,
	// which is where the two engines part.
	p.redirsAfter(c.Redirs, !first || p.layout.BlankBeforeAWordlessRedirection)
}

func (p *printer) assign(a *Assign) {
	p.str(a.Name)
	if a.Index != nil {
		p.str("[")
		if a.IndexFlags != nil {
			// A flag group's parentheses belong to the subscript, exactly as
			// a pattern's do inside `[[ … ]]`: escaping them would put a
			// backslash where the reader expects a group, and `b[(r)y]=Q`
			// would come back as `b[\(r\)y]=Q` — arithmetic again, and a
			// failure rather than the element it named.
			p.rawWord(a.Index)
		} else {
			p.word(a.Index)
		}
		p.str("]")
	}
	// The member path stands between the bracket and the operator, which is
	// where it was written: `a[1].p=9`. See Assign.Member.
	p.str(a.Member)
	if a.Append {
		p.str("+")
	}
	p.str("=")
	switch {
	case a.Members != nil:
		// A compound variable's body, whose items are separated by the `;`
		// that was written between them rather than by a blank: a blank
		// between two of them is what the source may have had, and either
		// spelling reads back as the same tree — but `;` is the one spelling
		// that is right for every item, since a declaration command's
		// operands run on without it.
		p.str("(")
		for i, item := range a.Members {
			if i > 0 {
				p.str("; ")
			}
			p.compoundVariableItem(item)
		}
		p.str(")")
	case a.IsArray:
		p.str("(")
		p.arrayElems(a.Elems)
		p.str(")")
	case a.Value != nil:
		p.word(a.Value)
	}
}

// arrayElem writes back one element of an array literal: an ordinary word, or
// a literal of its own.
//
// The nested form is written with no blank inside its parentheses —
// `a=((1 2) (3 4))` — which is what the outer literal's own elements are
// written with, so the two nest without the spacing drifting a level at a
// time. What `typeset -p` writes is a separate question and is the
// interpreter's; see interp.nestedListing.
func (p *printer) arrayElem(e *ArrayElem) {
	if e.Word != nil {
		// The `[sub]=` head where the literal is what goes under the key,
		// and the whole element where it is not. Written with no blank
		// between the two, which is the spelling that reads back the same:
		// a blank there is taken the same way, but the head-less form is not
		// what `a=( [0]= (1 2) )` said.
		p.word(e.Word)
		if e.Nested == nil {
			return
		}
	}
	p.str("(")
	p.arrayElems(e.Nested.Elems)
	p.str(")")
}

// arrayElems writes an element list, with one blank the source may not have
// had: a literal whose **first** element is a literal of its own is written
// `( (1 2) )` rather than `((1 2) )`, because `((` behind an `=` is an
// arithmetic command and what this printed would not read back as what it
// printed. The empty case is where it bites — `a=(())` is `((` followed by
// `))` and nothing else.
func (p *printer) arrayElems(elems []*ArrayElem) {
	for i, e := range elems {
		if i > 0 || (e.Word == nil && e.Nested != nil) {
			p.str(" ")
		}
		p.arrayElem(e)
	}
}

// compoundVariableItem writes back one declaration of a compound variable's
// body, in the order the words and assignments were written: a declaration
// command's own operands may be assignments, so the two lists interleave and
// the command word is what says which order to take them in.
func (p *printer) compoundVariableItem(c *SimpleCmd) {
	first := true
	space := func() {
		if !first {
			p.str(" ")
		}
		first = false
	}
	for _, w := range c.Args {
		space()
		p.word(w)
	}
	for _, a := range c.Assigns {
		space()
		p.assign(a)
	}
}

func (p *printer) redirs(rs []*Redirect) { p.redirsAfter(rs, true) }

// redirsAfter is redirs with the leading blank made a decision.
//
// blank is whether anything the redirections have to be kept apart from was
// written before them. It is false only for a simple command with no words
// at all, and only where the arrangement says the blank goes with the
// command rather than with the redirection.
func (p *printer) redirsAfter(rs []*Redirect, blank bool) {
	for _, rd := range rs {
		if rd.PipeBoth && !p.layout.PipeBothWrittenOut {
			// Nobody wrote this one: it is what the `|&` after the command
			// means, and the pipeline writes that operator back instead —
			// unless the arrangement asked for the meaning rather than the
			// operator, which is what PipeBothWrittenOut says.
			continue
		}
		if blank {
			p.str(" ")
		}
		blank = true
		if rd.Op == TokLessAmp || rd.Op == TokGreatAmp {
			p.dup(rd)
			continue
		}
		if rd.N != nil {
			p.word(rd.N)
		}
		p.str(rd.Op.String())
		if rd.Op.IsSeek() {
			// The operand is an arithmetic command and is written back as
			// one: the span is the same ArithSubst `$((…))` carries, and the
			// general word printer would spell it `$((0))` — which is a word
			// where an arithmetic command has to stand, and does not parse
			// back as a seek at all.
			p.str("((" + seekExpr(rd.Word) + "))")
			continue
		}
		// A space, because the two run together otherwise: `< <(cmd)`
		// written without one is `<<`, a here-document, which is not a
		// redirection with a target at all. The round trip found that; a
		// rule about which characters can combine would have been a guess
		// about the lexer, and this needs none.
		//
		// A here-document is the exception, and it is measured rather than
		// preferred: every shell in the panel that says a body back writes
		// `cat <<XEOF` tight, and none writes `cat << XEOF`. See
		// heredocDelimiterNeedsABlank for the two delimiters that still take
		// the space.
		delim := p.printedWord(rd.Word)
		if rd.Op.IsHeredoc() && rd.Word.IsQuoted() && p.layout.HereDocumentWordSingleQuoted {
			// A quoted delimiter says one thing — the body is literal — so
			// the arrangement that normalizes it writes every spelling the
			// same way. See [Layout.HereDocumentWordSingleQuoted].
			delim = singleQuotedWord(rd.Word.Literal())
		} else if rd.Op.IsHeredoc() && !rd.Word.IsQuoted() {
			// A delimiter is subject to quote removal and to nothing else,
			// so the general word printer's escaping changes what it means:
			// `<<$d` came back as `<<\$d`, which is the same delimiter and a
			// *quoted* one, and a quoted delimiter makes the body literal.
			// `<<$(echo X)` was worse — every character of it escaped.
			//
			// An unquoted delimiter needs no escaping at all and can be
			// written as it stands. Nothing was taken out of it on the way
			// in, which is what "unquoted" means here: had a backslash or a
			// quote removed anything, the span it came from would carry that
			// quoting and this branch would not be taken.
			delim = rd.Word.Literal()
		}
		if !rd.Op.IsHeredoc() || heredocDelimiterNeedsABlank(rd.Op, delim) {
			p.str(" ")
		}
		p.str(delim)
		if rd.Op.IsHeredoc() {
			// The body is not on the line the operator is on — it is the
			// lines after the command — so it is written after everything
			// else this statement has to say. A printer that stopped at the
			// delimiter produced something that *parsed*, and read from the
			// terminal instead of from the body: the failure a round trip
			// through the parser cannot see, and the reason there is a test
			// that runs both.
			p.heredocs = append(p.heredocs, rd)
		}
	}
}

// printedWord is a word written to a string rather than to the output, for
// the one decision that needs the text before it is committed: whether an
// operator can be written tight against what follows it.
//
// The sub-printer carries this one's arrangement and its two word-level
// flags, so the text it produces is the text that would have been written.
func (p *printer) printedWord(w *Word) string {
	// The collector travels with it, because a word written through here is
	// still a word of the program: a redirection target and a here-document
	// delimiter both arrive this way, and one written `$"…"` is a marked
	// string wherever it stands. The lookahead printer in withNext
	// deliberately does not take it — that one writes the same span twice.
	sub := printer{layout: p.layout, raw: p.raw, caseBlanks: p.caseBlanks, translated: p.translated}
	sub.word(w)
	return sub.b.String()
}

// heredocDelimiterNeedsABlank reports whether a here-document's operator has
// to be held off its delimiter by a space.
//
// Only where writing them tight would spell a *longer operator*, which is two
// leading characters and no more: after `<<`, a `-` makes `<<-`, the
// tab-stripping document, and a `<` makes `<<<`, the herestring. Both are a
// different construct reading a different body, printed at status 0.
//
// The dash is the reachable one and has a row of its own; the `<` is the same
// rule rather than a second, and no word this printer writes can begin with
// one — an unquoted `<` is escaped, and a quoted delimiter's first character
// is the quote. It is written out because the rule is about the operator the
// two would spell together, which is a fact about the lexer and not about how
// a word happens to be escaped today.
//
// `<<-` extends into nothing, so a document written with it is always tight.
func heredocDelimiterNeedsABlank(op Kind, delim string) bool {
	return op == TokDLess && delim != "" && (delim[0] == '<' || delim[0] == '-')
}

// dup writes a `<&` or `>&` redirection, which is spelled with no space.
//
// Measured through `type`, which is the only place any shell in the panel
// shows a function body back, and so the only oracle a printer has at all.
// Two things bash does there that the general shape above does not.
//
// It writes the operator tight against its target, whatever the target is:
// `>&$fd` and `>&/dev/null` come back exactly as tight as `2>&1`. The space
// the other operators take is there because `< <(cmd)` written without one
// is `<<`, a different construct — and no dup target can begin an operator,
// so the hazard the space guards against does not arise here.
//
// And it writes out the descriptor being redirected: `>&2` comes back as
// `1>&2` and `<&3` as `0<&3`. Only where the target is one it can read as a
// descriptor now — an unquoted number, or the `-` that closes one. `>&"1"`
// and `>&$fd` are settled when the command runs rather than when it is read,
// and both come back written as they were.
func (p *printer) dup(rd *Redirect) {
	op := rd.Op
	// A close is written the same way whichever operator asked for it, which
	// is the one case where the operator itself changes: `<&-` comes back as
	// `0>&-`. Measured rather than reasoned — it is a normalization, and
	// both spellings close the same descriptor.
	closing := explicitDupTarget(rd.Word) && rd.Word.Literal() == "-"
	if closing {
		op = TokGreatAmp
	}
	switch {
	case rd.N != nil:
		p.word(rd.N)
	case !explicitDupTarget(rd.Word):
	case rd.Op == TokLessAmp:
		// The descriptor is the one the operator as *written* names, even
		// where the operator itself is about to be normalized: `<&-` comes
		// back as `0>&-` and not as `1>&-`.
		p.str("0")
	default:
		p.str("1")
	}
	p.str(op.String())
	p.word(rd.Word)
}

// explicitDupTarget reports whether a dup names a descriptor the reader can
// resolve, which is what decides whether the source descriptor is written
// out. A quoted number does not count: quoting is what tells the two apart.
func explicitDupTarget(w *Word) bool {
	if w == nil || w.IsQuoted() {
		return false
	}
	lit := w.Literal()
	if lit == "-" {
		return true
	}
	if lit == "" {
		return false
	}
	for i := 0; i < len(lit); i++ {
		if lit[i] < '0' || lit[i] > '9' {
			return false
		}
	}
	return true
}

// queueCarriedHeredocs puts a substitution's carried here-document bodies back
// on the enclosing statement's queue, so that the lines the outer lexer read
// as bodies are written back after the command that holds them.
//
// Without it the printer drops them: those redirections are on no node of the
// tree — the sub-parse that found them is thrown away — so nothing else in
// the walk would ever reach them, and `echo $(cat <<EOF)` came back with its
// body and its delimiter gone and a blank line where they had been. See
// [File.CarriedHeredocs] and [Dialect.HeredocBodyFromAfterTheCommand].
//
// Matched on the span's own position, which is how the runner matches them
// too. The list is empty in every dialect that reads a body from between the
// parentheses, so this is a length check on all but a handful of files.
func (p *printer) queueCarriedHeredocs(s Span) {
	for _, c := range p.carried {
		if c.At == s.Pos {
			p.heredocs = append(p.heredocs, c.Redirs...)
			return
		}
	}
}

// flushHeredocs writes the bodies queued by the statement just printed.
//
// Neither newline around a body is the printer's to assume, and assuming each
// of them was a silent corruption: the text came back valid and meant
// something else.
//
// Before the body, because a body goes on lines of its own — but after a
// previous body's delimiter the line is already ended, and a second newline
// opens the next body with a blank first line that was never in it. `cat <<A
// <<B` reads B's body, so that blank line is the whole of the difference:
// every shell in the panel prints `b` for the original and an empty line then
// `b` for what we wrote.
//
// After the body there is no newline to write at all, and that is the part
// the bug report guessed wrong. The delimiter only delimits when it is alone
// on a line, so writing it straight after a body that ended mid-line joins it
// to that line and the body gains the delimiter's text: `cat <<END\nbody`
// came back as `cat << END\nbodyEND\n`, which every shell in the panel runs
// as `bodyEND`. But supplying the missing newline and then the delimiter is
// not the fix either — it gives the body a newline it did not have, which is
// a different body and so a different tree, and the printer's promise is the
// tree. Measured, the difference is real and it is not even the same size in
// every shell: `cat <<END\nbody` writes `body` in dash, ksh93 and zsh and
// `body\n` in bash, so a printer that closed the document would be choosing
// one shell's answer for every caller.
//
// A body ends mid-line only by running to the end of the input, which means
// nothing followed it and nothing may follow it here. So the document is
// written back the way it was read — open — and that is the one form that
// parses to the body it came from.
func (p *printer) flushHeredocs() {
	pending := p.heredocs
	p.heredocs = nil
	for _, rd := range pending {
		if !p.atLineStart() {
			p.str("\n")
		}
		if rd.Heredoc != nil {
			p.str(rd.Heredoc.Literal())
			// The last source line this statement consumed, which its own
			// End does not name: a body is read from the lines *after* the
			// command. Only units asks, and it asks because a gap is the
			// distance from where the statement really ended.
			//
			// One line back from the body's end, which sits at the start of
			// the line *after* the delimiter — so the delimiter's own line
			// is the last one the unit took.
			if end := rd.Heredoc.End().Line - 1; end > p.consumed {
				p.consumed = end
			}
		}
		if !p.atLineStart() {
			return
		}
		// The delimiter alone on its line is what ends it, and it is written
		// bare however it was quoted: the quoting on the operator's word
		// says whether the *body* expands, and the closing line is never
		// quoted in any shell.
		p.str(rd.Word.Literal() + "\n")
		p.oweASkippedSeparator = true
	}
}

// rawWord writes a word whose unquoted punctuation belongs to it.
//
// Inside `[[ … ]]` the lexer reads a word by different rules — parentheses
// and `|` are the pattern's there — so a printer that applies the ordinary
// ones is quoting characters the parser never treated as operators.
func (p *printer) rawWord(w *Word) {
	saved := p.raw
	p.raw = true
	p.word(w)
	p.raw = saved
}

// word writes a word, span by span, putting the quotes back.
//
// Adjacent spans quoted the same way share one pair, which is not decoration:
// `"a $x b"` is three spans and re-emitting each in its own quotes would be
// three words after splitting rather than one.
func (p *printer) word(w *Word) {
	if w == nil {
		return
	}
	if len(w.Spans) == 0 {
		// A word with nothing in it was written as an empty quoted string,
		// and has to be written as one again or it disappears.
		p.str(`''`)
		return
	}
	// A `{ … }` run written immediately after `$$` goes back as it came.
	// Its blanks and operators were characters — see [Span.PidBrace] — and
	// escaping them would leave the tree identical and split the word.
	savedRaw := p.raw
	defer func() { p.raw = savedRaw }()

	for i := 0; i < len(w.Spans); {
		// Only the outer pair of a run is ever marked and a run never nests,
		// so this is a switch and not a depth: `{` turns the raw mode on and
		// the matching `}` — written raw itself — puts back whatever the
		// caller had, which is what lets a word hold two runs and ordinary
		// text between them.
		if w.Spans[i].PidBrace {
			if w.Spans[i].Value == "{" {
				p.raw = true
			} else {
				p.span(w.Spans[i])
				p.raw = savedRaw
				i++
				continue
			}
		}
		q := w.Spans[i].Quoting
		j := i
		for j < len(w.Spans) && w.Spans[j].Quoting == q && q != Unquoted && q != BackslashQuoted {
			j++
		}
		if j == i {
			p.withNext(w.Spans, i, func() { p.span(w.Spans[i]) })
			i++
			continue
		}
		p.quoted(q, w.Spans[i:j])
		i = j
	}
}

// withNext runs write with `after` set to what follows span i, so that a
// parameter can tell whether dropping its braces would swallow it.
func (p *printer) withNext(spans []Span, i int, write func()) {
	saved := p.after
	p.after = ""
	if i+1 < len(spans) {
		var peek printer
		peek.span(spans[i+1])
		p.after = peek.b.String()
	}
	write()
	p.after = saved
}

// quoted writes a run of spans inside one pair of quotes.
func (p *printer) quoted(q Quoting, spans []Span) {
	switch q {
	case SingleQuoted:
		// Nothing expands inside these, so a run of them is always one
		// literal span and the value goes in as it is.
		p.str("'")
		for _, s := range spans {
			p.str(s.Value)
		}
		p.str("'")
	case DollarSingleQuoted:
		if decode := p.layout.AnsiCQuotedWordIsItsValue; decode != nil {
			var b strings.Builder
			for _, s := range spans {
				b.WriteString(s.Value)
			}
			p.str(singleQuotedWord(decode(b.String())))
			return
		}
		p.str("$'")
		for _, s := range spans {
			p.str(s.Value)
		}
		p.str("'")
	default:
		// The `$` of a `$"…"`, which is the whole of what the mark changes
		// about the writing: the run inside is an ordinary double-quoted one
		// and goes back exactly as it would without the mark. Dropping it
		// would hand back a program the one option that lists these strings
		// could not see.
		if spans[0].Translated && !p.layout.TranslatedWordWrittenPlain {
			p.str("$")
		}
		p.str(`"`)
		at := p.b.Len()
		for i := range spans {
			p.withNext(spans, i, func() { p.span(spans[i]) })
		}
		if spans[0].Translated && p.translated != nil {
			// Taken from what was just written rather than from the spans,
			// which is the point of collecting here: the text a listing shows
			// is the text between the quotes, and this is the one place that
			// text exists.
			*p.translated = append(*p.translated, TranslatedString{
				Text: p.b.String()[at:],
				Line: spans[0].Pos.Line,
			})
		}
		p.str(`"`)
	}
}

// span writes one span, without the quotes a run of them shares.
func (p *printer) span(s Span) {
	p.queueCarriedHeredocs(s)
	switch s.Kind {
	case CommandSubst:
		if s.CurrentShell {
			// The third spelling, written back as it was read: the space
			// after the brace is what makes it a command rather than a
			// parameter, and it is already the first byte of the body.
			p.str("${")
			if s.ReplyValue {
				// The fourth, whose marker is *not* body text — the lexer
				// took the `|` off, so this is the one byte that has to be
				// put back rather than reprinted from the value. See
				// Span.ReplyValue.
				p.str("|")
			}
			p.str(s.Value)
			p.str("}")
			return
		}
		if s.Backquoted {
			// Written back the way it was read, because the two spellings
			// end differently: this one ends at its closing backquote, and
			// the other where its contents end. A substitution holding a
			// here-document whose delimiter never matches is terminated by
			// the first and not by the second — which is a real script on
			// this machine, and the last one that would not reprint.
			p.str("`" + escapeBackquoted(s.Value) + "`")
			return
		}
		// A space where the command starts with its own parenthesis and
		// dropping it would change the construct: `$((` opens arithmetic
		// wherever the count closes as `))`, so a body of `(1+2)` written
		// without one comes back an expression. The same trap as a
		// redirection whose target begins with `<`, and found the same way —
		// by printing scripts nobody wrote for this.
		//
		// Asked of the text rather than assumed from the first byte, and
		// through the reader's own rule, so that a body the reader gives back
		// unchanged is written unchanged: `$((echo x) )` is a command
		// substitution in five of the panel's seven shells and printing it as
		// `$( (echo x) )` would edit a script that needed no editing. The two
		// minimal shells have no such reading, and the space is what a body
		// of theirs was written with in the first place — the construct is
		// unreachable there without it.
		// Asked with both delimiter pairs *joined*, which is the reading that
		// finds arithmetic in the most texts and so writes the space in the
		// most. A space nobody needed is layout; a space omitted where some
		// dialect would read the construct back as arithmetic is a changed
		// program. See Dialect.ContinuationPartsTheArithmeticOpener.
		// The quoting half of the same choice: a scan that tracks quoting
		// finds the arithmetic reading in more texts than one that does not,
		// because a `)` inside quotes closes nothing for it. See
		// Dialect.ArithSubstScanIgnoresQuoting, which is the two columns
		// whose scan is blind to it.
		//
		// Asked of the text that is about to be written and not of the text
		// the span holds, because an arrangement that reprints the body
		// decides those characters: `$( (a; b) )` holds a body beginning
		// with a blank and comes back beginning with `(`, and writing it
		// without the space would hand back `$(( a; b ))` — arithmetic, and
		// a different program. Measured, bash 5.3.20 writes `$( ( a; b ))`.
		body := p.commandSubstBody(s.Value)
		if strings.HasPrefix(body, "(") && doubleParenIsArith("$("+body+")", 3, false, false) {
			p.str("$( " + body + ")")
			return
		}
		p.str("$(" + body + ")")
	case ArithSubst:
		// The spelling that was read, for the reason Span.Bracketed gives:
		// the two are one node and are not one syntax, and normalizing them
		// would edit a script rather than print it.
		if s.Bracketed {
			p.str("$[" + s.Value + "]")
			return
		}
		if s.Braced {
			p.str("${((" + s.Value + "))}")
			return
		}
		p.str("$((" + s.Value + "))")
	case ParamExp:
		// `$x` where that is what it means, and `${x}` where the braces are
		// doing something. The braced form is always *correct* — `${x}y` and
		// `$xy` are different words — and always using it is still wrong in
		// a way the tree cannot see: a diagnostic that quotes the target as
		// it was written reads `${e}: ambiguous redirect` where every shell
		// says `$e`.
		if bare, ok := p.bareParam(s); ok {
			p.str("$" + bare)
			return
		}
		p.str("${" + p.ansiCInText(p.paramExpText(s.Value)) + "}")
	case ProcSubstIn:
		p.str("<(" + s.Value + ")")
	case ProcSubstOut:
		p.str(">(" + s.Value + ")")
	case ProcSubstFile:
		p.str("=(" + s.Value + ")")
	default:
		p.literal(s)
	}
}

// commandSubstBody is what goes between the `$(` and the `)`.
//
// The characters the span holds, unless the arrangement has a reader for them
// — see [Layout.CommandSubstitutionIsReprinted], which re-reads the body and
// lays it out through the same listing, and which hands back `false` for a
// body it could not read.
//
// Reached only past the arithmetic guard above, so a body beginning with `(`
// is never re-read: what a reprint hands back decides whether the text comes
// out as `$((` and so whether it re-reads as arithmetic, and the guard's
// question was asked of the characters that are there.
func (p *printer) commandSubstBody(body string) string {
	reprint := p.layout.CommandSubstitutionIsReprinted
	if reprint == nil {
		return body
	}
	if out, ok := reprint(body); ok {
		return out
	}
	return body
}

// paramExpText is what goes between the `${` and the `}`.
//
// The characters the span holds, unless the arrangement has a reader for them
// — see [Layout.ParameterExpansionIsReprinted], which finds the command
// substitutions written in the operand and lays their bodies out through the
// same listing, and which hands back `false` for text it could not read.
//
// Ahead of ansiCInText rather than behind it, because the reader is given
// offsets into the text the span holds and that scan rewrites them: a `$'…'`
// run inside a reprinted body has already been decoded by the printer that
// reprinted it, so there is nothing left there for the scan to find twice.
func (p *printer) paramExpText(text string) string {
	reprint := p.layout.ParameterExpansionIsReprinted
	if reprint == nil {
		return text
	}
	if out, ok := reprint(text); ok {
		return out
	}
	return text
}

// bareParam reports whether a parameter can be written without its braces.
//
// Three conditions, and the third is the one that bites: the arrangement has
// to allow the braces to go, the name has to be one the shell would read
// unbraced, and nothing may follow that would run into it. `${x}y` unbraced is
// the parameter `xy`.
//
// A `{` counts as running into it, and that is measured rather than cautious:
// brace expansion happens before parameter expansion, and in the shell that
// hands its output back to the word as **text** the character a group produced
// continues the name. With `var=baz; varx=vx; vary=vy`, bash 5.3.20 answers
// `${var}{x,y}` with `bazx bazy` and `$var{x,y}` with `vx vy`, so dropping the
// pair there writes a different program (#4200). See
// interp.Semantics.BraceOutputRereadAsText, and note that the two spellings
// were the same program in every column until that axis was measured — which
// is why this had been right for as long as it was.
//
// So in front of a `{` the **spelling** is load-bearing and [Span.Bare] is
// honored whatever the arrangement says: a pair that was written stays, and a
// name that was written bare must not acquire one, since adding the braces
// there changes the program in the same shell and in the other direction.
func (p *printer) bareParam(s Span) (string, bool) {
	if p.layout.ParameterBracesAsWritten && !s.Bare {
		// Written with braces, so it keeps them: the tree records which
		// spelling was read, and a caller that asked for the spelling back
		// is asking for this field to be honored.
		return "", false
	}
	v := s.Value
	if v == "" || !plainParamName(v) {
		return "", false
	}
	if next := p.after; next != "" &&
		(continuesName(next[0]) || next[0] == '{' && !s.Bare && nameCanRunOn(v)) {
		return "", false
	}
	return v, true
}

// nameCanRunOn reports whether a bare parameter's name could take another
// character on the end of it, which only a *name* can: every special and every
// positional parameter a bare `$` may be followed by is one character long, so
// `$#{a,b}` is `$#` with text behind it however the text arrived and `$${a,b}`
// is the run one shell spells that way. It is a name that has more to read —
// `$var` against `$varx` — and so a name that a brace group's output can join.
func nameCanRunOn(v string) bool {
	return v != "" && (v[0] == '_' || (v[0] >= 'a' && v[0] <= 'z') || (v[0] >= 'A' && v[0] <= 'Z'))
}

// plainParamName reports whether a name needs no braces of its own — a name,
// a single digit, or one of the specials.
func plainParamName(v string) bool {
	if len(v) == 1 && strings.IndexByte("@*#?$!-0123456789", v[0]) >= 0 {
		return true
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

// arithForOne is the `1` an omitted expression of an arithmetic `for` stands
// for, written out — see [Layout.EmptyArithmeticForExpressionIsOne].
func arithForOne(text string) string {
	if strings.TrimSpace(text) == "" {
		return "1"
	}
	return text
}

// singleQuotedWord is a value written as a shell word that reads back as that
// exact value: single quotes around it, with each quote in it closed, escaped
// and reopened, which is the one spelling every shell in the panel writes.
func singleQuotedWord(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// escapeInDoubleQuotes protects the characters that still mean something
// inside double quotes, and only where they do.
//
// next is what follows the span, so the decision about its last character is
// made on the text that will really be beside it.
func escapeInDoubleQuotes(value, next string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		following := next
		if i+1 < len(value) {
			following = value[i+1:]
		}
		switch c {
		case '"', '`':
			b.WriteByte('\\')
		case '$':
			if dollarOpensAnExpansion(following) {
				b.WriteByte('\\')
			}
		case '\\':
			if following != "" && strings.IndexByte("\"\\$`\n", following[0]) >= 0 {
				b.WriteByte('\\')
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// dollarOpensAnExpansion reports whether a `$` with this text after it would
// be read as one. Nothing after it is read as one too, since the next thing
// written is not this printer's to know.
func dollarOpensAnExpansion(following string) bool {
	if following == "" {
		return true
	}
	c := following[0]
	return c == '{' || c == '(' || continuesName(c) ||
		strings.IndexByte("@*#?-!$", c) >= 0
}

// ansiCInText rewrites the `$'…'` runs of a piece of text this printer keeps
// as source rather than as spans — a parameter expansion's body — under
// [Layout.AnsiCQuotedWordIsItsValue].
//
// The one engine that decodes these decodes them wherever they are written,
// `${x-$'\t'}` included, because its lexer reads the word before anything
// else looks at it. A scan is what reaches them here, since the braces hold
// text and not a tree.
//
// Three states, because `$'` is only an ANSI-C quote where quoting has not
// already claimed it: ordinary text, inside single quotes, and inside double
// quotes — where `$'` is a dollar sign and a quote in every shell measured.
func (p *printer) ansiCInText(text string) string {
	decode := p.layout.AnsiCQuotedWordIsItsValue
	if decode == nil || !strings.Contains(text, "$'") {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); {
		switch c := text[i]; {
		case c == '\\' && i+1 < len(text):
			b.WriteString(text[i : i+2])
			i += 2
		case c == '\'' || c == '"':
			end := strings.IndexByte(text[i+1:], c)
			if end < 0 {
				b.WriteString(text[i:])
				return b.String()
			}
			b.WriteString(text[i : i+2+end])
			i += 2 + end
		case c == '$' && i+1 < len(text) && text[i+1] == '\'':
			body, end, ok := ansiCBody(text, i+2)
			if !ok {
				b.WriteString(text[i:])
				return b.String()
			}
			b.WriteString(singleQuotedWord(decode(body)))
			i = end
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// ansiCBody reads the text of a `$'…'` that starts at from, reporting where
// the closing quote left off. A backslash takes the character after it,
// which is what keeps `$'a\'b'` one word.
func ansiCBody(text string, from int) (body string, end int, ok bool) {
	for i := from; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case '\'':
			return text[from:i], i + 1, true
		}
	}
	return "", 0, false
}

func continuesName(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// literal writes text, quoting it where leaving it bare would change what it
// means.
func (p *printer) literal(s Span) {
	switch s.Quoting {
	case BackslashQuoted:
		p.str("\\" + s.Value)
	case SingleQuoted:
		p.str("'" + s.Value + "'")
	case DollarSingleQuoted:
		if decode := p.layout.AnsiCQuotedWordIsItsValue; decode != nil {
			p.str(singleQuotedWord(decode(s.Value)))
			return
		}
		p.str("$'" + s.Value + "'")
	case DoubleQuoted:
		// Inside double quotes only four characters still mean anything,
		// and two of them mean it only in front of something. A `$` is an
		// expansion when a name, a brace or a parenthesis follows it and
		// is a dollar sign otherwise, and a backslash protects only the
		// four and a newline — so escaping either one unconditionally
		// writes back text the source never had. `"x$'\\t'"` came back
		// `"x\\$'\\\\t'"`, which is the same value spelled three
		// characters longer, where every shell in the panel writes it
		// exactly as it was written.
		//
		// What follows the last character of the span is the *next*
		// span, which is why this needs p.after: a `$` at the end of a
		// literal with a `$x` after it is two dollars run together.
		p.str(escapeInDoubleQuotes(s.Value, p.after))
	default:
		// A pattern group's text goes back as it came, for the reason
		// `rawWord` above gives for a condition's operands: its `(`, `|`
		// and `)` are the pattern's, and protecting them leaves the tree
		// identical and the meaning gone. The span says so — the byte
		// cannot (#1221).
		if p.raw || s.PatternGroup {
			p.str(s.Value)
			return
		}
		if p.caseBlanks {
			p.str(escapeBareWith(s.Value, bareEscapedKeepingBlanks, true))
			return
		}
		p.str(escapeBare(s.Value))
	}
}

// escapeBackquoted puts back the one layer of backslashes the older
// substitution form requires, which the lexer took off.
//
// The three characters POSIX gives a meaning to there, and no others: a
// backslash before anything else is literal inside backquotes, so escaping it
// would add a character rather than protect one.
func escapeBackquoted(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '$', '`', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// escapeBare protects an unquoted literal.
//
// Not the pattern characters, and not braces: an unquoted `*` in the source
// was a pattern and `{1..3}` was a brace expansion, and both have to stay
// what they were. Escaping them leaves the tree identical and the meaning
// gone, which the behavioral round trip caught and the syntactic one could
// not. What is escaped is everything that would end the word or start
// something else —
// which cannot appear in an unquoted literal span from this parser, and is
// escaped anyway because a tree does not have to have come from a parser.
func escapeBare(s string) string { return escapeBareWith(s, bareEscaped, false) }

// bareEscaped is everything that would end an unquoted word or start
// something else, and bareEscapedKeepingBlanks is the same set without the
// two blanks — the one place a word may hold one bare. See printer.caseBlanks.
const (
	bareEscaped              = " \t\"'\\$`|&;<>()"
	bareEscapedKeepingBlanks = "\"'\\$`|&;<>()"
)

// escapeBareWith is escapeBare over a given set of characters to protect.
//
// bareNewline says a newline may be written as itself, which is the same
// permission keeping the blanks is and comes from the same place — a `case`
// arm's parenthesized pattern list, where both characters are the pattern's.
// See printer.caseBlanks.
func escapeBareWith(s, protect string, bareNewline bool) string {
	if s == "" {
		return "''"
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		// A numeric range is pattern text and not a pair of redirections,
		// so it goes back as it came. It is here for the same reason `*` is
		// left alone above: escaping it would leave the tree identical and
		// the meaning gone — `echo \<-\>` parses, and prints a literal
		// `<->` where the source globbed. The shape is the only thing that
		// says so, since a span carries no note of having been a pattern,
		// and the shape is narrow enough that nothing else wears it.
		if n, ok := numericRangeIn(s[i:]); ok {
			b.WriteString(s[i : i+n])
			i += n
			continue
		}
		if s[i] == '\n' {
			// A backslash before a newline is a *line continuation*, which
			// the next read removes — so escaping it that way is the one
			// case where protecting a character loses it.
			//
			// Reachable only from a `case` arm whose parenthesized pattern
			// list spans one; nothing else in the grammar puts a bare
			// newline inside a word. Where the arm's parenthesis is being
			// written the newline is a character of the pattern exactly as a
			// blank there is, so it goes back as itself — which is what the
			// shell being modeled writes, its own `functions` listing
			// printing an arm of `(a |` ⏎ ` b)` back with the newline bare
			// (#1254). Anywhere else single quotes keep it, and keep it as
			// the same character: a newline is literal text under either
			// quoting, so nothing that matched the word before stops
			// matching it.
			if bareNewline {
				b.WriteByte('\n')
			} else {
				b.WriteString("'\n'")
			}
			i++
			continue
		}
		if strings.IndexByte(protect, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// numericRangeIn is the length of the numeric range at the start of s, or
// false. The lexer's [Lexer.numericRangeAt] reads the same shape off the
// input; this reads it back off a span.
func numericRangeIn(s string) (int, bool) {
	if s == "" || s[0] != '<' {
		return 0, false
	}
	i := 1
	for i < len(s) && isDigitByte(s[i]) {
		i++
	}
	if i >= len(s) || s[i] != '-' {
		return 0, false
	}
	i++
	for i < len(s) && isDigitByte(s[i]) {
		i++
	}
	if i >= len(s) || s[i] != '>' {
		return 0, false
	}
	return i + 1, true
}

// seekExpr is the expression a file-position redirection's operand holds. The
// parser builds that word from one ArithSubst span; anything else reached
// this from a tree built by hand, and the literal text is the best it can do.
func seekExpr(w *Word) string {
	if w != nil && len(w.Spans) == 1 && w.Spans[0].Kind == ArithSubst {
		return w.Spans[0].Value
	}
	if w == nil {
		return ""
	}
	return w.Literal()
}
