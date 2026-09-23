// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/blairham/sh/syntax"
)

// condStatus is a condition operand that produced neither true nor false but
// a status of its own — `[[ -o name ]]` in the one dialect that refuses a
// name it does not have.
//
// An error rather than a third boolean because that is what already
// propagates the way the shells were measured to: `!` leaves it alone, which
// falls out of CondNot returning what it was given, and `&&` stops on it,
// which falls out of the short-circuit. Only `||` had to learn it.
//
// Error is never rendered — the complaint is written where the status is
// produced, because it is said even in the arrangements where the status is
// then thrown away.
type condStatus struct{ code int }

func (c condStatus) Error() string { return "condition option status" }

// testClause evaluates `[[ … ]]`. It exits 0 when the condition holds.
func (r *Runner) testClause(ctx context.Context, c *syntax.TestClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		// The trace is opened here and closed on the way out, because the
		// shell that writes one line for the whole condition cannot write it
		// until the condition is over — and the shell that writes a line per
		// primary writes each of them from inside the walk below. One tracer
		// answers both; see condTrace.
		defer r.beginConditionTrace()()
		// The same clearing a compound command's heading does, and for the
		// same reason: an operand is expanded before the condition decides
		// anything, and a failure left over from the previous command would
		// abandon this one. See Runner.beginHeading.
		r.beginHeading()
		ok, err := r.evalCond(ctx, c.Expr)
		if r.unspecified {
			r.status = 2
			return nil
		}
		if err != nil {
			if errors.Is(err, errCondOperandFailed) {
				// An operand whose expansion failed, already complained
				// about where it failed and already ended whatever this
				// dialect ends. The status failedHeading left stands.
				return nil
			}
			// A status the condition produced for itself, already complained
			// about where it happened; anything else is a failure this
			// construct reports at 2.
			var cs condStatus
			if errors.As(err, &cs) {
				r.status = cs.code
				return nil
			}
			r.diagf("%v\n", err)
			r.status = 2
			return nil
		}
		if r.ctl == controlExit {
			// The condition rejected its pattern outright and the shell is
			// being abandoned, so the status belongs to the refusal rather
			// than to a comparison that never finished. Measured, real zsh
			// exits 2 from `[[ x == (#Z)a ]]` and the answer would read as
			// the ordinary `1` for "did not match" without this.
			return nil
		}
		r.status = boolInt(!ok)
		return nil
	})
}

func (r *Runner) evalCond(ctx context.Context, c syntax.CondExpr) (bool, error) {
	switch x := c.(type) {
	case nil:
		return false, nil

	case *syntax.CondGroup:
		return r.evalCond(ctx, x.X)

	case *syntax.CondNot:
		// The `!` prints with the primary it negates rather than as a part of
		// its own: `[[ ! -z a ]]` is one line in every shell that has the
		// construct.
		r.traceConditionNot()
		v, err := r.evalCond(ctx, x.X)
		return !v, err

	case *syntax.CondLogic:
		l, err := r.evalCond(ctx, x.X)
		if err != nil {
			// An operand that produced a status of its own rather than a
			// truth value. The combining operators read it as a status, so
			// `||` goes on to the right exactly as it would past a false and
			// the right-hand answer is the whole answer, where `&&` stops on
			// it. Measured on zsh: `[[ -o zzz || 1 == 1 ]]` is 0 and
			// `[[ 1 == 1 && -o zzz ]]` is 3.
			//
			// Only this error. A condition that failed for another reason —
			// a regular expression that will not compile — is a plain false
			// in all three shells, `!` flips it and `||` sees a false, so
			// widening this would be a change nothing measured asked for.
			var cs condStatus
			if x.Op == "||" && errors.As(err, &cs) {
				r.traceConditionOp(x.Op)
				return r.evalCond(ctx, x.Y)
			}
			return false, err
		}
		// Short-circuit, as everywhere else.
		if x.Op == "&&" && !l {
			return false, nil
		}
		if x.Op == "||" && l {
			return true, nil
		}
		// Recorded on the way *through* rather than on the way in, so an
		// operator whose right-hand side never ran leaves nothing in the
		// line. Measured: `[[ -n a || -n b ]]` traces `[[ -n a ]]` in zsh,
		// the one shell whose line could have held the whole expression.
		r.traceConditionOp(x.Op)
		return r.evalCond(ctx, x.Y)

	case *syntax.CondArity:
		return r.condWrongArity(x)

	case *syntax.CondCompletion:
		return r.evalCondCompletion(ctx, x)

	case *syntax.CondUnary:
		return r.evalCondUnary(x)

	case *syntax.CondBinary:
		return r.evalCondBinary(x)
	}
	return false, arithError{msg: "unsupported condition"}
}

