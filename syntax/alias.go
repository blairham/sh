// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// Alias expansion, which happens when a line is *read* and not when it runs.
//
// That is why it lives here rather than in the interpreter: an alias may hold
// a keyword, so `alias iff='if true; then'` followed by `iff echo yes; fi`
// prints `yes`. Nothing that substitutes at execution time can do that,
// because by then the grammar has already been decided.
//
// It is a token-level substitution rather than a textual splice, and that
// choice is measured rather than assumed. Every diagnostic about a command
// that came from a *single-line* alias names the line the alias word was
// written on — bash, dash and ksh93 all report line 3 for `alias
// bad=nosuchcmd` on line 2 used on line 3, and `$LINENO` inside such a body
// reads the same. So a token from an expansion carries the position of the
// word it replaced, every position still points into the real input, and
// there is no position mapping to keep.
//
// The model has one seam and [Parser.carryOpenWord] is it. A body lexed
// between its own edges cannot be asked what it left *open*, and every
// column in the panel answers that question textually: a quote the body
// opens is still open when the rest of the line is read, and one nothing
// closes is an unterminated quote (#2685). So the body's unfinished tail is
// read again joined to what follows it, and what that reading took is spent
// — pending tokens dropped, the input's own lexer moved — one construct
// spanning the seam, without the positions moving.
//
// What follows it is not only the input. Where the alias word was itself a
// token of another body's expansion, the text after it is that body's
// remainder and then the remainder of every body that one was spliced into,
// and only then the input. That is [Parser.pendingTails], the one thing the
// queue carries that is text rather than tokens, and it is what lets the
// seam be crossed at every level rather than at the outermost only (#2709).
//
// The one place the two models are visible from outside is a body containing
// a newline, and [Dialect.AliasBodyCountsLines] is the axis. Where it is on,
// the body's newlines are lines of the input: a command on the body's second
// line is reported there, and every later line of the file shifts by one per
// newline in every body that was expanded. That is still a token
// substitution here — the tokens are given lines rather than the text being
// spliced — which is what keeps positions pointing into the real input while
// the numbering matches. See docs/spec/grammar/tokenization.md.

// Aliases answers whether a word names an alias, and what it stands for.
//
// The parser holds no table of its own: the table belongs to the shell, is
// changed by the `alias` builtin while the script runs, and is therefore the
// caller's to keep. A nil Aliases expands nothing, which is what a shell that
// does not expand them in this context supplies.
//
// The same type answers for all three kinds. [Parser.GlobalAliases] is asked
// about every word rather than only a command word, and
// [Parser.SuffixAliases] is asked about a command word's *extension* rather
// than about the whole word — so the argument is a suffix there, and nothing
// else about the seam changes.
type Aliases func(name string) (value string, ok bool)

// expandCommandWord is alias expansion where a command word stands: the table
// first, and then the suffix kind, which is keyed on the word's extension.
//
// The order is the measured one and it falls out of doing both while reading
// the line: `alias p.sh='echo ALIAS'` beside `alias -s sh='echo SUFFIX'` runs
// the regular alias, and a suffix alias beats an executable of that name on
// PATH and a function of that name — neither of which exists yet when the
// line is read.
func (p *Parser) expandCommandWord(done map[string]bool) {
	p.expandAlias(done, p.Aliases)
	p.expandSuffixAlias(done)
}

// expandCommandStart offers the current token to the alias table as the word
// a command begins with, beginning a fresh set of spent names for it.
//
// Two callers, one door. parseCommand makes this call before it dispatches on
// the keyword, and parsePipeline makes it for the two words it reads in front
// of a command — see [Parser.expandPipelineHead]. A second copy of the three
// lines is what let the head of a pipeline drift away from every other
// command word in the first place.
func (p *Parser) expandCommandStart() {
	if p.Aliases == nil && p.SuffixAliases == nil {
		return
	}
	p.aliasNextWord = false
	p.aliasDone = map[string]bool{}
	if p.aliasAtAFunctionName() {
		return
	}
	p.expandCommandWord(p.aliasDone)
}

