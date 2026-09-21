// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"fmt"
	"strings"
)

// Parser builds a syntax tree from tokens.
//
// It is written from docs/spec/grammar/commands.md and carries the lexer's two
// requirements, for the same reason: a shell re-parses the current line on
// every keypress, so it never panics, and unfinished input is distinguishable
// from wrong input because `if x; then` mid-typing is normal.
type Parser struct {
	lex     *Lexer
	dialect Dialect

	tok Token

	// Aliases answers what a word stands for, and is the caller's table
	// rather than one this package keeps — see alias.go. Nil expands
	// nothing.
	Aliases Aliases

	// GlobalAliases answers for the kind expanded wherever a word stands
	// rather than only where a command word does, and SuffixAliases for the
	// kind keyed on a command word's extension. Both are the caller's
	// tables, both are nil in a dialect without the kind, and one dialect in
	// the panel has either — see interp.Semantics.GlobalAliases.
	GlobalAliases Aliases
	SuffixAliases Aliases

	// pending are tokens an alias expansion put in front of the lexer, and
	// aliasNextWord says the last expansion ended in a space, so the word
	// after it is eligible in turn.
	pending       []Token
	aliasNextWord bool
	// literalReading is what the declaration command now being read has
	// settled about the parentheses of its next operand's literal, where the
	// dialect has a construct its letters can settle. Set by
	// declarationArray around one call and consumed by parseAssign, which
	// clears it so that a nested literal is read on its own terms. See
	// [compoundLiteralReading].
	literalReading compoundLiteralReading
	// pendingTouches runs beside pending: whether each token was written
	// with nothing at all between it and the one in front of it *in the
	// alias body*. It cannot be recovered from the tokens themselves,
	// because every spliced token is given the position of the word it
	// replaced — see spliceAlias — so their offsets say nothing about what
	// stood between them.
	//
	// One question in the grammar asks it: `a=(x y)` is an array and
	// `a= (x y)` is not, and the parenthesis has to touch the `=`. Without
	// this an alias whose body is a compound assignment was refused outright
	// — `alias f='a=(x y)'; f` is an unexpected `(` here and an array in
	// bash, ksh93 and zsh (#2299).
	pendingTouches []bool
	// pendingChains runs beside pending too: the names of the expansions
	// each token came *out of*, innermost last. A token the lexer read
	// belongs to none and carries nil.
	//
	// It is what stops a body that names itself past a separator. aliasDone
	// is the set for one *command*, and a body holding `;` is more than one
	// command, so `alias a='echo took;a'` re-expanded `a` in the second of
	// them and did it forever — a hang, where every shell in the panel
	// prints `took` and then `a: not found`. The set cannot simply be kept
	// for the whole body either: `alias e=echo` with `alias a='e X; e Y'`
	// expands `e` twice, also unanimous. What is spent is the chain of
	// expansions still *open* around the token, and that is per token rather
	// than per command or per body, so it travels with the token.
	pendingChains []map[string]bool
	// pendingTails runs beside pending as well: for each token, the text
	// that still follows it — the rest of the body it came from, and then
	// the rest of every body that one was spliced into, in order. Empty for
	// a token the lexer read, whose remaining text is the input itself.
	//
	// It is what lets a construct an alias body leaves open be continued
	// over the *enclosing* body rather than only over the input. A token
	// keeps no text, which is the whole of the token model, so a body one
	// level in had nothing to be joined to and #2685's seam was crossed at
	// the outermost level only. This is the text each token came from, kept
	// as the queue's own column, and [Parser.carryOpenWord] reads it (#2709).
	pendingTails []string
	// tokTail is pendingTails' entry for the current token: the text that
	// follows it across every body it is inside. Empty for a token of the
	// input.
	tokTail string
	// pendingCarries runs beside pending for the one token of a splice that
	// may still be reading: the body's unfinished tail, or the empty string
	// for every token that is not it.
	//
	// The carry is deferred to the moment that token is *handed out* rather
	// than performed when the body is spliced, and the reason is order. A
	// body's own tokenization is not final while an alias word stands
	// earlier in it: `alias b='echo "'` with `alias a='b x"'` lexes a's body
	// as a word `x` and a quote nothing closes, and the quote that closes it
	// is the one `b` is about to contribute. Carrying at splice time reads
	// that open quote over the input and swallows the rest of the file;
	// carrying when the token is reached lets `b` expand first, and its own
	// carry then takes the `x"` out of the queue (#2709).
	pendingCarries []string
	// tokTouches is the same fact about the current token, and is false for
	// a token read from the input, where the offsets answer directly.
	tokTouches bool
	// aliasChain is pendingChains' entry for the current token: the names
	// whose bodies this token is inside. Nil for a token of the input.
	aliasChain map[string]bool
	// aliasSpliced counts the tokens of the current expansion still in hand,
	// so the trailing-space rule can tell a word that *came from* the value
	// from the word that follows it.
	aliasSpliced int
	// aliasSource is the body of the expansion those tokens came from, for
	// the diagnostic that echoes the text a failure was inside rather than
	// the line the script wrote. See Error.AliasSource.
	aliasSource string
	// aliasLineShift totals the lines every expansion so far has added to
	// the input, where the dialect counts an alias body's newlines. Reported
	// by LineShift, for a caller that parses a program in pieces and has to
	// carry the numbering from one piece to the next.
	aliasLineShift int
	// flagTailFrom is the byte offset, one past a refused expansion flag
	// group's `)`, that the rest of the word is read from — set by
	// parseParamExp, which has the braces alone, and spent by newWord, which
	// is the one place that knows where the word ends. Zero when no flag
	// group has been refused. See Error.FlagGroupWordTail.
	flagTailFrom int32
	// refused is a failure that gives up the line being read rather than the
	// file, held here from the moment it is raised until NextLine hands it
	// back on the File. See File.Refused.
	refused error
	// lineSubsts are the command substitutions read since the last line was
	// handed back, for the dialects that parse a body with its line. Nil
	// under every other dialect, where nothing is gathered at all. Drained by
	// NextLine onto the File. See File.Substitutions.
	lineSubsts []Span
	// quotedInTheScriptsRead are the runs of the fragment now being re-read
	// that the script's own read had inside single quotes, as byte offsets
	// into that fragment. Nil while the script itself is being read, where there is no
	// second reading to disagree with. See Parser.hiddenFromTheScriptsRead.
	quotedInTheScriptsRead [][2]int
	// allHiddenFromTheScriptsRead says the fragment being re-read stood
	// inside such a run in its own turn, so everything under it was hidden
	// from the script's read whatever this fragment's own quoting says.
	allHiddenFromTheScriptsRead bool
	// aliasDone are the names already expanded in the command being read. It
	// is a field rather than a local because the command word is not always
	// reached from one place: assignment prefixes stand in front of it, so
	// the word after them is expanded in parseSimple while the first word was
	// expanded in parseCommand, and the two have to share a set or
	// `alias al='y=1 al'` expands forever. Measured: that alias is
	// `al: not found` in ksh93 and zsh alike, so the inner name is spent.
	aliasDone map[string]bool
	// globalDone is the same set for one word's *global* expansion, which is
	// per word rather than per command. Kept on the parser and cleared
	// rather than allocated each time, because it is reached from next.
	globalDone map[string]bool
	// aliasPrimed records that the first token has been offered to the
	// global-alias hook. See primeAliases.
	aliasPrimed bool

	// aliasHeadHandled says the *current* token has already been offered to
	// the alias table as a command word, by parsePipeline rather than by
	// parseCommand. It is about that one token and nothing else, which is
	// why next clears it.
	//
	// Two words are read one level out from a command — a pipeline's
	// leading `!` and the `time` in front of it — so the table has to be
	// consulted before either is answered. parseCommand would otherwise
	// offer the same word a second time, and a second offer is not
	// harmless: it begins a fresh set of spent names and clears the
	// trailing-blank flag the first one set, which is what makes
	// `alias '!'='echo '` leave the word after it eligible. See
	// [Parser.expandPipelineHead].
	aliasHeadHandled bool

	// aliasFuncRefused says the command word about to be read as a function
	// name is one the alias table holds, in the dialect that refuses such a
	// definition. Set where the expansion is declined and spent at the
	// parentheses, which is where the refusal is located — see
	// [Dialect.AliasRefusesAFunctionName].
	aliasFuncRefused bool

	err        error
	incomplete bool

	// open is the constructs the parser is inside, innermost last, and
	// lastText the token before the current one. Both exist for one reason:
	// when the input runs out, the panel names four different parts of that
	// state and this is where they come from.
	open     []opener
	lastText string

	// openAtEnd is what open held when the input ran out, kept because the
	// stack is unwound by the time Parse returns: opens closes by deferring,
	// so a caller that asked afterwards would always be told nothing was
	// open. A prompt asking what it is waiting for asks afterwards.
	openAtEnd []opener

	// funcBody says the command about to be parsed is a function's body, so
	// that a brace group standing as one is recorded as the function rather
	// than as a group. They are the same syntax and not the same thing to
	// someone being told what is still open.
	funcBody bool

	// depth bounds nesting while parsing operands, which are themselves
	// words and may hold further expansions. Pathological input is the
	// normal case on the keystroke path, so this is a bound rather than a
	// trust.
	depth int

	// bodyTookTerm is where a command's own body took the separator that
	// would otherwise have ended the statement around it, and it is the one
	// way a statement is terminated without holding the terminator itself.
	//
	// A short loop body is a whole statement, terminator and all: the `;` in
	// `for i (a b) echo $i; echo end` belongs to `echo $i`, and there is
	// nothing left between the loop and `echo end` — so the loop is
	// terminated by it too, and the list may go on. A `do … done` or a brace
	// body is closed by its own word instead and leaves the statement
	// unterminated, which is measured: the shell with short loops parses
	// `for i (a b) echo $i; echo end` and refuses
	// `for i (a b) { echo $i; } echo end`.
	bodyTookTerm Pos

	// bodyTookKind is which token that was — see [Terminator]. It travels
	// with bodyTookTerm because the two are one fact read two ways, and a
	// statement that inherits the position without the kind is a statement
	// whose listing cannot say what separated it from the next one.
	bodyTookKind Terminator

	// shortBodyBraced says whether the short body just read was a brace
	// group rather than the single command the same production also allows.
	//
	// The two are one production everywhere but here: a short `if` whose
	// last arm is a brace group may be closed with a redundant `fi`, and one
	// whose last arm is a bare command may not. Measured on zsh 5.9.2 —
	// `if (( 1 )) { echo A } fi` runs, `if (( 1 )) (( 2 )) fi` is a parse
	// error near `fi`, and `if (( 1 )) echo A fi` prints `A fi`, the word
	// never having been in command position at all. See
	// [Parser.redundantFi].
	shortBodyBraced bool

	// substitutionBody says the text this parser was handed is the inside of
	// a substitution, so the end of the input is that construct's closing
	// delimiter. See [Parser.InsideASubstitution], which is the only thing
	// that sets it.
	substitutionBody bool

	// separatorStood is where a `;` the dialect stepped over stands, while
	// it is still the innermost thing the parse is inside.
	//
	// Measured on ksh93u+ 2026-09-12, `-n` over a script file ending where
	// it is shown. A separator that was stepped over becomes the token that
	// shell names when the input then runs out, in place of the keyword it
	// otherwise names:
	//
	//	{ :                     `{' unmatched
	//	{ : ;                   `{' unmatched   — a *terminator* is not this
	//	{ ;                     `;' unmatched
	//	{ ; :                   `;' unmatched   — and more input does not clear it
	//	( ;                     `;' unmatched
	//	if :; then ;            `;' unmatched
	//	while :; do ;           `;' unmatched
	//	x() { ; 	        `;' unmatched
	//	{ false || ;            `;' unmatched
	//	{ false && ;            `;' unmatched
	//	{ : |& ;                `;' unmatched
	//	if false || ; then      `;' unmatched   — a clause does not displace it
	//	{ ; } ; if :; then      `then' unmatched — but its construct closing does
	//
	// #1207 filed this as the `;` that admitted an empty **and-or** operand.
	// It is not: `{ ; ` has no and-or in it and answers the same, so it is
	// the step-over and not the operator.
	//
	// **A `case` arm is the exception and is measured, not assumed.**
	// `case x in x) ;` and `case x in x) false || ;` both answer
	// `` `case' unmatched `` there. Two neighbors of that are left
	// unmodeled deliberately: once the arm's `;;` has been read the `;` is
	// named again, and with a newline between them the `;;` is named — and
	// that last answer comes back for `case x in x) : ;` + newline + `;;`,
	// which has no stepped-over separator in it at all, so it is a fact
	// about the terminator rather than about this.
	//
	// **Diagnostic only.** It is deliberately not an entry on p.open, so it
	// never reaches Parser.Open() and never reaches a continuation prompt.
	// That shell has no open-state prompt escape, so what it prompts here
	// cannot be measured, and #1207 is explicit that guessing both halves at
	// once is how a wrong rule gets into the tables.
	separatorStood Pos

	// terminatorStood is where a `case` arm's terminator — `;;` or `;&` —
	// stands when a command separator stood in front of it, and armTerminator
	// is how that terminator was spelled.
	//
	// The same question separatorStood answers, one construct over, and the
	// two neighbors that field's comment left unmodeled are what this is.
	// Measured on ksh93u+ 2012-08-01, 2026-09-12, `-n` over a script file
	// ending where it is shown:
	//
	//	case x in x) : ;;        `case' unmatched  — nothing in front of it
	//	case x in x) : ;&        `case' unmatched
	//	case x in x) : ; ;;      `;;' unmatched
	//	case x in x) : & ;;      `;;' unmatched
	//	case x in x) : ⏎ ;;      `;;' unmatched    — a newline counts too
	//	case x in x) : ; ⏎ ;;    `;;' unmatched
	//	case x in x) : ; ⏎ ;&    `;&' unmatched
	//	case x in x) ; ;;        `;' unmatched     — the arm has no command
	//	case x in x) : ; ;; y) : ;
	//	                         `case' unmatched  — the next arm clears it
	//
	// #2233 filed the pair as "the newline may be what decides". It is not:
	// row three has no newline in it and answers the same as row five, and
	// row one has neither and answers `case`. What decides is a separator —
	// `;`, `&` or a newline — standing between the arm's last command and
	// its terminator. Row eight is separatorStood's and keeps that answer:
	// an arm with no command at all is the step-over that field measures,
	// and it outranks this.
	//
	// **Diagnostic only**, for the reason written on separatorStood: that
	// shell has no open-state prompt escape, so what it prompts cannot be
	// measured and this never reaches Parser.Open().
	terminatorStood Pos
	armTerminator   string

	// inCondition says the list about to be read is a keyword's condition,
	// where one dialect refuses the `;` it steps over elsewhere. Set by
	// parseCondition and cleared by the parseList that reads it.
	inCondition bool

	// inCaseWord says the token about to be refused stands where a `case`
	// wants a word — its **subject**, and an arm's **pattern**, the latter
	// after the arm's optional `(` and before the `)` that closes the list.
	// One dialect reads the end of the input in either place as the newline
	// that would have ended the line, and those are the only positions it
	// does that in. See Dialect.CaseWordRunsOutAsANewline, where the six
	// columns are, and failUnexpectedAt, which is the one place that reads
	// this.
	//
	// A field rather than a test at each refusing site because there are
	// four of them — the subject, the list that never reached a `)`, the
	// alternative after a `|` that never arrived, and the `(` with nothing
	// behind it — and a rule written out at some of them is the shape of
	// defect this tree keeps producing.
	// condWords are the words of the `[[ ]]` group being read, as each was
	// written, for the one dialect whose refusal counts them. Rebuilt at
	// every condPrimary — which is where each group begins — and read by
	// Parser.recordCondGroup. See Error.CondWords.
	condWords []string

	// condStart is where the `[[` now being read stood, for the refusals
	// raised below the frame that knows it. See Parser.failCondTerm.
	condStart Pos

	// condGroups is how many `(` of the condition being read have been
	// entered and not yet closed. One dialect writes a line per open group
	// in front of a refusal; see Error.CondGroupsOpen.
	condGroups int

	inCaseWord bool
}

// maxParamDepth is how far `${x:-${y:-…}}` may nest before the parser stops.
// Deep enough that no real script reaches it, shallow enough that no input
// can exhaust the stack.
const maxParamDepth = 64

// NewParser returns a parser over src.
func NewParser(src string, d Dialect) *Parser { return NewParserAt(src, d, 1) }

// NewParserAt returns a parser over src whose first line is numbered first.
//
// Input does not always arrive whole. A shell reading its program from a
// descriptor takes a piece of it, runs that, and comes back for more — the
// piece is all the parser has, and a diagnostic still has to name the line of
// the *input* rather than the line of the piece. Nothing else changes: an
// offset stays relative to src, because it is only ever used to quote the text
// a node was written as, and that text is here.
//
// first is the number to give src's first line, so 1 is NewParser.
func NewParserAt(src string, d Dialect, first int) *Parser {
	lex := NewLexer(src, d)
	if first > 0 {
		// Before the first token is read: the lookahead carries a position,
		// and a line set afterwards would leave that one token behind.
		lex.line = first
	}
	return newParserOn(lex, d)
}

// newParserOn wraps a lexer the caller has already set up.
//
// It exists because the first token is read here, before anything outside can
// reach the lexer: a caller with something to say to it — [ShellWords], which
// has to install its recorder and stop here-document bodies being read — has
// to say it before that, or the very first token is lexed under the wrong
// settings and never recorded.
func newParserOn(lex *Lexer, d Dialect) *Parser {
	p := &Parser{lex: lex, dialect: d}
	p.next()
	return p
}

