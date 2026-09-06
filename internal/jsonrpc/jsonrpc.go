// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package jsonrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Package jsonrpc is JSON-RPC 2.0 over a newline-delimited stream: one side
// launches the other as a subprocess, messages cross on that process's
// standard input and standard output as UTF-8, one message per line, and
// neither side writes anything else there.
//
// **Nothing in here has a side.** A Conn is a peer: it answers what arrives
// and can call out at the same time, on the same stream. Which end launched
// which, and which methods either end serves, is the concern of the protocol
// built on top.
//
// It was written for ACP (docs/design/acp.md) and lives in a package of its
// own because a second protocol needs the same framing — see
// docs/design/plugins.md, which chose this transport over gRPC and gives the
// measured argument for it. A copy would have been a place for the decision to
// go stale.
//
// It is written here rather than taken from a library because the whole of it
// is below, and a dependency that arrives with a linter of its own (AGENTS.md)
// has to be worth more than the file it replaces. encoding/json does the part
// that is genuinely hard.

// The standard JSON-RPC 2.0 error codes, plus -32800, which is the code a
// request abandoned by its caller is answered with.
//
// The -32000 to -32099 block is reserved by the specification for the
// application, so a code in it belongs to the protocol using this package and
// not here: ACP's CodeAuthRequired is one, declared in internal/acp.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	CodeCancelled      = -32800
)

// Error is a JSON-RPC error object, and is what a handler returns when it
// wants to choose the code the peer sees. Any other error becomes an internal
// error, because a code invented from a Go error is a code that means nothing.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("jsonrpc %d: %s", e.Code, e.Message) }

// Errorf builds an Error with a formatted message.
func Errorf(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ErrClosed is what an outstanding call fails with when the connection ends
// underneath it. It is deliberately distinguishable, because a caller has to
// be able to tell "the peer is gone" from "the peer answered": an ACP
// permission request that ends this way is a denial, and a plugin call that
// ends this way is a command that visibly did not run.
var ErrClosed = errors.New("jsonrpc: connection closed")

// Handler answers what the peer sends. A request expects a result or an error;
// a notification expects nothing and cannot fail, because there is nobody to
// tell.
//
// The two are dispatched differently, and both ways round are deliberate.
//
// A request is handled on a goroutine of its own, which is not an
// optimization: a handler routinely does work that itself calls back to the
// peer — an ACP agent answering session/prompt asks the client's permission
// before a consequential action — and a handler running on the read loop
// could not receive the reply to its own call.
//
// A notification is handled on the read loop, in order. Notifications are a
// stream and their order is their meaning; a goroutine each would deliver the
// chunks of what a command wrote in whatever order the scheduler chose. So
// Notify must not block — it holds the connection while it runs — and a
// handler with slow work to do starts it and returns.
type Handler interface {
	Handle(ctx context.Context, method string, params json.RawMessage) (any, error)
	Notify(ctx context.Context, method string, params json.RawMessage)
}

// Conn is one JSON-RPC peer: it answers what arrives and can call out at the
// same time.
type Conn struct {
	r *bufio.Reader
	w io.Writer
	h Handler

	// writes serializes the writer. Every message is one line, and two
	// goroutines writing at once would interleave two of them into a line
	// that is neither.
	writes sync.Mutex

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan *incoming
	closed  bool
	// wg tracks the goroutines handling inbound requests, so that Serve does
	// not return while one of them still holds a call this connection is
	// meant to answer.
	wg sync.WaitGroup
}

// NewConn builds a peer that reads from r, writes to w, and hands what
// arrives to h. A nil handler answers every request with "method not found",
// which is what a peer that only calls out is.
func NewConn(r io.Reader, w io.Writer, h Handler) *Conn {
	return &Conn{r: bufio.NewReader(r), w: w, h: h, pending: map[int64]chan *incoming{}}
}

// incoming is a message as it arrives, before it is known which of the three
// kinds it is. A response carries an id and no method; a request carries both;
// a notification carries a method and no id.
type incoming struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Serve reads until the stream ends, dispatching as it goes.
//
// It returns nil at end of input, which is how the launching side says it is
// finished: it closes the subprocess's standard input. ctx cancellation does
// not interrupt a blocked read — there is no portable way to make it — so a
// caller that wants to stop early closes the reader.
func (c *Conn) Serve(ctx context.Context) error {
	defer c.shutdown()
	for {
		line, err := c.r.ReadBytes('\n')
		if len(line) > 0 {
			c.dispatch(ctx, line)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				c.wg.Wait()
				return nil
			}
			c.wg.Wait()
			return fmt.Errorf("jsonrpc: read: %w", err)
		}
	}
}

