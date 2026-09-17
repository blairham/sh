// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// One dialect reads `test`'s operands as a single expression taken off the
// *front* of the list and never looks at the words behind it. Every other
// column reads the whole list and refuses what is left over — the sentence
// `too many arguments`, which this column has not got at all.
//
// That is one rule, not a leniency, and it has a seam in it: the moment the
// word straight after the reading is a connective, the whole list becomes
// load-bearing and a word it cannot place is `incorrect syntax`. Both
// spellings below hold the same words and only the connective has moved:
//
//	test x = x junk -a junk        0, silent — nothing past `x = x` is read
//	test x = x -a x = x junk       incorrect syntax, 2
//
// See Semantics.TestReadsOneExpressionOffTheOperands for the measurement and
// for why the rest of this file is a second reader rather than a flag on the
// first: the two disagree about what a *primary* is, not only about what to
// do with the words after one, and a shared reader with six exceptions in it
// would be harder to check against the panel than two readers are.
//
// Measured on 93u+ 2012-08-01 across 152 operand lists on 2026-09-16, each
// run as a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin
// on /dev/null and in a fresh directory — that shell's preset aliases make an
// interactive probe answer a different question.

// frontExpr is the reading: fixed arity up to four operands, a grammar past
// them, and words behind the reading that are never looked at.
//
// Four is where the grammar starts and not three, which two rows pin. At
// three, `test -n x y` is 0 — the two-word file test, with the third word
// dropped — and `test -z "" -a` is 0 for the same reason, where the grammar
// would take the trailing connective and complain that nothing follows it.
// At four the same trailing connective *is* taken: `test x = x -a` is
// `argument expected`.
func (r *Runner) frontExpr(form testForm, args []string) (bool, error) {
	switch len(args) {
	case 0:
		return false, nil
	case 1:
		return r.testOneOperand(args[0])
	case 2:
		return r.frontTwoOperands(form, args[0], args[1])
	case 3:
		return r.frontThreeOperands(form, args)
	case 4:
		// The parenthesised two-word form, and it is the *only* shortcut at
		// four: `test ( x y )` is `argument expected`, which is what the
		// two-word reading of `x y` says, where the grammar would have
		// blamed the `y` standing in an operator's place.
		//
		// There is deliberately no `!` shortcut beside it, because the pair
		// `test ! x -a ""` measures its absence: 1, which is `(not x) and
		// ""`, where negating the three-word reading of `x -a ""` would have
		// been 0. `!` is read by the grammar below like any other word.
		if args[0] == "(" && args[3] == ")" {
			return r.frontTwoOperands(form, args[1], args[2])
		}
	}
	p := &frontParser{r: r, form: form, args: args}
	v, err := p.orExpr()
	if err != nil {
		return false, err
	}
	if p.pos != len(p.args) && p.joined {
		// The seam. Words are left and a connective was taken between two
		// primaries, so the list was an expression the grammar could not
		// finish rather than an expression with junk behind it.
		//
		// Only a connective at the *top* of the reading counts: `test ( x =
		// x -a y = z ) junk` is 1, so the one inside the group did not make
		// the trailing word load-bearing.
		return false, &testError{kind: errIncorrectSyntax}
	}
	return v, nil
}

// frontTwoOperands is the two-word reading: an operator and its operand.
//
// The complaint when the first word is not an operator is the half that
// differs from every other column, and it turns on *shape*. A word spelled
// like a one-letter operator is named — `test -Q x` is `-Q: unknown
// operator`, and so is `test - x` — while a word that is not spelled like
// one leaves the shell with nothing to name and it says the form ran out
// instead: `test x junk`, `test -QQ x` and `test !! x` are all `argument
// expected`.
func (r *Runner) frontTwoOperands(form testForm, op, operand string) (bool, error) {
	if op == "!" {
		if operand == "-t" && r.bareTerminalTest() {
			on, err := r.unaryTest("-t", "1")
			return !on, err
		}
		return operand == "", nil
	}
	if !r.isTestUnary(op) && !frontOperatorShaped(op) {
		return false, &testError{kind: errOperandExpected}
	}
	return r.unaryTest(op, operand)
}

// frontThreeOperands is the three-word reading, and the order of its five
// questions is the whole of it. Each step below is pinned by a row that
// moves if the step is taken later:
//
//	test ! -a /       1        a negated file test, not `!` and `/` joined
//	test -n = x       1        a comparison, not `-n` applied to `=`
//	test -z -a y      1        `-z` applied to `-a`, not `-z` and `y` joined
//	test x -a y       0        two strings joined
//	test ( x )        0        the parenthesised one-word reading
//	test ( = )        1        except where the middle word is a comparison
//
// The third word of the file-test reading is never looked at: `test -n x y`
// is 0, and so is `test -z "" -a`.
func (r *Runner) frontThreeOperands(form testForm, args []string) (bool, error) {
	if args[0] == "!" && r.isTestUnary(args[1]) {
		v, err := r.unaryTest(args[1], args[2])
		return !v, err
	}
	connective := args[1] == form.and || args[1] == form.or
	if !connective {
		if ok, err, handled := r.binaryTest(form, args[0], args[1], args[2]); handled {
			return ok, err
		}
	}
	if r.isTestUnary(args[0]) {
		return r.unaryTest(args[0], args[1])
	}
	if connective {
		left, right := args[0] != "", args[2] != ""
		if args[1] == form.and {
			return left && right, nil
		}
		return left || right, nil
	}
	if args[0] == "(" && args[2] == ")" {
		return args[1] != "", nil
	}
	// Three words are a comparison that had no operator, so the middle word
	// is the one blamed — unless the first is spelled like a one-letter
	// operator, in which case the reading it asked for is the one that
	// failed. `test a b c` is `b: unknown operator` and `test -Q x y` is
	// `-Q: unknown operator`, while `test -QQ x y` is `x` again.
	if frontOperatorShaped(args[0]) {
		return false, &testError{kind: errUnaryExpected, operand: args[0]}
	}
	return false, &testError{kind: errBinaryExpected, operand: args[1]}
}

