// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// Alias expansion, which happens when a line is *read* and not when it runs.
//
// That is why it lives here rather than in the interpreter: an alias may hold
// a keyword, so `alias iff='if true; then'` followed by `iff echo yes; fi`
// prints `yes`. Nothing that substitutes at execution time can do that,
// because by then the grammar has already been decided.
//
// It is a token-level substitution rather than a textual splice, and that
// choice is measured rather than assumed. Every diagnostic about a command
// that came from an alias names the line the *alias word* was written on,
// never a line inside the alias body — bash, dash and ksh93 all report line 3
// for `alias bad=nosuchcmd` on line 2 used on line 3, and `$LINENO` inside a
// body reads the same. So a token from an expansion carries the position of
// the word it replaced, every position still points into the real input, and
// there is no position mapping to keep.
//
// The one place the two models differ from outside is a body containing a
// newline: dash, ksh93 and zsh count it and every later line shifts by one,
// where bash does not. Measured 2026-09-05 with $LINENO on the line after a
// two-line alias body — 5 in bash, 6 in the other three, against a physical
// line 5. This parser does not count it either, so it matches bash and
// diverges from the two dialects it expands aliases for; recorded rather
// than fixed here, and there is no axis for it yet. See
// docs/spec/grammar/tokenization.md and issue #583.

// Aliases answers whether a word names an alias, and what it stands for.
//
// The parser holds no table of its own: the table belongs to the shell, is
// changed by the `alias` builtin while the script runs, and is therefore the
// caller's to keep. A nil Aliases expands nothing, which is what a shell that
// does not expand them in this context supplies.
type Aliases func(name string) (value string, ok bool)

// expandAlias replaces the current token when it names an alias, and keeps
// doing so while the replacement names another.
//
// done is the names already used in this command, and is how the recursion
// stops: an alias is never expanded twice in one command, so `alias
// echo='echo x'` gives `x hi` rather than looping, and `a` → `b x` → `a y x`
// leaves the inner `a` as an ordinary word that is then not found. Both
// measured, and unanimous.
func (p *Parser) expandAlias(done map[string]bool) {
	for p.aliasable() {
		name := p.tok.Text
		if done[name] {
			return
		}
		value, ok := p.Aliases(name)
		if !ok {
			return
		}
		done[name] = true
		p.spliceAlias(value)
	}
}

// aliasable reports whether the current token could name an alias.
//
// Unquoted words only: `"a"` is a command name and not an alias, unanimously.
// A quoted word is not the same word, which is the rule that lets a script
// call the real thing past an alias that shadows it. The lookup is by the
// token's source text, quotes and all, so a table holding `"a"` would match a
// quoted `"a"` without this — which is how the check is tested.
//
// The kind check is belt and braces: both callers are positions where a word
// is the only thing that can be, so no operator ever reaches here. It says
// what may be expanded rather than relying on where this happens to be
// called from.
func (p *Parser) aliasable() bool {
	return p.Aliases != nil && p.tok.Kind == TokWord && !p.tok.IsQuoted() && p.tok.Text != ""
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
	var toks []Token
	for {
		t := sub.Next()
		if t.Kind == TokEOF {
			break
		}
		t.Pos, t.End = at, end
		toks = append(toks, t)
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
