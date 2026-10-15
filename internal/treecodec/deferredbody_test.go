// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package treecodec_test

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/treecodec"
	"github.com/blairham/sh/syntax"
)

// countDeferred is how many definitions in a tree hold a body not read yet.
// It looks without reading, so it can be asked before Settle.
func countDeferred(tree any) int {
	n := 0
	seen := map[*syntax.FuncDecl]bool{}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() {
				return
			}
			if fd, ok := v.Interface().(*syntax.FuncDecl); ok {
				if seen[fd] {
					return
				}
				seen[fd] = true
				if fd.Deferred() {
					n++
					return
				}
			}
			walk(v.Elem())
		case reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Struct:
			for i := range v.NumField() {
				if v.Type().Field(i).IsExported() {
					walk(v.Field(i))
				}
			}
		case reflect.Slice:
			for i := range v.Len() {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(tree))
	return n
}

// roundTrip encodes src's whole tree in d and decodes it again, without
// settling it.
func roundTrip(t *testing.T, src string, d syntax.Dialect) (parsed, got *syntax.File) {
	t.Helper()
	parsed, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	enc := treecodec.NewEncoder(src)
	if err := enc.File(parsed); err != nil {
		t.Fatal(err)
	}
	got, err = treecodec.NewDecoder(enc.Bytes(), src).File()
	if err != nil {
		t.Fatal(err)
	}
	return parsed, got
}

func decl(t *testing.T, f *syntax.File, i int) *syntax.FuncDecl {
	t.Helper()
	fd, ok := f.Stmts[i].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
	if !ok {
		t.Fatalf("statement %d is not a definition", i)
	}
	return fd
}

// big is a body long enough to be deferred, with a definition inside it.
var big = "big() {\n" + strings.Repeat("  print -r -- \"$1 ${(j:,:)@} $(( x + 1 ))\" >> /dev/null\n", 20) +
	"  inner() {\n" + strings.Repeat("    local a=$1; [[ $a == b* ]] && print $a\n", 12) + "  }\n}\n"

// TestABodyIsReadWhenItIsAskedFor is #6483's claim: a long body comes back
// unread, says where it ends without being read, and reads as the parser's
// body the first time anything asks — including a definition nested in it,
// which is deferred in its turn.
func TestABodyIsReadWhenItIsAskedFor(t *testing.T) {
	src := big + "small() { :; }\nbig\n"
	parsed, got := roundTrip(t, src, zsh.Dialect())
	fd, want := decl(t, got, 0), decl(t, parsed, 0)
	if !fd.Deferred() {
		t.Fatal("a body of a kilobyte was read with the tree")
	}
	if fd.BodyCommand != nil {
		t.Fatal("a deferred body is in BodyCommand as well")
	}
	if fd.End() != want.End() || !fd.HasBody() {
		t.Fatalf("End is %v, want %v", fd.End(), want.End())
	}
	if !fd.Deferred() {
		t.Fatal("End or HasBody read the body")
	}
	body := fd.Body()
	if fd.Deferred() {
		t.Fatal("still deferred after it was read")
	}
	if body != fd.Body() {
		t.Fatal("a second read made a second body")
	}
	if countDeferred(body) != 1 {
		t.Fatal("the definition inside the body was not deferred in its turn")
	}
	treecodec.Settle(got)
	if !reflect.DeepEqual(got, parsed) {
		t.Fatal("the body read late is not the parser's")
	}
	if syntax.PrintWith(fd.Body(), syntax.Layout{}) != syntax.PrintWith(want.Body(), syntax.Layout{}) {
		t.Fatal("the body prints differently")
	}
}

// TestASmallBodyIsReadWithTheTree: below MinDeferredBody the record costs
// more than the decode, so the body is where it stands.
func TestASmallBodyIsReadWithTheTree(t *testing.T) {
	_, got := roundTrip(t, "small() { :; }\n", zsh.Dialect())
	fd := decl(t, got, 0)
	if fd.Deferred() || fd.BodyCommand == nil {
		t.Fatal("a body of a few bytes was deferred")
	}
}

// TestABodyTheSideListsReachIsReadWithTheTree: a file's side lists may hold
// a node that is also inside a definition's body — a span in Substitutions
// carries the operand pointers of the word it came out of. A deferred body
// is read on its own and could not hand that node back as the one the side
// list holds, so such a body is written where it stands.
//
// The parser makes this shape rarely and in only some dialects, so the test
// makes it by hand: a long body, and a substitution span holding one of its
// parameter expansions.
func TestABodyTheSideListsReachIsReadWithTheTree(t *testing.T) {
	src := "f() {\n" + strings.Repeat("  echo \"${x:-y}\" >> /dev/null\n", 20) + "}\n"
	parsed, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	pe := firstParam(reflect.ValueOf(decl(t, parsed, 0).Body()))
	if pe == nil {
		t.Fatal("the body holds no parameter expansion, so it tests nothing")
	}
	parsed.Substitutions = append(parsed.Substitutions, syntax.Span{Param: pe})
	enc := treecodec.NewEncoder(src)
	if err := enc.File(parsed); err != nil {
		t.Fatal(err)
	}
	got, err := treecodec.NewDecoder(enc.Bytes(), src).File()
	if err != nil {
		t.Fatal(err)
	}
	if decl(t, got, 0).Deferred() {
		t.Fatal("a body the side lists point into was deferred")
	}
	if !sameSharing(parsed, got) || len(pointerPaths(got)) == 0 {
		t.Fatal("the tree came back sharing differently")
	}
	if firstParam(reflect.ValueOf(decl(t, got, 0).Body())) != got.Substitutions[len(got.Substitutions)-1].Param {
		t.Fatal("the side list's node is not the body's")
	}
}

// firstParam is the first parameter expansion under v, in field order.
func firstParam(v reflect.Value) *syntax.ParamExpr {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		if pe, ok := v.Interface().(*syntax.ParamExpr); ok {
			return pe
		}
		return firstParam(v.Elem())
	case reflect.Interface:
		if !v.IsNil() {
			return firstParam(v.Elem())
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				if pe := firstParam(v.Field(i)); pe != nil {
					return pe
				}
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			if pe := firstParam(v.Index(i)); pe != nil {
				return pe
			}
		}
	}
	return nil
}

// TestABodyIsReadOnceWhoeverAsks: a function's first call can come from any
// goroutine — a pipeline runs its stages on several — and every caller must
// get the one body.
func TestABodyIsReadOnceWhoeverAsks(t *testing.T) {
	_, got := roundTrip(t, big, zsh.Dialect())
	fd := decl(t, got, 0)
	var wg sync.WaitGroup
	bodies := make([]syntax.Command, 16)
	for i := range bodies {
		wg.Go(func() { bodies[i] = fd.Body() })
	}
	wg.Wait()
	for _, b := range bodies {
		if b == nil || b != bodies[0] {
			t.Fatal("two goroutines were handed different bodies")
		}
	}
}

// TestADecodedTreeEncodesAgain: a tree read back with bodies still deferred
// is written out with them, read through the accessor, and not as empty
// definitions.
func TestADecodedTreeEncodesAgain(t *testing.T) {
	parsed, got := roundTrip(t, big, zsh.Dialect())
	enc := treecodec.NewEncoder(big)
	if err := enc.File(got); err != nil {
		t.Fatal(err)
	}
	again, err := treecodec.NewDecoder(enc.Bytes(), big).File()
	if err != nil {
		t.Fatal(err)
	}
	treecodec.Settle(again)
	if !reflect.DeepEqual(again, parsed) {
		t.Fatal("a tree with deferred bodies was written out as something else")
	}
}
