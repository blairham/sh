// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An operand C's reader takes whole is the core's, with no dialect consulted.
//
// Run under CoreSemantics deliberately: every column in the panel answers
// these the same way, so an implementation that reached an axis to produce
// them would be asking a question the panel does not have. The range rows are
// the ones #2727 was about — `strconv` hands back the infinity it overflowed
// to *and* `ErrRange`, and the value was being discarded with the error.
func TestPrintfReadsTheNumberCReads(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"a plain integer", `printf '[%d]' 42`, "[42]"},
		{"a negative one", `printf '[%d]' -42`, "[-42]"},
		{"hexadecimal", `printf '[%d]' 0x10`, "[16]"},
		// `007` and `00` are the same number under every reading, which is
		// why the octal question below is decided by comparing the readings
		// and not by spotting the leading zero.
		{"a leading zero the readings agree about", `printf '[%d]' 007`, "[7]"},
		{"and one that is only zeros", `printf '[%d]' 00`, "[0]"},
		{"blanks around it", `printf '[%d]' ' 42 '`, "[42]"},
		{"a float at a float conversion", `printf '[%f]' 1.5`, "[1.500000]"},
		// C's `strtod` takes a hexadecimal float where Go's ParseFloat wants
		// a binary exponent. All six columns answer 16.
		{"hexadecimal at a float conversion", `printf '[%f]' 0x10`, "[16.000000]"},
		{"an exponent at a float conversion", `printf '[%f]' 1e3`, "[1000.000000]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// An operand too large or too small for a double keeps its value.
//
// #2727: `strconv.ParseFloat` returns the infinity it overflowed to *and*
// `ErrRange`, and taking the error while discarding the value wrote
// `0.000000` where six columns write `inf`. The value is unanimous; only
// whether anything is *said* about it is a dialect's, which is why this runs
// with PrintfReportsBadNumber answered rather than under CoreSemantics.
func TestPrintfRangeErrorKeepsTheValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"an overflow is the infinity, not a zero", `printf '[%f]' 1e400`, "[inf]"},
		{"and keeps its sign", `printf '[%f]' -1e400`, "[-inf]"},
		{"the value reaches the field", `printf '[%10f]' 1e400`, "[       inf]"},
		{"and the conversion", `printf '[%E]' 1e400`, "[INF]"},
		{"an underflow is a zero", `printf '[%f]' 1e-400`, "[0.000000]"},
		{"a negative underflow keeps its sign", `printf '[%f]' -1e-400`, "[-0.000000]"},
		{"a denormal is the denormal", `printf '[%g]' 1e-320`, "[9.99989e-321]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfNonFiniteIsConverted = Yes
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The three readings of Semantics.PrintfNumberOperand.
//
// Every operand here is one that at least two of the three readings answer
// differently. An operand all three agree about would prove only that
// something was read — which is what `abc` is here for, as a control rather
// than as evidence.
func TestPrintfNumberOperandIsThreeReadings(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		src                        string
		whole, leading, arithmetic string
	}{
		// A fraction parts reading something from reading nothing.
		{"a fraction", `printf '[%d]' 1.5`, "[0]", "[1]", "[1]"},
		{"one that would round up", `printf '[%d]' 1.9`, "[0]", "[1]", "[1]"},
		{"a negative one truncates toward zero", `printf '[%d]' -1.9`, "[0]", "[-1]", "[-1]"},
		// An exponent parts `strtoimax`, which stops at the `e`, from an
		// evaluation, which does not.
		{"an exponent", `printf '[%d]' 1e3`, "[0]", "[1]", "[1000]"},
		{"one with a fraction in it", `printf '[%d]' 1.5e3`, "[0]", "[1]", "[1500]"},
		// A leading zero parts C's base-zero reader from arithmetic, and is
		// the row that makes the third reading a reading of the operand
		// rather than a longer scan.
		{"a leading zero is octal to C and decimal to arithmetic", `printf '[%d]' 010`, "[8]", "[8]", "[10]"},
		{"and signed", `printf '[%d]' -010`, "[-8]", "[-8]", "[-10]"},
		// An operator parts an evaluation from any kind of number scan.
		{"an operator", `printf '[%d]' 1+1`, "[0]", "[1]", "[2]"},
		{"and one that binds tighter", `printf '[%d]' '2*3'`, "[0]", "[2]", "[6]"},
		{"a trailing word", `printf '[%d]' 42abc`, "[0]", "[42]", "[0]"},
		{"a float conversion reads its own kind of number", `printf '[%f]' 1e3abc`, "[0.000000]", "[1000.000000]", "[0.000000]"},
		{"and an integer one reads its own", `printf '[%d]' 1e3abc`, "[0]", "[1]", "[0]"},
		// The controls: operands every reading answers the same way, which
		// have to stay still while the readings move.
		{"an unset name is zero under all three", `printf '[%d]' abc`, "[0]", "[0]", "[0]"},
		{"and so is a name with digits after it", `printf '[%d]' abc42`, "[0]", "[0]", "[0]"},
		{"hexadecimal is sixteen under all three", `printf '[%d]' 0x10`, "[16]", "[16]", "[16]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reading := range []struct {
				value PrintfNumberReading
				want  string
			}{
				{PrintfNumberWholeOperand, tc.whole},
				{PrintfNumberLeadingNumber, tc.leading},
				{PrintfNumberArithmetic, tc.arithmetic},
			} {
				sem := printfSem()
				sem.PrintfNumberOperand = reading.value
				out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
				// Contains rather than equal: the arithmetic reading writes
				// its own complaint beside the value, and what this test is
				// about is the value.
				if !strings.Contains(out, reading.want) {
					t.Errorf("%v: got %q, want it to hold %q", reading.value, out, reading.want)
				}
			}
		})
	}
}