// aliasAtAFunctionName answers what this dialect does with an alias standing
// where a **function name** is being defined, and reports whether the
// expansion is off.
//
// Three of the five columns expand there like anywhere else, which is what
// this shell does and what the zero value keeps. The other two are measured
// and are not the same answer: one declines the expansion where the `(`
// stands immediately after the word, and the other refuses the definition
// outright. See [Dialect.AliasAtAFunctionName], which has the panel.
//
// It matters here rather than in principle because a dialect ships preset
// aliases whose values are declaration words: `nameref() { :; }` and
// `float() { :; }` expanded to `typeset -n () { :; }`, which is a **parse
// error** and so costs every line of the file rather than its own (#3643).
//
// The table is asked, which is what the refusing column needs: `zz() { :; }`
// is an ordinary definition there when `zz` names no alias. The other reading
// would answer the same either way, and asking once keeps the two on one
// route.
func (p *Parser) aliasAtAFunctionName() bool {
	reading := p.dialect.AliasAtAFunctionName
	if reading == AliasExpandsAtAFunctionName || p.Aliases == nil {
		return false
	}
	if !p.at(TokWord) {
		return false
	}
	if reading == AliasSuppressedWhereTheParenIsAdjacent {
		if !p.peekIsFuncParensAdjacent() {
			return false
		}
	} else if !p.peekIsFuncParens() {
		return false
	}
	name := p.tok.Text
	if _, ok := p.Aliases(name); !ok {
		return false
	}
	if reading == AliasRefusesAFunctionName {
		p.lex.remarks = append(p.lex.remarks, Remark{
			Kind: RemarkFunctionNameIsAnAlias,
			Pos:  p.tok.Pos, At: p.tok.Pos, Token: name,
		})
		p.aliasFuncRefused = true
	}
	return true
}

// expandPipelineHead offers the word a pipeline begins with to the alias
// table, for the two words [Parser.parsePipeline] reads for itself.
//
// [Dialect.AliasesExpandReservedWords] governs every other reserved word
// without anything further, because every other one is read by parseCommand
// and parseCommand asks the table before it dispatches — which is what lets
// `alias iff='if true; then'` supply the keyword. A pipeline's leading `!`
// and the `time` in front of it are read one level out, before a command
// exists to be parsed, and nothing there asked: `alias '!'='echo took'`
// followed by `! true` printed nothing and answered 1, where bash 5.3, bash
// 3.2 and zsh each print `took true` and answer 0. The five columns that
// protect a reserved-word alias protect this one too, so it is the same field
// reaching one position further rather than an axis of its own (#2638).
//
// The reservation itself is not decided here. [Parser.expandAlias] owns it,
// for these two words as for every other, and a second copy of the condition
// in front of this call would be a second place to fix it — the mistake this
// tree has made four times. What this guard decides is *which words
// parsePipeline reads for itself*, which is a fact about the grammar: only
// those two can be answered before a command is parsed, and `time` only where
// the grammar has the keyword. Everywhere else `time` is an ordinary command
// name and parseCommand reads it.
//
// So an ordinary alias at the head of a pipeline takes the route it always
// took, through parseCommand, and the dialect is not consulted about it at
// all — see TestAnOrdinaryHeadAsksTheReservedWordFieldNothing.
//
// Nothing else `!` means is reachable from here. Negation is the only one
// parsePipeline reads; `[[ ! x ]]` is read by parseCond, `${!v}` by
// parseParamExp, and neither is a token this ever sees.
func (p *Parser) expandPipelineHead() {
	readsBang := p.atWord("!")
	readsTime := p.dialect.TimeKeyword && p.atWord("time")
	if !readsBang && !readsTime {
		return
	}
	p.expandCommandStart()
	// Whatever came of it, this token has been offered and parseCommand must
	// not offer it again. True even when the table declined — the word is
	// then still `!` or `time`, parsePipeline consumes it, and next clears
	// this before the word behind it is reached.
	p.aliasHeadHandled = true
}

// expandAlias replaces the current token when look says it names an alias,
// and keeps doing so while the replacement names another.
//
// done is the names already used, and is how the recursion stops: an alias is
// never expanded twice in one chain, so `alias echo='echo x'` gives `x hi`
// rather than looping, and `a` → `b x` → `a y x` leaves the inner `a` as an
// ordinary word that is then not found. Both measured, and unanimous.
//
// look is a parameter rather than always [Parser.Aliases] because the global
// kind is the same substitution asked at a different place: one loop, one
// splice, and the caller says which table and which set of spent names. A
// second copy of it for globals is exactly the duplication that has bitten
// this parser before.
func (p *Parser) expandAlias(done map[string]bool, look Aliases) {
	for p.aliasable(look) {
		name := p.tok.Text
		if done[name] || p.aliasChain[name] {
			// Spent in this command, or spent by an expansion this token is
			// still inside — `alias a='echo took;a'` reaches the second
			// reading and nothing else does. See Parser.pendingChains.
			return
		}
		if !p.dialect.AliasesExpandReservedWords && p.reservedInDialect(name) {
			// A name the grammar reserves keeps meaning what the grammar
			// says. The table still holds it — the `alias` builtin stored it
			// and lists it — and only the substitution is declined, which is
			// what the standard asks for and what both shells with a POSIX
			// mode do while they are in it. See
			// [Dialect.AliasesExpandReservedWords].
			return
		}
		value, ok := look(name)
		if !ok {
			return
		}
		done[name] = true
		p.spliceAlias(name, value)
	}
}

