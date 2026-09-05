// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/internal/acp"
)

// handlerFuncs is a Handler assembled from two closures, so a test can say
// what it answers without a type of its own each time.
type handlerFuncs struct {
	handle func(context.Context, string, json.RawMessage) (any, error)
	notify func(context.Context, string, json.RawMessage)
}

func (h handlerFuncs) Handle(ctx context.Context, m string, p json.RawMessage) (any, error) {
	if h.handle == nil {
		return nil, acp.Errorf(acp.CodeMethodNotFound, "no method %q", m)
	}
	return h.handle(ctx, m, p)
}

func (h handlerFuncs) Notify(ctx context.Context, m string, p json.RawMessage) {
	if h.notify != nil {
		h.notify(ctx, m, p)
	}
}

// pair wires two connections to each other over an in-memory stream and serves
// both, which is the arrangement the real thing has: a client and an agent
// that each answer and each call out.
func pair(t *testing.T, agent, client acp.Handler) (agentConn, clientConn *acp.Conn) {
	t.Helper()
	a, c := net.Pipe()
	agentConn = acp.NewConn(a, a, agent)
	clientConn = acp.NewConn(c, c, client)
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = agentConn.Serve(ctx) }()
	go func() { defer wg.Done(); _ = clientConn.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		// Closing the streams is what ends a read; cancellation alone cannot
		// interrupt one, which Serve documents.
		_ = a.Close()
		_ = c.Close()
		wg.Wait()
	})
	return agentConn, clientConn
}

func TestCallReturnsTheResult(t *testing.T) {
	t.Parallel()
	agent := handlerFuncs{handle: func(_ context.Context, m string, p json.RawMessage) (any, error) {
		if m != "session/prompt" {
			return nil, acp.Errorf(acp.CodeMethodNotFound, "no method %q", m)
		}
		var req acp.PromptRequest
		if err := json.Unmarshal(p, &req); err != nil {
			return nil, err
		}
		return acp.PromptResponse{StopReason: "end_turn:" + req.SessionID}, nil
	}}
	_, client := pair(t, agent, nil)

	var got acp.PromptResponse
	err := client.Call(t.Context(), "session/prompt", acp.PromptRequest{SessionID: "s1"}, &got)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got.StopReason != "end_turn:s1" {
		t.Errorf("stopReason = %q, want end_turn:s1", got.StopReason)
	}
}

func TestCallSurfacesTheErrorObject(t *testing.T) {
	t.Parallel()
	agent := handlerFuncs{handle: func(context.Context, string, json.RawMessage) (any, error) {
		return nil, acp.Errorf(acp.CodeInvalidParams, "no such session")
	}}
	_, client := pair(t, agent, nil)

	err := client.Call(t.Context(), "session/prompt", nil, nil)
	var rpcErr *acp.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("err = %v, want an *acp.Error", err)
	}
	if rpcErr.Code != acp.CodeInvalidParams {
		t.Errorf("code = %d, want %d", rpcErr.Code, acp.CodeInvalidParams)
	}
	if !strings.Contains(rpcErr.Message, "no such session") {
		t.Errorf("message = %q, want it to name the failure", rpcErr.Message)
	}
}

// A handler that returns a plain Go error must not put an invented code on the
// wire: there is no number that means "something went wrong in Go", so it is
// the internal error and the message survives.
func TestAPlainErrorBecomesAnInternalError(t *testing.T) {
	t.Parallel()
	agent := handlerFuncs{handle: func(context.Context, string, json.RawMessage) (any, error) {
		return nil, errors.New("the runner is gone")
	}}
	_, client := pair(t, agent, nil)

	err := client.Call(t.Context(), "session/prompt", nil, nil)
	var rpcErr *acp.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("err = %v, want an *acp.Error", err)
	}
	if rpcErr.Code != acp.CodeInternalError {
		t.Errorf("code = %d, want %d", rpcErr.Code, acp.CodeInternalError)
	}
	if !strings.Contains(rpcErr.Message, "the runner is gone") {
		t.Errorf("message = %q, want the Go error's text", rpcErr.Message)
	}
}

func TestNoHandlerAnswersMethodNotFound(t *testing.T) {
	t.Parallel()
	_, client := pair(t, nil, nil)

	err := client.Call(t.Context(), "session/load", nil, nil)
	var rpcErr *acp.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("err = %v, want an *acp.Error", err)
	}
	if rpcErr.Code != acp.CodeMethodNotFound {
		t.Errorf("code = %d, want %d", rpcErr.Code, acp.CodeMethodNotFound)
	}
}

