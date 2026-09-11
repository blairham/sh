// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acpcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The graded properties.
//
// Each row is one claim docs/design/acp.md makes, turned into something a
// process can be wrong about. Two rules, both learned elsewhere in this tree:
//
// A row that can only pass is not a row. Where a claim is "this is refused",
// the row does the allowed case too and requires it to *differ* — a check that
// reports a refusal without ever having seen the same action succeed cannot
// tell a working gate from a shell that failed for its own reasons.
//
// And a row asserts on the side effect, not on the message. "The file does not
// exist afterwards" is enforcement; "the agent said refused" is a string, and a
// shell that printed it while writing the file anyway would pass.

// Row is one graded property.
type Row struct {
	Name   string
	Claim  string // why anyone should care, in one line
	Pass   bool
	Detail string
	Known  string // the issue that owns a known gap, if one does
}

// Result is a whole run.
type Result struct {
	Rows  []Row
	Agent string // the version the agent named itself
}

// Failed reports whether anything failed that no issue owns.
func (r Result) Failed() bool {
	for _, row := range r.Rows {
		if !row.Pass && row.Known == "" {
			return true
		}
	}
	return false
}

// check is one row's body: it gets a scratch directory and the agent binary.
type check struct {
	name  string
	claim string
	known string
	run   func(t *harness) (bool, string)
}

// harness is what a row is given: the binary, a directory of its own, and a
// context with the deadline the whole run shares.
type harness struct {
	ctx context.Context
	bin string
	dir string
}

// dial opens a connection with the given flags and answer policy, already
// through the handshake and with a session open.
func (t *harness) dial(args []string, answer func(Ask) string) (*Client, string, error) {
	c, err := Dial(t.bin, Options{Args: append(args, "-acp"), Dir: t.dir, Answer: answer})
	if err != nil {
		return nil, "", err
	}
	if _, err := c.Initialize(t.ctx); err != nil {
		_ = c.Close()
		return nil, "", err
	}
	s, err := c.NewSession(t.ctx, t.dir)
	if err != nil {
		_ = c.Close()
		return nil, "", err
	}
	return c, s, nil
}

// always answers every request with the same option.
func always(option string) func(Ask) string {
	return func(Ask) string { return option }
}

// Run grades the binary and returns the table.
//
// root is where each row's scratch directory is made; the caller owns it, so
// a run whose files are worth looking at can be pointed at somewhere that
// survives.
func Run(ctx context.Context, bin, root, self, only string) Result {
	var res Result
	all := checks()
	// The client rows need a second process that speaks ACP, and this binary
	// is it — re-executed with -as-agent. A run given no path to itself
	// grades the agent direction only, and says so by leaving the rows out
	// rather than by passing them.
	if self != "" {
		all = append(all, clientChecks(self)...)
	}
	for _, ch := range all {
		if only != "" && !strings.Contains(ch.name, only) {
			continue
		}
		dir := filepath.Join(root, ch.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			res.Rows = append(res.Rows, Row{Name: ch.name, Claim: ch.claim, Detail: err.Error(), Known: ch.known})
			continue
		}
		// Each row gets its own deadline. A hung agent is a failure of the
		// row rather than of the run, so the rest still report.
		rowCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		pass, detail := ch.run(&harness{ctx: rowCtx, bin: bin, dir: dir})
		cancel()
		res.Rows = append(res.Rows, Row{Name: ch.name, Claim: ch.claim, Pass: pass, Detail: detail, Known: ch.known})
	}
	return res
}

