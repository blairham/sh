// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild

import (
	"fmt"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// A ranked list of causes is worth more than a list of failures, and the
// difference is not presentation. Forty-seven scripts refusing on the same
// construct is one missing construct; forty-seven separate lines read as
// forty-seven problems and get triaged as forty-seven. The sweep is the only
// thing in a position to say which it is, because it is the only thing that
// saw all forty-seven.

// Cause is one reason, and every failure that shares it.
type Cause struct {
	// Reason is the failure with the parts that differ per file taken out: a
	// kind, and the token where the token comes from a fixed vocabulary.
	Reason string
	// Paths are the scripts that failed this way, in the order they were
	// swept.
	Paths []string
	// Example is the first failure of this kind, kept whole so a reader has a
	// line to look at rather than a category.
	Example Result
}

// Count is how many scripts this cause accounts for.
func (c Cause) Count() int { return len(c.Paths) }

// Causes groups failures by what went wrong and ranks them by how many scripts
// each accounts for.
//
// Ties break on the reason text so the report is the same twice on the same
// machine — the sweep's own answer already varies with what is installed, and
// a report whose *order* also moved would be unreadable against yesterday's.
func Causes(failures []Result) []Cause {
	index := map[string]int{}
	var causes []Cause
	for _, f := range failures {
		reason := Reason(f.Err)
		i, ok := index[reason]
		if !ok {
			i = len(causes)
			index[reason] = i
			causes = append(causes, Cause{Reason: reason, Example: f})
		}
		causes[i].Paths = append(causes[i].Paths, f.Path)
	}
	sort.SliceStable(causes, func(i, j int) bool {
		if causes[i].Count() != causes[j].Count() {
			return causes[i].Count() > causes[j].Count()
		}
		return causes[i].Reason < causes[j].Reason
	})
	return causes
}

// Reason is a parse failure with the per-file detail removed, so that two
// scripts failing on the same construct produce the same string.
//
// The position goes, because it is a fact about the file. An ordinary *word*
// goes the same way — a script refused at `myfunc` and one refused at
// `install_deps` have the same thing wrong with them — while an operator or a
// reserved word stays, because those come from a vocabulary the grammar knows
// and `unexpected )` and `unexpected fi` are two different gaps.
func Reason(err error) string {
	var perr *syntax.Error
	if !asParseError(err, &perr) {
		return err.Error()
	}
	switch perr.Kind {
	case syntax.ErrUnexpected:
		return "unexpected " + vocabulary(perr.Token, perr.Class)
	case syntax.ErrUnterminated:
		return unterminated(perr)
	case syntax.ErrUnmatched:
		return "unmatched " + quoteToken(perr.Token)
	case syntax.ErrBadSubstitution:
		return "bad substitution"
	case syntax.ErrForName:
		return "for: not a name"
	case syntax.ErrArithOperand, syntax.ErrArithOperandEnd,
		syntax.ErrArithOperator, syntax.ErrArithBadOperator:
		// The expression itself is per-file; which of the four arithmetic
		// complaints it drew is the cause.
		return "arithmetic: " + arithName(perr.Kind)
	default:
		return perr.Msg
	}
}

// unterminated names the construct that was left open, which is the cause. The
// closer it wanted is named too when the parser knew one, because `if` without
// `fi` and `case` without `esac` are the same shape and not the same gap.
func unterminated(perr *syntax.Error) string {
	switch {
	case perr.Expected != "":
		return "unterminated: wanted " + perr.Expected
	case perr.Innermost != "":
		return "unterminated: inside " + perr.Innermost
	default:
		return "unterminated"
	}
}

// vocabulary keeps a token that came from the grammar's own vocabulary and
// drops one that came from the script's.
func vocabulary(token string, class syntax.TokenClass) string {
	if class == syntax.ClassWord {
		return "a word"
	}
	return quoteToken(token)
}

func quoteToken(token string) string {
	if token == "" {
		return "end of input"
	}
	return fmt.Sprintf("%q", token)
}

func arithName(kind syntax.ErrorKind) string {
	switch kind {
	case syntax.ErrArithOperand:
		return "operand expected"
	case syntax.ErrArithOperandEnd:
		return "operand expected, input ended"
	case syntax.ErrArithOperator:
		return "operator expected"
	default:
		return "not an operator"
	}
}

func asParseError(err error, target **syntax.Error) bool {
	perr, ok := err.(*syntax.Error)
	if ok {
		*target = perr
	}
	return ok
}

// LineAt is the source line a position falls on, trimmed and shortened to
// something a report can print.
//
// It is the closest thing the sweep can offer to a minimal reproduction: the
// sweep found the construct rather than inventing it, so the honest artifact
// is the line it was found on, and cutting that down further is a person's job.
func LineAt(src string, line int) string {
	if line < 1 {
		return ""
	}
	lines := strings.Split(src, "\n")
	if line > len(lines) {
		return ""
	}
	text := strings.TrimSpace(lines[line-1])
	const most = 120
	if len(text) > most {
		text = text[:most] + "…"
	}
	return text
}
