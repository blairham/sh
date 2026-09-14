// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// opensACompoundVariableBody reports whether the token standing just inside a
// literal's `(` makes the parentheses a compound variable's body rather than a
// list of array elements.
//
// A pure lookahead: nothing is consumed, because the answer no is the array
// reading and that reading starts from this same token.
//
// The first word decides, and it decides on what was *written*. A quoted
// assignment is not one — `c=("a=1")` is an array of the one string — and
// neither is one that arrives from an expansion, which is why this asks
// isAssign rather than looking for an `=` in the expanded value. See
// [Dialect.CompoundVariableDeclarators] for the measured rows.
func (p *Parser) opensACompoundVariableBody() bool {
	if len(p.dialect.CompoundVariableDeclarators) == 0 || p.tok.Kind != TokWord {
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
	var body []*SimpleCmd
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
			if !p.opensACompoundVariableBody() {
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
