// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// The walker's own rules for the conditional escape, asked of a table that is
// nobody's dialect.
//
// A shell's answers live in its own dialect package, where they were
// measured. What is here is what the *substrate* promises whatever dialect
// fills the table in:
// that a style naming no conditional never reads one, that a letter the table
// has no entry for is dropped rather than refused, that a letter it does have
// and the reader cannot answer is refused and named, and that the column the
// tests count is the one this walk has drawn so far.

// conditionalStyle is a table with a conditional and nothing borrowed: the
// escape is a letter rather than a percent and the construct is written with
// square brackets, so nothing here can pass by resembling a real shell.
func conditionalStyle() PromptStyle {
	return PromptStyle{
		Escape:          '@',
		NumericArgument: true,
		Conditional:     '[',
		ConditionalEnd:  ']',
		Codes:           map[rune]PromptField{'@': FieldEscape, 'G': FieldCountedColumn},
		Conditions: map[rune]PromptCondition{
			'q': ConditionJobs,
			'k': ConditionColumn,
			'z': ConditionEvalDepth,
		},
	}
}

// answerAll is a reader with an answer for everything the style asks.
func answerAll(width, jobs int) PromptQuantityResolver {
	return func(c PromptCondition, _ int) (int, bool) {
		switch c {
		case ConditionLineWidth:
			return width, true
		case ConditionJobs:
			return jobs, true
		}
		return 0, false
	}
}

func drawField(f PromptField, arg string, _ bool) (string, bool) {
	switch f {
	case FieldEscape:
		return "@", true
	case FieldNone:
		return arg, true
	}
	return "", true
}

func TestTheWalkerReadsTheConditional(t *testing.T) {
	st := conditionalStyle()
	for _, tc := range []struct{ text, want string }{
		// The arms, and the delimiter the writer chose.
		{"@[q.yes.no]", "yes"},
		{"@1[q.yes.no]", "no"},
		{"@[q/yes/no]", "yes"},
		// The true arm ends at the delimiter, the false arm at the closer.
		{"@1[q.yes.no.tail]", "no.tail"},
		{"@[q.yes.no.tail]", "yes"},
		{"@[q.yes.no]after", "yesafter"},
		// A letter with no entry draws nothing and swallows the construct,
		// which is the table saying there is no such question.
		{"@[x.yes.no]after", "after"},
		// Nesting, where the inner construct's delimiter and closer belong to
		// it and not to the arm around it.
		{"@[q.a@1[q.x.y]b.c]", "ayb"},
		{"@[q.a@1[q.x].y]b.c]", "ayb"},
		// The closer written after the escape is itself.
		{"a@]b", "a]b"},
	} {
		out, refused, ok := ExpandPromptStyle(st, tc.text, drawField, answerAll(80, 0))
		if !ok || out != tc.want {
			t.Errorf("%q = %q (refused %q, ok %v), want %q", tc.text, out, refused, ok, tc.want)
		}
	}
}

// A letter the table has and the reader cannot answer is refused, and named
// with the character that opened the construct in front of it — which is what
// tells a reader looking at the diagnostic that `z` is a test and not an
// escape of its own.
func TestATestTheReaderCannotAnswerIsRefusedAndNamed(t *testing.T) {
	st := conditionalStyle()
	out, refused, ok := ExpandPromptStyle(st, "ab@[z.yes.no]cd", drawField, answerAll(80, 0))
	if ok || refused != "[z" {
		t.Errorf("got %q (refused %q, ok %v), want a refusal naming %q", out, refused, ok, "[z")
	}
	if out != "ab" {
		// What had been drawn when the walk stopped, which is what lets a
		// caller say where in the text it happened.
		t.Errorf("drew %q before the refusal, want %q", out, "ab")
	}
}

// A table with no conditional never reads one, and never asks the reader for
// a width either — the whole construct is a code the table does not know.
func TestATableWithNoConditionalNeverReadsOne(t *testing.T) {
	st := conditionalStyle()
	st.Conditional, st.ConditionalEnd, st.Conditions = 0, 0, nil
	asked := false
	quantity := func(PromptCondition, int) (int, bool) {
		asked = true
		return 0, true
	}
	out, _, ok := ExpandPromptStyle(st, "a@[q.yes.no]b", drawField, quantity)
	if !ok || out != "a[q.yes.no]b" {
		t.Errorf("got %q (ok %v), want the code handed to the resolver as unknown", out, ok)
	}
	if asked {
		t.Error("asked the reader a question about a construct this table has not got")
	}
}

