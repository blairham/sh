// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// The arithmetic expression tree, from docs/spec/grammar/arithmetic.md.
//
// Operators are held as their source spelling rather than as an enum. There
// are around thirty of them, they are already unambiguous strings, and every
// consumer either switches on them or prints them; a parallel set of constants
// would be a second thing to keep in step for no gain.

// ArithExpr is a node in an arithmetic expression.
type ArithExpr interface {
	Node
	arithNode()
}

// ArithNum is a numeric literal, kept as written.
//
// The text is not converted here, because what it means is a dialect
// question: a leading zero is octal everywhere but zsh, where `0100` is one
// hundred rather than sixty-four. Converting at parse time would bake one
// answer into the tree.
type ArithNum struct {
	Text  string
	Start Pos
	Stop  Pos
}

func (n *ArithNum) Pos() Pos   { return n.Start }
func (n *ArithNum) End() Pos   { return n.Stop }
func (n *ArithNum) arithNode() {}

// ArithVar is a bare name, which inside arithmetic is a variable reference.
// An unset one is zero rather than an error.
type ArithVar struct {
	Name  string
	Start Pos
	Stop  Pos
}

func (n *ArithVar) Pos() Pos   { return n.Start }
func (n *ArithVar) End() Pos   { return n.Stop }
func (n *ArithVar) arithNode() {}

// ArithCharCode is the character-code operator: `#name` is the code of the
// first character of that parameter's value, and `##c` the code of the
// character written out.
//
// Not a unary operator, because its operand is not an expression: `#b` reads
// the *parameter* b rather than b's arithmetic value, and `##a` reads a
// character where an expression would read a name. It is a primary, and the
// two spellings differ in how the operand is read rather than in what the
// result means.
type ArithCharCode struct {
	// Op is `#`, `##` or `''`, kept because it decides how Char is read:
	// `$((##\n))` is a newline and `$((#\n))` the letter n. `''` is the
	// character *constant* rather than an operator — `$(( 'a' ))` — and reads
	// its character the way `##` does, escapes and all.
	Op string
	// Name is the parameter whose first character is taken, empty when the
	// operand is a character or when there is no operand at all. A `#` with
	// nothing it can read is zero rather than a refusal, measured.
	Name string
	// Char is the character the operand spells out, as written — `a`, `\n`,
	// `\x41`. The escapes are decoded where Op says they are, which is done
	// against the dialect's table rather than here.
	Char string
	// Subscripted records a `[…]` written after the name. The shell reads it
	// and then finds nothing under the whole of it, so `$((#a[1]))` is zero
	// however `${a[1]}` reads — measured, and a fact about that shell rather
	// than a rule anything derives.
	Subscripted bool
	Start       Pos
	Stop        Pos
}

func (n *ArithCharCode) Pos() Pos   { return n.Start }
func (n *ArithCharCode) End() Pos   { return n.Stop }
func (n *ArithCharCode) arithNode() {}

// ArithOutput is the bracketed output-format specifier — `[#16]`, `[##16]`,
// `[#16_4]`, `[#_]` — which says the base the expression's *result* is
// written in and how its digits are grouped.
//
// One node, at the top of the tree, however many specifiers the text held and
// wherever they stood. That is not a simplification: the construct is lexical
// in the dialect that has it, so it takes effect from a branch that is never
// evaluated and from a position after the value it formats, and the textually
// last one wins. A node where the specifier was written would answer all three
// of those wrongly. See [Dialect.ArithOutputFormat] for the measurements.
//
// The value of the node is the value of X, unchanged — the format is a
// side-channel to whoever writes the answer down and is not arithmetic. It
// reaches two of them: the text an arithmetic expansion produces, and the text
// an assignment inside the expression stores.
type ArithOutput struct {
	// Base is the base the result is written in, and Based says one was
	// written at all: `[#_]` groups decimal digits and names no base, which
	// is not the same as naming zero — `[#0]` is a base out of range and is
	// refused. Two fields rather than a sentinel because the refusal has to
	// quote the number back.
	//
	// Whether a base is one the dialect can spell is not decided here: the
	// range is a property of the alphabet a shell renders in, so it is
	// checked where that alphabet is, and a base out of range is a *runtime*
	// failure rather than a parse one.
	Base  int
	Based bool
	// Prefixed says the `base#` is written in front of the digits, which is
	// the single `#` spelling: `[#16] 255` is `16#FF` and `[##16] 255` is
	// `FF`. Base ten writes no prefix under either spelling.
	Prefixed bool
	// Group is how many digits stand between `_` separators, and 0 for no
	// grouping. A bare `_` with no number after it is three.
	Group int
	// Text is the specifier as written, brackets included, for a diagnostic
	// that quotes it back.
	Text string
	// X is the expression, and nil when the text held nothing but the
	// specifier: `$(( [#16] ))` is `16#0`, not a failure.
	X     ArithExpr
	Start Pos
	Stop  Pos
}

func (n *ArithOutput) Pos() Pos   { return n.Start }
func (n *ArithOutput) End() Pos   { return n.Stop }
func (n *ArithOutput) arithNode() {}

// ArithUnary is a prefix or postfix operator.
type ArithUnary struct {
	Op string // + - ~ ! ++ --
	// Postfix distinguishes `x++` from `++x`, which differ in what they
	// evaluate to even though they have the same effect.
	Postfix bool
	X       ArithExpr
	Start   Pos
}

func (n *ArithUnary) Pos() Pos   { return n.Start }
func (n *ArithUnary) End() Pos   { return n.X.End() }
func (n *ArithUnary) arithNode() {}

// ArithBinary is any infix operator, including the sequence operator.
type ArithBinary struct {
	Op   string
	X, Y ArithExpr
	// YStart is the byte offset, within the expression's own text, where the
	// right operand was written — after the operator and after the blanks
	// behind it.
	//
	// It is here because the tree cannot answer the question a diagnostic
	// asks. A shell that blames a division by zero blames the *divisor as
	// written*, parentheses and sign included — `1/((0))` names `((0))` and
	// `1/-0` names `-0` — and the tree records what an operand is rather than
	// the characters it was spelled with, so a parenthesised divisor has no
	// text of its own to name and a signed one has the wrong text. Searching
	// the expression for the operand's leaf text answers neither, and answers
	// `0/0` with the dividend. The offset is the only thing that does.
	YStart int
}

func (n *ArithBinary) Pos() Pos   { return n.X.Pos() }
func (n *ArithBinary) End() Pos   { return n.Y.End() }
func (n *ArithBinary) arithNode() {}

// ArithCond is `c ? a : b`.
type ArithCond struct {
	Cond, Then, Else ArithExpr
}

