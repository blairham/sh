// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package boundary_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The hole #942 named was not one bad line. It was an API shape: this package
// answered a bool and each caller then opened the file itself, so nine call
// sites each had to remember to be honest and the tenth would be written by
// copying one of them.
//
// The shape is fixed — Boundary makes the descriptor now, so there is nothing
// to ask permission for and then do differently — but that only holds while
// the front end goes through it. A package that reaches for os.Open again has
// the old arrangement back, and it compiles, and every test about the nine
// still passes.
//
// So the rule is enforced over the source. Every direct filesystem open in the
// four packages that hold a Boundary must be named here with a reason, and the
// reason has to be one of the two the design already accepts: the path is
// fixed and chosen by the front end, or the access is the policy apparatus
// itself, which cannot be subject to the policy it is reading.

// frontEnd is every package that holds a Boundary, relative to this one.
var frontEnd = []string{"../../driver", "../../repl", "../acp", "../blocks", "../../cmd/sh"}

// openers are the calls that open something on a path.
//
// Not os.Stat and its neighbors: a probe is answered without opening anything
// and is a metadata disclosure rather than a content one, which is the split
// docs/design/sandboxing.md already argues under "What this closes, and what
// it does not".
//
// os.ReadDir is on the list and is not a probe. A listing is the loudest
// oracle the filesystem has — it enumerates rather than answering one question
// — which is why ActionReadDir exists and why completion's two listings, which
// were the whole of #951, went through Boundary.ReadDir rather than staying
// written down here.
var openers = map[string]bool{
	"Open": true, "OpenFile": true, "Create": true, "CreateTemp": true,
	"ReadFile": true, "WriteFile": true, "ReadDir": true,
}

// exempt is every direct open the front end still makes, and why.
//
// An entry is a package, a function, and the reason the access is outside the
// boundary. Adding one is the deliberate act the list exists to force: a
// filesystem open in these packages that is not here fails this test, and the
// two acceptable reasons are the ones the design document already gives.
var exempt = map[string]string{
	"driver.controllingTerminal": "/dev/tty, opened to name a terminal in an ioctl rather " +
		"than to read anything. A fixed path the front end chose, and gating it would stop " +
		"^C and ^Z reaching commands while protecting no file. A redirection a script writes " +
		"to /dev/tty is an ordinary open and is gated.",
	"driver.readFdDir": "/dev/fd, listed to learn which descriptors this process was started " +
		"holding. The capabilities are in the table before a line is read, so there is nothing " +
		"a refusal could prevent — which is the argument ActionInherit already makes.",
	"repl.lookupTerminal": "/dev, listed to name this session's terminal. A fixed path, and " +
		"no content is read.",
	"repl.userHomes": "the account file, read to answer `~name` completion. A fixed path the " +
		"front end chose — the person types a prefix, never the path — and the standard " +
		"library has no call that enumerates accounts. It is the last of the three #951 " +
		"named and the one #951 did not close: gating this read would hide the prefix " +
		"listing while leaving `~name` itself resolving, because expandTilde falls back to " +
		"user.Lookup for a name the file does not hold and that is a library call no gate is " +
		"on. Closing it means gating account lookup as a whole, which is a different " +
		"question from a directory listing.",
	"repl.trim": "the history file's rewrite, on the path the append already passed the gate " +
		"on, through a temporary in the same directory. The shell's own scaffolding, which " +
		"ActionOpen's rule places outside the boundary.",
	"acp.newSession": "os.DevNull, so an agent this client starts does not inherit a terminal.",
	"main.openAudit": "the audit stream's own file. The apparatus is outside the boundary it " +
		"enforces — a policy that could hide its own log would be a policy nobody could check, " +
		"which docs/design/sandboxing.md states under `The apparatus is outside the boundary`.",
}

// TestEveryFrontEndOpenGoesThroughTheBoundaryOrSaysWhyNot.
func TestEveryFrontEndOpenGoesThroughTheBoundaryOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	var found []string
	for _, dir := range frontEnd {
		for _, call := range directOpens(t, dir) {
			found = append(found, call)
			if _, ok := exempt[call]; !ok {
				t.Errorf("%s opens a file through the os package and nothing says why.\n"+
					"\tUse Boundary.OpenFile, .ReadFile or .WriteFile, which consults the gate,\n"+
					"\topens, and asks the kernel what the open reached — or add %s to exempt\n"+
					"\tin this file with the reason it is outside the boundary.", call, call)
			}
		}
	}
	// A walk that found nothing passes for the wrong reason, which is the
	// failure mode of every assertion made over a traversal.
	if len(found) < len(exempt) {
		t.Fatalf("the walk found %d direct opens and the exemption list names %d; "+
			"the detector is not reading these packages", len(found), len(exempt))
	}
	// And nothing is exempted that no longer exists: a list that outlives its
	// subject is a rule nobody is following.
	sort.Strings(found)
	for call := range exempt {
		if sort.SearchStrings(found, call) == len(found) || found[sort.SearchStrings(found, call)] != call {
			t.Errorf("exempt names %s and no such open is there any more", call)
		}
	}
}

// directOpens names every `os.<opener>(` call in a package's non-test files,
// as "package.enclosingFunction".
//
// Files are read and parsed one at a time rather than through parser.ParseDir,
// which does not consider build tags and is deprecated for saying so. The
// package name is taken from the file, so a `_unix.go` and its `_windows.go`
// sibling both count — an exemption that held only on one platform is exactly
// the kind that goes unnoticed.
func directOpens(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			if opensDirectly(fd.Body) {
				out = append(out, fmt.Sprintf("%s.%s", f.Name.Name, fd.Name.Name))
			}
		}
	}
	return out
}

// opensDirectly reports whether a body calls one of the os package's openers.
func opensDirectly(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && openers[sel.Sel.Name] {
			found = true
		}
		return true
	})
	return found
}

// TestTheOpenGuardSeesACallItWasNotToldAbout. A detector that has gone quiet
// passes over a broken tree and a clean one alike, so it is handed the
// violation it exists to find.
func TestTheOpenGuardSeesACallItWasNotToldAbout(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", `package repl

func forgot() ([]byte, error) { return os.ReadFile(path) }

func remembered() ([]byte, error) { return b.ReadFile(ctx, path) }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	var opens []string
	for _, d := range f.Decls {
		fd := d.(*ast.FuncDecl)
		if opensDirectly(fd.Body) {
			opens = append(opens, fd.Name.Name)
		}
	}
	if len(opens) != 1 || opens[0] != "forgot" {
		t.Errorf("the detector read %v, want only the function that opens directly", opens)
	}
}
