// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/blairham/sh/syntax"
)

// The trees of arithmetic text read at run time, kept per text so that a text
// is parsed once rather than at every evaluation.
//
// An expression written in a program is parsed with the program. The ones
// that are not reach the evaluator as a string, after something has expanded
// them: a subscript (`${s[offset + n + 1,len]}`), a substring's range, the
// value assigned to a name declared integer, an operand of `[[ -lt ]]`, a
// value read again as an expression. Each of those built a parser and parsed
// the text at every evaluation, and the text is almost always the same one:
// F-Sy-H's highlighter cuts its buffer with `${proc_buf[offset + __asize +
// 1,__len]}` once per word, and parsed about 75 texts per keystroke, a fresh
// parser and lexer for each (#6423).
//
// What a text parses to is a fact about the text and the grammar reading it,
// so the key is the text, the dialect pointer (a runner swaps its dialect
// rather than writing it, so a pointer is one grammar; see dialectmove.go),
// the arithmetic order, which the dialect a parse is handed carries, and which
// of the two readings asked. The same reasoning as arithSpanCache, which keeps
// the cut of a text that still has expansions in it; this keeps the tree of
// one that has none left.
//
// Three kinds of text are not kept. Bytes past ASCII, whose characters are
// the locale's to delimit, which the key does not hold. A `<<`, which could
// open a here-document whose delimiter the runner decodes. And a text that
// does not parse, so that its refusal is worded and reported every time,
// exactly as before.
//
// Process-wide and shared by every runner, so a kept tree is never written:
// the evaluator reads a tree and writes nothing into it, which is already what
// a tree parsed with the program relies on — a loop evaluates one many times.
var arithTreeCache sync.Map // arithTreeKey -> syntax.ArithExpr

// arithTreeCacheSize counts the entries. When it reaches arithTreeCacheLimit
// the cache is emptied and starts again, rather than stopping. Most of what a
// long session reads is a program's own text, but a value read as an
// expression is a text per value, and a cache that stopped taking entries
// once full would be holding whatever filled it first, for as long as the
// shell runs. Starting again bounds the footprint and lets the texts in use
// now back in.
var arithTreeCacheSize atomic.Int64

const arithTreeCacheLimit = 4096

// arithTreeCacheDisabled turns the cache off, so that a test can read a text
// both ways. Nothing outside a test writes it.
var arithTreeCacheDisabled bool

// arithReading is which of the parser's three readings of run-time text is
// asked for, which decides what a `$` left in the text is and whether its
// double quotation comes off: see syntax.Parser.ParseArithFor,
// syntax.Parser.ParseArithExpanded and syntax.Parser.ParseArithSubscript.
type arithReading uint8

const (
	// arithReadExpanded is ParseArithExpanded: the text is a result, and a
	// `$` in it is an ordinary character.
	arithReadExpanded arithReading = iota
	// arithReadAsWritten is ParseArithFor: a `$` in the text is an expansion
	// the caller still has to perform, and the parser leaves no tree.
	arithReadAsWritten
	// arithReadSubscript is ParseArithSubscript: arithReadExpanded for the
	// text between an index's brackets, which takes the dialect's
	// double-quote removal.
	arithReadSubscript
)

type arithTreeKey struct {
	text    string
	d       *syntax.Dialect
	prec    syntax.ArithPrecedencePolicy
	moved   bool
	reading arithReading
}

// readArithText parses text as reading asks, from the cache where the text is
// one it keeps. The error is the parser's own, for the caller to word.
//
// A nil tree with no error — text ParseArithFor leaves for its caller to
// expand — is answered the way the parser answers it and is not kept.
func (r *Runner) readArithText(text string, reading arithReading) (syntax.ArithExpr, error) {
	keep := !arithTreeCacheDisabled && isASCII(text) && !strings.Contains(text, "<<")
	var k arithTreeKey
	if keep {
		k = arithTreeKey{
			text:    text,
			d:       r.Dialect,
			prec:    r.arithPrecedence,
			moved:   r.arithPrecedenceMoved,
			reading: reading,
		}
		if v, ok := arithTreeCache.Load(k); ok {
			// What building a dialect for the parse would have recorded,
			// which a hit builds none to record. See
			// Runner.arithReadInAnotherOrder.
			r.noteTheArithReadingOrder()
			tree, _ := v.(syntax.ArithExpr)
			return tree, nil
		}
	}
	p := syntax.NewParser("", r.dialect())
	var tree syntax.ArithExpr
	switch reading {
	case arithReadAsWritten:
		tree = p.ParseArithFor(text, syntax.Pos{})
	case arithReadSubscript:
		tree = p.ParseArithSubscript(text, syntax.Pos{})
	default:
		tree = p.ParseArithExpanded(text, syntax.Pos{})
	}
	err := p.Err()
	if keep && err == nil && tree != nil {
		keepArithTree(k, tree)
	}
	return tree, err
}

// keepArithTree adds one entry, emptying the cache first when it is full. The
// count can drift by a racing writer or two between the emptying and the next
// add, which moves when the next emptying comes and nothing else.
func keepArithTree(k arithTreeKey, tree syntax.ArithExpr) {
	if arithTreeCacheSize.Load() >= arithTreeCacheLimit {
		arithTreeCache.Clear()
		arithTreeCacheSize.Store(0)
	}
	if _, loaded := arithTreeCache.LoadOrStore(k, tree); !loaded {
		arithTreeCacheSize.Add(1)
	}
}