// expandGlobalAlias replaces the current token when it names a *global*
// alias — one expanded wherever a word stands rather than only where a
// command word does.
//
// Called from [Parser.next], which is what "wherever a word stands" means
// here: an argument, a `for` list, a `case` pattern, a redirection target, a
// heredoc delimiter, a word inside `[[ ]]`. All measured against the shell
// that has them, along with the negative half — a quoted word is not one, and
// an assignment `v=G` is a single word whose text is not the alias name, so
// neither expands.
//
// The set of spent names belongs to the *chain*, and fresh says whether this
// token begins one. A token straight from the lexer does; a token still being
// handed out from a splice does not, and carries the names its own expansion
// spent. Both halves are measured:
//
//   - `alias -g S=x` used twice in one command expands twice, where a regular
//     alias used twice in one command expands once — so the set cannot be the
//     command's;
//   - `alias -g f='echo f'` run as a command prints `f` rather than
//     recurring, and the inner `f` arrives from the splice — so the set
//     cannot be the token's either.
func (p *Parser) expandGlobalAlias(fresh bool) {
	if p.GlobalAliases == nil {
		return
	}
	if fresh {
		p.globalDone = clearedSet(p.globalDone)
	}
	if !p.aliasable(p.GlobalAliases) {
		return
	}
	// Asked before anything is allocated, so a word naming no global alias —
	// which is nearly every word — costs one lookup and nothing else. next is
	// on the keystroke path.
	if _, ok := p.GlobalAliases(p.tok.Text); !ok {
		return
	}
	if p.globalDone == nil {
		p.globalDone = map[string]bool{}
	}
	p.expandAlias(p.globalDone, p.GlobalAliases)
}

// clearedSet empties a set without throwing its storage away, and takes a nil
// one as already empty.
func clearedSet(m map[string]bool) map[string]bool {
	clear(m)
	return m
}

// expandSuffixAlias replaces a command word of the form `text.name` with the
// text `value text.name`, where `name` names a suffix alias.
//
// Measured: text must be non-empty — `.zsh` is not one, `a/.txt` is — the
// extension is the run after the *last* dot, and the word is appended to the
// value rather than consumed, so `alias -s txt=cat` turns `./x.txt` into `cat
// ./x.txt`. The value is spliced as text like any other alias body, so one
// holding a pipeline puts the filename after the pipeline's last command.
//
// A trailing space in the value is not special here, which comes free: the
// text spliced ends in the word.
//
// The word goes into done so the splice cannot find itself. Without it a
// value that leaves the same word standing in command position — an empty
// one does — matches the same suffix forever.
func (p *Parser) expandSuffixAlias(done map[string]bool) {
	if !p.aliasable(p.SuffixAliases) {
		return
	}
	word := p.tok.Text
	if done[word] {
		return
	}
	dot := strings.LastIndexByte(word, '.')
	if dot <= 0 {
		return
	}
	value, ok := p.SuffixAliases(word[dot+1:])
	if !ok {
		return
	}
	done[word] = true
	p.spliceAlias(word, value+" "+word)
}

// aliasable reports whether the current token could name an alias that look
// answers for.
//
// Unquoted words only: `"a"` is a command name and not an alias, unanimously.
// A quoted word is not the same word, which is the rule that lets a script
// call the real thing past an alias that shadows it. The lookup is by the
// token's source text, quotes and all, so a table holding `"a"` would match a
// quoted `"a"` without this — which is how the check is tested.
//
// The kind check is belt and braces where a command word stands, and load
// bearing where a global alias is looked for: that one is asked of every
// token the lexer hands over, so an operator really does reach here.
func (p *Parser) aliasable(look Aliases) bool {
	return look != nil && p.tok.Kind == TokWord && !p.tok.IsQuoted() && p.tok.Text != ""
}