// condWrongArity refuses a known conditional operator that stood with the
// wrong number of operands.
//
// The refusal the grammar handed on rather than made — see
// syntax.Dialect.ConditionArityIsCheckedWhenItRuns, which is what lets the
// condition parse at all, and #965. The operator is named and nothing else
// is: measured on zsh 5.9.2, `[[ -n ]]`, `[[ -n x y ]]` and `[[ -n x -z "" ]]`
// are all `unknown condition: -n`, so it is the operator's name and not the
// surplus word's.
//
// It ends the shell at 2, in every position measured — before a `||`, inside
// an `if` head, inside a function, from `-c` and from a script file — and the
// commands before it on the same line have already run, which is the whole
// difference this makes. The status is written here rather than left to
// FatalErrorStatusIsOne: that axis answers the *generic* fatal error and this
// refusal is 2 in the one shell that has it, where the same shell's generic
// answer is 1.
func (r *Runner) condWrongArity(x *syntax.CondArity) (bool, error) {
	d := r.diag()
	r.diagf("%s\n", Wording(d.UnknownCondition, "unknown condition: %s", x.Op))
	status := orDefault(d.UnknownConditionStatus, 2)
	r.status = status
	r.stopTheShell()
	// A status rather than a message, because the complaint is already
	// written: condStatus is the shape testClause takes the number from
	// without saying anything further, and the shell has been stopped above.
	return false, condStatus{code: status}
}

// evalCondCompletion answers one of the four completion-context conditions —
// `-prefix`, `-suffix`, `-after` and `-between`.
//
// The grammar has them wherever a dialect asked for them and the *meaning* is
// the dialect's, because it is a question about a completion in flight and
// this package has no completion system. A dialect says what it means with
// Runner.SetConditionAnswer; nothing registered is the refusal below, which
// is also what a shell with the grammar and no completion system gives.
//
// **The refusal is what a completion condition reached anywhere else gets**,
// and it is fatal: measured 2026-09-12 over a script file on zsh 5.9.2, the
// line after it does not run and the status is 1. The sentence names neither
// the operator nor the operand — `[[ -prefix : ]]`, `[[ -prefix 'ab' ]]` and
// `[[ -prefix //(a|b)/ ]]` all get the same one.
//
// **Loading the module is not what decides this.** Measured 2026-09-19 on a
// fresh `zsh -f`, whose module listing is `zsh/main` alone: the sentence is
// identical before and after `zmodload zsh/complete`, so the conditions are
// the grammar's at all times and the state that matters is whether a
// completion is running (#3042).
func (r *Runner) evalCondCompletion(ctx context.Context, x *syntax.CondCompletion) (bool, error) {
	// Before any word is expanded, so the command a refused process
	// substitution holds is never started — the rule every other primary
	// here follows.
	for _, w := range x.Words {
		if err := r.condProcSubAllowed(w, false); err != nil {
			return false, err
		}
	}
	// Each operand as the matcher reads it: unquoted it is a pattern and
	// quoted it is a literal, which is measured inside a real completion and
	// is why the tree keeps words rather than strings. See ConditionAnswer.
	operands := make([]string, 0, len(x.Words))
	for _, w := range x.Words {
		operands = append(operands, r.patternOf(w))
	}
	if r.condOperandDidNotExpand() {
		return false, errCondOperandFailed
	}
	r.traceConditionPrimary(append([]string{x.Op}, operands...)...)
	if ask := r.conditionAnswers[x.Op]; ask != nil {
		if ok, answered := ask(r, ctx, x.Op, operands); answered {
			return ok, nil
		}
	}
	r.diagf("%s\n", Wording(r.diag().CompletionConditionOutsideCompletion,
		"condition can only be used in completion function"))
	r.stopTheShell()
	return false, condStatus{code: 1}
}

