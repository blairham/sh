// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// traceOf runs a script and returns only what went to stderr, because the
// whole question about `set -x` is which stream it uses.
func traceOf(t *testing.T, src string, sem Semantics, diag Diagnostics) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errOut bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &errOut, Semantics: &sem, Diagnostics: &diag, Name: "sh", Env: testPATH()})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errOut.String()
}

// TestXtraceStructureIsUnanimous: every simple command is written to stderr
// before it runs, with its words already expanded, and a `while`, `until` or
// `if` header is not traced. The decoration axes do not move any of that,
// which is what this asserts by holding the structure still while the quoting
// answer varies.
//
// "Compound commands are not traced" is what this used to say, and it was
// wrong — see TestTraceConditionIsATwoWayAnswer and its neighbors, and #2126.
func TestXtraceStructureIsUnanimous(t *testing.T) {
	for _, q := range []TraceQuoting{QuoteNever, QuoteShell, QuoteDollar, QuoteSingleOnly} {
		diag := Diagnostics{TraceQuoting: q}
		sem := permissive()
		// Each simple command, expanded, before it runs — and nothing on
		// stdout.
		if got := traceOf(t, `set -x; echo a b`, sem, diag); got != "+ echo a b\n" {
			t.Errorf("quoting %v: got %q", q, got)
		}
		// A branch or loop header is not traced; the commands inside it are.
		if got := traceOf(t, `set -x; if true; then echo y; fi`, sem, diag); got != "+ true\n+ echo y\n" {
			t.Errorf("quoting %v: compound: got %q", q, got)
		}
		// Off by default, and `set +x` stops it.
		if got := traceOf(t, `echo a`, sem, diag); got != "" {
			t.Errorf("quoting %v: off by default: got %q", q, got)
		}
	}
}

// TestTraceQuotingIsAFourWayAnswer names the TraceQuoting values rather than
// the shells that picked them; the presets' picks are asserted in dialect/.
// The fourth spelling has a test of its own below, because it parts from the
// other two quoting values in three separate places rather than one.
func TestTraceQuotingIsAFourWayAnswer(t *testing.T) {
	const src = `set -x; x="hello wor"; echo "$x"`
	sem := permissive()
	// QuoteNever prints an expanded field with a space unquoted, so one
	// argument and two are indistinguishable in its trace.
	if got := traceOf(t, src, sem, Diagnostics{TraceQuoting: QuoteNever}); !strings.Contains(got, "+ echo hello wor\n") {
		t.Errorf("QuoteNever: got %q", got)
	}
	for _, q := range []TraceQuoting{QuoteShell, QuoteDollar, QuoteSingleOnly} {
		if got := traceOf(t, src, sem, Diagnostics{TraceQuoting: q}); !strings.Contains(got, `+ echo 'hello wor'`) {
			t.Errorf("%v: got %q", q, got)
		}
	}
	// An embedded quote is where the three quoting styles part: one closes,
	// escapes and reopens, one reaches for $'…', and one closes and puts the
	// quote inside double quotes.
	const q = `set -x; x="it's"; echo "$x"`
	if got := traceOf(t, q, sem, Diagnostics{TraceQuoting: QuoteShell}); !strings.Contains(got, `'it'\''s'`) {
		t.Errorf("QuoteShell: got %q", got)
	}
	if got := traceOf(t, q, sem, Diagnostics{TraceQuoting: QuoteDollar}); !strings.Contains(got, `$'it\'s'`) {
		t.Errorf("QuoteDollar: got %q", got)
	}
}