// spliceAlias lexes an alias value and puts its tokens in front of the input,
// with the current token becoming the first of them.
//
// Every spliced token is given the position of the word it replaced. A word
// that came from an alias is reported where the alias was *used*, which is
// what all three shells that expand do.
//
// A value ending in a space makes the word after the expansion eligible too,
// which is the rule behind `alias sudo='sudo '`: the command after `sudo` is
// itself expanded. Unanimous, and it is why this is not simply "expand the
// command word".
func (p *Parser) spliceAlias(name, value string) {
	// The chain this body's tokens are inside: whatever the alias word was
	// already inside, plus this name. Built fresh rather than written
	// through, because the tokens already handed out hold the old one.
	chain := make(map[string]bool, len(p.aliasChain)+1)
	for n := range p.aliasChain {
		chain[n] = true
	}
	chain[name] = true
	at := p.tok.Pos
	end := p.tok.End
	// A comment the value opens and does not end reaches past the alias
	// word, because substitution is textual: the rest of the line the alias
	// word was written on is inside that comment. Spent before the body is
	// read, so the text each of the body's tokens is followed by is what is
	// really left. See Parser.spendCommentedTail.
	if strings.IndexByte(value, '#') >= 0 && valueOpensAComment(value, p.dialect) {
		p.spendCommentedTail()
	}
	sub := NewLexer(value, p.dialect)
	// The body's own newlines, where the dialect counts them: a token on the
	// body's second line is reported one line below the alias word, and
	// everything read after this expansion moves down by as many lines as the
	// body had. The offsets stay the alias word's, so the text a diagnostic
	// quotes is still text that is really there.
	counts := p.dialect.AliasBodyCountsLines
	var toks []Token
	// Whether each token touches the one before it, taken from the *body's*
	// own offsets and kept, because the loop below is about to overwrite
	// them with the alias word's. See Parser.pendingTouches for the one
	// question in the grammar that has no other way to ask.
	var touches []bool
	// The text that still follows each token: the rest of this body, and
	// then the rest of every body this one was spliced into. See
	// Parser.pendingTails.
	var tails []string
	// What stood after the alias word being replaced, across every body it
	// was inside. Read before the splice overwrites it, because that is the
	// text a construct this body leaves open has to reach before it reaches
	// the input.
	outer := p.tokTail
	// The here-document bodies this value opens, where the value — or the
	// text of the bodies it was spliced into — holds them whole. See
	// aliasHeredocBodies.
	bodies, from, took, skip := p.aliasHeredocBodies(value)
	if took > from {
		p.spendPending(from, took)
		outer = outer[:from] + outer[took:]
	}
	if skip > 0 {
		// And a body that went on into the input takes those lines from
		// the input's own lexer.
		p.lex.skipOver(skip)
	}
	lastEnd := -1
	// Where the body's last token began, in the body's own offsets, for the
	// one that may still be running when the body ends. See carryOpenWord.
	lastStart := 0
	var heredocOp Kind
	for {
		sub.inHeredocDelimiter = heredocOp != 0 && bodies != nil
		t := sub.Next()
		sub.inHeredocDelimiter = false
		if t.Kind == TokEOF {
			break
		}
		switch {
		case bodies == nil:
		case t.Kind.IsHeredoc():
			heredocOp = t.Kind
		case heredocOp != 0 && t.Kind == TokWord:
			// Queued on the value's own lexer as well, so the body lines the
			// value holds are skipped rather than read as words; the body
			// handed to the parser is the one read from the joined text.
			queueAliasHeredoc(sub, heredocOp, t)
			heredocOp = 0
			if len(bodies) > 0 {
				t.aliasBody, bodies = bodies[0], bodies[1:]
				for j := range t.aliasBody.Heredoc.Spans {
					t.aliasBody.Heredoc.Spans[j].Pos = at
				}
				t.aliasBody.Heredoc.Start, t.aliasBody.Heredoc.Stop = at, at
			}
		default:
			heredocOp = 0
		}
		touches = append(touches, int(t.Pos.Offset) == lastEnd)
		lastEnd = int(t.End.Offset)
		lastStart = int(t.Pos.Offset)
		tails = append(tails, value[lastEnd:]+outer)
		pos, stop := at, end
		if counts {
			pos.Line += t.Pos.Line - 1
			stop.Line += t.End.Line - 1
		}
		t.Pos, t.End = pos, stop
		toks = append(toks, t)
	}
	if counts {
		p.lex.shiftLines(strings.Count(value, "\n"))
		p.aliasLineShift += strings.Count(value, "\n")
	}
	// A value that is empty or all blanks leaves nothing behind, and the
	// command becomes whatever followed it.
	p.aliasNextWord = endsInBlank(value)
	if len(toks) == 0 {
		p.next()
		return
	}
	// The body's last token ended at the body's own edge, so whatever it was
	// may still have more to read. Which is a wider test than "the body's
	// lexer said it ran out", and deliberately: since #2704 a backslash the
	// input ends after is read as *part of the word* rather than as input
	// that ran out, so a body ending in one is complete by that lexer's
	// reckoning and still has a character that has escaped nothing (#2710).
	//
	// Whether there is a seam to cross at all is carryOpenWord's own
	// question, and it answers it by reading the join: a body whose last
	// token is finished takes none of the input and is left exactly as it
	// was lexed.
	carries := make([]string, len(toks))
	if lastEnd == len(value) {
		carries[len(toks)-1] = value[lastStart:]
	}
	p.aliasSource = value
	// The body itself, for the diagnostic that echoes the borrowed text
	// rather than the line the alias word was written on. Cleared by next()
	// once the last of these tokens has been handed out. (Assigned above,
	// before the carry, because a carry that fails reports through it.)
	p.pending = append(toks[1:], p.pending...)
	p.pendingTouches = append(touches[1:], p.pendingTouches...)
	p.pendingTails = append(tails[1:], p.pendingTails...)
	p.pendingCarries = append(carries[1:], p.pendingCarries...)
	chains := make([]map[string]bool, len(toks)-1)
	for i := range chains {
		chains[i] = chain
	}
	p.pendingChains = append(chains, p.pendingChains...)
	p.aliasChain = chain
	p.aliasSpliced = len(toks)
	p.tok = toks[0]
	p.tokTail = tails[0]
	if carries[0] != "" {
		// A one-token body, whose only token is the one that may still be
		// reading: it is current now, so there is nothing to defer.
		p.carryOpenWord(&p.tok, carries[0])
	}
	// The first token of a body replaces the alias word, which stood where
	// it stood: nothing about the body says it touches what came before.
	p.tokTouches = false
}