// dispatch routes one line.
//
// A line that is not JSON is answered with a parse error where that is
// possible and dropped where it is not, rather than ending the connection.
// The transport is line-delimited precisely so that one bad message is one bad
// message.
func (c *Conn) dispatch(ctx context.Context, line []byte) {
	var m incoming
	if err := json.Unmarshal(line, &m); err != nil {
		// No id to answer against — the id is inside the thing that would
		// not parse — so this is the JSON-RPC null-id case.
		c.writeError(nil, &Error{Code: CodeParseError, Message: "invalid JSON"})
		return
	}
	switch {
	case m.Method != "" && len(m.ID) > 0:
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.answer(ctx, m)
		}()
	case m.Method != "":
		// Notifications are handled *on the read loop*, in the order they
		// arrived, and that is the contract rather than a shortcut. A
		// session's updates are a stream — the chunks of what a command wrote,
		// in the order it wrote them — and handing each to a goroutine of its
		// own scrambles them: two chunks appended by two goroutines arrive in
		// whichever order the scheduler picks, so `echo one; echo two` reaches
		// a person as either. It was written that way first and the agent's
		// own end-to-end tests caught it.
		//
		// The cost is that a notification handler must not block: it holds the
		// connection while it runs. That is the right way round. A handler
		// that needs to do something slow starts it and returns, where a
		// stream that arrives out of order cannot be repaired by anybody.
		if c.h != nil {
			c.h.Notify(ctx, m.Method, m.Params)
		}
	case len(m.ID) > 0:
		c.deliver(&m)
	default:
		c.writeError(nil, &Error{Code: CodeInvalidRequest, Message: "neither a request nor a response"})
	}
}

// answer runs a handler and writes whatever it produced.
func (c *Conn) answer(ctx context.Context, m incoming) {
	if c.h == nil {
		c.writeError(m.ID, Errorf(CodeMethodNotFound, "no handler for %q", m.Method))
		return
	}
	result, err := c.h.Handle(ctx, m.Method, m.Params)
	if err != nil {
		var rpcErr *Error
		if !errors.As(err, &rpcErr) {
			// A Go error carries no code, and inventing one would put a
			// number on the wire that means nothing to the peer.
			rpcErr = &Error{Code: CodeInternalError, Message: err.Error()}
		}
		c.writeError(m.ID, rpcErr)
		return
	}
	c.writeResult(m.ID, result)
}

// deliver hands a response to whoever is waiting for it.
//
// A response for an id nobody is waiting on is dropped: it is either a reply
// to a call that has already given up or a peer inventing ids, and neither is
// worth ending a connection over.
func (c *Conn) deliver(m *incoming) {
	var id int64
	if err := json.Unmarshal(m.ID, &id); err != nil {
		return
	}
	c.mu.Lock()
	ch, ok := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ok {
		ch <- m
	}
}

// Call sends a request and waits for its answer, unmarshaling the result into
// result when one is given.
//
// It blocks, and that is the contract rather than an implementation detail: a
// permission request has to hold the goroutine that reached the action until a
// person answers it, and a builtin whose body is in another process has to
// hold the goroutine running the command.
func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	ch := make(chan *incoming, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	c.nextID++
	id := c.nextID
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.write(outRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}
	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("jsonrpc: %s: %w", method, ctx.Err())
	case m := <-ch:
		if m == nil {
			return ErrClosed
		}
		if m.Error != nil {
			return m.Error
		}
		if result == nil || len(m.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(m.Result, result); err != nil {
			return fmt.Errorf("jsonrpc: %s: %w", method, err)
		}
		return nil
	}
}

// Notify sends a notification, which has no id and no answer.
func (c *Conn) Notify(method string, params any) error {
	return c.write(outNotification{JSONRPC: "2.0", Method: method, Params: params})
}

// shutdown fails every outstanding call. A call left waiting on a connection
// that has ended would wait forever, and a goroutine parked on a peer that no
// longer exists is the leak class #690 is the standing example of.
func (c *Conn) shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for id, ch := range c.pending {
		delete(c.pending, id)
		close(ch)
	}
}

type outRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type outNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type outResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
}

type outErrorResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   *Error          `json:"error"`
}

func (c *Conn) writeResult(id json.RawMessage, result any) {
	if err := c.write(outResponse{JSONRPC: "2.0", ID: id, Result: result}); err != nil {
		// A result that could not be marshaled is this side's bug, and the
		// peer is still owed an answer to the id it sent.
		c.writeError(id, Errorf(CodeInternalError, "%v", err))
	}
}

func (c *Conn) writeError(id json.RawMessage, e *Error) {
	if id == nil {
		id = json.RawMessage("null")
	}
	// Nothing useful to do about a failed write to the one stream there is;
	// the connection is over and Serve will say so.
	_ = c.write(outErrorResponse{JSONRPC: "2.0", ID: id, Error: e})
}

// write marshals one message and puts it on the stream as a single line.
//
// Marshaled before the lock rather than under it, so that an expensive
// encoding does not hold the stream, and because a message that will not
// marshal must not leave a half-written line behind. encoding/json escapes any
// newline inside a string, which is what the transport requires: a message
// must not contain an embedded newline.
func (c *Conn) write(m any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("jsonrpc: marshal: %w", err)
	}
	b = append(b, '\n')
	c.writes.Lock()
	defer c.writes.Unlock()
	if _, err := c.w.Write(b); err != nil {
		return fmt.Errorf("jsonrpc: write: %w", err)
	}
	return nil
}
