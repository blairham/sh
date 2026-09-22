// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strconv"
	"strings"
	"testing"
)

// An array literal that runs out **inside a command substitution** is still
// the literal's refusal, and the substitution writes its own complaint around
// it rather than in front of it.
//
// #4132 made a literal's refusal survive an enclosing *construct*. This is
// the last shape of the family and a different mechanism, which is why it was
// left out: the substitution's unmatched `$(` is recorded by the **lexer**
// rather than raised by the grammar, and the body is read by a parser of its
// own — see Lexer.parseToClose — whose refusal had nowhere to go. It was
// dropped, and the substitution's complaint about the same end of input stood
// in its place, naming the substitution's line at status 2 where bash names
// the literal's at 1.
//
// Measured 2026-09-22 against bash 5.3.20 through `-c`, with the status taken
// from the shell itself rather than from a pipeline:
//
//	$(a=(        line 1: unexpected EOF while looking for matching `)'  status 1
//	x=$(a=(      the same                                              status 1
//
// where this shell answered the same sentence at `line 2` and status 2.
//
// # Which line, and whose `)`
//
// The line follows the **literal** and not the substitution, which is
// measured rather than assumed: `$(⏎a=(` is `line 2` in bash, so a refusal
// carrying the substitution's opener would have been right for the one-line
// shapes and wrong here.
//
// # The clause, and the one construct that tells the two apart
//
// A body that refused a **token** is written with the substitution's own
// clause after it — `syntax error near unexpected token `;' while looking for
// matching `)'` — and a body that merely **ran out** is written alone. Both
// parentheses are spelled the same in `$( … )`, so the split is invisible
// there; bash 5.3.20's `${ … }` substitution is what says it out loud,
// measured the same day:
//
//	${ a=(      unexpected EOF while looking for matching `)'   the literal's
//	${ a=( ;    … unexpected token `;' while looking for matching `}'   the body's
//
// so the clause is the *substitution's* closer and it is written only for the
// token. That is the split Diagnostics.substitutionBodyReplacesTheQuote
// already drew from the other end, reached here through Error.BodyRefusal.
//
// # The status is the point
//
// bash exits 1 for `a=(` and 2 for `echo $(` over the same message and the
// same end of input, which is what syntax.File.Refused exists to keep apart.
// So the refusal is handed to the parser rather than recorded as the lexer's
// error: recording it would say 2, which is the answer this replaced.
func TestAnArrayLiteralsRefusalSurvivesTheSubstitutionAroundIt(t *testing.T) {
	const matching = "unexpected EOF while looking for matching `)'"
	for _, c := range []struct {
		name, src, want string
		line, status    int
	}{
		{"inside a substitution", "$(a=(", matching, 1, 1},
		{"inside an assigned one", "x=$(a=(", matching, 1, 1},
		{"after a command word", "echo $(a=(", matching, 1, 1},
		{"with elements read", "$( a=(1 2", matching, 1, 1},
		{"two substitutions deep", "$($(a=(", matching, 1, 1},
		{"inside a process substitution", "cat <(a=(", matching, 1, 1},
		{"a construct inside the substitution", "$( ( a=(", matching, 1, 1},
		{"after a line that ran", "echo before; $(a=(", matching, 1, 1},
		// The line is the literal's rather than the substitution's, which
		// only a shape spread over two lines can say.
		{"the literal on the next line", "$(\na=(", matching, 2, 1},
		// The token branch, and the clause that comes with it.
		{
			"a token refusal inside a substitution", "$(a=( ;",
			"syntax error near unexpected token `;' while looking for matching `)'", 1, 1,
		},
		{
			"a token refusal in an assigned one", "x=$(a=( ;",
			"syntax error near unexpected token `;' while looking for matching `)'", 1, 1,
		},
		// Controls. The literal alone was always right; a substitution that
		// runs out with no literal in it keeps its own complaint, at the end
		// of the input and at the status of a file that would not parse; and
		// a closed literal inside a closed substitution runs.
		{"the literal alone", "a=(", matching, 1, 1},
		{"a substitution alone", "echo $(", matching, 2, 2},
		{"a body that ran out with no literal", "v=$(echo hi", matching, 2, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, status := runScript(t, "-c", c.src)
			if out != "" {
				t.Errorf("ran %q: wrote %q, want nothing", c.src, out)
			}
			if status != c.status {
				t.Errorf("ran %q: status %d, want %d (said %q)", c.src, status, c.status, errs)
			}
			if !strings.Contains(errs, c.want) {
				t.Errorf("ran %q: said %q, want it to carry %q", c.src, errs, c.want)
			}
			if at := "line " + strconv.Itoa(c.line) + ":"; !strings.Contains(errs, at) {
				t.Errorf("ran %q: said %q, want it blamed on %q", c.src, errs, at)
			}
		})
	}
}

// A refused token inside a substitution is echoed back with the line it stood
// on, which is the quote bash writes under the complaint.
//
// It is a second test because it is a second mechanism: the sentence comes
// from Diagnostics.substitutionBodyReplacesTheQuote and the quote from
// Diagnostics.offendingLine, and the second one used to stop at the
// substitution's *unmatched* error — a kind that has no offending line — and
// write nothing. So `$(esac` lost its quote as well, with no array literal
// anywhere in it, and that row is here to say the fix is not the literal's.
//
// Measured 2026-09-22, bash 5.3.20 through `-c`.
func TestARefusedTokenInsideASubstitutionIsEchoedBack(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an array literal's token", "$(a=( ;", "`$(a=( ;'"},
		{"an element separator", "$(a=(p & q", "`$(a=(p & q'"},
		{"a reserved word with no literal", "$(esac", "`$(esac'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, errs, _ := runScript(t, "-c", c.src)
			if !strings.Contains(errs, c.want) {
				t.Errorf("ran %q: said %q, want it to quote %s", c.src, errs, c.want)
			}
		})
	}
}