// aliasHeredocBodies reads the here-document bodies an alias value opens,
// where they are text of the value or of the bodies it was spliced into.
//
// Substitution is textual in every column: the value stands in the input
// where the alias word stood, and a body is read from the lines after the
// operator's. For a value holding newlines those are the value's own, so
// `alias hd='cat <<EOF` ⏎ `hello` ⏎ `EOF'` used as `hd` prints `hello` in
// bash 5.3, ksh93 and dash — where reading the body from the input after the
// alias word ran `hello` and `EOF` as commands. And for a value spliced into
// another, they may be the *outer* value's: `alias Y='cat <<\E'` with
// `alias X='Y` ⏎ `text` ⏎ `E'` used as `X` prints `text`.
//
// So the value and what follows it inside enclosing values are read as one
// text, and a body that ends inside it is taken. from and took bound the
// part of the enclosing values' text the bodies used, which the caller
// spends: from is where the bodies began — after the newline that ends the
// operator's line, which stays, since it ends the command — and took where
// they ended.
//
// And past them, the input: a body the value leaves unfinished goes on into
// the lines after the alias word, and skip is how much of the input it took.
// That is also what decides a delimiter written on the value's last line
// with no newline after it — `hd` alone on its line ends the body there, and
// `hd; echo x` makes the line `EOF; echo x`, which is body, in bash 5.3,
// ksh93 and dash alike.
//
// A body whose delimiter never arrives runs to the end of that joined text,
// which is the end of the input — the same thing it would have run to had it
// been written there — so it is taken like any other, and what the probe
// remarked about it is re-sited onto the input. See
// adoptAliasHeredocRemarks.
//
// Nil where the bodies begin in the input rather than in the alias text: the
// operator's line ends there and the input's own lexer reads them at its own
// newline. All or nothing, because the bodies of one line are read in order.
func (p *Parser) aliasHeredocBodies(value string) (bodies []*Redirect, from, took, skip int) {
	if !strings.Contains(value, "<<") {
		return nil, 0, 0, 0
	}
	// The seam between the value and what follows it. zsh 5.9.2 reads a
	// blank there unless one is already there — `alias hd='cat <<EOF` ⏎ `x`
	// ⏎ `EOF'` used as `hd` on a line of its own gives the body `x` ⏎ `EOF `
	// and runs on, a body line the value ends in the middle of gains one,
	// and `hd x` after a value ending mid-line gives `… x` with one blank
	// rather than two. It is the same blank that keeps a backslash ending a
	// value from joining the next line there, so the one field answers both.
	// See Dialect.AliasBodyBackslashJoinsTheNextLine.
	outer := p.tokTail
	after := outer + p.lex.src[p.lex.off:]
	seam := ""
	if !p.dialect.AliasBodyBackslashJoinsTheNextLine && (after == "" || !isBlank(after[0])) {
		seam = " "
	}
	head := len(value) + len(seam)
	text := value + seam + after
	begin := -1
	probe := NewLexer(text, p.dialect)
	var op Kind
	for {
		probe.inHeredocDelimiter = op != 0
		waiting := len(probe.pending) > 0
		t := probe.Next()
		probe.inHeredocDelimiter = false
		if t.Kind == TokEOF || probe.err != nil {
			break
		}
		if waiting && len(probe.pending) == 0 && begin < 0 {
			// The newline the first bodies were read at. Its own End is
			// past them.
			begin = int(t.Pos.Offset) + 1
		}
		switch {
		case int(t.Pos.Offset) >= len(value) && op == 0:
			// Past the value: only a body the value opened is its business,
			// and the tokens after it stay whoever's they were.
		case t.Kind.IsHeredoc():
			op = t.Kind
		case op != 0 && t.Kind == TokWord:
			bodies = append(bodies, queueAliasHeredoc(probe, op, t))
			op = 0
		default:
			op = 0
		}
		if int(t.End.Offset) >= len(value) && len(probe.pending) == 0 {
			break
		}
	}
	// Bodies that begin in the input are the ordinary route's: the operator's
	// line ends there, and the input's lexer reads them at its own newline.
	if len(bodies) == 0 || probe.err != nil || len(probe.pending) > 0 || begin > head+len(outer) {
		return nil, 0, 0, 0
	}
	end, ranOut := 0, false
	for _, r := range bodies {
		if r.Heredoc == nil {
			return nil, 0, 0, 0
		}
		// A body whose delimiter never arrived took everything to the end
		// of the joined text, which is the end of the input: the command
		// runs with what it has, and one column says so on the way past.
		ranOut = ranOut || r.HeredocAtEOF
		end = max(end, int(r.Heredoc.Stop.Offset))
	}
	from = min(max(0, begin-head), len(outer))
	took = min(max(0, end-head), len(outer))
	skip = max(0, end-head-len(outer))
	if ranOut {
		p.adoptAliasHeredocRemarks(probe.remarks, value, head+len(outer))
	}
	return bodies, from, took, skip
}

