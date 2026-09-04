// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A remark is the parser's third channel: input it accepted anyway, with
// something to say about it. There is one today and these pin its shape,
// because the shape was measured before it was built and the measurements
// rule out simpler ones.

func remarksOf(t *testing.T, src string) []syntax.Remark {
	t.Helper()
	p := syntax.NewParser(src, syntax.Core())
	p.Parse()
	return p.Remarks()
}

// TestAHereDocumentWithNoDelimiterIsRemarkedOn, and the two lines it carries
// are different: where the input ran out, and where the here-document began.
func TestAHereDocumentWithNoDelimiterIsRemarkedOn(t *testing.T) {
	rs := remarksOf(t, "echo one\ncat <<EOF\nbody\n")
	if len(rs) != 1 {
		t.Fatalf("got %d remarks, want 1: %+v", len(rs), rs)
	}
	if rs[0].Kind != syntax.RemarkHeredocAtEOF {
		t.Errorf("kind = %v, want RemarkHeredocAtEOF", rs[0].Kind)
	}
	if rs[0].At.Line != 2 {
		t.Errorf("At.Line = %d, want the line the here-document began on", rs[0].At.Line)
	}
	if rs[0].Pos.Line != 3 {
		t.Errorf("Pos.Line = %d, want the line the input ran out on", rs[0].Pos.Line)
	}
	if rs[0].Token != "EOF" {
		t.Errorf("Token = %q, want the delimiter that never arrived", rs[0].Token)
	}
}

// TestABodyWithNoLinesNamesTheHereDocumentsOwnLine: with nothing between the
// operator and the end, both lines are the same one. Measured — the location
// is the last line that had something on it and not one past the end.
func TestABodyWithNoLinesNamesTheHereDocumentsOwnLine(t *testing.T) {
	for _, src := range []string{"cat <<X\n", "cat <<X"} {
		rs := remarksOf(t, src)
		if len(rs) != 1 {
			t.Fatalf("%q: got %d remarks, want 1", src, len(rs))
		}
		if rs[0].Pos.Line != 1 || rs[0].At.Line != 1 {
			t.Errorf("%q: lines %d and %d, want both 1", src, rs[0].Pos.Line, rs[0].At.Line)
		}
	}
}

// TestADelimiterThatArrivesIsNotRemarkedOn, which is the control: the remark
// is about the delimiter never coming and not about here-documents.
func TestADelimiterThatArrivesIsNotRemarkedOn(t *testing.T) {
	if rs := remarksOf(t, "cat <<EOF\nbody\nEOF\n"); len(rs) != 0 {
		t.Errorf("got %+v, want nothing said", rs)
	}
}

// TestARemarkSurvivesAFatalError is the constraint that rules out the
// simplest design. A front end that reads remarks only when the parse
// succeeded would drop this one, and the shell that remarks prints both.
func TestARemarkSurvivesAFatalError(t *testing.T) {
	p := syntax.NewParser("f() { cat <<X\ny\n", syntax.Core())
	p.Parse()
	if p.Err() == nil {
		t.Fatal("want a parse failure, so that this tests what it says")
	}
	if rs := p.Remarks(); len(rs) != 1 {
		t.Errorf("got %d remarks alongside the failure, want 1", len(rs))
	}
}

// TestEachHereDocumentGetsItsOwn, so a front end that renders them can render
// more than one and a counter that tracks what it has shown has something to
// count.
func TestEachHereDocumentGetsItsOwn(t *testing.T) {
	rs := remarksOf(t, "cat <<A <<B\nbody\n")
	if len(rs) != 2 {
		t.Fatalf("got %d remarks, want one for each delimiter: %+v", len(rs), rs)
	}
	if rs[0].Token != "A" || rs[1].Token != "B" {
		t.Errorf("tokens %q and %q, want A and B in order", rs[0].Token, rs[1].Token)
	}
}