func TestTraceStyleIsADialectAnswer(t *testing.T) {
	// TraceNameLine makes the prefix a *location*, which is the same thing
	// the diagnostics are and is read by the same code — so what stands in
	// it is the other location fields' answer and not a second rule. With
	// none of them, every line is the shell's name and the line.
	sem := permissive()
	got := traceOf(t, `set -x; f() { echo in; }; f`, sem, Diagnostics{TraceStyle: TraceNameLine})
	if !strings.Contains(got, "+sh:1> f\n") || !strings.Contains(got, "+sh:1> echo in\n") {
		t.Errorf("TraceNameLine: got %q", got)
	}
	// And with LocationNamesTheFunction, the function and the offset within
	// it — nought here, because the body is on the line the function was
	// written on. The trace writes that nought where a diagnostic leaves it
	// out, which is the one place the two part company (#2134).
	got = traceOf(t, `set -x; f() { echo in; }; f`, sem,
		Diagnostics{TraceStyle: TraceNameLine, LocationNamesTheFunction: true})
	if !strings.Contains(got, "+sh:1> f\n") || !strings.Contains(got, "+f:0> echo in\n") {
		t.Errorf("TraceNameLine with the function named: got %q", got)
	}
	// Two lines apart, so the offset is one rather than nought and a prefix
	// that wrote a literal nought could not pass.
	got = traceOf(t, "set -x\nf() {\n echo in\n}\nf\n", sem,
		Diagnostics{TraceStyle: TraceNameLine, LocationNamesTheFunction: true})
	if !strings.Contains(got, "+f:1> echo in\n") {
		t.Errorf("TraceNameLine over a body of its own: got %q", got)
	}
	if got := traceOf(t, `set -x; echo a`, sem, Diagnostics{TraceStyle: TracePlain}); got != "+ echo a\n" {
		t.Errorf("TracePlain: got %q", got)
	}
}

// TestTraceAssignmentsSeparately pins the axis by name: Yes gives each
// assignment of `a=1 b=2` its own line, No traces them as one.
//
// It decides *when* the line is written as well as how many it holds, and the
// second half is only visible when a value takes a command of its own to
// produce: a line that stands for one assignment can be written as soon as that
// value is known, and a line that stands for the whole list cannot be written
// until the last one is.
func TestTraceAssignmentsSeparately(t *testing.T) {
	sem := permissive()
	sem.TraceAssignmentsSeparately = Yes
	if got := traceOf(t, `set -x; a=1 b=2`, sem, Diagnostics{}); got != "+ a=1\n+ b=2\n" {
		t.Errorf("Yes: got %q", got)
	}
	if got := traceOf(t, `set -x; a=$(echo x) b=$(echo y)`, sem, Diagnostics{}); got != "+ echo x\n+ a=x\n+ echo y\n+ b=y\n" {
		t.Errorf("Yes: order: got %q", got)
	}
	sem.TraceAssignmentsSeparately = No
	if got := traceOf(t, `set -x; a=1 b=2`, sem, Diagnostics{}); got != "+ a=1 b=2\n" {
		t.Errorf("No: got %q", got)
	}
	if got := traceOf(t, `set -x; a=$(echo x) b=$(echo y)`, sem, Diagnostics{}); got != "+ echo x\n+ echo y\n+ a=x b=y\n" {
		t.Errorf("No: order: got %q", got)
	}
}

// TestTraceReportsTheValueStored: an assignment lands before the next value is
// expanded, so the trace of `y=$x` reports what `x` was just set to. Both
// answers to the per-line axis, because the value is not the axis's to move.
//
// Expanding the whole list up front to print it reported `y=”` while storing
// `1`, which is a trace that contradicts the run it is describing.
func TestTraceReportsTheValueStored(t *testing.T) {
	for _, tc := range []struct {
		separately Answer
		want       string
	}{
		{Yes, "+ x=1\n+ y=1\n"},
		{No, "+ x=1 y=1\n"},
	} {
		sem := permissive()
		sem.TraceAssignmentsSeparately = tc.separately
		if got := traceOf(t, `set -x; x=1 y=$x`, sem, Diagnostics{}); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.separately, got, tc.want)
		}
	}
}

