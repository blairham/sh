// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"

	"github.com/blairham/sh/internal/jsonrpc"
)

// A script's `read` reaching a person, and the one thing that must not happen
// while it does: a question per external command.
//
// The agent side has no terminal, so a session's standard input was the null
// device and `read` got end of file. elicitation/create is the protocol's own
// answer, and what stood in the way was one level below the protocol: a
// shell's input is inherited by every command it runs, and os/exec reads a
// non-file one on the child's behalf whether or not the child ever reads it.
// interp.Runner.ChildStdin is the split that settles it (#934), and the rows
// below are what it has to buy — a question for a `read`, and none for
// anything else.

// asker is a client that can reach a person: it claims form elicitation and
// answers with a line, a decline, or a refusal.
type asker struct {
	conn *jsonrpc.Conn

	mu    sync.Mutex
	forms []acp.CreateElicitationRequest
	out   strings.Builder
	// lines are the answers, in order. Once they run out the person
	// declines, which is what lets a `while read` loop end.
	lines []string
	// refuse answers the method with an error, as a client that claimed the
	// capability and cannot serve it would.
	refuse bool
}

func (a *asker) Handle(_ context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case acp.MethodRequestPermission:
		return acp.RequestPermissionResponse{Outcome: acp.PermissionOutcome{
			Outcome: acp.OutcomeSelected, OptionID: acp.OptionAllowOnce,
		}}, nil
	case acp.MethodCreateElicitation:
		var req acp.CreateElicitationRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		a.forms = append(a.forms, req)
		if a.refuse {
			return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "this client will not")
		}
		if len(a.lines) == 0 {
			return acp.CreateElicitationResponse{Action: acp.ElicitDecline}, nil
		}
		line := a.lines[0]
		a.lines = a.lines[1:]
		return acp.CreateElicitationResponse{
			Action:  acp.ElicitAccept,
			Content: map[string]any{"line": line},
		}, nil
	}
	return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "client has no %s", method)
}

func (a *asker) Notify(_ context.Context, method string, params json.RawMessage) {
	if method != acp.MethodSessionUpdate {
		return
	}
	var raw struct {
		Update map[string]any `json:"update"`
	}
	if err := json.Unmarshal(params, &raw); err != nil {
		return
	}
	if raw.Update["sessionUpdate"] != acp.UpdateAgentMessageChunk {
		return
	}
	content, _ := raw.Update["content"].(map[string]any)
	text, _ := content["text"].(string)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.out.WriteString(text)
}

func (a *asker) asked() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.forms)
}

func (a *asker) said() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.out.String()
}

func (a *asker) form(i int) acp.CreateElicitationRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.forms[i]
}

// connectAsker is `connect` for a client that states what it can serve, which
// the shared helper does not: whether a session's input can reach a person is
// read off the capability, so a case about it has to be able to claim one.
func connectAsker(t *testing.T, a *asker, claim bool) (dir string, run func(string) string) {
	t.Helper()
	dir = t.TempDir()
	x, y := net.Pipe()
	agent := acp.NewAgent(driver.Shell{Name: "sh"}, acp.Implementation{Name: "sh", Version: "test"})
	a.conn = jsonrpc.NewConn(y, y, a)
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = agent.Serve(ctx, x, x) }()
	go func() { defer wg.Done(); _ = a.conn.Serve(ctx) }()
	t.Cleanup(func() {
		agent.Close(context.Background())
		cancel()
		_ = x.Close()
		_ = y.Close()
		wg.Wait()
	})

	caps := acp.ClientCapabilities{}
	if claim {
		caps.Elicitation = &acp.ElicitationCapabilities{Form: &acp.ElicitationMode{}}
	}
	var init acp.InitializeResponse
	if err := a.conn.Call(t.Context(), acp.MethodInitialize, acp.InitializeRequest{
		ProtocolVersion: acp.Version,
		ClientInfo:      &acp.Implementation{Name: "test-client", Version: "1"},
		Capabilities:    caps,
	}, &init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var made acp.NewSessionResponse
	if err := a.conn.Call(t.Context(), acp.MethodNewSession, acp.NewSessionRequest{Cwd: dir}, &made); err != nil {
		t.Fatalf("session/new: %v", err)
	}
	run = func(src string) string {
		var resp acp.PromptResponse
		if err := a.conn.Call(t.Context(), acp.MethodPrompt, acp.PromptRequest{
			SessionID: made.SessionID, Prompt: []acp.ContentBlock{acp.TextBlock(src)},
		}, &resp); err != nil {
			t.Fatalf("session/prompt: %v", err)
		}
		return resp.StopReason
	}
	return dir, run
}

// TestAScriptsReadReachesAPerson: the row #934 is named for. One `read`, one
// form, and the line the person wrote is the variable's value.
func TestAScriptsReadReachesAPerson(t *testing.T) {
	t.Parallel()
	a := &asker{lines: []string{"a person typed this"}}
	_, run := connectAsker(t, a, true)
	if got := run(`read -r line; echo "got=[$line]"`); got != acp.StopEndTurn {
		t.Fatalf("stop reason %q", got)
	}
	if want := "got=[a person typed this]\n"; a.said() != want {
		t.Errorf("the shell wrote %q, want %q", a.said(), want)
	}
	if a.asked() != 1 {
		t.Fatalf("the client was asked %d times, want exactly 1", a.asked())
	}
	// And the question says what it is: the form mode, one field, and the
	// session it belongs to — a client cannot render a question it cannot
	// place.
	q := a.form(0)
	if q.Mode != acp.ElicitForm || q.SessionID == "" || q.Message == "" {
		t.Errorf("the form was %+v, want a form-mode question in a session", q)
	}
	if q.RequestedSchema == nil || len(q.RequestedSchema.Properties) != 1 {
		t.Errorf("the schema was %+v, want exactly one field", q.RequestedSchema)
	}
}