// adoptAliasHeredocRemarks moves what the probe said about a here-document
// that ran to the end of the joined text onto the input's own lexer, with
// its two positions re-sited from that text to the input.
//
// The probe reads the value, the values it was spliced into, and then the
// input as one text, so a body that runs out there is a body the *input* ran
// out inside — which is exactly what the one column that remarks on this
// says, and where it says it. Measured 2026-09-19 against bash 5.3.20 over
// `alias hd='cat <<EOF` / `in alias` / `EOF'` used as `hd; echo same`: the
// warning is located on the last line the input had and names the line the
// alias word stands on, whether the body's last line came from the value or
// from the input.
//
// alias is where the alias text ends in the joined text. An offset in front
// of it is text that is not in the input at all and stands where the alias
// word stands; one past it is the input's own, plus the lines every
// expansion so far has inserted above it — and this value's own, where the
// dialect counts them, because they are inserted at the alias word and this
// runs before spliceAlias shifts the lexer over them.
func (p *Parser) adoptAliasHeredocRemarks(remarks []Remark, value string, alias int) {
	// The input really did run out with a document still open, which is a
	// different question at a prompt than in a script: the front ends read
	// this to ask for another line. See Lexer.ranOut.
	p.lex.ranOut("<<")
	shift := p.aliasLineShift
	if p.dialect.AliasBodyCountsLines {
		shift += strings.Count(value, "\n")
	}
	for _, r := range remarks {
		if r.Kind != RemarkHeredocAtEOF {
			continue
		}
		// The operator is text of the value, so the construct the remark is
		// about began where the alias word did.
		r.At = p.tok.Pos
		if int(r.Pos.Offset) < alias {
			r.Pos = p.tok.Pos
		} else {
			r.Pos = p.lex.posAt(p.lex.off + int(r.Pos.Offset) - alias)
			r.Pos.Line += int32(shift)
		}
		p.lex.remarks = append(p.lex.remarks, r)
	}
}