// The column the tests count is the one this walk has drawn, and what counts
// is not the same as what is written: a code that draws nothing can occupy a
// column and text between the non-printing markers occupies none.
func TestTheColumnCountsWhatReachesTheScreen(t *testing.T) {
	st := conditionalStyle()
	st.Codes['s'] = FieldNonPrintingStart
	st.Codes['e'] = FieldNonPrintingEnd
	for _, tc := range []struct {
		text string
		want string
	}{
		{"ab@2[k.T.F]", "abT"},
		{"ab@3[k.T.F]", "abF"},
		// The escape's own drawn character counts.
		{"@@@1[k.T.F]", "@T"},
		// A code that draws nothing and occupies a column.
		{"@G@1[k.T.F]", "T"},
		{"@G@2[k.T.F]", "F"},
		// And text that is drawn and occupies none.
		{"@sxy@e@[k.T.F]", "xyT"},
		{"@sxy@e@1[k.T.F]", "xyF"},
		{"@sxy@ez@1[k.T.F]", "xyzT"},
		{"@sxy@ez@2[k.T.F]", "xyzF"},
		// A conditional's own output is counted by the next one.
		{"a@1[k.xy.no]@3[k.T.F]", "axyT"},
	} {
		out, refused, ok := ExpandPromptStyle(st, tc.text, drawField, answerAll(80, 0))
		if !ok || out != tc.want {
			t.Errorf("%q = %q (refused %q, ok %v), want %q", tc.text, out, refused, ok, tc.want)
		}
	}
}

// The wrap, which is two wraps: one before a character that will not fit and
// one after a line filled exactly. Each case here is got wrong by the other
// reading on its own.
func TestTheColumnWrapsBeforeAndAfter(t *testing.T) {
	st := conditionalStyle()
	for _, tc := range []struct {
		width int
		text  string
		want  string
	}{
		// Filled exactly: the line wraps after, so the column is nought.
		{5, "xxxxx@[k.T.F]", "xxxxxT"},
		{5, "xxxxx@1[k.T.F]", "xxxxxF"},
		// Will not fit: the wide character starts a new line before it is
		// drawn, so two columns are left rather than none.
		{5, "日日日@2[k.T.F]", "日日日T"},
		{5, "日日日@3[k.T.F]", "日日日F"},
		// Wider than the whole line, which is neither of those: the second
		// wrap is skipped and the column is the character's own width.
		{1, "日@2[k.T.F]", "日T"},
		{1, "日@3[k.T.F]", "日F"},
		// A width of nought never fits anything, so what is left is the last
		// character alone.
		{0, "xxxxx@1[k.T.F]", "xxxxxT"},
		{0, "xxxxx@2[k.T.F]", "xxxxxF"},
		// A negative count asks about the space left rather than the space
		// used.
		{10, "xxx@-7[k.T.F]", "xxxT"},
		{10, "xxx@-8[k.T.F]", "xxxF"},
	} {
		out, _, ok := ExpandPromptStyle(st, tc.text, drawField, answerAll(tc.width, 0))
		if !ok || out != tc.want {
			t.Errorf("width %d, %q = %q (ok %v), want %q", tc.width, tc.text, out, ok, tc.want)
		}
	}
}

// The count's own grammar, read once for the walk and the skip alike.
func TestThePromptCount(t *testing.T) {
	for _, tc := range []struct {
		arg  string
		want int
	}{
		{"", 0},
		{"0", 0},
		{"12", 12},
		{"-", -1},
		{"-0", 0},
		{"-3", -3},
		// Too long to be a number is the largest one, which is false for
		// every test either way.
		{"99999999999999999999", 1<<63 - 1},
	} {
		if got := promptCount(tc.arg); got != tc.want {
			t.Errorf("promptCount(%q) = %d, want %d", tc.arg, got, tc.want)
		}
	}
}