// The arithmetic reading is the shell's own arithmetic, not a self-contained
// number grammar that happens to know `+`.
//
// A name is read out of the shell's variables and an assignment outlives the
// conversion, which is what settles what kind of evaluation it is. Measured
// 2026-09-14: `printf '%d' 'x=5'` leaves `x` set to 5 in zsh 5.9.2 and
// ksh93u+ and leaves it unset in the other five columns.
func TestPrintfArithmeticOperandIsTheShellsArithmetic(t *testing.T) {
	sem := printfSem()
	sem.PrintfNumberOperand = PrintfNumberArithmetic

	out, st := run(t, `v=7; printf '[%d]' v`, func(r *Runner) { r.Semantics = &sem })
	if out != "[7]" || st != 0 {
		t.Errorf("a name: got %q status %d, want [7] and 0", out, st)
	}

	out, st = run(t, `printf '[%d]' 'x=5'; printf '[%s]' "$x"`, func(r *Runner) { r.Semantics = &sem })
	if out != "[5][5]" || st != 0 {
		t.Errorf("an assignment: got %q status %d, want [5][5] and 0", out, st)
	}

	// The control, and the one that would otherwise let a reading which
	// merely *parsed* arithmetic pass: a name the shell does not have is
	// zero here, and silently.
	out, st = run(t, `printf '[%d]' nosuchname_zz`, func(r *Runner) { r.Semantics = &sem })
	if out != "[0]" || st != 0 {
		t.Errorf("an unset name: got %q status %d, want [0] and 0", out, st)
	}
}

