// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Two operators written with no blank between them (#2409).
//
// It is the second remark whose subject is *layout* rather than a construct:
// the text lexes and means exactly what it says, writing the two apart
// changes nothing but the silence, and half the shapes that draw it parse and
// run. One shell in the panel says something on the way past and the rest
// read it without a word, so these tests name the trigger and never a shell.

func spacingRemarks(t *testing.T, src string, d syntax.Dialect) []syntax.Remark {
	t.Helper()
	p := syntax.NewParser(src, d)
	p.Parse()
	var out []syntax.Remark
	for _, r := range p.Remarks() {
		if r.Kind == syntax.RemarkOperatorsNotSeparated {
			out = append(out, r)
		}
	}
	return out
}

// TestTwoOperatorsRunTogetherAreRemarkedOn, with both spellings carried:
// neither can be derived from the other, and the sentence names both.
func TestTwoOperatorsRunTogetherAreRemarkedOn(t *testing.T) {
	for _, c := range []struct{ src, first, second string }{
		{"(:);(:)\n", ";", "("},
		// Whether the grammar then *takes* this text is a separate
		// question — one dialect runs it and the rest refuse the `;` — and
		// the remark is recorded either way, which is the point.
		{"echo a&;b\n", "&", ";"},
		{"a|;b\n", "|", ";"},
		{"if |; then :; fi\n", "|", ";"},
		{"if ;| then :; fi\n", ";", "|"},
		{":;<f\n", ";", "<"},
		{":;>f\n", ";", ">"},
		{":&(:)\n", "&", "("},
		{":|(:)\n", "|", "("},
		{":;((1))\n", ";", "("},
	} {
		rs := spacingRemarks(t, c.src, syntax.Core())
		if len(rs) != 1 {
			t.Errorf("%q: got %d remarks, want 1: %+v", c.src, len(rs), rs)
			continue
		}
		if rs[0].Token != c.first || rs[0].Next != c.second {
			t.Errorf("%q: remarked on %q and %q, want %q and %q",
				c.src, rs[0].Token, rs[0].Next, c.first, c.second)
		}
		if rs[0].Pos.Line != 1 || rs[0].At.Line != 1 {
			t.Errorf("%q: lines %d and %d, want both 1", c.src, rs[0].Pos.Line, rs[0].At.Line)
		}
	}
}

// TestASecondOperatorIsNamedByItsFirstByte and not by the whole token, which
// is measured: `:;>>f` names `>` where the operator lexed after it is `>>`.
// A remark that carried the following *token* would name `>>` and read as a
// different sentence.
func TestASecondOperatorIsNamedByItsFirstByte(t *testing.T) {
	for _, c := range []struct{ src, second string }{
		{":;>>f\n", ">"},
		{":;<<E\n", "<"},
		{":;||:\n", "|"},
	} {
		rs := spacingRemarks(t, c.src, syntax.Core())
		if len(rs) != 1 {
			t.Fatalf("%q: got %d remarks, want 1: %+v", c.src, len(rs), rs)
		}
		if rs[0].Next != c.second {
			t.Errorf("%q: second operator %q, want %q", c.src, rs[0].Next, c.second)
		}
	}
}

// TestABlankBetweenThemSaysNothing is the control that says this is about
// layout: the same two operators, spaced, and the parse is otherwise
// identical — `a |; b` is refused exactly as `a|;b` is.
func TestABlankBetweenThemSaysNothing(t *testing.T) {
	for _, src := range []string{
		"(:) ; (:)\n", "(:);\t(:)\n", "(:);\n(:)\n", "echo a & ; b\n", "a | ; b\n",
	} {
		if rs := spacingRemarks(t, src, syntax.Core()); len(rs) != 0 {
			t.Errorf("%q: got %+v, want nothing said", src, rs)
		}
	}
}

// TestOnlyAnOperatorFollowsAnOperator: the byte after has to begin another
// one. A word, a reserved word, a brace, a closing parenthesis and a `!` are
// all silent, and so is `$(` — which is the row that says the trigger is the
// literal byte rather than "a parenthesis comes next".
func TestOnlyAnOperatorFollowsAnOperator(t *testing.T) {
	for _, src := range []string{
		":;$(:)\n", ":;x\n", ":;{ :; }\n", ":;!:\n", ":;[[ x ]]\n",
		"(:;)\n", ":;'a'\n", ":;\"a\"\n", ":;\\;\n",
	} {
		if rs := spacingRemarks(t, src, syntax.Core()); len(rs) != 0 {
			t.Errorf("%q: got %+v, want nothing said", src, rs)
		}
	}
}

