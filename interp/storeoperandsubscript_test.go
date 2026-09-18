// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The operand a builtin **writes through** — `read 'r[2]'` and
// `printf -v 'r[2]'` — and the three things it was not asking about the
// brackets it carries.
//
// Whether the dialect has brackets at all (#3516), what `@` and `*` mean in
// them (#3486, #3498), and whether the word in front of them is a name at all
// when `printf -v` is the builtin (#3515). One file, because they are one
// operand read by one pair of builtins, and a fix at one of them that skipped
// the other is how this family got into the state it was in.

// storeOpReadSrc is a `read` whose status is on the *same* line as the builtin
// and whose array is on the line after it, because "gave up the line" and
// "ended the script" print the same nothing when there is only one line.
//
// A group with a here-document rather than a pipeline, for the reason the
// empty-subscript rows next door use one: a pipeline's element already runs
// somewhere a give-up unwinds out of.
const storeOpReadSrc = "r=(1 2 3)\n" +
	`{ read 'r[@]'; echo "same=$?"; } <<EOF` + "\n" +
	"Y\nEOF\n" +
	`echo "next=$? r=[${r[*]}]"`

// storeOpPrintfSrc is the same operand one builtin over, which is the whole
// point of the pair: the answer is the store's and not the builtin's.
const storeOpPrintfSrc = "r=(1 2 3)\n" +
	`printf -v 'r[@]' %s Q; echo "same=$?"` + "\n" +
	`echo "next=$? r=[${r[*]}]"`

// `@` and `*` on a builtin's operand name the whole array rather than an
// element, and **no column reads them as arithmetic** — which is what this
// did in every dialect, so one column got the evaluator's sentence in place of
// its own and another refused a line it fills.
//
// Measured 2026-09-17, `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin /dev/null,
// a script file and again as one `-c` string, `r=(1 2 3)` in front and the
// status read on the same line:
//
//	bash 5.3.20   `r[@]: bad array subscript`, 1, `1 2 3`, the line goes on
//	ksh93u+       `read: @: arithmetic syntax error`, 1, `1 2 3`, likewise
//	zsh 5.9.2     silent at 0, and `r` is the one element `Y`
//
// `r[*]` answers identically in each, so there is one field and not two.
func TestAWholeArraySubscriptOnAStoreOperandIsNotAnExpression(t *testing.T) {
	for _, c := range []struct {
		name  string
		p     StoreOperandWholeArraySubscriptPolicy
		want  string
		after string
		says  string
	}{
		// Refused by the operand as written, nothing stored, and the rest of
		// the line still running — which is how this parts from a subscript
		// that will not evaluate, where the same column gives the command up.
		{"bad", StoreOperandWholeArraySubscriptIsBad, "same=1", "next=0 r=[1 2 3]", "bad array subscript"},
		// The brackets really are an expression here, and `@` is not an
		// operand, so the evaluator's own complaint is the answer.
		{"an expression", StoreOperandWholeArraySubscriptIsAnExpression, "same=1", "next=0 r=[1 2 3]", "operand expected"},
		// The whole name, not its elements: one element holding the field.
		{"every element", StoreOperandWholeArraySubscriptNamesEveryElement, "same=0", "next=0 r=[Y]", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := storeOpRun(t, func(s *Semantics) {
				s.StoreOperandWholeArraySubscript = c.p
			}, Diagnostics{}, storeOpReadSrc)
			if !strings.Contains(out, c.want) {
				t.Errorf("out %q is missing %q", out, c.want)
			}
			if !strings.Contains(out, c.after) {
				t.Errorf("out %q is missing %q", out, c.after)
			}
			if c.says == "" {
				if strings.Contains(out, ":") {
					t.Errorf("out %q writes a complaint where the column is silent", out)
				}
			} else if !strings.Contains(out, c.says) {
				t.Errorf("out %q is missing the complaint %q", out, c.says)
			}
		})
	}
}