func checks() []check {
	return []check{{
		name:  "handshake",
		claim: "the shipped binary answers protocol version 1 and names itself",
		run: func(t *harness) (bool, string) {
			c, err := Dial(t.bin, Options{Args: []string{"-acp"}, Dir: t.dir})
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			in, err := c.Initialize(t.ctx)
			if err != nil {
				return false, err.Error()
			}
			if in.ProtocolVersion != 1 {
				return false, fmt.Sprintf("protocolVersion %d, want 1", in.ProtocolVersion)
			}
			if in.AgentInfo.Name == "" {
				return false, "agentInfo carried no name"
			}
			return true, fmt.Sprintf("%s %s, protocol 1", in.AgentInfo.Name, in.AgentInfo.Version)
		},
	}, {
		name:  "handshake-first",
		claim: "a session asked for before the handshake is refused, not served",
		run: func(t *harness) (bool, string) {
			c, err := Dial(t.bin, Options{Args: []string{"-acp"}, Dir: t.dir})
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			if _, err := c.NewSession(t.ctx, t.dir); err != nil {
				return true, "refused: " + err.Error()
			}
			return false, "session/new was served before initialize"
		},
	}, {
		name:  "turn",
		claim: "a prompt runs shell and streams what it printed back as session updates",
		run: func(t *harness) (bool, string) {
			c, s, err := t.dial(nil, nil)
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			stop, err := c.Prompt(t.ctx, s, "echo out-here; echo err-here >&2")
			if err != nil {
				return false, err.Error()
			}
			out, errOut := c.Output("stdout"), c.Output("stderr")
			if stop != "end_turn" {
				return false, "stopReason " + stop
			}
			if out != "out-here\n" {
				return false, fmt.Sprintf("stdout %q", out)
			}
			// The streams stay apart, which is the half an instrument that
			// merges them can never check again.
			if errOut != "err-here\n" {
				return false, fmt.Sprintf("stderr %q", errOut)
			}
			return true, "end_turn; stdout and stderr arrived on separate streams"
		},
	}, {
		name:  "stdout-is-protocol-only",
		claim: "nothing but JSON-RPC is ever written on the agent's standard output",
		run: func(t *harness) (bool, string) {
			c, s, err := t.dial(nil, nil)
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			// Everything that has any business writing to a descriptor: the
			// shell's own builtins, a child that inherits one, an explicit
			// redirection onto fd 1, and a diagnostic.
			script := `echo builtin; /bin/echo external; exec 3>&1; echo through-fd3 >&3; ` +
				`printf 'no-newline-at-end'; echo diag >&2; /bin/sh -c 'echo grandchild'`
			if _, err := c.Prompt(t.ctx, s, script); err != nil {
				return false, err.Error()
			}
			if bad := c.Impure(); len(bad) > 0 {
				return false, fmt.Sprintf("%d non-protocol line(s), first: %q", len(bad), bad[0])
			}
			return true, "every line the agent wrote parsed as a JSON-RPC message"
		},
	}, {
		name:  "gate-reaches-inside-eval",
		claim: "the write an eval hid is still put to the client — what a pipe cannot see",
		run: func(t *harness) (bool, string) {
			c, s, err := t.dial(nil, always(AllowOnce))
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			// Buried two levels down and built from a string, so that
			// nothing outside the interpreter could have read it off the
			// command line.
			script := `w=$(printf 'e%s' 'cho'); eval "$w hidden > $(printf 'buried')".txt`
			if _, err := c.Prompt(t.ctx, s, script); err != nil {
				return false, err.Error()
			}
			for _, a := range c.Asks() {
				if strings.Contains(a.Title, "buried.txt") {
					return true, "asked permission for: " + a.Title
				}
			}
			return false, fmt.Sprintf("no permission request named the file; asks=%v", c.Asks())
		},
	}, {
		name:  "refusal-is-enforced",
		claim: "reject and the write does not happen; allow and it does — the same script both ways",
		run: func(t *harness) (bool, string) {
			// The pair is the row. One half alone proves nothing: a file
			// that is missing after a refusal is also a file that is missing
			// because the shell fell over.
			run := func(answer, name string) (string, error) {
				c, s, err := t.dial(nil, always(answer))
				if err != nil {
					return "", err
				}
				defer func() { _ = c.Close() }()
				if _, err := c.Prompt(t.ctx, s, "echo written > "+name); err != nil {
					return "", err
				}
				status := ""
				if calls := c.ToolCalls(); len(calls) > 0 {
					status = c.Status(calls[0].ToolCallID)
				}
				return status, nil
			}
			rejected, err := run(RejectOnce, "rejected.txt")
			if err != nil {
				return false, err.Error()
			}
			allowed, err := run(AllowOnce, "allowed.txt")
			if err != nil {
				return false, err.Error()
			}
			_, rejErr := os.Stat(filepath.Join(t.dir, "rejected.txt"))
			allowedBody, allowErr := os.ReadFile(filepath.Join(t.dir, "allowed.txt"))
			switch {
			case !os.IsNotExist(rejErr):
				return false, "the rejected write happened anyway"
			case allowErr != nil:
				return false, "the allowed write did not happen: " + allowErr.Error()
			case strings.TrimSpace(string(allowedBody)) != "written":
				return false, fmt.Sprintf("allowed file holds %q", allowedBody)
			}
			return true, fmt.Sprintf("rejected: no file, tool call %s; allowed: file written, tool call %s",
				rejected, allowed)
		},
	}, {
		name:  "unknown-option-is-denial",
		claim: "a client that answers with an option we never offered fails closed",
		run: func(t *harness) (bool, string) {
			c, s, err := t.dial(nil, always("allow_once")) // an underscore: close, and not ours
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			if _, err := c.Prompt(t.ctx, s, "echo nope > invented.txt"); err != nil {
				return false, err.Error()
			}
			if _, err := os.Stat(filepath.Join(t.dir, "invented.txt")); !os.IsNotExist(err) {
				return false, "an unrecognized option id was taken as consent"
			}
			return true, `answered "allow_once" where the offer was "allow-once": refused`
		},
	}, {
		name:  "exec-is-gated",
		claim: "starting a program is put to the client, and its exit status comes back",
		run: func(t *harness) (bool, string) {
			c, s, err := t.dial(nil, always(AllowOnce))
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			if _, err := c.Prompt(t.ctx, s, "/bin/sh -c 'exit 7'"); err != nil {
				return false, err.Error()
			}
			asked := false
			for _, a := range c.Asks() {
				if a.Kind == "execute" {
					asked = true
				}
			}
			if !asked {
				return false, "no execute permission was requested"
			}
			for _, u := range c.Updates() {
				if u.Kind == "tool_call_update" && strings.Contains(string(u.Raw), `"exitStatus":7`) {
					return true, "asked before the exec, and reported exitStatus 7"
				}
			}
			return false, "the exec was gated but no exit status came back"
		},
	}, {
		name:  "signal-is-gated",
		claim: "a signal leaving this process is an action too: asked about, and refusable",
		run: func(t *harness) (bool, string) {
			// Signal 0 delivers nothing and still goes through kill(2), so
			// the row is about the gate rather than about killing anything.
			// A refused signal is EPERM, which `kill` reports as a status —
			// so the pair is a status and not a message, and no redirection
			// appears in the script: a `2>/dev/null` is itself a write the
			// gate would refuse first, and the row would then be measuring
			// the redirection it added.
			run := func(answer string) (string, []Ask, error) {
				c, s, err := t.dial(nil, always(answer))
				if err != nil {
					return "", nil, err
				}
				defer func() { _ = c.Close() }()
				if _, err := c.Prompt(t.ctx, s, "kill -0 $$; echo status=$?"); err != nil {
					return "", nil, err
				}
				return c.Output("stdout"), c.Asks(), nil
			}
			refused, asks, err := run(RejectOnce)
			if err != nil {
				return false, err.Error()
			}
			allowed, _, err := run(AllowOnce)
			if err != nil {
				return false, err.Error()
			}
			asked := ""
			for _, a := range asks {
				if strings.Contains(strings.ToLower(a.Title), "signal") {
					asked = a.Title
				}
			}
			switch {
			case asked == "":
				return false, fmt.Sprintf("no signal permission request; asks=%v", asks)
			case strings.TrimSpace(refused) != "status=1":
				return false, fmt.Sprintf("a refused signal reported %q, want status=1", strings.TrimSpace(refused))
			case strings.TrimSpace(allowed) != "status=0":
				return false, fmt.Sprintf("an allowed signal reported %q, want status=0", strings.TrimSpace(allowed))
			}
			return true, "asked permission for: " + asked + "; refused reports EPERM, allowed succeeds"
		},
	}, {
		name:  "policy-answers-before-the-client-is-asked",
		claim: "a path the command line already refused is never offered to the client to allow",
		run: func(t *harness) (bool, string) {
			// The point of the row: -deny is not a suggestion the client can
			// overrule. If the request reached the client at all, a client
			// that always allows would have undone the policy.
			target := filepath.Join(t.dir, "policy.txt")
			c, s, err := t.dial([]string{"-deny", target}, always(AllowAlways))
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			if _, err := c.Prompt(t.ctx, s, "echo nope > "+target); err != nil {
				return false, err.Error()
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				return false, "the denied path was written"
			}
			for _, a := range c.Asks() {
				if strings.Contains(a.Title, "policy.txt") {
					return false, "the policy's refusal was put to the client as a question: " + a.Title
				}
			}
			return true, "refused by policy, with no permission request sent"
		},
	}, {
		name:  "cancel",
		claim: "a turn a client cancels stops, and says so",
		run: func(t *harness) (bool, string) {
			c, s, err := t.dial(nil, always(AllowOnce))
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			done := make(chan struct {
				stop string
				err  error
			}, 1)
			go func() {
				stop, err := c.Prompt(t.ctx, s, "/bin/sleep 30; echo should-not-print")
				done <- struct {
					stop string
					err  error
				}{stop, err}
			}()
			// Wait for the turn to have actually started before canceling
			// it: canceling a turn that has not begun measures nothing.
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) && len(c.ToolCalls()) == 0 {
				time.Sleep(10 * time.Millisecond)
			}
			c.Cancel(s)
			select {
			case r := <-done:
				if r.err != nil {
					return false, r.err.Error()
				}
				if strings.Contains(c.Output("stdout"), "should-not-print") {
					return false, "the rest of the turn ran anyway"
				}
				// The protocol's own spelling, which is not ours to tidy —
				// acp.OutcomeCancelled carries the same exemption.
				if r.stop != "cancelled" { //nolint:misspell // the protocol's spelling
					return false, "stopReason " + r.stop
				}
				//nolint:misspell // the protocol's spelling
				return true, "stopReason cancelled, and nothing after the cancel ran"
			case <-t.ctx.Done():
				return false, "the turn never answered after cancel"
			}
		},
	}, {
		name:  "sessions-are-separate-shells",
		claim: "two sessions on one connection do not share a shell's state",
		run: func(t *harness) (bool, string) {
			c, first, err := t.dial(nil, always(AllowOnce))
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			second, err := c.NewSession(t.ctx, t.dir)
			if err != nil {
				return false, err.Error()
			}
			if first == second {
				return false, "the same session id came back twice"
			}
			if _, err := c.Prompt(t.ctx, first, "LEAK=yes"); err != nil {
				return false, err.Error()
			}
			if _, err := c.Prompt(t.ctx, second, "echo \"leak=[$LEAK]\""); err != nil {
				return false, err.Error()
			}
			if strings.Contains(c.Output("stdout"), "leak=[yes]") {
				return false, "a variable set in one session was visible in the other"
			}
			return true, "a variable set in " + first + " was not visible in " + second
		},
	}, {
		name:  "exited-session-refuses-the-next-prompt",
		claim: "a session whose shell has exited says so, rather than answering end_turn (#1804)",
		run: func(t *harness) (bool, string) {
			c, s, err := t.dial(nil, always(AllowOnce))
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			if _, err := c.Prompt(t.ctx, s, "exit 3"); err != nil {
				return false, "the exiting turn itself failed: " + err.Error()
			}
			stop, err := c.Prompt(t.ctx, s, "echo after-exit")
			if err != nil {
				return true, "refused: " + err.Error()
			}
			return false, fmt.Sprintf("answered %q to a prompt for a shell that had exited", stop)
		},
	}}
}

