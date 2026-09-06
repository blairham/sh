// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// What a served method does about the boundary.
const (
	// gated: the agent chose the path or the process, so it is an
	// interp.Action put to the gate before anything happens.
	gated = "gated"
	// recorded: the act is the *client's* rather than the agent's, so there
	// is nothing to refuse — but there is something to write down.
	recorded = "recorded"
	// inside: the method touches nothing outside this process, so there is
	// nothing for a boundary to be about.
	inside = "inside"
)

// reach is every inbound method Client.Handle serves, and what each does about
// the boundary. The reason is carried beside the answer because an exemption
// with no reason is how one gets copied.
var reach = map[string]struct {
	how, why string
}{
	"MethodReadTextFile": {gated,
		"a path the agent chose, opened by us: ActionOpen with Write false"},
	"MethodWriteTextFile": {gated,
		"a path the agent chose, written by us: ActionOpen with Write true"},
	"MethodCreateTerminal": {gated,
		"a program the agent chose, started by us: ActionExec with the argv"},
	"MethodKillTerminal": {gated,
		"the agent reaching a running process it chose: ActionSignal"},
	"MethodReleaseTerminal": {recorded,
		"the client ending something the client started — the protocol's only " +
			"way for an agent to say it is finished, and refusing it would leave " +
			"this client holding the process forever"},
	"MethodRequestPermission": {inside,
		"the agent asking about something it will do in its own process, which " +
			"our boundary does not cover: this is a person's answer, not a policy's"},
	"MethodCreateElicitation": {inside,
		"a question put to a person; nothing is opened, started or signaled"},
	"MethodTerminalOutput": {inside,
		"what a command we already started has written, out of our own buffer"},
	"MethodWaitForExit": {inside,
		"waiting for a command we already started, which was gated when it started"},
}

// TestEveryInboundMethodDeclaresWhatItDoesAboutTheBoundary is the half of #786
// that this repository can actually hold.
//
// The issue is that a coding agent which runs commands in its own process
// bypasses the gate, and nothing here can force it not to: an agent's own fork
// and exec is outside our boundary by construction. What *is* ours is the
// route an agent takes when it does ask — `fs/*` and `terminal/*` — and that
// route is nine hand-written call sites today. The tenth will be written by
// copying one of them, and a copy that drops `c.Boundary.…` still compiles,
// still answers the agent, and still passes every test about the nine.
//
// So the rule is enforced over the source rather than asserted per method.
// Adding a `case Method…:` to Client.Handle fails this test until the method
// is declared here, and declaring it `gated` fails until the handler reaches
// the boundary. A behavioral test cannot do this job — it can only be written
// about a method somebody already thought about, which is exactly the method
// that was not going to be forgotten.
func TestEveryInboundMethodDeclaresWhatItDoesAboutTheBoundary(t *testing.T) {
	t.Parallel()
	pkg := parsePackage(t)
	served := servedMethods(t, pkg, "Client")
	if len(served) < 9 {
		// A walk that silently found nothing passes for the wrong reason,
		// which is the failure mode of every assertion made over a traversal.
		t.Fatalf("the dispatch reads as %d methods; Client.Handle serves nine", len(served))
	}
	for _, m := range sortedKeys(served) {
		want, ok := reach[m]
		if !ok {
			t.Errorf("Client.Handle serves %s and nothing says what it does about the boundary.\n"+
				"\tAdd it to reach in this file: gated if the agent chose the path or the\n"+
				"\tprocess, recorded if the act is this client's own, inside if it touches\n"+
				"\tnothing outside this process — and say why.", m)
			continue
		}
		got := boundaryCalls(pkg, served[m])
		switch want.how {
		case gated:
			if len(got) == 0 {
				t.Errorf("%s is declared gated — %s — and %s reaches no boundary call.\n"+
					"\tAn agent's request that opens, starts or signals anything is an\n"+
					"\tinterp.Action first. Call c.Boundary.Open, .Exec or .Signal and\n"+
					"\trefuse when it answers no.", m, want.why, served[m])
			}
			if len(got) == 1 && got[0] == "Record" {
				t.Errorf("%s is declared gated and %s only records: a record is not a refusal",
					m, served[m])
			}
		case recorded:
			if !contains(got, "Record") {
				t.Errorf("%s is declared recorded — %s — and %s reaches no Boundary.Record",
					m, want.why, served[m])
			}
		case inside:
			if len(got) != 0 {
				t.Errorf("%s is declared as touching nothing outside this process — %s —\n"+
					"\tand %s reaches %v. One of the two is wrong.", m, want.why, served[m], got)
			}
		}
	}
	// And nothing is declared that is no longer served: a table that outlives
	// its subject is a rule nobody is following.
	for m := range reach {
		if _, ok := served[m]; !ok {
			t.Errorf("reach declares %s and Client.Handle does not serve it", m)
		}
	}
}