func (n *ArithCond) Pos() Pos   { return n.Cond.Pos() }
func (n *ArithCond) End() Pos   { return n.Else.End() }
func (n *ArithCond) arithNode() {}

// ArithAssign is `x = e` and its compound forms.
//
// It is a separate node rather than a binary operator because its effect
// outlives the expression — `$((x=5))` leaves x set — which combined with
// short-circuiting makes evaluation order part of the specification.
// ArithIndex is `name[expr]` inside an arithmetic expression.
//
// The subscript is an expression of its own — `a[i+1]` — and neither the name
// nor the subscript is written with a `$`, which is what makes this a node
// rather than an expansion: `${a[1]}` is substituted before the expression is
// read, and `a[1]` is read as part of it.
type ArithIndex struct {
	Name string
	// Index is the subscript read as an expression, and nil when the text
	// will not read as one — `m[.accept-line]`, whose brackets hold a key and
	// not a sum. That is not a parse failure, because whether it is a key or
	// an expression depends on an attribute of the name, which the parser
	// cannot see: see Sub, and subscript. Nil also when the brackets held
	// nothing: see Empty.
	Index ArithExpr
	// Sub is the subscript exactly as it was written, brackets excluded.
	//
	// Kept beside the parsed expression because a subscript is only an
	// expression when the name is an indexed array. On an associative one it
	// is a key — `m[k]` is the element stored under the two characters, not
	// the one at whatever `k` evaluates to — and which of the two a name is
	// cannot be known until it runs. Measured unanimous in the three shells
	// with the attribute, whitespace included: `m[ k ]` is a different key
	// from `m[k]`, in an expression exactly as in `${m[ k ]}`.
	Sub string
	// Empty says the brackets were written with nothing at all between
	// them — `a[]` — which is what `a[$w]` becomes when `$w` holds the empty
	// string, because an arithmetic expansion substitutes its parameters
	// before the expression is read. Index is nil and Sub is empty then, and
	// neither of those alone would say it: a node with no subscript at all is
	// an ArithVar and never reaches here.
	//
	// Accepted by the grammar rather than refused, because every shell in the
	// panel with subscripts parses it and answers at run time — bash reports
	// a bad subscript and carries on with zero, ksh93 is silently zero, zsh
	// makes it depend on whether the name exists — and three run-time answers
	// are a semantics axis and not a parse error. It is refused in one
	// position still: an *assignment target*, `(( a[] = 9 ))`, where the same
	// three shells part again and part differently.
	Empty bool
	// Flags is the parenthesized flag group the subscript opened with, where
	// the dialect has them — `$(( a[(r)20] ))` is the element whose value is
	// `20`, exactly as `${a[(r)20]}` is.
	//
	// Index is nil when a group is present and Sub still holds the whole
	// subscript as written, group included, so a diagnostic quotes back what
	// the source said. The operand behind the group is Flags.Arg, and it is
	// a *literal* word: an arithmetic expression has already been expanded
	// once by the time it is parsed, so a `$` left in it is a `$` and not
	// the start of anything.
	//
	// The group is carried here rather than resolved in the parser for the
	// reason Sub is: what a group selects depends on whether the name is an
	// association, which the parser cannot see.
	Flags *SubscriptFlags
	Start Pos
	Stop  Pos
}

func (n *ArithIndex) Pos() Pos   { return n.Start }
func (n *ArithIndex) End() Pos   { return n.Stop }
func (n *ArithIndex) arithNode() {}

// ArithCall is `name(args)` inside an arithmetic expression: a *math
// function*, whose value is produced by running a shell function.
//
// One shell in the panel has the construct at all — zsh, through
// `functions -M` — so the grammar is a dialect's, [Dialect.ArithFunctionCall],
// and where it is off the `(` after a name is a leftover operator, which is
// the complaint bash, dash and ksh93 make about `mf(5)`.
//
// The name is not resolved here and does not have to exist: an unregistered
// name is a runtime failure with its own sentence, not a parse error, which is
// measured — `$(( nosuchmf(1) ))` is `unknown function: nosuchmf` in zsh 5.9.2
// and never a syntax complaint.
type ArithCall struct {
	Name string
	// Args are the arguments, each an expression of its own. `mf()` with
	// none is a call and not a name: zsh registers a math function with a
	// minimum of zero and takes `$(( mf() ))`.
	Args []ArithExpr
	// Text is the call exactly as it was written, from the first byte of
	// the name through the closing parenthesis.
	//
	// Kept because the one shell with the construct quotes it back verbatim
	// when the count of arguments is wrong: measured 2026-09-08,
	// `$(( mf( 5 , 6 ) ))` against a one-argument registration is
	// `wrong number of arguments: mf( 5 , 6 )`, spaces and all, so the
	// sentence is the source text rather than a rendering of the tree.
	Text  string
	Start Pos
	Stop  Pos
}

func (n *ArithCall) Pos() Pos   { return n.Start }
func (n *ArithCall) End() Pos   { return n.Stop }
func (n *ArithCall) arithNode() {}

type ArithAssign struct {
	Name string
	// Index is the subscript when the target is an array element, as in
	// `(( a[0] = 9 ))`, and nil when the target is a plain name.
	Index ArithExpr
	// Sub is the subscript as written, for the reason ArithIndex.Sub is.
	Sub string
	// Flags is the group the subscript opened with, for the reason
	// ArithIndex.Flags is: `(( a[(r)20] = 9 ))` writes the element the same
	// search reads.
	Flags *SubscriptFlags
	Op    string // = += -= *= /= %= <<= >>= &= ^= |=
	Value ArithExpr
	Start Pos
}

func (n *ArithAssign) Pos() Pos   { return n.Start }
func (n *ArithAssign) End() Pos   { return n.Value.End() }
func (n *ArithAssign) arithNode() {}

// arithLevels are the binary operators by precedence, loosest first. The order
// follows ISO C, which POSIX defers to.
var arithLevels = [][]string{
	{"||"},
	{"&&"},
	{"|"},
	{"^"},
	{"&"},
	{"==", "!="},
	{"<=", ">=", "<", ">"},
	{"<<", ">>"},
	{"+", "-"},
	{"*", "/", "%"},
}

var assignOps = []string{"<<=", ">>=", "*=", "/=", "%=", "+=", "-=", "&=", "^=", "|=", "="}

