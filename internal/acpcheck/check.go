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
	"slices"
	"sort"
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
	ctx     context.Context
	bin     string
	dir     string
	dialect string
}

// args puts the dialect in front of a row's own flags, so that every way a
// row has of starting the shell starts the same shell. Empty means the
// binary's default, which is what `-dialect` being absent has always meant.
//
// It returns a fresh slice: rows build on the result, and a shared backing
// array is how one row's flags end up on another's command line.
func (t *harness) args(extra ...string) []string {
	var args []string
	if t.dialect != "" {
		args = append(args, "-dialect", t.dialect)
	}
	return append(args, extra...)
}

// dialectOr is args for a row that needs a particular dialect when the run
// did not name one. The run's choice wins: a row that forces bash on a zsh
// run would be grading a shell nobody asked about.
func (t *harness) dialectOr(fallback string) []string {
	if t.dialect == "" {
		return []string{"-dialect", fallback}
	}
	return t.args()
}

// dial opens a connection with the given flags and answer policy, already
// through the handshake and with a session open.
func (t *harness) dial(args []string, answer func(Ask) string) (*Client, string, error) {
	c, err := Dial(t.bin, Options{Args: t.args(append(args, "-acp")...), Dir: t.dir, Answer: answer})
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

// sameLines reports whether two runs said the same things, disregarding the
// order they came in. The `-c` route's streams are interleaved as they were
// written and a session's arrive as two named streams, so comparing the two
// as text would report a difference that is only an artifact of how each
// route reports itself.
func sameLines(a, b string) bool {
	split := func(s string) []string {
		var out []string
		for _, l := range strings.Split(s, "\n") {
			if l = strings.TrimRight(l, "\r"); l != "" {
				out = append(out, unprefixed(l))
			}
		}
		sort.Strings(out)
		return out
	}
	return slices.Equal(split(a), split(b))
}

// unprefixed drops the name a shell puts in front of its own diagnostics.
//
// The two routes are entitled to disagree about that name and only that name:
// a `-c` run is invoked by path and says so, the way every shell does, while
// the one behind a session calls itself `sh`. Comparing the lines whole made
// the row report that difference as two different shells, which is who was
// speaking rather than what was said.
func unprefixed(line string) string {
	head, rest, ok := strings.Cut(line, ": ")
	if !ok || strings.ContainsAny(head, " \t") {
		return line
	}
	return rest
}

// always answers every request with the same option.
func always(option string) func(Ask) string {
	return func(Ask) string { return option }
}

// Config is what a run is given beside its context.
//
// Root is where each row's scratch directory is made; the caller owns it, so
// a run whose files are worth looking at can be pointed at somewhere that
// survives. Dialect is which shell the binary should be — empty for its
// default, which for the multi-call binary is the core.
type Config struct {
	Bin     string
	Root    string
	Self    string
	Only    string
	Dialect string
}

// Run grades the binary and returns the table.
func Run(ctx context.Context, cfg Config) Result {
	var res Result
	all := checks()
	// The client rows need a second process that speaks ACP, and this binary
	// is it — re-executed with -as-agent. A run given no path to itself
	// grades the agent direction only, and says so by leaving the rows out
	// rather than by passing them.
	if cfg.Self != "" {
		all = append(all, clientChecks(cfg.Self)...)
	}
	for _, ch := range all {
		if cfg.Only != "" && !strings.Contains(ch.name, cfg.Only) {
			continue
		}
		dir := filepath.Join(cfg.Root, ch.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			res.Rows = append(res.Rows, Row{Name: ch.name, Claim: ch.claim, Detail: err.Error(), Known: ch.known})
			continue
		}
		// Each row gets its own deadline. A hung agent is a failure of the
		// row rather than of the run, so the rest still report.
		rowCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		pass, detail := ch.run(&harness{ctx: rowCtx, bin: cfg.Bin, dir: dir, dialect: cfg.Dialect})
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
			c, err := Dial(t.bin, Options{Args: t.args("-acp"), Dir: t.dir})
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
			c, err := Dial(t.bin, Options{Args: t.args("-acp"), Dir: t.dir})
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
		name:  "job-control-does-not-depend-on-the-route",
		claim: "the same script gets the same shell here as through -c: a job a session started is one it can signal (#1814)",
		run: func(t *harness) (bool, string) {
			// The monitor is turned on in the script on purpose. With it off
			// a background job shares the shell's own group and `%1` is
			// signaled as a process, which works on every route and would
			// grade a question this row is not asking. With it on the job
			// leads a group, and reaching a group is the capability the two
			// routes were given differently.
			//
			// `kill -0` delivers nothing, so what is measured is whether the
			// job could be *named*; the job is ended after, so the row leaves
			// nothing running. The job's own streams go nowhere, so the `-c`
			// run's pipes close when its shell does: a background child that
			// inherited them would hold the read open for as long as it ran,
			// and a regression here would then be reported thirty seconds
			// late rather than at once.
			const script = `set -m; sleep 30 >/dev/null 2>&1 & kill -0 %1; echo "probe=$?"; kill %1`
			// A dialect is named because the *core* refuses `set -m` as an
			// axis nothing answered, which is this binary talking rather than
			// a shell behaving. A run that chose a dialect keeps it: this row
			// used to force bash unconditionally, so inside a zsh run it was
			// reporting on a shell nobody had asked about (#2258).
			args := t.dialectOr("bash")

			cmd := exec.CommandContext(t.ctx, t.bin, append(append([]string{}, args...), "-c", script)...)
			cmd.Dir = t.dir
			// The status is deliberately not checked. A shell with no
			// terminal is entitled to refuse the monitor — real zsh answers
			// `can't change option: -m` and exits 1, and ours matches it — and
			// that refusal is one of the two answers this row compares, not a
			// broken run.
			piped, _ := cmd.CombinedOutput()

			c, s, err := t.dial(args, always(AllowOnce))
			if err != nil {
				return false, err.Error()
			}
			defer func() { _ = c.Close() }()
			if _, err := c.Prompt(t.ctx, s, script); err != nil {
				return false, err.Error()
			}
			session := c.Output("stdout") + c.Output("stderr")

			// The claim is that the route does not change which shell you
			// get, so the two are compared against each other rather than
			// against one hardcoded answer. That holds the row to something
			// falsifiable in a dialect that cannot turn the monitor on: a
			// session that swallowed the refusal, or worded it differently,
			// or hung, is a difference between the routes exactly as much as
			// a lost job is.
			if !sameLines(string(piped), session) {
				return false, fmt.Sprintf("the routes gave the same script different shells: -c said %q, the session said %q", string(piped), session)
			}
			if strings.Contains(string(piped), "probe=0") {
				return true, "both routes named the job and signaled it"
			}
			return true, "this dialect refuses the monitor without a terminal, as the shell it imitates does, and both routes refuse it alike: " + strings.TrimSpace(string(piped))
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
	args := append(t.args(flags...),
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
