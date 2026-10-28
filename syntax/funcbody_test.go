// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// bodyFieldReaders are the files allowed to name FuncDecl.BodyCommand: the
// parser that fills it in, the accessor that reads it, and the parse cache's
// test support that settles a decoded tree back into the parser's shape.
var bodyFieldReaders = []string{
	"syntax/ast.go",
	"syntax/funcbody.go",
	"syntax/parser.go",
	"internal/treecodec/settle.go",
	"internal/treecodec/deferredbody_test.go",
}

// readsOfTheBodyField lists every `x.BodyCommand` in one Go source file. A
// composite literal's `BodyCommand:` key is a construction and not a read,
// and is not a selector, so it is not listed.
func readsOfTheBodyField(fset *token.FileSet, name string, src any) ([]string, error) {
	f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "BodyCommand" {
			out = append(out, fset.Position(sel.Sel.Pos()).String())
		}
		return true
	})
	return out, nil
}

// TestNothingReadsADefinitionsBodyField holds the whole module to reading a
// definition's body through FuncDecl.Body (#6483). A tree read back from the
// parse cache keeps each long body encoded until it is asked for, and its
// BodyCommand is nil all that time, so one direct read anywhere is a function
// that runs nothing, or lists as empty, only when the cache served it — which
// no test of the parser would ever see.
//
// The name BodyCommand appears nowhere else in the tree, so a selector
// spelled that way is this field and needs no type information to say so.
func TestNothingReadsADefinitionsBodyField(t *testing.T) {
	// The instrument first: a read it must find, and a construction it must
	// not.
	fset := token.NewFileSet()
	got, err := readsOfTheBodyField(fset, "probe.go", `package p
func f(d *FuncDecl) any { _ = FuncDecl{BodyCommand: nil}; return d.BodyCommand }`)
	if err != nil || len(got) != 1 {
		t.Fatalf("the scan found %v (%v) in a file holding one read", got, err)
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("%s is not the module root: %v", root, err)
	}
	files, allowed := 0, 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "build" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		files++
		reads, err := readsOfTheBodyField(fset, path, nil)
		if err != nil {
			return err
		}
		if slices.Contains(bodyFieldReaders, rel) {
			if len(reads) > 0 {
				allowed++
			}
			return nil
		}
		for _, at := range reads {
			t.Errorf(
				"%s reads FuncDecl.BodyCommand; read the body through FuncDecl.Body, which decodes a body the parse cache deferred",
				at,
			)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A walk that found nothing to look at is not a clean tree.
	if files < 1000 || allowed < 3 {
		t.Fatalf("scanned %d files, %d of them allowed readers: the walk did not see the module", files, allowed)
	}
}