// TestAnOperatorOfTwoBytesNeverStartsOne. Length is the whole of it: `&&(` is
// silent where `|(` is remarked on, and a pair that spells one operator has
// no blank to be missing.
func TestAnOperatorOfTwoBytesNeverStartsOne(t *testing.T) {
	for _, src := range []string{
		":&&(:)\n", ":||(:)\n", ":;;:\n", ":;&:\n", "case x in x) :;; esac\n",
	} {
		if rs := spacingRemarks(t, src, syntax.Core()); len(rs) != 0 {
			t.Errorf("%q: got %+v, want nothing said", src, rs)
		}
	}
}

// TestTheOperatorTableDecidesAndNotAListOfPairs is the discriminating one,
// and it is why the rule is asked of the lexer rather than written out.
//
// `;&` is one operator only where the grammar has it. Turn it off and the
// same two bytes are `;` followed by `&` — a one-byte operator with an
// operator byte after it — so the remark appears. A hardcoded exclusion list
// would be silent in both dialects and could not tell them apart.
func TestTheOperatorTableDecidesAndNotAListOfPairs(t *testing.T) {
	with := syntax.Core()
	with.CaseFallthrough = true
	if rs := spacingRemarks(t, ":;&:\n", with); len(rs) != 0 {
		t.Errorf("with `;&` in the table: got %+v, want nothing said", rs)
	}

	without := syntax.Core()
	without.CaseFallthrough = false
	rs := spacingRemarks(t, ":;&:\n", without)
	if len(rs) != 1 {
		t.Fatalf("without `;&` in the table: got %d remarks, want 1: %+v", len(rs), rs)
	}
	if rs[0].Token != ";" || rs[0].Next != "&" {
		t.Errorf("remarked on %q and %q, want %q and %q", rs[0].Token, rs[0].Next, ";", "&")
	}
}

// TestQuotingAndCommentsHideIt, because the trigger is lexical: text inside
// quotes or after a `#` is not operators at all.
func TestQuotingAndCommentsHideIt(t *testing.T) {
	for _, src := range []string{
		"#(:);(:)\n", "echo \"(:);(:)\"\n", "echo '(:);(:)'\n", "echo a\\&\\;b\n",
	} {
		if rs := spacingRemarks(t, src, syntax.Core()); len(rs) != 0 {
			t.Errorf("%q: got %+v, want nothing said", src, rs)
		}
	}
}

// TestOneRemarkPerOccurrenceInOrder, so a front end that renders them renders
// each and a counter has something to count.
func TestOneRemarkPerOccurrenceInOrder(t *testing.T) {
	rs := spacingRemarks(t, "(:);(:);(:)\n", syntax.Core())
	if len(rs) != 2 {
		t.Fatalf("got %d remarks, want one per occurrence: %+v", len(rs), rs)
	}
	for i, r := range rs {
		if r.Token != ";" || r.Next != "(" {
			t.Errorf("remark %d = %q and %q, want ; and (", i, r.Token, r.Next)
		}
	}
}

// TestItNamesTheLineItIsOn, which a one-line case cannot show: a remark that
// reported the parser's current line rather than the operator's would pass
// every case above and name the wrong line for a real script.
func TestItNamesTheLineItIsOn(t *testing.T) {
	rs := spacingRemarks(t, "echo one\necho two\n(:);(:)\necho four\n", syntax.Core())
	if len(rs) != 1 {
		t.Fatalf("got %d remarks, want 1: %+v", len(rs), rs)
	}
	if rs[0].Pos.Line != 3 || rs[0].At.Line != 3 {
		t.Errorf("lines %d and %d, want both 3", rs[0].Pos.Line, rs[0].At.Line)
	}
}

// TestItSurvivesAFatalError is the constraint the here-document remark has
// too, and the shapes that draw both are exactly the ones #2409 was filed
// from: the remark comes first and the refusal follows.
func TestItSurvivesAFatalError(t *testing.T) {
	p := syntax.NewParser("if |; then :; fi\n", syntax.Core())
	p.Parse()
	if p.Err() == nil {
		t.Fatal("want a parse failure, so that this tests what it says")
	}
	var n int
	for _, r := range p.Remarks() {
		if r.Kind == syntax.RemarkOperatorsNotSeparated {
			n++
		}
	}
	if n != 1 {
		t.Errorf("got %d remarks alongside the failure, want 1", n)
	}
}

// TestTheProgramItRemarksOnStillRuns is the other half, and it is what rules
// out hanging this off the error: `(:);(:)` and `:;(:)` are ordinary programs
// that parse cleanly, so a remark reached only from the error-reporting path
// would never be printed for them.
func TestTheProgramItRemarksOnStillRuns(t *testing.T) {
	for _, src := range []string{"(:);(:)\n", ":;(:)\n", ":;>f\n"} {
		p := syntax.NewParser(src, syntax.Core())
		p.Parse()
		if err := p.Err(); err != nil {
			t.Errorf("%q: %v, want a clean parse with a remark beside it", src, err)
		}
		if rs := spacingRemarks(t, src, syntax.Core()); len(rs) != 1 {
			t.Errorf("%q: got %d remarks, want 1", src, len(rs))
		}
	}
}