// What survives an operand the arithmetic reading would not evaluate.
//
// Semantics.PrintfRefusedOperandKeepsItsLeadingNumber, and the number kept is
// the one C's `strtod` would have read rather than the one `strtoimax` would:
// ksh93u+ answers 1000 for `printf '%d' 1e3abc`, which an integer reader
// stopping at the `e` could not have produced.
func TestPrintfRefusedArithmeticOperandKeepsItsLeadingNumber(t *testing.T) {
	for _, tc := range []struct {
		name         string
		src          string
		kept, zeroed string
	}{
		{"a trailing word", `printf '[%d]' 42abc`, "[42]", "[0]"},
		{"an exponent before it", `printf '[%d]' 1e3abc`, "[1000]", "[0]"},
		{"a fraction before it", `printf '[%f]' 1.5abc`, "[1.500000]", "[0.000000]"},
		{"hexadecimal before it", `printf '[%d]' 0x10zz`, "[16]", "[0]"},
		{"a second number after a blank", `printf '[%d]' '3 4'`, "[3]", "[0]"},
		// The control: what is kept is a *prefix* and not a search, so an
		// operand with nothing at its front is zero under both readings.
		{"nothing at the front", `printf '[%d]' abc42`, "[0]", "[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reading := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.kept}, {No, tc.zeroed}} {
				sem := printfSem()
				sem.PrintfNumberOperand = PrintfNumberArithmetic
				sem.PrintfRefusedOperandKeepsItsLeadingNumber = reading.answer
				out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
				if !strings.Contains(out, reading.want) {
					t.Errorf("%v: got %q, want it to hold %q", reading.answer, out, reading.want)
				}
			}
		})
	}
}

// Both axes are asked at the disagreement and nowhere else.
//
// Both halves are asserted, because either one alone is satisfiable by a
// mistake: an implementation that never asked would pass the first and an
// implementation that always asked would pass the second.
func TestPrintfNumberOperandIsAskedWhereTheReadingsDiffer(t *testing.T) {
	t.Run("an operand every reading takes whole needs no dialect", func(t *testing.T) {
		for _, src := range []string{
			`printf '[%d]' 42`,
			`printf '[%d]' -42`,
			`printf '[%d]' 0x10`,
			`printf '[%d]' 007`,
			`printf '[%f]' 1.5`,
			`printf '[%f]' 1e3`,
			`printf '[%f]' 0x10`,
			`printf '[%f]' inf`,
		} {
			sem := CoreSemantics()
			out, st := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if strings.Contains(out, "no dialect") || st == 2 {
				t.Errorf("%s: got %q status %d, want no question", src, out, st)
			}
		}
	})

	t.Run("an operand the readings differ about is refused", func(t *testing.T) {
		for _, src := range []string{
			`printf '[%d]' 1.5`,
			`printf '[%d]' 1e3`,
			`printf '[%d]' 010`,
			`printf '[%d]' 1+1`,
			`printf '[%d]' 42abc`,
		} {
			sem := CoreSemantics()
			out, st := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if !strings.Contains(out, "no dialect") || st != 2 {
				t.Errorf("%s: got %q status %d, want the unanswered refusal", src, out, st)
			}
		}
	})

	t.Run("the second axis is asked only after an arithmetic failure", func(t *testing.T) {
		// The reading is answered and the operand evaluates, so the question
		// of what a failure leaves behind never arises.
		sem := printfSem()
		sem.PrintfNumberOperand = PrintfNumberArithmetic
		out, st := run(t, `printf '[%d]' 1e3`, func(r *Runner) { r.Semantics = &sem })
		if out != "[1000]" || st != 0 {
			t.Errorf("got %q status %d, want [1000] and 0", out, st)
		}
		// And the readings that never evaluate do not reach it either.
		sem = printfSem()
		sem.PrintfNumberOperand = PrintfNumberLeadingNumber
		out, st = run(t, `printf '[%d]' 42abc`, func(r *Runner) { r.Semantics = &sem })
		if out != "[42]" || st != 0 {
			t.Errorf("got %q status %d, want [42] and 0", out, st)
		}
		// Where it does arise, an unanswered second axis refuses.
		sem = printfSem()
		sem.PrintfNumberOperand = PrintfNumberArithmetic
		sem.PrintfRefusedOperandKeepsItsLeadingNumber = Unspecified
		out, st = run(t, `printf '[%d]' 42abc`, func(r *Runner) { r.Semantics = &sem })
		if !strings.Contains(out, "no dialect") || st != 2 {
			t.Errorf("got %q status %d, want the unanswered refusal", out, st)
		}
	})
}