func (r *Runner) evalCondUnary(x *syntax.CondUnary) (bool, error) {
	// Before the word is expanded, so the command a refused process
	// substitution holds is never started. The bare-word form arrives here
	// too, as `-n` over the word, which is what puts every operand of every
	// operator behind the one question.
	if err := r.condProcSubAllowed(x.X, false); err != nil {
		return false, err
	}
	// Nothing inside `[[ ]]` is split or globbed, so a word yields exactly
	// one operand however it was written — which is why `[[ -z $u ]]` needs
	// no quoting where the `[` builtin does.
	s := r.condOperandText(x.X)
	if r.condOperandDidNotExpand() {
		return false, errCondOperandFailed
	}
	r.traceConditionPrimary(x.Op, r.traceCondOperand(s))
	switch x.Op {
	case "-n":
		return s != "", nil
	case "-z":
		return s == "", nil
	case "-t":
		// Whether this shell's descriptor is a terminal — the same question
		// `test -t` asks and the same answer, through the same helper. An
		// operand that is not a number is a question of its own first.
		on, isNumber := r.terminalTest(s)
		if !isNumber &&
			r.ask(r.sem().TerminalTestRequiresANumber, "`[[ -t x ]]` refusing a non-number") {
			return false, arithError{msg: Wording(r.diag().TestIntegerExpected,
				"%[2]s: %[1]s: integer expected", s, "[[")}
		}
		return on, nil
	case "-v":
		// Whether a parameter is set, which is a question about the
		// parameter and not about its value: a name holding the empty
		// string is set. Shared with `test -v` — see parameterIsSet, where
		// the whole of the answer and its two axes are.
		//
		// The word rather than s, because the one thing this route has that
		// the builtin's does not is which brackets were written: see
		// condParameterIsSet.
		return r.condParameterIsSet(x.X, s)
	case "-R":
		// Whether the name is a **reference**, which is a question about the
		// binding rather than about what it points at: the reference answers
		// true and its target answers false. The same question `test -R`
		// asks and the same answer, through the same helper — the corpus case
		// puts the two spellings side by side so they cannot come apart.
		//
		// The grammar has already decided this word is an operator at all
		// (Dialect.NameReferenceTest); this asks the separate question of
		// whether the dialect answers it, which is what the builtin's gate
		// asks too.
		if !r.ask(r.sem().TestHasTheNameReferenceOperator,
			"`[[ -R r ]]` asking whether r is a name reference") {
			return false, nil
		}
		return r.isNameref(s), nil
	case "-o":
		// The shell's own option state, read through the dialect's namespace
		// — which for one of the panel is far wider than its `set -o` names.
		// A name this shell has is a plain true or false in all three that
		// have the operator; only a name none of them would know is a
		// question, and it is asked there and nowhere else.
		if on, known := r.conditionOption(s); known {
			return on, nil
		}
		if !r.ask(r.sem().UnknownConditionOptionIsAStatus,
			"`[[ -o ]]` given a name this shell does not have") {
			return false, nil
		}
		d := r.diag()
		// Said here rather than carried out in the error, because it is said
		// even when nothing downstream reports the status: `[[ -o zzz || 1
		// == 1 ]]` is 0 in zsh with the complaint already written.
		r.diagf("%s\n", Wording(d.UnknownConditionOption, "no such option: %s", s))
		return false, condStatus{code: d.UnknownConditionOptionStatus}
	case "-e", "-f", "-d", "-s", "-r", "-w", "-x",
		"-b", "-c", "-p", "-S", "-g", "-u", "-k", "-L", "-h",
		"-O", "-G":
		// The file questions are `test`'s, answered by the same code: the
		// two constructs disagree about how an operand is obtained, never
		// about what the filesystem says about it. fileTest stats through
		// the gate, so a `[[ ]]` probe is as visible to a policy as the
		// builtin's — a file test is an existence oracle either way.
		return r.fileTest(x.Op, s), nil
	}
	return false, arithError{msg: "unsupported test " + x.Op}
}

