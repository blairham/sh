// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// FunctionBatch defines many functions that share one body text, parsing that
// text once per name *length* instead of once per name.
//
// It exists for one caller and one shape. A completion dump declares every
// completion function on a single `autoload -Uz` line — 2,275 names on the
// maintainer's real configuration — and each name gets the same stub body.
// [Runner.DefineFunction] reads `name() {` + body + `}` through a new parser
// for every one of them, which was ~3.5 µs a name and ~8 ms of every
// interactive start, against 0.3 ms for the whole line in zsh 5.9.2
// (measured 2026-10-12, #6363).
//
// Two names give the same tree, apart from the name itself, when the parser
// reads them the same way and they are the same length: every position in
// the body is an offset past the name, so the length is what moves them, and
// the spelling moves nothing once the word is an ordinary literal. So a
// declaration parsed for one name is copied for another of its length, with
// the name replaced, and is indistinguishable from the parse —
// TestAFunctionBatchIsTheParse holds that, name by name, against
// DefineFunction.
//
// "Read the same way" is kept narrow on purpose rather than derived: a name
// that starts with `_` and holds only letters, digits, `_` and `-`. No reserved
// word, precommand or declaration utility starts with `_` in any dialect, so
// no such name is a keyword to the parser, and nothing in it is a quote, an
// expansion or a pattern. Every other name is parsed, exactly as
// DefineFunction would. That covers the dump, whose names are all completion
// functions, and leaves the general case alone.
//
// A batch is good for one builtin's run and no longer. The tree depends on the
// grammar, and a script can change the grammar between two commands; within
// one command nothing runs in between, so nothing can.
//
// The copies share the body with the declaration they were copied from. A
// body shared by two names is already a state the interpreter holds — see
// [Runner.CopyFunction], `functions -c` — and the names, which are what the
// function table and every frame are keyed on, are each their own.
type FunctionBatch struct {
	r     *Runner
	body  string
	byLen map[int]*syntax.FuncDecl
}

// FunctionBatch starts a batch of definitions sharing body. See FunctionBatch.
func (r *Runner) FunctionBatch(body string) *FunctionBatch {
	return &FunctionBatch{r: r, body: body}
}

// Define is [Runner.DefineFunction] for one name of the batch, with the same
// result.
func (b *FunctionBatch) Define(name string) bool {
	if !batchableName(name) {
		return b.r.DefineFunction(name, b.body)
	}
	if like := b.byLen[len(name)]; like != nil {
		decl := *like
		decl.Name = name
		b.r.defineDecl(name, &decl)
		return true
	}
	if !b.r.DefineFunction(name, b.body) {
		return false
	}
	decl := b.r.funcs[name]
	if decl == nil || decl.SourceText != "" {
		// A grammar that keeps the definition's written text keeps the name
		// in it, and a copy would carry the first name's text. The other
		// places a name can sit — the written word, a refused name, the
		// extra names of a list — need a character this name class does
		// not have.
		return true
	}
	if b.byLen == nil {
		b.byLen = map[int]*syntax.FuncDecl{}
	}
	b.byLen[len(name)] = decl
	return true
}

// batchableName reports whether a name is one FunctionBatch may copy a
// declaration for. See FunctionBatch.
func batchableName(name string) bool {
	if len(name) < 2 || name[0] != '_' {
		return false
	}
	for i := 1; i < len(name); i++ {
		switch c := name[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
