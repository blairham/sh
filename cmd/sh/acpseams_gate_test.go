// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

// These two stayed here when the client direction moved to internal/acpboot
// (#2585). They are not about the protocol: they drive this binary's own
// installSeams and ownFlags, which are how `-deny`, `-trace-events` and
// `-policy` reach a gate before anything runs. A test of cmd/sh's flag pass
// belongs with cmd/sh.

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/acpboot"
)

// The claim the whole client side rests on, and the reason an agent's command
// line is interpreted rather than exec'd: the gate sees the commands *inside*
// the line.
//
// It has to be tested at full depth rather than at the seam, because every
// piece of it can be right while the composition is wrong — a Shell copied
// without its Gate would run the line perfectly and see nothing. The probe is
// `/bin/echo` rather than `echo`, which is exactly the distinction that makes
// this a test: `echo` is a builtin, so a line containing one execs nothing and
// a gate that was never wired would look identical to one that was.
func TestTheInterpreterRunsUnderTheSessionsGate(t *testing.T) {
	var trace bytes.Buffer
	sh, closer, err := installSeams(driver.Shell{}, ownFlags{traceEvents: true}, &trace)
	if err != nil {
		t.Fatalf("installing the seams: %v", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	var out bytes.Buffer
	code := acpboot.Interpreter(sh)(t.Context(), acp.TerminalCommand{
		Line: "/bin/echo inside-the-line", Env: os.Environ(),
	}, &out)

	if code != 0 {
		t.Errorf("status = %d, want the line to have run", code)
	}
	if got := out.String(); !strings.Contains(got, "inside-the-line") {
		t.Errorf("output = %q, want what the command wrote", got)
	}
	if got := trace.String(); !strings.Contains(got, "/bin/echo") {
		t.Errorf("the gate never saw the exec inside the line; trace = %q", got)
	}
}

// And a policy reaches it. This is the half that does not negotiate: the
// person's answer to the agent's permission request has already been given by
// the time a line is interpreted, and the policy still refuses.
//
// It is also the sharpest argument for interpreting rather than exec'ing. On
// the exec route a gate would be asked about one filename; here it is asked
// about every command the line runs, so a rule naming `/bin/echo` reaches an
// `/bin/echo` the agent buried in a pipeline.
func TestAPolicyRefusesACommandInsideAnAgentsLine(t *testing.T) {
	var trace bytes.Buffer
	sh, closer, err := installSeams(driver.Shell{},
		ownFlags{deny: []string{"exec:/bin/echo"}}, &trace)
	if err != nil {
		t.Fatalf("installing the seams: %v", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	var out bytes.Buffer
	acpboot.Interpreter(sh)(t.Context(), acp.TerminalCommand{
		Line: "true | /bin/echo refused-inside-a-pipeline", Env: os.Environ(),
	}, &out)

	if got := out.String(); strings.Contains(got, "refused-inside-a-pipeline") {
		t.Errorf("the denied command ran anyway: %q", got)
	}
	if got := out.String(); !strings.Contains(got, "refused") {
		t.Errorf("output = %q, want the refusal said out loud", got)
	}
}

// An `exec` inside the agent's line must not replace the process serving the
// connection, which is also the process that *is* the boundary: every gate
// consultation and every audit record for the session comes from it.
//
// An awkward test, because a regression does not produce an assertion
// failure. The binary is replaced partway through and the package exits with
// no test output at all — no `--- PASS`, no `--- FAIL`, and nothing from the
// logging below, which under `-v` always prints. So reaching the end of this
// function is itself part of what is being asserted.