func (r *Runner) evalCondBinary(x *syntax.CondBinary) (bool, error) {
	// Both operands, and the question is asked before either word is
	// expanded: a shell that refuses a process substitution must not have
	// started the command first, which is observable because the command has
	// side effects. The left is asked first for the same reason — the
	// operands are read left to right, so the sentence names the first of
	// them where a condition holds two.
	pattern := x.Op == "==" || x.Op == "=" || x.Op == "!="
	// The status the refusal leaves is a property of the **expression** and
	// not of the word the sentence names, which is measured: `[[ <(x) ==
	// <(y) ]]` and `[[ <(x) == >(y) ]]` both refuse the left one by name and
	// leave 2 and 1, differing only in what stands behind the operator.
	rightIsInput := pattern && wordOpensAnInputProcSubst(x.Y)
	if err := r.condProcSubAllowed(x.X, rightIsInput); err != nil {
		return false, err
	}
	if err := r.condProcSubAllowed(x.Y, rightIsInput); err != nil {
		return false, err
	}
	leftMarked := r.condOperand(x.X)
	if r.condOperandDidNotExpand() {
		return false, errCondOperandFailed
	}
	left := syntax.UnmarkArithValue(leftMarked)

	if x.Op == "=~" {
		// The one place the pattern language is regular expressions rather
		// than globs, and — like the glob operators below — a place where
		// the right operand has to be read a span at a time rather than as
		// the string it expands to. bash matches the **quoted portions** of
		// the expression as literal text and leaves the rest an expression,
		// so `[[ ab =~ ^"a"b$ ]]` holds: the `a` is a letter and the anchors
		// on either side of it are still anchors.
		//
		// This used to ask the *word* whether anything in it was quoted and
		// then escape the whole expanded value, which made one quote
		// anywhere in the operand turn every metacharacter in it into a
		// letter. Measured against bash 5.3.20, 2026-09-22 — the left column
		// is what that reading answered:
		//
		//	[[ ab  =~ ^"a"b$ ]]    was 1, is 0 — the anchors became letters
		//	[[ ab  =~ ^'ab'$ ]]    was 1, is 0
		//	[[ ab  =~ ^\ab$ ]]     was 1, is 0 — a backslash counts as a quote
		//	[[ axb =~ "a".b ]]     was 1, is 0 — and so did the `.`
		//	[[ aab =~ "a"a*b ]]    was 1, is 0
		//	[[ x   =~ [$"a"-z] ]]  was 1, is 0 — a quote inside a bracket
		//
		// The control that says the escaping still happens where it should
		// is the row a quote is *about*: `[[ axb =~ "a.b" ]]` is 1 and
		// `[[ a.b =~ "a.b" ]]` is 0, in bash and here alike.
		text, literal := r.condRegexOperand(x.Y)
		if r.condOperandDidNotExpand() {
			return false, errCondOperandFailed
		}
		// The trace holds the operand as it expanded, without the escaping
		// the match is about to apply: bash traces `[[ ab =~ ab ]]` for
		// `[[ ab =~ "a"b ]]`, so the line says what the words came to rather
		// than how the matcher was told to read them.
		r.traceConditionPrimary(r.traceCondOperand(left), x.Op, r.traceCondOperand(text))
		pat := text
		// bash treats a quoted portion as a literal string; ksh93 and zsh
		// keep it an expression, so quoting a regex is unportable in either
		// direction. Asked only where a quote was written, so the axis is
		// not consulted about a word that has nothing to quote.
		if x.Y.IsQuoted() && r.ask(r.sem().RegexQuotingMakesLiteral, "quoting a =~ regex making it literal") {
			pat = literal
		}
		return r.regexMatch(pat, left)
	}

	if pattern {
		// Unquoted, the right operand is a pattern; quoted, a literal. Only
		// the spans still know which, which is why the tree keeps a word.
		pat := r.patternOf(x.Y)
		// The trace prints the pattern the matcher is about to be handed,
		// backslashes and all, rather than a quoted value — which is what two
		// of the three shells do and is the more informative of the two
		// renderings: `p='a*'; [[ abc == $p ]]` traces `a\*` where the
		// expansion is literal and `a*` where it is live, so the line says
		// which characters were patterns. It is also the only rendering that
		// costs nothing, since re-expanding the word to print it would run a
		// substitution in it twice (#1915). ksh93 quotes the unexpanded value
		// instead and is recorded rather than modeled.
		r.traceConditionPrimary(r.traceCondOperand(left), x.Op, pat)
		got := r.matchPatternR(pat, left, true)
		if x.Op == "!=" {
			return !got, nil
		}
		return got, nil
	}

	// Every other operator reads its right-hand side as a value. It is
	// expanded here rather than inside each branch because the trace holds
	// both operands and is written before the test is answered, which is
	// where every shell that has the construct puts it — and because the
	// expansion must happen exactly once however many readers it has.
	rightMarked := r.condOperand(x.Y)
	if r.condOperandDidNotExpand() {
		return false, errCondOperandFailed
	}
	right := syntax.UnmarkArithValue(rightMarked)
	r.traceConditionPrimary(r.traceCondOperand(left), x.Op, r.traceCondOperand(right))

	switch x.Op {
	case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		// The word-spelled operators compare numbers, and their operands are
		// *expressions* rather than literals: with `n=5`, `[[ n -eq 5 ]]`
		// holds, because the bare name is read the way `$(( n ))` reads it.
		// Every shell in the panel that has `[[ ]]` does this, so it is the
		// core's answer and not an axis — see condArith.
		//
		// `<` and `>` compare strings, which is why `[[ 10 > 9 ]]` is false
		// and `[[ 10 -gt 9 ]]` is true — the sharpest trap in the construct.
		l, err := r.condArith(r.conditionSubscriptText(leftMarked, left))
		if err != nil {
			return false, err
		}
		rv, err := r.condArith(r.conditionSubscriptText(rightMarked, right))
		if err != nil {
			return false, err
		}
		switch x.Op {
		case "-eq":
			return l == rv, nil
		case "-ne":
			return l != rv, nil
		case "-lt":
			return l < rv, nil
		case "-le":
			return l <= rv, nil
		case "-gt":
			return l > rv, nil
		}
		return l >= rv, nil

	case "-nt", "-ot", "-ef":
		return r.compareFiles(x.Op, left, right)

	case "<":
		return left < right, nil
	case ">":
		return left > right, nil
	}
	return false, arithError{msg: "unsupported test " + x.Op}
}