// The value, the complaint and the status are three observations.
//
// A range error is where they came apart: the value survives in six columns
// whatever they say about it, the complaint is
// Semantics.PrintfReportsBadNumber, and the sentence is the dialect's — bash
// and dash write C's errno back in both directions where BusyBox ash writes
// its ordinary bad-number line.
func TestPrintfRangeErrorSaysAndCostsSeparately(t *testing.T) {
	t.Run("the value survives whether or not anything is said", func(t *testing.T) {
		for _, answer := range []Answer{Yes, No} {
			sem := printfSem()
			sem.PrintfReportsBadNumber = answer
			out, _ := run(t, `printf '[%f]' 1e400`, func(r *Runner) { r.Semantics = &sem })
			if !strings.Contains(out, "[inf]") {
				t.Errorf("%v: got %q, want it to hold [inf]", answer, out)
			}
		}
	})

	t.Run("the silent reading says nothing and costs nothing", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfReportsBadNumber = No
		out, st := run(t, `printf '[%f]' 1e400`, func(r *Runner) { r.Semantics = &sem })
		if out != "[inf]" || st != 0 {
			t.Errorf("got %q status %d, want [inf] and 0", out, st)
		}
	})

	t.Run("an underflow earns the same sentence as an overflow", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfReportsBadNumber = Yes
		diag := Diagnostics{PrintfNumberOutOfRange: "printf: %[1]s: Result too large"}
		for _, src := range []string{`printf '[%f]' 1e400`, `printf '[%f]' 1e-400`, `printf '[%g]' 1e-320`} {
			out, st := run(t, src, func(r *Runner) { r.Semantics = &sem; r.Diagnostics = &diag })
			if !strings.Contains(out, "Result too large") || st != 1 {
				t.Errorf("%s: got %q status %d, want the range sentence and 1", src, out, st)
			}
		}
	})

	t.Run("a written zero is not a range error", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfReportsBadNumber = Yes
		diag := Diagnostics{PrintfNumberOutOfRange: "printf: %[1]s: Result too large"}
		for _, src := range []string{`printf '[%f]' 0.0`, `printf '[%f]' 0`, `printf '[%f]' -0.0`} {
			out, st := run(t, src, func(r *Runner) { r.Semantics = &sem; r.Diagnostics = &diag })
			if strings.Contains(out, "Result too large") || st != 0 {
				t.Errorf("%s: got %q status %d, want no complaint", src, out, st)
			}
		}
	})

	t.Run("the integer conversions part the readings", func(t *testing.T) {
		// The reader that takes a prefix saturates the way `strtoimax` does;
		// the one that must read the whole operand is left with nothing,
		// which is BusyBox ash's `%d` beside its own `%f`.
		for _, tc := range []struct {
			reading PrintfNumberReading
			want    string
		}{
			{PrintfNumberLeadingNumber, "[9223372036854775807]"},
			{PrintfNumberWholeOperand, "[0]"},
		} {
			sem := printfSem()
			sem.PrintfNumberOperand = tc.reading
			out, _ := run(t, `printf '[%d]' 99999999999999999999`, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("%v: got %q, want %q", tc.reading, out, tc.want)
			}
		}
	})
}

