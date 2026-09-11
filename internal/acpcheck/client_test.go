// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acpcheck

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The instrument's own machinery, tested against an agent that is not a
// shell.
//
// The graded rows are a target you run, for the reason `make smoke`'s session
// is: they launch a real binary and time real children. What can be pinned
// here is the half that would make those rows lie — framing, correlation,
// whether a non-protocol line is noticed — and pinning it needs an agent this
// test controls completely, including one that misbehaves. So the test binary
// re-executes itself as a scripted agent: `fakeAgent` below is the whole of
// it, and every case says what that agent should do.

// The environment variable that turns this test binary into an agent, and the
// script it should follow.
const fakeAgentEnv = "ACPCHECK_FAKE_AGENT"

func TestMain(m *testing.M) {
	if script := os.Getenv(fakeAgentEnv); script != "" {
		fakeAgent(script)
		return
	}
	// And the same trick for the scripted agent the client rows drive: it is
	// the instrument's own code, so it is graded here rather than only by a
	// row that needs a shell to be correct first.
	if script := os.Getenv(envAgentScript); script != "" {
		os.Exit(RunAgent(script))
	}
	os.Exit(m.Run())
}

// fakeAgent reads requests and answers them the way the named script says.
//
// It is deliberately not built on the real agent: an instrument tested
// against the implementation it grades can only prove they agree.
func fakeAgent(script string) {
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		var m struct {
			ID     json.Number     `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(in.Bytes(), &m) != nil {
			continue
		}
		if m.Method == "" { // an answer to something we asked
			continue
		}
		switch script {
		case "impure":
			// The failure every client suffers at once: something that is
			// not a message, on the stream that carries them.
			fmt.Println("warning: this line is not JSON-RPC")
		case "slow":
			// Answers nothing at all, which is the other way an agent
			// breaks a client.
			continue
		}
		switch m.Method {
		case "initialize":
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentInfo":{"name":"fake","version":"1"}}}`+"\n", m.ID)
		case "session/new":
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"s1"}}`+"\n", m.ID)
		case "session/prompt":
			if script == "asks" {
				// Announce, ask, and report what the answer was — the
				// three-message shape a permission costs.
				fmt.Println(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"tool_call","toolCallId":"t1","title":"write /tmp/x","kind":"edit","status":"pending"}}}`)
				fmt.Println(`{"jsonrpc":"2.0","id":99,"method":"session/request_permission","params":{"sessionId":"s1","toolCall":{"toolCallId":"t1"},"options":[{"optionId":"allow-once","kind":"allow_once"},{"optionId":"reject-once","kind":"reject_once"}]}}`)
				// Wait for the client's answer before finishing the turn.
				for in.Scan() {
					var a struct {
						Result struct {
							Outcome struct {
								OptionID string `json:"optionId"`
							} `json:"outcome"`
						} `json:"result"`
					}
					if json.Unmarshal(in.Bytes(), &a) == nil && a.Result.Outcome.OptionID != "" {
						fmt.Printf(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":%q},"_meta":{"sh.blairham.github.com/stream":"stdout"}}}}`+"\n",
							"answered:"+a.Result.Outcome.OptionID)
						break
					}
				}
			}
			if script == "streams" {
				fmt.Println(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"out\n"},"_meta":{"sh.blairham.github.com/stream":"stdout"}}}}`)
				fmt.Println(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"err\n"},"_meta":{"sh.blairham.github.com/stream":"stderr"}}}}`)
			}
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"stopReason":"end_turn"}}`+"\n", m.ID)
		case "refuse":
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"error":{"code":-32600,"message":"no"}}`+"\n", m.ID)
		}
	}
}