// The agent side serves three methods and touches the world through none of
// them: what a session does, the *interpreter* does, behind the gate driver
// wires into every Runner. A fourth method here is a fourth chance to reach
// past that, so it has to be a decision rather than an addition.
func TestTheAgentSideStillServesOnlyTheThreeMethodsThatNeedNoBoundary(t *testing.T) {
	t.Parallel()
	pkg := parsePackage(t)
	served := servedMethods(t, pkg, "Agent")
	want := []string{"MethodInitialize", "MethodNewSession", "MethodPrompt"}
	if got := sortedKeys(served); !equal(got, sorted(want)) {
		t.Errorf("Agent.Handle serves %v, want %v.\n"+
			"\tEverything a session does reaches the world through interp, whose gate\n"+
			"\tdriver installs. A method that reached past it would need internal/boundary\n"+
			"\tthe way the client side does — see reach in this file.", got, sorted(want))
	}
}

// servedMethods maps each method constant named in a `case` of the receiver's
// Handle to the handler that case calls.
//
// The dispatch is read rather than exercised because the question is what the
// source says: a handler reached by a case nobody wrote a test for is exactly
// the one this guard is for.
func servedMethods(t *testing.T, pkg []*ast.File, receiver string) map[string]string {
	t.Helper()
	handle := method(pkg, receiver, "Handle")
	if handle == nil {
		t.Fatalf("no %s.Handle in this package", receiver)
	}
	out := map[string]string{}
	ast.Inspect(handle.Body, func(n ast.Node) bool {
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
			if id, ok := e.(*ast.Ident); ok && strings.HasPrefix(id.Name, "Method") {
				out[id.Name] = called
			}
		}
		return true
	})
	return out
}

// boundaryCalls names every Boundary method the handler reaches, in the order
// found. Directly: a handler that delegated its gate call to a helper would
// read as reaching none, which is a false negative in the safe direction —
// the guard complains and somebody looks.
func boundaryCalls(pkg []*ast.File, handler string) []string {
	var got []string
	for _, f := range pkg {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name != handler || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				inner, ok := sel.X.(*ast.SelectorExpr)
				if ok && inner.Sel.Name == "Boundary" {
					got = append(got, sel.Sel.Name)
				}
				return true
			})
		}
	}
	return got
}

func method(pkg []*ast.File, receiver, name string) *ast.FuncDecl {
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

// A detector that has gone quiet passes over a broken tree and over a clean
// one alike, so it is handed the violation it exists to find.
//
// The tenth method, written by copying the ninth and dropping the gate call:
// it compiles, it answers the agent, and every test about the other nine still
// passes. This is the only thing that sees it.
func TestTheGuardSeesAHandlerThatForgetsTheBoundary(t *testing.T) {
	t.Parallel()
	pkg := parseSource(t, `package acp

func (c *Client) Handle(method string) any {
	switch method {
	case MethodReadTextFile:
		return c.readFile()
	case MethodListDirectory:
		return c.listDirectory()
	}
	return nil
}

func (c *Client) readFile() any {
	if !c.Boundary.Open(ctx, path, false) {
		return nil
	}
	return os.ReadFile(path)
}

func (c *Client) listDirectory() any { return os.ReadDir(path) }
`)
	served := servedMethods(t, pkg, "Client")
	if served["MethodListDirectory"] != "listDirectory" {
		t.Fatalf("the dispatch read as %v; the detector is not reading cases at all", served)
	}
	if got := boundaryCalls(pkg, "readFile"); !contains(got, "Open") {
		t.Errorf("the gated handler reads as reaching %v, want Open: the detector is blind", got)
	}
	if got := boundaryCalls(pkg, "listDirectory"); len(got) != 0 {
		t.Errorf("the handler that forgets reads as reaching %v, want nothing", got)
	}
	if _, declared := reach["MethodListDirectory"]; declared {
		t.Error("reach declares a method this package does not serve")
	}
}

// And the other direction, because a detector that answers "no boundary call"
// for everything would pass the test above by being broken.
func TestTheGuardSeesABoundaryCallBehindAHelperlessHandler(t *testing.T) {
	t.Parallel()
	pkg := parseSource(t, `package acp

func (c *Client) killTerminal() any {
	if !c.Boundary.Signal(ctx, pid, syscall.SIGKILL) {
		return nil
	}
	return nil
}

func (c *Client) releaseTerminal() any {
	c.Boundary.Record(ctx, action)
	return nil
}
`)
	if got := boundaryCalls(pkg, "killTerminal"); !equal(got, []string{"Signal"}) {
		t.Errorf("killTerminal reads as %v, want [Signal]", got)
	}
	if got := boundaryCalls(pkg, "releaseTerminal"); !equal(got, []string{"Record"}) {
		t.Errorf("releaseTerminal reads as %v, want [Record]", got)
	}
}

// parsePackage reads this package's own source, which `go test` runs in.
func parsePackage(t *testing.T) []*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		files = append(files, f)
	}
	if len(files) < 5 {
		t.Fatalf("read %d source files; this package has several", len(files))
	}
	return files
}

// parseSource is parsePackage over source a test wrote, which is how the
// detector is shown a violation the real tree does not contain.
func parseSource(t *testing.T, srcs ...string) []*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	for i, src := range srcs {
		f, err := parser.ParseFile(fset, "case"+string(rune('a'+i))+".go", src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing the test's own source: %v", err)
		}
		files = append(files, f)
	}
	return files
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