// condOperand expands a word to a single string. Nothing inside `[[ ]]` is
// split or globbed, so joining is the whole of it — with the one exception
// the vendor manual states, a word ending in a `(#q…)` group, which is
// measured and explained in interp/condqualifier.go.
//
// The pattern-match operators' right-hand side does not come through here:
// evalCondBinary reads it with patternOf, which is what keeps this exception
// off the one side the manual says it does not apply to.
func (r *Runner) condOperand(w *syntax.Word) string {
	if r.condWordQualifies(w) {
		return r.condGlobbedOperand(w)
	}
	// Marked, and every caller but the arithmetic one takes the marks
	// straight back off. A condition still holds the *word*, so it still
	// knows which of the brackets in the finished text a script wrote
	// unquoted — and that is the only stage at which anything can know it.
	// The two readings are one expansion, which is what the operand must
	// have however many readers it has.
	// A condition's operand protects every span that is not plain unquoted
	// text: a bracket behind a quote, and one out of an expansion, are both
	// content. Measured 2026-09-16 from a script file with `declare -A a;
	// a[']']=5`, and again 2026-09-22 over the other two spellings:
	// `[[ a[']'] -eq 5 ]]`, `[[ a["]"] -eq 5 ]]` and `[[ a[\]] -eq 5 ]]` all
	// hold in bash 5.3.20 and under that build as `sh`. It is bash's alone —
	// ksh93u+ 2012-08-01 says `a[]]: arithmetic syntax error` to the first of
	// them, and reads the same subscript inside `(( ))` perfectly well — so
	// which of the two readings is taken is the dialect's, and the marking is
	// only what makes both available. See
	// Semantics.ConditionArithmeticReadsTheWrittenSubscript and
	// conditionSubscriptText, which asks it (#3302).
	//
	// A substring's range is the other caller of the same marking and does
	// **not** protect the same set — see rangeProtectsSpan.
	return r.markedSubscriptWord(w, func(syntax.Span) bool { return true })
}

// condOperandText is condOperand with the marks off: the text every reader
// but the arithmetic one wants. See syntax.ArithValueMark.
func (r *Runner) condOperandText(w *syntax.Word) string {
	return syntax.UnmarkArithValue(r.condOperand(w))
}

// condRegexOperand expands a `=~` right operand once and gives back both
// readings of it: the text the word came to, and the same text with the
// **quoted spans** escaped so that a matcher reads them as letters.
//
// Two readings out of one pass, for the reason every other paired reading in
// this file is built that way: the operand must be expanded exactly once
// however many readers it has, and `[[ x =~ $(f) ]]` must not run `f` twice.
//
// Which spans are the expression's and which are text is the span's own
// quoting, and nothing else: unquoted literal text and the result of an
// unquoted expansion are both regular expression, and every other span is a
// string. That is the same split [Runner.patternSpan] makes for globs, with
// one difference that matters — a dialect can re-read an expansion's result
// as a *pattern*, and no dialect re-reads one as an expression, so there is
// no axis in the middle of this one.
func (r *Runner) condRegexOperand(w *syntax.Word) (text, literal string) {
	if r.condWordQualifies(w) {
		// A word that ends in a glob qualifier group has already matched
		// against the filesystem, so what comes back is a path and not
		// something a quote could have divided. See interp/condqualifier.go.
		t := syntax.UnmarkArithValue(r.condGlobbedOperand(w))
		return t, t
	}
	var b strings.Builder
	text = r.wordTextNoSplit(w, func(s syntax.Span, part string) string {
		if regexSpanIsLive(s) {
			b.WriteString(part)
		} else {
			b.WriteString(regexp.QuoteMeta(part))
		}
		return part
	})
	return text, b.String()
}

// regexSpanIsLive reports whether this span's metacharacters are the regular
// expression's rather than letters.
//
// The quoting is the whole of the reading, and a backslash counts: `\.` is a
// span of its own whose quoting says the script wrote a quote, which is why
// `[[ ab =~ ^\ab$ ]]` matches where a reading that only looked for quotation
// marks would have made the anchors literal too.
func regexSpanIsLive(s syntax.Span) bool {
	return s.Quoting == syntax.Unquoted
}

// conditionSubscriptText is which of the two readings of a comparison's
// operand the dialect takes: the one that still knows which brackets the
// script wrote, or the text as it stands once the word has been expanded.
//
// The two are the same string wherever no quoted or expanded span put one of
// the bytes a subscript scan reads into the operand, and that equality is
// what keeps the axis from being asked of `[[ n -eq 5 ]]`. See
// Semantics.ConditionArithmeticReadsTheWrittenSubscript.
func (r *Runner) conditionSubscriptText(marked, plain string) string {
	// Nothing to read the two ways: with no bracket anywhere in the operand
	// there is no subscript for a mark to change the extent of, and an
	// operand whose quoted spans carried none of the bytes a scan reads is
	// the same string either way. Both are the ordinary case, and neither
	// demands a dialect for a question that has no operand to ask it about.
	if marked == plain || !strings.Contains(plain, "[") {
		return plain
	}
	if r.ask(r.sem().ConditionArithmeticReadsTheWrittenSubscript,
		"a comparison operand being read as the subscript the script wrote") {
		return marked
	}
	return plain
}

