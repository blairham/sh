// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package treecodec_test

import (
	"encoding/binary"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/internal/testenv"
	"github.com/blairham/sh/internal/treecodec"
	"github.com/blairham/sh/syntax"
)

// dialects are the grammars a tree is read in, each of which reaches node
// kinds and fields the others do not.
func dialects() map[string]syntax.Dialect {
	return map[string]syntax.Dialect{
		"core": syntax.Core(), "posix": syntax.POSIX(),
		"bash": bash.Dialect(), "zsh": zsh.Dialect(), "ksh": ksh.Dialect(), "dash": dash.Dialect(),
	}
}

// texts is every piece of shell the repository holds: the corpus, our own
// suite, the function library, and the preludes.
func texts(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, c := range oracle.Corpus {
		out = append(out, c.Snippet)
	}
	root := filepath.Join("..", "..", "share")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err == nil && !strings.ContainsRune(string(b[:min(len(b), 512)]), 0) {
			out = append(out, string(b))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, bash.Prelude(), zsh.Prelude(), ksh.Prelude())
	if len(out) < 4000 {
		t.Fatalf("only %d texts: the corpora were not found", len(out))
	}
	return out
}

// dialectNames are the dialects' names in a fixed order, so that each is given
// the same share of the texts on every run — see textsFor.
func dialectNames() []string {
	return slices.Sorted(maps.Keys(dialects()))
}

// textsFor is the texts the k-th grammar reads: all of them, except under the
// race detector, where it is every raceStride()-th text starting at k.
//
// TestATreeComesBackAsTheParserMadeIt reads the texts this way. It runs in one
// goroutine, so the detector has nothing to compare in it and can find
// nothing, and it cost 49s of this package's 69s on a hosted ubuntu runner
// (#7290). Every run without -race — CI's macOS leg, `make test` — still reads
// every text in every grammar, so the codec's claim is proved whole on every
// pull request. TestOnlyTheListedFieldsShare does not: the sharing its control
// needs is in a handful of texts, and a sample misses them.
func textsFor(all []string, k int) []string {
	if !testenv.RaceDetector {
		return all
	}
	var out []string
	for i := k; i < len(all); i += raceStride() {
		out = append(out, all[i])
	}
	return out
}

// raceStride is how far apart the texts are that a grammar reads under the
// race detector, and 1 without it. Wider than the number of grammars, so that
// each reads texts none of the others does.
func raceStride() int {
	if testenv.RaceDetector {
		return 30
	}
	return 1
}

// TestATreeComesBackAsTheParserMadeIt is the codec's whole claim: every tree
// the parser makes of every text in the repository, in every grammar, read
// back equal to itself — and sharing what it shared.
func TestATreeComesBackAsTheParserMadeIt(t *testing.T) {
	all := texts(t)
	trees, refused, deferred := 0, 0, 0
	for k, name := range dialectNames() {
		d := dialects()[name]
		for _, src := range textsFor(all, k) {
			p := syntax.NewParser(src, d)
			for {
				f, ok := p.NextLine()
				if !ok {
					break
				}
				enc := treecodec.NewEncoder(src)
				if err := enc.File(f); err != nil {
					if !errors.Is(err, treecodec.ErrUncacheable) {
						t.Fatalf("%s: %v", name, err)
					}
					refused++
					continue
				}
				got, err := treecodec.NewDecoder(enc.Bytes(), src).File()
				if err != nil {
					t.Fatalf("%s: decoding %q: %v", name, src, err)
				}
				deferred += countDeferred(got)
				treecodec.Settle(got)
				if !reflect.DeepEqual(got, f) {
					t.Fatalf("%s: the tree of %q came back different", name, src)
				}
				if !sameSharing(f, got) {
					t.Fatalf("%s: the tree of %q came back sharing differently", name, src)
				}
				trees++
			}
		}
	}
	// The refusals are the trees holding a deferred refusal, which a cache
	// does not keep. They are few; if they were most, the cache would be
	// keeping nothing and this test would be passing on nothing.
	if trees < 50000/raceStride() || refused > trees/20 {
		t.Fatalf("%d trees carried and %d refused", trees, refused)
	}
	// And bodies came back deferred, so the comparison above covered the
	// read that happens when a function is first called (#6483).
	if deferred < 1000/raceStride() {
		t.Fatalf("only %d bodies came back deferred", deferred)
	}
}

// sameSharing reports whether two trees share pointers in the same places: a
// node reached twice in one is reached twice in the other, at the same paths.
func sameSharing(a, b *syntax.File) bool {
	pa, pb := pointerPaths(a), pointerPaths(b)
	return reflect.DeepEqual(pa, pb)
}