// TestTraceDoesNotRunTheValueTwice: tracing an assignment observes it and does
// not run it again.
//
// The trace prints the value, and printing it by expanding the right-hand side
// a second time ran the command substitution there twice, with both sets of
// side effects — a traced `x=$(mktemp)` left a file behind and `n=$(curl …)`
// made two requests (#1915).
//
// It counts a side effect rather than comparing the value, which is the only
// probe that can tell the two apart: running `$(printf .)` twice leaves the
// value right and the file twice as long, so a test that read `$x` passed with
// the bug fully intact.
func TestTraceDoesNotRunTheValueTwice(t *testing.T) {
	for _, separately := range []Answer{Yes, No} {
		path := filepath.Join(t.TempDir(), "ran")
		sem := permissive()
		sem.TraceAssignmentsSeparately = separately
		traceOf(t, `set -x; x=$(printf . >> `+path+`)`, sem, Diagnostics{})
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v: %v", separately, err)
		}
		if string(got) != "." {
			t.Errorf("%v: the substitution ran %d times, want 1", separately, len(got))
		}
	}
}

// TestTraceShowsItsOwnDisabling pins the axis by name: No applies `set +x`
// before printing it, so it leaves no trace of itself; Yes prints it and then
// stops.
func TestTraceShowsItsOwnDisabling(t *testing.T) {
	sem := permissive()
	sem.TraceShowsItsOwnDisabling = No
	if got := traceOf(t, `set -x; set +x; echo done`, sem, Diagnostics{}); got != "" {
		t.Errorf("No: got %q", got)
	}
	sem.TraceShowsItsOwnDisabling = Yes
	if got := traceOf(t, `set -x; set +x; echo done`, sem, Diagnostics{}); got != "+ set +x\n" {
		t.Errorf("Yes: got %q", got)
	}
}

// TestXtracePipelineOrderIsDeterministic guards the gate chain. Pipeline
// elements run at the same time, so without one their trace lines come out in
// whatever order the scheduler chose.
func TestXtracePipelineOrderIsDeterministic(t *testing.T) {
	sem := permissive()
	for i := 0; i < 40; i++ {
		got := traceOf(t, `set -x; echo a | cat`, sem, Diagnostics{})
		if got != "+ echo a\n+ cat\n" {
			t.Fatalf("run %d: got %q", i, got)
		}
	}
}

// TestTraceForHeaderHasThreeAnswers records what a `for` loop prints when it
// takes another turn. It is not a bool: one answer prints nothing, one
// reprints the header, and one prints something the header does not say.
func TestTraceForHeaderHasThreeAnswers(t *testing.T) {
	const src = `set -x; for i in 1 2; do echo $i; done`
	sem := permissive()
	// TraceForSource reprints the header once per iteration, as written.
	want := "+ for i in 1 2\n+ echo 1\n+ for i in 1 2\n+ echo 2\n"
	if got := traceOf(t, src, sem, Diagnostics{TraceForHeader: TraceForSource}); got != want {
		t.Errorf("TraceForSource: got %q, want %q", got, want)
	}
	// TraceForNone prints only the commands inside.
	want = "+ echo 1\n+ echo 2\n"
	if got := traceOf(t, src, sem, Diagnostics{TraceForHeader: TraceForNone}); got != want {
		t.Errorf("TraceForNone: got %q, want %q", got, want)
	}
	// TraceForAssign prints neither, and shows the assignment instead.
	// Without the trailing space its dialect puts on an assignment that
	// stands alone as a command — measured, and the reason this does not
	// reuse traceLine.
	got := traceOf(t, src, sem, Diagnostics{TraceForHeader: TraceForAssign})
	if !strings.Contains(got, "+ i=1\n") || strings.Contains(got, "for i in") {
		t.Errorf("TraceForAssign: got %q, want an assignment and no header", got)
	}
}

// TestXtraceForHeaderIsUnexpanded is the reason the header is kept as source
// rather than rebuilt from the tree: TraceForSource prints what was written,
// so a rebuilt line would lose the `$` and the quotes.
func TestXtraceForHeaderIsUnexpanded(t *testing.T) {
	sem := permissive()
	diag := Diagnostics{TraceForHeader: TraceForSource}
	got := traceOf(t, `x="a b"; set -x; for i in $x; do :; done`, sem, diag)
	if !strings.Contains(got, "+ for i in $x\n") {
		t.Errorf("got %q, want it to contain %q", got, "+ for i in $x\n")
	}
	got = traceOf(t, `set -x; for i in "a b"; do :; done`, sem, diag)
	if !strings.Contains(got, `+ for i in "a b"`+"\n") {
		t.Errorf("got %q, want the quotes kept", got)
	}
}