// The other direction's rows.
//
// Everything above grades the shell as an agent, which is the direction an
// editor uses. These grade it as a *client*: `sh -acp-connect CMD` launches an
// agent and is the environment it runs inside, and the claim docs/design/acp.md
// makes there is stronger than anything on the agent side — that a shell can
// enforce a policy on an agent that no editor can, because the file the agent
// reads is a file we open and the command it runs is a command we start.
//
// That claim is a pair of file-system facts, so the rows are a pair of
// file-system facts: the same agent, asking for the same thing, with and
// without the policy.
func clientChecks(self string) []check {
	return []check{{
		name:  "client-answers-an-agents-file-read",
		claim: "the shell reads a file on the agent's behalf and hands it over",
		run: func(t *harness) (bool, string) {
			target := filepath.Join(t.dir, "secret.txt")
			if err := os.WriteFile(target, []byte("the contents\n"), 0o600); err != nil {
				return false, err.Error()
			}
			r, err := driveAgent(t, self, "read", target, nil)
			if err != nil {
				return false, err.Error()
			}
			if r.Refused || !strings.Contains(r.Got, "the contents") {
				return false, fmt.Sprintf("the agent got %+v", r)
			}
			return true, "the agent asked for a file and the shell opened it: " + strings.TrimSpace(r.Got)
		},
	}, {
		name:  "client-refuses-an-agents-file-read-by-policy",
		claim: "a policy on the shell reaches the agent it is running — what no editor can do",
		run: func(t *harness) (bool, string) {
			// The same agent, the same request, one flag apart. Without the
			// pair this row would prove only that something failed.
			target := filepath.Join(t.dir, "denied.txt")
			if err := os.WriteFile(target, []byte("the contents\n"), 0o600); err != nil {
				return false, err.Error()
			}
			open, err := driveAgent(t, self, "read", target, nil)
			if err != nil {
				return false, err.Error()
			}
			denied, err := driveAgent(t, self, "read", target, []string{"-deny", target})
			if err != nil {
				return false, err.Error()
			}
			switch {
			case open.Refused:
				return false, "the agent was refused even without the policy: " + open.Error
			case !denied.Refused:
				return false, fmt.Sprintf("the policy did not reach the agent; it read %q", denied.Got)
			}
			// And the refusal must not be answerable by the blanket allow:
			// -acp-allow says yes to every question a person would be asked,
			// and a policy that -acp-allow can overrule is a suggestion.
			return true, "without -deny the agent read the file; with it: " + denied.Error
		},
	}, {
		name:  "client-runs-an-agents-command-through-the-shell",
		claim: "a command the agent asks for is run by this shell, so the gate reaches inside it",
		run: func(t *harness) (bool, string) {
			// A command *line*, not an argv: the agent sends text and the
			// shell interprets it, which is what puts the commands inside it
			// on the gate rather than only the program named first.
			r, err := driveAgent(t, self, "run", "echo from-the-agent", nil)
			if err != nil {
				return false, err.Error()
			}
			if r.Refused || !strings.Contains(r.Got, "from-the-agent") {
				return false, fmt.Sprintf("the agent got %+v", r)
			}
			return true, "the shell ran the agent's line and returned: " + strings.TrimSpace(r.Got)
		},
	}, {
		name:  "client-refuses-an-agents-command-by-policy",
		claim: "and the same policy refuses a program the agent asked the shell to run",
		run: func(t *harness) (bool, string) {
			line := "/bin/echo denied-program"
			open, err := driveAgent(t, self, "run", line, nil)
			if err != nil {
				return false, err.Error()
			}
			denied, err := driveAgent(t, self, "run", line, []string{"-deny", "/bin/echo"})
			if err != nil {
				return false, err.Error()
			}
			switch {
			case !strings.Contains(open.Got, "denied-program"):
				return false, "the program did not run even without the policy: " + open.Got
			case strings.Contains(denied.Got, "denied-program"):
				return false, "the policy did not reach the agent's command: " + denied.Got
			}
			return true, "with -deny /bin/echo the agent's command produced: " + strings.TrimSpace(denied.Got)
		},
	}}
}