// One column names the *base* the operand was spelled in, where the rest of
// the panel has one sentence for every bad number.
//
// The question is the operand's spelling and nothing else: the value, the
// status and the conversion are the general sentence's in every row, so a
// test that read the wording off the value would pass on rows that never
// reach the choice. Each row below is paired with one the other way — `08`
// against `0z`, `0x1z` against `0X1z` — because the two halves of the rule
// are what a single positive row cannot state. See
// Diagnostics.PrintfBadHexNumber for the measurement (#2764).
func TestPrintfBadNumberCanNameTheBase(t *testing.T) {
	diag := Diagnostics{
		PrintfBadNumber:      "printf: %[1]s: invalid number",
		PrintfBadHexNumber:   "printf: %[1]s: invalid hex number",
		PrintfBadOctalNumber: "printf: %[1]s: invalid octal number",
	}
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"a written 0x is hexadecimal", `printf '[%d]' 0x10zz`, "invalid hex number"},
		{"however little follows it", `printf '[%d]' 0x`, "invalid hex number"},
		{"a leading zero and a digit is octal", `printf '[%d]' 08`, "invalid octal number"},
		{"even where the octal run is only the zero", `printf '[%d]' 00z`, "invalid octal number"},
		{"a capital X is neither", `printf '[%d]' 0X1z`, "invalid number"},
		{"nor is a zero followed by anything else", `printf '[%d]' 0z`, "invalid number"},
		{"nor a zero and a point", `printf '[%d]' 0.5zz`, "invalid number"},
		{"a sign in front takes the base away", `printf '[%d]' +0x10zz`, "invalid number"},
		{"and so does a blank", `printf '[%d]' ' 08'`, "invalid number"},
		{"the float conversions ask the same question", `printf '[%f]' 0x1.8p3zz`, "invalid hex number"},
		{"and get the same answer for an operand with no base", `printf '[%f]' 0.5zz`, "invalid number"},
		{"an octal spelling reaches a float conversion too", `printf '[%f]' 08zz`, "invalid octal number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfReportsBadNumber = Yes
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem; r.Diagnostics = &diag })
			line, _, _ := strings.Cut(out, "\n")
			if !strings.HasSuffix(line, tc.want) || st != 1 {
				t.Errorf("got %q status %d, want a first line ending %q and 1", out, st, tc.want)
			}
		})
	}
}

// A dialect with no base-named sentence keeps the general one, which is what
// every column but the one that has them says for the same operands.
func TestPrintfBadNumberKeepsOneSentenceWithoutTheBaseWordings(t *testing.T) {
	diag := Diagnostics{PrintfBadNumber: "printf: %[1]s: invalid number"}
	for _, src := range []string{`printf '[%d]' 0x10zz`, `printf '[%d]' 08`, `printf '[%d]' abc`} {
		sem := printfSem()
		sem.PrintfReportsBadNumber = Yes
		out, st := run(t, src, func(r *Runner) { r.Semantics = &sem; r.Diagnostics = &diag })
		line, _, _ := strings.Cut(out, "\n")
		if !strings.HasSuffix(line, "invalid number") || st != 1 {
			t.Errorf("%s: got %q status %d, want the general sentence and 1", src, out, st)
		}
	}
}

// An operand C read as a number and could not hold is a range error and keeps
// the range sentence, base or no base — the base wording is the *general*
// sentence's refinement and not a reading of the operand's front.
func TestPrintfRangeErrorIsNotBaseNamed(t *testing.T) {
	diag := Diagnostics{
		PrintfBadNumber:        "printf: %[1]s: invalid number",
		PrintfBadHexNumber:     "printf: %[1]s: invalid hex number",
		PrintfNumberOutOfRange: "printf: %[1]s: Result too large",
	}
	sem := printfSem()
	sem.PrintfReportsBadNumber = Yes
	out, st := run(t, `printf '[%d]' 0xFFFFFFFFFFFFFFFFFF`, func(r *Runner) { r.Semantics = &sem; r.Diagnostics = &diag })
	if !strings.Contains(out, "Result too large") || strings.Contains(out, "hex") || st != 1 {
		t.Errorf("got %q status %d, want the range sentence and 1", out, st)
	}
}

