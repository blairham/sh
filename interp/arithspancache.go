// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/blairham/sh/syntax"
)

// How an arithmetic expression with an expansion in it is cut into spans,
// kept per text so that it is read once rather than at every evaluation.
//
// An expression with a `$` in it is expanded and then read when it is
// evaluated, because what it expands to is part of the expression — that
// half has to run every time. The cut before it does not: which spans a
// text holds, and the parameter expansion each `${…}` is, are facts about
// the text and the grammar reading it, and nothing the script does between
// two evaluations changes them. F-Sy-H's highlighter evaluates
// `(( ${+seen[$line]} ))` twenty times a keystroke, and the cut was a
// fresh lexer, a fresh parser and a parse of every `${…}` each time (#6423).
//
// The key is the text and what reads it: the dialect pointer, which a
// runner swaps rather than writes (so a pointer is one grammar), and the
// arithmetic order, which the dialect a parse is handed carries. Two kinds
// of text are never kept, because something else reads them: bytes past
// ASCII, whose characters are the locale's to delimit, and a `<<`, which
// could open a here-document whose delimiter the runner decodes. A text
// that does not read is not kept either, so its refusal is said every time.
//
// Process-wide and shared by every runner, so what it holds is never
// written: a hit is handed a copy of the span slice, and the parameter
// nodes in it are shared the way a function body's nodes are shared by
// every call of the function.
var arithSpanCache sync.Map // arithSpanKey -> *arithSpanEntry

// arithSpanEntry is one text's spans, and every parameter expansion node in
// them, nested ones included: the nodes are handed to every evaluation of the
// text, and what an evaluation holds about one has to be let go when it ends.
type arithSpanEntry struct {
	spans []syntax.Span
	nodes []*syntax.ParamExpr
}

// arithSpanCacheSize counts the entries, which stop being added at
// arithSpanCacheLimit: the texts are a program's own, but an expression
// that arrived expanded — `let "$e"` — is a text per value.
var arithSpanCacheSize atomic.Int64

const arithSpanCacheLimit = 4096

// arithSpanCacheDisabled turns the cache off, so that a test can read a
// text both ways. Nothing outside a test writes it.
var arithSpanCacheDisabled bool

type arithSpanKey struct {
	text  string
	d     *syntax.Dialect
	prec  syntax.ArithPrecedencePolicy
	moved bool
}

// cachedArithSpans is arithSpans, answered from the cache where the text is
// one it keeps.
func (r *Runner) cachedArithSpans(text string) ([]syntax.Span, []*syntax.ParamExpr, bool) {
	if arithSpanCacheDisabled || !isASCII(text) || strings.Contains(text, "<<") {
		spans, ok := r.rawSpansRead(text, syntax.ArithTextSpans)
		return spans, nil, ok
	}
	k := arithSpanKey{text: text, d: r.Dialect, prec: r.arithPrecedence, moved: r.arithPrecedenceMoved}
	if v, ok := arithSpanCache.Load(k); ok {
		e := v.(*arithSpanEntry)
		out := make([]syntax.Span, len(e.spans))
		copy(out, e.spans)
		return out, e.nodes, true
	}
	spans, ok := r.rawSpansRead(text, syntax.ArithTextSpans)
	if !ok {
		return spans, nil, ok
	}
	if arithSpanCacheSize.Load() >= arithSpanCacheLimit {
		return spans, nil, true
	}
	e := &arithSpanEntry{spans: make([]syntax.Span, len(spans))}
	copy(e.spans, spans)
	collectParamExprs(reflect.ValueOf(e.spans), &e.nodes, map[uintptr]bool{})
	if v, loaded := arithSpanCache.LoadOrStore(k, e); loaded {
		// Another runner kept the text first; its nodes are the ones that
		// will be handed out, and these are this read's alone.
		_ = v
		return spans, nil, true
	}
	arithSpanCacheSize.Add(1)
	return spans, e.nodes, true
}

var paramExprType = reflect.TypeFor[*syntax.ParamExpr]()

// collectParamExprs appends every *syntax.ParamExpr reachable from v. It runs
// once per kept text, so it can afford to be general rather than know the
// shape of every node a span may hold.
func collectParamExprs(v reflect.Value, out *[]*syntax.ParamExpr, seen map[uintptr]bool) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() || seen[v.Pointer()] {
			return
		}
		seen[v.Pointer()] = true
		if v.Type() == paramExprType {
			*out = append(*out, v.Interface().(*syntax.ParamExpr))
		}
		collectParamExprs(v.Elem(), out, seen)
	case reflect.Interface:
		if !v.IsNil() {
			collectParamExprs(v.Elem(), out, seen)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			collectParamExprs(v.Field(i), out, seen)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			collectParamExprs(v.Index(i), out, seen)
		}
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			collectParamExprs(it.Value(), out, seen)
		}
	}
}

// forgetHoldsOn lets go of every hold an expansion left on one of nodes.
//
// A word's expansion clears them at its start for the same reason (see
// expandAt): a loop expands one node many times. A kept arithmetic text is
// the same nodes every evaluation, and `for p ( $keep ) { (( ! $on_disk[(I)$p]
// )) }` reached the second evaluation with the first one's subscript still
// held, and answered it. Only holds on these nodes go — one an enclosing
// expansion made on its own node is left where it was, as it was when every
// evaluation read the text afresh.
func (r *Runner) forgetHoldsOn(nodes []*syntax.ParamExpr) {
	if r.sourceHeld.held && slices.Contains(nodes, r.sourceHeld.node) {
		r.sourceHeld = sourceHold{}
	}
	if r.subscriptHeld.held && slices.Contains(nodes, r.subscriptHeld.node) {
		r.subscriptHeld = subscriptHold{}
	}
	if r.nestedHeld.held && slices.Contains(nodes, r.nestedHeld.node) {
		r.nestedHeld = nestedHold{}
	}
	if r.nestedShape.known && slices.Contains(nodes, r.nestedShape.node) {
		r.nestedShape = nestedShapeMemo{}
	}
}
