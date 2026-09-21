// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package mcp_test

import (
	"testing"

	"github.com/blairham/sh/internal/gateguard"
)

// reach is every tool this server dispatches, and what each does about the
// boundary. The reason is carried beside the answer because an exemption with
// no reason is how one gets copied.
//
// It is the same table internal/acp keeps for its methods, over the same
// machinery, and that is the point: the two front ends serve the same five
// verbs, so if one of them ever stops gating a create the guard says so on
// whichever side it happened.
var reach = map[string]gateguard.Reach{
	"ToolCreate": {
		How: gateguard.Gated,
		Why: "a program the client chose, started by us: ActionExec with the argv — " +
			"or, for a command line, every exec and open inside it",
	},
	"ToolKill": {
		How: gateguard.Gated,
		Why: "the client reaching a running process it chose: ActionSignal",
	},
	"ToolRelease": {
		How: gateguard.Recorded,
		Why: "the server ending something the server started — the only way a client " +
			"has of saying it is finished, and refusing it would leave this server " +
			"holding the process forever",
	},
	"ToolOutput": {
		How: gateguard.Inside,
		Why: "what a command we already started has written, out of our own buffer",
	},
	"ToolWait": {
		How: gateguard.Inside,
		Why: "waiting for a command we already started, which was gated when it started",
	},
}

// TestEveryToolDeclaresWhatItDoesAboutTheBoundary is #786's enforceable half,
// read over the second front end.
//
// Adding a `case Tool…:` to Server.call fails this test until the tool is
// declared here, and declaring it gated fails until the handler reaches a
// Boundary method that can answer *no* — through internal/termhost, which is
// why that directory is parsed beside this one.
func TestEveryToolDeclaresWhatItDoesAboutTheBoundary(t *testing.T) {
	t.Parallel()
	pkg, err := gateguard.Parse(".", "../termhost")
	if err != nil {
		t.Fatal(err)
	}
	served := gateguard.Served(pkg, "Server", "call", "Tool")
	if len(served) != 5 {
		// A walk that silently found nothing passes for the wrong reason,
		// which is the failure mode of every assertion made over a traversal.
		t.Fatalf("the dispatch reads as %d tools; Server.call serves five", len(served))
	}
	for _, name := range gateguard.SortedKeys(served) {
		want, ok := reach[name]
		if !ok {
			t.Errorf("Server.call serves %s and nothing says what it does about the boundary.\n"+
				"\tAdd it to reach in this file: gated if the client chose the path or the\n"+
				"\tprocess, recorded if the act is this server's own, inside if it touches\n"+
				"\tnothing outside this process — and say why.", name)
			continue
		}
		got := gateguard.BoundaryCalls(pkg, served[name])
		switch want.How {
		case gateguard.Gated:
			if !gateguard.Asks(got) {
				t.Errorf("%s is declared gated — %s — and %s reaches %v.\n"+
					"\tA client's request that opens, starts or signals anything is an\n"+
					"\tinterp.Action first: reach one of %v and refuse when it answers no.\n"+
					"\tRecord and Failed are not refusals.", name, want.Why, served[name], got, gateguard.Asking())
			}
		case gateguard.Recorded:
			if !gateguard.Contains(got, "Record") {
				t.Errorf("%s is declared recorded — %s — and %s reaches no Boundary.Record",
					name, want.Why, served[name])
			}
		case gateguard.Inside:
			if len(got) != 0 {
				t.Errorf("%s is declared as touching nothing outside this process — %s —\n"+
					"\tand %s reaches %v. One of the two is wrong.", name, want.Why, served[name], got)
			}
		}
	}
	for name := range reach {
		if _, ok := served[name]; !ok {
			t.Errorf("reach declares %s and Server.call does not serve it", name)
		}
	}
}