// condArith reads a condition operand as an arithmetic expression, which is
// what the word-spelled comparisons compare.
//
// The word has already been expanded by the time it arrives, and it is *not*
// expanded again. That is measured rather than assumed: with `x=7; v='$x'`,
// `[[ v -eq 7 ]]` does not come to 7 in any shell in the panel — the
// arithmetic sees the two characters `$x` and blames them, bash naming them
// as its error token. So the expansion happens once, on the word, and what
// reaches here is text to be read as an expression.
//
// An empty operand is zero. `e=”; [[ e -eq 0 ]]` holds in bash and zsh, and
// the expression parser has no primary to offer for nothing at all, so the
// case is answered before it is asked.
func (r *Runner) condArith(text string) (int, error) {
	v, msg := r.conditionOperand(text)
	if msg != "" {
		return 0, r.condArithFailed(r.diag().arithConstructFailure("[[", msg))
	}
	return v, nil
}

// conditionOperand is the reading itself: the value, or the dialect's worded
// complaint about text that would not read as an expression.
//
// Split from condArith because `[[ ]]` is not the only construct that reads an
// operand this way. One dialect's `test` and `[` do too, and they answer a
// failure differently — status 1 with the builtin's name in front of the same
// sentence, and the script runs on. See
// Semantics.TestBuiltinComparisonOperandsAreArithmetic.
func (r *Runner) conditionOperand(text string) (value int, failure string) {
	if strings.TrimSpace(text) == "" {
		return 0, ""
	}
	// Asked of the trimmed text and read from the untrimmed one: the answer
	// for an all-blank operand is zero, and everything else is an expression
	// whose complaint quotes the text as the condition held it. Reading the
	// trimmed text instead lost ksh93's blanks — ` 1/0 : divide by zero`
	// against `1/0: divide by zero` (#2010).
	// An operand arrives here already expanded, so a zero-padded numeral in
	// it is text the *word* carried rather than a literal the arithmetic
	// lexer read — which is the shape one shell reads in decimal. Measured
	// 2026-09-12 on ksh93u+: `[[ 010 -eq 10 ]]` holds and `(( 010 == 10 ))`
	// does not, and `[[ 1+010 -eq 9 ]]` holds too, so it is the *leading*
	// numeral alone and not the whole operand. That is the stored value's
	// reader, which is why this is the same axis read at a site that was
	// missing it rather than one of its own (#1867) — with the one difference
	// conditionLeadingNumeral records: here the zeros go in front of an `0x`
	// prefix as well, so `[[ 0x10 -eq 16 ]]` is false in that shell (#1627).
	text = r.conditionLeadingNumeral(text)
	p := syntax.NewParser("", r.dialect())
	// An operand arrives already expanded — the doc above measures that with
	// `x=7; v='$x'`, which no column reads as 7 — so it is read as the
	// result it is. Reading it as a program left a `$` with no tree and no
	// complaint, which evaluated to a silent 0 where bash writes `$x:
	// arithmetic syntax error: operand expected` (#3303).
	tree := p.ParseArithExpanded(text, syntax.Pos{})
	// The text a complaint quotes back is the one a script would recognize,
	// which is this one without the marks: they are this implementation's
	// bookkeeping, and a refusal carrying one prints a stray NUL into the
	// log. The *reading* is done from the marked text, which is the whole
	// point of having it. See syntax.ArithValueMark.
	shown := stripArithValueMarks(text)
	if perr := p.Err(); perr != nil {
		return 0, r.diag().ParseFailure(unmarkArithFailure(perr))
	}
	v, err := r.evalArith(tree)
	if err != nil {
		return 0, r.arithFailure(shown, err)
	}
	return v, ""
}

// condArithFailed reports an unreadable condition operand and says how the
// construct ends.
//
// The complaint is written here rather than carried out in the error because
// it is written either way, and only what happens next differs: two of the
// three shells with `[[ ]]` abandon the input and one lets the condition be
// false and goes on. Both leave the status at 1, so the status is the
// substrate's and the abandoning is the dialect's — which is why
// ConditionArithmeticErrorIsFatal is a single boolean and not a status.
func (r *Runner) condArithFailed(msg string) error {
	r.diagf("%s\n", msg)
	if r.ask(r.sem().ConditionArithmeticErrorIsFatal, "an unreadable operand of a `[[ ]]` comparison") {
		r.abandonOverArithmetic()
	}
	return condStatus{code: 1}
}