// The refusing column's sentence names the **operand as written** and says
// nothing about arithmetic, which is the half a status cannot show: measured,
// bash 5.3.20 answers `r[@]: bad array subscript` where this sent `@` to the
// evaluator and reported `operand expected (error token is "@")`.
func TestAWholeArraySubscriptRefusalNamesTheOperandAndNotTheArithmetic(t *testing.T) {
	out, _ := storeOpRun(t, func(s *Semantics) {
		s.StoreOperandWholeArraySubscript = StoreOperandWholeArraySubscriptIsBad
	}, Diagnostics{BadArraySubscript: "SUB %[1]s %[2]s"}, storeOpReadSrc)
	if !strings.Contains(out, "SUB r @") {
		t.Errorf("out %q does not write the operand's own sentence", out)
	}
	if strings.Contains(out, "arithmetic") {
		t.Errorf("out %q reached the arithmetic evaluator", out)
	}
}

// `printf -v` answers exactly as `read` does, including the 1 — so the status
// here is the **refusal's** and not the builtin's own, which is the opposite
// of the bad-*name* refusal one branch over, where `printf` counts 2 and
// `read` counts 1. Measured 2026-09-17 on bash 5.3.20 at both builtins.
func TestAWholeArraySubscriptAnswersTheSameAtBothStoreBuiltins(t *testing.T) {
	for _, c := range []struct {
		name  string
		p     StoreOperandWholeArraySubscriptPolicy
		want  string
		after string
	}{
		{"bad", StoreOperandWholeArraySubscriptIsBad, "same=1", "next=0 r=[1 2 3]"},
		{"every element", StoreOperandWholeArraySubscriptNamesEveryElement, "same=0", "next=0 r=[Q]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := storeOpRun(t, func(s *Semantics) {
				s.StoreOperandWholeArraySubscript = c.p
			}, Diagnostics{}, storeOpPrintfSrc)
			if !strings.Contains(out, c.want) || !strings.Contains(out, c.after) {
				t.Errorf("out %q, want %q and %q", out, c.want, c.after)
			}
		})
	}
}

// A name **declared a table** holds a key in its brackets rather than an
// expression, and the panel there is not the panel above — which is why it is
// a second field. Measured 2026-09-17 with `typeset -A m; m[k]=v` in front:
// bash 5.3.20 and ksh93u+ both store under the one-character key `@` at 0, and
// zsh 5.9.2 refuses `m: attempt to set slice of associative array` and ends
// the input.
func TestAWholeArraySubscriptOverATableIsItsOwnAnswer(t *testing.T) {
	const src = "typeset -A m; m[k]=v\n" +
		`{ read 'm[@]'; echo "same=$?"; } <<EOF` + "\n" +
		"Z\nEOF\n" +
		`echo "next=$? n=${#m[@]}"`
	t.Run("an ordinary key", func(t *testing.T) {
		out, _ := storeOpRun(t, func(s *Semantics) {
			s.StoreOperandWholeArraySubscriptOverATable = WholeArraySubscriptIsAnOrdinaryKey
		}, Diagnostics{}, src)
		if !strings.Contains(out, "same=0") || !strings.Contains(out, "n=2") {
			t.Errorf("out %q, want the key stored at 0", out)
		}
	})
	t.Run("a slice of a table", func(t *testing.T) {
		out, _ := storeOpRun(t, func(s *Semantics) {
			s.StoreOperandWholeArraySubscriptOverATable = WholeArraySubscriptIsASliceOfATable
		}, Diagnostics{SliceOfAnAssociativeArray: "SLICE %[1]s"}, src)
		if !strings.Contains(out, "SLICE m") {
			t.Errorf("out %q does not refuse the table by name", out)
		}
		if strings.Contains(out, "next=") {
			t.Errorf("out %q carried on past a refusal that ends the input", out)
		}
	})
}

// The table is read from a field of its own and **not** from the assignment's,
// which is measured rather than tidy: one column swaps sides between the two
// constructs, answering `m[@]=Z` with `invalid subscript in assignment` and
// the same brackets on an operand with the key at 0. So the assignment's own
// answer must not reach here.
func TestAWholeArraySubscriptOperandDoesNotReadTheAssignmentsField(t *testing.T) {
	const src = "typeset -A m; m[k]=v\n" +
		`{ read 'm[@]'; echo "same=$?"; } <<EOF` + "\n" +
		"Z\nEOF\n" +
		`echo "next=$? n=${#m[@]}"`
	out, _ := storeOpRun(t, func(s *Semantics) {
		s.StoreOperandWholeArraySubscriptOverATable = WholeArraySubscriptIsAnOrdinaryKey
		s.WholeArraySubscriptAssigningATable = WholeArraySubscriptIsInvalidInAnAssignment
		s.WholeArraySubscriptAssigningAnArray = WholeArraySubscriptIsInvalidInAnAssignment
	}, Diagnostics{InvalidSubscriptInAssignment: "ASSIGN %s"}, src)
	if strings.Contains(out, "ASSIGN") {
		t.Errorf("out %q answered an operand with the assignment's field", out)
	}
	if !strings.Contains(out, "same=0") || !strings.Contains(out, "n=2") {
		t.Errorf("out %q, want the key stored at 0", out)
	}
}