// arithParser is a precedence-climbing parser over one expression's text.
type arithParser struct {
	src  string
	off  int
	at   Pos
	p    *Parser
	dial Dialect
	// blame is where the text a leftover failure names begins: the cursor
	// after the whitespace in front of it, but *before* any double quote the
	// dialect steps over.
	//
	// The two part only in the dialect that skips a quote, and there the
	// measurement says they must: `$(( "1" "2" ))` is reported against
	// ``" "2" `` in zsh 5.9.2 — the quote it read through is in the sentence
	// — while `$(( 1 2 ))` is reported against `2 ` with the space gone. So
	// the reader consumes the whitespace after a token and stops at a quote,
	// and blaming from the cursor alone would have dropped a quote from every
	// such sentence.
	blame int
	// conditionals counts how many `?` frames are open, so that the byte the
	// dialect reads as a token wherever it stands is still the conditional's
	// where a conditional is waiting for one.
	conditionals int
	// format is the output specifier the text held, and the *last* one when
	// it held several: `$(( [#16] 255 + [#8] 1 ))` is written in base 8.
	// Kept on the parser rather than built into the tree where it was read,
	// because the construct is lexical — see ArithOutput.
	format *ArithOutput
	// stopped is where the last skip ended, which makes space idempotent:
	// several frames ask for it at the same cursor on the way down, and only
	// the first of them may move blame. Without it the second call would
	// re-blame from *after* a quote the first had stepped over. -1 rather
	// than 0 so that the first call at the start of the text still runs.
	stopped int
}

// hasExpansion reports whether an arithmetic expression contains something
// that has to be expanded before the expression can be read.
//
// `$(( $x$y ))` with x=`1+` and y=`2` is 3 in every shell in the panel: the
// text is substituted first and the *result* is the expression. So an
// expression with a `$` or a backtick in it has no tree until it runs, and a
// tree built now would be built from something that is not the program.
func hasExpansion(src string) bool {
	return strings.ContainsAny(src, "$`")
}

// parseArithLater is parseArith for the places where the text is read as part
// of reading the file, and where a failure is not the file's to report.
//
// No shell in the panel refuses a program for an expression it cannot read.
// `bash -n` takes `((echo hi))`, and so do ksh93 and zsh; the complaint comes
// when the command runs, so a branch that never runs never raises it and
// `false && ((echo hi)); echo reached` reaches the echo in all six. Ours
// refused the whole program instead — a parse-time answer to a run-time
// question, and the one on the conformance list for that reason.
//
// The tree a *successful* read builds is still kept, because it is the same
// tree the run would build and building it once is free. A failure is
// discarded along with the error it recorded, which leaves nil beside the raw
// text — the state an expression with an expansion in it has always been in,
// and the state [Runner.arithTree] already reads: expand, then parse, then
// evaluate. So there is nothing new on the far side of this.
//
// Rolling the error back rather than not looking is what keeps it to one
// question. The read has to happen either way for the tree, and the error is
// the only part of it that was ever wrong.
func (p *Parser) parseArithLater(src string, at Pos) ArithExpr {
	saved := p.err
	e := p.parseArith(src, at)
	if p.err != saved {
		p.err = saved
		return nil
	}
	return e
}

// parseArith parses the text inside `$(( … ))` or `(( … ))`.
//
// An expression containing an expansion is left alone: nil, with the raw text
// still beside it, for the interpreter to expand and read when it runs. That
// is the only thing that can — `$#` is not an operand until something knows
// what the parameters are.
//
// A failure here is recorded on the parser, which is right for the one caller
// that is reading an expression *as* the program — the interpreter, through
// [Parser.ParseArithFor], at the moment the command runs. Everything reading
// an expression while reading a file goes through parseArithLater instead.
func (p *Parser) parseArith(src string, at Pos) ArithExpr {
	if hasExpansion(src) {
		return nil
	}
	if p.dialect.ArithDoubleQuote == ArithDoubleQuoteRemoved {
		// Removed before anything reads the text, which is the whole of that
		// reading: it is what makes `1"0"` the number 10, and what makes the
		// text a failure quotes back the one without the quotes in it.
		src = strings.ReplaceAll(src, `"`, "")
	}
	a := &arithParser{src: src, at: at, p: p, dial: p.dialect, stopped: -1}
	e := a.expr()
	a.space()
	if e != nil && a.off < len(a.src) {
		kind := a.leftoverKind()
		// The two operand and operator failures name everything from here to
		// the end of the expression; the byte's own verdict names the byte.
		token := a.src[a.blame:]
		if kind == ErrArithIllegalByte {
			token = a.src[a.off : a.off+1]
		}
		a.failArith(kind, token)
	}
	if a.format != nil {
		// Lifted to the top rather than left where it was written, and over
		// the whole expression rather than over the operand beside it: the
		// specifier formats the answer, and the answer is the whole of it.
		a.format.X, a.format.Start, a.format.Stop = e, at, at
		return a.format
	}
	return e
}

// leftoverKind tells the three ways an expression can have something left
// over apart: an operand where an operator belonged, a byte the dialect's
// reader refuses outright, or text that could be neither.
//
// Only one shell in the panel words all three differently, but the
// distinction is the parser's to make — it is about what the text could have
// been, which is a grammar question and not a wording one.
func (a *arithParser) leftoverKind() ErrorKind {
	c := a.src[a.off]
	switch {
	case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z',
		c == '_', c == '(', c == '$':
		return ErrArithOperator
	case c == ':':
		// A `:` reaches here only where the dialect does *not* take it as a
		// math token, and it is text left over rather than a byte the reader
		// refuses — the byte being an operator in every shell that has a
		// conditional. It is nonetheless its own kind, because one shell
		// words a stray colon in a shape no other leftover gets: ksh93u+
		// writes `:: invalid character in expression -  1 : ` where `1 2`
		// and `1 ]` are the ordinary `<expression>: <reason>`, measured
		// 2026-09-12 over every printable byte (#2224). Two dialects have
		// no sentence for it and fall back to the leftover one, which is
		// what they said before this kind reached them.
		//
		// Wherever the colon stands: after a complete conditional as much as
		// with no `?` at all — `$(( 1 ? 2 : 3 : 4 ))` is the same shape
		// there — and inside a subscript, which is an expression like any
		// other.
		return ErrArithColonWithoutQuestion
	}
	// Where an operator belonged is one of the two positions the byte's own
	// verdict stands at, the expression having read a complete value and been
	// able to stop.
	if a.refusedOutright() {
		return ErrArithIllegalByte
	}
	return ErrArithBadOperator
}

// refusedOutright reports whether the byte at the cursor is one this dialect's
// arithmetic reader refuses as part of no token.
func (a *arithParser) refusedOutright() bool {
	return a.off < len(a.src) &&
		strings.IndexByte(a.dial.ArithBytesRefusedOutright, a.src[a.off]) >= 0
}

// nothingReadYet reports whether the expression has consumed nothing but
// whitespace, which is the *other* position a refused byte's own verdict
// stands at.
//
// Asked of the text rather than kept as a flag, and that is deliberate: the
// question is "could this expression have stopped here", and the answer is a
// property of what has been consumed, not of which of the seven frames that
// consume an operator happened to recurse last. A flag would have to be set
// at every one of them and would be wrong the first time one was added —
// `$((  @  ))` is `illegal character` in that shell, so the cursor being at
// offset zero is not the test either.
func (a *arithParser) nothingReadYet() bool {
	return strings.TrimLeft(a.src[:a.off], " \t\n") == ""
}