// driveAgent runs one turn of `sh -acp-connect` against the scripted agent and
// returns what the agent was given or refused.
//
// The shell is the process under test and the agent is this binary again, so
// the only thing that varies between a pair of runs is the flags — which is
// what makes a pair worth comparing.
func driveAgent(t *harness, self, script, target string, flags []string) (AgentReport, error) {
	var r AgentReport
	out := filepath.Join(t.dir, script+"-report.json")
	_ = os.Remove(out)
	args := append(append([]string{}, flags...),
		// -acp-allow answers the questions a person would be asked, because
		// there is no person here. A policy refusal is not one of those
		// questions, which is the point of the row that uses both.
		"-acp-allow", "-acp-connect", self, "-as-agent", script)
	cmd := exec.CommandContext(t.ctx, t.bin, args...)
	cmd.Dir = t.dir
	cmd.Env = append(os.Environ(),
		envAgentScript+"="+script, envAgentOut+"="+out, envAgentTarget+"="+target)
	// One prompt, then end of input, which is how the prompt loop ends.
	cmd.Stdin = strings.NewReader("do the thing\n")
	shellOut, _ := cmd.CombinedOutput()
	b, readErr := os.ReadFile(out)
	if readErr != nil {
		return r, fmt.Errorf("the agent left no report (%v); the shell said: %s", readErr, shellOut)
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	return r, nil
}