// TestTraceOfAnAssignmentKeepsItsOwnSpelling: the target comes from the tree
// and the value from the expansion, so an element write keeps its subscript,
// an append keeps its operator and an array literal keeps its elements.
//
// The unanimous half of #1937 — every shell that has these constructs agrees
// on all three — where the trace used to be built from the name and one
// scalar value and answered `a=”`, `a=z` and `x=b`.
func TestTraceOfAnAssignmentKeepsItsOwnSpelling(t *testing.T) {
	sem := permissive()
	for _, c := range []struct{ src, want string }{
		// The elements, rather than the empty string a nil value renders to.
		{`set -x; a=(1 2)`, "+ a=(1 2)\n"},
		// An empty literal is not a bare `name=`, and does not print as one.
		{`set -x; b=()`, "+ b=()\n"},
		// The subscript, so an element write cannot be read as one that
		// replaced the whole name.
		{`set -x; a=(x y); a[1]=z`, "+ a=(x y)\n+ a[1]=z\n"},
		// The operator, so an append cannot be read as a replacement.
		{`set -x; x=1; x+=b`, "+ x=1\n+ x+=b\n"},
		{`set -x; a=(1); a+=(2)`, "+ a=(1)\n+ a+=(2)\n"},
	} {
		if got := traceOf(t, c.src, sem, Diagnostics{}); got != c.want {
			t.Errorf("%s: got %q, want %q", c.src, got, c.want)
		}
	}
}

// TestTraceOfASubscriptIsWhatWasWritten: the subscript is printed unexpanded,
// which is what keeps the trace from evaluating it a second time — `a[$((i++))]=v`
// increments once, and the trace says what the script said.
func TestTraceOfASubscriptIsWhatWasWritten(t *testing.T) {
	sem := permissive()
	got := traceOf(t, `set -x; i=2; a[$i]=v; echo $i`, sem, Diagnostics{})
	if want := "+ i=2\n+ a[$i]=v\n+ echo 2\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	got = traceOf(t, `set -x; i=0; a[$((i++))]=v; echo $i`, sem, Diagnostics{})
	if want := "+ i=0\n+ a[$((i++))]=v\n+ echo 1\n"; got != want {
		t.Errorf("side effect: got %q, want %q", got, want)
	}
}

// TestTraceArrayLiteralSpacingIsADialectAnswer names the TraceArrayLiteral
// values rather than the shells that picked them; the presets' picks are
// asserted in dialect/.
func TestTraceArrayLiteralSpacingIsADialectAnswer(t *testing.T) {
	sem := permissive()
	for _, c := range []struct {
		style      TraceArrayLiteral
		full, none string
	}{
		{TraceArrayTight, "+ a=(1 2)\n", "+ a=()\n"},
		{TraceArraySpaced, "+ a=( 1 2 )\n", "+ a=( )\n"},
	} {
		diag := Diagnostics{TraceArrayLiteral: c.style}
		if got := traceOf(t, `set -x; a=(1 2)`, sem, diag); got != c.full {
			t.Errorf("%v: got %q, want %q", c.style, got, c.full)
		}
		// An empty list has one pair of parentheses either way, so the
		// spaced answer cannot double its space.
		if got := traceOf(t, `set -x; a=()`, sem, diag); got != c.none {
			t.Errorf("%v: empty: got %q, want %q", c.style, got, c.none)
		}
	}
}