// condProcSubAllowed refuses a process substitution standing as a condition's
// operand where the dialect does not have one there.
//
// Measured: bash performs it and matches against the path, which never
// matches anything a script would write down; zsh reads the word and then
// refuses it — `process substitution <(x) cannot be used here`; ksh93 refuses
// while reading and dash has no `[[ ]]` at all. Three shells say no and only
// the moment and the wording differ, which is the line between the axis and
// the Diagnostics vector.
//
// **Every operand, not only the one a comparison holds.** Measured 2026-09-18
// on zsh 5.9.2 over eight shapes — `[[ -e <(echo x) ]]`, `[[ -n <(echo x) ]]`,
// `[[ <(echo x) == x ]]`, `[[ b -nt <(echo x) ]]`, `[[ a =~ <(echo x) ]]`,
// `[[ ! -e <(echo x) ]]`, `[[ ( -e <(echo x) ) ]]` and the comparison this
// used to be asked at — and the refusal reaches all of them. It is lazy, so a
// short-circuit still hides one: `[[ x == y && -e <(echo x) ]]` runs the
// command in neither shell and answers 1 (#3280).
//
// rightIsInput says the right operand of a pattern comparison holds a `<(`,
// which is the one shape whose status differs — see condProcSubRefusal, where
// the measurement is. It is a property of the expression and not of the word
// this call is about.
func (r *Runner) condProcSubAllowed(w *syntax.Word, rightIsInput bool) error {
	if w == nil {
		return nil
	}
	for _, s := range w.Spans {
		if s.Kind != syntax.ProcSubstIn && s.Kind != syntax.ProcSubstOut &&
			s.Kind != syntax.ProcSubstFile {
			continue
		}
		answer := r.sem().ProcessSubstitutionInCondition
		if r.ask(answer, "a process substitution standing as a condition's operand") {
			return nil
		}
		if answer != No {
			// Unanswered: ask has already refused by name, and the refusal
			// is the substrate's rather than a shell's, so it stops at the
			// condition the way every other unanswered axis does.
			return condStatus{code: 2}
		}
		r.diagf("%s\n", Wording(r.diag().ProcessSubstitutionNotInCondition,
			"process substitution %[1]s cannot be used here", procSubSource(s)))
		// And the input is abandoned, not merely this condition. Measured:
		// the rest of the command string does not run, and the status is
		// neither the 1 a condition that simply did not hold gives nor the
		// status this dialect gives an ordinary fatal error, so it is
		// written here rather than routed through either.
		r.stopTheShell()
		return condStatus{code: condProcSubRefusal(s.Kind, rightIsInput)}
	}
	return nil
}

// condProcSubRefusal is the status a refused process substitution leaves
// behind in a condition.
//
// **One is the answer and 2 is the exception**, which is measured rather than
// a slip. 2026-09-18 on zsh 5.9.2, the only shell that reaches this wording,
// over fourteen shapes: every operand of every operator leaves 1 — `[[ -e
// <(x) ]]`, `[[ <(x) == a ]]`, `[[ b -nt <(x) ]]`, `[[ a =~ <(x) ]]`,
// `[[ a == >(x) ]]`, `[[ a == =(x) ]]` — and 2 comes back only where the
// **right** operand of a pattern comparison is the **input** spelling:
//
//	[[ a == <(echo hi) ]]        2       [[ a == >(echo hi) ]]   1
//	[[ a != <(echo hi) ]]        2       [[ a == =(echo hi) ]]   1
//	[[ <(x) == <(y) ]]           2       [[ <(x) == >(y) ]]      1
//	[[ <(x) == a ]]              1       [[ b -nt <(x) ]]        1
//
// The third pair on the left is what says the status belongs to the *right*
// operand and not to the word the sentence names: both of those refuse the
// left one by name and differ only in what stands behind the operator. The
// earlier reading of this had the direction wrong — `>(` was 2 and only the
// file spelling was 1 — which the six rows on the right now pin. Same
// sentence and the same abandoned input in all fourteen; `echo after` runs in
// none of them. It is a fact about one shell's word handling rather than
// about the panel, since no second shell reaches the wording at all, which is
// why it is written here beside it instead of becoming an axis nobody could
// answer twice.
func condProcSubRefusal(kind syntax.SpanKind, rightIsInput bool) int {
	if rightIsInput {
		return 2
	}
	return 1
}

// wordOpensAnInputProcSubst reports whether a word holds a `<(cmd)`, which is
// the one spelling the status above turns on. `>(cmd)` and `=(cmd)` are the
// other two and neither moves it.
func wordOpensAnInputProcSubst(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	for _, s := range w.Spans {
		if s.Kind == syntax.ProcSubstIn {
			return true
		}
	}
	return false
}

// procSubSource is a process substitution as it was written. The span keeps
// its inside and its direction, and a diagnostic names the whole of it.
func procSubSource(s syntax.Span) string {
	open := "<("
	switch s.Kind {
	case syntax.ProcSubstOut:
		open = ">("
	case syntax.ProcSubstFile:
		open = "=("
	}
	return open + s.Value + ")"
}

