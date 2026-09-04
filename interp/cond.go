// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

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