// queueAliasHeredoc registers a here-document on a lexer reading alias text,
// from the delimiter token, the way the parser registers one on the input's.
func queueAliasHeredoc(l *Lexer, op Kind, delim Token) *Redirect {
	r := &Redirect{Op: op, Word: &Word{Spans: delim.Spans, Start: delim.Pos, Stop: delim.End}}
	l.queueHeredoc(r, delim.Text != delim.Literal())
	return r
}

// spendPending drops the pending tokens that stand for the text between from
// and took after the current token, which here-document bodies have used:
// they are inside a body now and are no longer words of their own. The tokens
// in front of that text stay, and what follows each of them no longer holds
// it. See carryOpenWord, which spends a body's text the same way.
func (p *Parser) spendPending(from, took int) {
	outer := p.tokTail
	n := 0
	for i := range p.pending {
		end := len(outer) - len(p.pendingTails[i])
		if end > from && end <= took {
			// Only ever called while a splice is being made, whose own count
			// replaces aliasSpliced once it is done, so nothing is counted
			// down here.
			continue
		}
		if end <= from {
			p.pendingTails[i] = outer[end:from] + outer[took:]
		}
		p.pending[n] = p.pending[i]
		p.pendingTouches[n] = p.pendingTouches[i]
		p.pendingChains[n] = p.pendingChains[i]
		p.pendingTails[n] = p.pendingTails[i]
		p.pendingCarries[n] = p.pendingCarries[i]
		n++
	}
	p.pending = p.pending[:n]
	p.pendingTouches = p.pendingTouches[:n]
	p.pendingChains = p.pendingChains[:n]
	p.pendingTails = p.pendingTails[:n]
	p.pendingCarries = p.pendingCarries[:n]
}

// carryOpenWord continues a construct the alias body opened and did not close
// over the text the body was substituted into.
//
// This is the one place the token model and the textual one part company, and
// the measurement says the text wins. Substitution replaces the alias word
// with the alias *value* in the input the shell is reading, and lexing carries
// on over the join — so a quote the body opens is still open when the rest of
// the line is read, and a quote the body leaves open at the end of the input
// is an unterminated quote. `alias q='echo "'` followed by `q hello"` prints
// ` hello` in dash, bash 5.3, that binary as `sh`, bash 3.2, ksh93, zsh and
// BusyBox ash; `alias a='echo "x'` followed by `a` is a refusal in all seven
// (#2685). A body lexed on its own can do neither, because its lexer starts
// and ends at the body's edges: the quote could not enter it and could not
// leave it, and the second half is the worse one — closing a quote the script
// never closed runs a command the author did not write.
//
// The join is read by a lexer over the body's unfinished tail and everything
// that follows it, which is the only arrangement in which one construct can
// span the two. What it consumed is then spent: text that belonged to an
// enclosing body is spent by dropping the pending tokens it stands for, and
// text of the input by skipping it in the input's own lexer. So every token
// after this one is read from the real text at the real position, and the
// carried token keeps the alias word's start — the position every spliced
// token carries — and ends where the input's lexer now stands.
//
// It is called when the token is *handed out* rather than when the body is
// spliced, because a body's own tokenization is not final while an alias word
// stands earlier in it. See [Parser.pendingCarries].
//
// tail is the body's last token, which is the one that was still being read
// when the body ran out; last is where it has been put in the splice.
func (p *Parser) carryOpenWord(last *Token, tail string) {
	// What follows the token, across every body it is inside and then the
	// input. The two halves are kept apart because what is done with them
	// differs: text of a body is spent by dropping the pending tokens it
	// stands for, and text of the input is spent by moving the input's own
	// lexer. See Parser.pendingTails.
	outer := p.tokTail
	rest := outer + p.lex.src[p.lex.off:]
	if !p.dialect.AliasBodyBackslashJoinsTheNextLine &&
		endsInLoneBackslash(tail) && strings.HasPrefix(rest, "\n") {
		// A backslash the body ends with, with nothing after the alias word
		// for it to escape but the newline. That is an ordinary line
		// continuation where the dialect takes it and nothing where it does
		// not, and the panel splits five to two. See
		// [Dialect.AliasBodyBackslashJoinsTheNextLine]; the carry below is
		// the same backslash meeting anything else, which is not split.
		return
	}
	join := NewLexer(tail+rest, p.dialect)
	t := join.Next()
	if !join.Incomplete() && int(t.End.Offset) <= len(tail) {
		// The joined reading finished without taking any of the input, so
		// there was no seam to cross: the route that ends an unterminated
		// quote at the end of the input has already ended this one, and the
		// body stands as it was lexed. See [Dialect.CloseQuotesAtEOF].
		return
	}
	// The blank an unfinished body may end in is inside the construct rather
	// than between two words. The word the construct swallows is never a
	// candidate — `alias q='echo "x '` used as `q b"` is `x  b` in all
	// seven columns and never the expansion of `b` — and whether the word
	// *past* the construct is one splits the panel three to four. See
	// [Dialect.AliasTrailingBlankReachesPastAnOpenConstruct].
	if !p.dialect.AliasTrailingBlankReachesPastAnOpenConstruct {
		p.aliasNextWord = false
	}
	// How much of what followed the token the joined reading took.
	took := len(rest)
	if !join.Incomplete() {
		took = int(t.End.Offset) - len(tail)
	}
	// The part of it that was another body's text is spent by dropping the
	// pending tokens that text stands for — they are inside the construct
	// now and are no longer words of their own. pendingTails[0] is what
	// follows the first of them, so what it *is* runs from there back to the
	// end of outer.
	for len(p.pending) > 0 && took >= len(outer)-len(p.pendingTails[0]) {
		p.pending = p.pending[1:]
		p.pendingTouches = p.pendingTouches[1:]
		p.pendingChains = p.pendingChains[1:]
		p.pendingTails = p.pendingTails[1:]
		p.pendingCarries = p.pendingCarries[1:]
		if p.aliasSpliced > 0 {
			p.aliasSpliced--
		}
	}
	if took > len(outer) {
		// And it reached the input, so the input's own lexer moves over what
		// was taken there. The construct crossed both seams; p.tokTail is
		// spent either way, because nothing of it is left to follow this
		// token.
		p.lex.skipOver(took - len(outer))
	}
	p.tokTail = outer[min(took, len(outer)):]
	if join.Incomplete() {
		// Nothing in the input closes it either. The construct has swallowed
		// the rest of the file, which is what every column reports and is the
		// whole of the second half of #2685.
		p.lex.adoptOpenConstruct(join, len(tail), last.Pos)
	}
	t.Pos, t.End = last.Pos, p.lex.pos()
	*last = t
}

