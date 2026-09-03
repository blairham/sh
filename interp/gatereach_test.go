// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The gate is consulted however the interpreter is re-entered.
//
// This is the package's own claim about itself: a policy enforced outside the
// interpreter does not reach inside it, because eval, source, command
// substitution and subshells all re-enter with their own input, so the first
// eval walks around anything that is not at the point of execution.
//
// Nothing else grades it. No binary sets a Gate, so the conformance harness
// cannot see this seam at all, and a hole in it would look exactly like a
// shell that works.
func TestTheGateSeesEveryWayIn(t *testing.T) {
	dir := t.TempDir()
	sourced := filepath.Join(dir, "sourced.sh")
	if err := os.WriteFile(sourced, []byte("/bin/echo from-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src string }{
		{"a plain command", "/bin/echo hi"},
		{"eval", `eval '/bin/echo hi'`},
		{"source", ". " + sourced},
		{"command substitution", "x=$(/bin/echo hi)"},
		{"backquotes", "x=`/bin/echo hi`"},
		{"a subshell", "( /bin/echo hi )"},
		{"a group", "{ /bin/echo hi; }"},
		{"a background job", "/bin/echo hi & wait"},
		{"a function body", "f() { /bin/echo hi; }; f"},
		{"a loop body", "for i in 1; do /bin/echo hi; done"},
		{"a case arm", "case x in x) /bin/echo hi;; esac"},
		{"the right of &&", "true && /bin/echo hi"},
		{"through the command builtin", "command /bin/echo hi"},
		{"a here-document's expansion", "/bin/cat <<EOF\n$(/bin/echo hi)\nEOF"},
		{"a process substitution", "/bin/cat <(/bin/echo hi)"},
		{"a trap firing at exit", "trap '/bin/echo hi' EXIT; exit 0"},
		{"an expansion in an assignment", "x=$(/bin/echo hi); :"},
		{"an expansion inside arithmetic", "x=$(( $(/bin/echo 1) + 1 ))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Guarded: a background job asks the gate from its own
			// goroutine, which is the contract and is why this is a mutex
			// and not a plain bool.
			var mu sync.Mutex
			var seen bool
			sem := PosixSemantics()
			r := &Runner{
				Semantics: &sem,
				Stdout:    &strings.Builder{}, Stderr: &strings.Builder{},
				Gate: GateFunc(func(_ context.Context, a Action) Decision {
					mu.Lock()
					defer mu.Unlock()
					if a.Kind == ActionExec && filepath.Base(a.Path) == "echo" {
						seen = true
					}
					return Allow
				}),
			}
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if !seen {
				t.Errorf("the gate was never asked about the command in %q", tc.src)
			}
		})
	}
}

// A pipeline asks about both halves, which is worth its own case: each is a
// process of its own and one of them is not the one being waited for.
func TestTheGateSeesBothHalvesOfAPipeline(t *testing.T) {
	// Each half runs on its own goroutine, so the record of what was asked
	// is shared and has to be guarded — the contract on Gate, demonstrated.
	var mu sync.Mutex
	var seen []string
	sem := PosixSemantics()
	r := &Runner{
		Semantics: &sem,
		Stdout:    &strings.Builder{}, Stderr: &strings.Builder{},
		Gate: GateFunc(func(_ context.Context, a Action) Decision {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, filepath.Base(a.Path))
			return Allow
		}),
	}
	f, err := syntax.Parse("/bin/echo hi | /bin/cat", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Errorf("gate saw %v, want both halves", seen)
	}
}

// The expansion a caller can ask for directly goes through it too.
//
// That entry point is how a prompt with a command substitution in it is drawn
// and how `ENV=$HOME/.shrc` is turned into a path — both of them run commands
// on behalf of a caller who never wrote a script.
func TestTheGateSeesAnExpansionAskedForDirectly(t *testing.T) {
	var seen []string
	sem := PosixSemantics()
	r := &Runner{
		Semantics: &sem,
		Stdout:    &strings.Builder{}, Stderr: &strings.Builder{},
		Gate: GateFunc(func(_ context.Context, a Action) Decision {
			seen = append(seen, filepath.Base(a.Path))
			return Deny
		}),
	}
	if got := r.Expand("[$(/bin/echo hi)]"); got != "[]" {
		t.Errorf("Expand gave %q, want the refused command to have produced nothing", got)
	}
	if len(seen) != 1 {
		t.Errorf("gate saw %v, want the command inside the expansion", seen)
	}
}