// A dialect with no arrays has no subscripts either, so a bracketed operand is
// simply a word holding characters a variable name may not hold — and it gets
// the one sentence that dialect gives any other bad name.
//
// Measured 2026-09-17, `/bin/dash`, a script file: `read 'r[2]' < in.txt` is
// `read: r[2]: bad variable name` at 2 with nothing stored, and `r[b c]`,
// `r[]`, `r[@]` and `r[*]` all get that same sentence, as does the bare `1x`.
// Before this the operand walked into the array machinery and refused array
// axes by name there, after reporting 0 (#3516).
func TestABracketedStoreOperandIsABadNameWithoutArrays(t *testing.T) {
	const src = `{ read 'r[2]'; echo "same=$?"; } <<EOF` + "\n" +
		"Y\nEOF\n" +
		`echo "next=$? r=[$r]"`
	for _, c := range []struct {
		name  string
		takes Answer
		bad   bool
	}{
		// The brackets are a subscript, so the element is filled and nothing
		// is said — which is what four of the six columns do.
		{"with subscripts", Yes, false},
		// They are not, so the word is refused as the name it is not.
		{"without subscripts", No, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := storeOpRun(t, func(s *Semantics) {
				s.StoreOperandTakesASubscript = c.takes
			}, Diagnostics{BuiltinBadName: map[string]string{"read": "BAD %[2]s"}}, src)
			if said := strings.Contains(out, "BAD r[2]"); said != c.bad {
				t.Errorf("out %q, want a bad-name refusal: %v", out, c.bad)
			}
			if strings.Contains(out, "no dialect was chosen") {
				t.Errorf("out %q refuses an array axis in a dialect with no arrays", out)
			}
		})
	}
}

// And an unanswered axis is **read** rather than asked, which is the one place
// this family does not complain: a dialect that says nothing here has arrays
// in the core and reaching them is what the path already did, so the fallback
// is a correct answer rather than a missing one.
func TestAnUnansweredStoreOperandSubscriptAxisKeepsTheElement(t *testing.T) {
	// A group with a here-document and not a pipeline: a pipeline's element
	// runs in a subshell, so the parent's array is untouched however the
	// element was filled and the row could not see the difference.
	const src = "r=(1 2 3)\n" +
		`{ read 'r[1]'; echo "same=$?"; } <<EOF` + "\n" +
		"Y\nEOF\n" +
		`echo "next=$? r=[${r[*]}]"`
	out, _ := storeOpRun(t, func(s *Semantics) {
		s.StoreOperandTakesASubscript = Unspecified
	}, Diagnostics{}, src)
	if strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out %q complains about an axis with a correct fallback", out)
	}
	if !strings.Contains(out, "r=[1 Y 3]") {
		t.Errorf("out %q did not fill the element", out)
	}
}