// TestAnExternalCommandIsNotAQuestion: the whole reason the field on the
// runner exists. os/exec reads a non-file standard input on the child's
// behalf, so without the split this snippet would put a form in front of a
// person for `/bin/echo` — which reads nothing.
func TestAnExternalCommandIsNotAQuestion(t *testing.T) {
	t.Parallel()
	a := &asker{lines: []string{"unused"}}
	_, run := connectAsker(t, a, true)
	run(`/bin/echo hi; /bin/echo again`)
	if a.asked() != 0 {
		t.Errorf("the client was asked %d times, want none: no command read anything", a.asked())
	}
	if !strings.Contains(a.said(), "hi\n") {
		t.Errorf("the shell wrote %q, want the commands to have run", a.said())
	}
}

// TestAClientThatCannotAskLeavesReadAtEndOfFile: the behavior every session
// had before this, kept for every client that could not have answered anyway.
// A capability is read before it is used, which is the rule the file and
// terminal capabilities are already held to.
func TestAClientThatCannotAskLeavesReadAtEndOfFile(t *testing.T) {
	t.Parallel()
	a := &asker{lines: []string{"never reached"}}
	_, run := connectAsker(t, a, false)
	run(`read -r line; echo "st=$? line=[$line]"`)
	if a.asked() != 0 {
		t.Errorf("the client was asked %d times, want none: it claimed nothing", a.asked())
	}
	if want := "st=1 line=[]\n"; a.said() != want {
		t.Errorf("the shell wrote %q, want %q", a.said(), want)
	}
}

// TestDecliningIsEndOfFile: a person saying no is what a shell's input says
// when there is no more of it, so a `while read` loop ends rather than
// asking forever. Two lines then a decline, and the loop runs twice.
func TestDecliningIsEndOfFile(t *testing.T) {
	t.Parallel()
	a := &asker{lines: []string{"one", "two"}}
	_, run := connectAsker(t, a, true)
	run(`while read -r line; do echo "line=[$line]"; done; echo done`)
	if want := "line=[one]\nline=[two]\ndone\n"; a.said() != want {
		t.Errorf("the shell wrote %q, want %q", a.said(), want)
	}
	if a.asked() != 3 {
		t.Errorf("the client was asked %d times, want 3 — two lines and the decline that ends it", a.asked())
	}
}

// TestARefusedElicitationIsAskedOnce: a client that claimed the capability
// and then will not serve it is not asked again by every later read. The
// difference from a decline is the point — one is an answer and the other is
// a broken connection.
func TestARefusedElicitationIsAskedOnce(t *testing.T) {
	t.Parallel()
	a := &asker{refuse: true}
	_, run := connectAsker(t, a, true)
	run(`while read -r line; do echo "line=[$line]"; done; echo done`)
	if want := "done\n"; a.said() != want {
		t.Errorf("the shell wrote %q, want %q", a.said(), want)
	}
	if a.asked() != 1 {
		t.Errorf("the client was asked %d times, want 1", a.asked())
	}
}

// TestAStreamTheScriptNamedStillReachesAChild: the boundary the field must
// not cross, asked where it can be got wrong. The substitution is over the
// shell's *own* input, so a redirection and a pipe reach an external command
// as themselves — a rule that replaced whatever a child was about to inherit
// would leave `cat < f` reading the empty stream and printing nothing, at
// status 0, which is the silent wrong answer this project exists to avoid.
//
// Both halves are external commands on purpose. A builtin `read` never goes
// near the child path, so a case written with one passes whether or not the
// rule is there.
func TestAStreamTheScriptNamedStillReachesAChild(t *testing.T) {
	t.Parallel()
	a := &asker{lines: []string{"not this"}}
	dir, run := connectAsker(t, a, true)
	if err := os.WriteFile(filepath.Join(dir, "in.txt"), []byte("from the file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(`/bin/cat < in.txt; /bin/echo piped | /bin/cat`)
	if want := "from the file\npiped\n"; a.said() != want {
		t.Errorf("the shell wrote %q, want %q", a.said(), want)
	}
	if a.asked() != 0 {
		t.Errorf("the client was asked %d times, want none: the script named both streams", a.asked())
	}
}

// TestABuiltinReadOfANamedStreamIsNotAQuestionEither: the same boundary on
// the other side of the split, where the shell itself is the reader.
func TestABuiltinReadOfANamedStreamIsNotAQuestionEither(t *testing.T) {
	t.Parallel()
	a := &asker{lines: []string{"not this"}}
	dir, run := connectAsker(t, a, true)
	if err := os.WriteFile(filepath.Join(dir, "in.txt"), []byte("from the file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(`read -r line < in.txt; echo "got=[$line]"`)
	if want := "got=[from the file]\n"; a.said() != want {
		t.Errorf("the shell wrote %q, want %q", a.said(), want)
	}
	if a.asked() != 0 {
		t.Errorf("the client was asked %d times, want none: the script named the stream", a.asked())
	}
}
