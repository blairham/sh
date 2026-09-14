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
