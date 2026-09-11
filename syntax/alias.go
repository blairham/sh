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
		if done[name] {
			return
		}
		value, ok := look(name)
		if !ok {
			return
		}
		done[name] = true
		p.spliceAlias(value)
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
	p.spliceAlias(value + " " + word)
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
func (p *Parser) spliceAlias(value string) {
	at := p.tok.Pos
	end := p.tok.End
	sub := NewLexer(value, p.dialect)
	// The body's own newlines, where the dialect counts them: a token on the
	// body's second line is reported one line below the alias word, and
	// everything read after this expansion moves down by as many lines as the
	// body had. The offsets stay the alias word's, so the text a diagnostic
	// quotes is still text that is really there.
	counts := p.dialect.AliasBodyCountsLines
	var toks []Token
	for {
		t := sub.Next()
		if t.Kind == TokEOF {
			break
		}
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
	p.pending = append(toks[1:], p.pending...)
	p.aliasSpliced = len(toks)
	p.tok = toks[0]
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