// failArith records an arithmetic failure with the whole expression and the
// part of it that failed.
//
// It is not p.fail: a shell does not call this a syntax error. All four report
// it the way they report a division by zero — the expression quoted, and a
// reason — so it travels as its own kind and is worded by the same vector.
func (a *arithParser) failArith(kind ErrorKind, token string) {
	if a.p.err != nil {
		return
	}
	a.p.err = &Error{
		Pos: a.at, Kind: kind, Expr: a.src, Token: token,
		Msg: "arithmetic expression: " + a.src,
	}
}

// space skips what stands between tokens: blanks, the double quote the
// dialect reads through, and the output-format specifier.
//
// The specifier is skipped *here*, with the blanks, rather than parsed as an
// operand, and that is the whole of how the construct is positioned: it is a
// token that produces no value, so it may stand anywhere one may — before an
// operand, after one, in a branch that is never taken — and every one of those
// is measured. See [Dialect.ArithOutputFormat].
func (a *arithParser) space() {
	if a.off == a.stopped {
		return
	}
	for {
		a.blanks()
		if !a.outputFormat() {
			break
		}
	}
	a.stopped = a.off
}

// blanks is the whitespace half of space, kept apart so that a specifier
// consumed between two runs of it does not leave blame pointing at itself.
func (a *arithParser) blanks() {
	for a.off < len(a.src) && isArithSpace(a.src[a.off]) {
		a.off++
	}
	a.blame = a.off
	if a.dial.ArithDoubleQuote == ArithDoubleQuoteSkipped {
		for a.off < len(a.src) && (isArithSpace(a.src[a.off]) || a.src[a.off] == '"') {
			a.off++
		}
	}
}

// outputFormat reads one `[#…]` specifier, reporting whether there was one to
// read. A bracketed group the dialect cannot read is consumed and refused, so
// the failure names the specifier rather than leaving a `[` to be blamed as a
// missing operand.
func (a *arithParser) outputFormat() bool {
	if !a.dial.ArithOutputFormat || a.off >= len(a.src) || a.src[a.off] != '[' {
		return false
	}
	end := strings.IndexByte(a.src[a.off:], ']')
	if end < 0 {
		// No closing bracket at all. Unreachable from `$(( ))`, whose word
		// scanner never hands over text with an unbalanced `[` in it, and
		// still answered here rather than left to the operand failure: the
		// text begins a specifier and the sentence should say so.
		a.failArith(ErrArithBadOutputFormat, a.src[a.off:])
		a.off = len(a.src)
		return true
	}
	text := a.src[a.off : a.off+end+1]
	a.off += end + 1
	n, ok := parseOutputFormat(text)
	if !ok {
		// All digits inside the brackets is the one shape worded apart, and
		// the two are one character from each other.
		kind := ErrArithBadOutputFormat
		if body := text[1 : len(text)-1]; body != "" && isAllArithDigits(body) {
			kind = ErrArithBadBaseSyntax
		}
		a.failArith(kind, text)
		return true
	}
	// The textually last specifier decides, so a later one simply replaces
	// what an earlier one said.
	a.format = n
	return true
}

// parseOutputFormat reads the inside of a specifier: `#`, an optional second
// `#`, an optional base, and an optional `_` with an optional group size.
//
// At least one of the base and the `_` has to be there — `[#]` and `[##]` are
// both refused — and nothing may follow, so a blank anywhere in it is a
// failure rather than something to skip.
func parseOutputFormat(text string) (*ArithOutput, bool) {
	body, ok := strings.CutPrefix(text[1:len(text)-1], "#")
	if !ok {
		return nil, false
	}
	n := &ArithOutput{Text: text, Prefixed: true}
	if rest, ok := strings.CutPrefix(body, "#"); ok {
		n.Prefixed, body = false, rest
	}
	digits := leadingArithDigits(body)
	body = body[len(digits):]
	grouped := false
	if rest, ok := strings.CutPrefix(body, "_"); ok {
		grouped = true
		// A bare `_` is three, which is how a decimal thousands separator is
		// written; `_0` is the way to turn grouping off again.
		n.Group = 3
		size := leadingArithDigits(rest)
		body = rest[len(size):]
		if size != "" {
			g, err := strconv.Atoi(size)
			if err != nil {
				return nil, false
			}
			n.Group = g
		}
	}
	if body != "" || (digits == "" && !grouped) {
		return nil, false
	}
	if digits != "" {
		b, err := strconv.Atoi(digits)
		if err != nil {
			return nil, false
		}
		n.Base, n.Based = b, true
	}
	return n, true
}

func leadingArithDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

func isAllArithDigits(s string) bool { return leadingArithDigits(s) == s }

func isArithSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' }

func (a *arithParser) has(s string) bool {
	return strings.HasPrefix(a.src[a.off:], s)
}

func (a *arithParser) take(s string) bool {
	if a.has(s) {
		a.off += len(s)
		return true
	}
	return false
}

// expr is the loosest level: the sequence operator, when the dialect has it.
func (a *arithParser) expr() ArithExpr {
	x := a.assign()
	if x == nil {
		return nil
	}
	for {
		a.space()
		if !a.has(",") || !a.dial.ArithComma {
			return x
		}
		a.off++
		y := a.assign()
		if y == nil {
			a.p.fail("expected an expression after , in arithmetic")
			return x
		}
		x = &ArithBinary{Op: ",", X: x, Y: y}
	}
}

// assign handles assignment, which is right-associative and only valid with a
// name on the left.
func (a *arithParser) assign() ArithExpr {
	save := a.off
	a.space()
	start := a.at
	if name, ok := a.name(); ok {
		// `a[0] = 9` assigns to an element, so the subscript belongs to the
		// target rather than being a value of its own.
		// The empty subscript is carried rather than refused here, because
		// this is a *guess* that the name begins an assignment and the guess
		// is taken back below when no operator follows: refusing inside the
		// bracket read would make `$(( a[] ))` a parse error on the strength
		// of an assignment that is not there, which is what it was.
		sub := a.subscript(true)
		a.space()
		for _, op := range assignOps {
			// `==` is equality, not assignment, so it must not be taken here.
			if op == "=" && a.has("==") {
				break
			}
			at := a.off
			if a.take(op) {
				if sub.Empty {
					// An assignment *target* with nothing between the
					// brackets keeps the refusal it has always had. The three
					// shells with the construct part here too and part
					// differently from the way they part over reading one —
					// `not an identifier: a[]`, `not a valid identifier`, and
					// a silent write to element zero — so the grammar holds
					// until that second disagreement has an axis of its own.
					a.p.failKind(ErrArithOperand, "bad array subscript: %s", "")
					return nil
				}
				v := a.assign()
				if v == nil {
					a.failArith(ErrArithOperandEnd, a.src[at:])
					return nil
				}
				return &ArithAssign{
					Name: name, Index: sub.Index, Sub: sub.Text, Flags: sub.Flags,
					Op: op, Value: v, Start: start,
				}
			}
		}
	}
	a.off = save
	return a.ternary()
}

