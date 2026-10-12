// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"reflect"
	"strconv"
	"testing"

	"github.com/blairham/sh/syntax"
)

// TestAFunctionBatchIsTheParse holds FunctionBatch to what it claims: for
// every name, the declaration it stores and the answer it gives are the ones
// DefineFunction gives for that name alone. The names are ordered so that each
// shortcut the batch could wrongly take has a name to take it on — a keyword,
// a name with a quote or a dot, a name of a length already seen, the same
// name twice — and the grammars include the three that could carry a name
// outside Name.
func TestAFunctionBatchIsTheParse(t *testing.T) {
	names := []string{
		"_a", "_b", "if", "fi", "do", "_ab", "_a-b", "_1", "_12", "_a.b", "_a'", "_x$",
		"_", "a", "ab", "time", "_abc", "_xyz", "_x-y", "_Ab9", "!x", "{", "_a",
		"function", "_xyzwvut", "_é", "_a b", "_long_completion-name", "_long_completion-nbme",
	}
	bodies := []string{
		"builtin autoload -XUz",
		"builtin autoload -XU /some/dir",
		"builtin autoload -X '/a b'",
		"print -r -- $0 $LINENO",
		"if",
	}
	grammars := map[string]func(*syntax.Dialect){
		"core":         func(*syntax.Dialect) {},
		"name word":    func(d *syntax.Dialect) { d.FunctionNameExpands = true },
		"source text":  func(d *syntax.Dialect) { d.FunctionDefinitionIsSourceText = true },
		"several name": func(d *syntax.Dialect) { d.FunctionMultipleNames = true },
	}
	for gname, set := range grammars {
		for _, body := range bodies {
			d := syntax.Core()
			set(&d)
			batched := newTestRunner(t, &Runner{Dialect: &d, Stdout: io.Discard, Stderr: io.Discard})
			alone := newTestRunner(t, &Runner{Dialect: &d, Stdout: io.Discard, Stderr: io.Discard})
			b := batched.FunctionBatch(body)
			for _, name := range names {
				got, want := b.Define(name), alone.DefineFunction(name, body)
				if got != want {
					t.Errorf("%s, %q, %q: batch says %v, a parse says %v", gname, body, name, got, want)
					continue
				}
				if !reflect.DeepEqual(batched.funcs[name], alone.funcs[name]) {
					t.Errorf("%s, %q, %q: the batch stored\n%#v\nwhere a parse stores\n%#v",
						gname, body, name, batched.funcs[name], alone.funcs[name])
				}
				if !reflect.DeepEqual(batched.funcOrigins[name], alone.funcOrigins[name]) {
					t.Errorf("%s, %q, %q: origin %+v, a parse's %+v",
						gname, body, name, batched.funcOrigins[name], alone.funcOrigins[name])
				}
			}
		}
	}
}

// TestAFunctionBatchParsesOncePerLength pins the cost the batch exists for: a
// line of two thousand names whose lengths are few is a few parses, not two
// thousand. Counted in allocations, because a parse is hundreds of them and a
// copied declaration is one.
func TestAFunctionBatchParsesOncePerLength(t *testing.T) {
	d := syntax.Core()
	r := newTestRunner(t, &Runner{Dialect: &d, Stdout: io.Discard, Stderr: io.Discard})
	names := make([]string, 2000)
	for i := range names {
		names[i] = "_fn" + strconv.Itoa(i)
	}
	// Warm the function table to its size first, so what is counted is the
	// definitions rather than the map growing under them.
	for _, n := range names {
		r.DefineFunction(n, "builtin autoload -XUz")
	}
	b := r.FunctionBatch("builtin autoload -XUz")
	allocs := testing.AllocsPerRun(1, func() {
		for _, n := range names {
			if !b.Define(n) {
				t.Fatalf("%s was refused", n)
			}
		}
	})
	if per := allocs / float64(len(names)); per > 20 {
		t.Errorf("%.1f allocations a name; a copied declaration is a handful, a parse is hundreds", per)
	}
}