// dialFake connects to this test binary running as the named script.
func dialFake(t *testing.T, script string, opts Options) *Client {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	opts.Env = append(os.Environ(), fakeAgentEnv+"="+script)
	opts.Dir = t.TempDir()
	c, err := Dial(self, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestTheHandshakeAndSessionAreReadFromTheWire(t *testing.T) {
	c := dialFake(t, "plain", Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	in, err := c.Initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if in.ProtocolVersion != 1 || in.AgentInfo.Name != "fake" {
		t.Fatalf("initialize read back %+v", in)
	}
	s, err := c.NewSession(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s != "s1" {
		t.Fatalf("session id %q", s)
	}
}

func TestAPermissionRequestIsAnsweredWithTheOptionTheCallerChose(t *testing.T) {
	// The client must echo an option the agent offered, and must report both
	// what was asked and what it said — a row that asserts on a refusal has
	// nothing to assert on otherwise.
	c := dialFake(t, "asks", Options{Answer: always(RejectOnce)})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	s, err := c.NewSession(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prompt(ctx, s, "anything"); err != nil {
		t.Fatal(err)
	}
	asks := c.Asks()
	if len(asks) != 1 {
		t.Fatalf("asks = %v, want one", asks)
	}
	if asks[0].Title != "write /tmp/x" || asks[0].Answered != RejectOnce {
		t.Fatalf("ask = %+v", asks[0])
	}
	if got := c.Output("stdout"); !strings.Contains(got, "answered:"+RejectOnce) {
		t.Fatalf("the agent saw %q", got)
	}
}

func TestTheTitleComesFromTheToolCallThatAnnouncedIt(t *testing.T) {
	// The permission request carries only an id; a row that greps titles
	// would see nothing if this lookup broke, and would report "not gated"
	// for a shell that gated it.
	c := dialFake(t, "asks", Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustInitialize(t, c, ctx)
	s, _ := c.NewSession(ctx, t.TempDir())
	_, _ = c.Prompt(ctx, s, "anything")
	if calls := c.ToolCalls(); len(calls) != 1 || calls[0].Title != "write /tmp/x" {
		t.Fatalf("tool calls = %+v", calls)
	}
}

func TestALineThatIsNotProtocolIsRecordedRatherThanIgnored(t *testing.T) {
	// This is the purity row's only mechanism. If a stray line were skipped
	// quietly, that row could never fail.
	c := dialFake(t, "impure", Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if bad := c.Impure(); len(bad) != 1 || !strings.Contains(bad[0], "not JSON-RPC") {
		t.Fatalf("impure = %q", bad)
	}
}

func TestAProtocolErrorIsReturnedRatherThanHung(t *testing.T) {
	c := dialFake(t, "plain", Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, "refuse", nil); err == nil {
		t.Fatal("an error response came back as success")
	}
}

func TestAnAgentThatNeverAnswersTimesOutRatherThanHangingTheRun(t *testing.T) {
	// A hung agent must fail its own row and leave the rest of the table
	// reporting, which is only true if the wait is bounded.
	c := dialFake(t, "slow", Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := c.Initialize(ctx); err == nil {
		t.Fatal("a silent agent answered")
	}
}

func TestTheStreamsStaySeparate(t *testing.T) {
	c := dialFake(t, "streams", Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustInitialize(t, c, ctx)
	s, _ := c.NewSession(ctx, t.TempDir())
	_, _ = c.Prompt(ctx, s, "anything")
	if out, err := c.Output("stdout"), c.Output("stderr"); out != "out\n" || err != "err\n" {
		t.Fatalf("stdout %q, stderr %q", out, err)
	}
}

func TestTheMedianIsTheMiddleSampleAndNotTheMean(t *testing.T) {
	// One slow sample must not move the number: a process start on a loaded
	// machine produces exactly that, and a mean would report it as the cost.
	n := 0
	got := median(5, func() {
		n++
		if n == 3 {
			time.Sleep(50 * time.Millisecond)
		}
	})
	if got > 10*time.Millisecond {
		t.Fatalf("one slow sample moved the median to %s", got)
	}
}

func TestEveryAnnotationIsAttachedByMethodRatherThanByPosition(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, "the client opens"},
		{`{"jsonrpc":"2.0","id":1,"method":"session/request_permission","params":{}}`, "stops and asks"},
		{`{"jsonrpc":"2.0","id":1,"result":{"stopReason":"end_turn"}}`, "end_turn"},
		{`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"no"}}`, "refused: no"},
		{`not json at all`, ""},
	} {
		if got := annotate(c.line); !strings.Contains(got, c.want) {
			t.Errorf("annotate(%s) = %q, want something with %q", c.line, got, c.want)
		}
	}
}

func TestTheScriptedAgentRecordsWhatAClientGaveIt(t *testing.T) {
	// The agent mode is half of what the client rows measure: if its report
	// were wrong, those rows would be reporting the instrument rather than
	// the shell. So it is driven here by a client that answers, with no shell
	// in the arrangement at all.
	dir := t.TempDir()
	report := filepath.Join(dir, "report.json")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	served := 0
	c, err := Dial(self, Options{
		Dir: dir,
		Env: append(os.Environ(),
			envAgentScript+"=read", envAgentOut+"="+report, envAgentTarget+"=/somewhere/x"),
		Serve: func(method string, params json.RawMessage) (any, *RPCError) {
			served++
			if method != "fs/read_text_file" {
				return nil, &RPCError{Code: -32601, Message: method}
			}
			var p struct {
				Path string `json:"path"`
			}
			_ = json.Unmarshal(params, &p)
			if p.Path != "/somewhere/x" {
				return nil, &RPCError{Code: -1, Message: "asked for " + p.Path}
			}
			return map[string]any{"content": "what the file held"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	s, err := c.NewSession(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prompt(ctx, s, "go on then"); err != nil {
		t.Fatal(err)
	}
	_ = c.Close()

	b, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var r AgentReport
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if served != 1 {
		t.Fatalf("the agent made %d requests of the client, want 1", served)
	}
	if r.Refused || r.Got != "what the file held" {
		t.Fatalf("report = %+v", r)
	}
}

func TestTheScriptedAgentRecordsARefusalAsARefusal(t *testing.T) {
	// The row that matters most on the client side asserts on this field, so
	// a refusal recorded as an empty read would turn an enforced policy into
	// a quietly passing row.
	dir := t.TempDir()
	report := filepath.Join(dir, "report.json")
	self, _ := os.Executable()
	c, err := Dial(self, Options{
		Dir: dir,
		Env: append(os.Environ(),
			envAgentScript+"=read", envAgentOut+"="+report, envAgentTarget+"=/denied"),
		// Nil Serve: the client answers "I do not serve that", which is the
		// shape a refusal arrives in.
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustInitialize(t, c, ctx)
	s, _ := c.NewSession(ctx, dir)
	_, _ = c.Prompt(ctx, s, "go on then")
	_ = c.Close()

	b, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var r AgentReport
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if !r.Refused || r.Error == "" {
		t.Fatalf("report = %+v, want a refusal with a reason", r)
	}
}

// mustInitialize is the handshake where a test is not about the handshake.
//
// Written out rather than ignored: an initialize that failed leaves every
// later assertion in the test measuring a connection that was never open,
// and the failure it produces names the wrong thing.
func mustInitialize(t *testing.T, c *Client, ctx context.Context) {
	t.Helper()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
}