// ternary reads `cond ? then : else`, and the three ways it can be
// incomplete.
//
// Each of the three is a kind of its own rather than a sentence written here,
// because the panel words them apart in two different places — bash parts a
// conditional's missing value from an ordinary one, and ksh93 parts the
// *then* from the *else* — and because a bare string here reaches no dialect
// at all, which is what `expected : in an arithmetic conditional` was.
//
// The token blamed is the last thing read, in every case, which is what bash
// names: `?` for a missing then, the then itself for a missing colon, and `:`
// for a missing else.
func (a *arithParser) ternary() ArithExpr {
	cond := a.binary(0)
	if cond == nil {
		return nil
	}
	a.space()
	question := a.off
	if !a.take("?") {
		return a.colonWithoutQuestion(cond)
	}
	a.space()
	thenAt := a.off
	// The colon this conditional is waiting for belongs to it and not to a
	// stray-colon reading: the then-expression is parsed at the assignment
	// level, which recurses through here, and a nested frame that took the
	// `:` for itself would break every `a ? b : c` in the dialect that reads
	// the byte as a token.
	a.conditionals++
	then := a.assign()
	a.conditionals--
	if then == nil {
		a.failArith(ErrArithConditionalThen, a.src[question:])
		return cond
	}
	a.space()
	colon := a.off
	if !a.take(":") {
		a.failArith(ErrArithConditionalColon, a.src[thenAt:])
		return cond
	}
	a.space()
	els := a.assign()
	if els == nil {
		a.failArith(ErrArithConditionalElse, a.src[colon:])
		return cond
	}
	return &ArithCond{Cond: cond, Then: then, Else: els}
}

// colonWithoutQuestion is a `:` standing where no `?` opened a conditional, in
// the dialect whose reader takes the byte as a math token wherever it is
// written — see Dialect.ArithColonIsAToken.
//
// It is read *and then* complained about, which is the whole of the
// difference: `$(( 1 : ))` runs out of input looking for the value after the
// colon and earns the ordinary end-of-input sentence, and `$(( 1 : 2 ))`
// finds one and earns a sentence of its own. A reader that stopped at the
// byte would give the same complaint to both.
func (a *arithParser) colonWithoutQuestion(cond ArithExpr) ArithExpr {
	if a.conditionals > 0 || !a.dial.ArithColonIsAToken || !a.has(":") {
		return cond
	}
	colon := a.off
	a.off++
	a.space()
	if a.off >= len(a.src) {
		a.failArith(ErrArithOperandEnd, a.src[colon:])
		return cond
	}
	if a.assign() == nil {
		return cond
	}
	a.failArith(ErrArithColonWithoutQuestion, a.src[colon:])
	return cond
}

func (a *arithParser) binary(level int) ArithExpr {
	if level >= len(arithLevels) {
		return a.power()
	}
	x := a.binary(level + 1)
	if x == nil {
		return nil
	}
	for {
		a.space()
		op := ""
		for _, cand := range arithLevels[level] {
			if !a.has(cand) {
				continue
			}
			// Longest match: `<` must not be taken out of `<<` or `<=`, and
			// `&` not out of `&&`.
			if longer := a.longerOperator(cand); longer {
				continue
			}
			op = cand
			break
		}
		if op == "" {
			return x
		}
		at := a.off
		a.off += len(op)
		a.space()
		yStart := a.off
		y := a.binary(level + 1)
		if y == nil {
			a.failArith(ErrArithOperandEnd, a.src[at:])
			return x
		}
		x = &ArithBinary{Op: op, X: x, Y: y, YStart: yStart}
	}
}

// longerOperator reports whether a longer operator starts here, so a shorter
// one at a looser level is not taken by mistake.
func (a *arithParser) longerOperator(cand string) bool {
	for _, longer := range []string{"<<=", ">>=", "&&", "||", "<<", ">>", "<=", ">=", "==", "!="} {
		if len(longer) > len(cand) && a.has(longer) && strings.HasPrefix(longer, cand) {
			return true
		}
	}
	// `x =` after an operator candidate like `<` is fine; only compound
	// assignment matters, and those are caught above.
	//
	// `**` is here only when the dialect has it: where it does not, the
	// first `*` is a multiplication and the second fails as an operand,
	// which is how the shell without the operator reports it.
	return cand == "*" && a.dial.ArithExponent && a.has("**")
}

// power is `**`, when the dialect has it. Measured across the three shells
// that parse it: tighter than `*` (`2*3**2` is 18) and looser than unary
// (`-2**2` is 4 — the sign is part of the base), and right-associative
// (`2**3**2` is 512), which the recursion on the right encodes.
func (a *arithParser) power() ArithExpr {
	x := a.unary()
	if x == nil {
		return nil
	}
	a.space()
	at := a.off
	if !a.dial.ArithExponent || !a.take("**") {
		return x
	}
	a.space()
	yStart := a.off
	y := a.power()
	if y == nil {
		a.failArith(ErrArithOperandEnd, a.src[at:])
		return x
	}
	return &ArithBinary{Op: "**", X: x, Y: y, YStart: yStart}
}

func (a *arithParser) unary() ArithExpr {
	a.space()
	start := a.at
	switch {
	case a.has("++"), a.has("--"):
		op := a.src[a.off : a.off+2]
		if !a.dial.ArithIncDec {
			a.p.fail("%s is not available in this dialect", op)
			return nil
		}
		a.off += 2
		x := a.unary()
		if x == nil {
			return nil
		}
		return &ArithUnary{Op: op, X: x, Start: start}
	case a.has("+"), a.has("-"), a.has("~"), a.has("!"):
		at := a.off
		op := a.src[a.off : a.off+1]
		a.off++
		x := a.unary()
		if x == nil {
			a.failArith(ErrArithOperandEnd, a.src[at:])
			return nil
		}
		return &ArithUnary{Op: op, X: x, Start: start}
	}
	return a.postfix()
}

func (a *arithParser) postfix() ArithExpr {
	x := a.primary()
	if x == nil {
		return nil
	}
	a.space()
	if (a.has("++") || a.has("--")) && a.dial.ArithIncDec {
		op := a.src[a.off : a.off+2]
		a.off += 2
		return &ArithUnary{Op: op, Postfix: true, X: x, Start: x.Pos()}
	}
	return x
}