// TestTraceConditionIsATwoWayAnswer names TraceCondition rather than the
// shells that picked its values.
//
// The construct is traced at all, which is the whole of #2126: it was traced
// by neither reading, so a script's every `[[ ]]` guard was invisible and a
// gap in the log read as a line that had not run.
func TestTraceConditionIsATwoWayAnswer(t *testing.T) {
	sem := permissive()
	const src = `set -x; [[ -n a && -n b && -n c ]]`
	// A line per primary, as it is evaluated.
	want := "+ [[ -n a ]]\n+ [[ -n b ]]\n+ [[ -n c ]]\n"
	if got := traceOf(t, src, sem, Diagnostics{TraceCondition: TraceCondPrimary}); got != want {
		t.Errorf("TraceCondPrimary: got %q, want %q", got, want)
	}
	// One line for the condition, written once it has finished.
	want = "+ [[ -n a && -n b && -n c ]]\n"
	if got := traceOf(t, src, sem, Diagnostics{TraceCondition: TraceCondWhole}); got != want {
		t.Errorf("TraceCondWhole: got %q, want %q", got, want)
	}
	// Both print only what was evaluated. A short-circuited operand appears
	// under neither reading, which is what makes the line evidence about the
	// run rather than an echo of the source.
	for _, c := range []TraceCondition{TraceCondPrimary, TraceCondWhole} {
		got := traceOf(t, `set -x; [[ -n a || -n b ]]`, sem, Diagnostics{TraceCondition: c})
		if got != "+ [[ -n a ]]\n" {
			t.Errorf("%v: short circuit: got %q", c, got)
		}
	}
	// A `( )` group is dropped and a `!` prints with the primary it negates.
	for _, c := range []TraceCondition{TraceCondPrimary, TraceCondWhole} {
		got := traceOf(t, `set -x; [[ ( ! -z a ) ]]`, sem, Diagnostics{TraceCondition: c})
		if got != "+ [[ ! -z a ]]\n" {
			t.Errorf("%v: group and negation: got %q", c, got)
		}
	}
}

// TestTraceConditionQuotingIsNotTheCommandQuoting: the two are separate
// fields because one preset answers them differently.
func TestTraceConditionQuotingIsNotTheCommandQuoting(t *testing.T) {
	sem := permissive()
	const src = `set -x; x="a b"; [[ $x == "a b" ]]`
	got := traceOf(t, src, sem, Diagnostics{TraceQuoting: QuoteShell, TraceConditionQuoting: QuoteNever})
	if !strings.Contains(got, "+ [[ a b == a b ]]\n") {
		t.Errorf("QuoteNever operands: got %q", got)
	}
	// The command trace in the same run still quotes, which is what says the
	// two answers are independent rather than one field read twice.
	if !strings.Contains(got, "+ x='a b'\n") {
		t.Errorf("command quoting unchanged: got %q", got)
	}
	got = traceOf(t, src, sem, Diagnostics{TraceQuoting: QuoteShell, TraceConditionQuoting: QuoteShell})
	if !strings.Contains(got, `+ [[ 'a b' == a b ]]`) {
		t.Errorf("QuoteShell operands: got %q", got)
	}
	// An empty operand is quoted under every answer, including the two that
	// render an empty *word* as nothing at all: `[[ -z ]]` is a condition no
	// shell would accept.
	for _, q := range []TraceQuoting{QuoteNever, QuoteShell, QuoteDollar, QuoteSingleOnly} {
		got := traceOf(t, `set -x; [[ -z "" ]]`, sem, Diagnostics{TraceConditionQuoting: q})
		if got != "+ [[ -z '' ]]\n" {
			t.Errorf("%v: empty operand: got %q", q, got)
		}
	}
}

// TestTraceConditionPatternKeepsItsEscapes: the right operand of `==` is
// printed from the string the matcher was handed, so the line says which
// characters were live.
func TestTraceConditionPatternKeepsItsEscapes(t *testing.T) {
	sem := permissive()
	diag := Diagnostics{}
	// Written out, the metacharacter is a pattern and is printed bare.
	if got := traceOf(t, `set -x; [[ abc == a* ]]`, sem, diag); got != "+ [[ abc == a* ]]\n" {
		t.Errorf("live pattern: got %q", got)
	}
	// Quoted, it is a literal and carries a backslash — the same two
	// characters, a different rendering, and the only place a reader of the
	// line can learn the difference.
	if got := traceOf(t, `set -x; [[ abc == "a*" ]]`, sem, diag); got != `+ [[ abc == a\* ]]`+"\n" {
		t.Errorf("literal pattern: got %q", got)
	}
}

