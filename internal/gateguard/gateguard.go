// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package gateguard reads a protocol front end's source and says, per served
// method, whether it reaches the boundary.
//
// It is the machinery behind #786's enforceable half. The issue is that a
// coding agent which runs commands in its own process bypasses the gate, and
// nothing here can force it not to: an agent's own fork and exec is outside
// our boundary by construction. What *is* ours is the route a peer takes when
// it does ask — `fs/*` and `terminal/*` over ACP, the terminal tools over MCP —
// and that route is a set of hand-written call sites. The next one will be
// written by copying one of them, and a copy that drops `Boundary.…` still
// compiles, still answers the peer, and still passes every test about the
// others.
//
// So the rule is enforced over the *source*. A behavioral test cannot do this
// job — it can only be written about a method somebody already thought about,
// which is exactly the method that was not going to be forgotten.
//
// It is a package rather than a file in one front end's tests because there
// are two front ends now and the guard is the last thing that should exist
// twice: a second copy of a detector drifts silently, and a detector that has
// gone quiet passes over a broken tree and a clean one alike.
package gateguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// What a served method does about the boundary.
const (
	// Gated: the peer chose the path or the process, so it is an interp.Action
	// put to the gate before anything happens.
	Gated = "gated"
	// Recorded: the act is the *front end's* rather than the peer's, so there
	// is nothing to refuse — but there is something to write down.
	Recorded = "recorded"
	// Inside: the method touches nothing outside this process, so there is
	// nothing for a boundary to be about.
	Inside = "inside"
)

// Reach is what one served method does about the boundary. The reason is
// carried beside the answer because an exemption with no reason is how one
// gets copied.
type Reach struct{ How, Why string }

// Parse reads the non-test Go source of each directory into one set of files.
//
// Several directories, because the handler and the machinery it calls no
// longer live together: the terminal verbs are in internal/termhost, shared by
// both front ends, so a guard that read only the front end's own directory
// would report every one of them as reaching nothing. That is a false negative
// in the unsafe direction, which is the one shape this must not have.
func Parse(dirs ...string) ([]*ast.File, error) {
	fset := token.NewFileSet()
	var files []*ast.File
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				return nil, fmt.Errorf("parsing %s: %w", name, err)
			}
			files = append(files, f)
		}
	}
	return files, nil
}

// ParseSource is Parse over source a test wrote, which is how the detector is
// shown a violation the real tree does not contain.
func ParseSource(srcs ...string) ([]*ast.File, error) {
	fset := token.NewFileSet()
	var files []*ast.File
	for i, src := range srcs {
		f, err := parser.ParseFile(fset, "case"+string(rune('a'+i))+".go", src, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parsing the test's own source: %w", err)
		}
		files = append(files, f)
	}
	return files, nil
}

// Served maps each constant named in a `case` of the receiver's dispatch to
// the function that case calls.
//
// The dispatch is read rather than exercised because the question is what the
// source says: a handler reached by a case nobody wrote a test for is exactly
// the one this guard is for. prefix names the constants that count — "Method"
// for a protocol whose verbs are methods, "Tool" for one whose verbs are tools.
func Served(pkg []*ast.File, receiver, dispatch, prefix string) map[string]string {
	fn := Method(pkg, receiver, dispatch)
	if fn == nil {
		return nil
	}
	out := map[string]string{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		cl, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		called := ""
		ast.Inspect(cl, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if _, isCall := sel.X.(*ast.Ident); isCall && called == "" {
				called = sel.Sel.Name
			}
			return true
		})
		for _, e := range cl.List {
			if id, ok := e.(*ast.Ident); ok && strings.HasPrefix(id.Name, prefix) {
				out[id.Name] = called
			}
		}
		return true
	})
	return out
}

// BoundaryCalls names every Boundary method the handler reaches, following
// calls into anything else the parsed source defines.
//
// It used to look only inside the handler itself, on the reasoning that a
// handler delegating its gate call to a helper would read as reaching none —
// a false negative in the safe direction, since the guard complains and
// somebody looks. That stopped being safe the moment the terminal verbs became
// one shared implementation: every one of them delegates now, so the guard
// would have failed on a correct tree and the fix somebody reached for would
// have been to declare them Inside. Following the call is the honest version,
// and it is what makes the guard say something about the shared machinery at
// all.
//
// Names, not types: the parsed set is one front end plus the machinery, and a
// call is followed when something in that set declares a function or method of
// that name. Two methods sharing a name are both followed, which can only
// widen what is found.
func BoundaryCalls(pkg []*ast.File, handler string) []string {
	decls := map[string][]*ast.FuncDecl{}
	for _, f := range pkg {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
				decls[fd.Name.Name] = append(decls[fd.Name.Name], fd)
			}
		}
	}
	var got []string
	seen := map[string]bool{}
	var walk func(name string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		for _, fd := range decls[name] {
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "Boundary" {
					got = append(got, sel.Sel.Name)
					return true
				}
				walk(sel.Sel.Name)
				return true
			})
		}
	}
	walk(handler)
	return got
}

// asking is every Boundary method that can answer *no*. The rest — Record and
// Failed — write something down and refuse nothing.
//
// A list of the ones that ask rather than of the ones that do not, because the
// two failure modes are not symmetric: a Boundary method added tomorrow and
// left off a deny-list would silently count as a refusal, and left off this
// one it makes the guard complain and somebody look. That is not hypothetical
// — the first version of this counted any Boundary call at all, and removing
// the gate from the shared create left the guard green, because the same
// function also reports a failed start through Boundary.Failed.
var asking = map[string]bool{
	"Exec":      true,
	"Signal":    true,
	"OpenFile":  true,
	"ReadFile":  true,
	"WriteFile": true,
	"Modify":    true,
	"ReadDir":   true,
	"Stat":      true,
}

// Asks reports whether any of the reached calls could have refused.
func Asks(got []string) bool {
	for _, v := range got {
		if asking[v] {
			return true
		}
	}
	return false
}

// Asking names the Boundary methods that answer a question, for a diagnostic
// that has to tell somebody what to reach for.
func Asking() []string {
	out := make([]string, 0, len(asking))
	for k := range asking {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Method finds a method on a pointer receiver.
func Method(pkg []*ast.File, receiver, name string) *ast.FuncDecl {
	for _, f := range pkg {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name != name || fd.Recv == nil || len(fd.Recv.List) != 1 {
				continue
			}
			star, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			if id, ok := star.X.(*ast.Ident); ok && id.Name == receiver {
				return fd
			}
		}
	}
	return nil
}

// SortedKeys is the served methods in a stable order, so a failure reads the
// same twice.
func SortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Contains reports whether a boundary call was reached.
func Contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
