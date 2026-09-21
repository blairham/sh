// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"go/ast"
	"testing"

	"github.com/blairham/sh/internal/gateguard"
)

// reach is every inbound method Client.Handle serves, and what each does about
// the boundary. The reason is carried beside the answer because an exemption
// with no reason is how one gets copied.
var reach = map[string]gateguard.Reach{
	"MethodReadTextFile": {
		How: gateguard.Gated,
		Why: "a path the agent chose, opened by us: ActionOpen with Write false",
	},
	"MethodWriteTextFile": {
		How: gateguard.Gated,
		Why: "a path the agent chose, written by us: ActionOpen with Write true",
	},
	"MethodCreateTerminal": {
		How: gateguard.Gated,
		Why: "a program the agent chose, started by us: ActionExec with the argv",
	},
	"MethodKillTerminal": {
		How: gateguard.Gated,
		Why: "the agent reaching a running process it chose: ActionSignal",
	},
	"MethodReleaseTerminal": {
		How: gateguard.Recorded,
		Why: "the client ending something the client started — the protocol's only " +
			"way for an agent to say it is finished, and refusing it would leave " +
			"this client holding the process forever",
	},
	"MethodRequestPermission": {
		How: gateguard.Inside,
		Why: "the agent asking about something it will do in its own process, which " +
			"our boundary does not cover: this is a person's answer, not a policy's",
	},
	"MethodCreateElicitation": {
		How: gateguard.Inside,
		Why: "a question put to a person; nothing is opened, started or signaled",
	},
	"MethodTerminalOutput": {
		How: gateguard.Inside,
		Why: "what a command we already started has written, out of our own buffer",
	},
	"MethodWaitForExit": {
		How: gateguard.Inside,
		Why: "waiting for a command we already started, which was gated when it started",
	},
}

// TestEveryInboundMethodDeclaresWhatItDoesAboutTheBoundary is the half of #786
// that this repository can actually hold. gateguard's doc comment is the
// argument; this is the table it is applied to.
//
// Adding a `case Method…:` to Client.Handle fails this test until the method
// is declared here, and declaring it gated fails until the handler reaches the
// boundary — through internal/termhost, which is why that directory is parsed
// beside this one.
func TestEveryInboundMethodDeclaresWhatItDoesAboutTheBoundary(t *testing.T) {
	t.Parallel()
	pkg := parsePackage(t)
	served := gateguard.Served(pkg, "Client", "Handle", "Method")
	if len(served) < 9 {
		// A walk that silently found nothing passes for the wrong reason,
		// which is the failure mode of every assertion made over a traversal.
		t.Fatalf("the dispatch reads as %d methods; Client.Handle serves nine", len(served))
	}
	for _, m := range gateguard.SortedKeys(served) {
		want, ok := reach[m]
		if !ok {
			t.Errorf("Client.Handle serves %s and nothing says what it does about the boundary.\n"+
				"\tAdd it to reach in this file: gated if the agent chose the path or the\n"+
				"\tprocess, recorded if the act is this client's own, inside if it touches\n"+
				"\tnothing outside this process — and say why.", m)
			continue
		}
		got := gateguard.BoundaryCalls(pkg, served[m])
		switch want.How {
		case gateguard.Gated:
			if !gateguard.Asks(got) {
				t.Errorf("%s is declared gated — %s — and %s reaches %v.\n"+
					"\tAn agent's request that opens, starts or signals anything is an\n"+
					"\tinterp.Action first: reach one of %v and refuse when it answers no.\n"+
					"\tRecord and Failed are not refusals.", m, want.Why, served[m], got, gateguard.Asking())
			}
		case gateguard.Recorded:
			if !gateguard.Contains(got, "Record") {
				t.Errorf("%s is declared recorded — %s — and %s reaches no Boundary.Record",
					m, want.Why, served[m])
			}
		case gateguard.Inside:
			if len(got) != 0 {
				t.Errorf("%s is declared as touching nothing outside this process — %s —\n"+
					"\tand %s reaches %v. One of the two is wrong.", m, want.Why, served[m], got)
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
	served := gateguard.Served(pkg, "Agent", "Handle", "Method")
	want := []string{"MethodInitialize", "MethodNewSession", "MethodPrompt"}
	if got := gateguard.SortedKeys(served); !equal(got, want) {
		t.Errorf("Agent.Handle serves %v, want %v.\n"+
			"\tEverything a session does reaches the world through interp, whose gate\n"+
			"\tdriver installs. A method that reached past it would need internal/boundary\n"+
			"\tthe way the client side does — see reach in this file.", got, want)
	}
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
	b, err := c.Boundary.ReadFile(ctx, path)
	if err != nil {
		return nil
	}
	return b
}

func (c *Client) listDirectory() any { return os.ReadDir(path) }
`)
	served := gateguard.Served(pkg, "Client", "Handle", "Method")
	if served["MethodListDirectory"] != "listDirectory" {
		t.Fatalf("the dispatch read as %v; the detector is not reading cases at all", served)
	}
	if got := gateguard.BoundaryCalls(pkg, "readFile"); !gateguard.Contains(got, "ReadFile") {
		t.Errorf("the gated handler reads as reaching %v, want ReadFile: the detector is blind", got)
	}
	if got := gateguard.BoundaryCalls(pkg, "listDirectory"); len(got) != 0 {
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
	if got := gateguard.BoundaryCalls(pkg, "killTerminal"); !equal(got, []string{"Signal"}) {
		t.Errorf("killTerminal reads as %v, want [Signal]", got)
	}
	if got := gateguard.BoundaryCalls(pkg, "releaseTerminal"); !equal(got, []string{"Record"}) {
		t.Errorf("releaseTerminal reads as %v, want [Record]", got)
	}
}

// And the third direction, which is the one this refactor created: a handler
// whose gate call is in the *machinery it delegates to* rather than in its own
// body. A guard that stopped at the handler would read this as reaching
// nothing, which is the false negative that would have had somebody declare
// the terminal verbs Inside.
func TestTheGuardFollowsTheCallIntoTheSharedMachinery(t *testing.T) {
	t.Parallel()
	pkg := parseSource(t, `package acp

func (c *Client) createTerminal() any { return c.terminals().Create(ctx, req) }
`, `package termhost

func (h *Host) Create(ctx context.Context, req Request) (string, error) {
	if !h.Boundary.Exec(ctx, req.Command, argv) {
		return "", ErrRefused
	}
	return "", nil
}
`)
	if got := gateguard.BoundaryCalls(pkg, "createTerminal"); !gateguard.Contains(got, "Exec") {
		t.Errorf("the delegating handler reads as reaching %v, want Exec", got)
	}
}

// parsePackage reads this package's source and the shared machinery's, which
// is where the terminal verbs' gate calls now are.
func parsePackage(t *testing.T) []*ast.File {
	t.Helper()
	files, err := gateguard.Parse(".", "../termhost")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 5 {
		t.Fatalf("read %d source files; these packages have several", len(files))
	}
	return files
}

func parseSource(t *testing.T, srcs ...string) []*ast.File {
	t.Helper()
	files, err := gateguard.ParseSource(srcs...)
	if err != nil {
		t.Fatal(err)
	}
	return files
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