// EndsWithContinuation reports whether text ends with a backslash joining it to
// a line that has not arrived yet.
//
// A parser cannot answer this, and is right not to. `echo one \` at the end of
// a *file* is a finished command — the continuation joins it to nothing — while
// the same text at the end of what has been read so far is a command still
// being written. Which one it is depends on whether there is more input, which
// only the thing doing the reading knows.
func EndsWithContinuation(text string) bool {
	text = strings.TrimSuffix(text, "\n")
	n := 0
	for i := len(text) - 1; i >= 0 && text[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// Parse parses src completely.
func Parse(src string, d Dialect) (*File, error) {
	p := NewParser(src, d)
	f := p.Parse()
	return f, p.Err()
}

// Err reports why parsing stopped, or nil.
func (p *Parser) Err() error { return p.err }

// SetDialect replaces the dialect for input that has not been read yet.
//
// A shell can change its own grammar while it runs: a builtin executed on one
// line decides whether a quantified group is a group on the next. The parser
// cannot know that — the builtin runs in an interpreter this package has never
// heard of — so the front end, which is the only place the two meet, reads the
// interpreter's answer between lines and hands it in here. The same joint as
// Aliases, for the same reason.
//
// It applies to what has not been tokenized. The parser holds one token of
// lookahead, so the very first token of the next line may have been read under
// the old dialect; every construct a runtime toggle governs sits deeper in a
// line than its first token, which is what makes the boundary safe.
func (p *Parser) SetDialect(d Dialect) {
	p.dialect = d
	p.lex.dialect = d
}

// InsideProgramParentheses tells this parser that the text it is about to read
// is the inside of parentheses holding a program — the body of a `$( )`,
// already cut out of the script it came from.
//
// The parser sets this for itself where it reads such a body in place (see
// [Lexer.parseToClose]); what it cannot know is that a *caller* has handed it
// a body it cut earlier, which is what an interpreter does when it parses a
// substitution's text at expansion time. Told nothing, that read has no
// parentheses around it and a here-document in it runs to the end of the text
// where the shell being modeled ends it at the delimiter (#785, #1021).
//
// The counterpart of [Parser.SetDialect]: a fact about the text this parser
// was given, handed in by the only place that knows it, and read for what has
// not been tokenized yet.
func (p *Parser) InsideProgramParentheses() { p.lex.inProgramParens = true }

// InsideASubstitution tells this parser that the text it is about to read is a
// substitution's body — the inside of a `$( )` or of a pair of backquotes,
// already cut out of the script it came from — so that the end of this input
// is that construct's closing delimiter rather than the end of a program.
//
// The distinction is only visible where a closing context is *lenient*.
// [Dialect.OpenEndedAndOr] is the one that reaches it, and its own
// documentation has always named "a command substitution's `)`" among the
// places an and-or may end on its operator — which nothing could ever take,
// because the parenthesis is cut off before this text is handed over and
// [Parser.atListEnd] therefore sees the end of the input instead. Measured
// 2026-09-20 on zsh 5.9.2 from script files:
//
//	v=$(echo x &&); print -r -- "[$v]"       [x]
//	v=$(echo x ||); print -r -- "[$v]"       [x]
//	v=$( : || );    print -r -- "[$v]"       []
//	v=`echo x &&`;  print -r -- "[$v]"       [x]
//	v=$(echo x |);  print -r -- "[$v]"       refused — the pipeline never takes it
//	v=$(echo x ;;); print -r -- "[$v]"       refused — nor does a stray terminator
//
// Deliberately narrower than "the input cannot be extended". Whether the end
// of a *script* or of a `-c` string ends such a list is a route question that
// [Dialect.OpenEndedAndOr] parks — zsh takes `echo x &&` from a file and still
// draws a continuation prompt for the same text typed at a terminal — and
// nothing here answers it. A substitution's body has no such second reading:
// its end is a delimiter the script wrote.
//
// The counterpart of [Parser.InsideProgramParentheses], and a separate fact
// from it: that one is about `$( )` alone, for a here-document rule the
// backquoted spelling measurably does not take, and this one is about both.
func (p *Parser) InsideASubstitution() { p.substitutionBody = true }

// Incomplete reports whether the input ended part-way through a construct that
// could still be finished. A prompt should ask for another line.
func (p *Parser) Incomplete() bool { return p.incomplete || p.lex.Incomplete() }

// LineShift is how many lines the alias bodies this parser expanded added to
// the numbering, where [Dialect.AliasBodyCountsLines] says they count.
//
// A caller that reads a program in pieces — one that arrives on a descriptor —
// retires a piece by counting its newlines and starting the next parser after
// them. That count is of the text, and an expanded alias contributed lines
// that were never in the text, so without this the numbering resets at the
// first refill.
func (p *Parser) LineShift() int { return p.aliasLineShift }

// slice returns the source between two positions, which is how a node keeps
// the text it was written as. A diagnostic quotes what the author typed, and
// no reconstruction from the tree can be relied on to match it.
func (p *Parser) slice(from, to Pos) string {
	if from.Offset < 0 || int(to.Offset) > len(p.lex.src) || from.Offset > to.Offset {
		return ""
	}
	return p.lex.src[from.Offset:to.Offset]
}

func (p *Parser) next() {
	if p.tok.Kind != TokEOF && p.tok.Text != "" {
		p.lastText = p.tok.Text
	}
	// Whatever parsePipeline offered to the alias table, it offered the
	// token that is about to stop being current. Cleared here rather than
	// where it is read, so that the flag cannot outlive the word it
	// describes: `alias '!'='! x'` expands at the head, the `!` the body
	// begins with is then read as the negation, and `x` behind it is a
	// command word that has never been offered to anything.
	p.aliasHeadHandled = false
	if p.aliasSpliced > 0 {
		p.aliasSpliced--
		if p.aliasSpliced == 0 {
			p.aliasSource = ""
		}
	}
	if len(p.pending) > 0 {
		// An alias expansion is still being handed out. Nothing else about
		// the input has moved, so the lexer is not touched.
		p.tok, p.pending = p.pending[0], p.pending[1:]
		p.tokTouches, p.pendingTouches = p.pendingTouches[0], p.pendingTouches[1:]
		p.aliasChain, p.pendingChains = p.pendingChains[0], p.pendingChains[1:]
		p.tokTail, p.pendingTails = p.pendingTails[0], p.pendingTails[1:]
		carry := p.pendingCarries[0]
		p.pendingCarries = p.pendingCarries[1:]
		if carry != "" {
			// The one token of its splice that may still be reading, now
			// that everything in front of it has had its turn. See
			// Parser.pendingCarries for why it waits until here.
			p.carryOpenWord(&p.tok, carry)
		}
		// A global alias inside an alias body is expanded in turn — measured
		// `alias -g B=x; alias -g H='a B'` gives `a x` — so the tokens being
		// handed out are asked as well as the ones being read. Not a fresh
		// chain: these tokens are the expansion's, and the names it has
		// already spent are spent for them.
		p.expandGlobalAlias(false)
		return
	}
	p.tok = p.lex.Next()
	// A token of the input answers the adjacency question from its own
	// offset, so nothing here has to be remembered for it, and it is inside
	// no expansion: whatever a body spent is spent only for the body, and
	// the text that follows it is the input rather than any body's.
	p.tokTouches = false
	p.aliasChain = nil
	p.tokTail = ""
	if p.err == nil && p.lex.Err() != nil {
		p.err = p.lex.Err()
	}
	// A global alias is expanded wherever a word stands, so the question is
	// asked of every token rather than at the handful of places a command
	// word is read. A token read here begins a chain of its own. See
	// expandGlobalAlias.
	p.expandGlobalAlias(true)
	if p.lex.Incomplete() {
		// The lexer ran out inside a quote or an expansion. The parser may
		// never fail over it — a word that never finished is still a word —
		// so the snapshot has to be taken here, while the constructs around
		// it are still on the stack. Taken once, like every other.
		p.ranOut()
	}
}

// primeAliases offers the *first* token of the input to the global-alias
// hook, which could not have seen it when it was read.
//
// [newParserOn] reads that token in the constructor, deliberately — a caller
// with something to say to the lexer has to say it before anything is lexed —
// and the three alias hooks are plain fields the caller sets afterwards. Every
// later token passes through [Parser.next] with the tables in hand; this one
// is the exception, and without this a global alias written as the first word
// of a parse is the one word that never expands. Command position is not
// affected, because the table there is read when the command is parsed rather
// than when its word was lexed.
//
// Once, because a second pass over a token already expanded would spend its
// names again: `alias -g S='S x'` would come out `S x x`.
func (p *Parser) primeAliases() {
	if p.aliasPrimed {
		return
	}
	p.aliasPrimed = true
	p.expandGlobalAlias(true)
}

func (p *Parser) at(k Kind) bool { return p.tok.Kind == k }

// atWord reports whether the current token is the given reserved word, written
// unquoted. Quoting removes the reservation: `"if"` is a command name.
func (p *Parser) atWord(s string) bool {
	return p.tok.Kind == TokWord && !p.tok.IsQuoted() && p.tok.Literal() == s
}

// stopWords end a list. They are only reserved where a command may begin,
// which is exactly where parseList tests them: `echo if then` passes them
// through as arguments because parseSimple never asks.
var stopWords = map[string]bool{
	"then": true, "elif": true, "else": true, "fi": true,
	"do": true, "done": true, "esac": true, "}": true,
}

// reservedWords is every word the grammar reserves, which is the stop words
// plus the ones that open a construct. It is the class distinction one
// dialect's wording turns on: `fi` is quoted there and `echo` is "word".
var reservedWords = map[string]bool{
	"if": true, "then": true, "elif": true, "else": true, "fi": true,
	"for": true, "while": true, "until": true, "do": true, "done": true,
	"case": true, "in": true, "esac": true, "{": true, "}": true,
	"function": true, "select": true, "time": true,
	// `!` is one too, and it was missing. It shows only where a refusal
	// names it — a second `!` in a dialect that will not toggle them — and
	// the shell that draws the class distinction quotes it there:
	// `` "!" unexpected `` in dash, against `word unexpected` for a name.
	"!": true,
}

// reservedInDialect reports whether name is a word *this* dialect reserves.
//
// [reservedWords] is the union, because the class distinction a diagnostic
// draws is the same in every dialect. Three of its members are constructs a
// preset adds, and an alias expansion is the one caller that has to tell them
// apart: a shell with no `select` loop has nothing to protect the word for,
// and dash, which has none of the three, takes an alias for all three where
// ksh93 takes none. See [Dialect.AliasesExpandReservedWords] for the panel.
func (p *Parser) reservedInDialect(name string) bool {
	return p.dialect.reservesWord(name)
}

// atReservedWord reports whether the current token is one of the words the
// grammar reserves, written unquoted. Quoting removes the reservation exactly
// as it does for [Parser.atWord]: `"if"` is a command name.
func (p *Parser) atReservedWord() bool {
	return p.tok.Kind == TokWord && !p.tok.IsQuoted() && reservedWords[p.tok.Literal()]
}

// atReservedPrecommand reports whether the current token is one of the words
// the dialect takes away in front of a command — see
// [Dialect.ReservedPrecommands]. Quoting removes the reservation, exactly as
// it does for `if`: `\nocorrect echo hi` is `command not found`.
func (p *Parser) atReservedPrecommand() bool {
	if p.tok.Kind != TokWord || p.tok.IsQuoted() {
		return false
	}
	return p.dialect.ReservedPrecommands[p.tok.Literal()]
}

func (p *Parser) atStopWord() bool {
	if p.tok.Kind != TokWord || p.tok.IsQuoted() {
		return false
	}
	// `end` closes a `foreach`, and it is reserved wherever a command may
	// begin rather than only inside one: measured, `end` alone and
	// `end() { :; }` are both parse errors in the shell that has the loop,
	// while `echo end` and `end=5` are not. So it is a stop word for that
	// dialect and an ordinary word for the other four.
	if p.dialect.Foreach && p.tok.Literal() == "end" {
		return true
	}
	return stopWords[p.tok.Literal()]
}

// atListEnd reports whether the current token closes the list the parser is
// reading, rather than being something a command could begin with.
//
// The tokens are the ones every enclosing construct stops on: a stop word,
// the `)` of a subshell or a `case` arm's pattern list, and a `case`
// terminator. The end of input is deliberately not one of them — a caller
// asking this is deciding whether a list *ended*, and input that ran out has
// not ended, it has stopped, which is a separate answer with a continuation
// prompt attached to it.
//
// A terminator is not one either. `;` and `&` end a statement rather than the
// list holding it, and the shell that takes an and-or with no right-hand side
// splits exactly there: measured 2026-09-07 on zsh 5.9.2, `{ true || ⏎ }`,
// `( true || )`, `if x; then true || fi`, `while …; do true || done` and
// `case x in x) true || ;; esac` all run, and `true || & b` is a parse error
// naming the `&`.
func (p *Parser) atListEnd() bool {
	switch p.tok.Kind {
	case TokRightParen, TokDSemi, TokSemiAmp, TokDSemiAmp, TokSemiPipe:
		return true
	}
	return p.atStopWord()
}

// atASubstitutionCloser reports whether the end of this input is a
// substitution's closing delimiter that was cut off before the text arrived.
//
// It is [Parser.atListEnd]'s missing row rather than a rule of its own: the
// `)` of a `$( )` and the second backquote both close a list, and neither is
// in the text a body is parsed from. It is asked beside atListEnd rather than
// folded into it because only one caller has been measured — see
// [Parser.InsideASubstitution] for the rows, and for why the end of a script
// is a different question.
func (p *Parser) atASubstitutionCloser() bool {
	return p.substitutionBody && p.at(TokEOF)
}

// bareNegationStandsHere reports whether a `!` that has just been read may be
// the whole of the pipeline, the dialect's reach deciding where.
//
// The end of input is in every accepting set, and it is in this one flag
// rather than behind a route question the way [Dialect.OpenEndedAndOr]'s is:
// `!` at the end of a `-c` string and at the end of a script file answer
// alike in all three shells that take it, measured 2026-09-12. What a
// *terminal* does with it has not been measured, and nothing here depends on
// it — a bare `!` is a finished pipeline either way, where an and-or ending
// on its operator is a line a shell may still be waiting to complete.
func (p *Parser) bareNegationStandsHere() bool {
	terminator := p.at(TokSemi) || p.at(TokNewline) || p.at(TokEOF)
	switch p.dialect.BareNegationReach {
	case BareNegationBeforeATerminator:
		return terminator || p.at(TokAmp)
	case BareNegationWhereAListEnds:
		return terminator || p.atListEnd() || p.at(TokAndAnd) || p.at(TokOrOr)
	case BareNegationAtEitherPlace:
		return terminator || p.at(TokAmp) || p.atListEnd() ||
			p.at(TokAndAnd) || p.at(TokOrOr)
	}
	return false
}

// bareFunctionKeywordStandsHere reports whether the `function` keyword that
// has just been read is the whole of the command.
//
// The set is measured on [Dialect.BareFunctionKeyword], and it is a set
// rather than "anything that is not a name" in both directions. A
// redirection counts, because the form takes one and applies it to a body
// that runs nothing. The background operators do not, in any spelling, which
// is the same boundary [Dialect.BareNegationReach] draws at
// [BareNegationWhereAListEnds]. And of the reserved words only `}` counts:
// every other one is read as the name this form does not have, so a keyword
// written before `fi`, `done` or `esac` is a refusal there and must stay one
// here.
func (p *Parser) bareFunctionKeywordStandsHere() bool {
	switch p.tok.Kind {
	case TokSemi, TokNewline, TokEOF, TokRightParen,
		TokPipe, TokPipeAmp, TokAndAnd, TokOrOr,
		TokDSemi, TokSemiAmp, TokDSemiAmp, TokSemiPipe:
		return true
	}
	if p.tok.Kind.IsRedirect() || p.at(TokIONumber) {
		return true
	}
	return p.atWord("}")
}

// tokenText names a token the way a diagnostic should: the word itself when
// there is one, and the operator's spelling otherwise.
func tokenText(tok Token) string {
	if tok.Kind == TokWord {
		return `"` + tok.Literal() + `"`
	}
	return `"` + tok.Kind.String() + `"`
}

// opener is a construct or clause the parser is currently inside.
//
// It is kept so that running out of input can be *described* rather than just
// reported: the panel names four different parts of that state — see
// ErrUnterminated — and all four are here.
type opener struct {
	word string
	line int
	// construct marks a compound command rather than a clause of one. `if`
	// is a construct and the `then` inside it is not, which is the
	// distinction one shell's wording turns on.
	construct bool
}

// opens records a construct and returns the function that closes it.
//
// The close truncates rather than pops, so a clause opened inside it — `then`,
// `else` — needs no unwinding of its own and an early return cannot leave the
// stack out of step with the parse.
func (p *Parser) opens(word string) func() {
	depth := len(p.open)
	// A separator stood over inside this construct belongs to it and goes
	// with it. `{ ; } ; if :; then` names the `then` in the shell that names
	// one of these, where `{ ; ` on its own names the `;` — measured.
	stood := p.separatorStood
	p.separatorStood = Pos{}
	// And an arm terminator recorded inside this construct goes with it, for
	// the same reason: `case x in x) : ; ;; esac` then `{` names the `{`.
	termStood, termText := p.terminatorStood, p.armTerminator
	p.terminatorStood, p.armTerminator = Pos{}, ""
	p.open = append(p.open, opener{word: word, line: int(p.tok.Pos.Line), construct: true})
	return func() {
		p.open = p.open[:depth]
		p.separatorStood = stood
		p.terminatorStood, p.armTerminator = termStood, termText
	}
}

// opensClause records a keyword that is itself awaiting a partner.
//
// A clause replaces the clause before it rather than stacking on it: `then`
// and `else` are alternatives within one `if`, not one inside the other, and
// a list saying both were open would be describing a state the parser was
// never in. Constructs below are untouched, so `for` holding an `if` holding
// an `else` still reads as the three of them.
func (p *Parser) opensClause(word string) {
	for len(p.open) > 0 && !p.open[len(p.open)-1].construct {
		p.open = p.open[:len(p.open)-1]
	}
	p.open = append(p.open, opener{word: word, line: int(p.tok.Pos.Line)})
}

// ranOut records that the input ended with something unfinished, and keeps
// what was open at that moment.
//
// One place rather than an assignment at each site. The snapshot has to be
// taken while the stack is still standing, and a site that set the flag
// without taking it would leave a prompt with nothing to say — a failure that
// looks exactly like nothing having been open.
func (p *Parser) ranOut() {
	if !p.incomplete {
		p.openAtEnd = append([]opener(nil), p.open...)
	}
	p.incomplete = true
}

// Open is what the parser was still inside when the input ran out, outermost
// first.
//
// Empty when the input was complete, or when it was wrong in some way other
// than ending too soon. A caller drawing a continuation prompt wants this: it
// is the difference between "there is more to type" and "there is more to
// type and it is the `for` from three lines up".
//
// The words are the shell's own keywords, which is all this package knows.
// What a dialect calls them at a prompt is the dialect's business — one of
// them says `for` where another would say the clause inside it.
func (p *Parser) Open() []Open {
	out := make([]Open, 0, len(p.openAtEnd)+1)
	for _, o := range p.openAtEnd {
		out = append(out, Open{Word: o.word, Line: o.line, Construct: o.construct})
	}
	// The lexer's own, innermost: a quote or an expansion is inside whatever
	// construct the parser had reached, and it is what the next line goes on
	// with. It arrives even when the parser recorded nothing, because a word
	// that never finished can end the input without the parser having asked
	// for anything.
	if w := p.lex.Open(); w != "" {
		out = append(out, Open{Word: w})
	}
	return out
}

// OpenQuote is the **lexer's** own unfinished thing, spelled as it is
// written: `'`, `"`, `$'`, `${`, `$((`, “ ` “ or `<<`. Empty when the input
// was whole, or when what ran out was the parser's rather than the lexer's.
//
// Open above answers a continuation prompt, which wants the whole stack and
// the construct that opened it. This answers a different question and wants
// only the innermost: a shell reading a script one physical line at a time has
// to know what the *next* line begins inside, because a quote opened on one
// line is still open on the next and a here-document's body is not shell text
// at all. See driver's history gate, which is the caller.
//
// A method rather than reading Open()[len-1], because that slice is allocated
// per call and its last entry is a construct keyword whenever the lexer had
// nothing open — `then` is not a quote, and a caller reading it as one would
// silence expansion inside every `if`.
func (p *Parser) OpenQuote() string { return p.lex.OpenInnermost() }

// Open is one thing the parser is inside.
type Open struct {
	// Word is the keyword that opened it: `if`, `for`, `case`, `{`, or a
	// clause's own word such as `then` or `do`.
	Word string
	// Line is where it was opened, which is what a diagnostic names when the
	// construct began further up than the failure.
	Line int
	// Construct distinguishes a compound command from a clause of one. `if`
	// is a construct and the `then` inside it is not.
	Construct bool
}

// unterminated describes the state the parser gave up in.
func (p *Parser) unterminated(expected string) *Error {
	e := &Error{
		Pos: p.tok.Pos, Kind: ErrUnterminated,
		Expected: expected, LastToken: p.lastText,
	}
	if n := len(p.open); n > 0 {
		e.Innermost = p.open[n-1].word
		if p.separatorStood.IsValid() {
			// A `;` this dialect stepped over is what the one shell naming
			// an innermost keyword names instead, and it outlasts a clause:
			// `if false || ; then` with the input ending there is
			// `` `;' unmatched `` in ksh93u+ where `if :; then` is
			// `` `then' unmatched ``. See Parser.separatorStood.
			e.Innermost = ";"
		} else if p.terminatorStood.IsValid() {
			// And a `case` arm's terminator with a separator in front of it
			// is what that same shell names. See Parser.terminatorStood.
			e.Innermost = p.armTerminator
		}
		for i := n - 1; i >= 0; i-- {
			if p.open[i].construct {
				e.Construct, e.ConstructLine = p.open[i].word, p.open[i].line
				break
			}
		}
	}
	e.EndLine = int(p.tok.Pos.Line)
	if !strings.HasSuffix(p.lex.src, "\n") {
		// The text stopped mid-line, so the end of it is the line after.
		e.EndLine++
	}
	e.Msg = "unexpected end of input"
	if e.Construct != "" {
		e.Msg = "unterminated " + e.Construct
	}
	return e
}

// failUnexpected records a token the grammar did not want, with what would
// have been valid where the parser knows it.
//
// The class travels because one dialect names it rather than the token: an
// ordinary word is "word unexpected" there, where a reserved word and an
// operator are quoted.
// failUnexpectedOperand is failUnexpected where a *command* could not begin,
// so no word there is reserved.
//
// A reserved word is only reserved where the grammar could take a command:
// `esac` in the pattern position of `case a in a) echo x;;& esac` is an
// ordinary word, and the dialect that names a word's class calls it one —
// "word unexpected", not `"esac" unexpected`. Treating the list of reserved
// words as reserved everywhere got that wrong in the one place it shows.
func (p *Parser) failUnexpectedOperand(expected string) {
	p.failUnexpectedAs(expected, true)
}

func (p *Parser) failUnexpected(expected string) {
	p.failUnexpectedAs(expected, false)
}

// failUnexpectedWord is failUnexpected where what would have stood there is a
// word of any spelling rather than one particular word.
//
// The distinction is the expectation's and not the refused token's, so it is
// recorded beside Expected rather than beside Class: the two dialects that
// print an expectation quote a spelling and leave a class bare — dash answers
// `for in x; do :; done` with `(expecting "do")` and `case ; in x) ;; esac`
// with `(expecting word)`. See Error.ExpectedIsAClass.
func (p *Parser) failUnexpectedWord() {
	p.failUnexpectedWordAt(p.tok)
}

// failUnexpectedWordAt is failUnexpectedWord where the token to blame is not
// the one the parser is on — the run-out a dialect reads as a newline.
func (p *Parser) failUnexpectedWordAt(tok Token) {
	if p.err != nil {
		return
	}
	p.failUnexpectedAt(tok, "word", false)
	if e, ok := p.err.(*Error); ok {
		e.ExpectedIsAClass = true
	}
}

func (p *Parser) failUnexpectedAs(expected string, plain bool) {
	p.failUnexpectedAt(p.tok, expected, plain)
}

// failUnexpectedAt records a token the grammar did not want when the parser
// has already read past it.
//
// A rule that can only be checked once more input has been consumed — the
// function body that has to be compound, which is not known to be simple
// until it has been parsed — still has to name the token where the trouble
// began, and point the echo of the offending line at *its* line rather than
// at wherever the parser ended up. So the token travels rather than being
// read off the parser's current position.
func (p *Parser) failUnexpectedAt(tok Token, expected string, plain bool) {
	if p.err != nil {
		return
	}
	if tok.Kind == TokEOF {
		if !p.inCaseWord || !p.dialect.CaseWordRunsOutAsANewline {
			p.ranOut()
			p.err = p.unterminated(expected)
			return
		}
		// The dialect reads the run-out here as the newline that would have
		// ended the line, and the newline standing there for real already
		// produces the right bytes through this same path — so the reading
		// is a token substitution and not a second wording. Kept at the same
		// position, which is what numbers the line the pattern is on rather
		// than the line after it.
		tok = Token{Kind: TokNewline, Pos: tok.Pos, End: tok.End, Text: "\n"}
	}
	literal, text := tokenLiteral(tok), tokenText(tok)
	source := tokenSource(tok)
	if p.emptyParensStartAt(tok) {
		// The dialect that reads `()` as one token names the pair wherever a
		// refusal falls on the first of them, and not only inside the
		// definition production that consumes them. See
		// [Dialect.EmptyParensAreOneToken].
		literal, text, source = "()", `"()"`, ""
	}
	p.err = &Error{
		Pos: tok.Pos, Kind: ErrUnexpected,
		Token: literal, TokenOpener: tokenOpener(tok),
		TokenSource: source, TokenHoldsExpansion: tokenHoldsAnExpansion(tok),
		Class: tokenClass(tok, plain), Expected: expected,
		Redirect:    tok.Kind.IsRedirect(),
		AliasSource: p.aliasSource,
		Msg:         text + " unexpected",
	}
}

// emptyParensStartAt reports whether tok is the `(` of an adjacent `()`, in a
// dialect that reads the pair as one token.
//
// Adjacency is the rule and it is measured: `x=1 f () { … }` is a parse
// error naming `()` on zsh 5.9.2, and `x=1 f ( ) { … }` — the same
// line with one blank inside the parentheses — is blamed at the `}` instead,
// the two characters then being a subshell. So this reads the source rather
// than skipping blanks the way the definition path's lookahead does.
func (p *Parser) emptyParensStartAt(tok Token) bool {
	if !p.dialect.EmptyParensAreOneToken || tok.Kind != TokLeftParen {
		return false
	}
	i := int(tok.End.Offset)
	return i >= 0 && i < len(p.lex.src) && p.lex.src[i] == ')'
}

// tokenLiteral is the token as a diagnostic writes it, without the quotes a
// message may add of its own.
func (p *Parser) tokenLiteral() string { return tokenLiteral(p.tok) }

func tokenLiteral(tok Token) string {
	switch tok.Kind {
	case TokWord:
		return tok.Literal()
	case TokArithCmd:
		// The expression the construct held, blanks and all — ` 2 ` for
		// `(( 2 ))` — which is what two of the panel's four quote back.
		// Kind.String() answers `arithmetic command`, a description written
		// for prose, and standing it where a diagnostic quotes what it read
		// produced a sentence no shell writes (#2013).
		return tok.Text
	}
	return tok.Kind.String()
}

// tokenSource is the word as it was written, quotes and all, for the dialects
// that echo a refused word back rather than naming what it comes to. See
// Error.TokenSource.
//
// Only a word has two spellings: every other token's source and its name are
// the same characters, and TokArithCmd's Text is already the expression rather
// than the source, so answering it here would name the same thing twice.
func tokenSource(tok Token) string {
	if tok.Kind != TokWord {
		return ""
	}
	return tok.Text
}

// tokenOpener is the second spelling of a token whose text is not what it was
// written as. See Error.TokenOpener; only the arithmetic command has one.
func tokenOpener(tok Token) string {
	if tok.Kind == TokArithCmd {
		return "(("
	}
	return ""
}

func (p *Parser) tokenClass(plain bool) TokenClass { return tokenClass(p.tok, plain) }

func tokenClass(tok Token, plain bool) TokenClass {
	if tok.Kind == TokNewline {
		return ClassNewline
	}
	if tok.Kind != TokWord {
		return ClassOperator
	}
	if !plain && !tok.IsQuoted() && reservedWords[tok.Literal()] {
		return ClassReserved
	}
	return ClassWord
}

func (p *Parser) fail(format string, args ...any) {
	p.failKind(ErrSyntax, format, args...)
}

// failKind records a parse failure with a classification, so a dialect can
// word it without matching on the message text.
func (p *Parser) failKind(kind ErrorKind, format string, args ...any) {
	if p.err != nil {
		return
	}
	if p.at(TokEOF) {
		// Running out of input is unfinished rather than wrong.
		p.ranOut()
	}
	p.err = &Error{Pos: p.tok.Pos, Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

// expectWord consumes a reserved word or records what was missing.
func (p *Parser) expectWord(s string) Pos {
	pos := p.tok.Pos
	if !p.atWord(s) {
		if p.at(TokEOF) {
			// The input ended with something still open, which every shell in
			// the panel reports as its own kind of failure rather than as a
			// word in the wrong place.
			p.ranOut()
			if p.err == nil {
				p.err = p.unterminated(s)
			}
			return pos
		}
		p.failUnexpected(s)
		return pos
	}
	p.next()
	return pos
}

// Parse reads the whole input.
func (p *Parser) Parse() *File {
	f := &File{}
	for {
		line, ok := p.NextLine()
		if !ok {
			break
		}
		if line.Refused != nil {
			// The line is thrown away unrun, here as everywhere: what was
			// read of it is not appended. Reading carries on so that the
			// lines after it are still read — this is the route `-n` takes —
			// and the refusal is kept for the end.
			if f.Refused == nil {
				f.Refused = line.Refused
			}
			continue
		}
		f.Stmts = append(f.Stmts, line.Stmts...)
		f.Substitutions = append(f.Substitutions, line.Substitutions...)
		f.CarriedHeredocs = append(f.CarriedHeredocs, line.CarriedHeredocs...)
	}
	f.Last = p.tok.Pos
	if p.err == nil && f.Refused != nil {
		// Reading the whole file at once has no next line to go on to, so a
		// refusal here *is* the answer — which is what `bash -n` reports, and
		// what it exits non-zero for. The incremental route keeps it on the
		// line instead; see File.Refused.
		p.err = f.Refused
	}
	return f
}

// NextLine parses one logical line: the statements up to the newline that ends
// them, which is more than one line of text when a construct is still open.
//
// It exists because a shell runs what it has read rather than reading
// everything first. `echo one` on line 1 runs before line 3 fails to parse,
// which is unanimous across the panel and is why a script that ends badly
// still does what its good lines said.
//
// The *line* is the unit and not the statement, which is measured: with
// `echo one; { fi; }` on one line, nothing runs. So everything up to the
// newline is parsed before any of it is run, and a failure anywhere in it
// discards the whole line.
//
// The second result is false at the end of the input, and when parsing has
// already failed.
func (p *Parser) NextLine() (*File, bool) {
	p.primeAliases()
	p.skipNewlines()
	if p.at(TokEOF) || p.err != nil {
		return nil, false
	}
	f := &File{}
	for p.err == nil {
		// A `;` where a command belongs, for the dialects that step over one:
		// `; echo two` and `true ; ; echo two` alike, since the second
		// statement of a line begins here as much as the first does.
		if p.skipSeparators(separatorAtAStatement) {
			p.skipNewlines()
		}
		st := p.parseStmt()
		if st == nil {
			if p.err == nil && !p.at(TokEOF) {
				// Nothing here can begin a command: a stop word with no
				// construct open, most often. Every shell in the panel calls
				// that a syntax error — `}` alone is one in all four — where
				// this used to stop quietly and silently truncate the rest of
				// the script. `function f { ...; }` in a dialect without the
				// keyword is the case that found it: the `}` ended parsing,
				// and the commands after it never ran.
				p.failUnexpected("")
			}
			break
		}
		f.Stmts = append(f.Stmts, st)
		if p.at(TokNewline) || p.at(TokEOF) {
			break
		}
		if !st.Semi.IsValid() {
			// Nothing terminated the statement, so the line ended with it:
			// two commands need a `;`, a newline or an `&` between them.
			// Only a command that ends itself gets this far — a simple
			// command absorbs the next word as an argument — which is why it
			// shows on `{ :; } echo x` and never on `true echo x`.
			p.failUnexpected("")
			break
		}
	}
	f.Last = p.lineEnd()
	f.Refused, p.refused = p.refused, nil
	f.Substitutions, p.lineSubsts = p.lineSubsts, nil
	f.CarriedHeredocs, p.lex.carried = p.lex.carried, nil
	if p.err != nil {
		// The line did not read, so none of it runs. That is this function's
		// own rule — everything up to the newline is parsed before any of it
		// runs, and a failure anywhere in it discards the whole line — and
		// the loop above had a hole in it: parseStmt hands back the tree it
		// had built when the failure landed *inside* the last construct on
		// the line, and that tree was appended and run anyway.
		//
		// `eval 'f() {` / `}'` is the case that found it. bash refuses the
		// empty body and leaves an earlier `f` standing; here the refused
		// definition was still bound, so a helper was silently replaced by a
		// function that prints nothing and answers 0. The same shape with the
		// failure between two statements — `echo one; { fi; }` — was already
		// right, because parseStmt returned nil there and the loop broke
		// before appending, which is why the hole survived: the two shapes
		// are one rule and only one of them was covered.
		//
		// Not the same as a refusal. Refused is a line the reader gave up and
		// went on from, and it is carried on the File for the caller to
		// report; this is the reader stopping, and p.Err is what says so.
		f.Stmts = nil
	}
	return f, true
}

// lineEnd is where the logical line just parsed ends in the input.
//
// The token that ended it, ordinarily — and the line that closed a
// here-document where one was read, because its body and its delimiter are
// physical lines of the command that owns them and the token says nothing
// about them. The newline that triggers the read sits at the end of the
// command's *first* line and the body is consumed behind it, so `cat <<END`
// over three lines of script otherwise reports one.
//
// The furthest of the two, which is the whole rule: a line with no
// here-document is unaffected, and one with several is closed by the last
// delimiter rather than the first.
func (p *Parser) lineEnd() Pos {
	at := p.tok.Pos
	if e := p.lex.heredocEnd; e.Offset > at.Offset {
		return e
	}
	return at
}

func (p *Parser) skipNewlines() {
	for p.at(TokNewline) {
		p.next()
	}
}

// skipAnonBodySeparators steps over what may stand between a nameless
// function's header and its body.
//
// A `;` as well as a newline, and for both spellings of the header: measured
// 2026-09-19 on zsh 5.9.2, `function; { echo $#; } a b` and `() ; { echo $#; }
// a b` each print `2`, so the separator is a property of the position rather
// than of the keyword. See [Parser.peekIsAnonBody], which has to agree with
// this over the raw source before a token is asked for.
func (p *Parser) skipAnonBodySeparators() {
	for p.at(TokNewline) || p.at(TokSemi) {
		p.next()
	}
}

// skipCaseHeaderSeparators steps over what may stand inside a `case` header:
// newlines everywhere, and a `;` where the dialect takes one. See
// [Dialect.CaseHeaderSpansSeparators].
//
// TokSemi and not the rest of the family: `;;` is the arm terminator and `&`
// is an operator, and the one shell that takes this refuses both here.
func (p *Parser) skipCaseHeaderSeparators() {
	for p.at(TokNewline) || (p.dialect.CaseHeaderSpansSeparators && p.at(TokSemi)) {
		p.next()
	}
}

// skipArrayElementSeparators steps over what may stand between the elements of
// an array literal: newlines everywhere, and a `;` as far as the dialect takes
// one. See [syntax.ArraySemicolon].
//
// afterAnElement is what separates ksh93's reading from zsh's. There a `;`
// *ends* the element list rather than standing between two elements, so it
// needs an element in front of it, may be written once, and leaves nothing but
// newlines and the closing `)` behind it — which is why this reports having
// *ended* the list and the caller stops reading elements, rather than reading
// on and failing later at a `)` that is present.
func (p *Parser) skipArrayElementSeparators(afterAnElement bool) (ended bool) {
	p.skipNewlines()
	switch p.dialect.SemicolonInAnArrayLiteral {
	case SemicolonSeparatesArrayElementsLikeANewline:
		for p.at(TokSemi) {
			p.next()
			p.skipNewlines()
		}
	case OneSemicolonEndsTheArrayElements:
		if afterAnElement && p.at(TokSemi) {
			p.next()
			p.skipNewlines()
			// The list is over. Anything but the `)` is reported against the
			// token itself, which is the shell's own answer: `a=( x; y )` is
			// `` `y' unexpected `` there.
			return true
		}
	case NoSemicolonInAnArrayLiteral:
	}
	return false
}

// separatorPosition is where the parser was looking for a command when it met
// a `;`. The dialects that step over one draw their lines by position rather
// than by the token, so this is what a caller has to say.
type separatorPosition uint8

const (
	// separatorAtAStatement is where a statement of a list begins — the top
	// of a file, and between two statements of any list.
	separatorAtAStatement separatorPosition = iota
	// separatorAfterABar is the command a pipeline wants after its bar.
	separatorAfterABar
	// separatorInACondition is a statement of a keyword's condition list.
	separatorInACondition
	// separatorAtAnAndOrOperand is an and-or's right-hand side.
	separatorAtAnAndOrOperand
)

// conditionPosition names the position a list's own separator is in, which is
// one of two depending on whether the list is a condition's.
func conditionPosition(inCondition bool) separatorPosition {
	if inCondition {
		return separatorInACondition
	}
	return separatorAtAStatement
}

// skipSeparators steps over a `;` written where a command belongs, as far as
// the dialect goes, and reports whether it stepped over any.
//
// A newline *before* one is stepped over by the caller and always was —
// `a || ⏎ b` is core — so `a || ⏎ ; b` runs wherever `a || ; b` does. A
// newline **after** one is the dialect's own question and the two shells
// answer it differently, which is measured:
//
//	true || ; ⏎ echo two   →  two        ksh93
//	                       →  (nothing)  zsh
//
// zsh reads on and makes `echo two` the right-hand side, so the `true`
// short-circuits past it. ksh93 stops at the newline, the and-or ends with
// nothing on its right, and `echo two` is the next statement — which is why
// only the wider value steps over what follows.
//
// at says which of the four positions the caller is in. They are the caller's
// to know and not the token's — the same `;` is taken in one and refused in
// another — and the wider value takes all four.
func (p *Parser) skipSeparators(at separatorPosition) bool {
	limit := 0
	crossNewlines := false
	switch p.dialect.SeparatorWhereACommandBelongs {
	case OneSeparatorExceptAfterABarOrBeforeACondition:
		if at == separatorAfterABar || at == separatorInACondition {
			return false
		}
		limit = 1
	case AnySeparatorWhereACommandBelongs:
		limit = -1
		crossNewlines = true
	case SeparatorOnlyWhereAnAndOrWantsOne:
		if at != separatorAtAnAndOrOperand {
			return false
		}
		limit = 1
	default:
		return false
	}
	skipped := false
	for limit != 0 && p.at(TokSemi) {
		if !p.insideACaseArm() {
			// What the dialect that names an innermost keyword names from
			// here on. See Parser.separatorStood, where the arm exception is
			// measured too.
			p.separatorStood = p.tok.Pos
		}
		p.next()
		if crossNewlines {
			p.skipNewlines()
		}
		skipped = true
		if limit > 0 {
			limit--
		}
	}
	return skipped
}

// parseList reads statements until a stop word, a closing paren, or the end —
// and until a statement that was written with no terminator after it.
//
// That last one is the list's real boundary and nothing else marks it: two
// statements need a `;`, a newline or an `&` between them, so a command
// standing straight after one that ended itself — `(( i < 2 )) echo hi` — is
// not a second statement of the same list. Every shell in the panel refuses
// that text where it is only a list; the one with short loops reads the second
// command as a loop *body*, which is why the boundary has to be visible here
// rather than guessed at afterwards.
//
// The list stops rather than failing, and the caller says what was wrong,
// because the caller is the one that knows what it was waiting for: the panel
// names the token it met and one shell also names the closer it wanted, which
// is `)` for a subshell and `done` for a loop. A simple command hides the gap
// entirely, since its words absorb whatever follows — `true echo x` is one
// command with an argument — so this only ever shows after a compound.
func (p *Parser) parseList() []*Stmt {
	// Read and cleared at the top, so a list nested inside a condition — a
	// brace group written as one, a substitution in one — is an ordinary
	// list again. Only the two calls in this function are what the rule is
	// about: an and-or's right-hand side and a pipeline's are their own
	// positions, and ksh93 answers those differently. Measured.
	inCondition := p.inCondition
	p.inCondition = false
	var out []*Stmt
	p.skipNewlines()
	if p.skipSeparators(conditionPosition(inCondition)) {
		// A newline after the separator ends nothing here: between two
		// statements it is an ordinary terminator, which every shell takes.
		// It is only where an and-or's right-hand side belongs that one shell
		// stops at it.
		p.skipNewlines()
	}
	for p.err == nil && !p.at(TokEOF) && !p.atStopWord() && !p.at(TokRightParen) {
		st := p.parseStmt()
		if st == nil {
			if p.err == nil && len(out) > 0 && p.at(TokSemi) {
				// A `;` the dialect would not step over, standing where the
				// *next* statement of this list begins. The list stopped
				// silently and left it for whatever the caller wanted a
				// separator before, which took it: `if :; ; then :; fi`
				// parsed in all four dialects where dash, bash 5.3 and ksh93
				// each name the second `;` and only zsh runs it (#2023).
				//
				// Only once the list has something in it. An empty one is
				// [Parser.requireBody]'s question — the dialect that allows
				// an empty body allows `if ; then :; fi` with it — and
				// failing here would answer it twice and differently.
				p.failUnexpected("")
			}
			break
		}
		out = append(out, st)
		if !st.Semi.IsValid() {
			break
		}
		p.skipNewlines()
		if p.skipSeparators(conditionPosition(inCondition)) {
			p.skipNewlines()
		}
	}
	if len(out) == 0 {
		p.blameTheEmptyList(inCondition)
	}
	return out
}

// parseBody is parseList where the grammar requires the list to have
// something in it, which is every compound command's body and every
// condition — but not a `case` arm, whose body may be empty in all four, and
// not a command substitution, which is a program rather than a body.
//
// The check is here rather than in parseList because parseList is also how
// the places that *may* be empty read their contents, and because the answer
// is one answer: an empty body is refused by dash, bash and ksh93 in every
// shape and taken by zsh in every shape, so a shell asks once.
//
// The token the parser stopped on is what is named, which is what the panel
// names: `}` for a brace group, `)` for a subshell, `fi` and `done` for the
// keyword forms. Nothing is named as expected alongside it — an empty body
// has nothing to be in the middle of, which is the same distinction
// parseGroup already draws for a reserved word it cannot use.
func (p *Parser) parseBody() []*Stmt {
	return p.requireBody(p.parseList())
}

// recordArmTerminator notes a `case` arm's terminator as the innermost
// unclosed thing, where a command separator stood in front of it.
//
// The separator is read off the source rather than off the tree, because
// which of the three it was does not matter and a blank between the body and
// the terminator is not one: `case x in x) : ;;` names the `case` and
// `case x in x) : ⏎ ;;` names the `;;`. See Parser.terminatorStood for the
// panel row this answers and for why it is diagnostic only.
func (p *Parser) recordArmTerminator(it *CaseItem, bodyFrom Pos) {
	p.terminatorStood, p.armTerminator = Pos{}, ""
	take := func(text string) {
		p.terminatorStood, p.armTerminator = p.tok.Pos, text
	}
	// Read off the source rather than off the last statement's End(), which
	// a bare `!` has none of — and a bare `!` is one of the rows: measured,
	// `case x in x) ! ;;` names the `;;` where `case x in x) : ;;` names the
	// `case`, so an arm that ran no command answers as the empty one does.
	before := strings.TrimRight(p.sourceBetween(bodyFrom, p.tok.Pos), " \t")
	if !armRanACommand(it.Body) {
		// An arm with no command in it is the terminator's either way, and
		// the one shape that is not is the step-over: a `;` standing alone
		// in front of the terminator on the same line is what that shell
		// names. A newline after it gives the terminator back, which is the
		// row #2233 says two probes could not tell apart.
		if strings.TrimLeft(before, " \t") == ";" {
			take(";")
			return
		}
		take(p.tok.Kind.String())
		return
	}
	if before == "" || !strings.ContainsAny(before[len(before)-1:], ";&\n") {
		return
	}
	take(p.tok.Kind.String())
}

// armRanACommand reports whether a `case` arm's body ends in a statement that
// has a command in it. A bare `!` is a pipeline with no commands, which is
// both the one shape with no End() to ask and the one the shell answers as
// though the arm were empty.
func armRanACommand(body []*Stmt) bool {
	if len(body) == 0 {
		return false
	}
	pipe, isPipe := body[len(body)-1].Expr.(*Pipeline)
	return !isPipe || len(pipe.Cmds) > 0
}

// insideACaseArm reports whether the innermost construct the parse is inside
// is a `case`. See Parser.separatorStood for why one position is excepted.
func (p *Parser) insideACaseArm() bool {
	for i := len(p.open) - 1; i >= 0; i-- {
		if p.open[i].construct {
			return p.open[i].word == "case"
		}
	}
	return false
}

// parseCondition is parseBody for the list a keyword takes as its *condition*
// — `if`, `elif`, `while`, `until` — where one dialect will not step over a
// `;` that it steps over everywhere else.
//
// Measured on ksh93u+ 2026-09-12: `if; then :; fi` and `if :; ; then :; fi`
// are both “ `;' unexpected “ there, while `if :; then : ; ; fi` and
// `if false || ; then :; fi` run — so the position is where a *statement of
// the condition list* begins, and neither the header as a whole nor the token
// after the keyword. See Dialect.SeparatorWhereACommandBelongs, where the
// nine rows are (#2023).
func (p *Parser) parseCondition() []*Stmt {
	p.inCondition = true
	list := p.parseBody()
	p.inCondition = false
	return list
}

// requireBody is parseBody for a caller that read its list some other way —
// a loop header, where whether the list stops at the body is a dialect's
// answer of its own.
func (p *Parser) requireBody(list []*Stmt) []*Stmt {
	if len(list) != 0 || p.dialect.EmptyCompoundBody || p.err != nil {
		return list
	}
	if p.dialect.SteppedOverSeparatorIsABody && p.separatorStood.IsValid() &&
		(p.atStopWord() || p.at(TokRightParen)) {
		// The body was written as a `;` this dialect steps over, and the
		// construct's closer is what came next: `{ ; }` runs where `{ }` is
		// refused. The closer has to be there for it — `{ ; ; }` leaves the
		// parse sitting on the second `;`, which is one more than the
		// dialect's count steps over and is refused there too.
		//
		// End of input is not a closer, so it falls through to the caller
		// below, which is what reports an unterminated construct.
		return list
	}
	if p.at(TokEOF) {
		// The input ran out rather than the body being empty, and what is
		// waiting for more is the caller's to report — it knows which
		// construct is open.
		return list
	}
	p.failUnexpected("")
	return list
}

// blameTheEmptyList records the refusal a list with nothing in it draws when
// it stopped on a terminator the dialect blames something else for.
//
// Three of the seven columns name the terminator where it stands and two
// name what is behind it, over different sets of terminators and by
// different rules — see [Dialect.EmptyBodyBlame], which has the table, and
// [EmptyBodyBlame]'s values for how each one steps.
//
// Here rather than in [Parser.requireBody], which is where an empty body is
// refused, because one of the two dialects *allows* an empty body — its
// `if ; then :; fi` runs — and so never reaches that check at all. What it
// refuses is the terminator, and the terminator is what this asks about.
// Nothing is recorded unless the token is one the dialect blames, which is
// what keeps a list that may legitimately be empty — a `case` arm's, a
// command substitution's — from being refused here.
//
// The terminator's own refusal is recorded *first* and then replaced, rather
// than the next token being peeked at: the parser has no lookahead, and by
// the time a decision can be made the terminator has been stepped past.
// Recording first is what keeps anything the step lexes from raising a
// refusal of its own in between.
func (p *Parser) blameTheEmptyList(inCondition bool) {
	at := p.tok
	if p.err != nil || !p.dialect.EmptyBodyBlamed[at.Kind] {
		return
	}
	p.failUnexpected("")
	p.next()
	if !p.blamesWhatFollows(inCondition) {
		return
	}
	p.err = nil
	p.failUnexpectedAt(p.tok, "", false)
}

// blamesWhatFollows reports whether the token the parser now stands on is the
// one to name, the terminator behind it having been stepped over.
func (p *Parser) blamesWhatFollows(inCondition bool) bool {
	switch p.dialect.EmptyBodyBlame {
	case BlameTheTokenAfterIt:
		// Anything but a command, and anything but a newline — which is the
		// one place this shell takes the line rather than refusing it.
		return !p.at(TokNewline) && !p.at(TokEOF) && p.cannotBeginACommand()
	case BlameTheKeywordAfterIt:
		if inCondition {
			// A condition is named at the keyword that *ends the header*,
			// however much stands in between: `if | :; then :; fi` and
			// `if | :; :; then :; fi` are both the `then`. So the refusal is
			// read forward to it rather than off the next token.
			p.skipToTheHeadersKeyword()
		}
		// `}` is the reserved word this one does not name: `{ | }` names the
		// operator where `if | fi` names the `fi`. It is skipped rather than
		// stopped at above for the same reason — a group written inside a
		// condition closes with one, and `if | { :; }; then :; fi` is the
		// `then` there.
		return p.atStopWord() && p.tok.Literal() != "}"
	}
	return false
}

// skipToTheHeadersKeyword reads forward to the reserved word that ends a
// condition, so that a refusal inside one can be named there.
//
// Bounded rather than open-ended: the parser is already on its way out with a
// refusal recorded, and a header is a header — a scan that has read a hundred
// tokens without finding a keyword has found something this rule was not
// measured on, and the terminator it started from is the answer then.
func (p *Parser) skipToTheHeadersKeyword() {
	for range 100 {
		if p.at(TokEOF) {
			return
		}
		if p.atStopWord() && p.tok.Literal() != "}" {
			return
		}
		p.next()
	}
}

// cannotBeginACommand reports whether no command could start at the current
// token, which is what says a refusal standing here is the grammar's own
// rather than a line the shell would have taken.
//
// A stop word and the operators, and deliberately not a redirection — `> f`
// is a command with nothing but redirections in every shell in the panel —
// nor a `(`, which opens a subshell, nor a word, which is a command's name.
func (p *Parser) cannotBeginACommand() bool {
	switch p.tok.Kind {
	case TokSemi, TokAmp, TokPipe, TokPipeAmp, TokAndAnd, TokOrOr,
		TokDSemi, TokSemiAmp, TokDSemiAmp, TokSemiPipe, TokRightParen:
		return true
	}
	return p.atStopWord()
}

// parseStmt reads one and-or list and its terminator.
func (p *Parser) parseStmt() *Stmt {
	// Cleared here rather than after it is read, so that the statement being
	// started asks about its own body and never about an earlier one's.
	p.bodyTookTerm, p.bodyTookKind = Pos{}, TerminatedByNothing
	expr := p.parseAndOr()
	if expr == nil {
		return nil
	}
	st := &Stmt{Expr: expr, Semi: p.bodyTookTerm, Term: p.bodyTookKind}
	// Where the statement's terminator ends, for keepFunctionSource below.
	// Invalid until a terminator is read, which is the "nothing followed it"
	// case that method is written for.
	var term Pos
	switch p.tok.Kind {
	case TokPipeAmp:
		// Only where the dialect reads the operator as a coprocess. Where it
		// reads it as a pipe the token was consumed by parsePipeline and
		// never arrives here, and where it has neither reading the lexer
		// never made the token at all.
		//
		// Which makes the guard **equivalent rather than decisive today**,
		// and it is kept for what it says: mutating it to `if false`
		// survives the suite, and a probe of both other dialects says why —
		// with the pipe reading, `cat |&`, `cat |& b`, `a && cat |&` and a
		// trailing newline all answer exactly as they do without the mutant,
		// because parsePipeline never leaves this token behind; with neither
		// reading the lexer makes `|` and `&`. A dialect setting both flags
		// is the only reachable difference, no preset does, and the pipe
		// reading would win there anyway. Recorded so the next reader does
		// not go looking for the row that would kill it.
		if !p.dialect.CoprocPipeOperator {
			break
		}
		// `&` plus two pipes, and it terminates the same thing `&` does: the
		// whole and-or, measured — `echo A && cat |&` sends `echo A`'s
		// output into the coprocess.
		st.Background, st.Coprocess = true, true
		st.Semi = p.tok.Pos
		st.Text = p.textBetween(expr.Pos(), st.Semi)
		p.next()
	case TokAmp, TokAmpBang, TokAmpPipe:
		// `&` belongs to the statement, not the command: `a && b &`
		// backgrounds the whole and-or. `&!` and `&|` are the same
		// terminator with the job let go of, and they reach the same
		// statement for the same reason — `true && false &!` disowns the
		// whole and-or.
		st.Background = true
		st.Disown = p.tok.Kind != TokAmp
		st.Semi = p.tok.Pos
		// What was written, for a `jobs` listing to show. Taken from the
		// input rather than rebuilt from the tree: `jobs` shows what someone
		// typed, spacing and quoting included, and a printer would show what
		// the parser understood — which is a different thing and the wrong
		// one here.
		st.Text = p.textBetween(expr.Pos(), st.Semi)
		p.next()
	case TokSemi:
		st.Semi, st.Term = p.tok.Pos, TerminatedBySemicolon
		term = p.tok.End
		p.next()
	case TokNewline:
		st.Semi, st.Term = p.tok.Pos, TerminatedByNewline
		term = p.tok.End
	}
	p.keepFunctionSource(expr, term)
	return st
}

// keepFunctionSource records a definition's own source text on it, for the
// dialect that says a function back as it was written rather than as a tree.
// See [Dialect.FunctionDefinitionIsSourceText] and [FuncDecl.SourceText].
//
// It happens here and not where the declaration is parsed because the span
// runs **through the terminator**, which the declaration never holds: ksh93
// writes `f() { :; };` for a definition followed by a `;` and `f() { :; }`
// plus the newline for one written in a file. A definition with nothing after
// it — the last thing an `eval` string holds — ends at its body, which is the
// invalid `term` this is called with everywhere else.
//
// Only a statement that is nothing *but* one definition is recorded. `f() {
// :; } && echo ok` is a binary expression whose terminator belongs to the
// whole of it, and a pipeline of several commands has no one declaration to
// carry the text.
func (p *Parser) keepFunctionSource(expr Expr, term Pos) {
	if !p.dialect.FunctionDefinitionIsSourceText {
		return
	}
	pl, ok := expr.(*Pipeline)
	if !ok || pl.Negated || len(pl.Cmds) != 1 {
		return
	}
	fn, ok := pl.Cmds[0].(*FuncDecl)
	if !ok || fn.Body == nil {
		return
	}
	if !term.IsValid() {
		term = fn.End()
	}
	fn.SourceText = p.rawBetween(fn.Pos(), term)
}

// refusedFuncName is the word a definition's complaint names, for the dialects
// that read it whole and refuse it when the definition runs.
//
// Two answers, and [Dialect.FunctionNameIsSourceText] tells them apart. bash
// names the word **as written**, quotes and all: given a name spelled with
// single quotes around it, its complaint carries those quotes. ksh93 takes
// them off first and says `a$b: invalid function name`, and an empty quoted
// name is a complaint naming nothing at all — measured 2026-09-12 on ksh93u+
// 2012-08-01 across a dollar, a blank, a `;`, a `*` and the empty name
// (#2345).
//
// What survives the second answer is an *expansion*: `_p_${w}` is written back
// whole, because the quoting rule is about quote characters and `${w}` holds
// none. So this is a span walk rather than either whole-token spelling — a
// literal span contributes its value, which is the lexer's text with its own
// quotes already gone, and every other span contributes its source.
//
// [Dialect.FunctionNameIsAnyBareWord] takes the first answer for a different
// reason, and it is worth saying which: that shell prints no complaint at all,
// so nothing here is a wording. What the word is kept for there is the
// **print**, and a definition printed back without the quotes it was written
// with is a definition the shell would then take — `'f'()` reads back as
// `f()`, which defines `f` where the input defined nothing.
func (p *Parser) refusedFuncName(t Token) string {
	if p.dialect.FunctionNameIsSourceText || p.dialect.FunctionNameIsAnyBareWord {
		return p.textBetween(t.Pos, t.End)
	}
	var b strings.Builder
	for i, sp := range t.Spans {
		if sp.Kind == Literal {
			b.WriteString(sp.Value)
			continue
		}
		end := t.End
		if i+1 < len(t.Spans) {
			end = t.Spans[i+1].Pos
		}
		b.WriteString(p.textBetween(sp.Pos, end))
	}
	return b.String()
}

// textBetween is the input from one position up to another, trimmed.
//
// Bounds-checked rather than trusted: a Pos is only as good as whatever
// produced it, and this is on the path a `jobs` listing prints from.
// offsetBy is a position n bytes further along the same line, which is what a
// byte offset inside a span's value means for a span whose value is its
// source.
func offsetBy(pos Pos, n int) Pos {
	pos.Offset += int32(n)
	pos.Col += int32(n)
	return pos
}

func (p *Parser) textBetween(from, to Pos) string {
	return strings.TrimSpace(p.rawBetween(from, to))
}

// rawBetween is the same span untrimmed, for the one caller that wants the
// characters exactly as they were written — see [FuncDecl.SourceText], where
// the blanks before a terminator are part of what the shell says back.
func (p *Parser) rawBetween(from, to Pos) string {
	src := p.lex.src
	if from.Offset < 0 || int(to.Offset) > len(src) || from.Offset >= to.Offset {
		return ""
	}
	return src[from.Offset:to.Offset]
}

// parseAndOr reads pipelines joined by && and ||.
//
// One precedence level, left-associative. Building a right-leaning tree here,
// or giving && a tighter binding, is C's rule and is wrong: every shell prints
// B for `true || echo A && echo B`, which needs `(true || echo A) && echo B`.
func (p *Parser) parseAndOr() Expr {
	left := p.parsePipeline()
	if left == nil {
		return nil
	}
	depth := len(p.open)
	for p.at(TokAndAnd) || p.at(TokOrOr) {
		op, pos := p.tok.Kind, p.tok.Pos
		// Open while the command after it is looked for, the same way a
		// pipeline's bar is: input ending on `&&` is a line waiting for its
		// other half rather than a line that merely stopped.
		p.open = append(p.open, opener{word: op.String(), line: int(pos.Line)})
		p.next()
		p.skipNewlines()
		skipped := p.skipSeparators(separatorAtAnAndOrOperand)
		right := p.parsePipeline()
		if right == nil {
			if skipped && p.dialect.AbsentAndOrOperandIsAnEmptyCommand &&
				(p.at(TokEOF) || p.at(TokNewline) || p.atListEnd()) {
				// A separator stood where the right-hand side belongs and
				// the list ended there, so a command that does nothing and
				// succeeds stands in its place: `false || ;` answers 0.
				//
				// The end of input counts here where it does not for
				// OpenEndedAndOr, and that is measured rather than assumed:
				// through a pty, ksh93 answers `false || ;` at its prompt
				// with **another PS1** and `$?` of 0 — the line is finished —
				// where `false ||` alone draws PS2 and waits. zsh draws PS2
				// for both, which is why its end-of-input answer stays the
				// route question #1174 left open and this one does not.
				//
				// A newline counts too, and only here: `true || ; ⏎ echo two`
				// prints `two` in this shell, so the and-or ended at the
				// newline and the `echo` is the statement after it — where
				// zsh reads on and makes it the right-hand side.
				//
				// A second separator is none of the three, so `a || ; ; b`
				// still fails on the one it found, which is what this shell
				// does.
				//
				// A node rather than an answer the interpreter reads, because
				// an empty command already runs and already succeeds — the
				// difference between this dialect and the one that drops the
				// operator is what is *in* the tree.
				p.open = p.open[:depth]
				empty := &SimpleCmd{Start: p.tok.Pos, Stop: p.tok.Pos}
				left = &BinaryExpr{
					X: left, Op: op, OpPos: pos,
					Y: &Pipeline{Cmds: []Command{empty}},
				}
				continue
			}
			if p.dialect.OpenEndedAndOr && (p.atListEnd() || p.atASubstitutionCloser()) {
				// The right-hand side is absent and the list ends here, so
				// the operator is dropped: `{ : || ⏎ }` is `{ : ⏎ }`.
				//
				// Dropped rather than stood in for, which is measured. The
				// status is the left-hand side's — `false ||` answers 1 and
				// `true &&` answers 0 — so an absent operand is neither an
				// implicit success nor an implicit failure. Either stand-in
				// gets exactly one of that pair wrong.
				//
				// No check that parsing is still clean, deliberately: a
				// failure already recorded is never cleared — every fail
				// site returns early once p.err is set — so taking this
				// branch cannot turn a refusal into an acceptance. A guard
				// here was written and then removed as unreachable: nothing
				// distinguished the two, because the only other thing this
				// branch does is give up the openers below, and once a
				// failure is recorded the prompt reads the snapshot ranOut
				// took rather than the live stack.
				p.open = p.open[:depth]
				return left
			}
			// The token that stopped it is named, not the operator behind
			// it — the same rule a bar follows, and wrong here for the same
			// reason it was wrong there: the whole panel quotes what it
			// found and none of them mentions the `&&`, which by now is
			// read and was never the problem. `echo a && fi` is `"fi"
			// unexpected` and `true && & b` blames the `&`.
			//
			// Not the operand form: a command *may* begin after `&&`, so a
			// reserved word standing there is reserved, and the dialect that
			// names a word's class instead of quoting it quotes this one.
			//
			// Input that ran out is unfinished rather than wrong, which
			// failUnexpected already distinguishes: a line ending on `&&`
			// is waiting for its other half, and every shell in the panel
			// draws a continuation prompt for it — measured through a pty
			// on all three that have one.
			p.failUnexpected("")
			return left
		}
		p.open = p.open[:depth]
		left = &BinaryExpr{X: left, Op: op, OpPos: pos, Y: right}
	}
	return left
}

// parsePipeline reads commands joined by `|`, with an optional leading `!`
// that negates the whole pipeline rather than its first command.
//
// `time` is read here rather than in parseCommand because that is what it
// binds: the whole pipeline, on either side of the `!` — `time ! true` and
// `! time true` both parse, and both report.
func (p *Parser) parsePipeline() Expr {
	// The two words below are read before a command is parsed at all, so
	// the alias table has to be consulted here or never. Every other
	// reserved word is reached through parseCommand, which asks first.
	p.expandPipelineHead()
	if p.dialect.TimeKeyword && p.atWord("time") {
		return p.parseTime(false, Pos{})
	}
	pl := &Pipeline{}
	if p.atWord("!") {
		pl.Negated = true
		pl.Bang, pl.Stop = p.tok.Pos, p.tok.End
		p.next()
		// Each further `!` inverts the one before it where the dialect says
		// so, which is why one flag on the tree is enough: an even count is
		// no negation and an odd one is a single negation, measured.
		for p.dialect.RepeatedNegationToggles && p.atWord("!") {
			pl.Negated = !pl.Negated
			pl.Stop = p.tok.End
			p.next()
		}
		if p.dialect.TimeKeyword && p.atWord("time") {
			return p.parseTime(true, pl.Bang)
		}
		if p.atWord("!") {
			// A second one, where the dialect does not toggle. It is a
			// reserved word standing where a command belongs and the panel
			// names it: `` "!" unexpected `` in dash and ``parse error near
			// `!'`` in zsh, both of which quote it rather than calling it a
			// word.
			p.failUnexpected("")
			return pl
		}
		if p.bareNegationStandsHere() {
			// A pipeline with no commands in it. Nothing runs and the
			// negation inverts a success, so the status is 1 — which is what
			// the three shells that take the line answer, whatever `$?` held
			// before it. See Dialect.BareNegationReach.
			return pl
		}
	}
	// A bar is recorded while the command after it is being looked for, so
	// that input ending there is describable as a pipeline waiting for its
	// other half and not only as input that ended.
	depth := len(p.open)
	for {
		cmd := p.parseCommand()
		if cmd == nil {
			if len(pl.Cmds) == 0 {
				if pl.Negated && p.err == nil {
					// A `!` was read and nothing the dialect will let it
					// stand in front of came after it. Returning nil here
					// swallowed the `!` outright and left an empty program,
					// so `dash -c '!'` exited 0 where that shell says
					// `` end of file unexpected `` — the refusal names what
					// it found, which is this token.
					p.failUnexpected("")
					return pl
				}
				return nil
			}
			// The token that stopped it is named, not the bar behind it.
			// Every shell in the panel that refuses this quotes what it
			// found — `;`, `&`, the end of input — and none of them says
			// anything about the `|`, which by now is read and was never
			// the problem. It reaches `|&` because a dialect without that
			// operator lexes those two bytes as a bar and an ampersand, so
			// the refusal lands on the `&` where bash 3.2's and dash's do.
			//
			// Not the operand form: a command *may* begin after a bar, so a
			// reserved word standing there is reserved, and the dialect that
			// names a word's class instead of quoting it quotes this one —
			// `echo a | fi` is `"fi" unexpected` there and not `word
			// unexpected`. The only words that reach here are stop words,
			// which are exactly the reserved ones.
			p.failUnexpected("")
			return pl
		}
		// The bar has its command, so the pipeline is whole again: a failure
		// after this has nothing to do with it.
		p.open = p.open[:depth]
		pl.Cmds = append(pl.Cmds, cmd)
		// `|&` joins two commands only where it is the *pipe* reading. Where
		// it is the coprocess operator the pipeline is over and the token
		// belongs to the statement, which is what makes `cat |& | wc -l` a
		// refusal at the second bar — measured, ksh93 says
		// ``syntax error … `|' unexpected``.
		bothStreams := p.at(TokPipeAmp) && p.dialect.PipeBothStreams
		if !p.at(TokPipe) && !bothStreams {
			return pl
		}
		if bothStreams {
			p.mergeStderr(cmd, p.tok.Pos)
		}
		// Pushed rather than opened as a clause: a clause displaces the
		// clause before it, and a bar is not one of those — it belongs to
		// the pipeline and not to whatever construct the pipeline is in. As
		// a clause it evicted the `then` it was written inside, which then
		// reported an `if` waiting for a bar.
		p.open = append(p.open, opener{word: p.tok.Kind.String(), line: int(p.tok.Pos.Line)})
		p.next()
		p.skipNewlines()
		// And a `;` written where the command after the bar belongs, for the
		// one dialect that steps over one there. ksh93 will not: it takes
		// `a || ; b` and refuses `a | ; b`, which is why this asks.
		p.skipSeparators(separatorAfterABar)
	}
}

// parseTime reads `time [-p] [pipeline]`, the keyword already at hand.
//
// The pipeline is read by parsePipeline itself, so `time` takes everything a
// pipeline takes — the bars, an inner `!`, even another `time` — and nothing
// more: an `&&` past it belongs to the caller. A bare `time` is legitimate
// and reports on nothing, but a bare `time` followed by `|` is a pipe with no
// first element, which is the syntax error bash makes of it.
func (p *Parser) parseTime(negated bool, bang Pos) Expr {
	tc := &TimeClause{Negated: negated, Bang: bang, Time: p.tok.Pos, Stop: p.tok.End}
	p.next()
	if p.dialect.TimePosixFlag && p.atWord("-p") {
		tc.Posix, tc.PosixPos = true, p.tok.Pos
		tc.Stop = p.tok.End
		p.next()
	}
	tc.Pipeline = p.parsePipeline()
	if tc.Pipeline == nil && p.at(TokPipe) {
		p.failUnexpected("")
	}
	return tc
}

// parseCommand dispatches on what begins the command.
func (p *Parser) parseCommand() Command {
	// Taken here and cleared, so that only the command the function opened
	// can be its body: anything nested inside is a group like any other.
	body := p.funcBody
	p.funcBody = false
	// Before the keyword dispatch below, because an alias may hold one:
	// `alias iff='if true; then'` has to produce the `if` the grammar reads.
	// The set is fresh per command, so `e yes; e two` expands `e` twice.
	//
	// Unless parsePipeline has already offered this very word — it reads two
	// words of its own in front of a command and so has to ask first. See
	// Parser.aliasHeadHandled.
	if !p.aliasHeadHandled {
		p.expandCommandStart()
	}
	switch {
	case p.at(TokEOF), p.at(TokNewline), p.atStopWord():
		return nil
	case p.at(TokLeftParen) && p.dialect.AnonymousFunction && p.peekIsRightParen():
		// `()` where a command begins is an empty parameter list rather than
		// a subshell with nothing in it — which every dialect refuses, so
		// nothing is taken away by reading it this way.
		return p.withRedirs(p.parseAnonFunc(false))
	case p.at(TokLeftParen):
		return p.withRedirs(p.parseSubshell())
	case p.at(TokArithCmd):
		return p.withRedirs(p.parseArithCmd())
	case p.atWord("{"):
		return p.withRedirs(p.parseGroupOrTry(body))
	case p.atWord("if"):
		return p.withRedirs(p.parseIf())
	case p.atWord("while"), p.atWord("until"):
		return p.withRedirs(p.parseLoop())
	case p.atWord("for"):
		return p.withRedirs(p.parseFor())
	case p.atWord("repeat") && p.dialect.Repeat:
		return p.withRedirs(p.parseRepeat())
	case p.atWord("foreach") && p.dialect.Foreach:
		return p.withRedirs(p.parseForeach())
	case p.atWord("select") && p.dialect.Select:
		return p.withRedirs(p.parseSelect())
	case p.atWord("case"):
		return p.withRedirs(p.parseCase())
	case p.atWord("[[") && p.dialect.DoubleBracket:
		return p.withRedirs(p.parseTestClause())
	case p.atWord("function") && p.dialect.AnonymousFunction && p.peekIsAnonBody():
		return p.withRedirs(p.parseAnonFunc(true))
	case p.atWord("function") && p.dialect.FunctionKeyword:
		return p.parseFuncKeyword()
	case p.atWord("coproc") && p.dialect.Coproc:
		return p.parseCoproc()
	case p.atWord("namespace") && p.dialect.NamespaceBlock:
		return p.withRedirs(p.parseNamespace())
	}
	return p.parseSimple()
}

// parseNamespace reads `namespace NAME { list }`, the one column's
// name-resolution region. See [Dialect.NamespaceBlock] for the panel.
//
// The name is one word, taken **as written** and never expanded: measured
// 2026-09-19 on ksh93u+ 2012-08-01, `n=ns; namespace $n { x=1; }` reports
// `.$n: invalid variable name` and runs nothing, so what the shell judges is
// the text. A word that is no name at all is carried to the interpreter and
// refused there, which is the same stage split [Parser.forName] makes for a
// loop's variable.
//
// The brace is a token of its own and may stand on the next line. `namespace
// ns{ echo hi; }` is a syntax error naming `echo` in that column, which falls
// out of reading the word greedily rather than being written down twice: the
// name is `ns{`, and `echo` is then standing where the brace belongs.
func (p *Parser) parseNamespace() Command {
	c := &NamespaceClause{Start: p.tok.Pos}
	defer p.opens("namespace")()
	p.next()
	if p.tok.Kind != TokWord {
		// `namespace;` and `namespace` at the end of the input: nothing that
		// could be a name stands here, and the grammar complains about the
		// token before the construct gets to complain about the name. The
		// column that has the word names the token it stopped on and names no
		// expectation beside it — `` syntax error at line 1: `;' unexpected ``.
		p.failUnexpected("")
		return c
	}
	c.Name = p.namespaceName()
	p.next()
	// The brace may follow a newline, measured. Nothing else may: a separator
	// there is the failure below, naming the token.
	p.skipNewlines()
	if !p.atWord("{") {
		p.failUnexpected("")
		return c
	}
	g, ok := p.parseGroupOpenedBy("namespace").(*Group)
	if !ok {
		return c
	}
	c.List, c.Stop = g.List, g.Stop
	return c
}

// namespaceName is the word standing where a namespace's name belongs, as it
// was written. [Parser.forNameAsWritten] with a name of its own, because the
// two productions ask the same question of different tokens and a shared
// helper named for the loop would read as one.
func (p *Parser) namespaceName() string {
	for _, sp := range p.tok.Spans {
		if sp.Kind != Literal {
			// A word with an expansion in it is judged as it was *written*:
			// measured, `n=ns; namespace $n { x=1; }` reports `.$n: invalid
			// variable name` and never expands it.
			return p.forNameAsWritten()
		}
	}
	// Quoting is removed, and that is measured rather than assumed:
	// `namespace "ns" { echo hi; }` runs there, where `"namespace" ns { … }`
	// is a syntax error. So the quotes may be on the name and not on the
	// word that reserves the production.
	return p.tok.Literal()
}

// parseCoproc reads `coproc [NAME] command`.
//
// The name is only a name when a compound command follows it — with a simple
// command the first word is the command, which is why `coproc cat` runs cat
// rather than defining a coprocess called cat that runs nothing. Measured:
// bash reports `MY: command not found` for `coproc MY cat`. The construct is
// specified in the `coproc` section of docs/spec/grammar/commands.md.
//
// A dialect with the word and no CoprocName never reads a name at all, so
// `coproc MY { cat; }` is `MY {` followed by an unexpected `}` there — which
// is the failure zsh reports, and it falls out of the missing flag rather
// than being written down twice.
//
// **The word standing there is not required to be a name.** Measured
// 2026-09-17 on bash 5.3.20, from script files under `env -i`: `coproc @
// { :; }`, `coproc a-b { :; }` and `coproc 1x { :; }` all parse, report that
// the word is not an identifier when the clause runs, leave the body unrun
// and carry on at status 1 — where refusing them here ended the script at 2
// and took every later line of it with them. So the grammar takes the word
// and [Runner.coprocClause] judges what it expands to. bash 3.2 does refuse
// these while parsing, and that is not a second answer to this question: it
// has no `coproc` word at all, and what it refuses is `@ {` as a command.
func (p *Parser) parseCoproc() Command {
	c := &CoprocClause{Coproc: p.tok.Pos}
	p.next()
	if p.dialect.CoprocName && p.at(TokWord) && p.mayNameACoproc() {
		w := p.word()
		if p.startsCompoundCommand() {
			c.setName(w)
			c.Cmd = p.parseCommand()
		} else {
			// The word was the command after all, and the rest of the
			// simple command is read from here.
			sc, _ := p.parseSimple().(*SimpleCmd)
			if sc == nil {
				sc = &SimpleCmd{Start: w.Pos(), Stop: p.tok.Pos}
			}
			sc.Args = append([]*Word{w}, sc.Args...)
			sc.Start = w.Pos()
			c.Cmd = sc
		}
	} else {
		c.Cmd = p.parseCommand()
	}
	if c.Cmd == nil && p.err == nil {
		p.failUnexpected("")
		return nil
	}
	if c.Cmd != nil {
		c.Stop = c.Cmd.End()
	}
	return c
}

// mayNameACoproc reports whether the current word could be the name in
// `coproc NAME compound` rather than the start of the command itself.
//
// It is not a check that the word *is* a name — see parseCoproc — but a check
// that reading it as one cannot swallow the command. A word that opens a
// compound command is the command (`coproc { :; }`, `coproc if …`), a stop
// word ends the enclosing list, and `!` is refused outright where the name
// belongs: measured the same day, `coproc ! { :; }` is a syntax error at the
// `!` in bash 5.3.20 while `coproc @ { :; }` parses.
func (p *Parser) mayNameACoproc() bool {
	lit := p.tokenLiteral()
	return lit != "!" && !stopWords[lit] && !p.startsCompoundCommand()
}

// setName records the word written where a coprocess name belongs, as text
// where that is all it is and as the word itself otherwise. See
// [CoprocClause.NameWord].
func (c *CoprocClause) setName(w *Word) {
	if lit, ok := unquotedLiteralWord(w); ok && isPlainName(lit) {
		c.Name = lit
		return
	}
	c.NameWord = w
}

// startsCompoundCommand reports whether the current token opens a compound
// command — the question `coproc NAME …` turns on.
func (p *Parser) startsCompoundCommand() bool {
	if p.at(TokLeftParen) || p.at(TokArithCmd) {
		return true
	}
	if !p.at(TokWord) {
		return false
	}
	switch p.tokenLiteral() {
	case "{", "if", "while", "until", "for", "case", "select", "[[":
		return true
	}
	return false
}

// isPlainName reports whether s could name a variable: letters, digits and
// underscores, not starting with a digit.
func isPlainName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// withRedirs attaches trailing redirections to a compound command, because a
// redirection on one applies to everything inside it.
func (p *Parser) withRedirs(c Command) Command {
	type hasRedirs interface{ addRedir(*Redirect) }
	h, ok := c.(hasRedirs)
	if !ok {
		return c
	}
	for p.tok.Kind.IsRedirect() || p.at(TokIONumber) {
		r := p.parseRedirect()
		if r == nil {
			break
		}
		h.addRedir(r)
	}
	return c
}

func (r *redirs) addRedir(x *Redirect) { r.Redirs = append(r.Redirs, x) }

// prependRedirs puts redirections written *in front* of a compound command at
// the head of its list, which is the order they were written in and the order
// they are applied in. See [Dialect.RedirectionBeforeACompound].
func (r *redirs) prependRedirs(xs []*Redirect) {
	r.Redirs = append(append(make([]*Redirect, 0, len(xs)+len(r.Redirs)), xs...), r.Redirs...)
}

// prependRedirs on a definition reaches its body, as addRedir does and for the
// same reason: the definition itself has no list to hold one.
func (c *FuncDecl) prependRedirs(xs []*Redirect) {
	if h, ok := c.Body.(interface{ prependRedirs([]*Redirect) }); ok {
		h.prependRedirs(xs)
	}
}

func (c *SimpleCmd) addRedir(x *Redirect) { c.Redirs = append(c.Redirs, x) }

// addRedir on a definition reaches its body, which is where the parser
// already puts a written one: `f() { :; } 2>&1` reads the redirection as the
// body compound's, and the definition itself has no list to hold it.
func (c *FuncDecl) addRedir(x *Redirect) {
	if h, ok := c.Body.(interface{ addRedir(*Redirect) }); ok {
		h.addRedir(x)
	}
}

// mergeStderr attaches the redirection `|&` stands for to the command before
// it, at the end of that command's own list.
//
// At the end and not the start, which is the whole of the operator: both
// shells that have it show the error for `e 2>/dev/null |& cat` and show
// nothing for `e >/dev/null |& cat`, and only a `2>&1` written *after*
// whatever the command wrote for itself gives that pair of answers.
//
// A command with nowhere to put a redirection is left alone. The one that
// reaches here is a definition whose body never parsed, which is already an
// error, and a definition writes nothing either way.
func (p *Parser) mergeStderr(cmd Command, pos Pos) {
	h, ok := cmd.(interface{ addRedir(*Redirect) })
	if !ok {
		return
	}
	h.addRedir(&Redirect{
		N:        literalWord("2", pos),
		Op:       TokGreatAmp,
		OpPos:    pos,
		Text:     "1",
		Word:     literalWord("1", pos),
		PipeBoth: true,
	})
}

// literalWord is a word the grammar supplies rather than one the script
// wrote, positioned at the operator it stands for.
//
// It is also what a flag group's operand is inside an *arithmetic*
// expression: the text there has already been expanded once — `$(( ))`
// substitutes into the whole expression before reading any of it — so lexing
// it again would perform a substitution twice and read a leftover `$` as the
// start of one.
func literalWord(text string, pos Pos) *Word {
	return &Word{
		Spans: []Span{{Kind: Literal, Value: text, Quoting: Unquoted, Pos: pos}},
		Start: pos, Stop: pos,
	}
}

func (p *Parser) word() *Word {
	if p.tok.Kind != TokWord {
		return nil
	}
	w := p.newWord(p.tok.Spans, p.tok.Pos, p.tok.End)
	p.next()
	return w
}

// newWord builds a word and parses the expansions inside it. Every word in
// the tree goes through here, so no path can produce one with an unparsed
// ${ } in it.
func (p *Parser) newWord(spans []Span, start, stop Pos) *Word {
	out := make([]Span, len(spans))
	copy(out, spans)
	for i := range out {
		// Whether the script's own read of this line could see the span at
		// all. A fragment re-read with the quotes standing for themselves
		// yields spans that were inside quotes the first time through, and
		// those are not the line's — see Parser.hiddenFromTheScriptsRead.
		hidden := p.hiddenFromTheScriptsRead(out[i])
		// The substitutions a dialect reads with the line, gathered for the
		// shell that will read them. Here because every word in the tree
		// comes through this function, an expansion's operand included, and
		// because the *lexer* that built the span cannot see which word it
		// ended up in. See File.Substitutions (#2857).
		if !hidden && p.dialect.SubstitutionBodyRead != SubstitutionBodyReadWhenItRuns &&
			p.readsBodyWithItsLine(out[i]) {
			p.lineSubsts = append(p.lineSubsts, out[i])
		}
		switch {
		case out[i].Kind == ParamExp && out[i].Param == nil:
			// An expansion the first read could not see hides everything
			// inside it too, however many operands deep the nesting goes.
			outer := p.allHiddenFromTheScriptsRead
			p.allHiddenFromTheScriptsRead = outer || hidden
			out[i].Param = p.parseParamExp(out[i].Value, out[i].Pos, out[i].Quoting, out[i].Bare)
			p.allHiddenFromTheScriptsRead = outer
		case out[i].Kind == ArithSubst && out[i].Arith == nil:
			// Deferring, like the arithmetic command: a `$(( ))` whose
			// expression will not read does not refuse the file. Both
			// spellings come through here, so `$[echo hi]` is covered by
			// the same line (#865).
			out[i].Arith = p.parseArithLater(out[i].Value, out[i].Pos)
		}
	}
	// An unbraced `$name` with a `[` behind it that the word never closes.
	// Here rather than in the lexer for the reason the field carries: the
	// question is about where the *word* ends, which scanBareParam does not
	// know when it gives the brackets back. See ParamExpr.BareIndexUnclosed.
	if p.dialect.BareSubscript {
		for i := range out {
			e := out[i].Param
			if out[i].Kind != ParamExp || e == nil || !out[i].Bare ||
				e.Index != nil || e.BareIndexText != nil ||
				!takesBareSubscript(out[i].Value) {
				continue
			}
			from := stop
			if i+1 < len(out) {
				from = out[i+1].Pos
			}
			rest := p.sourceBetween(from, stop)
			e.BareIndexUnclosed = strings.HasPrefix(rest, "[") &&
				bareSubscriptClose(rest, p.dialect, nil) < 0
		}
	}
	// Where a `}` ends a `${ … }` is a grammar question POSIX mode moves in
	// one dialect, and the mode is entered long after the word was cut. Same
	// place and the same reason as the tail below: this is where the word
	// ends. See ParamExpr.RawTail.
	p.setRawTails(out, stop)
	// A flag group the parser could not read reports the rest of the word it
	// stands in, and this is the one place that knows where the word ends —
	// scanParamFlags is called with the braces alone, halfway through.
	// Gated on the failure, so the common word pays a nil check.
	// See ParamExpr.FlagsErrTail for what the text is and why.
	for i := range out {
		if out[i].Kind == ParamExp && out[i].Param != nil && out[i].Param.FlagsErrPos > 0 {
			out[i].Param.FlagsErrTail = p.sourceBetween(out[i].Pos, stop)
		}
	}
	// And the dialect that has no flag groups at all refuses the same text
	// while reading it, so the tail lands on the error rather than on a
	// node. Same word and same end; a different start, because that shell
	// quotes back only what follows the group's `)`.
	if from := p.flagTailFrom; from > 0 {
		p.flagTailFrom = 0
		tail := flagGroupTail(p.sourceBetween(Pos{Offset: from, Line: 1, Col: 1}, stop))
		if pe, isErr := p.err.(*Error); isErr && pe.FlagGroupWordTail == "" {
			pe.FlagGroupWordTail = tail
		}
		// And where the dialect carries the refusal to the run instead, the
		// same text lands on the failure the node is holding: the word ends
		// in the same place whether the sentence is written now or later,
		// and reading it twice is how one rule becomes two.
		for i := range out {
			if out[i].Kind != ParamExp || out[i].Param == nil {
				continue
			}
			if pe := out[i].Param.RefusedAtExpansion; pe != nil && pe.FlagGroupWordTail == "" {
				pe.FlagGroupWordTail = tail
			}
		}
	}
	return &Word{Spans: out, Start: start, Stop: stop}
}

// sourceBetween is the input between two positions, or empty where they do
// not name a piece of it.
//
// The guards are not defensive clutter: a word the grammar supplied stands
// at a single position with nothing between its ends, and an alias body is
// lexed apart from the input these offsets count in, so an offset pair that
// does not describe a range of this lexer's source describes some other
// string and must produce nothing rather than a slice of the wrong text.
func (p *Parser) sourceBetween(from, to Pos) string {
	src := p.lex.src
	lo, hi := int(from.Offset), int(to.Offset)
	if lo < 0 || hi > len(src) || lo >= hi {
		return ""
	}
	return src[lo:hi]
}

// parseRedirect reads an optional IO number, an operator and its target.
func (p *Parser) parseRedirect() *Redirect {
	r := &Redirect{}
	if p.at(TokIONumber) {
		r.N = p.newWord(p.tok.Spans, p.tok.Pos, p.tok.End)
		p.next()
	}
	if !p.tok.Kind.IsRedirect() {
		p.fail("expected a redirection operator")
		return nil
	}
	r.Op, r.OpPos = p.tok.Kind, p.tok.Pos
	// A target is read where a command would otherwise begin, and no
	// assignment may be written there: `> a==(echo hi) cat` is `missing end
	// of string` in the dialect where the same word at command position
	// assigns a path. Written with the redirection *first*, because that is
	// the only one this reaches — a target after a word is already in
	// argument position. Set before the `p.next()` that reads the target,
	// because that is the token the flag has to reach; see
	// Lexer.noAssignment.
	//
	// inRedirectTarget goes with it and says only *that* this is a target;
	// whether a subscript in it spans separators is decided beside
	// inArgument, in the lexer. See atSpanningRedirectTarget.
	//
	// inHeredocDelimiter goes with them for the here-document operators,
	// whose target is a delimiter rather than a path: nothing in a delimiter
	// expands, so `<<$d` waits for a line reading `$d`. The operator is
	// already known here, which is why the flag can be set before the word
	// is read at all.
	savedNoAssign, savedInRedirect := p.lex.noAssignment, p.lex.inRedirectTarget
	savedDelimiter := p.lex.inHeredocDelimiter
	p.lex.noAssignment, p.lex.inRedirectTarget = true, true
	p.lex.inHeredocDelimiter = r.Op.IsHeredoc()
	p.next()
	p.lex.noAssignment, p.lex.inRedirectTarget = savedNoAssign, savedInRedirect
	p.lex.inHeredocDelimiter = savedDelimiter
	if r.Op.IsSeek() {
		// The file-position operators take an arithmetic command where every
		// other redirection takes a target: `exec 3<#((0))` seeks, and the
		// expression may name parameters. The token already holds the
		// expression as an ArithSubst span — the same span `$((…))` produces
		// — so the word is built from it and the offset is whatever that
		// expands to, with no second evaluator to keep in step.
		//
		// The text is kept as it was written, which is both what the printer
		// writes back and the only record that the parentheses were there.
		if p.tok.Kind != TokArithCmd {
			// ksh93 reads a pattern here as well and seeks to the line that
			// matches it; this grammar does not claim that half, so a word
			// is refused rather than read as something it is not (#3034).
			p.failUnexpectedOperand("a seek offset")
			return nil
		}
		r.Word = p.newWord(p.tok.Spans, p.tok.Pos, p.tok.End)
		r.Text = p.textBetween(p.tok.Pos, p.tok.End)
		p.next()
		return r
	}
	if p.tok.Kind != TokWord {
		// The token that is there, not the one that is missing. `cat <(x)` in
		// a dialect without process substitution is `"(" unexpected` in dash,
		// which is what it says about every other token in the wrong place —
		// and this was the one redirection failure that said something else.
		//
		// Nothing named where a token is there, because dash names what it
		// was waiting for only when that is a keyword. At end of input there
		// is a construct to name, and the wording that reports one always
		// prints the clause.
		if p.at(TokEOF) {
			p.failUnexpectedOperand("a redirection target")
		} else {
			p.failUnexpectedOperand("")
		}
		return nil
	}
	// Built without advancing, because advancing is what reaches the newline
	// where the body is read — and the queue has to be set before that
	// happens. Registering after p.word() looks equivalent and silently
	// collects nothing.
	r.Word = p.newWord(p.tok.Spans, p.tok.Pos, p.tok.End)
	// As it was written, for the one dialect that names it when the target
	// turns out not to be a single word. Taken from the input rather than
	// rebuilt from the spans: `$e` and `${e}` are the same word and not the
	// same text, and it is the text that goes in the message.
	r.Text = p.textBetween(p.tok.Pos, p.tok.End)
	if r.Op == TokTLess && p.refuseProcSubstOutOfPlace(r.Word) {
		// A here-string's operand is text to be fed in rather than a file to
		// be opened, and the dialect that admits `cat < <(:)` refuses
		// `cat <<< <(:)` while reading. The operator is what separates them,
		// so it is tested here rather than in the helper (#930).
		return nil
	}
	if r.Op.IsHeredoc() {
		// Any quoting *anywhere* in the delimiter makes the whole body
		// literal, and a backslash counts. Both are detected the same way:
		// if removing quotes changed the text, it was quoted. `\EOF` and
		// `"EOF"` both differ from their literal; a bare `EOF` does not.
		quoted := p.tok.Text != p.tok.Literal()
		if b := p.tok.aliasBody; b != nil {
			// The alias value held the body as well as the operator, and
			// it was read from there when the value was. See
			// Parser.readAliasHeredocs.
			r.Heredoc = b.Heredoc
		} else {
			// The body starts after the next newline, which the lexer
			// reaches; the delimiter is here, which the parser has. Hence
			// the handoff.
			p.lex.queueHeredoc(r, quoted)
		}
	}
	p.next()
	return r
}

// assignHead is the name half of an assignment, already taken apart: the name,
// the subscript if one was written, whether the `=` was an append, and where
// the value begins.
//
// The subscript is a span sequence rather than a string because it may hold
// expansions — `a[$i]=v` is how a loop ordinarily writes an element — and a
// substitution is not text the parser may flatten. That is also why the head
// carries a position into the token rather than a length: the value's first
// span is whatever is left of the span holding the `=`, and the spans after
// it belong to the value whole.
type assignHead struct {
	name   string
	index  []Span // the subscript's spans, nil when none was written
	append bool
	span   int // the span holding the `=`
	off    int // byte offset just past the `=`, within that span's value
	// from and to bracket the subscript in the input, just inside the
	// brackets: the source text a diagnostic quotes back. Zero when no
	// subscript was written. See Assign.IndexText.
	from, to Pos
	// leading are the subscripts written before the last one, where the
	// dialect lets a name carry several. Nil is the ordinary one subscript.
	// See Assign.Leading and Dialect.ChainedAssignSubscript.
	leading []assignSubscript
	// subscripted says brackets were written after the name, which index
	// cannot say on its own: `a[]=v` has brackets and no spans between them,
	// and comes here indistinguishable from `a=v` without this. See
	// Assign.EmptySubscript.
	subscripted bool
	// member is the dotted path written between the closing bracket and the
	// `=`, leading dot and all. See Assign.Member.
	member string
}

// assignSubscript is one link of a chained assignment subscript, in the same
// three pieces the head keeps for the final one.
type assignSubscript struct {
	spans    []Span
	from, to Pos
}

// emptySubscriptWritten reports whether any of an assignment's subscripts was
// written with nothing between its brackets — the last one, which the caller
// has already decided, or any link in front of it.
//
// Every link and not only the last, which is measured: on ksh93u+
// 2012-08-01, `a[][2]=6` and `a[2][]=6` are both “syntax error at line 1:
// `[]' empty subscript“. A chain is the same subscript twice, so nothing
// here may know less about a link than about the one it precedes.
func emptySubscriptWritten(last bool, leading []assignSubscript) bool {
	if last {
		return true
	}
	for _, link := range leading {
		if link.spans == nil {
			return true
		}
	}
	return false
}

// failEmptyAssignSubscript records `a[]=v` in the grammar that refuses it
// while reading — see [Dialect.EmptyAssignSubscriptIsASyntaxError].
//
// Its own recorder rather than the unexpected-token path's, because there is
// no token to name: what is refused is a pair of brackets inside a word the
// lexer has already taken whole, and the sentence that shell writes names
// those two characters.
func (p *Parser) failEmptyAssignSubscript(pos Pos) {
	if p.err != nil {
		return
	}
	p.err = &Error{
		Pos: pos, Kind: ErrEmptyAssignSubscript,
		Token: "[]",
		Msg:   "`[]' empty subscript",
	}
}

// isAssign reports whether a word is `name=…` written so the name is unquoted.
func (p *Parser) isAssign(t Token) (assignHead, bool) {
	var h assignHead
	if t.Kind != TokWord || len(t.Spans) == 0 || t.Spans[0].Quoting != Unquoted ||
		t.Spans[0].Kind != Literal {
		return h, false
	}
	head := t.Spans[0].Value
	eq := strings.IndexByte(head, '=')
	// `name[i]=` is an assignment too, and the bracket has to be looked for
	// before the `=`: a `=` inside the value — `a=b[0]` — is the ordinary
	// scalar case and must not be read as a subscript.
	if open := strings.IndexByte(head, '['); open > 0 && (eq < 0 || open < eq) {
		return p.subscriptedAssign(t, open)
	}
	// Only the first span is looked at here, and that is the rule rather than
	// a shortcut: `a$b=c` is a command name in every shell on the panel, so a
	// name interrupted by an expansion is not a name.
	if eq <= 0 {
		return h, false
	}
	name := head[:eq]
	h.span, h.off = 0, eq+1
	// `name+=value` appends. The `+` is part of neither the name nor the
	// value, so it is taken off here.
	if strings.HasSuffix(name, "+") {
		if !p.dialect.AppendAssign {
			return h, false
		}
		name = name[:len(name)-1]
		h.append = true
	}
	if !isNameIn(name, p.dialect.DottedName) {
		// A run of digits names a positional parameter where the dialect has
		// the construct. Behind isName rather than inside it, because every
		// other caller of that function is asking about an identifier — a
		// `for` variable, a function's name, a declaration's operand — and
		// none of them takes a digit in any shell on the panel.
		if !p.dialect.PositionalAssignment || !isDigitRun(name) {
			return h, false
		}
	}
	h.name = name
	return h, true
}

// isDigitRun reports whether s is one or more decimal digits and nothing else.
//
// Leading zeros included: `01=z` writes the first positional parameter in the
// shell with the construct, measured 2026-09-07, so the digits are read as a
// number rather than matched against a canonical spelling.
func isDigitRun(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// subscriptedAssign reads the `name[subscript]=` shape, whose subscript may
// hold expansions and so may run across several spans. open is where the `[`
// stands in the token's first span.
//
// The closing `]` is the first one written as unquoted literal text with an
// `=` or a `+=` after it, so that anything a quoted or substituted span
// contributes is data — the subscript is expanded later, and a `]` that
// arrives from an expansion never closes anything.
func (p *Parser) subscriptedAssign(t Token, open int) (assignHead, bool) {
	var h assignHead
	// Without arrays there is no subscript to read, and the word is a command
	// name: the shell without them answers `a[1]=Q: not found`.
	if !p.dialect.ArraySubscript {
		return h, false
	}
	name := t.Spans[0].Value[:open]
	if !isNameIn(name, p.dialect.DottedName) {
		return h, false
	}
	// Where the chain starts, which is the open bracket until a link has been
	// closed and then just inside the bracket after it. A chain is one
	// dialect's and is otherwise dead weight: with the flag off no `]` is
	// ever read as a link and these never move.
	startSpan, startOff := 0, open+1
	for i, s := range t.Spans {
		if s.Kind != Literal || s.Quoting != Unquoted {
			continue
		}
		from := 0
		if i == 0 {
			from = open + 1
		}
		for j := from; j < len(s.Value); j++ {
			if s.Value[j] != ']' {
				continue
			}
			rest := s.Value[j+1:]
			if p.dialect.ChainedAssignSubscript && strings.HasPrefix(rest, "[") {
				// A `]` with a `[` behind it closes a *link* rather than the
				// name: the subscript after it reads into what this one
				// named. Without the flag it closes nothing and the text
				// runs on to the next candidate, which is the reading every
				// other dialect keeps — `a[1][2]=v` has the four-character
				// subscript `1][2` there, and is refused for it.
				h.leading = append(h.leading, assignSubscript{
					spans: spanRange(t.Spans, startSpan, startOff, i, j),
					from:  p.subscriptPos(t, startSpan, startOff),
					to:    offsetBy(s.Pos, j),
				})
				startSpan, startOff = i, j+2
				continue
			}
			// A dotted member path may stand between the bracket and the
			// `=`: `a[1].p=9` writes the member `p` of the compound the
			// element holds. Taken off before the operator is looked for,
			// because it is part of what is being named and not part of the
			// value — the same reading `${a[1].p}` takes. See Assign.Member.
			member, rest := memberPath(rest, p.dialect.DottedName)
			appends := false
			if strings.HasPrefix(rest, "+=") {
				if !p.dialect.AppendAssign {
					continue
				}
				appends = true
			} else if !strings.HasPrefix(rest, "=") {
				continue
			}
			h.name = name
			h.member = member
			h.append = appends
			h.subscripted = true
			h.index = spanRange(t.Spans, startSpan, startOff, i, j)
			// Just inside the brackets, in the input. Both ends land in an
			// *unquoted literal* span — the `[` in the first one and the `]`
			// in this one, which the loop above has already required — and
			// such a span's value is its source byte for byte, which is the
			// same assumption spanRange makes when it slides a clipped
			// span's position along.
			h.from = p.subscriptPos(t, startSpan, startOff)
			h.to = offsetBy(s.Pos, j)
			h.span = i
			h.off = j + 1 + len(member)
			if appends {
				h.off++
			}
			h.off++ // the `=` itself
			return h, true
		}
	}
	return h, false
}

// subscriptPos is where one link of an assignment's subscript begins in the
// input, just inside its opening bracket.
//
// A span's value is its source byte for byte only where it was written as
// unquoted literal text, which is what the caller has already required of
// every span it hands here — the same assumption spanRange makes when it
// slides a clipped span's position along.
func (p *Parser) subscriptPos(t Token, span, off int) Pos {
	return offsetBy(t.Spans[span].Pos, off)
}

// assignIndexFlags reads the flag group an assignment's subscript may open
// with, which is the same group a *read* takes and the same scanner.
//
// `a[(r)y]=Q` replaces the element whose value is `y`, and `a[(i)nomatch]=W`
// appends, because `(i)` missing answers one past the last element and that is
// the index an append writes to. The semantics need nothing new: the group
// names an *index*, which is what a subscript on this side has always been.
//
// Only a group written as unquoted literal text at the front of the subscript
// is one. That is the same rule the read side follows and it is what keeps
// `a["(r)y"]=Q` and `a[$g]=Q` out: a group that arrives from an expansion is
// text, since the operand behind it is lexed as a word of its own and a group
// the source did not write has no operand to lex.
func (p *Parser) assignIndexFlags(spans []Span) *SubscriptFlags {
	if !p.dialect.ArraySubscriptFlags || len(spans) == 0 {
		return nil
	}
	if spans[0].Kind != Literal || spans[0].Quoting != Unquoted {
		return nil
	}
	g, rest, ok := scanSubscriptFlags(spans[0].Value)
	if !ok {
		return nil
	}
	// The operand is the rest of the first span plus every span after it, so
	// a substitution in it is performed exactly as one in the subscript would
	// be — `a[(re)$want]=Q` is the shape worth having.
	operand := append([]Span{{
		Kind: Literal, Value: rest, Quoting: Unquoted, Pos: spans[0].Pos,
	}}, spans[1:]...)
	g.Arg = p.newWord(operand, spans[0].Pos, spans[0].Pos)
	return g
}

// toEnd is spanRange's toOff for "as far as the spans go".
const toEnd = -1

// spanRange takes the run of spans from fromOff in span from through span to,
// stopping at toOff there — or at the end of the run when toOff is toEnd. A
// partial literal keeps its own position advanced by the offset, so a
// diagnostic about a subscript points at the subscript.
//
// Empty pieces are dropped: `a[$i]=v` splits its first span at the bracket
// with nothing before the expansion, and a zero-length literal span would be
// a word the printer writes back and the expander has to carry.
//
// **An empty span that was written *quoted* is kept**, and it is the one
// exception. Such a span is not a piece of text that came out empty — it is
// the record that quotes were written, and it is the whole of the difference
// between `m[""]=4` and `m[]=4`: the first is a subscript holding the empty
// key and the second is brackets with nothing in them, which five of six
// shells refuse. Dropped, the two parsed identically and the assignment
// arrived with no subscript at all, so `typeset -A m; m[""]=4` stored under
// the key `0` — the empty-expression reading of a name that had never asked
// for one (#1938).
func spanRange(spans []Span, from, fromOff, to, toOff int) []Span {
	var out []Span
	for i := from; i <= to && i < len(spans); i++ {
		s := spans[i]
		if s.Kind != Literal {
			// A substitution is indivisible, so an offset can only fall
			// before or after it, never inside.
			out = append(out, s)
			continue
		}
		lo, hi := 0, len(s.Value)
		if i == from {
			lo = fromOff
		}
		if i == to && toOff != toEnd {
			hi = toOff
		}
		if lo >= hi {
			if s.Quoting != Unquoted && len(s.Value) == 0 {
				// The written-quotes record, kept whole — see above. Only a
				// span that *is* empty, never one this range clipped to
				// nothing, which is text belonging to a neighbor.
				out = append(out, s)
			}
			continue
		}
		s.Pos = offsetBy(s.Pos, lo)
		s.Value = s.Value[lo:hi]
		out = append(out, s)
	}
	return out
}

// isName reports whether s is a shell name: the production the grammar spells
// `name`, which POSIX defines as an identifier. `for 1 in …` is rejected by
// every shell for this reason.
// funcNamePunctuation is the punctuation a function name may carry where the
// dialect allows any, and it is measured rather than chosen — see
// [Dialect.FunctionNamePunctuation] for the run and for what was left out.
//
// `=` is not here and cannot be: an array assignment is a parenthesis after a
// word too, so admitting it would read `a=()` as a definition of a function
// called `a=`.
const funcNamePunctuation = "!#%+,-./:@]^"

// isFuncName is isName with that punctuation, per the flag, and with the bytes
// above ASCII — a name written in another script is a name to every shell that
// has punctuated ones at all.
// tokenHoldsAnExpansion reports whether a word token has a span the shell
// would expand, which is exactly when [Token.Literal] loses information.
//
// The test the name check needs: `_p_${w}` flattens to `_p_w`, a perfectly
// good name for a different function, so "is the literal a name" cannot see
// the difference and answered yes.
func tokenHoldsAnExpansion(t Token) bool { return spansHoldAnExpansion(t.Spans) }

func spansHoldAnExpansion(spans []Span) bool {
	for _, s := range spans {
		if s.Kind != Literal {
			return true
		}
	}
	return false
}

// keywordFuncName reports whether the word after the `function` keyword is a
// name.
//
// Two readings, and which one runs is [Dialect.FunctionKeywordNameIsAnyWord].
// Without it the name is checked as text, exactly as the POSIX form's is, and
// the two share isFuncName so that they cannot come apart. With it there is no
// text check at all: the shell that has the flag reads a word and the word is
// the name, whatever is in it.
//
// The one thing the flag does not carry is a name the shell would match
// against the *filesystem*. Measured on zsh 5.9.2: `function a*b { :; }` is
// `no matches found: a*b`, `a?b` the same, and `a[b` is `bad pattern: a[b` —
// so those are not definitions there either, and taking them as literal names
// here would put a function called `a*b` in the table at status 0 where the
// shell being modeled defines nothing. A refusal is the visible answer and a
// plausible wrong definition is not, so they stay refused. The quoting is what
// decides it and is read per span: `'a*b'` is ordinary text and is a name,
// `a*'b'` is not, because the `*` in it is still bare.
func (p *Parser) keywordFuncName(t Token) bool {
	if p.dialect.FunctionNameIsAnyBareWord {
		// The third reading: how the word was *written* decides it, so a
		// bare word is a name whatever its characters — `function a*b { … }`
		// defines there, the filesystem never being consulted — and a word
		// carrying any quoting is not one. The caller has already dealt with
		// an expansion, which is the other half of "written bare".
		return tokenIsWrittenBare(t)
	}
	if p.dialect.FunctionKeywordNameIsAnyBareWord {
		// The fourth reading, and the keyword form's alone: a bare word is a
		// name whatever its characters, and a word carrying quoting or an
		// expansion is not one — which leaves it to be refused where this
		// dialect already refuses it, in its own words and at its own
		// status. See [Dialect.FunctionKeywordNameIsAnyBareWord].
		return tokenIsWrittenBare(t)
	}
	if !p.dialect.FunctionKeywordNameIsAnyWord {
		return isFuncName(p.funcNameText(t), p.dialect.FunctionNamePunctuation)
	}
	return !holdsBarePatternCharacter(t)
}

// tokenIsWrittenBare reports whether every part of t was written as plain
// unquoted text: no quoting of any kind, and nothing the shell would expand.
//
// The test [Dialect.FunctionNameIsAnyBareWord] turns on, and it is about the
// *spelling* rather than about the text — `\f` and `'f'` and `a"b"` all come
// to names a shell would take, and none of them was written bare.
func tokenIsWrittenBare(t Token) bool {
	if len(t.Spans) == 0 {
		return false
	}
	for _, sp := range t.Spans {
		if sp.Kind != Literal || sp.Quoting != Unquoted {
			return false
		}
	}
	return true
}

// funcNameText is the text a definition's name is tested as.
//
// One dialect reads the word as it was **written** and the rest read what it
// comes to, which is [Dialect.FunctionNameIsSourceText] and is the whole of
// the difference between defining `f` from `function 'f'` and refusing it.
// The refused word is carried on the declaration as source text either way,
// so this changes which words are refused and never how one is named.
func (p *Parser) funcNameText(t Token) string {
	if p.dialect.FunctionNameIsSourceText {
		return p.textBetween(t.Pos, t.End)
	}
	return t.Literal()
}

// referenceListName reports whether t may stand in the word list a `function`
// keyword's name may be followed by in one dialect.
//
// A name and nothing else: quoting comes off first — `function a "b"` is
// taken there — and an expansion, an assignment and a word that is not an
// identifier are one refusal between them. See
// [Dialect.FunctionKeywordReferenceList], where the rows are.
func (p *Parser) referenceListName(t Token) bool {
	return !tokenHoldsAnExpansion(t) && isName(t.Literal())
}

// holdsBarePatternCharacter reports whether any literal span of t that the
// shell would match against the filesystem carries a pattern character.
//
// Per span rather than over [Token.Literal], because quoting is what decides
// whether a character is a pattern and Literal has already dropped it.
func holdsBarePatternCharacter(t Token) bool {
	return spansHoldBarePatternCharacter(t.Spans)
}

func spansHoldBarePatternCharacter(spans []Span) bool {
	for _, s := range spans {
		if s.Kind != Literal || s.Quoting != Unquoted {
			continue
		}
		if strings.ContainsAny(s.Value, "*?[") {
			return true
		}
	}
	return false
}

func isFuncName(s string, punctuation bool) bool {
	if isName(s) {
		return true
	}
	if !punctuation || s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 0x80 || c == '_' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			strings.IndexByte(funcNamePunctuation, c) >= 0
		if !ok {
			return false
		}
	}
	return true
}

func isName(s string) bool { return isNameIn(s, false) }

// isNameIn is isName with [Dialect.DottedName]'s answer carried in, for the
// positions that read a name the interpreter will then look up. The callers
// that ask about something else — a function's name, a `function` keyword's
// reference list — keep the strict spelling, because a dot is not measured
// there.
func isNameIn(s string, dot bool) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !nameByte(s[i], i, dot) {
			return false
		}
	}
	return true
}

// parseSimple reads assignments, arguments and redirections in any order.
//
// They interleave in the source — `>b echo hi` and `echo one >b two` both work
// — so redirections are lifted out wherever they appear rather than expected
// as a suffix.
func (p *Parser) parseSimple() Command {
	c := &SimpleCmd{Start: p.tok.Pos}
	seenArg := false
	// Whether an unquoted `[[` word has been read and its `]]` has not, in a
	// dialect whose `[[` is a command. See Dialect.DoubleBracketIsACommand.
	inDoubleBracket := false
	// Argument position lasts as long as this command does. The token that
	// ends it has already been read by the time this returns, so restoring
	// the flag here is soon enough and is the only place that catches every
	// way out.
	defer func() { p.lex.inArgument, p.lex.inDeclarationOperand = false, false }()

	for p.err == nil {
		switch {
		case p.at(TokIONumber), p.tok.Kind.IsRedirect():
			if r := p.parseRedirect(); r != nil {
				c.Redirs = append(c.Redirs, r)
			}
		case inDoubleBracket && (p.at(TokAndAnd) || p.at(TokOrOr)):
			// The connective of a `[[` that is a command, which ends nothing:
			// it is an operand the builtin reads. A literal word of the
			// operator's own spelling, so the tree prints back as written
			// and the reprint reads the same way.
			text := "&&"
			if p.at(TokOrOr) {
				text = "||"
			}
			p.lex.inArgument = true
			c.Args = append(c.Args, p.newWord([]Span{{Value: text, Pos: p.tok.Pos}}, p.tok.Pos, p.tok.End))
			p.next()
		case len(c.Args) == 0 && len(c.Assigns) == 0 && len(c.Precommands) == 0 &&
			len(c.Redirs) > 0 && p.redirectionMayPrecedeThisCompound():
			// Two dialects take a compound command's redirections in front
			// of it as well as after it. Only where nothing but redirections
			// has been read: an assignment prefix takes the reading away in
			// both of them. See Dialect.RedirectionBeforeACompound.
			return p.compoundBehindRedirections(c.Redirs)
		case len(c.Args) == 0 && len(c.Assigns) == 0 && len(c.Precommands) == 0 &&
			len(c.Redirs) > 0 && p.reservedBehindARedirection():
			// The same column keeps a reserved word's reading behind a
			// redirection, and a word that is not a command has nowhere to
			// stand there. See Parser.reservedBehindARedirection.
			p.failUnexpected("")
			return c
		case p.at(TokWord):
			// A function definition announces itself only at the paren —
			// which is why the words before it are read as arguments first
			// and turned into names here, where the dialect gives one body
			// several. An assignment ends that reading and is measured to:
			// `x=1 a b () { … }` is a refusal in the shell that has the
			// list, so the guard is the same one the first word takes.
			if len(c.Assigns) == 0 && !seenArg && p.looksLikeFuncDef() {
				return p.parseFuncPosix()
			}
			if !seenArg && p.atReservedPrecommand() {
				c.Precommands = append(c.Precommands, p.word())
				// The word after it stands where a command word stands, so
				// an alias there expands: `alias e=echo; nocorrect e hi`
				// prints `hi`. parseCommand expanded the *first* word before
				// dispatching here and this is the next one, so the call has
				// to be made again — leaving it out made the alias an
				// ordinary command name and `command not found` the answer.
				if p.Aliases != nil || p.SuffixAliases != nil {
					p.aliasNextWord = false
					p.expandCommandWord(map[string]bool{})
				}
				// And only a simple command may follow. A reserved word has
				// nowhere to go after it — `nocorrect if true; then echo hi;
				// fi` is `parse error near `if'` in the shell that has the
				// word, not an `if` with a modifier in front of it.
				if p.atStopWord() || p.atReservedWord() {
					p.failUnexpected("")
					return c
				}
				continue
			}
			if h, ok := p.isAssign(p.tok); ok && !seenArg {
				c.Assigns = append(c.Assigns, p.parseAssign(h))
				continue
			}
			if len(c.Assigns) > 0 && !seenArg && p.reservedBehindAnAssignmentPrefix() {
				// One dialect keeps a reserved word's reading here where the
				// rest drop it and read the word as an ordinary command name.
				// A compound command has nowhere to stand behind an
				// assignment prefix, so the complaint is at the word itself
				// rather than at the token that closes what it opened. See
				// Dialect.ReservedWordStandsBehindAnAssignmentPrefix.
				p.failUnexpected("")
				return c
			}
			// The command word is where an alias is expanded, and nothing a
			// command may begin with moves it. An assignment prefix does
			// not: `alias al=echo; y=2 al HI` prints `HI` in both shells of
			// the panel that expand aliases in a script. parseCommand
			// expanded the word it dispatched on, and that word turned out
			// to be an assignment, so the expansion has to be made again
			// here — where the real command word finally stands. Left out,
			// the word resolved as written and the answer was `command not
			// found` (#1942).
			//
			// A *leading redirection* does not move it either, and that is
			// the same fact rather than a second one: `2> /dev/null al HI`
			// is `HI` in bash 5.3, zsh 5.9, ksh93 and dash alike. It went
			// wrong the same way and for one more reason — parseCommand
			// offered the redirection *operator* to the table before it
			// dispatched, and the real command word, read after the
			// redirection, was offered to nothing (#2922). Two conditions
			// rather than one branch each, because what they have in common
			// is the whole point: the command word is the first word of the
			// command that is neither a prefix nor a redirection.
			//
			// aliasSpliced == 0 keeps this to a word of the *input*: an alias
			// value's own second word is not expanded in turn, only the word
			// after a value ending in a blank is, which is the rule the
			// trailing-space branch below carries. The set is
			// parseCommand's, so a name already spent in this command is not
			// expanded again.
			if (len(c.Assigns) > 0 || len(c.Redirs) > 0) && !seenArg && p.aliasSpliced == 0 &&
				(p.Aliases != nil || p.SuffixAliases != nil) {
				p.aliasNextWord = false
				p.expandCommandWord(p.aliasDone)
				if h, ok := p.isAssign(p.tok); ok {
					// The value put an assignment where the command word was
					// — `alias al='x=1 echo'` — and it is a prefix like the
					// ones written out, so the word after it is the command
					// word in turn. Measured `HI`, status 0.
					c.Assigns = append(c.Assigns, p.parseAssign(h))
					continue
				}
				if len(c.Assigns) > 0 && p.aliasSpliced > 0 && !p.tok.IsQuoted() &&
					p.dialect.AliasedReservedWordStandsBehindAnAssignmentPrefix &&
					p.tok.Kind == TokWord && p.reservedInDialect(p.tok.Literal()) {
					// The value put a *reserved word* where the command word
					// was, and one dialect keeps the reading there where a
					// written-out word loses it. A compound command has
					// nowhere to stand behind an assignment prefix, so the
					// complaint is at the word itself rather than at the
					// token that closes what it opened — `alias g="{ :; }";
					// v=x g` quotes the `{` and not the `}`. See
					// Dialect.AliasedReservedWordStandsBehindAnAssignmentPrefix.
					p.failUnexpected("")
					return c
				}
				// Otherwise the word stands as the command word, expanded or
				// not, and the readings below are the ones it would have had.
				// Falling through rather than looping is what keeps a word
				// that names no alias from being offered here forever.
			}
			if a, consumed := p.declarationArray(c); consumed {
				if a != nil {
					c.Assigns = append(c.Assigns, a)
				}
				seenArg = true
				continue
			}
			if p.dialect.TimesIsReserved && seenArg && len(c.Args) == 1 &&
				c.Args[0].Literal() == "times" {
				// A reserved word takes no arguments, so the word after it
				// has nowhere to go.
				p.failUnexpected("")
				return c
			}
			if p.dialect.CloseBraceAlwaysReserved && p.atWord("}") {
				// Reserved even here, so it ends the command rather than
				// becoming an argument to it.
				return c
			}
			if p.aliasNextWord && p.aliasSpliced == 0 && p.Aliases != nil {
				// Still the table and not the suffix kind: this is the word
				// *after* a value ending in a blank, which is an argument
				// rather than a command word.
				// The expansion before this one ended in a space, so this
				// word is eligible too — the rule behind `alias sudo='sudo '`.
				// Only once the expansion's own tokens are spent: the space
				// makes the word *after* the value eligible, not the value's
				// own second word. Cleared first so a value that does not end
				// in a space stops the chain here.
				p.aliasNextWord = false
				p.expandAlias(map[string]bool{}, p.Aliases)
				continue
			}
			// A word list in front of `()` is a definition's name list where
			// the dialect gives one body several names — `clipcopy
			// clippaste() { … }`, and `echo hi () { … }`, which is why this
			// stands in the argument loop rather than in looksLikeFuncDef:
			// nothing about the first word announces it.
			//
			// *After* the declaration readings on purpose. `typeset -aU e1=()`
			// is an array declaration and not a definition of `typeset`,
			// `-aU` and `e1=`, and the utility is what decides that: measured
			// 2026-09-10, `a e1=() { echo X; }` defines `a` and `e1=` in the
			// shell that has the list, where the same word after `typeset -a`
			// is the array and the `()` after *it* is a parse error.
			if len(c.Assigns) == 0 && seenArg && p.dialect.FunctionMultipleNames &&
				p.argsCanBeFuncNames(c.Args) &&
				p.canBeFuncName(p.tok.Spans, p.tok.Literal()) &&
				p.peekIsFuncParens() {
				return p.parseFuncPosixNames(c)
			}
			seenArg = true
			if p.dialect.DoubleBracketIsACommand {
				switch {
				case p.atWord("[["):
					inDoubleBracket = true
				case p.atWord("]]"):
					inDoubleBracket = false
				}
			}
			// The token *after* this word stands where an argument may, and
			// p.word() is what reads it — so the lexer is told before the
			// call rather than after it. One dialect reads a `(` there as
			// part of a word; everywhere else the flag changes nothing.
			//
			// And whether it stands where a *declaration's* operand does,
			// which is the same question one token earlier: an operand of a
			// declaration utility may open an array literal and an ordinary
			// argument may not, and the word being read here is the only
			// thing that says which. Set once, on the command word, and it
			// lasts as long as the command — `typeset a=(1) b=(2)` is two
			// arrays. See Lexer.arrayLiteralCouldStandHere.
			if len(c.Args) == 0 && p.declarationWordWritten(p.tok.Spans) {
				p.lex.inDeclarationOperand = true
			}
			p.lex.inArgument = true
			c.Args = append(c.Args, p.word())
		case p.at(TokLeftParen) && len(c.Assigns) == 0 && len(c.Redirs) > 0 &&
			p.dialect.FunctionMultipleNames && p.argsCanBeFuncNames(c.Args) &&
			p.peekIsRightParen():
			// A redirection written *between* the names and the parentheses,
			// which is a definition too and whose redirection is the body's:
			// `a b >out () { echo "[$0]"; }` sends both calls to the file.
			// The `(` is recognized from the inside here, no word standing in
			// front of it to announce the reading.
			return p.parseFuncPosixNamesAtParen(c)
		case p.at(TokLeftParen) && (seenArg || len(c.Assigns) > 0 || len(c.Redirs) > 0):
			// A `(` in command position opens a subshell; one *after* a word
			// opens nothing. All four shells call it a syntax error, so this
			// is core rather than a dialect question. It is what makes
			// `function f() { … }` an error where the keyword is absent, and
			// `[[ ( -n x ) ]]` an error where `[[` is not a construct — both
			// of which used to run as ordinary commands with surprising
			// arguments.
			p.failUnexpected("")
			return c
		default:
			c.Stop = p.tok.Pos
			// Precommands count towards the command existing. `nocorrect`
			// with nothing after it is a command that runs nothing and
			// succeeds in the shell that has the word, and returning nil
			// here left the word consumed and the command gone — which
			// parseCommand reads as end of input.
			if len(c.Assigns) == 0 && len(c.Args) == 0 && len(c.Redirs) == 0 && len(c.Precommands) == 0 {
				return nil
			}
			return c
		}
	}
	c.Stop = p.tok.Pos
	return c
}

// reservedBehindAnAssignmentPrefix reports whether the current token is a word
// this dialect still reads as reserved although an assignment prefix stands in
// front of it.
//
// Quoting removes the reservation here as it does everywhere else: `v=x "if"`
// is a command called `if` in that column too.
func (p *Parser) reservedBehindAnAssignmentPrefix() bool {
	if !p.dialect.ReservedWordStandsBehindAnAssignmentPrefix ||
		p.tok.Kind != TokWord || p.tok.IsQuoted() {
		return false
	}
	if p.dialect.CloseBraceAlwaysReserved && p.atWord("}") {
		// A `}` there *ends* the command rather than standing behind the
		// prefix — `{ a+=( $p ) }` is a brace body in this column and the
		// assignment is the last statement in it. The stray `}` at a command
		// start is refused one level out, which is where it always was and
		// which is the same sentence: `v=x }` names the `}` either way.
		return false
	}
	return p.dialect.reservedAtACommandStart(p.tok.Literal())
}

// redirectionMayPrecedeThisCompound reports whether the compound command the
// current token opens may stand behind the redirections already read.
//
// The two columns that take one take it before different things: see
// [RedirectionBeforeACompoundPolicy] for the rows. `function` and `time` are
// left out and it is measured rather than forgotten — a redirection in front
// of either runs in that column, and neither is a compound command reached
// through this production: `time` is a pipeline and a definition holds its
// redirections on its body, so both would need the leading ones written back
// somewhere other than where they belong.
func (p *Parser) redirectionMayPrecedeThisCompound() bool {
	switch p.dialect.RedirectionBeforeACompound {
	case RedirectionMayPrecedeAParenthesizedCommand:
		return p.at(TokLeftParen) || p.at(TokArithCmd)
	case RedirectionMayPrecedeAnyCompoundCommand:
		if p.startsCompoundCommand() {
			return true
		}
		if p.tok.Kind != TokWord || p.tok.IsQuoted() {
			return false
		}
		switch p.tok.Literal() {
		case "repeat":
			return p.dialect.Repeat
		case "foreach":
			return p.dialect.Foreach
		}
	}
	return false
}

// reservedBehindARedirection reports whether the current token is a reserved
// word that cannot stand behind the redirections already read, in the column
// that keeps a reserved word's reading there.
//
// The compound commands are taken by redirectionMayPrecedeThisCompound above
// and never reach this, so what is left is the words that end a construct and
// the two that open one a redirection cannot precede. Measured 2026-09-18 on
// zsh 5.9.2, a leading `>/dev/null` in front of each:
//
//	then, do, done, fi, esac, else, elif, end   parse error naming the word
//	!, coproc                                   parse error naming the word
//	time                                        runs, and is timed
//	{ … }, if, while, for, case, ( ), (( )),
//	  select, repeat, foreach, function, [[     run
//
// bash, ksh93 and dash read every one of those as a command name behind the
// redirection, which is what this shell did for all of them (#3560).
func (p *Parser) reservedBehindARedirection() bool {
	if p.dialect.RedirectionBeforeACompound != RedirectionMayPrecedeAnyCompoundCommand ||
		p.tok.Kind != TokWord || p.tok.IsQuoted() {
		return false
	}
	switch p.tok.Literal() {
	case "time", "function":
		// Both run there, measured. Neither is taken by the branch above —
		// see redirectionMayPrecedeThisCompound for why — so they are named
		// here rather than refused.
		return false
	}
	return p.dialect.reservedAtACommandStart(p.tok.Literal())
}

// compoundBehindRedirections reads the compound command standing behind
// redirections already read, and gives it those redirections in front of any
// it carries of its own — the order they were written in, which is the order
// they are applied in.
func (p *Parser) compoundBehindRedirections(leading []*Redirect) Command {
	cmd := p.parseCommand()
	if cmd == nil {
		if p.err == nil {
			p.failUnexpected("")
		}
		return nil
	}
	if h, ok := cmd.(interface{ prependRedirs([]*Redirect) }); ok {
		h.prependRedirs(leading)
	}
	return cmd
}

// touchesPrevious reports whether the current token was written with nothing
// between it and the position the token in front of it ended at.
//
// Two sources, one question. A token of the input answers from its own
// offset. A token an alias expansion put in front of the input cannot: every
// spliced token carries the position of the *word it replaced*, so their
// offsets are all the same span and say nothing about the body's layout. For
// those the answer was taken while the body was lexed — see spliceAlias.
func (p *Parser) touchesPrevious(after Pos) bool {
	if p.aliasSpliced > 0 {
		return p.tokTouches
	}
	return p.tok.Pos.Offset == after.Offset
}

func (p *Parser) parseAssign(h assignHead) *Assign {
	a := &Assign{Name: h.name, Member: h.member, Start: p.tok.Pos, Append: h.append}
	if h.index != nil {
		a.Index = p.newWord(h.index, h.index[0].Pos, p.tok.End)
		a.IndexFlags = p.assignIndexFlags(h.index)
		a.IndexText = p.textBetween(h.from, h.to)
	} else if h.subscripted {
		// `a[]=v`: brackets were written and they hold nothing. The spans
		// came to none, so without this the assignment arrives looking
		// exactly like a bare `a=v` — which is a plausible wrong answer at
		// status 0, since that spelling writes element zero in one dialect
		// and replaces the whole name in another (#3949).
		a.EmptySubscript = true
	}
	if p.dialect.EmptyAssignSubscriptIsASyntaxError && emptySubscriptWritten(a.EmptySubscript, h.leading) {
		// One grammar refuses the brackets while the program is read, so
		// the refusal reaches text that never runs. Recorded rather than
		// returned on: the parser reads the rest of the word as it would
		// have, and the first failure recorded is the one Parse reports.
		p.failEmptyAssignSubscript(a.Start)
	}
	for _, link := range h.leading {
		// Each link is read exactly as the final subscript is, flag group
		// included: a chain is the same subscript twice and not a shape of
		// its own, so nothing here may know less about a link than about the
		// one it precedes.
		if link.spans == nil {
			// `a[][2]=v` — an empty link, which is the empty subscript every
			// other route already carries as a nil Word.
			a.Leading = append(a.Leading, LeadingIndex{Text: p.textBetween(link.from, link.to)})
			continue
		}
		a.Leading = append(a.Leading, LeadingIndex{
			Index: p.newWord(link.spans, link.spans[0].Pos, p.tok.End),
			Flags: p.assignIndexFlags(link.spans),
			Text:  p.textBetween(link.from, link.to),
		})
	}
	// The value is what is left of the span holding the `=`, plus every span
	// after it — which is why the head reports a position rather than a count.
	// `a[$i]=$v` has an expansion on each side of the `=`, and only the spans
	// say which is which.
	spans := spanRange(p.tok.Spans, h.span, h.off, len(p.tok.Spans)-1, toEnd)
	a.Stop = p.tok.End
	if len(spans) > 0 {
		a.Value = p.newWord(spans, p.tok.Pos, p.tok.End)
	}
	p.next()

	// `a=(1 2)` is an array, and the parenthesis has to be adjacent. With a
	// space it is not a subshell — measured, against the comment that used
	// to stand here: `a= (echo x)` is a syntax error in dash, bash and zsh,
	// and only ksh93 accepts it, reading it as the literal and leaving `a`
	// holding `echo` and `x`. The adjacency check still matters, because it
	// decides *which* error, and the non-adjacent form now falls through to
	// the paren-after-a-word rule in parseSimple. Specified in the array
	// assignment section of docs/spec/grammar/commands.md.
	if a.Value == nil && p.at(TokLeftParen) && p.touchesPrevious(a.Stop) {
		if !p.dialect.ArrayLiteral {
			p.failUnexpected("")
			return a
		}
		a.IsArray = true
		// Between the parentheses an element stands where an argument does,
		// so a `(` that begins one belongs to the *element* — which is the
		// flag the lexer already has for it, set here for the same reason
		// parseSimple sets it before each argument.
		//
		// Without it the assignment found its closing `)` by counting, and a
		// glob flag opens one that is part of a word: `files=( (#i)a )` was
		// `expected ) to close an array assignment` where zsh accepts it, and
		// so was every other parenthesised shape that stands at the *front*
		// of an element. The three that stand after a pattern —
		// `*(-.DN)`, `*~(*/*)` and `*.(zip|tgz)` — were already right,
		// because mid-word the lexer folds the group without being told
		// (#1149).
		//
		// Restored rather than cleared, because an assignment is read at
		// command position as well as after a word, and the token *after*
		// the array is not an argument either way — parseSimple's own defer
		// is what ends argument position for the command.
		saved := p.lex.inArgument
		p.lex.inArgument = true
		// An element of a declaration's array is still inside the
		// declaration, so the nested `q=(p r)` of a compound variable keeps
		// the array reading that argument position would otherwise take away.
		// Restored with the rest, so the flag reaches no further than these
		// parentheses.
		savedDecl := p.lex.inDeclarationOperand
		p.lex.inDeclarationOperand = true
		// And where an *element* stands, which is a second question the
		// lexer cannot ask for itself: an element opening with `[` runs to
		// its matching `]` through the blanks inside it, so
		// `m=( [two words]=2 )` is one element rather than two fields. See
		// [syntax.Dialect.SubscriptSpansSeparators].
		savedArray := p.lex.inArrayLiteral
		p.lex.inArrayLiteral = true
		p.next()
		p.skipArrayElementSeparators(false)
		// Two constructs share these parentheses in the one dialect that has
		// compound variables, and the first word decides which was written.
		// Ahead of the element loop because the readings take a `;`
		// differently and an element read here could not be given back — see
		// [Parser.opensACompoundVariableBody].
		// A subscript alongside the parentheses makes the *element* a value
		// of its own — `a[1]=(p q)` — and that value may be a compound as
		// readily as an array: `a[1]=(p=1 q=2)` is `typeset -a
		// a=([1]=(p=1;q=2))` on ksh93u+ 2012-08-01 and its members answer to
		// `${a[1].p}`, where `a[1]=(x y)` is the nested array
		// [interp.Semantics.SubscriptedArrayLiteral] already answers for.
		// So the reading is offered under a subscript too, and the same
		// word decides it there with nothing taken out — an empty pair of
		// parentheses included, which was measured rather than assumed: a
		// guard that kept `a[1]=()` an empty nested array left four rows
		// disagreeing with the reference and fixed none, because the
		// element's observable answers are the same either way
		// (`a=(x y z); a[1]=()` lists as `typeset -a a=(x () z)` and reads
		// back as a newline between two parens under both readings) while
		// `typeset -p 'a[1]'` is `typeset -C a[1]=()` there and
		// `a[1]=(); a[1].p=3` gives `typeset -a a=([1]=(p=3))`, neither of
		// which the guarded reading could reach.
		//
		// The declaration's letters, consumed here and not carried down:
		// a body of its own is read by rules of its own, so the nested
		// `q=(p r)` in `typeset -C c=(a=1 q=(p r))` is an array member.
		reading := p.literalReading
		p.literalReading = literalWordDecides
		if p.opensACompoundVariableBody(reading) {
			a.Members = p.compoundVariableBody()
			if p.err == nil && !p.at(TokRightParen) {
				p.lex.inArgument = saved
				p.lex.inArrayLiteral = savedArray
				p.lex.inDeclarationOperand = savedDecl
				p.failUnexpected(")")
				return a
			}
			p.lex.inArgument = saved
			p.lex.inArrayLiteral = savedArray
			p.lex.inDeclarationOperand = savedDecl
			if p.err != nil {
				return a
			}
			a.Stop = p.tok.End
			p.next()
			return a
		}
		// The shape one dialect settles on its first element, and the token
		// it settled it on. Asked here rather than after the loop because
		// both consequences are ahead of the elements that follow: the
		// lexer's spanning has to be turned off before the *next* token is
		// read, and a bare element the shape has already refused has to be
		// named where it stands. See
		// [Dialect.ArrayLiteralShapeFollowsTheFirstElement].
		shaped := p.dialect.ArrayLiteralShapeFollowsTheFirstElement
		subscripted := false
		// Whether an element opening with `(` may be a **compound variable's
		// body** rather than the nested array — the name's own literal alone,
		// for the reason [Parser.nestedArrayLiteral] states: the members of
		// such an element are ordinary names spelled with the element's own
		// subscript in front, and the element is only known where the literal
		// belongs to the name. `a[1]=( (p=1) )` writes into `a[1]` and its
		// nested element's spelling would be `a[1][0]`, which is the deeper
		// row recorded there.
		elementCompound := a.Index == nil && len(a.Leading) == 0
		for p.err == nil {
			if p.dialect.NestedArrayLiteral && p.at(TokLeftParen) {
				if shaped && subscripted {
					// The literal took its keyed shape from a first element
					// that named a subscript, and there a parenthesis is a
					// **value** and nothing else: `a=( [0]=(1 2) )` is one
					// element under a key, where the two read apart would be
					// a key holding nothing and a nested element beside it.
					// Anywhere else in that shape it is refused, which is
					// measured — see subscriptedHeadAwaitingALiteral.
					head := subscriptedHeadAwaitingALiteral(a.Elems)
					if head == nil {
						p.lex.inArgument = saved
						p.lex.inArrayLiteral = savedArray
						p.lex.inDeclarationOperand = savedDecl
						p.failUnexpected("")
						return a
					}
					head.Nested = p.nestedArrayLiteral(elementCompound)
					if p.err != nil {
						break
					}
					if p.skipArrayElementSeparators(true) {
						break
					}
					continue
				}
				// A literal standing where an element goes, which is that
				// dialect's multi-dimensional array. Read before the word
				// branch because the parenthesis is a token here and not the
				// front of a word: `a=( (1 2)x )` is two elements, the
				// literal and `x`, so the close paren ends the element
				// wherever it falls. See [Dialect.NestedArrayLiteral].
				a.Elems = append(a.Elems, &ArrayElem{Nested: p.nestedArrayLiteral(elementCompound)})
				if p.err != nil {
					break
				}
				if p.skipArrayElementSeparators(true) {
					break
				}
				continue
			}
			if p.tok.Kind != TokWord {
				break
			}
			// Saved before p.word(), which reads the token after this one on
			// its way out: by the time the element is a word the parser has
			// moved past it, and a refusal has to quote what was written
			// here and blame the line it was written on.
			at := p.tok
			if shaped {
				if len(a.Elems) == 0 {
					subscripted = subscriptedElementSpans(at.Spans)
					if !subscripted {
						// A bare first element makes every bracket in this
						// literal text, and text is lexed by the ordinary
						// rules — `a=(p [1 2]=A)` is three words from here
						// on. Restored with the rest at the end of the
						// literal, so a nested one is unaffected.
						p.lex.inArrayLiteral = false
					}
				} else if subscripted && !subscriptedElementSpans(at.Spans) {
					p.lex.inArgument = saved
					p.lex.inArrayLiteral = savedArray
					p.lex.inDeclarationOperand = savedDecl
					p.failUnexpectedAt(at, "", false)
					return a
				}
			}
			el := p.word()
			// An element is not a word a command takes, and one dialect
			// refuses a process substitution there while reading (#930).
			if p.refuseProcSubstOutOfPlace(el) {
				break
			}
			a.Elems = append(a.Elems, WordElem(el))
			if p.skipArrayElementSeparators(true) {
				break
			}
		}
		if !p.at(TokRightParen) && p.giveUpOnTheArray(saved) {
			p.lex.inArrayLiteral = savedArray
			p.lex.inDeclarationOperand = savedDecl
			return a
		}
		p.lex.inArgument = saved
		p.lex.inArrayLiteral = savedArray
		p.lex.inDeclarationOperand = savedDecl
		if !p.at(TokRightParen) {
			// Named rather than described: bash answers
			// `syntax error near unexpected token `;'` and the `)` this used
			// to ask for is written right there, so a message demanding one
			// points at the wrong character (#1162).
			p.failUnexpected(")")
			return a
		}
		a.Stop = p.tok.End
		p.next()
	}
	return a
}

// subscriptedHeadAwaitingALiteral is the element a nested literal belongs to
// rather than standing beside: the one just read, where it named a subscript
// and was given no value.
//
// Blanks between the two are allowed and a value is not — measured on ksh93u+
// 2012-08-01, 2026-09-19:
//
//	a=( [0]= (1 2) )        typeset -A a=([0]=(1 2) )
//	a=( [0]+= (1 2) )       typeset -A a=([0]=(1 2) )
//	a=( [0]= [1]= (1 2) )   typeset -A a=([0]='' [1]=(1 2) )
//	a=( [0]=x (1 2) )       `(' unexpected
//	a=( [0]= (1 2) (3 4) )  `(' unexpected
//
// so it is the element just read and no further back, and the last two rows
// are what the caller refuses when this answers nil.
func subscriptedHeadAwaitingALiteral(elems []*ArrayElem) *ArrayElem {
	if len(elems) == 0 {
		return nil
	}
	last := elems[len(elems)-1]
	if last.Word == nil || last.Nested != nil {
		return nil
	}
	_, value, _, ok := ElementSubscript(last.Word)
	if !ok || (value != nil && value.Literal() != "") {
		return nil
	}
	return last
}

// nestedArrayLiteral reads a literal standing where an element goes —
// `a=( (1 2) (3 4) )`, and to any depth: `a=( ( (1 2) (3) ) (4) )` is three
// levels and `${a[0][0][1]}` reads `2` back out of it.
//
// The lexer state the outer literal set — an element stands where an argument
// does, subscripts span their blanks, and a declaration operand is still being
// read — is already what a nested one wants, so nothing is saved or restored
// here: the parentheses this reads are inside the ones that set it.
//
// **The compound-variable reading is offered where the element is one of the
// name's own** — `a=(x (p=1 q=2))` and `a=([1]=(p=1 q=2))` — and not deeper,
// which is the whole of what offersACompound carries. The members of a
// compound held in an element are ordinary names spelled with the element's
// own subscript in front, `a[1].p`, so a reading has to know which element it
// is being stored in; that is known for the name's own literal and is not
// known for a literal standing inside another one. The deeper row is measured
// and recorded rather than half-built: `a=( ( (p=1) ) )` is
// `typeset -a a=(((p=1)) )` on ksh93u+ 2012-08-01 with `${a[0][0].p}` of `1`,
// where this reads the inner parentheses as the nested array they were before
// (#3864).
func (p *Parser) nestedArrayLiteral(offersACompound bool) *Assign {
	n := &Assign{IsArray: true, Start: p.tok.Pos}
	p.next()
	p.skipArrayElementSeparators(false)
	if offersACompound && p.opensACompoundVariableBody(literalWordDecides) {
		// The same two constructs the parentheses of a bare name hold, told
		// apart by the same first word — see
		// [Parser.opensACompoundVariableBody], which is called with no
		// declaration letters because a body of its own is read by rules of
		// its own. An empty pair is the compound here too, measured:
		// `a=( () ); typeset -p 'a[0]'` is `typeset -C a1[0]=()` on ksh93u+
		// and `a=( () ); a[0].p=3` is `typeset -a a=((p=3))`, neither of
		// which the array reading can reach.
		n.Members = p.compoundVariableBody()
		if p.err == nil && !p.at(TokRightParen) {
			p.failUnexpected(")")
			return n
		}
		if p.err != nil {
			return n
		}
		n.Stop = p.tok.End
		p.next()
		return n
	}
	for p.err == nil && !p.at(TokRightParen) {
		switch {
		case p.at(TokLeftParen):
			n.Elems = append(n.Elems, &ArrayElem{Nested: p.nestedArrayLiteral(false)})
		case p.tok.Kind == TokWord:
			el := p.word()
			// An element is not a word a command takes, the same refusal the
			// outer literal makes of the same shape (#930).
			if p.refuseProcSubstOutOfPlace(el) {
				return n
			}
			n.Elems = append(n.Elems, WordElem(el))
		default:
			p.failUnexpected(")")
			return n
		}
		if p.err != nil || p.skipArrayElementSeparators(true) {
			return n
		}
	}
	if p.err != nil {
		return n
	}
	if !p.at(TokRightParen) {
		p.failUnexpected(")")
		return n
	}
	n.Stop = p.tok.End
	p.next()
	return n
}

// giveUpOnTheArray is the recovery for a syntax error inside a compound
// assignment's parentheses, and reports whether it took it.
//
// The shell that has arrays reads `a=( … )` as **one word**: the parentheses
// belong to the assignment and what stands between them is a list of its own,
// so a `&`, a `|`, a `;;` or a `>` in there is a complaint about that list and
// not about the file. Measured 2026-09-12 from a script file, bash 5.3.15 and
// bash 3.2.57, with `echo one` before and `echo two` after:
//
//	a=(p & q)              syntax error near unexpected token `&', then `two`
//	a=( [0]=p [1]=> )      the same with `>`
//	declare -a d=(p & q)   the same, so an operand form is this too
//	a+=(p & q)             and the appending one
//	echo $(if)             *fatal*, status 2 — a substitution is not this
//
// The line the error fell on is thrown away unrun, which is visible: with
// `f() { a=(p & q); }` the complaint is made and `f` is not defined. So the
// recovery is "read past this construct so the next line can be reached", and
// never "read the rest of the line as though it had parsed".
//
// The input running out between the parentheses is the same refusal, and it
// is what the *status* says. There is no next line to read on to — that is
// what EOF means — so the recovery is only how much the failure ended, and
// bash ends the line rather than the file there too. Measured 2026-09-12,
// bash 5.3.15, from a script file:
//
//	a=( x                    status 1     input ran out inside the parens
//	a=( $(                   status 1     and inside a construct inside them
//	a=( "x                    status 1     and inside a quote
//	echo $(                  status 2     the same message outside them
//	echo "x                  status 2     the same
//	set -e ⏎ a=( x           status 2     the refused *line*'s own rule
//	a=( $(if; then :; fi) )  status 2     a *token* refused deeper in is not
//
// Row one against row four is the whole of it: one message, two statuses, and
// the parens are the only difference. Row six is what says this is the
// refused line and not a status of its own — `set -e` turns it back into 2,
// exactly as it does for the token rows above, which a number attached to the
// failure could not have done. Row seven is the boundary the other way: it is
// the input running out that this takes, not everything that can go wrong
// inside the parens (#2404).
//
// inArgument is put back before anything else, because the skip reads the
// remaining elements the way the loop above read them, and what follows the
// array is not an argument either way.
func (p *Parser) giveUpOnTheArray(savedInArgument bool) bool {
	if !p.dialect.CompoundAssignmentErrorGivesUpTheLine {
		return false
	}
	if p.at(TokEOF) || p.err != nil {
		if !p.arrayLiteralInputRanOut() {
			// Something other than the end of the input, raised deeper in
			// than this production: the file's, as it always was.
			return false
		}
		if p.err == nil {
			// Nothing has recorded it yet, so the caller's own report is
			// made here instead — same token, same wording, same `)` asked
			// for — and only where it goes differs.
			p.failUnexpected(")")
		}
		p.refused, p.err = p.err, nil
		// The lexer's copy as well, where the failure was its: a parser that
		// re-reads at EOF adopts it again on the next token and the refusal
		// would become the file's after all. There is nothing left to read
		// but the end of the input, which is why forgetting it here is safe.
		p.lex.forgetErr()
		p.lex.inArgument = savedInArgument
		return true
	}
	p.failUnexpected("")
	// Moved off the parser before the skip, because a parser holding an error
	// reads nothing more and the whole point here is to read to the `)`.
	p.refused, p.err = p.err, nil
	// Past the parenthesis that closes the array, counting the ones the
	// elements opened so that `a=(p & (q) )` is not called closed by the
	// inner one. A token that holds a `(` inside a word — a substitution, a
	// quantified group — is one token here and never reaches the count.
	depth := 1
	for depth > 0 && !p.at(TokEOF) && p.err == nil {
		switch p.tok.Kind {
		case TokLeftParen:
			depth++
		case TokRightParen:
			depth--
		}
		p.next()
	}
	p.lex.inArgument = savedInArgument
	return true
}

// arrayLiteralInputRanOut reports whether what stopped the array literal was
// the input running out rather than a token the grammar did not want.
//
// Two shapes of the one fact. With no error recorded it is the element loop
// having reached EOF — `a=( x` — and with one it is a construct or a quote
// inside an element that the end of the input closed: `a=( $(` records the
// parser's unterminated, `a=( "x` the lexer's unmatched. A token refused
// deeper in — `a=( $(if; then :; fi) )` — is neither, and is the file's.
func (p *Parser) arrayLiteralInputRanOut() bool {
	if p.err == nil {
		return p.at(TokEOF)
	}
	var e *Error
	if !errors.As(p.err, &e) {
		return false
	}
	return e.Kind == ErrUnterminated || e.Kind == ErrUnmatched
}

// declarationArray reads `name=(x y)` written as an operand of a utility that
// takes assignments, and reports nil when this is not one.
//
// Only the array form. A scalar `local a=1` is an ordinary word and stays one:
// it expands by rules of its own that expandAssignArg already implements, and
// routing it here would change a path nothing asked to change. The array form
// has no such path — it was a syntax error — which is the whole of what this
// adds.
//
// The test for it is that the word ends at its `=`, because that is the only
// shape a `(` can follow: `a=(x y)` reaches the parser as the word `a=` and
// then a parenthesis, where `a=1` is one word. If no parenthesis turns out to
// be there the word is handed back unchanged, so `local a=` is still an
// ordinary operand.
// consumed is separate from the assignment because the word is read either
// way: `local a=` is an ordinary operand and is put back as one, and the
// caller must not read it a second time.
func (p *Parser) declarationArray(c *SimpleCmd) (a *Assign, consumed bool) {
	if len(c.Args) == 0 || len(p.dialect.DeclarationUtilities) == 0 {
		return nil, false
	}
	if !p.declarationWordWritten(c.Args[0].Spans) {
		return nil, false
	}
	h, ok := p.isAssign(p.tok)
	if !ok || !strings.HasSuffix(p.tok.Text, "=") {
		// The suffix test is what keeps a scalar off this path rather than
		// what makes it come out right — the fallback below would hand
		// `local a=1` back unchanged anyway. It is here so the common case
		// does not take a round trip through parseAssign to arrive where it
		// started.
		return nil, false
	}
	tok := p.tok
	// What the command's own letters have already settled about the
	// parentheses, where the dialect has a construct they can settle. Set
	// around the one call because parseAssign consumes it: see
	// [compoundLiteralReading].
	saved := p.literalReading
	p.literalReading = declarationLiteralReading(c.Args)
	a = p.parseAssign(h)
	p.literalReading = saved
	if a != nil && a.IsArray {
		a.Operand = true
		return a, true
	}
	// Not an array after all, so put the word back the way it came — read
	// once, by this, and not again by the caller.
	c.Args = append(c.Args, p.newWord(tok.Spans, tok.Pos, tok.End))
	return nil, true
}

// declarationWordWritten reports whether a command word made of these spans
// names a declaration utility the way [Dialect.DeclarationArrayFromTheCommandWord]
// requires, so that a `name=( … )` operand behind it is an array literal.
//
// Spans rather than a Word, because the question is asked of a token the
// parser has not turned into one yet — the lexer has to be told what position
// the *next* word stands in before it reads it. `Word.Literal()` is what this
// replaced and is exactly what it must not be: it hands back the text with the
// quotes taken off, so `'typeset'` and `typeset` are one word to it, and two
// of the three columns that have the construct refuse the first (#3351).
func (p *Parser) declarationWordWritten(spans []Span) bool {
	if len(p.dialect.DeclarationUtilities) == 0 {
		return false
	}
	var b strings.Builder
	for _, s := range spans {
		// An expansion anywhere in the word takes the reading away under
		// both readings: `cmd=typeset; $cmd a=(x y)` is refused in bash and
		// in ksh93 and is a glob qualifier in zsh.
		if s.Kind != Literal {
			return false
		}
		if p.dialect.DeclarationArrayFromTheCommandWord == DeclarationArrayFromAnUnquotedLiteralWord &&
			s.Quoting != Unquoted {
			return false
		}
		b.WriteString(s.Value)
	}
	return p.dialect.DeclarationUtilities[b.String()]
}

// looksLikeFuncDef reports whether the current word begins `name()`.
func (p *Parser) looksLikeFuncDef() bool {
	if p.dialect.FunctionNameIsAnyWord {
		return p.anyWordFuncDef()
	}
	if p.dialect.FunctionNameIsAnyBareWord {
		return p.bareWordFuncDef()
	}
	// A quoted name is not a definition in most dialects, and quoting is not
	// an expansion: `'q'() { :; }` is refused here as it was before the flag.
	// Two dialects read one, by different routes: the one whose names are any
	// word at all, in the branch above, and the one that reads the name as
	// *source text* and refuses it when the definition runs — where the
	// quotes are in the name rather than around it, so a quoted word is a
	// definition and the name it declares is not one.
	if p.tok.IsQuoted() && !p.dialect.FunctionNameIsSourceText {
		return false
	}
	// One unquoted literal span is the ordinary name. Where the dialect
	// expands a name, several spans are allowed and an expansion among them
	// is the point — and where it reads the name as source text, several
	// spans are allowed too so long as none of them is an expansion:
	// `a\ b()` is three spans there and is the definition bash reads before
	// refusing the name.
	if !p.dialect.FunctionNameExpands && !p.tokenIsPlainText(p.tok) {
		// A word carrying an expansion is still a definition where the name
		// is read as source text, and the refusal is the *definition's*
		// rather than the parser's: bash 5.3.20 and bash 3.2.57 answer
		// `_p_${w}() { :; }` with `` `_p_${w}': not a valid identifier ``,
		// give it 1 and carry on, where this parser called the `(` an
		// unexpected token and gave up the rest of the input. Measured
		// 2026-09-16 over `x$y`, `x${y}`, `x$(y)` and `_p_${w}`, with and
		// without a blank before the parentheses. #1296 fixed the same word
		// after the `function` keyword and this spelling kept the refusal —
		// which is the expensive half, a parse error costing every line of a
		// file after it rather than one definition.
		if !p.dialect.FunctionNameIsSourceText {
			return false
		}
		// An array assignment is a parenthesis after a word too, and its
		// subscript is where an expansion ordinarily goes: `a[$i]=()` empties
		// an element and defines nothing. The same lexical test the expanding
		// dialect makes below.
		if _, isAssign := p.isAssign(p.tok); isAssign {
			return false
		}
		return p.peekIsFuncParens()
	}
	if p.dialect.FuncDefAtParen {
		// The paren is the whole announcement here, and the word before it is
		// not checked for being a name: `[[ ( -n x ) ]]` is a definition of a
		// function called `[[` to a shell without `[[`, which is how the
		// dialect that does this reaches the diagnosis it reaches.
		//
		// `=` is still excluded, and for the reason below: an assignment of an
		// array is a parenthesis after a word too.
		return !strings.Contains(p.tok.Literal(), "=") && p.peekIsLeftParen()
	}
	// A function name is a name — plus the punctuation the dialect allows —
	// so it cannot contain `=`. Without this, `a=()` — an empty array — was
	// read as a definition of a function called `a=`, because a parenthesis
	// pair follows either way.
	if p.dialect.FunctionNameExpands && tokenHoldsAnExpansion(p.tok) {
		// The name is a word here, so whether its *text* is a name cannot be
		// known until it is expanded. The parentheses are what say this is a
		// definition at all — except where the word is an assignment's name
		// half, because an array assignment is a parenthesis after a word
		// too and the subscript is where a loop ordinarily puts an
		// expansion: `a[$i]=()` empties an element and defines nothing.
		//
		// The reading is lexical, which is what tells the two apart. In
		// `a[$i]=` the `=` follows a name and a subscript written as literal
		// text, so the word is an assignment; in `_p_${w}=` the `=` follows
		// an expansion, so it is not one, and the function it defines is
		// named `_p_foo=` — measured on zsh 5.9.2, which lists it under
		// `${(k)functions}` with no variable `_p_foo` anywhere.
		if _, isAssign := p.isAssign(p.tok); isAssign {
			return false
		}
		return p.peekIsFuncParens()
	}
	if !isFuncName(p.tok.Literal(), p.dialect.FunctionNamePunctuation) {
		return false
	}
	return p.peekIsFuncParens()
}

// tokenIsPlainText reports whether t holds text and nothing the shell would
// expand, which is what a name may be made of where a name is not a word.
//
// One span is the ordinary reading and the only one most dialects allow: a
// second span means quoting, and quoting is what those dialects refuse before
// this is reached. The dialect that reads a name as source text keeps the
// quoting *in* the name, so it has several spans to walk.
func (p *Parser) tokenIsPlainText(t Token) bool {
	if !p.dialect.FunctionNameIsSourceText {
		return len(t.Spans) == 1 && t.Spans[0].Kind == Literal
	}
	if len(t.Spans) == 0 {
		return false
	}
	for _, sp := range t.Spans {
		if sp.Kind != Literal {
			return false
		}
	}
	return true
}

// anyWordFuncDef is looksLikeFuncDef where the word before the parentheses is
// the name whatever is in it — see [Dialect.FunctionNameIsAnyWord].
//
// Nothing about the *text* decides this, so the parentheses are the whole
// announcement and there is no name test to fail. The two exclusions are the
// readings that are not definitions at all rather than names being refused:
//
//	a=()        an empty array, and the `=` has to be bare to make one —
//	            `'a=b'()` and `a\=b()` are definitions of `a=b` there. The
//	            same lexical test [Dialect.FunctionNameExpands] makes.
//	a*b()       matched against the filesystem in the shell this models, so
//	            it defines nothing there and must not define anything here.
//	            Quoted the characters are ordinary text and are taken.
func (p *Parser) anyWordFuncDef() bool {
	if _, isAssign := p.isAssign(p.tok); isAssign {
		return false
	}
	if holdsBarePatternCharacter(p.tok) {
		return false
	}
	if !p.dialect.FunctionNameExpands && tokenHoldsAnExpansion(p.tok) {
		// A name that is not text until the shell runs is a *different*
		// flag's question, and without that flag there is nowhere to keep the
		// word: [FuncDecl.Name] would take the token's literal spelling, so
		// `_p_${w}() { … }` would define `_p_w` — a perfectly good name for a
		// perfectly wrong function, at status 0. The refusal that stood here
		// before this flag existed stands still.
		return false
	}
	return p.peekIsFuncParens()
}

// bareWordFuncDef is looksLikeFuncDef for the dialect that reads the word
// whatever it is and defines nothing unless it was written bare — see
// [Dialect.FunctionNameIsAnyBareWord].
//
// There is no name test at all here, so the parentheses are the whole
// announcement, as they are for [Parser.anyWordFuncDef]. It parts from that
// one in the exclusion it does *not* make: a bare `*`, `?` or `[` in the word
// is refused there because that shell matches such a word against the
// filesystem, and this one defines `a*b` and calls it, so refusing the word
// would lose a definition the shell being modeled makes.
//
// The assignment exclusion is the same and is lexical for the same reason:
// `a=()` is a syntax error in this shell, and `'a=b'()` is a definition whose
// name was not written bare — read, and binding nothing.
func (p *Parser) bareWordFuncDef() bool {
	if _, isAssign := p.isAssign(p.tok); isAssign {
		return false
	}
	if p.dialect.FuncDefAtParen {
		return p.peekIsLeftParen()
	}
	return p.peekIsFuncParens()
}

// peekIsLeftParen and peekIsRightParen are peekIsFuncParens' one-parenthesis
// halves, asked the same way and for the same reason. The right-hand one is
// asked where the `(` is already the current token, which is the other side of
// the same seam: a body ending in `(` leaves the `)` pending behind it.
func (p *Parser) peekIsLeftParen() bool {
	if len(p.pending) > 0 {
		return p.pending[0].Kind == TokLeftParen
	}
	return p.lex.peekIsLeftParen()
}

func (p *Parser) peekIsRightParen() bool {
	if len(p.pending) > 0 {
		return p.pending[0].Kind == TokRightParen
	}
	return p.lex.peekIsRightParen()
}

// peekIsFuncParens is the `()` lookahead of a function definition, asked of
// whatever this parser reads next — which is not always the lexer.
//
// An alias expansion puts its body's tokens in front of the lexer rather than
// splicing its text into the input (see alias.go), so after `alias fn='f() {'`
// the parenthesis pair is in p.pending while the lexer stands *past* the alias
// word, on input that has nothing to do with the definition. Asking the lexer
// there reads the wrong text and the definition is refused — measured on
// `main`, where every other shell in the panel defines the function.
// peekIsFuncParensAdjacent is peekIsFuncParens with the blank in front of the
// `(` counting. A pending token carries no blank between it and the one
// before it, so an alias's own body is always adjacent — which is the same
// answer the lexer gives for text written that way.
func (p *Parser) peekIsFuncParensAdjacent() bool {
	if len(p.pending) > 0 {
		return p.peekIsFuncParens()
	}
	return p.lex.peekIsFuncParensAdjacent()
}

func (p *Parser) peekIsFuncParens() bool {
	switch len(p.pending) {
	case 0:
		return p.lex.peekIsFuncParens()
	case 1:
		// The body ended between the two, so the `)` is the lexer's.
		return p.pending[0].Kind == TokLeftParen && p.lex.peekIsRightParen()
	default:
		return p.pending[0].Kind == TokLeftParen && p.pending[1].Kind == TokRightParen
	}
}

func (p *Parser) parseFuncPosix() Command {
	fn := &FuncDecl{Name: p.tok.Literal(), Start: p.tok.Pos}
	switch text := p.funcNameText(p.tok); {
	case p.dialect.FunctionNameIsAnyBareWord && !tokenIsWrittenBare(p.tok):
		// The dialect where the *spelling* decides it, given a word that was
		// not written bare: the declaration is read whole and binds nothing
		// when it runs. There is no name, so none is kept — a declaration
		// carrying one is a declaration the interpreter could define.
		fn.Name, fn.RefusedName = "", p.refusedFuncName(p.tok)
	case text != fn.Name && !isFuncName(text, p.dialect.FunctionNamePunctuation):
		// The dialect that reads the name as source text, given a word whose
		// text is not one: the declaration is read whole and the word is
		// kept as it was written, which is what the complaint quotes when
		// the definition runs. The keyword form takes the same branch in
		// parseFuncKeyword; only these two spellings have a name at all.
		fn.RefusedName = text
	}
	if p.dialect.FunctionNameExpands && tokenHoldsAnExpansion(p.tok) {
		// A name that is not text until the shell runs, kept whole. p.word()
		// consumes it, which is the p.next() the plain path takes.
		fn.NameWord = p.word()
	} else {
		p.next()
	}
	return p.parseFuncParensAndBody(fn)
}

// refuseNameKeptByTheDialect settles a definition whose name this dialect
// keeps for itself — the special builtins, in the two columns that reserve
// them — and reports whether the parse is over.
//
// The set is [Dialect.FunctionNamesRefused] and the *stage* is
// [Dialect.FunctionNameCheckedWhenTheDefinitionRuns], which is the field
// already answering that question for a name that is not a name. Where the
// dialect checks while reading, this is a syntax error and nothing in the
// input runs; where it checks at the definition, the declaration is read
// whole and the word goes on [FuncDecl.RefusedName], which
// interp.Runner.funcDecl turns into the complaint. Both measured, and they
// are what the two shells do rather than two spellings of one answer: the
// reading refusal fires on a definition in a branch nothing takes, and the
// running one does not.
//
// A name that is not text until the shell runs is nobody's: the set holds
// words, and `${v}() { :; }` is a question about the expansion's product
// rather than about the word, which no column in the panel puts this way.
func (p *Parser) refuseNameKeptByTheDialect(fn *FuncDecl) bool {
	if fn.NameWord != nil || !p.dialect.FunctionNamesRefused[fn.Name] {
		return false
	}
	if p.dialect.FunctionNameCheckedWhenTheDefinitionRuns {
		fn.RefusedName = fn.Name
		return false
	}
	p.fail("Bad function name")
	return true
}

// parseFuncParensAndBody reads `() compound` with the name or names already
// on the declaration and the parser standing at the `(`.
//
// Split out because the names reach it three ways — one word, a word list,
// and a word list with a redirection between it and the parentheses — and
// everything after the `(` is the same production in all three.
func (p *Parser) parseFuncParensAndBody(fn *FuncDecl) Command {
	if p.aliasFuncRefused {
		// The name is one the alias table holds, in the dialect that will
		// not define a function under one. The remark has already been
		// written where the word stands; what is left is the parse failure,
		// located at the parentheses, which is where that shell puts it.
		// See Parser.aliasAtAFunctionName.
		p.aliasFuncRefused = false
		p.failUnexpected("")
		return fn
	}
	p.next() // (
	if !p.at(TokRightParen) {
		p.failUnexpectedOperand(")")
		return fn
	}
	p.next()
	// The dialect that commits at the paren without allowing punctuation
	// checks the name once the parens close: dash's `f-g() { :; }` is
	// `Bad function name`, said only after `[[ ( -n x ) ]]`-shaped text has
	// already reached its own unexpected-word diagnosis above.
	if p.dialect.FuncDefAtParen && !p.dialect.FunctionNamePunctuation &&
		fn.NameWord == nil && !isName(fn.Name) {
		p.fail("Bad function name")
		return fn
	}
	if p.refuseNameKeptByTheDialect(fn) {
		return fn
	}
	if p.dialect.EmptyParensAreOneToken {
		// The parens are empty by construction here — anything between them
		// was refused above — so joining them is a matter of saying so. It
		// stands until the next token is read, which is exactly as long as it
		// is the last thing seen.
		p.lastText = "()"
	}
	// Whether the body was allowed to start on a later line, which one dialect
	// reports differently from a body that never started at all.
	atParens := p.tok.Pos.Line
	p.skipNewlines()
	sameLine := p.tok.Pos.Line == atParens
	// The body's first token, kept before the body is read. Both ways of
	// refusing a body below name it, and neither can be decided until the
	// parser has moved on: `f() >out` is only known not to be compound once
	// the redirection has been parsed as a command of its own.
	body := p.tok
	p.funcBody = true
	if fn.Body = p.parseCommand(); fn.Body == nil {
		hadError := p.err != nil
		p.failUnexpectedAt(body, "", false)
		if se, ok := p.err.(*Error); ok && !hadError && sameLine {
			se.FuncBody = true
		}
		return fn
	}
	if p.dialect.FuncBodyMustBeCompound {
		if _, isSimple := fn.Body.(*SimpleCmd); isSimple {
			p.failUnexpectedAt(body, "", false)
			return fn
		}
	}
	if p.dialect.FuncBodyTakesNoRedirection {
		// The operator rather than the body's first token: this dialect takes
		// the command and objects to what it redirects, so `f() echo hi >out`
		// is refused at the `>` with the `echo` already accepted.
		if simple, isSimple := fn.Body.(*SimpleCmd); isSimple && len(simple.Redirs) > 0 {
			r := simple.Redirs[0]
			p.failRedirectAt(r.OpPos, r.Op)
		}
	}
	return fn
}

// parseFuncPosixNames is parseFuncPosix where the words already read are the
// definition's earlier names — `clipcopy clippaste() { … }`, and `a b () { …
// }` with a blank in front of the parentheses.
//
// The words arrive as a simple command's arguments because that is what they
// are until the parentheses appear, so the declaration is read by the
// ordinary path and the arguments are folded onto the front of its name list
// afterwards. Each is read the way a name after the keyword is read, which is
// what keeps the two spellings from drifting: a word holding an expansion
// stays a word, and a name is its literal text everywhere else.
//
// A redirection written *between* the names and the parentheses is a
// definition there too — `a b >out () { echo "$0"; }` sends both calls to the
// file — and is not read here, because the parentheses then follow the
// redirection's target rather than a name and there is no word in hand when
// the `(` arrives. [Parser.parseFuncPosixNamesAtParen] is that route.
func (p *Parser) parseFuncPosixNames(c *SimpleCmd) Command {
	cmd := p.parseFuncPosix()
	fn, ok := cmd.(*FuncDecl)
	if !ok || p.err != nil {
		return cmd
	}
	last := FuncName{Name: fn.Name, Word: fn.NameWord}
	names := make([]FuncName, 0, len(c.Args)+len(fn.AlsoNamed))
	for _, w := range c.Args[1:] {
		names = append(names, p.funcNameFromWord(w))
	}
	first := p.funcNameFromWord(c.Args[0])
	fn.Name, fn.NameWord = first.Name, first.Word
	fn.AlsoNamed = append(names, append(fn.AlsoNamed, last)...)
	fn.Start = c.Start
	// A redirection read before the names is the body's too, and it arrives
	// the same way: `>out a b () { … }` writes the file in the shell that has
	// the list. Attached once the body exists, for the reason
	// [Parser.parseFuncPosixNamesAtParen] gives.
	for _, r := range c.Redirs {
		fn.addRedir(r)
	}
	return fn
}

// parseFuncPosixNamesAtParen is parseFuncPosixNames where a *redirection*
// stands between the names and the parentheses, so the parser reaches the `(`
// with no word in front of it and the whole name list already read.
//
// The redirection is the **body's**, which is where a definition's written
// one goes everywhere else — `f() { :; } 2>&1` reads it that way — so it is
// attached once the body exists rather than kept on the declaration. Measured
// 2026-09-12 on zsh 5.9.2: `a b >out () { echo "[$0]"; }; a; b; cat out`
// prints nothing at the terminal and leaves `[b]` in the file, so both names
// share one redirected body. Several are taken and so is a leading one:
// `a b >o1 >o2 ()` writes both files and `>o1 a b ()` writes the one.
//
// See #1838; the formatter is the other half, and `printer.funcDecl` has it.
func (p *Parser) parseFuncPosixNamesAtParen(c *SimpleCmd) Command {
	fn := &FuncDecl{Start: c.Start}
	first := p.funcNameFromWord(c.Args[0])
	fn.Name, fn.NameWord = first.Name, first.Word
	for _, w := range c.Args[1:] {
		fn.AlsoNamed = append(fn.AlsoNamed, p.funcNameFromWord(w))
	}
	cmd := p.parseFuncParensAndBody(fn)
	if p.err != nil {
		return cmd
	}
	for _, r := range c.Redirs {
		fn.addRedir(r)
	}
	return cmd
}

// funcNameFromWord reads a word already parsed as one name of a definition.
//
// The same split [FuncDecl.NameWord] records: a word the shell would expand
// is kept whole, because its literal text names a different function, and
// everything else is its text. Which of the two applies is the dialect's
// answer and not this word's.
func (p *Parser) funcNameFromWord(w *Word) FuncName {
	n := FuncName{Name: w.Literal()}
	if p.dialect.FunctionNameExpands && spansHoldAnExpansion(w.Spans) {
		n.Word = w
	}
	return n
}

// canBeFuncName reports whether a word standing in a definition's name list
// after the first may be one of its names — the tests looksLikeFuncDef makes
// of the word in front of the parentheses, minus the one that is not about
// names at all.
//
// One production, so one rule: a bare pattern character is matched against the
// filesystem and names nothing, and a name that is not text until the shell
// runs is kept only where the dialect expands one. What is *not* asked here is
// whether the word is an assignment — that reading belongs to the first word
// of a command and nowhere else, so measured 2026-09-10 on zsh 5.9.2,
// `a c=d () { :; }` defines `a` and `c=d` where `c=d () { :; }` alone is an
// assignment of an empty array. `a*b c() { :; }` is `no matches found: a*b`
// there and `a "b c" () { :; }` defines both names.
func (p *Parser) canBeFuncName(spans []Span, literal string) bool {
	if spansHoldBarePatternCharacter(spans) {
		return false
	}
	if spansHoldAnExpansion(spans) {
		return p.dialect.FunctionNameExpands
	}
	if p.dialect.FunctionNameIsAnyWord {
		return true
	}
	return isFuncName(literal, p.dialect.FunctionNamePunctuation)
}

// wordCanBeFuncName is canBeFuncName asked of a word already parsed.
func (p *Parser) wordCanBeFuncName(w *Word) bool {
	return p.canBeFuncName(w.Spans, w.Literal())
}

// argsCanBeFuncNames is wordCanBeFuncName over every word already read, which
// is what says an argument list standing in front of `()` is a name list.
func (p *Parser) argsCanBeFuncNames(args []*Word) bool {
	if len(args) == 0 {
		return false
	}
	for _, w := range args {
		if !p.wordCanBeFuncName(w) {
			return false
		}
	}
	return true
}

// failRedirectAt records a redirection operator the grammar did not want,
// named by the operator and not by what follows it.
func (p *Parser) failRedirectAt(pos Pos, op Kind) {
	if p.err != nil {
		return
	}
	p.err = &Error{
		Pos: pos, Kind: ErrUnexpected,
		Token: op.String(), Class: ClassOperator, Redirect: true,
		Msg: op.String() + " unexpected",
	}
}

// failGroupOpeningAPatternOperand records a `(` standing first in an
// expansion's pattern operand, which one dialect refuses while reading — see
// [Dialect.GroupOpeningAPatternOperandIsRefused].
//
// Its own recorder rather than the token path's, because there is no token:
// the operand is text the brace scanner already took, and what is refused is
// its first byte.
func (p *Parser) failGroupOpeningAPatternOperand(pos Pos) {
	if p.err != nil {
		return
	}
	p.err = &Error{
		Pos: pos, Kind: ErrUnexpected,
		Token: "(", Class: ClassOperator,
		Msg: "`(' unexpected",
	}
}

// refuseProcSubstOutOfPlace refuses a process substitution carried by a word
// that does not stand where a command takes one, and reports whether it did.
//
// One helper called from the five positions the panel measures rather than a
// test written out at each: see
// [Dialect.ProcessSubstitutionOnlyWhereACommandTakesAWord], where the rows
// are. The five are a `[[ ]]` operand, a `case` subject, a `case` arm's
// pattern, a loop header's word list and an array literal's element, plus a
// here-string's operand — which is a redirection whose target is *text* to be
// fed in rather than a file to be opened, and is refused where `< <(:)` is
// taken.
//
// The word is scanned rather than its first span tested, because the opener
// need not begin it: `[[ x == a<(:) ]]` carries the substitution behind a
// literal and ksh93 refuses it there too.
func (p *Parser) refuseProcSubstOutOfPlace(w *Word) bool {
	if w == nil || !p.dialect.ProcessSubstitutionOnlyWhereACommandTakesAWord {
		return false
	}
	for _, s := range w.Spans {
		opener, ok := procSubstOpener(s.Kind)
		if !ok {
			continue
		}
		p.failProcSubstOutOfPlace(s.Pos, opener)
		return true
	}
	return false
}

// procSubstOpener is the two characters a process substitution is refused by
// name as, and whether the span is one at all.
func procSubstOpener(k SpanKind) (string, bool) {
	switch k {
	case ProcSubstIn:
		return "<(", true
	case ProcSubstOut:
		return ">(", true
	case ProcSubstFile:
		return "=(", true
	}
	return "", false
}

// failProcSubstOutOfPlace records a process substitution's opener standing
// where the dialect has no word for it.
//
// Its own recorder rather than the token path's, and for the same reason
// failGroupOpeningAPatternOperand has one: there is no token. The opener was
// folded into a word by the lexer, and what is refused is the two characters
// the fold began at.
func (p *Parser) failProcSubstOutOfPlace(pos Pos, opener string) {
	if p.err != nil {
		return
	}
	p.err = &Error{
		Pos: pos, Kind: ErrUnexpected,
		Token: opener, Class: ClassOperator,
		Msg: "`" + opener + "' unexpected",
	}
}

// peekIsAnonBody reports whether `function` is followed by a body rather than
// by a name, which is the keyword spelling of an anonymous function.
//
// The body may stand on a later line, and that is not a formatting liberty —
// it changes what the construct *is*. Measured 2026-09-19 against zsh 5.9.2
// with `-f`, each row a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`: `function` then a newline then `{ echo $#; } a b` prints `2`,
// and so do the spellings with a blank line, a `;`, a comment line or several
// of each in between. So the words after the body are the call's positional
// parameters exactly as they are on one line, and refusing them left this
// engine at a parse error where the reference runs.
//
// The rows with no arguments, which look like they already agreed, are what
// say this is a reach rather than a repair. Read as a bare keyword followed by
// an ordinary group they print the same bytes — and they are not the same
// program: `function` ⏎ `{ typeset y=2; }` ⏎ `echo $y` writes nothing in the
// reference, because the body was a function's and the name was local to it,
// where a group would have left `2` behind. That is measured, and it is why
// the reach does not wait for an argument to appear.
//
// What is *not* skipped is everything that ends the command rather than
// separating one: a redirection, `&&`, `||` and `|` all leave the keyword
// standing alone, measured a spelling at a time, and the arm terminators
// `;;`, `;&` and `;|` are not the separator `;` is. So this steps over blanks,
// newlines, a plain `;`, a line continuation and a comment, and stops at the
// first thing that is none of them (#3778).
func (p *Parser) peekIsAnonBody() bool {
	src, i := p.lex.src, p.lex.off
	for i < len(src) {
		switch c := src[i]; {
		case isBlank(c), c == '\n':
			i++
		case c == ';':
			if i+1 < len(src) && (src[i+1] == ';' || src[i+1] == '&' || src[i+1] == '|') {
				// An arm terminator, which ends the command the keyword is
				// in rather than separating it from the next one.
				return false
			}
			i++
		case c == '\\' && i+1 < len(src) && src[i+1] == '\n':
			i += 2
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		default:
			return c == '{' || c == '('
		}
	}
	return false
}

// parseAnonFunc reads `() body [word …]` and `function body [word …]`.
//
// The words after the body are the call's positional parameters, which is
// what makes this a call and not only a definition: there is no name to
// invoke it by later, so it runs where it stands. They are read the way a
// simple command's arguments are, and stop where a command stops.
func (p *Parser) parseAnonFunc(keyword bool) Command {
	fn := &AnonFunc{Keyword: keyword, Start: p.tok.Pos}
	p.next()
	if !keyword {
		if !p.at(TokRightParen) {
			p.failUnexpectedOperand(")")
			return fn
		}
		p.next()
	}
	p.skipAnonBodySeparators()
	// A nameless function's body is a function body, which is what says the
	// try-always keyword may not follow it: measured 2026-09-07, `() { echo
	// anon; } always { echo A; }` is a parse error on the `}` in the shell
	// that has both constructs — the `always` and the `{` after it are read
	// as *arguments* to the call, which is what the word loop below already
	// does. Without saying so here, the brace group is offered the keyword
	// the way one standing as a command is (#1216).
	p.funcBody = true
	fn.Body = p.parseCommand()
	if fn.Body == nil {
		if keyword {
			p.fail("expected a body after `function`")
			return fn
		}
		// `()` with nothing after it is the empty subshell it has always
		// been rather than a function with no body — measured, `( ); echo
		// ok` prints `ok` in the shell that has both readings, which is the
		// EmptyCompoundBody rule and not this one. The parentheses have been
		// consumed, so the node is built here rather than parsed again.
		return &Subshell{Start: fn.Start, Stop: p.tok.Pos}
	}
	for p.tok.Kind == TokWord && !p.atStopWord() {
		fn.Args = append(fn.Args, p.word())
	}
	return fn
}

// funcKeywordName reads one name after the `function` keyword and consumes it.
//
// The first name and every name after it go through this, so a definition
// that gives several cannot read its second name by a different rule from its
// first: whether a name may hold an expansion and whether any word at all is
// a name are the dialect's answers, and they are the same answers for every
// name in the list.
//
// ok is false where the word is not a name this dialect takes, and the caller
// raises the diagnostic — the token has not been consumed, so the failure is
// reported at the word that was refused.
func (p *Parser) funcKeywordName() (FuncName, bool) {
	if tokenHoldsAnExpansion(p.tok) {
		// A name that is not text until the shell runs. Where the dialect
		// expands one it is kept as a word; where it does not, it is refused
		// — this used to fall through to Literal(), which turns `_p_${w}`
		// into `_p_w` and defines a function nobody asked for, at status 0.
		if !p.dialect.FunctionNameExpands {
			return FuncName{}, false
		}
		n := FuncName{Name: p.tok.Literal()}
		n.Word = p.word()
		return n, true
	}
	if !p.keywordFuncName(p.tok) {
		return FuncName{}, false
	}
	n := FuncName{Name: p.tok.Literal()}
	p.next()
	return n, true
}

func (p *Parser) parseFuncKeyword() Command {
	keyword := p.tok
	fn := &FuncDecl{Keyword: true, Start: p.tok.Pos}
	p.next()
	if p.dialect.BareFunctionKeyword && p.bareFunctionKeywordStandsHere() {
		// The keyword and nothing else: an anonymous function whose body is
		// empty, which runs where it stands. The body is an empty group
		// rather than a nil one for the reason the optional-body branch
		// below gives — nothing downstream reads a missing body as a
		// refusal — and it is placed at the keyword's own end, which is
		// where an absent body was written.
		//
		// The redirections are taken here rather than by the caller because
		// [Parser.parseCommand] hands a keyword declaration straight back:
		// a definition's redirections belong to its body and are read with
		// it, and this form has no body to read them with.
		anon := &AnonFunc{Keyword: true, Bare: true, Start: keyword.Pos}
		anon.Body = &Group{Start: keyword.End, Stop: keyword.End}
		return p.withRedirs(anon)
	}
	if p.tok.Kind != TokWord {
		// The token, rather than a sentence of our own about what was
		// wanted. Every column that refuses this quotes what it found in
		// its ordinary unexpected-token wording, and that wording is
		// already what this parser produces everywhere else — see
		// [Dialect.BareFunctionKeyword] for the panel (#3732).
		//
		// A name takes no newline, so the input running out here is the
		// newline one dialect appends to what it reads, exactly as it is in
		// a loop's variable position — see
		// [Dialect.EndOfInputIsANewlineWhereNoneCouldStand].
		if p.tok.Kind == TokEOF && p.dialect.EndOfInputIsANewlineWhereNoneCouldStand {
			p.failUnexpectedWordAt(Token{Kind: TokNewline, Pos: p.tok.Pos})
			return fn
		}
		// A word of any spelling would have stood here, which the dialects
		// that print an expectation say bare: BusyBox ash 1.37.0 answers
		// `function; echo after` with `unexpected ";" (expecting word)`,
		// measured 2026-09-19 in the pinned container. See
		// Error.ExpectedIsAClass.
		p.failUnexpectedWord()
		return fn
	}
	first, ok := p.funcKeywordName()
	switch {
	case ok:
		fn.Name, fn.NameWord = first.Name, first.Word
		// The keyword spelling reaches the same set the `name()` spelling
		// does — `function export { :; }` is refused word for word as
		// `export() { :; }` is — so the one seam serves both.
		if p.refuseNameKeptByTheDialect(fn) {
			return fn
		}
	case p.dialect.FunctionNameCheckedWhenTheDefinitionRuns:
		// The word is not a name, and this dialect says so where the
		// definition *runs* rather than here — so the declaration is read
		// whole and the word is kept as it was written, which is what the
		// complaint quotes. The name list below is not entered: no dialect
		// has both this and [Dialect.FunctionMultipleNames], and a list of
		// names one of which is refused is a shape no shell in the panel has.
		fn.RefusedName = p.refusedFuncName(p.tok)
		p.next()
	default:
		p.fail("expected a name after `function`")
		return fn
	}
	// Every word after the first is another name for the same body, where the
	// dialect has the list. Greedily, the way a loop's name list is taken:
	// the words stop at the body's `{`, at the `()` of the hybrid form, at a
	// stop word and at anything that is not a word at all, and a word that
	// opens a construct is a name like any other — see
	// [Dialect.FunctionMultipleNames], where the readings are measured.
	for p.dialect.FunctionMultipleNames &&
		p.at(TokWord) && !p.atStopWord() && !p.atWord("{") {
		also, ok := p.funcKeywordName()
		if !ok {
			p.fail("expected a name after `function`")
			return fn
		}
		fn.AlsoNamed = append(fn.AlsoNamed, also)
	}
	// And where the dialect takes words after the name that are *not* names
	// for the body, they are read and dropped. The list stops at the end of
	// the line, which is what makes the one-line spelling blame the line
	// after it — see [Dialect.FunctionKeywordReferenceList].
	for p.dialect.FunctionKeywordReferenceList && p.at(TokWord) &&
		!p.atReservedWord() {
		if !p.referenceListName(p.tok) {
			p.fail("invalid reference list")
			return fn
		}
		p.next()
	}
	if p.at(TokLeftParen) {
		// The hybrid `function f() {}`: bash and zsh take it, ksh93 rejects
		// it. Accepting it everywhere the keyword exists meant the ksh
		// dialect ran a definition ksh93 calls a syntax error.
		if !p.dialect.FunctionKeywordParens {
			p.failUnexpected("")
			return fn
		}
		p.next()
		if p.at(TokRightParen) {
			p.next()
		}
	}
	// Where the body is optional a separator may stand in front of it, and
	// the position the names ended at is kept: it is where an absent body is
	// recorded as being. See [Dialect.FunctionKeywordBodyIsOptional].
	afterNames := p.tok.Pos
	p.skipNewlines()
	if p.dialect.FunctionKeywordBodyIsOptional {
		for p.err == nil && p.at(TokSemi) {
			p.next()
			p.skipNewlines()
		}
	}
	body := p.tok
	p.funcBody = true
	if fn.Body = p.funcKeywordBody(); fn.Body == nil {
		if p.dialect.FunctionKeywordBodyIsOptional && p.err == nil {
			// No body at all, which is a declaration rather than a failure:
			// each name is defined with an empty one. An empty group is how
			// that is said, so nothing downstream has a nil body to read as
			// a refusal — and it is what the shell itself reports, printing
			// `a () { }` for a name declared this way.
			fn.Body = &Group{Start: afterNames, Stop: afterNames}
			return fn
		}
		p.failUnexpectedAt(body, "", false)
		return fn
	}
	if !p.funcKeywordBodyIsTakenHere(fn.Body) {
		p.failUnexpectedAt(body, "", false)
	}
	return fn
}

// funcKeywordBody reads the body of a `function` keyword's declaration.
//
// One dialect ends the declaration at a brace group's `}` and lets every
// other body reach to the end of the and-or list — see
// [Dialect.FunctionKeywordBodyIsAnAndOrList], where the order the two
// readings print in is measured. Everywhere else a body is one command, which
// is what [Parser.parseCommand] reads.
//
// A list of more than one pipeline is wrapped in a [Group], because
// [FuncDecl.Body] is a Command and an and-or list is not one. A single
// command is handed back bare: it is the body it was before this flag, and
// wrapping it would change what every existing definition prints back as.
func (p *Parser) funcKeywordBody() Command {
	if !p.dialect.FunctionKeywordBodyIsAnAndOrList || p.atWord("{") {
		return p.parseCommand()
	}
	expr := p.parseAndOr()
	if expr == nil {
		return nil
	}
	if pl, ok := expr.(*Pipeline); ok && !pl.Negated && len(pl.Cmds) == 1 {
		return pl.Cmds[0]
	}
	return &Group{
		List:  []*Stmt{{Expr: expr}},
		Start: expr.Pos(),
		Stop:  expr.End(),
	}
}

// funcKeywordBodyIsTakenHere reports whether the command read after the
// `function` keyword may stand as a body in this dialect.
//
// The question is the body's *shape* and it is asked here rather than in
// [Parser.parseFuncParensAndBody] because the two spellings of a definition
// are not one rule: the shell that wants a compound body after the keyword
// wants one after the parentheses too, and the shell that wants a brace group
// after the keyword takes a bare simple command after the parentheses. See
// [Dialect.FunctionKeywordBodyMustBeBraceGroup].
func (p *Parser) funcKeywordBodyIsTakenHere(body Command) bool {
	if p.dialect.FunctionKeywordBodyMustBeBraceGroup {
		_, isGroup := body.(*Group)
		return isGroup
	}
	if p.dialect.FuncBodyMustBeCompound {
		_, isSimple := body.(*SimpleCmd)
		return !isSimple
	}
	return true
}

func (p *Parser) parseSubshell() Command {
	c := &Subshell{Start: p.tok.Pos}
	defer p.opens("(")()
	p.next()
	c.List = p.parseBody()
	if !p.at(TokRightParen) {
		if p.at(TokEOF) {
			p.ranOut()
			if p.err == nil {
				p.err = p.unterminated(")")
			}
			return c
		}
		// Named the same way a brace group names it: the token that stopped
		// the list, and the closer that was wanted alongside it only where
		// the subshell had something in it. `( echo a; fi )` is
		// `"fi" unexpected (expecting ")")` in the one shell that prints an
		// expectation and `( fi )` is `"fi" unexpected` there, which is
		// measured — an empty subshell has nothing to be in the middle of.
		expected := ""
		if len(c.List) > 0 {
			expected = ")"
		}
		p.failUnexpected(expected)
		return c
	}
	c.Stop = p.tok.End
	p.next()
	return c
}

func (p *Parser) parseGroup(funcBody bool) Command {
	word := "{"
	if funcBody {
		word = "function"
	}
	return p.parseGroupOpenedBy(word)
}

// parseGroupOpenedBy is parseGroup with the construct the braces belong to
// named by the caller, which is what an unterminated one is reported as.
//
// The word was already the enclosing construct's for a function body — the
// shells name `function` there and not `{` — and a `namespace` block is the
// second construct whose braces are its own punctuation: measured 2026-09-19,
// `namespace ns {` with nothing after it is “ syntax error at line 2:
// `namespace' unmatched “ on ksh93u+ and not “ `{' unmatched “.
func (p *Parser) parseGroupOpenedBy(word string) Command {
	c := &Group{Start: p.tok.Pos}
	defer p.opens(word)()
	p.next()
	c.List = p.parseBody()
	if !p.atWord("}") {
		if p.at(TokEOF) {
			p.ranOut()
			if p.err == nil {
				p.err = p.unterminated("}")
			}
			return c
		}
		// A reserved word the group cannot use — `{ echo a; do :; done; }`.
		// Every shell in the panel names the word it stopped on, so this goes
		// through the usual failure rather than describing the brace group:
		// a message that said only "expected }" named neither the token nor
		// the four different ways the shells say it.
		//
		// Whether the expectation is named alongside it depends on whether
		// the group had anything in it. dash writes `(expecting "}")` after
		// `{ echo a; esac; }` and not after `{ esac; }`, which is measured —
		// the empty group has nothing to be in the middle of.
		expected := ""
		if len(c.List) > 0 {
			expected = "}"
		}
		p.failUnexpected(expected)
		return c
	}
	c.Stop = p.tok.End
	p.next()
	return c
}

// parseGroupOrTry reads a brace group, and the `always` block after it where
// the dialect has one.
//
// The keyword is read *here* rather than from the command dispatch or from the
// stop-word set, and that placement is the production: `always` is an ordinary
// word everywhere else, so a shell with this construct still runs `always`,
// defines a function called it, and prints it as an argument. Measured — see
// [Dialect.TryAlways] for the shapes that are refused and why.
//
// A function body is not offered the keyword. `f() { :; } always { … }` is a
// parse error in the shell that has the construct, and funcBody is how the
// dispatch already says which brace group is a body; the loop bodies are
// refused by construction, since [Parser.braceLoopBody] reads its group
// directly rather than through the dispatch.
func (p *Parser) parseGroupOrTry(funcBody bool) Command {
	c := p.parseGroup(funcBody)
	if funcBody || !p.dialect.TryAlways || p.err != nil || !p.atWord("always") {
		return c
	}
	g, ok := c.(*Group)
	if !ok {
		return c
	}
	t := &TryClause{Try: g.List, Start: g.Start, Stop: g.Stop}
	p.next()
	if !p.atWord("{") {
		// The second half has to be a brace group. `{ echo t; } always echo
		// a` is a parse error on the `echo` in the shell that has the
		// construct rather than on the keyword, which is what naming the
		// token we actually stopped on gives.
		p.failUnexpected("")
		return t
	}
	always, ok := p.parseGroup(false).(*Group)
	if !ok {
		return t
	}
	t.Always, t.Stop = always.List, always.Stop
	return t
}

func (p *Parser) parseArithCmd() Command {
	c := &ArithCmdClause{Expr: p.tok.Text, Start: p.tok.Pos, Stop: p.tok.End}
	// The expression is not read as part of reading the file: every shell in
	// the panel that has this construct complains about it when the command
	// runs, so the raw text on Expr is what a diagnostic will quote and
	// Parsed is nil until then if it cannot be read at all (#865).
	c.Parsed = p.parseArithLater(p.tok.Text, p.tok.Pos)
	p.next()
	return c
}

// requireSep consumes the terminator a compound command needs before its
// keyword. The keyword does not delimit the condition; this does.
func (p *Parser) requireSep(before string) {
	switch p.tok.Kind {
	case TokSemi, TokNewline:
		p.next()
		p.skipNewlines()
	default:
		if p.at(TokEOF) {
			// `if` on its own: the input ran out before the construct could
			// be closed, which is a different failure from a word in the
			// wrong place and is reported as one.
			p.ranOut()
			if p.err == nil {
				p.err = p.unterminated(before)
			}
			return
		}
		if !p.atWord(before) {
			p.failUnexpected(before)
		}
	}
}

// peekIsArithCmd reports whether `((` follows, which is what tells a
// C-style `for` from one over a list. The lexer has already decided where the
// matching `))` is, so this only has to look.
func (p *Parser) peekIsArithCmd() bool {
	return p.lex.peekIsArithCommand()
}

// parseForArith reads `for ((init; cond; post))`.
//
// The three expressions arrive as one token — the lexer keeps `(( … ))` whole
// because what is inside is arithmetic and not a command list — so they are
// split here on the semicolons the arithmetic grammar has no use for.
//
// Specified in the C-style `for` section of docs/spec/grammar/commands.md,
// which also records the one shape this does not accept: the panel takes a
// brace group as the body where this requires `do … done`.
func (p *Parser) parseForArith(start Pos) Command {
	c := &ForArithClause{Start: start}
	p.next() // for
	text := p.tok.Text
	c.Header = "for ((" + text + "))"
	at := p.tok.Pos
	p.next()

	init, cond, post := splitForArith(text, p.dialect)
	c.InitText, c.CondText, c.PostText = init, cond, post
	p.checkForArithSeparators(text, at)
	// The deferred trees are built from the parts *as written* rather than
	// from the trimmed fields, so that an offset one of them records — the
	// `YStart` a division's blame is sliced from — indexes the same string
	// the interpreter later quotes back. Parsing the trimmed text and
	// quoting the untrimmed one moves every offset by the leading blanks,
	// which blamed `for (( i=1/0 ;; ))` on `/0 ` instead of `0 `.
	rawInit, rawCond, rawPost := c.PartsAsWritten()
	// The three parts defer too, and measured rather than assumed to: bash,
	// ksh93 and zsh all take `for ((echo hi;;))` under `-n` and all reach
	// past one in a branch that never runs. forArithPart already reads a
	// part from its text where there is no tree (#865).
	if init != "" {
		c.Init = p.parseArithLater(rawInit, at)
	}
	if cond != "" {
		c.Cond = p.parseArithLater(rawCond, at)
	}
	if post != "" {
		c.Post = p.parseArithLater(rawPost, at)
	}

	// This header ends itself, so the terminator before the body is optional
	// — and it is consumed here rather than by `requireSep` so that a brace
	// body can be looked for either side of it. `requireSep` still runs, and
	// still tells a loop that ran out of input from one that met the wrong
	// token.
	if p.tok.Kind == TokSemi || p.tok.Kind == TokNewline {
		p.next()
		p.skipNewlines()
	}
	if p.braceBodyFollows() {
		c.Body, c.Stop = p.braceLoopBody()
		return c
	}
	// This header ends itself too, so where a body may be short it may also
	// be one command, or nothing.
	if p.dialect.ShortForm && !p.atWord("do") {
		c.Body, c.Stop = p.shortFormBody()
		return c
	}

	p.requireSep("do")
	p.opensClause("do")
	p.expectWord("do")
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

// braceBodyFollows reports whether a brace group stands where `do` belongs.
func (p *Parser) braceBodyFollows() bool {
	return p.dialect.ForBraceBody && p.atWord("{")
}

// braceLoopBody reads a brace group standing where `do … done` stands.
//
// The group is the ordinary one and keeps every rule it already has: the body
// needs a terminator before `}` wherever a brace group does, an empty body is
// refused wherever an empty group is, and a redirection after the closing
// brace belongs to the loop exactly as one after `done` does. Its list *is*
// the loop's body, so `break` and `continue` reach the loop rather than a
// group in the way.
func (p *Parser) braceLoopBody() (body []*Stmt, stop Pos) {
	g, ok := p.parseGroup(false).(*Group)
	if !ok {
		return nil, p.tok.End
	}
	// A brace body is closed by its own `}` and takes no terminator with it,
	// which is the rule bodyTookTerm's own doc states — so whatever a *inner*
	// short form left there is not this construct's. Cleared rather than
	// left, because the last thing read inside the braces may well have been
	// one: `for i (a b) { for j (c d) echo $j; } echo end` is refused by the
	// shell that has short loops, exactly as `for i (a b) { echo $i; } echo
	// end` is, and without this line the inner loop's `;` terminated the
	// outer one and the tail ran (measured on zsh 5.9.2, 2026-09-12).
	p.bodyTookTerm, p.bodyTookKind = Pos{}, TerminatedByNothing
	// Set after the group is read and not before, for the same reason the
	// line above clears rather than leaves: a short form *inside* the braces
	// runs through here too, and its answer is not this one's.
	p.shortBodyBraced = true
	return g.List, g.Stop
}

// shortFormBody reads a body written where `do … done` stands, for a header
// that has already ended: one command, or none at all.
//
// One rather than a list, and that is the construct rather than a
// simplification — a second command needs a separator before it, and a
// separator here belongs to whatever encloses the loop. `for i (a b) echo $i;
// echo done` prints a, b, done and not a, done, b, done.
//
// None at all is the same rule with nothing after the header, and it is
// reached far more often than it looks: it is what `while cond; { … }` means,
// because the `;` puts the brace group in the condition list and leaves the
// body with nothing.
func (p *Parser) shortFormBody() (body []*Stmt, stop Pos) {
	if p.braceBodyFollows() {
		return p.braceLoopBody()
	}
	p.shortBodyBraced = false
	if p.at(TokEOF) {
		// Nothing at all because the input ended, which is two different
		// facts wearing one shape. To a program that is all there is, the
		// loop is finished and its body is empty: `for i in 1 2` and a
		// newline is a whole command to `-c`, and it runs nothing. At a
		// prompt the identical text is a promise — the shell asks for
		// another line and takes it as the body — so the difference cannot
		// be in the parse. What the parser can say is that the input ran out
		// while it was still inside the construct, which is what Incomplete
		// means: a reader that can fetch another line does, and one that
		// cannot keeps the empty body it already has (#1298).
		p.ranOut()
		return nil, p.tok.Pos
	}
	if p.atStopWord() || p.at(TokRightParen) ||
		(p.dialect.ShortBodyEndsOnAJoiningOperator && p.atAJoiningOperator()) {
		// A stop word or a `)` is somebody else's, and it is *here*, so the
		// input did not run out: the body is empty and the construct is
		// finished. `while cond; { … }` is this, with the group taken as the
		// condition and `}` left standing where a body could have been.
		//
		// A joining operator is the same answer arrived at from the other
		// side: it cannot begin a command, so the body it stands after is
		// empty and the loop it finishes is the operator's left-hand side.
		return nil, p.tok.Pos
	}
	st := p.parseStmt()
	if st == nil {
		if p.err == nil {
			// A token no command can begin with, standing where the short
			// body does: `while & do :; done`. Named here rather than left
			// for the caller, because leaving it named the `do` — the loop
			// closed with an empty body, the `&` was stepped past, and the
			// stop word two tokens later was the first thing anything
			// refused. Measured on zsh 5.9.2, 2026-09-12: that shell says
			// ``parse error near `&' `` for both `while` and `until`, which
			// is the token that is actually there.
			p.failUnexpected("")
		}
		return nil, p.tok.Pos
	}
	// The separator the body just took is the loop's as well: there is
	// nothing between the two commands but the one `;`, so a list may carry
	// on after the loop where it could not after a `done` or a `}`.
	p.bodyTookTerm, p.bodyTookKind = st.Semi, st.Term
	return []*Stmt{st}, st.End()
}

// atAJoiningOperator reports whether the token is one that joins two commands
// — the pipeline's bars and the and-or list's operators — and so can only
// stand *after* one.
//
// It is what says a short-form body is empty rather than missing. Measured
// 2026-09-20 on zsh 5.9.2, each line its own script file under
// `env -i -u FPATH PATH=/usr/bin:/bin LC_ALL=C` with no standard input:
//
//	for i in a b; | cat; print T          `T`        the loop ran, piped, nothing
//	for i in a b; |& cat; print T         `T`
//	for i in a b; && print x              `x`        so the loop succeeded
//	select o in a b; | cat; print T       menu, `T`
//	repeat 2; | cat; print T              `T`
//	for i in a b; & print x               refused    `&` is not one of them
//	for i in a b; ;; print x              refused    nor is a case terminator
//	for i in a b; do :; done; | cat       refused    nor is the long form lenient
//
// The last row is the control that makes this the *short* body's question:
// the same `|` after a `done` is a syntax error in zsh and here alike, so
// what moved is a body that was never written rather than what a bar may
// follow.
//
// A `;` is deliberately not in the set, and that is measured rather than
// tidied away: `for i in a b; ; print x` prints `x` twice in zsh, so the
// second separator is stepped over and the `print` becomes the body — an
// empty body there would run it once. That is
// [Dialect.SeparatorWhereACommandBelongs]'s question and not this one.
// Asked only where [Dialect.ShortBodyEndsOnAJoiningOperator] is on. The rows
// above are all zsh, and the core is the language every panel shell accepts —
// so this is added by a dialect rather than taken from one, the additive
// direction [Dialect.EmptyCompoundBody] describes for a grammar flag. Ungated
// it also reached the core, where it moved `while | do :; done` onto the stop
// word `do` — the exact shape TestAShortBodyRefusesTheTokenThatIsThere exists
// to refuse.
func (p *Parser) atAJoiningOperator() bool {
	switch p.tok.Kind {
	case TokPipe, TokPipeAmp, TokAndAnd, TokOrOr:
		return true
	}
	return false
}

// PartsAsWritten is the three parts with the blanks the script wrote around
// them, which the fields do not keep.
//
// [ForArithClause].InitText, CondText and PostText are trimmed because they
// are also what the printer lays a header out from, and untrimmed parts print
// back with their blanks doubled — see syntax/printroundtrip_test.go, whose
// promise is that printing a program gives the same program. A diagnostic
// wants the other answer: bash quotes a failing part back as it was written,
// so `for (( $x ;;))` with x=`echo hi` is `((: echo hi : …` there. So the
// spelling is re-split from [ForArithClause].Header, which already holds the
// header verbatim and which SameProgram already skips for being a spelling.
//
// Empty parts and a Header that is not this clause's both come back as the
// trimmed fields, so a tree built by hand answers as it always did.
func (c *ForArithClause) PartsAsWritten() (init, cond, post string) {
	text, ok := strings.CutPrefix(c.Header, "for ((")
	if text, ok2 := strings.CutSuffix(text, "))"); ok && ok2 {
		// The core grammar, because this is not a parse: the split needs a
		// dialect only to find where a command substitution ends, and a
		// header whose substitution this cannot follow falls back to the
		// fields below rather than to a different answer.
		parts := forArithSplit(text, Dialect{})
		if len(parts) == 3 {
			return parts[0], parts[1], parts[2]
		}
	}
	return c.InitText, c.CondText, c.PostText
}

// checkForArithSeparators refuses a C-style `for` header that does not hold
// the two separators its three expressions are parted by.
//
// The count is what the header *means*, not decoration: two separators are
// three expressions, and a missing one is a missing expression rather than an
// empty one. The condition is the expression that matters, because an absent
// condition is true — which is what makes `for ((;;))` the endless loop every
// shell spells that way, and what made `for (())` an endless loop here when it
// should have been refused before anything ran (#2225).
//
// text is the header between the parentheses, and at the position the header
// opens at, which is the line every shell that reports this names — the line
// the `for ((` is on, not the line the count ran out on, so a header written
// over four lines is still reported at its first.
func (p *Parser) checkForArithSeparators(text string, at Pos) {
	if p.err != nil {
		return
	}
	// The parts as the header wrote them rather than as splitForArith pads
	// them: one dialect names the last part's text, and a header with one
	// part has that part as its last where the padded three have an empty
	// string there.
	parts := forArithSplit(text, p.dialect)
	kind, msg := ErrForArithHeader, "arithmetic expression required"
	switch seps := len(parts) - 1; {
	case seps == 2:
		return
	case seps > 2:
		if p.dialect.ForArithExtraSeparators {
			// The dialect folds everything past the second separator into
			// the third expression and lets the arithmetic reader complain
			// about it if the loop ever gets there, which is what the two
			// shells that take this header do.
			return
		}
		kind, msg = ErrForArithSeparator, "`;' unexpected"
	}
	p.err = &Error{
		Pos: at, Kind: kind, Msg: msg,
		Token:     "((" + text + "))",
		LastToken: strings.TrimSpace(parts[len(parts)-1]),
	}
}

// forArithSplit cuts a C-style `for` header on the semicolons that are the
// header's own, and on no others.
//
// A `;` reached through a command substitution, a parenthesized group, a
// brace expansion or a quotation belongs to whatever encloses it, and is not
// one of the two separators the header is counted by. Measured 2026-09-12 on
// bash 5.3.15, ksh93u+ and zsh 5.9.2, which are unanimous: `for (( i=$(echo
// 1;true) ;; ))` runs the body in all three, and `for (( i=0; i<$(echo
// 1;echo 2); i++ ))` reaches all three's *arithmetic* reader with a
// two-line value rather than any of their parsers. A naive cut on every `;`
// counted four separators there and refused the script, which forfeited the
// whole file this was found in.
//
// A header whose nesting does not come out even is cut the old way instead.
// That is deliberate: an unbalanced header is a malformed one, the counting
// is what reports it, and a scanner that gave up in the middle would change
// which complaint a broken header draws.
func forArithSplit(text string, d Dialect) []string {
	if parts := forArithParts(text, d); parts != nil {
		return parts
	}
	return strings.Split(text, ";")
}

// endOfSubstitution reports the index of the `)` closing a `$( … )` that
// begins at text[i], and false where text[i] does not begin one or where what
// stands between the parentheses is not a program this dialect reads.
//
// False is the safe answer in both cases: the caller carries on counting
// parentheses, which is what it did for every construct before this.
func endOfSubstitution(text string, i int, d Dialect) (int, bool) {
	if i+2 > len(text)-1 || text[i+1] != '(' {
		return 0, false
	}
	if text[i+2] == '(' {
		// `$((` is an expression and not a program, and its parentheses
		// balance, so the counting reads it correctly on its own.
		return 0, false
	}
	sub := NewParserAt(text[i+2:], d, 1)
	sub.parseList()
	if sub.err != nil || !sub.at(TokRightParen) {
		return 0, false
	}
	return i + 2 + int(sub.tok.Pos.Offset), true
}

// endOfBraces reports the index of the `}` closing a `${ … }` that begins at
// text[i], and false where text[i] does not begin one or where the scan ran
// out of input before the brace arrived.
func endOfBraces(text string, i int, d Dialect) (int, bool) {
	if i+1 > len(text)-1 || text[i+1] != '{' {
		return 0, false
	}
	sub := NewLexer(text[i:], d)
	sub.scanBraces(Unquoted)
	if sub.Err() != nil {
		return 0, false
	}
	return i + sub.off - 1, true
}

// forArithParts is forArithSplit's scan, and nil where the header's quoting
// or nesting does not come out even.
//
// Nothing here interprets the header — it is one pass over the bytes that
// knows only which of them open and close something. The arithmetic grammar
// reads the parts afterwards, exactly as it did when the cut was a
// [strings.Split].
func forArithParts(text string, d Dialect) []string {
	var parts []string
	start, depth := 0, 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote == '\'':
			// A single-quoted run ends at its own quote and holds no escape.
			if c == '\'' {
				quote = 0
			}
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		default:
			switch c {
			case '\\':
				i++
			case '$':
				// A command substitution is stepped over by *parsing* it,
				// because a `)` inside one is not always a closer: a `case`
				// arm's pattern ends in one, and counting that as a closer
				// unbalanced the header and fell back to the naive cut —
				// which is how `for (( $(case q in q) echo 7;; esac) ;; ))`
				// came to be refused for holding three separators where
				// bash 5.3.15, ksh93u+ and zsh 5.9.2 all run the body.
				// `$((` is arithmetic rather than a program and balances on
				// its own parentheses, so it is left to the counting below.
				if n, ok := endOfSubstitution(text, i, d); ok {
					i = n
					break
				}
				// A `${ … }` ends at its own brace, and the form holding a
				// program may put a `)` inside that closes nothing — the
				// same shape one construct out. scanBraces is the scanner
				// that knows where one ends.
				if n, ok := endOfBraces(text, i, d); ok {
					i = n
				}
			case '\'', '"', '`':
				quote = c
			case '(', '{', '[':
				depth++
			case ')', '}', ']':
				depth--
				if depth < 0 {
					return nil
				}
			case ';':
				if depth == 0 {
					parts = append(parts, text[start:i])
					start = i + 1
				}
			}
		}
	}
	if depth != 0 || quote != 0 {
		return nil
	}
	return append(parts, text[start:])
}

// splitForArith cuts the header into its three parts.
//
// An omitted part is empty, and an omitted *condition* means true — which is
// what makes `for ((;;))` an endless loop rather than one that never runs.
func splitForArith(text string, d Dialect) (string, string, string) {
	parts := forArithSplit(text, d)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	if len(parts) > 3 {
		// Everything past the second separator is the third expression, and
		// it keeps the semicolons it was written with: that is what the two
		// dialects taking such a header do with it, and the arithmetic
		// reader is what complains about it if the loop ever gets there.
		parts = append(parts[:2:2], strings.Join(parts[2:], ";"))
	}
	// Trimmed, and that costs one blank in one diagnostic: bash quotes a
	// failing part back as it was written, so `for (( $x ;;))` with
	// x=`echo hi` is `((: echo hi : …` there and `((: echo hi: …` here. The
	// blanks are not kept because the tree is also what the printer reads,
	// and a header printed from untrimmed parts comes back with the blanks
	// doubled — see syntax/printroundtrip_test.go, whose promise is that
	// printing a program gives the same program.
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
}

// loopWord names the construct a loop opened with, which is what a
// diagnostic has to say back.
func loopWord(until bool) string {
	if until {
		return "until"
	}
	return "while"
}

func (p *Parser) parseIf() Command {
	c := &IfClause{Start: p.tok.Pos}
	defer p.opens("if")()
	p.next()
	c.Cond = p.parseCondition()
	// A condition that ended itself may be followed straight by the body,
	// exactly as a loop's header may — the rule ShortForm stands for, which
	// says nothing about looping. `if [[ -n x ]] { … }` and
	// `if (( 1 )) echo A` are that rule reaching `if`; `if true { … }` is
	// still refused, because `true` is a simple command and `{` is another
	// of its words.
	if body, stop, short := p.shortIf(c.Cond); short {
		c.Then, c.Stop = body, stop
		p.shortElse(c)
		return c
	}
	p.requireSep("then")
	// Recorded after it is consumed: until then the innermost thing awaiting
	// a partner is the `if` itself, which is what one shell names for
	// `if true` and not for `if true; then echo x`.
	p.opensClause("then")
	p.expectWord("then")
	c.Then = p.parseBody()
	p.longIfTail(c)
	return c
}

// longIfTail reads what is left of an `if` whose current arm was written the
// long way: the `elif` chain, the `else` arm and the `fi` that ends them.
//
// It is a function rather than the tail of [Parser.parseIf] because the two
// spellings compose in both directions and either one may hand over. A long
// arm may be followed by a short one — [Parser.shortIf] inside the loop below
// — and a short arm may be followed by a long one, which is
// [Parser.shortElse] calling back here.
func (p *Parser) longIfTail(c *IfClause) {
	for p.atWord("elif") && p.err == nil {
		e := &Elif{Start: p.tok.Pos}
		p.opensClause("elif")
		p.next()
		e.Cond = p.parseCondition()
		if body, stop, short := p.shortIf(e.Cond); short {
			// The two spellings compose: a long `if … ; then …` may carry an
			// `elif` whose condition ended itself and whose body is written
			// with braces. Measured on zsh 5.9.2 — `if (( 0 )); then echo A;
			// elif (( 1 )) { echo B }; echo tail` prints `B` then `tail`
			// there, and `elif [[ -n y ]] { … }` does the same while
			// `elif : { … }` is refused, which is shortIf's own test showing
			// through. From here the chain is a short one and there is no
			// `fi` left to read, so the rest of it is shortElse's (#1880).
			e.Then, c.Stop = body, stop
			c.Elifs = append(c.Elifs, e)
			p.shortElse(c)
			return
		}
		p.requireSep("then")
		p.expectWord("then")
		e.Then = p.parseBody()
		c.Elifs = append(c.Elifs, e)
	}
	if p.atWord("else") {
		p.opensClause("else")
		p.next()
		c.HasElse = true
		c.Else = p.parseBody()
	}
	c.Stop = p.tok.End
	p.expectWord("fi")
}

// shortIf reads the body of an `if` whose condition ended itself, where the
// dialect allows one without `then … fi`.
//
// It reports false wherever the long form still applies, so a condition that
// did not end itself, a dialect without the flag, and a `then` standing where
// it always could all fall through untouched. There is no separator to
// consume first, and that is the measurement rather than a simplification:
// `if [[ -z x ]] echo A; else echo B; fi` is an error in the shell that has
// this, because the `;` ended the whole command and left `else` with nothing
// to attach to.
func (p *Parser) shortIf(cond []*Stmt) (body []*Stmt, stop Pos, short bool) {
	if !p.dialect.ShortForm || !condEndedItself(cond) {
		return nil, Pos{}, false
	}
	// `then` needs no test of its own: it is a stop word, so the long form
	// falls through here with everything else the grammar could still want.
	// Naming it as well was a line no test could distinguish.
	if p.at(TokEOF) || p.atStopWord() {
		return nil, Pos{}, false
	}
	body, stop = p.shortFormBody()
	return body, stop, true
}

// shortElse reads the `elif` and `else` arms of a short `if`, which take the
// same body by the same rule and end where it ends: there is no `fi`.
//
// Each arm chooses its own form, and the first one not written the short way
// puts the rest of the construct in the long one — where there *is* a `fi`,
// and it is required. Measured on zsh 5.9.2 from a script file, 2026-09-12,
// with `if (( 0 )) { echo A }` as the opening arm every time:
//
//	else echo B; fi                → B          the arm is a long else
//	else echo B; echo tail; fi     → B, tail    so its body is a list
//	else echo B                    → parse error near `\n`, wanting the `fi`
//	else { echo B }                → B          the short arm, and no `fi`
//	else { echo B } fi             → parse error near `fi`, there being none
//	else ⏎ { echo B }              → B          a newline does not decide it
//	elif true; then echo C; fi     → C          the arm is a long elif
//	elif (( 1 )) ⏎ then echo C; fi → C          so is one whose body moved
//	elif (( 1 )) { echo C } else echo D; fi → C   and the two mix either way
//
// So the `{` is what says short, an `else` that does not open one is the long
// arm, and #1372's report — an `else` with nothing after it being accepted —
// is that rule arriving at the end of the input with the `fi` still owed.
// Written as "an empty short arm is an error" it would have taken the first
// row above as an error too.
func (p *Parser) shortElse(c *IfClause) {
	if p.bodyTookTerm.IsValid() {
		// The arm's body took the separator that would have ended the `if`,
		// so the `if` ended with it and there is no arm left to write: `if
		// (( 1 )) echo A; else echo B` is `parse error near \`else\`` on zsh
		// 5.9.2 with or without a `fi` after it, and so is the same shape
		// after an `elif`. It is the rule already stated in shortIf's doc —
		// the `;` ends the whole command and leaves `else` nothing to attach
		// to — read from the other side. A newline in place of the `;`
		// arrives here too and needs no test: it is still in hand, so the
		// `else` on the next line is not the token we are looking at.
		return
	}
	for p.atWord("elif") && p.err == nil {
		e := &Elif{Start: p.tok.Pos}
		p.opensClause("elif")
		p.next()
		e.Cond = p.parseCondition()
		body, stop, short := p.shortIf(e.Cond)
		if !short {
			// This arm is the long form: a condition that did not end
			// itself, or one that did with the body on the next line. Either
			// way a `then` is owed here and a `fi` at the end, so the rest of
			// the chain belongs to longIfTail.
			p.requireSep("then")
			p.expectWord("then")
			e.Then = p.parseBody()
			c.Elifs = append(c.Elifs, e)
			p.longIfTail(c)
			return
		}
		e.Then, c.Stop = body, stop
		c.Elifs = append(c.Elifs, e)
		if p.bodyTookTerm.IsValid() {
			return
		}
	}
	if !p.atWord("else") || p.err != nil {
		// The chain ended with a short arm and no `else`, which is the one
		// shape that may be closed with a `fi` written anyway.
		p.redundantFi(c)
		return
	}
	p.opensClause("else")
	p.next()
	c.HasElse = true
	// Past the newlines before asking, and only here: a newline between a
	// *condition* and its body is what puts an `if` or an `elif` in the long
	// form, but an `else` has no condition for one to end, and the shell
	// reads `else` ⏎ `{ echo B }` as the short arm — no `fi` is owed and
	// appending one is refused. So the brace decides the form and the
	// newlines in front of it do not.
	p.skipNewlines()
	if p.braceBodyFollows() {
		c.Else, c.Stop = p.shortFormBody()
		return
	}
	c.Else = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("fi")
}

// redundantFi reads the `fi` a short `if` with no `else` may be closed with,
// where there is one standing.
//
// It is optional and it is narrow in four directions at once. Measured on zsh
// 5.9.2, 2026-09-13, over `-c` and a script file alike:
//
//	if (( 1 )) { echo A } fi                        → A
//	if (( 1 )) { echo A } elif (( 1 )) { echo C } fi → A     an elif chain too
//	if (( 1 )); then echo A; elif (( 1 )) { echo C } fi → A  the last arm decides
//	if (( 1 )) { echo A } fi fi                     → parse error near `fi'
//	if (( 1 )) { echo A } else { echo B } fi        → parse error near `fi'
//	if (( 1 )) { echo A } ; fi                      → parse error near `fi'
//	if (( 1 )) { echo A } ⏎ fi                      → parse error near `fi'
//	if (( 1 )) (( 2 )) fi                           → parse error near `fi'
//	while (( 0 )) { : } done                        → parse error near `done'
//
// So: exactly one, only where the chain ended with a **brace** arm, only where
// it has no `else`, only with nothing between the `}` and the word — a `;` or
// a newline refuses it, which is why nothing here skips either — and only for
// `if`, the short loop forms taking no `done`. The last two rows are the
// controls that say this is not "a short form may be closed with its keyword".
//
// The `else` is the row to be careful with, and it is the caller that keeps it:
// this is reached only where the chain ended with no `else` at all, so a `fi`
// accepted unconditionally after the chain would take a line zsh refuses. #2242.
func (p *Parser) redundantFi(c *IfClause) {
	if c.HasElse || !p.shortBodyBraced || !p.atWord("fi") {
		return
	}
	c.Stop = p.tok.End
	p.next()
	// One and not a run: a second `fi` is a parse error there, and the flag
	// is what a second call would read, so it goes down with the first.
	p.shortBodyBraced = false
}

// condEndedItself reports whether the condition just parsed closed on its own
// — `(( … ))` and `[[ … ]]` do, a word does not.
//
// The list is what says so rather than the token that follows it: the parser
// stopped where it stopped because nothing could continue the condition, and
// the only lists that can be followed straight by a body are the ones whose
// last command was a construct with its own end. A simple command swallows
// what comes after it as another word, which is why `if true { … }` is a
// syntax error in the shell that takes `if [[ -n x ]] { … }`.
func condEndedItself(cond []*Stmt) bool {
	if len(cond) == 0 {
		return false
	}
	// Nothing here tests for a separator, and it looked as though something
	// should: `if [[ -n x ]]; { echo A }` is an error where the same line
	// without the `;` runs. It cannot reach here. A separator keeps the
	// condition *list* going, so whatever follows becomes another statement
	// of it and is the one this looks at — a brace group, which does not end
	// a header — and a separator with nothing after it leaves the parser at
	// end of input or at a stop word, which shortIf refuses before asking.
	// A guard for it would be a line no test could distinguish, which is how
	// it was found: removing it changed nothing anywhere.
	return exprEndsItself(cond[len(cond)-1].Expr)
}

// exprEndsItself walks to the command the condition finished on: an and-or
// list finishes on its right side and a pipeline on its last command.
//
// A pipeline was recorded as never ending itself, from `if [[ -n x ]] | cat
// { … }` being a syntax error — but that is `cat` failing to end it, not the
// pipe. `if true | [[ -n x ]] { … }` runs, which is the row that tells the
// two apart, and it was found by mutation rather than by a script.
func exprEndsItself(e Expr) bool {
	switch x := e.(type) {
	case *BinaryExpr:
		return exprEndsItself(x.Y)
	case *Pipeline:
		if len(x.Cmds) == 0 {
			// A bare negation — a `!` the dialect lets stand with no
			// pipeline after it. It ends nothing, and asking it for a last
			// command took the parser down: `if !; then :; fi` is `F` in
			// bash 5.3.20, zsh 5.9.2 and ksh93u+ alike (#3721).
			return false
		}
		return commandEndsItself(x.Cmds[len(x.Cmds)-1])
	}
	return false
}

// commandEndsItself is the whole of the rule: `(( … ))` and `[[ … ]]` close,
// and a word does not.
//
// The set is what was measured rather than what is tidy. A group, a subshell
// and a `case … esac` close too, and a *loop* does not — `if for i in a; do
// true; done { … }` is a syntax error in the shell that takes every other
// row, which is a fact about that shell rather than a rule anyone could
// derive. A pipeline of more than one command is refused by exprEndsItself
// above, for the same measured reason.
//
// A try-always block closes as well, measured 2026-09-07: `if { true; } always
// { :; } { echo A; }; echo after` prints A and after in zsh 5.9.2, where
// leaving the construct out of the set would call the body's `{` a syntax
// error. The trailing statement is not decoration — a command string whose
// last byte is the `}` of a short body is a parse error there whatever
// precedes it, so a probe without one cannot tell the readings apart.
func commandEndsItself(c Command) bool {
	switch c.(type) {
	case *TestClause, *ArithCmdClause, *Group, *Subshell, *CaseClause, *TryClause:
		return true
	}
	return false
}

func (p *Parser) parseLoop() Command {
	c := &LoopClause{Until: p.atWord("until"), Start: p.tok.Pos}
	defer p.opens(loopWord(c.Until))()
	p.next()
	p.inCondition = true
	c.Cond = p.requireBody(p.parseList())
	p.inCondition = false
	// Where the body may be short, the condition list is the whole header and
	// it has just ended: what stands here is either `do`, or the body, or
	// nothing. The list is what decides — a `;` kept it going, so anything
	// after one was tested rather than run.
	if p.dialect.ShortForm && !p.atWord("do") {
		c.Body, c.Stop = p.shortFormBody()
		return c
	}
	p.requireSep("do")
	// A `do` inside a while or until is a keyword awaiting its own partner,
	// and inside a `for` it is not — measured, in the one shell whose wording
	// can tell: `while true; do echo x` is "`do' unmatched" there and
	// `for i in a; do echo x` is "`for' unmatched". An irregularity in that
	// shell rather than in this one, recorded rather than smoothed over.
	p.opensClause("do")
	p.expectWord("do")
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

func (p *Parser) parseFor() Command {
	start := p.tok.Pos
	defer p.opens("for")()
	if p.dialect.CStyleFor && p.peekIsArithCmd() {
		return p.parseForArith(start)
	}
	c := &ForClause{Start: start}
	p.next()
	name, refused, ok := p.forName("for")
	if !ok {
		return c
	}
	if refused != "" {
		c.RefusedName = refused
	} else {
		c.Names = []string{name}
	}
	nameEnd := p.tok.End
	p.next()
	nameEnd = p.moreForNames(c, nameEnd, "for")
	p.skipNewlines()

	end := nameEnd
	// An absent word list is not an empty one: without `in` the loop iterates
	// the positional parameters, and with `in` and nothing after it, nothing.
	// The parenthesized spelling is the short loop's and says the same thing
	// as `in`, with the closing paren ending the header where `in` needs a
	// separator to.
	parens := p.shortItemsFollow()
	switch {
	case parens:
		c.HasItems = true
		c.Items, end = p.shortItems()
	case p.atWord("in"):
		c.HasItems = true
		end = p.itemList(&c.Items, end)
	}
	c.Header = p.slice(c.Start, end)
	if body, stop, short := p.shortBodyAfterHeader(parens || !c.HasItems); short {
		c.Body, c.Stop = body, stop
		return c
	}
	p.requireSep("do")
	// A brace group where `do … done` stands. The separator `requireSep` has
	// just consumed is what makes the form reachable at all: with nothing
	// between, `{` is another *item* of the list, and the loop then meets `}`
	// where `do` belongs — which is why `for i in a b { … }` is refused by
	// every shell that accepts `for i in a b; { … }`.
	if p.braceBodyFollows() {
		c.Body, c.Stop = p.braceLoopBody()
		return c
	}
	if p.dialect.ShortForm && !p.atWord("do") {
		c.Body, c.Stop = p.shortFormBody()
		return c
	}
	p.expectWord("do")
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

// itemList reads the word list after `in` — for a `for` loop, for `select`
// and for `foreach` alike — and returns where the last word ends.
//
// **No word in the list is a reserved word.** The list ends at a `;` or a
// newline and at nothing else, which is unanimous across the panel. Measured
// 2026-09-06, `env -i PATH=/usr/bin:/bin` with a scratch HOME and `-n` over a
// script file:
//
//	for x in a b do :; done       refused by all five — `do` is a *word*
//	                              there, so `done` stands where `do` belongs
//	for x in do; do :; done        taken by all five
//	for x in done; do :; done      taken by all five
//	select x in a b do :; done     refused by all five
//	foreach x in a b end           taken by the shell that has the loop
//
// This read `do`, `done` and the rest of the reserved words as stop words
// here, which got both directions wrong at once: it accepted the first line,
// where every shell refuses it, and refused the next two, which every shell
// takes. A list holding the word `done` is not exotic — `for f in $(ls)`
// reaches it the moment a file is called that — and the accepted-malformed
// half is the worse one, because the body then runs with `do` bound as a
// value and nothing is said.
//
// One reader for all three loops, because the three headers are the same
// grammar and each had its own copy of it (#1161).
func (p *Parser) itemList(items *[]*Word, end Pos) Pos {
	// Set before the `p.next()` that reads the first word, because that is
	// the token the flag has to reach. Each word stands where an argument
	// does, so a `(` that begins one belongs to it in the dialect that reads
	// glob qualifiers — the answer an array literal's element already gets
	// (#1149). Restored rather than cleared: a loop header is read at
	// command position, and the token after the list is not an argument
	// either way.
	saved := p.lex.inArgument
	p.lex.inArgument = true
	p.next()
	for p.tok.Kind == TokWord && p.err == nil {
		w := p.word()
		// Nor is a loop header's word, in the same dialect and for the same
		// reason: `for i in <(:)` and `select i in <(:)` are refused while
		// reading where `for i in a; do echo <(:); done` — the body, which
		// is commands — is not (#930).
		if p.refuseProcSubstOutOfPlace(w) {
			break
		}
		*items = append(*items, w)
	}
	p.lex.inArgument = saved
	if n := len(*items); n > 0 {
		end = (*items)[n-1].End()
	}
	return end
}

// forName reads the word standing where a loop's variable belongs and reports
// whether it may be one, recording the refusal where it may not.
//
// **A name may not come out of an expansion, and that is unanimous.** `n=x;
// for $n in a b` is refused by bash 5.3.15, the same binary as `sh`, bash
// 3.2.57, dash, ksh93u+ and zsh 5.9.2 alike — measured 2026-09-06, `env -i
// PATH=/usr/bin:/bin` with a scratch HOME, from a script file and through `-c`.
// So is `${n}`, `"$n"` and `$(echo n)`. The loop would otherwise bind a
// variable literally called `n`, the script's own `$n` would read the list's
// words, and the name it meant to reach through the expansion would stay
// empty — at status 0 and with nothing said (#1076).
//
// The token's literal cannot see it: a `$n` word reports `n`, so [isName] is
// satisfied by a word that names nothing yet. What tells them apart is the
// *spans* — a substitution is a span of its own kind — and asking that way
// rather than by comparing the source text is what keeps the two halves below
// separate, because quoting changes the source text too.
//
// **Whether the word may be quoted at all is an axis**, [Dialect.ForNameMayBeQuoted]:
// ksh93 removes the quoting and takes the name, where bash, dash and zsh want
// the name written plainly. Measured on `for "i"`, `for 'i'`, `for i""`,
// `for "i"x` and `for \i` — ksh93 runs all five and the other three refuse all
// five, so the escape travels with the quotes rather than being its own
// question.
//
// The name is reported **as written**, quotes, escapes, expansion and all:
// three of the four dialects quote the source text back and none of them
// quotes the literal. `for $n` names `$n` and not `n`.
func (p *Parser) forName(word string) (name, refused string, ok bool) {
	if p.forNameIsUsable() {
		return p.tok.Literal(), "", true
	}
	if p.dialect.ForNameCheckedWhenTheLoopRuns && p.tok.Kind == TokWord {
		// The parse succeeds and the word is carried to the interpreter,
		// which is the whole of #1110: `bash -n` accepts this, so refusing
		// it here reported a working script as broken.
		//
		// A *word* only. `for ; in a b` is an ordinary unexpected-token
		// failure in both shells that get here — status 2 and 3, with bash
		// echoing the line — because what they want in that position is a
		// word, and the name is checked afterwards. So the two questions
		// stay apart: whether a word may stand there is the grammar's, and
		// whether the word is a name is the loop's.
		return "", p.forNameAsWritten(), true
	}
	if p.tok.Kind != TokWord && !p.dialect.ForNonWordIsANameError {
		// Nothing that could be a name stands here at all, and the grammar
		// gets to complain about the token before the loop gets to complain
		// about the name. `for`, a newline after `for`, `for ;` — three of
		// the four dialects call each of those what they call the same token
		// anywhere else, so the failure is an ordinary one and the expected
		// word is the one the header is heading for.
		//
		// It reached the name check with the end of input in hand instead,
		// which named `end of input` as though a script had written it and
		// carried ForNameStatus rather than the dialect's parse-error status
		// — 1 where bash answers 2 and ksh93 answers 3 (#1319).
		//
		// The end of the input is a token like any other here, and goes
		// through failUnexpected for the same reason every other construct's
		// does: it is that call which knows an unterminated construct from an
		// unwanted token, and the two are worded differently by every shell
		// in the panel.
		//
		// Except where the input's end *is* a newline, which is one dialect's
		// answer rather than a third classification: the token it names is the
		// newline it appended, at the position the text ran out. Built here
		// rather than lexed, because nothing else in the grammar could see it
		// — every other position takes a newline and would consume this one.
		if p.tok.Kind == TokEOF && p.dialect.EndOfInputIsANewlineWhereNoneCouldStand {
			p.failUnexpectedAt(Token{Kind: TokNewline, Pos: p.tok.Pos}, "do", false)
			return "", "", false
		}
		p.failUnexpected("do")
		return "", "", false
	}
	if p.err == nil {
		p.err = &Error{
			Pos: p.tok.Pos, Kind: ErrForName,
			Token: p.forNameAsWritten(), Class: p.tokenClass(false),
			Msg: "expected a name after `" + word + "`",
		}
	}
	return "", "", false
}

// forNameIsUsable is the predicate.
//
// A token that is not a word needs no test of its own: only a word has spans,
// so its literal is empty and [isName] refuses it. A guard for it here read as
// if it decided something and did not — the mutant that removed it survived,
// which is the whole of the evidence that it was dead.
//
// What a non-word in that position *should* say is a separate answer and not a
// second predicate. `for ; in a b` is an ordinary unexpected-token failure in
// bash and ksh93 — status 2 and 3, with bash echoing the line — because those
// two want only a word there and check the name when the loop runs; dash gives
// it the same one sentence it gives every bad loop variable, and zsh's two
// wordings coincide. That is the stage question of #1110 showing through, and
// modeling it as its own axis would be modeling the symptom.
func (p *Parser) forNameIsUsable() bool {
	for _, sp := range p.tok.Spans {
		if sp.Kind != Literal {
			// A name out of a substitution: refused everywhere.
			return false
		}
	}
	if !p.dialect.ForNameMayBeQuoted && p.forNameAsWritten() != p.tok.Literal() {
		// Written with quotes or an escape in it, in a dialect that wants
		// the name plain. Comparing the source text with the literal is what
		// says so for both spellings at once.
		return false
	}
	if p.dialect.ForNameMayBeAPositionalParameter && isAllDigits(p.tok.Literal()) {
		// A positional parameter's number, which is not a name and is a loop
		// variable in one dialect. See
		// [Dialect.ForNameMayBeAPositionalParameter].
		return true
	}
	return isNameIn(p.tok.Literal(), p.dialect.DottedName)
}

// isAllDigits reports whether s is one or more decimal digits and nothing
// else, which is the whole of what a positional parameter is named by.
//
// Not a number: `01` is taken and is the first parameter, so the digits are
// read as a name rather than parsed as a value.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// forNameAsWritten is the word's source text, which is what a diagnostic
// quotes and what the plainness check above compares against.
//
// Read from the token rather than from the input where an alias put it
// there. A spliced token carries the position of the *word it replaced* —
// see spliceAlias — so slicing the input for one gives the alias's name back
// however the body was written, and the plainness check then compared a loop
// variable against the word that expanded to it. `alias f='for i in 1; do
// echo hi; done'; f` was refused for a loop variable named after the alias
// here, and is a loop in bash, ksh93 and zsh (#2299).
func (p *Parser) forNameAsWritten() string {
	if p.tok.Kind != TokWord {
		return p.tokenLiteral()
	}
	if p.aliasSpliced > 0 {
		return p.tok.Text
	}
	return p.slice(p.tok.Pos, p.tok.End)
}

// moreForNames reads the names after the first, where the dialect lets a loop
// have more than one. It returns where the last of them ends.
//
// **Greedy, and that is the whole of the ambiguity.** Every word after the
// first is another name until the header ends — at `in`, at `(`, at a
// separator, at `do`, or at `{` — so nothing else may stand there. Measured
// 2026-09-06 in zsh 5.9.2, the only shell with the form:
// `set -- p q; for a print -r -- "[$a]"` is a parse error near `-r`, because
// `print` was taken as a second name and `-r` is not a name; and
// `set -- p q; for a echo; print "[$a][$echo]"` prints `[p][q]`, which is the
// same reading seen from the side where it succeeds — `echo` was read as a
// name and bound to `q`.
//
// So the flag narrows what the dialect accepts as well as widening it: a short
// body may no longer follow the names *directly*. It still follows a header
// that ended itself, which is every spelling anyone writes —
// `for a b ( 1 2 ) print "$a$b"` and `for a b; print "$a$b"` both run.
//
// A name is a plain unquoted name and is not expanded: `for a "b" ( … )` and
// `for a $n ( … )` are parse errors in zsh, so anything that is not one is
// refused where it stands rather than quietly becoming a body.
//
// The source is compared with the token's literal to say so, because the
// literal alone cannot: a `$n` word reports `n`, so `isName` is satisfied by a
// word that names nothing yet. The *first* name has the same hole and it is
// not closed here — `for $n in a b` is refused by all five shells in the panel
// and accepted by every dialect of ours, which is a core bug of its own with
// five wordings to get right (#1076).
//
// `select` never calls this. Measured, `select a b (x y) { … }` is a parse
// error in the shell that accepts every other spelling here, so the loop whose
// header is otherwise a for-loop's parts company over exactly this.
func (p *Parser) moreForNames(c *ForClause, end Pos, word string) Pos {
	if !p.dialect.ForMultipleNames {
		return end
	}
	for p.tok.Kind == TokWord && !p.atStopWord() && !p.atWord("in") && !p.atWord("{") {
		name, refused, ok := p.forName(word)
		if !ok {
			return end
		}
		if refused != "" {
			// The **first** refused word is the one kept, because that is
			// the one a complaint will quote and a later bad word would
			// otherwise displace it silently.
			//
			// No shell in the panel is both of these — multiple names are
			// zsh's and zsh refuses the word while parsing — so the answer
			// is this repository's own rather than a measurement, and it is
			// written down in a test against a dialect assembled for it. A
			// mutant that let the later word win survived every row until
			// that test existed.
			if c.RefusedName == "" {
				c.RefusedName = refused
			}
			end = p.tok.End
			p.next()
			continue
		}
		c.Names = append(c.Names, name)
		end = p.tok.End
		p.next()
	}
	return end
}

// shortItemsFollow reports whether a `for` or `select` header's word list is
// written in parentheses.
func (p *Parser) shortItemsFollow() bool {
	return p.dialect.ShortForm && p.at(TokLeftParen)
}

// shortItems reads that list. The words are the ordinary ones — expanded,
// split and globbed like the words after `in` — and an empty list is legal
// and iterates nothing.
func (p *Parser) shortItems() (items []*Word, end Pos) {
	// The same two answers the `in` list gets, and for the same reasons: no
	// word in the list is a reserved word — `for x (do)`, `for x (done)` and
	// `for x (a do)` are all taken by the shell that has the form — and each
	// word stands where an argument does, so `for x ((#i)a)` reads the flag.
	// Measured 2026-09-06 on zsh 5.9.2; see itemList, which cannot be shared
	// here because this list is closed by a paren rather than by a separator.
	saved := p.lex.inArgument
	p.lex.inArgument = true
	p.next() // (
	p.skipNewlines()
	for p.tok.Kind == TokWord && p.err == nil {
		items = append(items, p.word())
		p.skipNewlines()
	}
	p.lex.inArgument = saved
	end = p.tok.End
	if !p.at(TokRightParen) {
		p.failUnexpected(")")
		return items, end
	}
	p.next()
	return items, end
}

// shortBodyAfterHeader reads the body of a `for` or `select` whose header
// ended itself, where the dialect allows one without `do … done`.
//
// The separator is optional there rather than required, which is the whole
// difference from the long form: `for i (a b) echo $i` and `for i (a b); echo
// $i` are the same loop. It reports false where the long form still applies,
// so a header that did not end itself, a dialect without the flag, and a `do`
// standing where it always could all fall through untouched.
func (p *Parser) shortBodyAfterHeader(headerEnded bool) (body []*Stmt, stop Pos, short bool) {
	if !p.dialect.ShortForm || !headerEnded {
		return nil, Pos{}, false
	}
	if p.tok.Kind == TokSemi || p.tok.Kind == TokNewline {
		p.next()
		p.skipNewlines()
	}
	if p.atWord("do") {
		return nil, Pos{}, false
	}
	body, stop = p.shortFormBody()
	return body, stop, true
}

// parseRepeat reads `repeat N` and its body.
//
// The header is one word and ends itself, so every body spelling the dialect
// has is reachable: `do … done`, a brace group, one command, and — with a
// separator between, which a `while` reads as more condition and this reads
// as nothing at all — the same three again.
func (p *Parser) parseRepeat() Command {
	c := &RepeatClause{Start: p.tok.Pos}
	defer p.opens("repeat")()
	p.next()
	c.Count = p.word()
	if c.Count == nil {
		p.failUnexpectedOperand("a count")
		return c
	}
	c.Header = p.slice(c.Start, c.Count.End())
	// A separator here is optional and belongs to the header rather than to
	// a condition list, because there is no condition: `repeat 2; echo R`
	// prints twice, where `while cond; echo R` would have tested the echo.
	if p.tok.Kind == TokSemi || p.tok.Kind == TokNewline {
		p.next()
		p.skipNewlines()
	}
	if p.braceBodyFollows() {
		c.Body, c.Stop = p.braceLoopBody()
		return c
	}
	if p.dialect.ShortForm && !p.atWord("do") {
		c.Body, c.Stop = p.shortFormBody()
		return c
	}
	p.opensClause("do")
	p.expectWord("do")
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

// parseForeach reads `foreach name (a b) … end`, which is a `for` under two
// other words.
//
// The tree is a ForClause, because the construct is one: the same loop
// variable, the same list, the same body, and `break` and `continue` reaching
// the same place. Only the words differ, and a printer that writes it back as
// `for … do … done` writes something every dialect can read.
func (p *Parser) parseForeach() Command {
	c := &ForClause{Start: p.tok.Pos}
	defer p.opens("foreach")()
	p.next()
	name, refused, ok := p.forName("foreach")
	if !ok {
		return c
	}
	if refused != "" {
		c.RefusedName = refused
	} else {
		c.Names = []string{name}
	}
	end := p.tok.End
	p.next()
	end = p.moreForNames(c, end, "foreach")
	p.skipNewlines()
	switch {
	case p.at(TokLeftParen):
		c.HasItems = true
		c.Items, end = p.shortItems()
	case p.atWord("in"):
		c.HasItems = true
		end = p.itemList(&c.Items, end)
	}
	c.Header = p.slice(c.Start, end)
	// The body has three spellings and `end` is only one of them. Measured
	// 2026-09-15 on zsh 5.9.2, each probe in a script file of its own and
	// each printing both passes:
	//
	//	foreach c (a b); do … done    the keyword body, separator or not
	//	foreach c (a b) do … done
	//	foreach c (a b) { … }         a brace group, no separator needed
	//	foreach c (a b); … end        the word's own closer
	//	foreach c (a b) … end
	//	foreach c in a b; do … done   the `in` list with a keyword body
	//	foreach c (a b); do … end     refused — the closers pair
	//
	// The parenthesized list ends the header itself, which is why the brace
	// needs no separator in front of it where `for i in a b { … }` is
	// refused by every shell that takes `for i in a b; { … }`.
	if p.braceBodyFollows() {
		c.Body, c.Stop = p.braceLoopBody()
		return c
	}
	if p.tok.Kind == TokSemi || p.tok.Kind == TokNewline {
		p.next()
		p.skipNewlines()
	}
	if p.braceBodyFollows() {
		c.Body, c.Stop = p.braceLoopBody()
		return c
	}
	if p.atWord("do") {
		p.next()
		c.Body = p.parseBody()
		c.Stop = p.tok.End
		p.expectWord("done")
		return c
	}
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("end")
	return c
}

// parseSelect reads the menu loop, whose header is a for-loop's.
func (p *Parser) parseSelect() Command {
	c := &SelectClause{Start: p.tok.Pos}
	defer p.opens("select")()
	p.next()
	name, refused, ok := p.forName("select")
	if !ok {
		return c
	}
	if refused != "" {
		c.RefusedName = refused
	} else {
		c.Name = name
	}
	nameEnd := p.tok.End
	p.next()
	p.skipNewlines()
	end := nameEnd
	// The menu loop's header is a for-loop's, parenthesized list included.
	parens := p.shortItemsFollow()
	switch {
	case parens:
		c.HasItems = true
		c.Items, end = p.shortItems()
	case p.atWord("in"):
		c.HasItems = true
		end = p.itemList(&c.Items, end)
	}
	c.Header = p.slice(c.Start, end)
	if body, stop, short := p.shortBodyAfterHeader(parens || !c.HasItems); short {
		c.Body, c.Stop = body, stop
		return c
	}
	p.requireSep("do")
	// The menu loop takes the brace body its header's loop takes.
	if p.braceBodyFollows() {
		c.Body, c.Stop = p.braceLoopBody()
		return c
	}
	if p.dialect.ShortForm && !p.atWord("do") {
		c.Body, c.Stop = p.shortFormBody()
		return c
	}
	p.expectWord("do")
	c.Body = p.parseBody()
	c.Stop = p.tok.End
	p.expectWord("done")
	return c
}

func (p *Parser) parseCase() Command {
	c := &CaseClause{Start: p.tok.Pos}
	defer p.opens("case")()
	// The subject stands at what is otherwise command position and is not an
	// assignment: measured, `case a==(echo hi) in *)` matches with no
	// command run and no file made, in the dialect where a bare
	// `a==(echo hi)` assigns a path. Told to the
	// lexer before the `p.next()` that reads it, for the reason inCaseArm is
	// below. See Lexer.noAssignment.
	// A `(` at the front of the subject is text in one dialect and an
	// operator in the rest, and the lexer cannot tell on its own: `(` is in
	// the operator table, so a token beginning with one never reaches the
	// word scanner. Told here for the same reason noAssignment is, and taken
	// back on the same line. See Lexer.inCaseSubject.
	p.lex.noAssignment, p.lex.inCaseSubject = true, true
	p.next()
	p.lex.noAssignment, p.lex.inCaseSubject = false, false
	if c.Word = p.word(); c.Word == nil {
		// The subject is a word position, so the dialect that reads a
		// run-out there as a newline reads this one too. See inCaseWord.
		p.inCaseWord = true
		defer func() { p.inCaseWord = false }()
		// The ordinary unexpected-token refusal, because that is what every
		// column answers here: a sentence of this parser's own would be the
		// one place a `case` subject is refused in words no shell uses.
		// Measured 2026-09-19 on `case ; in x) ;; esac` — bash 5.3.20
		// ``syntax error near unexpected token `;'``, zsh 5.9.2 ``parse
		// error near `;'``, ksh93u+ ``syntax error at line 1: `;'
		// unexpected``, dash 0.5.12 `";" unexpected (expecting word)`,
		// BusyBox ash 1.37.0 `unexpected ";" (expecting word)`. The status
		// was already each column's own; only the sentence was ours.
		p.failUnexpectedWord()
		return c
	}
	// The subject stands where no command takes a word, so one dialect
	// refuses a process substitution in it while reading (#930).
	if p.refuseProcSubstOutOfPlace(c.Word) {
		return c
	}
	p.skipCaseHeaderSeparators()
	inEnd := p.tok.End
	// An arm begins where no command may, so `((` there is the arm's own
	// paren in front of a group rather than an arithmetic command, and a
	// leading `(` may belong to the pattern. Told to the lexer before the
	// token is read — and `expectWord` is what reads it, the first arm's
	// first token arriving with the `in` — and taken back before the arm's
	// *body* is read, a body being ordinary commands in which `((1))` really
	// is an expression. See Lexer.inCaseArm.
	p.lex.inCaseArm = true
	defer func() { p.lex.inCaseArm = false }()
	braced := false
	if p.dialect.CaseBraceBody != NoCaseBraceBody && p.atWord("{") {
		// The brace spelling of the same header. Which words may close it is
		// the dialect's next answer: one shell pairs the two and the other
		// takes either closer after either opener, so which opener was read
		// has to be carried. See Dialect.CaseBraceBody.
		braced = true
		p.next()
	} else {
		p.expectWord("in")
	}
	c.Header = p.slice(c.Start, inEnd)
	// Where the dialect says so, an `esac` standing here — after the `in`,
	// before any newline — is the first arm's first pattern and not the
	// terminator. A newline takes the reading away again, so this is asked
	// before the newlines are skipped and cleared if any were.
	// See Dialect.CaseTerminatorIsAPatternAfterTheHeader.
	esacIsAPattern := p.dialect.CaseTerminatorIsAPatternAfterTheHeader && !p.at(TokNewline)
	p.skipCaseHeaderSeparators()

	// Only the word `esac` is taken away by that reading, and never the `}`:
	// the shell with both takes `case x { }` as an empty `case` and reads
	// `case x { esac` as an arm whose pattern is `esac`. Measured.
	atEnd := func() bool {
		if esacIsAPattern && p.atWord("esac") {
			return false
		}
		return p.atCaseEnd(braced)
	}
	for p.err == nil && !p.at(TokEOF) && !atEnd() {
		esacIsAPattern = false
		p.lex.inCaseArm = false
		// The arm before this one closed, so its terminator is no longer the
		// innermost thing anything is inside: measured, `case x in x) : ; ;;
		// y) : ;` names the `case` where the same text without the second arm
		// names the `;;`. See Parser.terminatorStood.
		p.terminatorStood, p.armTerminator = Pos{}, ""
		it := &CaseItem{Start: p.tok.Pos}
		// A pattern may carry a leading open paren. Where the paren opened
		// the *pattern* instead the lexer has already folded it into the
		// word, so this sees no paren at all and the pattern arrives whole.
		//
		// Each pattern stands where an argument does either way, so a `(`
		// beginning one belongs to it — `case x in ((a|b))` is the arm's
		// paren and then a group, and `case x in (a|b)|(c|d))` is two
		// groups and no arm paren. Set unconditionally rather than only on
		// the paren branch: the *second* alternative of a list is read by
		// `casePatterns` whichever way the first one arrived, and while
		// this was inside the branch a list whose first pattern was a group
		// left the rest of it in command position, where `(` is an operator
		// and `(a|b)|(c|d))` was `parse error near `(''`.
		saved := p.lex.inArgument
		savedList := p.lex.inCaseParenList
		p.lex.inArgument = true
		// From here to the `)` is the pattern position, which one dialect
		// reads the end of the input in as a newline. Cleared on every way
		// out below, the arm's *body* being ordinary commands where the end
		// of the input is the end of the input again.
		p.inCaseWord = true
		parenthesized := false
		if p.at(TokLeftParen) {
			if p.emptyParensStartAt(p.tok) {
				// `()` is one token to this dialect, so the `(` never opens
				// an arm: the pair is a word the grammar cannot take here
				// and the refusal names both characters. `( )` — the same
				// two with a blank between them — is an arm with an empty
				// pattern list and parses, which is what says the refusal
				// is the token's and not the emptiness's (#1111).
				p.lex.inArgument, p.lex.inCaseParenList = saved, savedList
				p.inCaseWord = false
				p.failUnexpected("")
				return c
			}
			// The paren is what puts one dialect's newline inside the
			// pattern rather than ending it. Only here: an arm written
			// *without* the paren is a parse error there too, so this is the
			// parenthesis's rule and not the position's. Set before the
			// `p.next()` that reads the first pattern, which is the token it
			// has to reach.
			p.lex.inCaseParenList = true
			parenthesized = true
			p.next()
		}
		if !p.casePatterns(it, parenthesized) {
			p.lex.inArgument, p.lex.inCaseParenList = saved, savedList
			p.inCaseWord = false
			return c
		}
		p.lex.inArgument = saved
		if !p.at(TokRightParen) {
			p.lex.inCaseParenList = savedList
			p.failUnexpectedOperand(")")
			p.inCaseWord = false
			return c
		}
		p.inCaseWord = false
		// Cleared before the read that follows, which is the arm's *body* —
		// ordinary commands, where a newline is a statement separator again.
		p.lex.inCaseParenList = savedList
		p.next()
		bodyFrom := p.tok.Pos
		it.Body = p.parseList()

		switch p.tok.Kind {
		case TokDSemi, TokSemiAmp, TokDSemiAmp, TokSemiPipe:
			it.Term, it.TermPos = p.tok.Kind, p.tok.Pos
			p.recordArmTerminator(it, bodyFrom)
			// The terminator's own `p.next()` reads the *next arm's* first
			// token, so the flag goes back on in front of it.
			p.lex.inCaseArm = true
			p.next()
		default:
			// The last arm may omit its terminator before `esac`.
			it.TermPos = p.tok.Pos
			if !atEnd() {
				if p.at(TokEOF) {
					// The panel expects `;;` here rather than `esac`: an arm
					// that has not been closed is what ran out, not the case.
					p.ranOut()
					if p.err == nil {
						p.err = p.unterminated(";;")
					}
					return c
				}
				// No expectation named: either `;;` or `esac` would be
				// valid here, so naming one of them would be inventing a
				// grammar the parser does not have — and the one dialect
				// that prints expectations does not print one here either.
				p.failUnexpected("")
				return c
			}
		}
		c.Items = append(c.Items, it)
		p.lex.inCaseArm = true
		p.skipNewlines()
	}
	p.lex.inCaseArm = false
	c.Stop = p.tok.End
	if atEnd() && p.atWord("}") {
		p.next()
	} else if braced && p.dialect.CaseBraceBody == CaseBraceBodyPairsWithItsOpener {
		// The opener was a `{` in a dialect that pairs them, so `esac` will
		// not do: the shell that draws it this way answers `` `case'
		// unmatched `` for `case x { … esac`, which is the unterminated
		// shape rather than a word in the wrong place.
		p.expectWord("}")
	} else {
		p.expectWord("esac")
	}
	return c
}

// atCaseEnd reports whether the current token closes a `case` that was opened
// the way braced says.
//
// Two words can, and only in a dialect that writes the brace spelling of the
// header — and whether they are interchangeable is that dialect's own answer:
// ksh93 pairs `{` with `}` and `in` with `esac`, while zsh takes either closer
// after either opener. See Dialect.CaseBraceBody.
func (p *Parser) atCaseEnd(braced bool) bool {
	switch p.dialect.CaseBraceBody {
	case CaseBraceBodyMixesWithTheKeyword:
		return p.atWord("esac") || p.atWord("}")
	case CaseBraceBodyPairsWithItsOpener:
		if braced {
			return p.atWord("}")
		}
	}
	return p.atWord("esac")
}

// casePatterns reads one arm's pattern list, `a | b | c`, and reports whether
// it got one. The caller has already consumed any open paren and stops at the
// close paren, which this does not read.
//
// Where the dialect allows it a pattern may be written as nothing, and the
// emptiness is read off the *separator* rather than off the position: a `|`
// with no word before it, after it, or on either side stands for a pattern
// that matches only the empty string. `(|a|b)` is the idiom for "one of these
// or none" and is what a real zsh library's own startup path is written with.
//
// The list may also be written as nothing at all, where the arm's parentheses
// are there to hold it: `( )` matches only the empty string in that dialect,
// measured 2026-09-12. `()` is a parse error there and this is why — the pair
// with no blank between is one token to that shell's lexer, so it never
// reaches the production at all (#1111). Without the parentheses there is
// nowhere for an empty list to be written and `case a in ) …` is refused.
func (p *Parser) casePatterns(it *CaseItem, parenthesized bool) bool {
	empty := p.dialect.CasePatternMayBeEmpty
	if empty && parenthesized && p.at(TokRightParen) {
		it.Patterns = append(it.Patterns, p.emptyPattern())
		return true
	}
	for {
		switch {
		case empty && (p.at(TokPipe) || p.at(TokOrOr)):
			it.Patterns = append(it.Patterns, p.emptyPattern())
		case p.dialect.CasePatternAcceptsOperator && !p.at(TokWord) && !p.at(TokEOF):
			// One operator may stand where a pattern belongs, and it
			// contributes no pattern at all — not even an empty one, which
			// is what separates this from the flag above. The two are never
			// set together, and the empty alternative is asked first because
			// its `|` is an operator too.
			p.next()
		default:
			w := p.word()
			if w == nil {
				p.failUnexpected("")
				return false
			}
			// And neither does an arm's pattern (#930).
			if p.refuseProcSubstOutOfPlace(w) {
				return false
			}
			it.Patterns = append(it.Patterns, w)
		}
		switch {
		case p.at(TokPipe):
			p.next()
		case empty && p.at(TokOrOr):
			// `||` is two separators with a pattern of nothing between,
			// and it arrives as one token because the lexer reads the
			// operator before anything has said this is a pattern list.
			it.Patterns = append(it.Patterns, p.emptyPattern())
			p.next()
		default:
			return true
		}
		if empty && p.at(TokRightParen) {
			it.Patterns = append(it.Patterns, p.emptyPattern())
			return true
		}
	}
}

// emptyPattern is the pattern written as nothing between two separators. It
// has no spans, so it expands to the empty string and matches only that.
func (p *Parser) emptyPattern() *Word {
	return &Word{Start: p.tok.Pos, Stop: p.tok.Pos}
}

// ParseParamExpFor parses the inside of a `${ }` that was captured outside the
// normal word path — a here-document body, which is read as raw text and
// expanded only when the command runs.
//
// The body is double-quoted context, which is what decides how the operand of
// a `${x:-word}` in it reads: `cat <<EOF` with `${u:-'$v'}` in the body prints
// `'VAL'` in every shell in the panel — the quotes two characters of the
// output and the `$v` between them still substituted, exactly as inside a
// pair of double quotes.
func (p *Parser) ParseParamExpFor(src string, at Pos) *ParamExpr {
	return p.parseParamExp(src, at, DoubleQuoted, false)
}

// ParseReference parses text that names a parameter and perhaps one of its
// elements — `x`, `x[@]`, `s[2]`, `a[(r)q]` — into the node every reading of
// a subscript already answers.
//
// It is the shape a *value* takes where a construct reads one as a reference
// at run time rather than at the parse: a `(P)` group's resolved text, and
// `unset`'s operand. Those arrive as strings with the subscript unlexed, so
// the group has nowhere to hang its operand and a range has no two ends —
// which is why the answer was a name plus one arithmetic index and nothing
// else.
//
// Unquoted, unlike ParseParamExpFor: a here-document body is inside quotes
// and a resolved value is not.
func (p *Parser) ParseReference(src string, at Pos) *ParamExpr {
	return p.parseParamExp(src, at, Unquoted, false)
}

// ParseArithFor is ParseParamExpFor for `$(( ))`.
func (p *Parser) ParseArithFor(src string, at Pos) ArithExpr {
	return p.parseArith(src, at)
}

// ParseArithExpanded is ParseArithFor for text that has **already** been
// expanded: a subscript and a substring's range, which are words their caller
// expands before anything arithmetic happens.
//
// The difference is what a `$` means. ParseArithFor is handed a program's
// text and leaves an expression with an expansion in it alone — nil, for the
// interpreter to expand and read when it runs — because `$(( $x$y ))` is the
// *result* of substituting. Text that arrives here has been through that
// already, so a `$` still standing in it is an ordinary character, and it
// begins no operand: reading it a second time ran what the first round had
// only produced, and `k='$(cmd)'; a[$k]=V` executed cmd (#3047).
//
// Measured 2026-09-15 from a script file, and unanimous in bash 5.3.20, bash
// 3.2.57, zsh 5.9.2 and ksh93u+ 2012-08-01: `$(cmd)`, a backquoted run,
// `$((1))` and a bare `$i` left in an expanded subscript are each refused for
// wanting an operand, and none of the four runs anything. A bare name is not
// one of them — `k='i'` and `k='1+1'` both name an element in all four —
// because resolving a name is the evaluator's job rather than a second round
// of expansion.
func (p *Parser) ParseArithExpanded(src string, at Pos) ArithExpr {
	return p.parseArithIn(src, at, true)
}

// readsBodyWithItsLine reports whether span is a substitution whose body this
// dialect parses while the line that holds it is read.
//
// The spelling decides, because the panel splits on it: bash 5.3 reads a
// `$( … )` body with the line and leaves the older spelling to the run, where
// dash and BusyBox ash read both. The current-shell spellings are outside it
// — no column that has them reads one with the line — and a body that ran out
// with the input is left alone, since there is no line for it to belong to
// yet. See [Dialect.SubstitutionBodyRead].
//
// **A process substitution's body is read the same way**, and it was left out
// while every column that reads one at all reads it. Measured 2026-09-20 from
// a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C bash s.sh` with standard
// input on the null device, bash 5.3.20, each line written after a
// `printf 'one\n'` and before a `printf 'two\n'`:
//
//	cat <(for)                              refused at the line, `two` absent
//	false && cat <(for)                     the same
//	false && cat <(v=$(echo hi; for))       the same
//	false && echo hi > >(for)               the same
//	false && echo hi > >(v=$(echo hi; for)) the same
//
// The `false &&` rows are the control that makes it a statement about
// *reading*: the substitution is never reached and the line is refused all
// the same. Without this the body was read only by the shell that runs it, so
// `while read -r l; do :; done < <(v=$(echo hi; for))` on the last line of a
// script wrote both of bash's messages and still left 0 (#3962).
//
// `=( … )` goes with them rather than being measured: no column that reads a
// body with its line has the spelling, so the branch is a statement of scope
// — a dialect that gained both would want the same answer — and nothing in
// the panel exercises it.
func (p *Parser) readsBodyWithItsLine(span Span) bool {
	switch span.Kind {
	case ProcSubstIn, ProcSubstOut, ProcSubstFile:
		return true
	case CommandSubst:
	default:
		return false
	}
	if span.CurrentShell {
		return false
	}
	if !span.Backquoted {
		return true
	}
	return p.dialect.SubstitutionBodyRead == EverySubstitutionBodyReadWithItsLine
}

// hiddenFromTheScriptsRead reports whether span exists only because a
// fragment of the script was read a second time under different quoting, so
// that the read of the line that holds it never saw the span at all.
//
// An expansion's operand inside double quotes is the case. The scan that
// finds the closing brace honors the quotes written between the braces; the
// operand is then read again with those quotes standing for themselves, and a
// `'` that stopped the first read stops nothing in the second. So `"${v-'$('}"`
// holds a command substitution on the second reading and none on the first,
// and a dialect that reads a body with its line still does not read that one.
//
// Measured 2026-09-19 against bash 5.3.20, from a script file under `env -i`,
// where the `$( … )` body *is* read with the line:
//
//	echo before; echo "${v-$(if)}"; echo after    refused, nothing written
//	echo before; echo "${v-'$(if)'}"; echo after  `before`, then a run-time
//	                                              complaint naming the
//	                                              substitution, then `after`
//	v=SET; echo "${v-'$('}"                       silence: never reached
//
// The second row is the discriminating one. Its body parses as readily as the
// first row's does, so a rule keyed on whether the body reads cannot tell the
// two apart; what separates them is only that the first read saw one opener
// and not the other.
func (p *Parser) hiddenFromTheScriptsRead(span Span) bool {
	if p.allHiddenFromTheScriptsRead {
		return true
	}
	at := int(span.Pos.Offset)
	for _, run := range p.quotedInTheScriptsRead {
		if at >= run[0] && at < run[1] {
			return true
		}
	}
	return false
}