func (a *arithParser) primary() ArithExpr {
	a.space()
	start := a.at
	if a.off >= len(a.src) {
		return nil
	}
	if a.take("(") {
		x := a.expr()
		a.space()
		if !a.take(")") {
			a.p.fail("expected ) in arithmetic")
		}
		return x
	}
	if a.dial.ArithCharacterCode && a.src[a.off] == '#' {
		return a.charCode(start)
	}
	if a.dial.ArithCharacterConstant && a.src[a.off] == '\'' {
		return a.charConstant(start)
	}
	if c := a.src[a.off]; c >= '0' && c <= '9' {
		return a.number(start)
	}
	// A literal may begin with its point where the dialect has floats: `.5`
	// is a number there and is an operator followed by one everywhere else,
	// which is why the two shells without floats blame the `.5` rather than
	// the `1.5` it was part of.
	if a.dial.ArithFloat && a.src[a.off] == '.' && a.off+1 < len(a.src) &&
		a.src[a.off+1] >= '0' && a.src[a.off+1] <= '9' {
		return a.number(start)
	}
	begin := a.off
	if name, ok := a.name(); ok {
		// A `(` *touching* the name is a call, and one with a space before
		// it is not: measured, `$(( mf ( 5 ) ))` against a live registration
		// is `bad math expression: operator expected at `( 5 ) '` where
		// `$(( mf(5) ))` runs the function. So the test is the very next
		// byte, before any space is skipped.
		if a.dial.ArithFunctionCall && a.off < len(a.src) && a.src[a.off] == '(' {
			return a.call(name, start, begin)
		}
		if sub := a.subscript(true); sub.Present {
			return &ArithIndex{
				Name: name, Index: sub.Index, Sub: sub.Text, Empty: sub.Empty,
				Flags: sub.Flags, Start: start, Stop: start,
			}
		}
		return &ArithVar{Name: name, Start: start, Stop: start}
	}
	// A `$` here is a parameter expansion the lexer left in place; both
	// spellings work, so it is read as the same variable reference.
	if a.take("$") {
		if name, ok := a.name(); ok {
			return &ArithVar{Name: name, Start: start, Stop: start}
		}
	}
	// Nothing here could begin a value, which is a missing *operand* and not
	// a leftover operator: `$((.5))` in a dialect without floats is blamed
	// for wanting a number, where `$((1 @))` is blamed for the `@`. Both
	// shells without floats word the two apart.
	//
	// There *is* text here, which is the half that separates this from a
	// caller's ErrArithOperandEnd. Running out is not reported at all —
	// primary returns nil at the top with no error, so the frame that wanted
	// the operand names the operator it was left holding. The two are the
	// same failure to two of the panel and different failures to the other
	// two, and this is the only place the parser can tell them apart.
	//
	// Unless the byte itself is one this dialect refuses and nothing has been
	// read yet, which is the start of the expression: there is no operator to
	// have been left wanting an operand, so the byte answers for itself. The
	// token is the byte alone rather than the rest of the text, because that
	// is what the sentence names.
	if a.refusedOutright() && a.nothingReadYet() {
		a.failArith(ErrArithIllegalByte, a.src[a.off:a.off+1])
		return nil
	}
	a.failArith(ErrArithOperand, a.src[a.off:])
	return nil
}

// call reads `name(a, b)`, the math-function call form, with the name already
// read and begin the offset it started at.
//
// Arguments are parsed at the assignment level rather than through expr,
// because the comma here separates arguments and is not the sequence
// operator: `mf(1,2)` is two arguments in the dialect that also reads
// `$(( 1,2 ))` as a sequence, so the two readings of `,` are told apart by
// which of them is inside the parentheses.
func (a *arithParser) call(name string, start Pos, begin int) ArithExpr {
	a.off++ // the `(`
	n := &ArithCall{Name: name, Start: start, Stop: start}
	a.space()
	if !a.has(")") {
		for {
			arg := a.assign()
			if arg == nil {
				return nil
			}
			n.Args = append(n.Args, arg)
			a.space()
			if a.take(",") {
				a.space()
				// A trailing comma before the `)` is allowed and adds no
				// argument: measured, `mf(5,)` passes one argument and
				// `mf(5,6,)` two. A comma with nothing *before* it is a
				// different matter — `mf(,)` and `mf(5,,6)` are both an
				// operand expected — which is what the loop's shape already
				// says, because an argument is read before every comma.
				if a.has(")") {
					break
				}
				continue
			}
			break
		}
	}
	a.space()
	if !a.take(")") {
		a.p.fail("expected ) in arithmetic")
		return nil
	}
	n.Text = a.src[begin:a.off]
	return n
}

// number reads a literal without converting it, including the `base#digits`
// form where the dialect has it.
func (a *arithParser) number(start Pos) ArithExpr {
	begin := a.off
	if a.dial.ArithNumeralEndsAtABadDigit {
		a.numberInItsOwnBase()
		return &ArithNum{Text: a.src[begin:a.off], Start: start, Stop: start}
	}
	// Every character the base-64 alphabet knows, whatever base the literal
	// turns out to be in: `1abc`, `0y`, `1@2` and `1_` are one numeral each
	// in bash, which reads them all and then reports a digit its base does
	// not have. Which digits a base really allows is decided at conversion,
	// where the number is read.
	//
	// Except a byte the dialect refuses outright, which is no part of any
	// token: `@` is digit 62 of the alphabet and is `illegal character` in
	// the dialect that refuses it, so the two rules would otherwise disagree
	// about the same byte.
	for a.off < len(a.src) && isBaseDigit(a.src[a.off]) && !a.refusedOutright() {
		a.off++
	}
	if a.dial.ArithFloat && !isBasedLiteral(a.src[begin:a.off]) {
		a.floatTail(begin)
	}
	if a.off < len(a.src) && a.src[a.off] == '#' && a.dial.ArithExplicitBase {
		a.off++
		for a.off < len(a.src) && isBaseDigit(a.src[a.off]) {
			a.off++
		}
	}
	return &ArithNum{Text: a.src[begin:a.off], Start: start, Stop: start}
}

