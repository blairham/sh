// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// compoundLiteralReading is what a declaration command's own letters have
// already settled about the parentheses of its operand's literal, before the
// first word inside them is looked at.
//
// The letters reach the *parse* and not only the store, which is measured
// rather than reasoned — ksh93u+ 2012-08-01, 2026-09-13:
//
//	written                       typeset -p c
//	c=(a=1 b=2)                   typeset -C c=(a=1;b=2)
//	typeset -a c=(a=1 b=2)        typeset -a c=(a\=1 b\=2)
//	typeset -A c=(a=1 b=2)        typeset -A c=([0]=(a=1;b=2))
//	typeset -A c=(x y)            cannot append index array to …
//	typeset -A c=([k]=v)          typeset -A c=([k]=v)
//	typeset -A c=()               typeset -A c=()
//	typeset -a c=(a=1; b=2)       syntax error: `b=2' unexpected
//	typeset -C c=(x y)            syntax error: `x' unexpected
//	typeset -a c=()               typeset -a c
//	typeset -C c=()               typeset -C c=()
//	c=()                          typeset -C c=()
//
// The last three rows are the whole of why this is a parse question: the two
// readings take a `;` differently and an *empty* literal has no word in it to
// decide with, so what the reading is has to be known before the parentheses
// are read rather than after.
type compoundLiteralReading uint8

const (
	// literalWordDecides is the ordinary case: nothing has settled the
	// reading, so the first word inside the parentheses does — and an empty
	// pair of them is a compound where the dialect has the construct.
	literalWordDecides compoundLiteralReading = iota
	// literalIsAnArray is an `-a` letter on the declaration, which takes the
	// compound reading off the table whatever is written inside.
	literalIsAnArray
	// literalIsATable is an `-A` letter, which does **not**: the word still
	// decides, and only an *empty* pair of parentheses is settled by the
	// letter. See [Parser.opensACompoundVariableBody] for the measured rows,
	// and the table above for the pair that separates the two letters.
	literalIsATable
	// literalIsACompound is a `-C` letter, which puts it on and leaves the
	// body with nothing else it may hold.
	literalIsACompound
)

// declarationLiteralReading reads the container letters off the words a
// declaration command has been given so far.
//
// `-C` wins over `-a` and `-A` where a line writes both, which is arbitrary
// only in the shape `typeset -C -A d=c` — measured `typeset -C -A d=()`
// there, a name with both letters and no value, which is neither reading's
// answer and is not a shape worth an axis. Every letter is read from a word
// that is literal, unquoted and begins with `-` or `+`, and the scan stops at
// the first word that is not: an operand's own value may hold any character,
// and reading letters out of it would let `typeset q=-a` change a grammar.
func declarationLiteralReading(args []*Word) compoundLiteralReading {
	reading := literalWordDecides
	for _, w := range args[1:] {
		if len(w.Spans) != 1 || w.Spans[0].Kind != Literal ||
			w.Spans[0].Quoting != Unquoted {
			return reading
		}
		text := w.Spans[0].Value
		if len(text) < 2 || (text[0] != '-' && text[0] != '+') {
			return reading
		}
		if text == "--" {
			return reading
		}
		for i := 1; i < len(text); i++ {
			switch text[i] {
			case 'C':
				return literalIsACompound
			case 'a':
				reading = literalIsAnArray
			case 'A':
				reading = literalIsATable
			}
		}
	}
	return reading
}

// opensACompoundVariableBody reports whether the parentheses a literal has
// just opened hold a compound variable's body rather than a list of array
// elements.
//
// A pure lookahead: nothing is consumed, because the answer no is the array
// reading and that reading starts from this same token.
//
// Three things can decide it, in this order. The declaration's own letters,
// where there are any — see [compoundLiteralReading]. Then an **empty** pair
// of parentheses, which is a compound wherever the dialect has the construct:
// `c=()` is `typeset -C c=()` on ksh93u+, `${#c[@]}` answers 1 and a later
// `c+=(x y)` starts at subscript 1, and a prior `typeset -a c` does not change
// it. And otherwise the first word, on what was *written*: a quoted assignment
// is not one — `c=("a=1")` is an array of the one string — and neither is one
// that arrives from an expansion, which is why this asks isAssign rather than
// looking for an `=` in the expanded value. See
// [Dialect.CompoundVariableDeclarators] for the measured rows.
func (p *Parser) opensACompoundVariableBody(reading compoundLiteralReading) bool {
	if len(p.dialect.CompoundVariableDeclarators) == 0 {
		return false
	}
	switch reading {
	case literalIsAnArray:
		return false
	case literalIsACompound:
		return true
	case literalIsATable:
		// The `A` letter forbids a *bare-word* literal and not a compound
		// one, which is the half that separates it from `a`. Measured on
		// ksh93u+ 2012-08-01, 2026-09-19:
		//
		//	typeset -A c=(a=1 b=2)   typeset -A c=([0]=(a=1;b=2))
		//	typeset -a c=(a=1 b=2)   typeset -a c=(a\=1 b\=2)
		//	typeset -A c=(x y)       cannot append index array to …
		//	typeset -A c=([k]=v)     typeset -A c=([k]=v)
		//	typeset -A c=()          typeset -A c=()
		//
		// So the word decides under `-A` exactly as it decides with no
		// letter at all, and the one row the letter does settle is the
		// empty pair: `c=()` with no letter is `typeset -C c=()` and
		// `typeset -A c=()` is the empty table. Falls through to the word
		// below, with that row taken out first.
		if p.at(TokRightParen) {
			return false
		}
		return p.atACompoundBodyItem()
	}
	if p.at(TokRightParen) {
		return true
	}
	return p.atACompoundBodyItem()
}

