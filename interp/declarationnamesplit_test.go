// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// declarationNameSplitByBytes is the walk as it was before the name was
// taken in slices (#5873): the same rules, with the name built one character
// at a time. It is the reference the slicing walk has to agree with.
func (r *Runner) declarationNameSplitByBytes(w *syntax.Word) (span, off int, appends, ok bool) {
	if w == nil || len(w.Spans) == 0 {
		return 0, 0, false, false
	}
	head := w.Spans[0]
	if head.Kind != syntax.Literal || head.Quoting != syntax.Unquoted {
		return 0, 0, false, false
	}
	depth, closed, plus, name := 0, false, false, ""
	for i, s := range w.Spans {
		if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
			if depth == 0 {
				return 0, 0, false, false
			}
			continue
		}
		for j := 0; j < len(s.Value); j++ {
			c := s.Value[j]
			if depth == 0 && plus && c != '=' {
				return 0, 0, false, false
			}
			switch {
			case c == '[':
				if depth == 0 && closed {
					return 0, 0, false, false
				}
				depth++
			case c == ']':
				if depth > 0 {
					depth--
					closed = depth == 0
				}
			case c == '+' && depth == 0:
				plus = true
			case c == '=' && depth == 0:
				if !isPlainName(name) && !r.isLocaleName(name) {
					return 0, 0, false, false
				}
				return i, j, plus, true
			default:
				if depth == 0 {
					if closed {
						return 0, 0, false, false
					}
					name += s.Value[j : j+1]
				}
			}
		}
	}
	return 0, 0, false, false
}

// TestDeclarationNameSplitAgreesWithTheByteWalk puts operands of every shape
// the walk has a rule for through both walks, each as one span and cut into
// spans at every byte, plus words with a quoted or substituted span in front,
// inside and after the subscript.
func TestDeclarationNameSplitAgreesWithTheByteWalk(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	texts := []string{
		"a=1", "abc=", "abc+=x", "a[1]=v", "a[k=1]=v", "a[1]+=v", "a]b=1", "x+y=v",
		"x++=v", "a[1][2]=v", "a[1]x=v", "LC_ALL=C", "1a=1", "a-b=1", "a", "a[", "=v",
		"[1]=v", "a[[1]]=v", "é=1", "_=1", "a b=1", "aVeryLongNameIndeed_0123456789=value",
	}
	lit := func(v string) syntax.Span {
		return syntax.Span{Kind: syntax.Literal, Quoting: syntax.Unquoted, Value: v}
	}
	var words []*syntax.Word
	for _, s := range texts {
		words = append(words, &syntax.Word{Spans: []syntax.Span{lit(s)}})
		for cut := 1; cut < len(s); cut++ {
			words = append(words, &syntax.Word{Spans: []syntax.Span{lit(s[:cut]), lit(s[cut:])}})
		}
		var each []syntax.Span
		for i := 0; i < len(s); i++ {
			each = append(each, lit(s[i:i+1]))
		}
		if len(each) > 0 {
			words = append(words, &syntax.Word{Spans: each})
		}
	}
	quoted := syntax.Span{Kind: syntax.Literal, Quoting: syntax.DoubleQuoted, Value: "k"}
	words = append(words,
		&syntax.Word{Spans: []syntax.Span{quoted, lit("=1")}},
		&syntax.Word{Spans: []syntax.Span{lit("m["), quoted, lit("]=v")}},
		&syntax.Word{Spans: []syntax.Span{lit("ab"), quoted, lit("=v")}},
		&syntax.Word{Spans: []syntax.Span{lit("a=b"), quoted}},
	)
	for _, w := range words {
		var parts []string
		for _, s := range w.Spans {
			parts = append(parts, s.Value)
		}
		gs, go_, ga, gok := r.declarationNameSplit(w)
		ws, wo, wa, wok := r.declarationNameSplitByBytes(w)
		if gs != ws || go_ != wo || ga != wa || gok != wok {
			t.Errorf("%q: slices say (%d %d %v %v), bytes say (%d %d %v %v)",
				strings.Join(parts, "|"), gs, go_, ga, gok, ws, wo, wa, wok)
		}
	}
}

// TestADeclarationNameIsNotBuiltACharacterAtATime pins the cost: a name in
// one span is a slice of it, however long it is.
func TestADeclarationNameIsNotBuiltACharacterAtATime(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	w := &syntax.Word{Spans: []syntax.Span{{
		Kind: syntax.Literal, Quoting: syntax.Unquoted,
		Value: "aVeryLongNameIndeed_0123456789=value",
	}}}
	if n := testing.AllocsPerRun(100, func() { r.declarationNameSplit(w) }); n > 0 {
		t.Errorf("splitting a 30-character name allocated %v times, want none", n)
	}
}