// numberInItsOwnBase reads a numeral the way the dialect that stops at a
// character its base cannot use reads one — see
// Dialect.ArithNumeralEndsAtABadDigit.
//
// The base is known from the text: a radix prefix names it, a `base#` names
// it, and otherwise it is ten. So the reader can stop where the shell stops,
// which is what makes `$(( 1abc ))` two tokens there and one everywhere else.
func (a *arithParser) numberInItsOwnBase() {
	begin := a.off
	base := 10
	switch {
	case a.hasPrefixAt("0x") || a.hasPrefixAt("0X"):
		a.off += 2
		base = 16
	case a.dial.ArithBinaryLiteral && (a.hasPrefixAt("0b") || a.hasPrefixAt("0B")):
		a.off += 2
		base = 2
	}
	a.digitsIn(base)
	if base != 10 {
		return
	}
	if a.dial.ArithFloat {
		before := a.off
		a.floatTail(begin)
		if a.off != before {
			return
		}
	}
	if a.off >= len(a.src) || a.src[a.off] != '#' || !a.dial.ArithExplicitBase {
		return
	}
	// The digits read so far are the base, in decimal — and a base of zero is
	// read as ten, which is what the shell that has this reader does with it:
	// `$(( 0#5 ))` is 5 there and `$(( 0#z ))` stops at the `z`.
	named, err := strconv.Atoi(a.src[begin:a.off])
	if err != nil {
		return
	}
	a.off++
	if named == 0 {
		// Zero is the base this reader goes *through*: what follows is read
		// as an ordinary constant, radix prefix and all, so `0#0x10` is one
		// token. Whether it comes to anything is the evaluator's question —
		// see Semantics.ArithBaseZeroReadsTheDigitsAsWritten.
		a.numberInItsOwnBase()
		return
	}
	a.digitsIn(named)
}

// digitsIn consumes the run of characters the base can use.
func (a *arithParser) digitsIn(base int) {
	for a.off < len(a.src) {
		v, known := baseDigitValue(a.src[a.off], base)
		if !known || v >= base {
			return
		}
		a.off++
	}
}

func (a *arithParser) hasPrefixAt(p string) bool {
	return strings.HasPrefix(a.src[a.off:], p)
}

// baseDigitValue is the base-64 alphabet: 0-9, then letters — one case as good
// as the other through 36, and apart above it, where a-z is 10..35, A-Z 36..61,
// `@` 62 and `_` 63. The second result says the byte is a digit somewhere in
// that alphabet even where this base cannot reach it.
func baseDigitValue(c byte, base int) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'Z':
		if base <= 36 {
			return int(c-'A') + 10, true
		}
		return int(c-'A') + 36, true
	case c == '@':
		return 62, true
	case c == '_':
		return 63, true
	}
	return 0, false
}

// floatTail extends a literal over the point and exponent a float may carry.
//
// Only where the dialect has floats: `1.5` is one number there and two tokens
// with an operator between them in the shells that do not, which is the
// difference their diagnostics show.
//
// A sign is taken only straight after the exponent letter, so `1e-3` is one
// literal and `1-3` stays a subtraction.
//
// The exponent needs care because the digit scan before this one accepts `e`
// as a hex digit, so `1e-3` arrives with the `e` already consumed and only the
// `-3` left to find.
func (a *arithParser) floatTail(begin int) {
	if a.off < len(a.src) && a.src[a.off] == '.' {
		a.off++
		for a.off < len(a.src) && a.src[a.off] >= '0' && a.src[a.off] <= '9' {
			a.off++
		}
	}
	next := a.off
	if a.off < len(a.src) && (a.src[a.off] == 'e' || a.src[a.off] == 'E') {
		next++
	} else if a.off > begin && (a.src[a.off-1] == 'e' || a.src[a.off-1] == 'E') {
		// Already swallowed by the digit scan, which reads `e` as hex.
	} else {
		return
	}
	if next < len(a.src) && (a.src[next] == '+' || a.src[next] == '-') {
		next++
	}
	if next < len(a.src) && a.src[next] >= '0' && a.src[next] <= '9' {
		a.off = next
		for a.off < len(a.src) && a.src[a.off] >= '0' && a.src[a.off] <= '9' {
			a.off++
		}
	}
}

// isBasedLiteral reports whether a literal states its own base, which takes it
// out of the float rules: `0x1e` is a hex integer whose digits include an `e`,
// not a number with an exponent.
func isBasedLiteral(text string) bool {
	return strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") ||
		strings.Contains(text, "#")
}

// isBaseDigit is the base-64 alphabet a `base#digits` literal may draw on:
// 0-9, both letter cases, `@` and `_`.
func isBaseDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') || c == '@' || c == '_'
}

// arithSubscript is what a bracketed subscript after a name came to.
//
// It is a record rather than a list of results because the four facts it
// carries are not independent, and a caller that reads only some of them gets
// the wrong answer: `a[]` and `a[.k]` both have no expression, and only one of
// them is empty.
type arithSubscript struct {
	// Present says there was a bracket pair at all. Everything else is
	// meaningless when it is false — a bare name has no subscript, and so
	// does `a[1` , whose bracket never closes.
	Present bool
	// Index is the expression the text parses as, and nil when it does not
	// parse as one. See Text.
	Index ArithExpr
	// Text is the subscript exactly as written, brackets excluded.
	Text string
	// Empty says the brackets held nothing at all. See ArithIndex.Empty.
	Empty bool
	// Flags is the parenthesized flag group the subscript opened with, where
	// the dialect has them. See ArithIndex.Flags.
	Flags *SubscriptFlags
}

// subscript reads `[…]` after a name.
//
// The inside is read as an expression, because on an indexed array that is
// what it is: `a[i+1]` counts from the value of i. It is *not* refused when it
// will not read as one, and that is the whole point of the record this
// returns. A subscript is only an expression when the name is an indexed
// array; on an associative one it is a key, read exactly as `${m[k]}` reads
// it — and the parser cannot know which the name is, because the name need not
// exist yet or at all. So the text is kept, the expression is left nil, and
// the question is handed to whoever evaluates it, which is the only place the
// answer is knowable.
//
// Refusing here is what `$(( m[.accept-line] ))` used to meet: a key that is
// not an expression, on a name the parser had no way to know was associative.
// Measured, bash 5.3 and zsh 5.9.2 both answer that with the element and not
// with a complaint.
//
// emptyOK says the caller can carry a `[]` written with nothing between the
// brackets. Where it is false the empty pair is the parse failure it has
// always been; where it is true Empty says so and Index is nil, which is a
// question for whoever evaluates it. See ArithIndex.Empty.
func (a *arithParser) subscript(emptyOK bool) arithSubscript {
	// The same flag that admits `${a[1]}`: one question about whether the
	// dialect has subscripts at all, asked in the two places that need it.
	// Where it is off, `a[0]` is a name followed by text that cannot be an
	// operator, and is reported as that.
	if !a.dial.ArraySubscript {
		return arithSubscript{}
	}
	if a.off >= len(a.src) || a.src[a.off] != '[' {
		return arithSubscript{}
	}
	open := a.off
	depth := 0
	for a.off < len(a.src) {
		switch a.src[a.off] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				inner := a.src[open+1 : a.off]
				a.off++
				if inner == "" && emptyOK {
					return arithSubscript{Present: true, Empty: true}
				}
				if a.dial.ArraySubscriptFlags {
					// A group decides how the subscript is *read*, so it is
					// taken off before anything tries to read the text as an
					// expression — `(r)20` is a search and not a sum, and
					// leaving it to the expression reader is what made the
					// whole group silently part of a key (#1986).
					//
					// The operand is a literal word rather than a lexed one:
					// an arithmetic expression is expanded whole before it is
					// parsed, so there is nothing left in it to expand and a
					// second pass would perform a substitution twice.
					if g, rest, ok := scanSubscriptFlags(inner); ok {
						g.Arg = literalWord(rest, a.at)
						return arithSubscript{Present: true, Text: inner, Flags: g}
					}
				}
				// The inner parser shares the outer one's error slot, so a
				// text that is not an expression would leave a refusal behind
				// even though this is no longer a refusal. Put back what was
				// there before it read, which is nil on the ordinary path and
				// an earlier failure on the path where one is already
				// recorded — either way the state the caller had.
				held := a.p.err
				sub := &arithParser{src: inner, at: a.at, p: a.p, dial: a.dial, stopped: -1}
				e := sub.expr()
				sub.space()
				if e == nil || sub.off < len(sub.src) {
					a.p.err = held
					return arithSubscript{Present: true, Text: inner}
				}
				return arithSubscript{Present: true, Index: e, Text: inner}
			}
		}
		a.off++
	}
	// No closing bracket: not a subscript at all, so the name stands alone
	// and whatever follows is the caller's problem to report.
	a.off = open
	return arithSubscript{}
}

