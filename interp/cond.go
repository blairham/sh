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
		r.unspecified = false
		ok, err := r.evalCond(c.Expr)
		if r.unspecified {
			r.status = 2
			return nil
		}
		if err != nil {
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

func (r *Runner) evalCond(c syntax.CondExpr) (bool, error) {
	switch x := c.(type) {
	case nil:
		return false, nil

	case *syntax.CondGroup:
		return r.evalCond(x.X)

	case *syntax.CondNot:
		// The `!` prints with the primary it negates rather than as a part of
		// its own: `[[ ! -z a ]]` is one line in every shell that has the
		// construct.
		r.traceConditionNot()
		v, err := r.evalCond(x.X)
		return !v, err

	case *syntax.CondLogic:
		l, err := r.evalCond(x.X)
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
				return r.evalCond(x.Y)
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
		return r.evalCond(x.Y)

	case *syntax.CondUnary:
		return r.evalCondUnary(x)

	case *syntax.CondBinary:
		return r.evalCondBinary(x)
	}
	return false, arithError{msg: "unsupported condition"}
}

func (r *Runner) evalCondUnary(x *syntax.CondUnary) (bool, error) {
	// Nothing inside `[[ ]]` is split or globbed, so a word yields exactly
	// one operand however it was written — which is why `[[ -z $u ]]` needs
	// no quoting where the `[` builtin does.
	s := r.condOperand(x.X)
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
		return r.parameterIsSet(s)
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
	case "-prefix", "-suffix":
		// The completion-context tests. One dialect's grammar has them
		// unconditionally and the restriction is on where they may run, so
		// reaching one anywhere else is a refusal rather than an answer —
		// and a fatal one: measured 2026-09-12 over a script file, the line
		// after it does not run.
		//
		// The operand is read and then dropped, which is what the trace
		// above already did with it. Nothing about the word decides this:
		// `[[ -prefix : ]]`, `[[ -prefix 'ab' ]]` and `[[ -prefix
		// //(a|b)/ ]]` all answer the same sentence.
		r.diagf("%s\n", Wording(r.diag().CompletionConditionOutsideCompletion,
			"condition can only be used in completion function"))
		r.ctl = controlExit
		return false, condStatus{code: 1}
	case "-e", "-f", "-d", "-s", "-r", "-w", "-x",
		"-b", "-c", "-p", "-S", "-g", "-u", "-k", "-L", "-h":
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
	left := r.condOperand(x.X)

	if x.Op == "==" || x.Op == "=" || x.Op == "!=" {
		// A process substitution in this position is one shell's alone, and
		// the question is asked before the word is expanded: a shell that
		// refuses it must not have started the command first, which is
		// observable because the command has side effects.
		if err := r.condProcSubAllowed(x.Y); err != nil {
			return false, err
		}
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
	right := r.condOperand(x.Y)
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
		l, err := r.condArith(left)
		if err != nil {
			return false, err
		}
		rv, err := r.condArith(right)
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

	case "=~":
		// The one place the pattern language is regular expressions rather
		// than globs. bash treats a *quoted* right operand as a literal
		// string; ksh93 and zsh keep it a regex. Following bash, which
		// docs/spec/semantics.md records as the axis default.
		pat := right
		// bash treats a quoted right operand as a literal string; ksh93 and
		// zsh keep it a regex, so quoting one is unportable either way.
		if x.Y.IsQuoted() && r.ask(r.sem().RegexQuotingMakesLiteral, "quoting a =~ regex making it literal") {
			pat = regexp.QuoteMeta(pat)
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return false, arithError{msg: "invalid regular expression: " + pat}
		}
		// The captures are the point of matching, not a by-product: element 0
		// is the whole match and the rest are the groups. The core records
		// them and a dialect names the record — see regexmatch.go.
		//
		// The offsets are asked for rather than the texts, because the two
		// records a dialect can ask for want different things out of one
		// match: the dense array wants the strings, and the reporting
		// parameters want where each span began and ended. Matching twice to
		// get both would be two answers to one question.
		loc := re.FindStringSubmatchIndex(left)
		var m []string
		if loc != nil {
			m = make([]string, len(loc)/2)
			for i := range m {
				if loc[2*i] >= 0 {
					m[i] = left[loc[2*i]:loc[2*i+1]]
				}
			}
		}
		r.recordRegexMatch(m)
		r.publishRegexCapture(left, loc)
		return loc != nil, nil

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
	return strings.Join(r.expandWordNoSplit(w), "")
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
	if strings.TrimSpace(text) == "" {
		return 0, nil
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
	// numeral alone and not the whole operand. That is exactly the stored
	// value's reader, which is why this is the same axis read at a site that
	// was missing it rather than one of its own (#1867).
	text = r.decimalLeadingNumeral(text)
	p := syntax.NewParser("", r.dialect())
	tree := p.ParseArithFor(text, syntax.Pos{})
	if perr := p.Err(); perr != nil {
		return 0, r.condArithFailed(r.diag().arithConstructFailure("[[", r.diag().ParseFailure(perr)))
	}
	v, err := r.evalArith(tree)
	if err != nil {
		return 0, r.condArithFailed(r.diag().arithConstructFailure("[[", r.arithFailure(text, err)))
	}
	return v, nil
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
// refuses it — `process substitution <(x) cannot be used here`, status 2;
// ksh93 refuses while reading and dash has no `[[ ]]` at all. Three shells
// say no and only the moment and the wording differ, which is the line
// between the axis and the Diagnostics vector.
func (r *Runner) condProcSubAllowed(w *syntax.Word) error {
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
		// the rest of the command string does not run, and the status is 2
		// — which is neither the 1 a condition that simply did not hold
		// gives nor the status this dialect gives an ordinary fatal error,
		// so it is written here rather than routed through either.
		r.ctl = controlExit
		return condStatus{code: condProcSubRefusal(s.Kind)}
	}
	return nil
}

// condProcSubRefusal is the status a refused process substitution leaves
// behind in a condition.
//
// Two answers for one refusal, which is measured rather than a slip.
// 2026-09-11 on zsh 5.9.2, the only shell that reaches this wording:
//
//	[[ a == <(echo hi) ]]   process substitution <(echo hi) cannot be used here, status 2
//	[[ a == =(echo hi) ]]   process substitution =(echo hi) cannot be used here, status 1
//
// Same sentence, same abandoned input — `echo after` runs in neither — and a
// different status. It is a fact about the spelling rather than about the
// shell, since no second shell has the file form to disagree about, which is
// why it is written here beside the wording instead of becoming an axis
// nobody could answer twice.
func condProcSubRefusal(kind syntax.SpanKind) int {
	if kind == syntax.ProcSubstFile {
		return 1
	}
	return 2
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