// regexMatch is `=~`: whether left holds a match for the extended regular
// expression pat, with the captures recorded the way the dialect names them.
// Shared by the conditional and by the `[[` that is a command, which differ
// in what a quoted pattern means and in nothing past that.
func (r *Runner) regexMatch(pat, left string) (bool, error) {
	// An empty right operand is where the engine underneath shows
	// through. POSIX ERE, which the shells that refuse this are built
	// on, has no empty expression; Go's regexp compiles `` happily and
	// then matches the empty string at every position, so a global
	// replace over `abc` writes between every pair of characters
	// instead of doing nothing. The compile cannot report it, so the
	// emptiness is asked about before the compile rather than by it.
	if pat == "" && r.ask(r.sem().EmptyRegexOperandIsAnError,
		"an empty =~ right operand being an error") {
		// The sentence and the status are both the dialect's, and they do
		// not move together: one column calls it a failure of the construct
		// and the other a match that did not happen. See
		// Diagnostics.EmptyRegexOperand.
		d := r.diag()
		r.diagf("%s\n", Wording(d.EmptyRegexOperand,
			"invalid regular expression: empty (sub)expression"))
		return false, condStatus{code: orDefault(d.EmptyRegexOperandStatus, 2)}
	}
	// The expression and the subject as the *engine* must see them,
	// which is not always as the script wrote them: the case fold is
	// narrowed to ASCII under a C or POSIX locale, and the only way to
	// narrow the engine's is to write the characters it must not fold
	// out of its reach. back maps an offset in the subject it matched
	// against back to an offset in the script's own. See regexmatch.go.
	expr, subject, back := r.regexOperands(pat, left)
	re, err := regexp.Compile(expr)
	if err != nil {
		// The pattern as the script wrote it, never the folded spelling:
		// a script that never asked for `(?i)` must not read about one.
		return false, arithError{msg: "invalid regular expression: " + pat}
	}
	// Leftmost-**longest**, which is what a POSIX regular expression means
	// and is not what this package matches by default: `regexp` prefers the
	// leftmost match the first alternative reaches, so `[[ ab =~ a|ab ]]`
	// matched `a` where every shell with the construct matches `ab`. The
	// whole match is what a script reads back — the first element of the
	// record, and the parameters that report where it began and ended — so
	// the difference is a value carried forward rather than a status.
	//
	// Measured 2026-09-22 against bash 5.3.20, the record's first element:
	//
	//	[[ ab   =~ a|ab ]]      ab, where this package answered a
	//	[[ abc  =~ ab|abc ]]    abc
	//	[[ aaa  =~ a|aa|aaa ]]  aaa
	//
	// The rule is the expression's and not the dialect's: POSIX defines the
	// match this way and ERE is what all three columns with `=~` compile, so
	// there is nothing here for a semantics axis to hold.
	re.Longest()
	// The captures are the point of matching, not a by-product: element 0
	// is the whole match and the rest are the groups. The core records
	// them and a dialect names the record — see regexmatch.go.
	//
	// The offsets are asked for rather than the texts, because the two
	// records a dialect can ask for want different things out of one
	// match: the dense array wants the strings, and the reporting
	// parameters want where each span began and ended. Matching twice to
	// get both would be two answers to one question.
	loc := scriptOffsets(re.FindStringSubmatchIndex(subject), back)
	var m []string
	var took []bool
	if loc != nil {
		m, took = make([]string, len(loc)/2), make([]bool, len(loc)/2)
		for i := range m {
			if loc[2*i] >= 0 {
				m[i], took[i] = left[loc[2*i]:loc[2*i+1]], true
			}
		}
	}
	// The offsets and not the texts say which groups took part: a group that
	// matched the empty string and a group the match never reached are both
	// the empty string by the time they are elements.
	r.recordRegexMatch(m, took)
	r.publishRegexCapture(left, loc)
	return loc != nil, nil
}

// errCondOperandFailed is a condition whose operand did not expand: the
// complaint is already written and whatever the dialect ends is already
// ended, so testClause takes the status that was left and says nothing more.
var errCondOperandFailed = errors.New("condition operand did not expand")

// condOperandDidNotExpand reports whether the operand just expanded failed,
// and ends what a failed expansion ends in this dialect.
//
// The same question a compound command's heading asks, through the same
// helper, and that is the whole of why the panel needs no axis of its own
// here: a failed expansion is fatal in three of the four columns with the
// construct and is not in the fourth, which is already measured and already
// answered. Measured 2026-09-18, a script file under `env -i`, over
// `[[ $((1/0)) -eq 0 ]]` with a `printf` on either side of it:
//
//	bash 5.3.20, bash 3.2.57   the division is reported, the condition is
//	                           **false** at 1, and the script carries on
//	zsh 5.9.2, ksh93u+, ash    the division is reported and the script ends
//
// The condition is abandoned whole rather than the primary being false,
// which is what the two shapes past a plain reading say: `[[ ! $((1/0)) -eq
// 0 ]]` is 1 in bash rather than the 0 a negated false would give, and
// `[[ $((1/0)) -eq 0 || 1 -eq 1 ]]` is 1 rather than the 0 the right-hand
// side would give. The `||` that never reaches it is the control —
// `[[ 1 -eq 1 || $((1/0)) -eq 0 ]]` is 0 and starts no division at all.
//
// An **empty** operand is not this: `[[ "" -eq 0 ]]` and `[[ $nosuch -eq 0 ]]`
// are both 0 in bash, so the row is about the expansion having failed and
// not about the text it left behind — which is exactly what this shell used
// to answer, the failed expansion leaving an empty string that read as zero
// and made the comparison hold (#3556).
func (r *Runner) condOperandDidNotExpand() bool {
	return r.failedHeading()
}