// pointerPaths lists, for every pointer reached more than once, the order in
// which each reach happened — the same numbers for two trees that share alike.
func pointerPaths(f *syntax.File) [][]int {
	seen := map[uintptr]int{}
	order := 0
	groups := map[int][]int{}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() {
				return
			}
			order++
			if first, ok := seen[v.Pointer()]; ok {
				groups[first] = append(groups[first], order)
				return
			}
			seen[v.Pointer()] = order
			walk(v.Elem())
		case reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Struct:
			for i := range v.NumField() {
				walk(v.Field(i))
			}
		case reflect.Slice:
			for i := range v.Len() {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(f))
	var out [][]int
	for first, rest := range groups {
		out = append(out, append([]int{first}, rest...))
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// TestOnlyTheListedFieldsShare holds the parser to what the encoder
// assumes: a node reached twice in a tree is one its side lists hold, or one
// of the few fields treecodec lists as shared. A statement that shared a node
// anywhere else would be read back as two equal nodes, which compares equal
// and is not the same tree — TestATreeComesBackAsTheParserMadeIt would fail on
// it, and this names the field.
func TestOnlyTheListedFieldsShare(t *testing.T) {
	all := texts(t)
	listed := map[string]bool{"SubscriptFlags.Arg": true, "SubscriptEnd.Text": true}
	sharing := 0
	for name, d := range dialects() {
		for _, src := range all {
			p := syntax.NewParser(src, d)
			for {
				f, ok := p.NextLine()
				if !ok {
					break
				}
				for _, pair := range sharedPaths(&syntax.File{Stmts: f.Stmts}) {
					if !listed[pair[0]] || !listed[pair[1]] {
						t.Fatalf("%s: %s and %s share a node in %q: list them in treecodec's sharedFields",
							name, pair[0], pair[1], src)
					}
					sharing++
				}
			}
		}
	}
	// The control: the listed fields really do share in the corpus, so the
	// walk above can see sharing when it is there.
	if sharing == 0 {
		t.Fatal("nothing shared anything: the walk cannot see sharing")
	}
}

// sharedPaths is, for each pointer reached twice, the field it was first
// reached through and the field it was reached through again.
func sharedPaths(f *syntax.File) [][2]string {
	first := map[uintptr]string{}
	var out [][2]string
	var walk func(v reflect.Value, field string)
	walk = func(v reflect.Value, field string) {
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() {
				return
			}
			if at, ok := first[v.Pointer()]; ok {
				out = append(out, [2]string{at, field})
				return
			}
			first[v.Pointer()] = field
			walk(v.Elem(), field)
		case reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem(), field)
			}
		case reflect.Struct:
			for i := range v.NumField() {
				walk(v.Field(i), v.Type().Name()+"."+v.Type().Field(i).Name)
			}
		case reflect.Slice:
			for i := range v.Len() {
				walk(v.Index(i), field)
			}
		}
	}
	walk(reflect.ValueOf(f), "File")
	return out
}

// TestTheRegistryNamesEveryNode reads the syntax package's source for every
// type that implements one of the tree's interfaces, and requires each to be
// in the registry. A node kind added without a line there makes its trees
// uncacheable, which is safe and slow; this says so the day it happens.
func TestTheRegistryNamesEveryNode(t *testing.T) {
	fset := token.NewFileSet()
	paths, err := filepath.Glob(filepath.Join("..", "..", "syntax", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	markers := map[string]bool{"exprNode": true, "commandNode": true, "condNode": true, "arithNode": true}
	var want []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !markers[fn.Name.Name] {
				continue
			}
			star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			if id, ok := star.X.(*ast.Ident); ok {
				want = append(want, "*syntax."+id.Name)
			}
		}
	}
	var have []string
	for _, ty := range treecodec.Registry() {
		have = append(have, ty.String())
	}
	sort.Strings(want)
	sort.Strings(have)
	if len(want) < 30 {
		t.Fatalf("found only %d node kinds in syntax: the scan is broken", len(want))
	}
	if !slices.Equal(want, have) {
		t.Errorf("the registry is not the node kinds\nsyntax:   %v\nregistry: %v", want, have)
	}
}

