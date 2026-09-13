// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// namedTest matches a Go test function name written into a field's verdict.
var namedTest = regexp.MustCompile(`\bTest[A-Z][A-Za-z0-9_]*`)

// TestAVerdictNamesATestThatExists.
//
// A standing verdict is the whole answer for an axis no corpus row can reach
// (#2058), and most of them say the same thing: the corpus has no terminal,
// no prompt and no login shell, so what pins the axis instead is a Go test.
// That sentence is a claim about the tree, and a claim about the tree that
// nothing checks is the kind that rots — a test renamed or deleted leaves the
// verdict reading exactly as it did while pointing at nothing.
//
// So every `TestSomething` a verdict names has to be a test function that is
// really there. It does not check that the named test *pins* the axis; that
// is a judgment. It checks the half a machine can: that the reader following
// the pointer arrives somewhere.
func TestAVerdictNamesATestThatExists(t *testing.T) {
	t.Parallel()
	notes, err := FieldNotes()
	if err != nil {
		t.Fatal(err)
	}
	have := testFunctions(t)
	seen := 0
	for field, n := range notes {
		for dialect, why := range n.Unpinned {
			for _, name := range namedTest.FindAllString(why, -1) {
				seen++
				if !have[name] {
					where := field
					if dialect != "" {
						where += " (" + dialect + ")"
					}
					t.Errorf("%s: the verdict names %s, and no test by that name is in the tree", where, name)
				}
			}
		}
	}
	// A pass from a regexp that matched nothing would say the same thing as a
	// pass from every name checking out, which is the shape this package has
	// been caught by before.
	if seen == 0 {
		t.Fatal("no verdict names a test, so this checked nothing")
	}
	t.Logf("%d test names checked across the standing verdicts", seen)
}

// testFunctions is every `func TestX(` in the module.
func testFunctions(t *testing.T) map[string]bool {
	t.Helper()
	// The same checkout the field comments were read from — fields_test.go
	// points it at the module root — so the two halves of this check cannot
	// be looking at different trees.
	root := moduleRoot
	decl := regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]*)\(`)
	out := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// The build directory is gitignored and may hold another
			// project's fetched suite; nothing there is ours to read.
			if name := d.Name(); name == "build" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range decl.FindAllStringSubmatch(string(b), -1) {
			out[m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