// How **many times** the complaint about a refused operand goes out, which is
// a count and not a wording: one column writes the arithmetic line twice for
// a *floating* conversion and once for the integer one (#2823).
//
// The `1/0` rows are what say this is not the `invalid argument of type`
// warning's companion: a division by zero doubles too and earns no warning
// line at all. The `%d` beside each `%f` is the control that keeps the
// operand fixed while the conversion moves.
func TestPrintfFloatOperandIsEvaluatedTwice(t *testing.T) {
	base := printfSem()
	base.PrintfNumberOperand = PrintfNumberArithmetic
	base.PrintfRefusedOperandKeepsItsLeadingNumber = Yes
	diag := Diagnostics{
		PrintfArithOperandFailure: "printf: %[1]s",
		PrintfArithArgumentType:   "printf: warning: invalid argument of type %[1]s",
	}
	for _, tc := range []struct {
		name  string
		twice Answer
		src   string
		want  string
	}{
		{
			"a floating conversion says it twice", Yes, `printf '[%f]' 42abc`,
			"sh: printf: invalid number: 42abc\nsh: printf: invalid number: 42abc\n" +
				"sh: printf: warning: invalid argument of type f\n[42.000000]",
		},
		{
			"the integer one beside it says it once", Yes, `printf '[%d]' 42abc`,
			"sh: printf: invalid number: 42abc\nsh: printf: warning: invalid argument of type d\n[42]",
		},
		{
			"an evaluation failure doubles with no warning", Yes, `printf '[%f]' 1/0`,
			"sh: printf: division by zero\nsh: printf: division by zero\n[1.000000]",
		},
		{
			"and singly at the integer conversion", Yes, `printf '[%d]' 1/0`,
			"sh: printf: division by zero\n[1]",
		},
		{
			"the other column says it once", No, `printf '[%f]' 42abc`,
			"sh: printf: invalid number: 42abc\nsh: printf: warning: invalid argument of type f\n[42.000000]",
		},
		{
			"once for a division by zero as well", No, `printf '[%f]' 1/0`,
			"sh: printf: division by zero\n[1.000000]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := base
			sem.PrintfFloatOperandIsEvaluatedTwice = tc.twice
			out, _ := run(t, tc.src, func(r *Runner) {
				r.Semantics = &sem
				r.Diagnostics = &diag
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// And the count is asked only where a floating conversion's arithmetic has
// actually failed. Run *unanswered*, so a route that consults it refuses the
// command and names the axis.
func TestPrintfFloatDoublingIsAskedOnlyAtItsOwnDisagreement(t *testing.T) {
	base := printfSem()
	base.PrintfNumberOperand = PrintfNumberArithmetic
	base.PrintfRefusedOperandKeepsItsLeadingNumber = Yes
	for _, tc := range []struct {
		name    string
		src     string
		refused bool
	}{
		{"a floating conversion whose operand was refused", `printf '[%f]' 42abc`, true},
		{"an integer one settles nothing", `printf '[%d]' 42abc`, false},
		{"nor a floating conversion of a number", `printf '[%f]' 1.5`, false},
		{"nor one whose expression evaluates", `printf '[%f]' 1+1`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := base
			sem.PrintfFloatOperandIsEvaluatedTwice = Unspecified
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if got := strings.Contains(out, "evaluating an operand"); got != tc.refused {
				t.Errorf("got %q, want the axis consulted = %v", out, tc.refused)
			}
		})
	}
}

// One column writes a **second** line after the arithmetic complaint, naming
// the conversion character rather than the operand — and the same thing that
// decides the line decides the status (#2765).
//
// What parts them is whether the operand's *reading as a number* failed, or
// the expression around it did. The two are one observation and not two,
// which is why one wording turns both on.
func TestPrintfArithArgumentTypeIsTheStatusToo(t *testing.T) {
	sem := printfSem()
	sem.PrintfNumberOperand = PrintfNumberArithmetic
	sem.PrintfRefusedOperandKeepsItsLeadingNumber = Yes
	diag := Diagnostics{
		PrintfArithOperandFailure: "printf: %[1]s",
		PrintfArithArgumentType:   "printf: warning: invalid argument of type %[1]s",
	}
	set := func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &diag
	}
	for _, tc := range []struct {
		name   string
		src    string
		want   string
		status int
	}{
		// A numeral the reader refused: two lines, and the status.
		{
			"a refused numeral", `printf '[%d]' 42abc`,
			"sh: printf: invalid number: 42abc\nsh: printf: warning: invalid argument of type d\n[42]", 1,
		},
		// The letter is the conversion's own, whichever asked.
		{
			"the letter follows the conversion", `printf '[%f]' 42abc`,
			"sh: printf: invalid number: 42abc\nsh: printf: warning: invalid argument of type f\n[42.000000]", 1,
		},
		// And a `*` operand names the constant `.`, which is the same name
		// the refusal of a starved star uses.
		{
			"a star operand is named `.`", `printf '[%*d]' 42abc 7`,
			"sh: printf: invalid number: 42abc\nsh: printf: warning: invalid argument of type .\n[" + strings.Repeat(" ", 41) + "7]", 1,
		},
		// Text left over after a complete expression is a reading failure
		// too: the operand was a number and then something else.
		{
			"text left over", `printf '[%d]' '3 4'`,
			"sh: printf: 3 4: operator expected\nsh: printf: warning: invalid argument of type d\n[3]", 1,
		},
		// The expression failing is not. One line, and no status.
		{
			"a division by zero", `printf '[%d]' 1/0`,
			"sh: printf: division by zero\n[1]", 0,
		},
		{
			"an expression that wanted more", `printf '[%d]' '1+'`,
			"sh: printf: 1+: operand expected\n[1]", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, set)
			if out != tc.want || st != tc.status {
				t.Errorf("got %q status %d, want %q and %d", out, st, tc.want, tc.status)
			}
		})
	}

	// Without the wording nothing is split: every arithmetic failure writes
	// its one line and costs the status, which is what zsh does.
	t.Run("the other columns say one line and always report", func(t *testing.T) {
		plain := Diagnostics{PrintfArithOperandFailure: "printf: %[1]s"}
		one := func(r *Runner) {
			r.Semantics = &sem
			r.Diagnostics = &plain
		}
		for _, src := range []string{`printf '[%d]' 42abc`, `printf '[%d]' 1/0`} {
			out, st := run(t, src, one)
			if strings.Contains(out, "warning") || st != 1 {
				t.Errorf("%s: got %q status %d, want one line at 1", src, out, st)
			}
		}
	})
}

// A value an *integer* conversion cannot hold, which one column reports and
// the same column says nothing about at `%f` — so the range is the
// conversion's and not the operand's (#2765).
func TestPrintfIntegerOverflowIsTheConversionsRange(t *testing.T) {
	sem := printfSem()
	sem.PrintfNumberOperand = PrintfNumberArithmetic
	diag := Diagnostics{PrintfIntegerOverflow: "printf: warning: %[1]s: overflow exception"}
	set := func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &diag
	}
	out, st := run(t, `printf '[%d]' 99999999999999999999`, set)
	if want := "sh: printf: warning: 99999999999999999999: overflow exception\n[9223372036854775807]"; out != want || st != 1 {
		t.Errorf("got %q status %d, want %q and 1", out, st, want)
	}
	// The clamp is to the extreme of the type in both directions.
	out, st = run(t, `printf '[%d]' -99999999999999999999`, set)
	if want := "sh: printf: warning: -99999999999999999999: overflow exception\n[-9223372036854775808]"; out != want || st != 1 {
		t.Errorf("got %q status %d, want %q and 1", out, st, want)
	}
	// The same operand at a float conversion is a perfectly good number.
	if out, st := run(t, `printf '[%f]' 99999999999999999999`, set); out != "[100000000000000000000.000000]" || st != 0 {
		t.Errorf("at %%f: got %q status %d, want the value in silence at 0", out, st)
	}
	// An operand that overflowed a *double* is not asked about: the column
	// that reports this answers zero for it and says nothing (#2766).
	if out, st := run(t, `printf '[%d]' 1e400`, set); strings.Contains(out, "overflow") || st != 0 {
		t.Errorf("at 1e400: got %q status %d, want silence at 0", out, st)
	}
	// And a dialect without the wording says nothing at all.
	plain := printfSem()
	plain.PrintfNumberOperand = PrintfNumberArithmetic
	if out, st := run(t, `printf '[%d]' 99999999999999999999`, func(r *Runner) { r.Semantics = &plain }); strings.Contains(out, "overflow") || st != 0 {
		t.Errorf("without the wording: got %q status %d, want silence at 0", out, st)
	}
}