// charCode reads `#name`, `#\c` and `##c`, where the dialect has them.
//
// The operand is not an expression and is read here rather than by recursing:
// `#b` names the parameter b, and the reading stops at the name. Measured on
// zsh 5.9.2, which is the whole panel for this — bash 5.3, bash 3.2, bash as
// `sh`, ksh93 and dash all call `$((#b))` an arithmetic syntax error, and the
// grammar without the flag reaches the same refusal by the same route.
func (a *arithParser) charCode(start Pos) ArithExpr {
	n := &ArithCharCode{Op: "#", Start: start, Stop: start}
	a.off++
	if a.off < len(a.src) && a.src[a.off] == '#' {
		n.Op = "##"
		a.off++
	}
	begin := a.off
	switch {
	case n.Op == "##" || (a.off < len(a.src) && a.src[a.off] == '\\'):
		// One character, and exactly one: `$((##ab))` is the code of `a` with
		// a `b` left over, which the caller then refuses as an operator it
		// cannot read. An escape counts as the character it stands for, so
		// `$((##\x41x))` is the same shape.
		size := charCodeOperandLen(a.src[a.off:], n.Op == "##")
		if size == 0 {
			a.p.failKind(ErrArithCharacterMissing, "character missing after %s", n.Op)
			return nil
		}
		a.off += size
		n.Char = a.src[begin:a.off]
	default:
		// A parameter, which here includes the positional ones: `$((#1))` is
		// the first character of `$1` and `$((#0))` of the shell's own name,
		// so the scan takes digits as readily as letters.
		for a.off < len(a.src) && isCharCodeNameByte(a.src[a.off]) {
			a.off++
		}
		n.Name = a.src[begin:a.off]
		if a.off < len(a.src) && a.src[a.off] == '[' {
			if end := closingBracket(a.src[a.off:]); end > 0 {
				a.off += end + 1
				n.Subscripted = true
			}
		}
	}
	n.Stop = start
	return n
}

// charConstant reads `'c'`, the character constant, where the dialect has it.
//
// It builds the same node the `##c` spelling does rather than one of its own,
// because it is the same question: the code of one character, with the escapes
// the dialect's table decodes. A second node would be a second place for the
// escape rules to drift apart.
//
// The closing quote is optional and is measured that way — `$(( 'a ))` is 97
// and `$(( ” ))` is 39, the second quote read as the character with nothing
// left to close it. Only one character is read, so `'ab'` leaves `b'` standing
// where an operator belongs and the caller reports it, which is the syntax
// error that shell gives.
func (a *arithParser) charConstant(start Pos) ArithExpr {
	a.off++ // the opening quote
	size := charCodeOperandLen(a.src[a.off:], true)
	if size == 0 {
		a.failArith(ErrArithOperandEnd, "'")
		return nil
	}
	n := &ArithCharCode{
		Op: "''", Char: a.src[a.off : a.off+size], Start: start, Stop: start,
	}
	a.off += size
	if a.off < len(a.src) && a.src[a.off] == '\'' {
		a.off++
	}
	return n
}

// isCharCodeNameByte reports whether a byte can stand in the parameter name
// the character-code operator reads.
func isCharCodeNameByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// charCodeOperandLen is how many bytes the one character after the operator
// occupies, 0 when there is nothing there.
//
// Shapes and not meanings: what an escape *stands for* is the dialect's table,
// decoded where the value is taken, and all this has to know is where the
// operand ends. `escapes` is false for the single-`#` spelling, where a
// backslash takes the next character as itself rather than beginning an escape
// — measured, `$((#\n))` is the letter n where `$((##\n))` is a newline.
func charCodeOperandLen(s string, escapes bool) int {
	if s == "" {
		return 0
	}
	if s[0] != '\\' {
		_, size := utf8.DecodeRuneInString(s)
		return size
	}
	if len(s) == 1 {
		return 0
	}
	if !escapes {
		_, size := utf8.DecodeRuneInString(s[1:])
		return 1 + size
	}
	switch c := s[1]; {
	case c == 'x':
		return 2 + baseDigits(s[2:], 16, 2)
	case c == 'u':
		return 2 + baseDigits(s[2:], 16, 4)
	case c == 'U':
		return 2 + baseDigits(s[2:], 16, 8)
	case c >= '0' && c <= '7':
		return 1 + baseDigits(s[1:], 8, 3)
	}
	_, size := utf8.DecodeRuneInString(s[1:])
	return 1 + size
}

// baseDigits counts how many of the first max bytes are digits in the base.
func baseDigits(s string, base, max int) int {
	n := 0
	for n < len(s) && n < max {
		c := s[n]
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'f':
			v = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v = int(c-'A') + 10
		default:
			return n
		}
		if v >= base {
			return n
		}
		n++
	}
	return n
}

func (a *arithParser) name() (string, bool) {
	if a.off >= len(a.src) || !isNameStart(a.src[a.off]) {
		return "", false
	}
	begin := a.off
	for a.off < len(a.src) && isNameByte(a.src[a.off], a.off-begin) {
		a.off++
	}
	return a.src[begin:a.off], true
}