// TestTraceArithSpellingIsAThreeWayAnswer names TraceArithCommand and
// TraceArithForPart, which are separate fields because one preset wraps a
// `(( ))` command and writes a loop header's parts bare.
func TestTraceArithSpellingIsAThreeWayAnswer(t *testing.T) {
	sem := permissive()
	const src = `set -x; n=1; (( n + 1 ))`
	for _, tc := range []struct {
		spelling TraceArithSpelling
		want     string
	}{
		{TraceArithSpaced, "+ ((  n + 1  ))\n"},
		{TraceArithTight, "+ (( n + 1 ))\n"},
		{TraceArithBare, "+  n + 1 \n"},
	} {
		got := traceOf(t, src, sem, Diagnostics{TraceArithCommand: tc.spelling})
		if !strings.HasSuffix(got, tc.want) {
			t.Errorf("%v: got %q, want suffix %q", tc.spelling, got, tc.want)
		}
	}
	// The text is the expression the evaluator received — expanded — which is
	// what makes printing it free rather than a second run of whatever is in
	// it.
	if got := traceOf(t, `set -x; n=3; (( $n + 1 ))`, sem, Diagnostics{}); !strings.HasSuffix(got, "+ ((  3 + 1  ))\n") {
		t.Errorf("expanded text: got %q", got)
	}
	// The three parts of a C-style header are traced as arithmetic in their
	// own right, in their own spelling, and the loop itself prints nothing
	// per pass — it binds no name, so the list loop's iteration line has
	// nothing to say about it.
	got := traceOf(t, `set -x; for ((i=0;i<1;i++)); do :; done`, sem,
		Diagnostics{TraceArithForPart: TraceArithBare, TraceForHeader: TraceForAssign})
	if got != "+ i=0\n+ i<1\n+ :\n+ i++\n+ i<1\n" {
		t.Errorf("for-arith parts: got %q", got)
	}
}

// TestTraceCaseHeaderIsAThreeWayAnswer names TraceCaseHeader rather than the
// shells that picked its values.
func TestTraceCaseHeaderIsAThreeWayAnswer(t *testing.T) {
	sem := permissive()
	const src = `set -x; x=abc; case $x in ab|abc) : ;; esac`
	// Nothing at all: only the commands of the arm that ran.
	if got := traceOf(t, src, sem, Diagnostics{TraceCaseHeader: TraceCaseNone}); !strings.HasSuffix(got, "+ :\n") ||
		strings.Contains(got, "case") {
		t.Errorf("TraceCaseNone: got %q", got)
	}
	// The header as written, once.
	if got := traceOf(t, src, sem, Diagnostics{TraceCaseHeader: TraceCaseSource}); !strings.Contains(got, "+ case $x in\n") {
		t.Errorf("TraceCaseSource: got %q", got)
	}
	// The expanded subject and the arm's patterns, once per arm tried, and
	// the patterns are those the match reached: `ab` did not match, so both
	// are on the line.
	if got := traceOf(t, src, sem, Diagnostics{TraceCaseHeader: TraceCaseArm}); !strings.Contains(got, "+ case abc (ab | abc)\n") {
		t.Errorf("TraceCaseArm: got %q", got)
	}
	// One line per arm *tried*, stopping at the match — which is what makes
	// the line count say how far down the arms the subject got.
	got := traceOf(t, `set -x; case c in a) : ;; b) : ;; c) : ;; d) : ;; esac`, sem,
		Diagnostics{TraceCaseHeader: TraceCaseArm})
	if got != "+ case c (a)\n+ case c (b)\n+ case c (c)\n+ :\n" {
		t.Errorf("arms tried: got %q", got)
	}
}

