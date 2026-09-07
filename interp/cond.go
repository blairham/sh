// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"regexp"
	"strconv"
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
	switch x.Op {
	case "-n":
		return s != "", nil
	case "-z":
		return s == "", nil
	case "-t":
		// A terminal test on a descriptor this shell may not even own.
		// Never true here: the streams are io.Writers, which is the honest
		// answer for a library rather than a guess about the process's
		// descriptors — the same answer `test -t` gives. An operand that is
		// not a number is a question of its own first.
		if _, err := strconv.Atoi(strings.TrimSpace(s)); err != nil &&
			r.ask(r.sem().TerminalTestRequiresANumber, "`[[ -t x ]]` refusing a non-number") {
			return false, arithError{msg: Wording(r.diag().TestIntegerExpected,
				"%[2]s: %[1]s: integer expected", s, "[[")}
		}
		return false, nil
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

	switch x.Op {
	case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		// The word-spelled operators compare numbers. `<` and `>` compare
		// strings, which is why `[[ 10 > 9 ]]` is false and `[[ 10 -gt 9 ]]`
		// is true — the sharpest trap in the construct.
		l, lerr := strconv.Atoi(strings.TrimSpace(left))
		rv, rerr := strconv.Atoi(strings.TrimSpace(r.condOperand(x.Y)))
		if lerr != nil || rerr != nil {
			return false, arithError{msg: "integer expression expected"}
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
		pat := r.condOperand(x.Y)
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
		m := re.FindStringSubmatch(left)
		r.recordRegexMatch(m)
		return m != nil, nil

	case "==", "=", "!=":
		// A process substitution in this position is one shell's alone, and
		// the question is asked before the word is expanded: a shell that
		// refuses it must not have started the command first, which is
		// observable because the command has side effects.
		if err := r.condProcSubAllowed(x.Y); err != nil {
			return false, err
		}
		// Unquoted, the right operand is a pattern; quoted, a literal. Only
		// the spans still know which, which is why the tree keeps a word.
		got := r.matchPatternR(r.patternOf(x.Y), left, true)
		if x.Op == "!=" {
			return !got, nil
		}
		return got, nil

	case "-nt", "-ot", "-ef":
		return r.compareFiles(x.Op, left, r.condOperand(x.Y))

	case "<":
		return left < r.condOperand(x.Y), nil
	case ">":
		return left > r.condOperand(x.Y), nil
	}
	return false, arithError{msg: "unsupported test " + x.Op}
}

// condOperand expands a word to a single string. Nothing inside `[[ ]]` is
// split or globbed, so joining is the whole of it.
func (r *Runner) condOperand(w *syntax.Word) string {
	return strings.Join(r.expandWordNoSplit(w), "")
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
		if s.Kind != syntax.ProcSubstIn && s.Kind != syntax.ProcSubstOut {
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
		return condStatus{code: 2}
	}
	return nil
}

// procSubSource is a process substitution as it was written. The span keeps
// its inside and its direction, and a diagnostic names the whole of it.
func procSubSource(s syntax.Span) string {
	open := "<("
	if s.Kind == syntax.ProcSubstOut {
		open = ">("
	}
	return open + s.Value + ")"
}