func TestNotificationReachesTheHandlerAndIsNotAnswered(t *testing.T) {
	t.Parallel()
	got := make(chan string, 1)
	agent := handlerFuncs{notify: func(_ context.Context, m string, p json.RawMessage) {
		var n acp.CancelNotification
		_ = json.Unmarshal(p, &n)
		got <- m + " " + n.SessionID
	}}
	_, client := pair(t, agent, nil)

	if err := client.Notify(acp.MethodCancel, acp.CancelNotification{SessionID: "s7"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	// Received once: a second receive on a channel nothing else writes to
	// would hang the test rather than fail it.
	if saw, want := <-got, "session/cancel s7"; saw != want {
		t.Errorf("handler saw %q, want %q", saw, want)
	}
}

// The case the whole design of the read loop is for. An agent answers
// session/prompt by calling back to the client — a permission request — and
// waits for the reply inside its own handler. A handler running on the read
// loop could not receive that reply, and the two sides would sit waiting for
// each other forever.
func TestAHandlerMayCallBackWhileHandling(t *testing.T) {
	t.Parallel()
	var agentConn *acp.Conn
	agent := handlerFuncs{handle: func(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
		var resp acp.RequestPermissionResponse
		if err := agentConn.Call(ctx, acp.MethodRequestPermission, acp.RequestPermissionRequest{
			SessionID: "s1",
			ToolCall:  acp.ToolCall{ToolCallID: "call-1"},
			Options:   acp.PermissionOptions(),
		}, &resp); err != nil {
			return nil, err
		}
		return acp.PromptResponse{StopReason: resp.Outcome.OptionID}, nil
	}}
	client := handlerFuncs{handle: func(context.Context, string, json.RawMessage) (any, error) {
		return acp.RequestPermissionResponse{Outcome: acp.PermissionOutcome{
			Outcome: acp.OutcomeSelected, OptionID: acp.OptionAllowOnce,
		}}, nil
	}}
	var clientConn *acp.Conn
	agentConn, clientConn = pair(t, agent, client)

	var got acp.PromptResponse
	if err := clientConn.Call(t.Context(), acp.MethodPrompt, nil, &got); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got.StopReason != acp.OptionAllowOnce {
		t.Errorf("the handler saw %q, want the option the client chose", got.StopReason)
	}
}

// Two calls in flight at once come back to the right callers. A shell has more
// than one goroutine reaching the gate — a background job, each half of a
// pipeline — so this is the ordinary case rather than a stress test.
func TestConcurrentCallsAreMatchedByID(t *testing.T) {
	t.Parallel()
	agent := handlerFuncs{handle: func(_ context.Context, _ string, p json.RawMessage) (any, error) {
		var req acp.PromptRequest
		if err := json.Unmarshal(p, &req); err != nil {
			return nil, err
		}
		return acp.PromptResponse{StopReason: req.SessionID}, nil
	}}
	_, client := pair(t, agent, nil)

	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, n)
	answers := make([]string, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := string(rune('a' + i))
			var got acp.PromptResponse
			errs[i] = client.Call(t.Context(), acp.MethodPrompt, acp.PromptRequest{SessionID: id}, &got)
			answers[i] = got.StopReason
		}()
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("call %d: %v", i, errs[i])
		}
		if want := string(rune('a' + i)); answers[i] != want {
			t.Errorf("call %d answered %q, want %q", i, answers[i], want)
		}
	}
}