// TestDamagedBytesAreRefusedNotPanicked hands the decoder every prefix of a
// real encoding and a copy with each byte changed: it may refuse, and it may
// read a different tree — the store's checksum is what keeps damage away from
// it — but it must never panic out of the shell.
func TestDamagedBytesAreRefusedNotPanicked(t *testing.T) {
	src := zsh.Prelude()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	enc := treecodec.NewEncoder(src)
	if err := enc.File(f); err != nil {
		t.Fatal(err)
	}
	good := enc.Bytes()
	refused := 0
	step := max(1, len(good)/4000)
	for n := 0; n < len(good); n += step {
		if _, err := treecodec.NewDecoder(good[:n], src).File(); err != nil {
			refused++
		}
	}
	for i := 0; i < len(good); i += step {
		bad := slices.Clone(good)
		bad[i] ^= 0xff
		_, _ = treecodec.NewDecoder(bad, src).File()
	}
	if refused == 0 {
		t.Fatal("no prefix was refused: the decoder is not checking its input")
	}
}

// TestADialectsFingerprintSeesEveryAnswer moves every grammar setting the
// dialect holds, one at a time, and requires each move to change the
// fingerprint — which is the whole of its claim to stand for the grammar.
func TestADialectsFingerprintSeesEveryAnswer(t *testing.T) {
	base := zsh.Dialect()
	sum, ok := treecodec.DialectFingerprint(&base)
	if !ok {
		t.Fatal("the dialect cannot be fingerprinted")
	}
	v := reflect.ValueOf(&base).Elem()
	moved := 0
	for i := range v.NumField() {
		d := zsh.Dialect()
		fv := reflect.ValueOf(&d).Elem().Field(i)
		switch fv.Kind() {
		case reflect.Bool:
			fv.SetBool(!fv.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fv.SetInt(fv.Int() + 1)
		case reflect.Uint8:
			fv.SetUint(fv.Uint() + 1)
		case reflect.String:
			fv.SetString(fv.String() + "x")
		case reflect.Map:
			m := reflect.MakeMap(fv.Type())
			for _, k := range fv.MapKeys() {
				m.SetMapIndex(k, fv.MapIndex(k))
			}
			key := reflect.New(fv.Type().Key()).Elem()
			if key.Kind() == reflect.String {
				key.SetString("not-a-word-anyone-uses")
			} else {
				key.SetUint(250)
			}
			val := reflect.New(fv.Type().Elem()).Elem()
			val.SetBool(true)
			m.SetMapIndex(key, val)
			fv.Set(m)
		case reflect.Func:
			if !fv.IsNil() {
				fv.Set(reflect.Zero(fv.Type()))
			} else {
				fv.Set(reflect.MakeFunc(fv.Type(), func(args []reflect.Value) []reflect.Value {
					out := make([]reflect.Value, fv.Type().NumOut())
					for i := range out {
						out[i] = reflect.Zero(fv.Type().Out(i))
					}
					return out
				}))
			}
		default:
			t.Fatalf("%s is a %s, which this test cannot move", v.Type().Field(i).Name, fv.Kind())
		}
		got, ok := treecodec.DialectFingerprint(&d)
		if !ok {
			t.Fatal("the dialect cannot be fingerprinted")
		}
		if got == sum {
			t.Errorf("moving %s did not move the fingerprint", v.Type().Field(i).Name)
		}
		moved++
	}
	if moved < 300 {
		t.Fatalf("moved only %d settings", moved)
	}
}

// TestADamagedCountAllocatesNothing gives the decoder a tree whose count of
// nodes has been changed to millions. The count says how much to allocate
// before anything is read, so a decoder that believed it would allocate
// gigabytes on the strength of one bad byte; it must refuse, having
// allocated nothing to speak of.
func TestADamagedCountAllocatesNothing(t *testing.T) {
	src := "echo hello world\n"
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	enc := treecodec.NewEncoder(src)
	if err := enc.File(f); err != nil {
		t.Fatal(err)
	}
	good := enc.Bytes()
	// The head is a count of types and then each type's id and count.
	types, n := binary.Uvarint(good)
	id, m := binary.Uvarint(good[n:])
	_, k := binary.Uvarint(good[n+m:])
	if types == 0 || n <= 0 || m <= 0 || k <= 0 {
		t.Fatal("the encoding has no head to damage")
	}
	bad := binary.AppendUvarint(nil, types)
	bad = binary.AppendUvarint(bad, id)
	bad = binary.AppendUvarint(bad, 4<<20)
	bad = append(bad, good[n+m+k:]...)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = treecodec.NewDecoder(bad, src).File()
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Fatal("a count of millions in a tree of three words was believed")
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 1<<20 {
		t.Fatalf("refusing it allocated %d bytes", grew)
	}
}