// TestTraceSkipsWhatNoShellPrints pins the narrower rule that replaced
// "compound commands are not traced": the headers no shell in the panel
// writes down.
//
// A row of its own because "nobody traces it" and "we do not trace it" are
// indistinguishable without one, and the second was the defect (#2126).
func TestTraceSkipsWhatNoShellPrints(t *testing.T) {
	sem := permissive()
	// Every decoration answer at once, so the assertion is about the
	// construct rather than about a preset that happens to be quiet.
	diag := Diagnostics{
		TraceCaseHeader:   TraceCaseArm,
		TraceCondition:    TraceCondWhole,
		TraceForHeader:    TraceForSource,
		TraceArithCommand: TraceArithSpaced,
	}
	got := traceOf(t, `set -x; i=0; while [ $i -lt 1 ]; do i=1; done; until [ $i -gt 0 ]; do :; done; if true; then :; fi; ( : ); { :; }`, sem, diag)
	for _, word := range []string{"while", "until", "if", "then", "(", "{"} {
		if strings.Contains(got, "+ "+word) {
			t.Errorf("traced a header no shell writes (%q): got %q", word, got)
		}
	}
}

// TestQuoteSingleOnlyIsTheFourthSpelling names the value rather than the shell
// that picked it; dialect/ash asserts the pick. Every expectation is a cell of
// the committed record — see `xtrace/embedded-quote-diverges`,
// `xtrace/a-control-character-diverges` and `xtrace/an-empty-field-diverges`.
//
// The three assertions are three separate measurements and not one restated:
// a shell could smuggle a quote through double quotes and still keep a `$'…'`
// for a control character, and either of those could still write `”` for an
// empty field. The panel's other quoting shells do all three the other way.
func TestQuoteSingleOnlyIsTheFourthSpelling(t *testing.T) {
	sem := permissive()
	diag := Diagnostics{TraceQuoting: QuoteSingleOnly}
	// The run of quotes goes inside double quotes, where QuoteShell
	// backslash-escapes one and QuoteDollar abandons single quotes entirely.
	if got := traceOf(t, `set -x; x="it's"; echo "$x"`, sem, diag); !strings.Contains(got, `+ echo 'it'"'"'s'`+"\n") {
		t.Errorf("embedded quote: got %q", got)
	}
	// No `$'…'` anywhere, so a control character is written as the byte it
	// is, inside plain single quotes.
	if got := traceOf(t, "set -x; x=\"$(printf 'a\\tb')\"; echo \"$x\"", sem, diag); !strings.Contains(got, "+ echo 'a\tb'\n") {
		t.Errorf("control character: got %q", got)
	}
	// And an empty field leaves no mark at all, where the other three write
	// `''`. The trailing `a` is what says the field is still counted.
	if got := traceOf(t, `set -x; echo '' a`, sem, diag); !strings.Contains(got, "+ echo  a\n") {
		t.Errorf("empty field: got %q", got)
	}
}

// TestQuoteSingleOnlyReopensLazily is the half a fixed replacement string
// cannot express, and the reason traceSingleQuote walks runs rather than
// calling strings.ReplaceAll. Measured 2026-09-13 on BusyBox v1.37.0.
func TestQuoteSingleOnlyReopensLazily(t *testing.T) {
	sem := permissive()
	diag := Diagnostics{TraceQuoting: QuoteSingleOnly}
	for _, tc := range []struct{ word, want string }{
		// Two quotes in a row share one pair of double quotes, so a
		// per-character replacement would write `'"'"'"'"'` here.
		{`a''b`, `'a'"''"'b'`},
		// Nothing reopens after a trailing run.
		{`ab'`, `'ab'"'"`},
		// But the first segment is opened even when it is empty.
		{`'ab`, `''"'"'ab'`},
		// Both of those at once, for a word that is one quote.
		{`'`, `''"'"`},
		// And the ordinary interior case, which either rule gets right.
		{`a'b'c`, `'a'"'"'b'"'"'c'`},
	} {
		// None of the words holds a `"`, a `$`, a backquote or a
		// backslash, so double quotes carry each one through unchanged.
		src := `set -x; x="` + tc.word + `"; echo "$x"`
		if got := traceOf(t, src, sem, diag); !strings.Contains(got, "+ echo "+tc.want+"\n") {
			t.Errorf("%q: got %q, want it traced as %s", tc.word, got, tc.want)
		}
	}
}