// A line that is not JSON is one bad message rather than the end of the
// connection: the transport is line-delimited exactly so that it can be.
func TestGarbageLineIsAnsweredAndTheConnectionSurvives(t *testing.T) {
	t.Parallel()
	a, c := net.Pipe()
	conn := acp.NewConn(a, a, handlerFuncs{handle: func(context.Context, string, json.RawMessage) (any, error) {
		return acp.PromptResponse{StopReason: acp.StopEndTurn}, nil
	}})
	go func() { _ = conn.Serve(t.Context()) }()
	t.Cleanup(func() { _ = a.Close(); _ = c.Close() })

	if _, err := io.WriteString(c, "{not json\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	dec := json.NewDecoder(c)
	var first struct {
		ID    any        `json:"id"`
		Error *acp.Error `json:"error"`
	}
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if first.Error == nil || first.Error.Code != acp.CodeParseError {
		t.Fatalf("answer = %+v, want a parse error", first.Error)
	}
	if first.ID != nil {
		t.Errorf("id = %v, want null: the id was inside what would not parse", first.ID)
	}

	// And the connection still works.
	if _, err := io.WriteString(c, `{"jsonrpc":"2.0","id":1,"method":"session/prompt"}`+"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	var second struct {
		Result acp.PromptResponse `json:"result"`
	}
	if err := dec.Decode(&second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if second.Result.StopReason != acp.StopEndTurn {
		t.Errorf("stopReason = %q, want %q", second.Result.StopReason, acp.StopEndTurn)
	}
}

// A call outstanding when the connection ends fails, and fails
// distinguishably. The gate turns this into a denial, and it could not if the
// call simply waited: a shell whose permission request never returns is a
// shell that has stopped.
func TestPendingCallsFailWhenTheConnectionEnds(t *testing.T) {
	t.Parallel()
	a, c := net.Pipe()
	// A handler that never answers, so the call is still outstanding when the
	// stream is closed underneath it.
	blocked := make(chan struct{})
	peer := acp.NewConn(c, c, handlerFuncs{handle: func(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
		close(blocked)
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	conn := acp.NewConn(a, a, nil)
	go func() { _ = peer.Serve(t.Context()) }()
	go func() { _ = conn.Serve(t.Context()) }()

	done := make(chan error, 1)
	go func() { done <- conn.Call(context.Background(), acp.MethodPrompt, nil, nil) }()
	<-blocked
	_ = a.Close()
	if err := <-done; !errors.Is(err, acp.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
	_ = c.Close()
}

// A call whose context is canceled gives up, and the connection stays usable
// for the calls that did not.
func TestCallGivesUpWhenItsContextIsCancelled(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	agent := handlerFuncs{handle: func(context.Context, string, json.RawMessage) (any, error) {
		<-release
		return acp.PromptResponse{StopReason: acp.StopEndTurn}, nil
	}}
	_, client := pair(t, agent, nil)
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- client.Call(ctx, acp.MethodPrompt, nil, nil) }()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// A response for an id nobody is waiting on is dropped rather than ending the
// connection. It is a reply to a call that has already given up, which the
// test above makes an ordinary occurrence.
func TestAnUnmatchedResponseIsIgnored(t *testing.T) {
	t.Parallel()
	a, c := net.Pipe()
	conn := acp.NewConn(a, a, handlerFuncs{handle: func(context.Context, string, json.RawMessage) (any, error) {
		return acp.PromptResponse{StopReason: acp.StopEndTurn}, nil
	}})
	go func() { _ = conn.Serve(t.Context()) }()
	t.Cleanup(func() { _ = a.Close(); _ = c.Close() })

	if _, err := io.WriteString(c, `{"jsonrpc":"2.0","id":99,"result":{}}`+"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := io.WriteString(c, `{"jsonrpc":"2.0","id":1,"method":"session/prompt"}`+"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	var got struct {
		Result acp.PromptResponse `json:"result"`
	}
	if err := json.NewDecoder(c).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Result.StopReason != acp.StopEndTurn {
		t.Errorf("stopReason = %q, want the connection to have survived", got.Result.StopReason)
	}
}

// Notifications arrive in the order they were sent, and that order is their
// meaning: session updates are a stream, and the chunks of what a command
// wrote are only what the command wrote if they stay in sequence. Handing each
// one to a goroutine delivers them in whatever order the scheduler picks,
// which is what this pins down.
func TestNotificationsAreDeliveredInOrder(t *testing.T) {
	t.Parallel()
	const n = 200
	got := make(chan string, n)
	agent := handlerFuncs{notify: func(_ context.Context, _ string, p json.RawMessage) {
		var v struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.Unmarshal(p, &v)
		got <- v.SessionID
	}}
	_, client := pair(t, agent, nil)

	for i := range n {
		if err := client.Notify(acp.MethodCancel,
			acp.CancelNotification{SessionID: strconv.Itoa(i)}); err != nil {
			t.Fatalf("Notify: %v", err)
		}
	}
	for i := range n {
		if saw := <-got; saw != strconv.Itoa(i) {
			t.Fatalf("notification %d arrived as %q — the stream was reordered", i, saw)
		}
	}
}
