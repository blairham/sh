// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/syntax"
)

// [syntax.Dialect] is a public struct a caller fills in, so every assignment
// of its fields is an input this parser has to survive — and until #911
// nothing exercised any of them but the four presets. That gap is what let a
// parse *panic* rather than return the syntax error its own doc comment
// promised: `Coproc` without `CoprocName` and with `FuncDefAtParen` reads
// `coproc MY ( … ` as a coprocess whose command is a function definition,
// the definition is refused and keeps a node with no body, and the coproc
// then measured the body that was not there.
//
// No preset has that pair of flags, and no preset has the second pair the
// same bug was reachable through either. That is the finding rather than the
// panic: a combination nothing constructs is a combination nothing tests, and
// there are 2^67 of them.
//
// So this file parses under combinations rather than under presets. It cannot
// be exhaustive — there are 2^67 vectors — and it does not try. It takes the
// two edges of the lattice a bug of this shape has to sit on to be
// interesting, every flag and every flag but one, plus a fixed spread of
// mixtures across the middle. The generator reads the struct by reflection, so
// a flag added tomorrow is covered by having been declared.

// dialectBoolFields is the position of every exported bool in
// [syntax.Dialect]. The non-bool fields are left at their zero value: an alias
// table and a routing enum are not a grammar switch, and a nil map is the
// "no aliases" every case here wants.
func dialectBoolFields() []int {
	var out []int
	t := reflect.TypeOf(syntax.Dialect{})
	for i := range t.NumField() {
		f := t.Field(i)
		if f.IsExported() && f.Type.Kind() == reflect.Bool {
			out = append(out, i)
		}
	}
	return out
}

type namedDialect struct {
	name string
	d    syntax.Dialect
}

// combinationVectors is the flag vectors every input below is parsed under.
//
// The named ones are deliberate: `every-flag-but-X` is the sweep that found
// #911 by hand, one flag at a time, and `every-flag` and `no-flag` are the
// ends it sweeps between. The mixtures are seeded from a fixed generator so a
// failure is reproducible; math/rand/v2's PCG is documented to be stable,
// which a test that names a seed relies on.
//
// A vector per flag is 67 subtests over the whole corpus, which is why they
// run in parallel: the sweep is seconds of wall time that way and a minute
// serially, and a test nobody wants to wait for is a test somebody skips.
func combinationVectors() []namedDialect {
	fields := dialectBoolFields()
	typ := reflect.TypeOf(syntax.Dialect{})
	set := func(on func(n int) bool) syntax.Dialect {
		var d syntax.Dialect
		v := reflect.ValueOf(&d).Elem()
		for n, i := range fields {
			v.Field(i).SetBool(on(n))
		}
		return d
	}
	out := []namedDialect{
		{"every-flag", set(func(int) bool { return true })},
		{"no-flag", set(func(int) bool { return false })},
	}
	for n, i := range fields {
		name := typ.Field(i).Name
		out = append(out, namedDialect{
			"every-flag-but-" + name,
			set(func(k int) bool { return k != n }),
		})
	}
	rng := rand.New(rand.NewPCG(0x5eed, 0xf1a6))
	for r := range 32 {
		out = append(out, namedDialect{
			"mixture-" + string(rune('a'+r)),
			set(func(int) bool { return rng.Uint64()&1 == 0 }),
		})
	}
	return out
}

// inputsFor is one corpus snippet turned into the family of inputs worth
// parsing under a grammar it was not written for.
//
// The snippet itself, first: it is a complete program a real shell ran.
//
// Then every prefix that stops after a byte the grammar structures on, which
// is the "ran out of input" half. A construct the parser opened and never
// closed is where a node keeps a child it never got, and that is the shape
// #911 turned out to be.
//
// Then a `(` inserted after each word, which is the other half and is
// targeted rather than random. A parenthesis where a word has just ended is
// how a *function definition* appears in text nobody wrote one in — that is
// what `FuncDefAtParen` means — and a definition whose body the grammar then
// refuses is exactly the node with nothing in it. Both call sites of the #911
// bug were reached this way and one of them only this way: no corpus snippet
// puts a bare `(` after a word, because no real shell would have run it.
func inputsFor(src string) []string {
	out := []string{src}
	const structural = "(){};&|<>\n"
	for i := 1; i < len(src); i++ {
		if strings.IndexByte(structural, src[i-1]) >= 0 {
			out = append(out, src[:i])
		}
	}
	for i := 1; i < len(src); i++ {
		if strings.IndexByte(structural+" \t", src[i]) >= 0 && strings.IndexByte(structural+" \t", src[i-1]) < 0 {
			out = append(out, src[:i]+"("+src[i:])
		}
	}
	return out
}

// TestNoDialectCombinationPanics is the whole property: syntax.Parse answers,
// for every input, under every grammar. It may answer with a syntax error —
// most of these inputs are nonsense under most of these grammars — but it may
// not panic, because a library that crashes on a value its own public struct
// permits has no answer for the caller that built it.
//
// The failure prints the input and the vector's name, which is enough to
// rebuild the dialect by hand: `every-flag-but-CoprocName` is `syntax.Core()`
// with everything on except that one.
func TestNoDialectCombinationPanics(t *testing.T) {
	for _, v := range combinationVectors() {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()
			for _, c := range oracle.Corpus {
				for _, src := range inputsFor(c.Snippet) {
					func() {
						defer func() {
							if e := recover(); e != nil {
								t.Fatalf("%s under %s: %q panicked: %v", c.ID, v.name, src, e)
							}
						}()
						_, _ = syntax.Parse(src, v.d)
					}()
				}
			}
		})
	}
}
