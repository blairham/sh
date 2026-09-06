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
	// Every case below is a here-document whose delimiter carries the closing
	// parenthesis, and whether that is a program at all is a dialect question
	// (#963). A grammar that refuses it has a refusal to make and nothing to
	// remark on, so the remarks are only reachable — and only meaningful —
	// through the grammar that reads it.
	d := syntax.Core()
	d.HeredocEndsAtClosingParen = true
	p := syntax.NewParser(src, d)
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

// TestARemarkFromInsideACommandSubstitutionSurvives is #785.
//
// A command substitution's contents are read by a parser of its own, and what
// that read had to say was dropped with it: `v=$(cat <<EOF` … `EOF)` runs, `v`
// is `a`, and the one thing a parser accepts and remarks on went nowhere.
//
// The two lines are the point. The here-document's input is the substitution's
// own text, closing parenthesis included — so `EOF)` is a body line, the input
// runs out at the substitution's last line, and both are lines of the
// *program* rather than of the fragment.
func TestARemarkFromInsideACommandSubstitutionSurvives(t *testing.T) {
	rs := remarksOf(t, "v=$(cat <<EOF\na\nEOF)\necho \"v=[$v]\"\n")
	if len(rs) != 1 {
		t.Fatalf("got %d remarks, want 1: %+v", len(rs), rs)
	}
	if rs[0].At.Line != 1 {
		t.Errorf("At.Line = %d, want 1 — the line the here-document began on", rs[0].At.Line)
	}
	if rs[0].Pos.Line != 3 {
		t.Errorf("Pos.Line = %d, want 3 — the substitution's last line, not the script's", rs[0].Pos.Line)
	}
	if rs[0].Token != "EOF" {
		t.Errorf("Token = %q, want EOF", rs[0].Token)
	}
}

// TestTheLinesAreTheProgramsWhereverTheSubstitutionBegins: the fragment is
// numbered from where it starts, so a substitution further down the file
// reports the file's lines and not the fragment's.
func TestTheLinesAreTheProgramsWhereverTheSubstitutionBegins(t *testing.T) {
	rs := remarksOf(t, "echo before\nv=$(cat <<EOF\na\nEOF)\necho after\n")
	if len(rs) != 1 {
		t.Fatalf("got %d remarks, want 1: %+v", len(rs), rs)
	}
	if rs[0].At.Line != 2 || rs[0].Pos.Line != 4 {
		t.Errorf("lines %d and %d, want 4 and 2", rs[0].Pos.Line, rs[0].At.Line)
	}
}

// TestASubstitutionWhoseDelimiterArrivesSaysNothing is the control, and it is
// the one that keeps this from being "every substitution is read twice and
// remarked on": with the delimiter on a line of its own the substitution reads
// cleanly the first time and there is nothing to say.
func TestASubstitutionWhoseDelimiterArrivesSaysNothing(t *testing.T) {
	if rs := remarksOf(t, "v=$(cat <<EOF\na\nEOF\n)\necho ok\n"); len(rs) != 0 {
		t.Errorf("got %+v, want nothing said", rs)
	}
}

// TestEachSubstitutionIsRemarkedOnOnce, so a script with two of them says two
// things rather than one or four.
func TestEachSubstitutionIsRemarkedOnOnce(t *testing.T) {
	rs := remarksOf(t, "v=$(cat <<EOF\na\nEOF)\nw=$(cat <<XX\nb\nXX)\necho ok\n")
	if len(rs) != 2 {
		t.Fatalf("got %d remarks, want 2: %+v", len(rs), rs)
	}
	if rs[0].Token != "EOF" || rs[1].Token != "XX" {
		t.Errorf("tokens %q and %q, want EOF and XX in order", rs[0].Token, rs[1].Token)
	}
	if rs[0].At.Line != 1 || rs[1].At.Line != 4 {
		t.Errorf("At lines %d and %d, want 1 and 4", rs[0].At.Line, rs[1].At.Line)
	}
}

// TestAProcessSubstitutionIsReadTheSameWay: the parentheses hold a program
// there too, and bash remarks on `<(cat <<EOF` … `EOF)` exactly as it does on
// the `$( )` form. Measured; without it the fix would have been about one
// spelling rather than about what is inside.
func TestAProcessSubstitutionIsReadTheSameWay(t *testing.T) {
	d := syntax.Core()
	d.ProcessSubstitution = true
	// And whether these parentheses end the body is the same dialect question
	// the `$( )` spelling asks, so the grammar that reads one reads the other
	// (#963).
	d.HeredocEndsAtClosingParen = true
	p := syntax.NewParser("cat <(cat <<EOF\na\nEOF)\necho done\n", d)
	p.Parse()
	rs := p.Remarks()
	if len(rs) != 1 {
		t.Fatalf("got %d remarks, want 1: %+v", len(rs), rs)
	}
	if rs[0].At.Line != 1 || rs[0].Pos.Line != 3 || rs[0].Token != "EOF" {
		t.Errorf("remark = %+v, want EOF opened at line 1 and run out at line 3", rs[0])
	}
}

// TestAnArithmeticSubstitutionIsNotAProgram is the other side of that line,
// and it is the reason the line exists at all: `<<` is a **left shift** there.
// Read as a program it is a here-document whose delimiter never arrives, and
// the shell would warn about a script that has none.
//
// The shift is written across lines, and that is the whole of the case rather
// than layout. A here-document's body is read at the newline that ends the
// command, so a one-line `$(( 4 << 2 ))` never reaches the read and passes
// whichever answer the code gives — which is exactly what it did while this
// was written on one line, and what a mutation of the rule showed.
func TestAnArithmeticSubstitutionIsNotAProgram(t *testing.T) {
	for _, src := range []string{
		"echo $(( 4 << 2 ))\necho $((a<<b))\n",
		"echo $((\n4 << 2\n))\n",
		"x=5\necho $(( x <<\n2 ))\n",
	} {
		if rs := remarksOf(t, src); len(rs) != 0 {
			t.Errorf("%q: got %+v, want nothing said about a shift", src, rs)
		}
	}
}