// `printf -v` judged its operand nowhere at all: it stored under whatever word
// it was handed and reported 0, so a name the script wrote badly became a
// parameter no expansion can read back, silently (#3515).
//
// Measured 2026-09-17, a script file and again under `( … )` and `-c`:
// bash 5.3.20 is “printf: `1x': not a valid identifier“ at 2 with the rest
// of the line running, and zsh 5.9.2 is `not an identifier: 1x` and the script
// ends. `a-b` is the same sentence as `1x` in both, so the leading-digit
// wording is not a split here.
func TestPrintfRefusesAnOperandThatIsNotAName(t *testing.T) {
	for _, operand := range []string{"1x", "a-b", "a b", "a.b"} {
		src := `printf -v '` + operand + `' %s Q; echo "same=$?"` + "\n" +
			`echo "next=$?"`
		out, _ := storeOpRun(t, func(s *Semantics) {
			s.BadNameToPrintfFatal = No
		}, Diagnostics{
			BuiltinBadName:          map[string]string{"printf": "BAD %[2]s"},
			BuiltinBadNameStatusFor: map[string]int{"printf": 2},
		}, src)
		if !strings.Contains(out, "BAD "+operand) {
			t.Errorf("%q: out %q does not refuse the operand", operand, out)
		}
		if !strings.Contains(out, "same=2") {
			t.Errorf("%q: out %q does not carry the builtin's own status", operand, out)
		}
		if !strings.Contains(out, "next=0") {
			t.Errorf("%q: out %q did not go on with the line", operand, out)
		}
	}
}

// The refusal ends the script where the dialect says so, and that is a field
// of its own rather than `read`'s read twice: the two builtins part on the
// status in the column that carries on, which is the same evidence that
// separated `read`'s fatality from `unset`'s.
func TestPrintfsBadNameRefusalEndsTheScriptWhereTheDialectSaysSo(t *testing.T) {
	const src = `printf -v '1x' %s Q; echo "same=$?"` + "\n" + `echo "next=$?"`
	for _, c := range []struct {
		name  string
		fatal Answer
		after bool
	}{
		{"reported", No, true},
		{"fatal", Yes, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := storeOpRun(t, func(s *Semantics) {
				s.BadNameToPrintfFatal = c.fatal
			}, Diagnostics{}, src)
			if reached := strings.Contains(out, "next="); reached != c.after {
				t.Errorf("out %q, want the next command reached: %v", out, c.after)
			}
			if !strings.Contains(out, "not a valid identifier") {
				t.Errorf("out %q does not refuse the operand", out)
			}
		})
	}
}

// A **subscripted** operand is still a name to `printf -v` where the dialect
// has subscripts, which is the row a second copy of the rule would have got
// wrong: measured, bash 5.3.20 fills the element for `printf -v 'r[1]'` and
// refuses `printf -v '1x'` in the same run.
func TestPrintfTakesASubscriptedOperandWhereTheDialectHasSubscripts(t *testing.T) {
	const src = "r=(1 2 3)\n" +
		`printf -v 'r[1]' %s Q; echo "same=$? r=[${r[*]}]"`
	out, _ := storeOpRun(t, nil, Diagnostics{}, src)
	if !strings.Contains(out, "same=0 r=[1 Q 3]") {
		t.Errorf("out %q refused a subscripted operand it fills", out)
	}
}

// An axis nobody answered is refused by name rather than guessed at: one
// column refuses the brackets, one evaluates them and one fills the array, and
// no two of those stand in for each other.
func TestAWholeArraySubscriptOperandRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := storeOpRun(t, func(s *Semantics) {
		s.StoreOperandWholeArraySubscript = StoreOperandWholeArraySubscriptUnspecified
	}, Diagnostics{}, storeOpReadSrc)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out %q is not a refusal naming the axis", out)
	}
	if !strings.Contains(out, "same=2") {
		t.Errorf("out %q does not leave the refusal's status behind", out)
	}
}

// storeOpRun answers everything a store through an operand needs except the
// axis under test, so a row varies that one alone.
func storeOpRun(t *testing.T, also func(*Semantics), dg Diagnostics, src string) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		// The option has to exist before a row about the store behind it can
		// reach one.
		s.PrintfAssignsWithV = Yes
		s.StoreOperandTakesASubscript = Yes
		s.StoreOperandWholeArraySubscript = StoreOperandWholeArraySubscriptIsAnExpression
		s.StoreOperandWholeArraySubscriptOverATable = WholeArraySubscriptIsAnOrdinaryKey
		// The neighboring refusals are answered flat so that a row varies the
		// brackets alone: how much an unevaluable subscript gives up, and
		// whether a bad name ends the script, are each their own axis.
		s.BadSubscriptToAnOutputOperand = BadSubscriptReported
		s.BadNameToReadFatal = No
		s.BadNameToPrintfFatal = No
		s.FatalErrorStatusIsOne = Yes
		if also != nil {
			also(s)
		}
	}, dg, src, RouteUnspecified)
}