// The two shells that spell an embedded quote `'\”` do not agree about the
// empty segments closing and reopening leaves, and one shape hides it.
//
// `it's` — a quote in the middle of a word — is spelled identically by both,
// and it was the only shape the corpus had, so the two shared one value until
// #2695. They part wherever a quote touches an **end**: the eager reading
// writes the empty `”` pairs and the lazy one drops them, at the front, at
// the end, and between two adjacent quotes.
//
// Measured 2026-09-13, bash 5.3.15 and zsh 5.9.2, `env -i PATH=/usr/bin:/bin`
// over a script file. Values are named here rather than shells, as everywhere
// under interp.
func TestTheTwoQuotingReadingsPartAtTheEndsOfAWord(t *testing.T) {
	for _, tc := range []struct{ name, word, eager, lazy string }{
		// The shape that hid the split, and the reason a row using it
		// could not have found this.
		{"a quote in the middle", "a'b", `'a'\''b'`, `'a'\''b'`},
		{"a quote at the end", "ab'", `'ab'\'''`, `'ab'\'`},
		{"a quote at the front", "'ab", `''\''ab'`, `\''ab'`},
		{"two adjacent quotes", "a''b", `'a'\'''\''b'`, `'a'\'\''b'`},
		// A word that is nothing but quotes, where the eager reading is
		// all empty pairs. The lone-quote case is deliberately not here:
		// it is a special case in one shell rather than this rule, and it
		// is pinned by TestALoneQuoteIsASpecialCaseAndNotTheLazyRule.
		{"two quotes alone", "''", `''\'''\'''`, `\'\'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, r := range []struct {
				name string
				q    TraceQuoting
				want string
			}{
				{"eager", QuoteShell, tc.eager},
				{"lazy", QuoteShellLazy, tc.lazy},
			} {
				src := "set -x; x=" + shellSingleQuoted(tc.word) + `; echo "$x"`
				got := tracedEcho(t, traceOf(t, src, permissive(), Diagnostics{TraceQuoting: r.q}))
				if got != r.want {
					t.Errorf("%q under %s = %s, want %s", tc.word, r.name, got, r.want)
				}
			}
		})
	}
}

// A lone quote is `\'` under the eager reading too, and that is a special case
// rather than the lazy rule leaking in: the word `”` keeps its empty pairs
// there. Measured 2026-09-13 — bash 5.3.15 traces `'` as `\'` and `”` as
// `”\”'\”'`, while bash 3.2.57 writes `”\”'` for the first, so the newer
// shell grew the case and the general rule is still visible in the older
// column (#2695).
func TestALoneQuoteIsASpecialCaseAndNotTheLazyRule(t *testing.T) {
	for _, r := range []struct {
		name string
		q    TraceQuoting
		one  string
		two  string
	}{
		{"eager", QuoteShell, `\'`, `''\'''\'''`},
		{"lazy", QuoteShellLazy, `\'`, `\'\'`},
	} {
		t.Run(r.name, func(t *testing.T) {
			for _, tc := range []struct{ word, want string }{
				{"'", r.one},
				{"''", r.two},
			} {
				src := "set -x; x=" + shellSingleQuoted(tc.word) + `; echo "$x"`
				got := tracedEcho(t, traceOf(t, src, permissive(), Diagnostics{TraceQuoting: r.q}))
				if got != tc.want {
					t.Errorf("%q under %s = %s, want %s", tc.word, r.name, got, tc.want)
				}
			}
		})
	}
}

// tracedEcho is the `echo` line of a trace that also traced the assignment
// setting the word up. Taking the whole output would compare two lines and
// report a mismatch whose text is the answer wanted, which is a confusing way
// to be told the extraction is wrong.
func tracedEcho(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if after, ok := strings.CutPrefix(line, "+ echo "); ok {
			return strings.TrimSpace(after)
		}
	}
	t.Fatalf("no `+ echo` line in %q", out)
	return ""
}