// atACompoundBodyItem reports whether the current token may begin one
// declaration of a compound variable's body: an assignment, or a declarator
// word.
//
// The raw test, with no declaration letters in it — which is what keeps
// `typeset -C c=(x y)` a syntax error rather than a body whose first item is
// the word `x`. See [Parser.compoundVariableItem], its only other caller.
func (p *Parser) atACompoundBodyItem() bool {
	if p.tok.Kind != TokWord {
		return false
	}
	if _, ok := p.isAssign(p.tok); ok {
		return true
	}
	// A declaration command word, which must be literal and unquoted for the
	// same reason the assignment must: `c=('typeset' x=1)` is a word rather
	// than a declaration.
	if len(p.tok.Spans) != 1 || p.tok.Spans[0].Kind != Literal ||
		p.tok.Spans[0].Quoting != Unquoted {
		return false
	}
	return p.dialect.CompoundVariableDeclarators[p.tok.Spans[0].Value]
}

// compoundVariableBody reads the declarations between a compound variable
// literal's parentheses, the caller having already stepped past the `(` and
// established that a declaration stands there.
//
// The body is a list of declarations and not a command list, which is measured
// rather than assumed: `c=(a=1; echo mid; b=2)` is “ `echo' unexpected “ in
// the shell that has the construct, and so is an `&&` between two of them. So
// the items are separated by `;` and newlines alone, each one needs something
// in front of it — `c=(; a=1)` and `c=(a=1; ; b=2)` are both “ `;' unexpected “
// — and a `;` after the last is allowed and separates nothing.
func (p *Parser) compoundVariableBody() []*SimpleCmd {
	// Empty and not nil, because an empty compound is a compound: `c=()` is
	// `typeset -C c=()` there and an array with nothing in it is a different
	// value. [Assign.Members] being nil is what says the literal was read as
	// an array, so a body with no items in it must still be a slice.
	body := []*SimpleCmd{}
	for p.err == nil {
		p.skipNewlines()
		if p.at(TokRightParen) {
			return body
		}
		item := p.compoundVariableItem()
		if p.err != nil || item == nil {
			return body
		}
		body = append(body, item)
		if p.at(TokSemi) {
			p.next()
			continue
		}
		if p.at(TokNewline) {
			continue
		}
		return body
	}
	return body
}

// compoundVariableItem reads one declaration of a compound variable's body.
//
// Two shapes, and they are the two a simple command already has: bare
// assignments — `a=1 b=2`, which is one item setting two members — and a
// declaration command with its options and operands, `typeset -i n=5`. The
// node is a SimpleCmd because that is what was written; nothing in it is a
// command that will run.
func (p *Parser) compoundVariableItem() *SimpleCmd {
	c := &SimpleCmd{Start: p.tok.Pos}
	for p.err == nil && p.at(TokWord) {
		if h, ok := p.isAssign(p.tok); ok && len(c.Args) == 0 {
			// An assignment in front of any command word is a member being
			// set. After one, `typeset -i n=5 m=6` has its own operands and
			// they go through the declaration path below.
			a := p.parseAssign(h)
			if a == nil {
				return nil
			}
			c.Assigns = append(c.Assigns, a)
			c.Stop = a.Stop
			continue
		}
		if len(c.Args) == 0 {
			// Neither an assignment nor a declarator, where an item begins:
			// the body holds declarations and this is not one. Named rather
			// than described, which is the shell's own wording —
			// `c=(a=1; echo mid)` is `` `echo' unexpected ``.
			if !p.atACompoundBodyItem() {
				p.failUnexpected("")
				return nil
			}
			c.Args = append(c.Args, p.word())
			c.Stop = p.tok.Pos
			continue
		}
		// An operand of the declaration. The array-valued spelling —
		// `typeset -a q=(1 2)` — is the one a declaration utility already
		// reads as an assignment rather than as a word.
		if a, consumed := p.declarationArray(c); consumed {
			if a != nil {
				c.Assigns = append(c.Assigns, a)
				c.Stop = a.Stop
			}
			continue
		}
		w := p.word()
		if w == nil {
			return nil
		}
		c.Args = append(c.Args, w)
		c.Stop = w.End()
	}
	if len(c.Args) == 0 && len(c.Assigns) == 0 {
		return nil
	}
	return c
}

// memberPath takes the dotted member path off the front of s — the text a
// name carries *after* a subscript's closing bracket — and returns it with
// its leading dot, along with what is left.
//
// `a[1].p=9` and `${a[1].p}` both reach this with `.p=9` and `.p}` in hand,
// and both get back `.p`. A path may be several links deep — `.q.r` — because
// a member may itself be a compound, measured: `a[1]=(p=1 q=(r=2))` then
// `${a[1].q.r}` answers `2` on ksh93u+ 2012-08-01.
//
// It is needed only after a `]`. Before one the whole thing is a single name,
// because `.` is a name byte where the dialect has the construct and `c.q.r`
// is read as one name by [Dialect.DottedName] — which is why this is not a
// second spelling of that rule but the piece it cannot reach.
//
// A dot with nothing readable after it yields no path at all, so the text
// stays whatever it was and the caller refuses it where it stands:
// `a[1].=9` is not an assignment, exactly as `a[1]x=9` is not.
func memberPath(s string, dot bool) (member, rest string) {
	if !dot {
		return "", s
	}
	end := 0
	for end < len(s) && s[end] == '.' {
		link := end + 1
		for link < len(s) && nameByte(s[link], link-(end+1), false) {
			link++
		}
		if link == end+1 {
			break
		}
		end = link
	}
	return s[:end], s[end:]
}
