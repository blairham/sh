// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// argText is the literal text of an operator's operand, which for every row
// here is a single literal span.
func argText(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var b []byte
	for _, s := range w.Spans {
		b = append(b, s.Value...)
	}
	return string(b)
}

// ParamColonBeforeTrimIsIgnored: a colon before a trim is read as nothing, so
// the node is the trim it would have been without it.
//
// Named for the flag and never for a shell — which shell answers it lives in
// dialect/. The node identity is the assertion: the whole point of dropping
// the colon rather than recording it is that everything downstream sees the
// same node the plain spelling produces, so a test that only checked the
// operator would miss a stray Colon flag nothing reads.
func TestAColonBeforeATrimIsReadAsTheTrim(t *testing.T) {
	d := syntax.Core()
	d.ParamColonBeforeTrimIsIgnored = true
	d.ParamSubstring = true

	for _, c := range []struct {
		src  string
		op   syntax.ParamOp
		word string
	}{
		{"echo ${v:#p}", syntax.ParamTrimPrefix, "p"},
		{"echo ${v:##p}", syntax.ParamTrimPrefixLong, "p"},
		{"echo ${v:%p}", syntax.ParamTrimSuffix, "p"},
		{"echo ${v:%%p}", syntax.ParamTrimSuffixLong, "p"},
		// An empty pattern is the trim with nothing to trim, not a refusal.
		{"echo ${v:#}", syntax.ParamTrimPrefix, ""},
	} {
		e := paramOf(t, c.src, d)
		if e.Op != c.op {
			t.Errorf("%s: Op = %v, want %v", c.src, e.Op, c.op)
		}
		if e.Colon {
			t.Errorf("%s: Colon = true, want false — the colon is ignored rather than recorded", c.src)
		}
		if got := argText(e.Arg); got != c.word {
			t.Errorf("%s: pattern = %q, want %q", c.src, got, c.word)
		}
	}
}

// Without the flag the same source is the substring reading, whose operand is
// an arithmetic expression beginning `#p`. That is the difference the flag
// exists to make, so it is asserted rather than assumed.
func TestWithoutTheFlagAColonBeforeATrimIsAnOffset(t *testing.T) {
	d := syntax.Core()
	d.ParamSubstring = true
	e := paramOf(t, "echo ${v:#p}", d)
	if e.Op != syntax.ParamSubstring {
		t.Errorf("Op = %v, want ParamSubstring", e.Op)
	}
	if got := argText(e.Arg); got != "#p" {
		t.Errorf("offset = %q, want %q", got, "#p")
	}
}

// Only the four trims. The flag must not widen to every colon, and the
// substring is what it would swallow if it did.
func TestTheFlagLeavesEveryOtherColonAlone(t *testing.T) {
	d := syntax.Core()
	d.ParamColonBeforeTrimIsIgnored = true
	d.ParamSubstring = true
	d.ParamSubstitution = true

	for _, c := range []struct {
		src string
		op  syntax.ParamOp
	}{
		{"echo ${v:2}", syntax.ParamSubstring},
		{"echo ${v:2:2}", syntax.ParamSubstring},
		{"echo ${v:-d}", syntax.ParamDefault},
		{"echo ${v:=d}", syntax.ParamAssign},
		{"echo ${v:?e}", syntax.ParamError},
		{"echo ${v:+w}", syntax.ParamAlternate},
		// The replacement and a colon with a space after it are not the
		// form: the disambiguation is the single character immediately after
		// the colon, exactly as it is for the element-selecting three.
		{"echo ${v:/l/L}", syntax.ParamSubstring},
		{"echo ${v: #p}", syntax.ParamSubstring},
	} {
		e := paramOf(t, c.src, d)
		if e.Op != c.op {
			t.Errorf("%s: Op = %v, want %v", c.src, e.Op, c.op)
		}
	}
}

// The other reading of `:#` is the element exclusion, and the two flags are
// never both on in the panel. The order is fixed anyway, so the pair has a
// defined reading rather than one that depends on which case came first.
func TestElementSelectionWinsOverTheColonTrim(t *testing.T) {
	d := syntax.Core()
	d.ParamColonBeforeTrimIsIgnored = true
	d.ParamElementSelection = true
	d.ParamSubstring = true
	e := paramOf(t, "echo ${v:#p}", d)
	if e.Op != syntax.ParamExclude {
		t.Errorf("Op = %v, want ParamExclude", e.Op)
	}
}