// valueOpensAComment reports whether reading an alias value leaves a comment
// open at the end of it.
//
// A read of its own rather than a question asked of the body's own lexer
// afterwards, because the answer is needed *before* the body is read: what
// follows each of the body's tokens is the text after the alias word, and a
// comment that swallowed some of that text has to have swallowed it already.
//
// Only ever reached for a value holding a `#`, which almost none do.
func valueOpensAComment(value string, d Dialect) bool {
	l := NewLexer(value, d)
	for {
		if t := l.Next(); t.Kind == TokEOF {
			return l.CommentRanToTheEnd()
		}
	}
}

// spendCommentedTail takes the rest of the line out of the text that follows
// the alias word, across every body the word was inside and then the input.
//
// The same spending carryOpenWord does for a construct that crosses the seam,
// and for the same reason: a comment the value opened is still open when what
// follows the alias word is read. See Lexer.CommentRanToTheEnd for the panel.
func (p *Parser) spendCommentedTail() {
	outer := p.tokTail
	rest := outer + p.lex.src[p.lex.off:]
	took := len(rest)
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		took = i
	}
	for len(p.pending) > 0 && took >= len(outer)-len(p.pendingTails[0]) {
		p.pending = p.pending[1:]
		p.pendingTouches = p.pendingTouches[1:]
		p.pendingChains = p.pendingChains[1:]
		p.pendingTails = p.pendingTails[1:]
		p.pendingCarries = p.pendingCarries[1:]
		if p.aliasSpliced > 0 {
			p.aliasSpliced--
		}
	}
	if took > len(outer) {
		p.lex.skipOver(took - len(outer))
	}
	p.tokTail = outer[min(took, len(outer)):]
}

// endsInLoneBackslash reports whether s ends with a backslash that escapes
// whatever comes next rather than one that is itself escaped.
//
// Counted rather than tested, because `a\\` ends in a backslash and that
// backslash is a character the pair in front of it already spent.
func endsInLoneBackslash(s string) bool {
	n := 0
	for n < len(s) && s[len(s)-1-n] == '\\' {
		n++
	}
	return n%2 == 1
}

// endsInBlank reports whether an alias value ends in a space or a tab, which
// is what makes the next word eligible for expansion in turn.
func endsInBlank(s string) bool {
	if s == "" {
		return false
	}
	c := s[len(s)-1]
	return c == ' ' || c == '\t'
}