// frontOperatorShaped is a word spelled the way this shell's operators are:
// a dash and at most one character after it.
//
// The boundary is measured rather than assumed. `-Q` and a bare `-` are
// named as operators nobody has; `-QQ`, `--foo` and `-Q-` are not, and the
// complaint about them is the one for a form with nothing to name.
func frontOperatorShaped(word string) bool {
	return len(word) >= 1 && len(word) <= 2 && word[0] == '-'
}

// frontParser is the grammar past four operands: `-a` binds tighter than
// `-o`, `!` binds tighter still, and `( )` groups.
//
// The primary reader is the part that is not the usual one, and joined is
// the seam — see primary and frontExpr.
type frontParser struct {
	r    *Runner
	form testForm
	args []string
	pos  int
	// depth counts the groups the reading is inside, so that a connective
	// taken within one does not arm the seam.
	depth int
	// joined records a connective taken between two top-level primaries,
	// which is what makes the words after the reading load-bearing.
	joined bool
}

func (p *frontParser) more() bool { return p.pos < len(p.args) }

func (p *frontParser) peek() string {
	if p.more() {
		return p.args[p.pos]
	}
	return ""
}

func (p *frontParser) orExpr() (bool, error) {
	left, err := p.andExpr()
	if err != nil {
		return false, err
	}
	for p.more() && p.peek() == p.form.or {
		p.pos++
		if p.depth == 0 {
			p.joined = true
		}
		right, err := p.andExpr()
		if err != nil {
			return false, err
		}
		left = left || right
	}
	return left, nil
}

func (p *frontParser) andExpr() (bool, error) {
	left, err := p.notExpr()
	if err != nil {
		return false, err
	}
	for p.more() && p.peek() == p.form.and {
		p.pos++
		if p.depth == 0 {
			p.joined = true
		}
		right, err := p.notExpr()
		if err != nil {
			return false, err
		}
		left = left && right
	}
	return left, nil
}

func (p *frontParser) notExpr() (bool, error) {
	if p.more() && p.peek() == "!" {
		p.pos++
		v, err := p.notExpr()
		return !v, err
	}
	return p.primary()
}

// primary is one term, and how many words it takes is decided by how many
// are left *to the end of the whole list* rather than to the next connective
// or closing paren. That is not a detail: it is why `test ( x ) junk` is
// `): unknown operator`. Inside the group three words remain — `x`, `)` and
// `junk` — so the reading asked for is a comparison, and the `)` is standing
// where its operator belonged.
//
// The order of the readings is measured the same way the three-word one is:
//
//	test -n = x y     1        a comparison wins over the file test
//	test -n -a y z    0        but a connective in that place does not
//	test -z -a y z    1        — it is the file test's operand instead
//	test x -a y z     argument expected, because `x` alone is the primary
//	                           and `y z` after the connective is a two-word
//	                           reading with no operator in it
//	test x y -a z     y: unknown operator, because `y` is not a connective
//	                           and three words remain
func (p *frontParser) primary() (bool, error) {
	rest := len(p.args) - p.pos
	if rest == 0 {
		return false, &testError{kind: errOperandExpected}
	}
	if p.peek() == "(" {
		p.pos++
		p.depth++
		v, err := p.orExpr()
		p.depth--
		if err != nil {
			return false, err
		}
		if !p.more() {
			// The list ended with the group still open: `test ( x = x` and
			// `test -n x -a (` are both `argument expected`.
			return false, &testError{kind: errOperandExpected}
		}
		if p.peek() != ")" {
			// A word where the group should have closed, which is a list the
			// grammar could not finish: `test ( x = x z )`.
			return false, &testError{kind: errIncorrectSyntax}
		}
		p.pos++
		return v, nil
	}

	word := p.args[p.pos]
	next := ""
	if rest >= 2 {
		next = p.args[p.pos+1]
	}
	isConnective := rest >= 2 && (next == p.form.and || next == p.form.or)
	if rest >= 3 && !isConnective {
		if ok, err, handled := p.r.binaryTest(p.form, word, next, p.args[p.pos+2]); handled {
			p.pos += 3
			return ok, err
		}
	}
	if rest >= 2 && p.r.isTestUnary(word) {
		v, err := p.r.unaryTest(word, next)
		p.pos += 2
		return v, err
	}
	if isConnective {
		// A word this shell cannot read as an operator, with a connective
		// right behind it: the primary is the word alone and it is true when
		// it is not empty. `test -Q -a y z` and `test x -a y z` both get as
		// far as the connective this way.
		p.pos++
		return word != "", nil
	}
	if rest >= 3 {
		return false, &testError{kind: errBinaryExpected, operand: next}
	}
	if rest == 2 {
		if frontOperatorShaped(word) {
			return false, &testError{kind: errUnaryExpected, operand: word}
		}
		return false, &testError{kind: errOperandExpected}
	}
	// One word left. An operator is a form that ran out — `test -n x -a -n`
	// is `argument expected` — and anything else is a string, including a
	// word merely spelled like an operator: `test -n x -a -Q` is 0.
	if p.r.isTestUnary(word) {
		return false, &testError{kind: errOperandExpected}
	}
	p.pos++
	return word != "", nil
}
